package frontend

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zylen-det/telegram-tui/internal/media/kitty"
)

const (
	overlayWindowTitle = "telegram-tui"
	overlayFrameMarker = ";overlay-frame="
	overlayFrameDigits = 16
	overlayNonceBytes  = 16
	overlayFrameLimit  = 128
)

var overlayFallbackNonce atomic.Uint64

// deleteInlineImage deletes one Kitty graphics image by id; a package-level
// indirection keeps overlay tests hermetic.
var deleteInlineImage = kitty.DeleteImage

type overlayImageManager interface {
	Show(image.Rectangle, image.Image) error
	Clear() error
}

type overlayPlacement struct {
	path       string
	bounds     image.Rectangle
	columns    int
	rows       int
	selectedID int64
}

// OutputOverlay serializes terminal rendering and Kitty image commands. Kitty
// writes must target the underlying writer passed here, not OutputOverlay.
type OutputOverlay struct {
	mu            sync.Mutex
	writer        io.Writer
	images        overlayImageManager
	cellPixels    func(columns, rows int) image.Point
	desired       overlayPlacement
	active        overlayPlacement
	nonce         string
	nextFrame     uint64
	applied       uint64
	frames        map[uint64]overlayPlacement
	observer      []byte
	inlineDesired []inlinePlacement
	inlineActive  map[uint32]inlinePlacement
}

// Read preserves the terminal file shape required by Bubble Tea. Renderer
// output is never read during normal operation; delegation keeps the wrapper a
// transparent terminal handle for capability checks.
func (o *OutputOverlay) Read(value []byte) (int, error) {
	if o == nil {
		return 0, io.EOF
	}
	if reader, ok := o.writer.(io.Reader); ok {
		return reader.Read(value)
	}
	return 0, io.EOF
}

// Close deliberately does not close the process-owned stdout handle.
func (o *OutputOverlay) Close() error { return nil }

// Fd preserves the terminal identity of the wrapped renderer output. Bubble
// Tea detects terminal size and resize events through this method.
func (o *OutputOverlay) Fd() uintptr {
	if o == nil {
		return ^uintptr(0)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if file, ok := o.writer.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// NewOutputOverlay creates a serialized renderer output with modal image
// reconciliation. A nil cell-pixel resolver uses the 1x2 fallback geometry.
func NewOutputOverlay(writer io.Writer, images overlayImageManager, cellPixels func(columns, rows int) image.Point) *OutputOverlay {
	if writer == nil {
		writer = io.Discard
	}
	return &OutputOverlay{
		writer:     writer,
		images:     images,
		cellPixels: cellPixels,
		nonce:      newOverlayNonce(),
	}
}

// SetDesired records the modal placement to reconcile after the next renderer
// write. selectedID invalidates an image retained across a selected-chat change.
func (o *OutputOverlay) SetDesired(path string, bounds image.Rectangle, columns, rows int, selectedID int64) string {
	if o == nil {
		return overlayWindowTitle
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if path == "" || bounds.Empty() {
		o.desired = overlayPlacement{}
		return o.bindFrameLocked()
	}
	o.desired = overlayPlacement{
		path:       path,
		bounds:     bounds,
		columns:    columns,
		rows:       rows,
		selectedID: selectedID,
	}
	return o.bindFrameLocked()
}

// ClearDesired removes the wanted placement. An active image is cleared after
// the next renderer write, leaving the text modal shell visible throughout.
func (o *OutputOverlay) ClearDesired() string {
	if o == nil {
		return overlayWindowTitle
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.desired = overlayPlacement{}
	return o.bindFrameLocked()
}

// SetInlineImages records the Kitty thumbnail placements to emit after the next
// renderer write. It binds no new frame generation; the modal marker bound by
// SetDesired/ClearDesired drives reconciliation.
func (o *OutputOverlay) SetInlineImages(placements []inlinePlacement) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inlineDesired = placements
}

// Write serializes a renderer frame with the Kitty commands derived from it.
func (o *OutputOverlay) Write(value []byte) (int, error) {
	if o == nil {
		return 0, errors.New("frontend: nil output overlay")
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	written, err := o.writer.Write(value)
	clamped := min(max(written, 0), len(value))
	if written != clamped || clamped != len(value) {
		o.observer = nil
		return clamped, errors.Join(err, io.ErrShortWrite)
	}
	if err != nil {
		o.observer = nil
		return clamped, err
	}

	for _, generation := range o.observeMarkersLocked(value) {
		reconcileErr := o.reconcileFrameLocked(generation)
		restoreErr := o.restoreTitleLocked()
		if markerErr := errors.Join(reconcileErr, restoreErr); markerErr != nil {
			return len(value), markerErr
		}
	}
	return len(value), nil
}

// Clear removes both the desired and active placement under the output lock.
func (o *OutputOverlay) Clear() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.desired = overlayPlacement{}
	o.inlineDesired = nil
	clear(o.frames)
	o.observer = nil
	o.applied = 0
	if err := o.clearLocked(); err != nil {
		return err
	}
	return o.clearInlineLocked()
}

func (o *OutputOverlay) bindFrameLocked() string {
	o.nextFrame++
	if o.nextFrame == 0 {
		clear(o.frames)
		o.observer = nil
		o.applied = 0
		o.nextFrame = 1
	}
	if o.frames == nil {
		o.frames = make(map[uint64]overlayPlacement)
	}
	o.frames[o.nextFrame] = o.desired
	for len(o.frames) > overlayFrameLimit {
		oldest := o.nextFrame
		for generation := range o.frames {
			oldest = min(oldest, generation)
		}
		delete(o.frames, oldest)
	}
	return fmt.Sprintf("%s%s%s:%0*x", overlayWindowTitle, overlayFrameMarker, o.nonce, overlayFrameDigits, o.nextFrame)
}

func newOverlayNonce() string {
	value := make([]byte, overlayNonceBytes)
	if _, err := cryptorand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	// Authentication should fail closed only if the platform random source
	// fails. Mix per-process and monotonic values for that explicit fallback.
	fallback := fmt.Sprintf("%d:%d:%d", os.Getpid(), time.Now().UnixNano(), overlayFallbackNonce.Add(1))
	digest := sha256.Sum256([]byte(fallback))
	return hex.EncodeToString(digest[:overlayNonceBytes])
}

func (o *OutputOverlay) observeMarkersLocked(value []byte) []uint64 {
	prefix := []byte("\x1b]2;" + overlayWindowTitle + overlayFrameMarker + o.nonce + ":")
	markerLength := len(prefix) + overlayFrameDigits + 1
	buffer := make([]byte, 0, len(o.observer)+len(value))
	buffer = append(buffer, o.observer...)
	buffer = append(buffer, value...)
	o.observer = nil

	var generations []uint64
	for len(buffer) > 0 {
		start := bytes.Index(buffer, prefix)
		if start < 0 {
			o.observer = overlayPrefixSuffix(buffer, prefix)
			break
		}
		buffer = buffer[start:]
		if len(buffer) < markerLength {
			o.observer = append([]byte(nil), buffer...)
			break
		}
		digitsEnd := len(prefix) + overlayFrameDigits
		generation, err := strconv.ParseUint(string(buffer[len(prefix):digitsEnd]), 16, 64)
		if err != nil || generation == 0 || buffer[digitsEnd] != '\a' {
			buffer = buffer[1:]
			continue
		}
		generations = append(generations, generation)
		buffer = buffer[markerLength:]
	}
	return generations
}

func overlayPrefixSuffix(value, prefix []byte) []byte {
	maximum := min(len(value), len(prefix)-1)
	for length := maximum; length > 0; length-- {
		if bytes.Equal(value[len(value)-length:], prefix[:length]) {
			return append([]byte(nil), value[len(value)-length:]...)
		}
	}
	return nil
}

func (o *OutputOverlay) reconcileFrameLocked(generation uint64) error {
	desired, exists := o.frames[generation]
	if !exists || generation <= o.applied {
		return nil
	}
	// Emit inline thumbnails before the modal image so the modal layers above
	// them; both write after the frame text.
	if err := o.reconcileInlineLocked(); err != nil {
		return err
	}
	if err := o.reconcileLocked(desired); err != nil {
		return err
	}
	o.applied = generation
	for frame := range o.frames {
		if frame <= generation {
			delete(o.frames, frame)
		}
	}
	return nil
}

func (o *OutputOverlay) restoreTitleLocked() error {
	value := []byte("\x1b]2;" + overlayWindowTitle + "\a")
	written, err := o.writer.Write(value)
	clamped := min(max(written, 0), len(value))
	if written != clamped || clamped != len(value) {
		return errors.Join(err, io.ErrShortWrite)
	}
	return err
}

func (o *OutputOverlay) reconcileLocked(desired overlayPlacement) error {
	if desired.path == "" || desired.bounds.Empty() {
		return o.clearLocked()
	}
	if o.active == desired {
		return nil
	}
	if err := o.clearLocked(); err != nil {
		return err
	}

	source, err := decodeOverlayImage(desired.path)
	if err != nil {
		return err
	}
	cellPixels := image.Pt(1, 2)
	if o.cellPixels != nil {
		cellPixels = o.cellPixels(desired.columns, desired.rows)
	}
	rectangle := fitOverlayImageRect(desired.bounds, source.Bounds().Size(), cellPixels)
	if rectangle.Empty() || o.images == nil {
		return nil
	}
	if err := o.images.Show(rectangle, source); err != nil {
		return errors.New("frontend: show modal image")
	}
	o.active = desired
	return nil
}

func (o *OutputOverlay) clearLocked() error {
	if o.active.path == "" || o.images == nil {
		o.active = overlayPlacement{}
		return nil
	}
	if err := o.images.Clear(); err != nil {
		return errors.New("frontend: clear modal image")
	}
	o.active = overlayPlacement{}
	return nil
}

// reconcileInlineLocked emits the current inline thumbnail placements after
// the frame text and deletes images no longer wanted. Kitty a=T (transient)
// images are erased whenever text is written over them, so every desired image
// is re-emitted on every frame from its cached transmit string; the active map
// only tracks ids for invalidation.
func (o *OutputOverlay) reconcileInlineLocked() error {
	desired := make(map[uint32]inlinePlacement, len(o.inlineDesired))
	for _, placement := range o.inlineDesired {
		desired[placement.ImageID] = placement
	}
	for imageID := range o.inlineActive {
		if _, keep := desired[imageID]; !keep {
			if err := deleteInlineImage(o.writer, imageID); err != nil {
				return errors.New("frontend: delete inline image")
			}
			delete(o.inlineActive, imageID)
		}
	}
	if len(o.inlineDesired) == 0 {
		return nil
	}
	if o.inlineActive == nil {
		o.inlineActive = make(map[uint32]inlinePlacement, len(o.inlineDesired))
	}
	for _, placement := range o.inlineDesired {
		if err := o.emitInlineLocked(placement); err != nil {
			return err
		}
		o.inlineActive[placement.ImageID] = placement
	}
	return nil
}

// emitInlineLocked preserves the terminal cursor, positions it at the image's
// top-left cell, emits the cached Kitty transmit, then restores the cursor.
func (o *OutputOverlay) emitInlineLocked(placement inlinePlacement) error {
	value := []byte(fmt.Sprintf("\x1b7\x1b[%d;%dH", placement.Y+1, placement.X+1) + placement.Text + "\x1b8")
	written, err := o.writer.Write(value)
	clamped := min(max(written, 0), len(value))
	if written != clamped || clamped != len(value) {
		return errors.Join(err, io.ErrShortWrite)
	}
	return err
}

// clearInlineLocked deletes every active inline image and resets the state.
func (o *OutputOverlay) clearInlineLocked() error {
	var errs []error
	for imageID := range o.inlineActive {
		if err := deleteInlineImage(o.writer, imageID); err != nil {
			errs = append(errs, errors.New("frontend: delete inline image"))
		}
	}
	o.inlineActive = nil
	return errors.Join(errs...)
}

func decodeOverlayImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("frontend: open modal image")
	}
	defer file.Close()
	source, _, err := image.Decode(file)
	if err != nil {
		return nil, errors.New("frontend: decode modal image")
	}
	return source, nil
}

func fitOverlayImageRect(bounds image.Rectangle, imageSize, cellPixels image.Point) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	if imageSize.X < 1 {
		imageSize.X = 1
	}
	if imageSize.Y < 1 {
		imageSize.Y = 1
	}
	if cellPixels.X < 1 {
		cellPixels.X = 1
	}
	if cellPixels.Y < 1 {
		cellPixels.Y = 2
	}

	width, height := bounds.Dx(), bounds.Dy()
	if imageSize.X*height*cellPixels.Y <= imageSize.Y*width*cellPixels.X {
		width = max(1, imageSize.X*height*cellPixels.Y/(imageSize.Y*cellPixels.X))
	} else {
		height = max(1, imageSize.Y*width*cellPixels.X/(imageSize.X*cellPixels.Y))
	}
	width = min(width, bounds.Dx())
	height = min(height, bounds.Dy())
	minimum := image.Pt(bounds.Min.X+(bounds.Dx()-width)/2, bounds.Min.Y+(bounds.Dy()-height)/2)
	return image.Rectangle{Min: minimum, Max: minimum.Add(image.Pt(width, height))}
}
