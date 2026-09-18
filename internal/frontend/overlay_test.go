package frontend

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestOverlayReconcilesAfterRendererWriteAndSkipsUnchanged(t *testing.T) {
	path := writeOverlayPNG(t, "first.png", 4, 2)
	events := []string{}
	output := &overlayEventWriter{events: &events}
	images := &overlayImages{events: &events}
	overlay := NewOutputOverlay(output, images, func(int, int) image.Point { return image.Pt(1, 2) })
	bounds := image.Rect(10, 4, 50, 20)

	title := overlay.SetDesired(path, bounds, 80, 24, 1)
	if _, err := overlay.Write(overlayRendererFrame(title, "frame-one")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got, want := events, []string{"render:frame-one", "show", "render:"}; !equalOverlayEvents(got, want) {
		t.Fatalf("first write events = %#v, want %#v", got, want)
	}
	if got, want := images.rectangles[0], image.Rect(10, 7, 50, 17); got != want {
		t.Fatalf("fitted rectangle = %v, want %v", got, want)
	}

	if _, err := overlay.Write(overlayRendererFrame(title, "frame-two")); err != nil {
		t.Fatalf("unchanged Write() error = %v", err)
	}
	if images.shows != 1 || images.clears != 0 {
		t.Fatalf("unchanged placement shows/clears = %d/%d, want 1/0", images.shows, images.clears)
	}
}

func TestOverlayClearsAndReplacesInvalidatedPlacement(t *testing.T) {
	first := writeOverlayPNG(t, "first.png", 4, 2)
	second := writeOverlayPNG(t, "second.png", 3, 5)
	images := &overlayImages{}
	overlay := NewOutputOverlay(&bytes.Buffer{}, images, func(int, int) image.Point { return image.Pt(1, 2) })
	bounds := image.Rect(4, 3, 44, 19)

	mustWriteOverlay(t, overlay, overlay.SetDesired(first, bounds, 80, 24, 1))
	mustWriteOverlay(t, overlay, overlay.SetDesired(first, image.Rect(5, 3, 45, 19), 81, 24, 1))
	mustWriteOverlay(t, overlay, overlay.SetDesired(second, bounds, 80, 24, 1))
	mustWriteOverlay(t, overlay, overlay.SetDesired(second, bounds, 80, 24, 2))
	clearTitle := overlay.ClearDesired()
	mustWriteOverlay(t, overlay, clearTitle)
	mustWriteOverlay(t, overlay, clearTitle)

	if images.shows != 4 || images.clears != 4 {
		t.Fatalf("invalidated placement shows/clears = %d/%d, want 4/4", images.shows, images.clears)
	}
	if err := overlay.Clear(); err != nil {
		t.Fatalf("final Clear() error = %v", err)
	}
	if images.clears != 4 {
		t.Fatalf("inactive final Clear() calls = %d, want 4", images.clears)
	}
}

func TestOverlayViewUsesReservedModalContentAndReadyState(t *testing.T) {
	state := app.InitialState()
	state.Width = 100
	state.Height = 24
	state.Focus = app.FocusModal
	path := writeOverlayPNG(t, "modal.png", 76, 30)
	state.Modal = &app.ModalState{Title: "Image", Path: path, PreviousFocus: app.FocusConversation}
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	images := &overlayImages{}
	overlay := NewOutputOverlay(&bytes.Buffer{}, images, func(int, int) image.Point { return image.Pt(1, 2) })
	model.SetOutputOverlay(overlay)

	view := model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	frame, content := modalSurfaceRectangles(image.Rect(0, 0, state.Width, state.Height))
	if got := images.rectangles[0]; got != content {
		t.Fatalf("modal placement = %v, want reserved content %v", got, content)
	}
	if content.Min.X-frame.Min.X < 2 || content.Min.Y-frame.Min.Y < 2 || frame.Max.X-content.Max.X < 2 || frame.Max.Y-content.Max.Y < 2 {
		t.Fatalf("content %v does not preserve frame/title/close inset within %v", content, frame)
	}

	state.Modal.Loading = true
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	state.Modal.Loading = false
	state.Modal.Error = &domain.AppError{Kind: domain.ErrorMedia, Message: "unavailable"}
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	state.Modal.Error = nil
	state.Modal.Path = ""
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	state.Modal.Path = path
	state.Width = minimumWidth - 1
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	if images.shows != 1 || images.clears != 1 {
		t.Fatalf("non-ready modal shows/clears = %d/%d, want 1/1", images.shows, images.clears)
	}
}

func TestOverlayDecodeFailureIsSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-image-name.png")
	if err := os.WriteFile(path, []byte("private decoder payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	images := &overlayImages{}
	overlay := NewOutputOverlay(&rendered, images, nil)
	title := overlay.SetDesired(path, image.Rect(1, 1, 20, 10), 80, 24, 0)

	_, err := overlay.Write(overlayRendererFrame(title, "safe-frame"))
	if err == nil || !strings.Contains(err.Error(), "decode modal image") {
		t.Fatalf("Write() error = %v, want sanitized decode stage", err)
	}
	for _, secret := range []string{path, "private-image-name", "private decoder payload", "image: unknown format"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("decode error exposed %q: %v", secret, err)
		}
	}
	if !strings.Contains(rendered.String(), "safe-frame") ||
		!strings.HasSuffix(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) ||
		strings.Contains(rendered.String(), path) || strings.Contains(rendered.String(), "private decoder payload") || images.shows != 0 {
		t.Fatalf("decode failure rendered/shows = %q/%d", rendered.String(), images.shows)
	}
}

func TestOverlayClearFailureIsSanitized(t *testing.T) {
	path := writeOverlayPNG(t, "active.png", 2, 2)
	images := &overlayImages{clearErr: errors.New("raw kitty payload")}
	overlay := NewOutputOverlay(&bytes.Buffer{}, images, nil)
	mustWriteOverlay(t, overlay, overlay.SetDesired(path, image.Rect(1, 1, 10, 10), 80, 24, 0))

	err := overlay.Clear()
	if err == nil || !strings.Contains(err.Error(), "clear modal image") || strings.Contains(err.Error(), "raw kitty payload") {
		t.Fatalf("Clear() error = %v, want sanitized clear stage", err)
	}
}

func TestProductionSignalGracefulAppModelRoutesThroughQuit(t *testing.T) {
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	_, command := model.Update(ProcessQuitMsg{})
	if !engine.Snapshot().Quitting || command == nil {
		t.Fatal("process quit did not begin application shutdown")
	}
}

func TestProductionFatalShutdownMetadataRecordedBeforeQuit(t *testing.T) {
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), newBoundedAppRuntimeForModelTest(t))
	failure := domain.AppError{Kind: domain.ErrorInternal, Message: "fatal shutdown sentinel"}
	updated, command := model.Update(appEventMsg{event: app.ShutdownComplete{Error: &failure}})
	if command == nil || model.ShutdownError() == nil {
		t.Fatal("shutdown failure was not recorded before the graceful quit command")
	}
	if updated.(AppModel).ShutdownError() == nil {
		t.Fatal("shutdown failure metadata was not shared across model copies")
	}
}

func TestOverlayFrameBindingOldFlushDoesNotUseNewDesired(t *testing.T) {
	first := writeOverlayPNG(t, "old.png", 8, 2)
	second := writeOverlayPNG(t, "new.png", 2, 8)
	var rendered bytes.Buffer
	images := &overlayImages{}
	overlay := NewOutputOverlay(&rendered, images, func(int, int) image.Point { return image.Pt(1, 2) })
	bounds := image.Rect(0, 0, 20, 10)

	oldTitle := overlay.SetDesired(first, bounds, 80, 24, 10)
	newTitle := overlay.SetDesired(second, bounds, 80, 24, 20)
	if _, err := overlay.Write(overlayRendererFrame(oldTitle, "old-frame")); err != nil {
		t.Fatal("old frame write failed")
	}
	if images.shows != 1 || images.clears != 0 || overlay.active.selectedID != 10 {
		t.Fatal("old frame reconciled newer desired metadata")
	}
	if _, err := overlay.Write(overlayRendererFrame(newTitle, "new-frame")); err != nil {
		t.Fatal("new frame write failed")
	}
	if images.shows != 2 || images.clears != 1 || overlay.active.selectedID != 20 {
		t.Fatal("matching new frame did not reconcile newer metadata")
	}
	if strings.Contains(rendered.String(), first) || strings.Contains(rendered.String(), second) ||
		!strings.HasSuffix(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) {
		t.Fatal("private content reached terminal output or the title was not restored")
	}
}

func TestOverlayFrameBindingSurvivesBubbleTeaRenderer(t *testing.T) {
	path := writeOverlayPNG(t, "renderer.png", 4, 2)
	var rendered bytes.Buffer
	shown := make(chan struct{})
	images := &overlayImages{shown: shown}
	overlay := NewOutputOverlay(&rendered, images, nil)
	model := &overlayRendererModel{overlay: overlay, path: path}
	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(overlay),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithoutSignalHandler(),
	)
	result := make(chan error, 1)
	go func() {
		_, err := program.Run()
		result <- err
	}()

	select {
	case <-shown:
		program.Send(tea.QuitMsg{})
	case <-time.After(5 * time.Second):
		program.Kill()
		t.Fatal("Bubble Tea renderer did not flush the bound frame")
	}
	if err := <-result; err != nil {
		t.Fatal("Bubble Tea renderer program failed")
	}
	if strings.Contains(rendered.String(), path) ||
		!strings.Contains(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) {
		t.Fatal("renderer output leaked private content or omitted the title restore")
	}
}

func TestOverlaySelectedChatIdentitySurvivesReorderAndReplacement(t *testing.T) {
	path := writeOverlayPNG(t, "identity.png", 4, 2)
	state := app.InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = app.FocusModal
	state.Modal = &app.ModalState{Title: "Image", Path: path, PreviousFocus: app.FocusConversation}
	state.Chats = []domain.Chat{{ID: 11}, {ID: 22}}
	state.SelectedChat = 0
	model := newAppModelForTest(t, app.NewEngine(state), newBoundedAppRuntimeForModelTest(t))
	images := &overlayImages{}
	overlay := NewOutputOverlay(&bytes.Buffer{}, images, nil)
	model.SetOutputOverlay(overlay)

	view := model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	state.Chats = []domain.Chat{{ID: 22}, {ID: 11}}
	state.SelectedChat = 1
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	if images.shows != 1 || images.clears != 0 || overlay.active.selectedID != 11 {
		t.Fatal("chat reorder invalidated the selected chat identity")
	}

	state.Chats = []domain.Chat{{ID: 22}, {ID: 33}}
	model.engine = app.NewEngine(state)
	view = model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)
	if images.shows != 2 || images.clears != 1 || overlay.active.selectedID != 33 {
		t.Fatal("chat replacement did not invalidate the selected chat identity")
	}
}

func TestOverlayForgedMarkerCannotReconcileOrEvict(t *testing.T) {
	path := writeOverlayPNG(t, "forgery.png", 2, 2)
	images := &overlayImages{}
	var rendered bytes.Buffer
	overlay := NewOutputOverlay(&rendered, images, nil)
	validTitle := overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1)

	forgedTitle := overlayWindowTitle + overlayFrameMarker + "0000000000000001"
	if _, err := overlay.Write(overlayRendererFrame(forgedTitle, "forged")); err != nil {
		t.Fatal("forged marker write returned an error")
	}
	if images.shows != 0 || len(overlay.frames) != 1 {
		t.Fatal("generation-only forgery reconciled or deleted a pending binding")
	}

	unknownTitle := validTitle[:len(validTitle)-overlayFrameDigits] + strings.Repeat("f", overlayFrameDigits)
	if _, err := overlay.Write(overlayRendererFrame(unknownTitle, "unknown")); err != nil {
		t.Fatal("unknown authenticated marker write returned an error")
	}
	if images.shows != 0 || len(overlay.frames) != 1 {
		t.Fatal("unknown authenticated generation reconciled or evicted a pending binding")
	}
	if _, err := overlay.Write(overlayRendererFrame(validTitle, "valid")); err != nil {
		t.Fatal("valid marker write returned an error")
	}
	if images.shows != 1 || len(overlay.frames) != 0 {
		t.Fatal("valid authenticated marker did not reconcile its pending binding")
	}
	if strings.Contains(rendered.String(), path) || !strings.HasSuffix(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) {
		t.Fatal("marker output leaked private content or did not restore the normal title")
	}
}

func TestOverlaySplitMarkerEveryBoundary(t *testing.T) {
	path := writeOverlayPNG(t, "split.png", 2, 2)
	probe := NewOutputOverlay(io.Discard, nil, nil)
	markerLength := len(overlayRendererFrame(probe.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1), ""))
	for split := 1; split < markerLength; split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			images := &overlayImages{}
			var rendered bytes.Buffer
			overlay := NewOutputOverlay(&rendered, images, nil)
			marker := overlayRendererFrame(overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1), "")
			if n, err := overlay.Write(marker[:split]); n != split || err != nil {
				t.Fatalf("first split Write() = %d, %v", n, err)
			}
			if images.shows != 0 {
				t.Fatal("incomplete split marker reconciled")
			}
			if n, err := overlay.Write(marker[split:]); n != len(marker)-split || err != nil {
				t.Fatalf("second split Write() = %d, %v", n, err)
			}
			if images.shows != 1 || !strings.HasSuffix(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) {
				t.Fatal("completed split marker did not reconcile and restore title")
			}
		})
	}

	images := &overlayImages{}
	overlay := NewOutputOverlay(io.Discard, images, nil)
	marker := overlayRendererFrame(overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1), "")
	if _, err := overlay.Write(marker[:len(marker)/2]); err != nil {
		t.Fatal("pre-clear split write returned an error")
	}
	if err := overlay.Clear(); err != nil {
		t.Fatal("Clear returned an error")
	}
	if _, err := overlay.Write(marker[len(marker)/2:]); err != nil {
		t.Fatal("post-clear split write returned an error")
	}
	if images.shows != 0 || len(overlay.frames) != 0 {
		t.Fatal("Clear retained parser or frame-map state")
	}
}

func TestOverlayMultipleMarkersProcessMonotonically(t *testing.T) {
	path := writeOverlayPNG(t, "multiple.png", 2, 2)
	for _, order := range []struct {
		name    string
		reverse bool
	}{
		{name: "old-then-new"},
		{name: "new-then-old", reverse: true},
	} {
		t.Run(order.name, func(t *testing.T) {
			images := &overlayImages{}
			overlay := NewOutputOverlay(io.Discard, images, nil)
			oldTitle := overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1)
			newTitle := overlay.SetDesired(path, image.Rect(1, 0, 11, 10), 80, 24, 2)
			oldMarker := overlayRendererFrame(oldTitle, "old")
			newMarker := overlayRendererFrame(newTitle, "new")
			value := append(append([]byte(nil), oldMarker...), newMarker...)
			if order.reverse {
				value = append(append([]byte(nil), newMarker...), oldMarker...)
			}
			if n, err := overlay.Write(value); n != len(value) || err != nil {
				t.Fatalf("coalesced Write() = %d, %v", n, err)
			}
			if overlay.active.selectedID != 2 {
				t.Fatal("coalesced markers did not finish at the newest active generation")
			}
		})
	}
}

func TestOverlayFrameMapBounded(t *testing.T) {
	path := writeOverlayPNG(t, "bounded.png", 2, 2)
	images := &overlayImages{}
	overlay := NewOutputOverlay(io.Discard, images, nil)
	var oldest, newest string
	for generation := 1; generation <= 192; generation++ {
		title := overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, int64(generation))
		if generation == 1 {
			oldest = title
		}
		newest = title
	}
	if got := len(overlay.frames); got > 128 || got == 0 {
		t.Fatalf("pending frame map length = %d, want 1..128", got)
	}
	before := len(overlay.frames)
	if _, err := overlay.Write(overlayRendererFrame(oldest, "dropped")); err != nil {
		t.Fatal("dropped old marker write returned an error")
	}
	if images.shows != 0 || len(overlay.frames) != before {
		t.Fatal("dropped old generation reconciled or evicted newer entries")
	}
	if _, err := overlay.Write(overlayRendererFrame(newest, "newest")); err != nil {
		t.Fatal("newest marker write returned an error")
	}
	if overlay.active.selectedID != 192 {
		t.Fatal("newest retained frame did not reconcile")
	}
}

func TestOverlayWriterContract(t *testing.T) {
	path := writeOverlayPNG(t, "contract.png", 2, 2)
	writeErr := errors.New("writer sentinel")
	for _, test := range []struct {
		name  string
		write func(int, int) (int, error)
	}{
		{name: "illegal-negative", write: func(_, _ int) (int, error) { return -4, nil }},
		{name: "illegal-too-large", write: func(length, _ int) (int, error) { return length + 9, nil }},
		{name: "short-before-marker", write: func(_, _ int) (int, error) { return 1, nil }},
		{name: "short-after-marker", write: func(_, markerLength int) (int, error) { return markerLength, nil }},
		{name: "partial-error", write: func(_, _ int) (int, error) { return 2, writeErr }},
		{name: "full-error", write: func(length, _ int) (int, error) { return length, writeErr }},
	} {
		t.Run(test.name, func(t *testing.T) {
			images := &overlayImages{}
			var markerLength int
			writer := overlayContractWriter(func(value []byte) (int, error) {
				return test.write(len(value), markerLength)
			})
			overlay := NewOutputOverlay(writer, images, nil)
			marker := overlayRendererFrame(overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1), "")
			markerLength = len(marker)
			value := append(append([]byte(nil), marker...), "tail"...)
			n, err := overlay.Write(value)
			if n < 0 || n > len(value) {
				t.Fatalf("Write() returned illegal n = %d", n)
			}
			if test.name != "full-error" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("Write() error = %v, want io.ErrShortWrite", err)
			}
			if strings.Contains(test.name, "error") && !errors.Is(err, writeErr) {
				t.Fatalf("Write() error = %v, want writer sentinel", err)
			}
			if images.shows != 0 {
				t.Fatal("unsuccessful underlying write reconciled a marker")
			}
		})
	}

	images := &overlayImages{}
	var rendered bytes.Buffer
	overlay := NewOutputOverlay(&rendered, images, nil)
	value := overlayRendererFrame(overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1), "content")
	if n, err := overlay.Write(value); n != len(value) || err != nil {
		t.Fatalf("successful Write() = %d, %v", n, err)
	}
	if !bytes.HasPrefix(rendered.Bytes(), value) {
		t.Fatal("successful Write did not pass renderer bytes through unchanged")
	}
	if images.shows != 1 || !strings.HasSuffix(rendered.String(), string(overlayRendererFrame(overlayWindowTitle, ""))) {
		t.Fatal("successful authenticated marker did not reconcile and restore title")
	}
}

func TestOverlayShortWriteNilErrorNeverReconciles(t *testing.T) {
	path := writeOverlayPNG(t, "short.png", 2, 2)
	images := &overlayImages{}
	overlay := NewOutputOverlay(overlayShortWriter{n: 3}, images, nil)
	title := overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1)
	n, err := overlay.Write(overlayRendererFrame(title, "frame"))
	if n != 3 || !errors.Is(err, io.ErrShortWrite) || images.shows != 0 || images.clears != 0 {
		t.Fatal("short nil-error write reconciled or lost io.ErrShortWrite")
	}
}

func TestOverlayShortWritePartialErrorNeverReconciles(t *testing.T) {
	path := writeOverlayPNG(t, "partial.png", 2, 2)
	writeErr := errors.New("writer sentinel")
	images := &overlayImages{}
	overlay := NewOutputOverlay(overlayShortWriter{n: 2, err: writeErr}, images, nil)
	title := overlay.SetDesired(path, image.Rect(0, 0, 10, 10), 80, 24, 1)
	_, err := overlay.Write(overlayRendererFrame(title, "frame"))
	if !errors.Is(err, io.ErrShortWrite) || !errors.Is(err, writeErr) || images.shows != 0 || images.clears != 0 {
		t.Fatal("partial error write reconciled or did not preserve both errors")
	}
}

func TestOverlayInlineImagesReconcileAndClear(t *testing.T) {
	images := &overlayImages{}
	var rendered bytes.Buffer
	overlay := NewOutputOverlay(&rendered, images, nil)
	transmitOne := "\x1b_Ga=T,i=11,c=20,r=10,q=2;QUJD\x1b\\"
	transmitTwo := "\x1b_Ga=T,i=22,c=10,r=5,q=2;Rk5P\x1b\\"

	overlay.SetInlineImages([]inlinePlacement{
		{ImageID: 11, X: 5, Y: 3, Width: 20, Height: 10, Text: transmitOne},
		{ImageID: 22, X: 40, Y: 12, Width: 10, Height: 5, Text: transmitTwo},
	})
	mustWriteOverlay(t, overlay, overlay.ClearDesired())

	first := rendered.String()
	for _, want := range []string{
		"\x1b7\x1b[4;6H" + transmitOne + "\x1b8",
		"\x1b7\x1b[13;41H" + transmitTwo + "\x1b8",
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("first frame missing emit %q in %q", want, first)
		}
	}

	// a=T transient images are erased by any text written over them, so an
	// unchanged placement is re-emitted on every subsequent frame.
	mustWriteOverlay(t, overlay, overlay.ClearDesired())
	second := rendered.String()[len(first):]
	for _, want := range []string{"\x1b7\x1b[4;6H", "\x1b7\x1b[13;41H"} {
		if !strings.Contains(second, want) {
			t.Fatalf("second frame missing re-emit %q in %q", want, second)
		}
	}

	// Dropping an image deletes its Kitty id from the screen.
	overlay.SetInlineImages([]inlinePlacement{{ImageID: 22, X: 40, Y: 12, Width: 10, Height: 5, Text: transmitTwo}})
	mustWriteOverlay(t, overlay, overlay.ClearDesired())
	if !strings.Contains(rendered.String(), "\x1b_Ga=d,d=i,i=11,q=2\x1b\\") {
		t.Fatalf("stale inline image 11 was not deleted: %q", rendered.String())
	}

	// Clear deletes every remaining active inline image.
	if err := overlay.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if !strings.Contains(rendered.String(), "\x1b_Ga=d,d=i,i=22,q=2\x1b\\") {
		t.Fatalf("Clear did not delete inline image 22: %q", rendered.String())
	}
}

type overlayEventWriter struct {
	bytes.Buffer
	events *[]string
}

func (w *overlayEventWriter) Write(value []byte) (int, error) {
	content := value
	if bytes.HasPrefix(value, []byte("\x1b]2;")) {
		if end := bytes.IndexByte(value, '\a'); end >= 0 {
			content = value[end+1:]
		}
	}
	*w.events = append(*w.events, "render:"+string(content))
	return w.Buffer.Write(value)
}

type overlayImages struct {
	events     *[]string
	shows      int
	clears     int
	rectangles []image.Rectangle
	clearErr   error
	shown      chan struct{}
	shownOnce  sync.Once
}

func (i *overlayImages) Show(rectangle image.Rectangle, _ image.Image) error {
	i.shows++
	i.rectangles = append(i.rectangles, rectangle)
	if i.events != nil {
		*i.events = append(*i.events, "show")
	}
	if i.shown != nil {
		i.shownOnce.Do(func() { close(i.shown) })
	}
	return nil
}

func (i *overlayImages) Clear() error {
	i.clears++
	if i.events != nil {
		*i.events = append(*i.events, "clear")
	}
	return i.clearErr
}

func writeOverlayPNG(t *testing.T, name string, width, height int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	source := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	encodeErr := png.Encode(file, source)
	closeErr := file.Close()
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return path
}

func mustWriteOverlay(t *testing.T, overlay *OutputOverlay, title string) {
	t.Helper()
	if _, err := overlay.Write(overlayRendererFrame(title, "frame")); err != nil {
		t.Fatalf("overlay Write() error = %v", err)
	}
}

func overlayRendererFrame(title, content string) []byte {
	return []byte("\x1b]2;" + title + "\x07" + content)
}

type overlayRendererModel struct {
	overlay *OutputOverlay
	path    string
}

func (m *overlayRendererModel) Init() tea.Cmd { return nil }

func (m *overlayRendererModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m *overlayRendererModel) View() tea.View {
	view := tea.NewView("renderer-frame")
	view.AltScreen = true
	view.WindowTitle = m.overlay.SetDesired(m.path, image.Rect(0, 0, 20, 10), 80, 24, 7)
	return view
}

type overlayShortWriter struct {
	n   int
	err error
}

func (w overlayShortWriter) Write([]byte) (int, error) { return w.n, w.err }

type overlayContractWriter func([]byte) (int, error)

func (w overlayContractWriter) Write(value []byte) (int, error) { return w(value) }

func equalOverlayEvents(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
