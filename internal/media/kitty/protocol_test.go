package kitty

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"sync"
	"testing"
)

const (
	kittyAPCStart = "\x1b_G"
	kittyAPCEnd   = "\x1b\\"
	kittyRestore  = "\x1b8"
)

func TestTransmitPNGSmallImage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	source.Set(1, 0, color.RGBA{G: 0xff, A: 0xff})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatalf("encode expected PNG: %v", err)
	}
	wantPayload := base64.StdEncoding.EncodeToString(encoded.Bytes())
	want := "\x1b7\x1b[3;4H" +
		"\x1b_Ga=T,f=100,i=7,c=4,r=3,q=2,m=0;" + wantPayload + "\x1b\\\x1b8"

	var output bytes.Buffer
	if err := TransmitPNG(&output, 7, image.Rect(3, 2, 7, 5), source); err != nil {
		t.Fatalf("TransmitPNG() error = %v", err)
	}
	if got := output.String(); got != want {
		t.Fatalf("TransmitPNG() output mismatch\n got prefix: %q\nwant prefix: %q", truncate(got, 100), truncate(want, 100))
	}
}

func TestDeleteImage(t *testing.T) {
	var output bytes.Buffer
	if err := DeleteImage(&output, 23); err != nil {
		t.Fatalf("DeleteImage() error = %v", err)
	}
	const want = "\x1b_Ga=d,d=i,i=23,q=2\x1b\\"
	if got := output.String(); got != want {
		t.Fatalf("DeleteImage() = %q, want %q", got, want)
	}
}

func TestTransmitPNGChunksLargeImage(t *testing.T) {
	source := noisyImage(image.Rect(5, 7, 133, 135))
	rectangle := image.Rect(11, 13, 31, 23)

	var output bytes.Buffer
	if err := TransmitPNG(&output, 42, rectangle, source); err != nil {
		t.Fatalf("TransmitPNG() error = %v", err)
	}

	const framingPrefix = "\x1b7\x1b[14;12H"
	framed := output.String()
	if !strings.HasPrefix(framed, framingPrefix) {
		t.Fatalf("output prefix = %q, want %q", truncate(framed, len(framingPrefix)), framingPrefix)
	}
	if !strings.HasSuffix(framed, kittyRestore) {
		t.Fatalf("output does not end with cursor restore: %q", truncate(framed, 100))
	}

	chunks := parseChunks(t, strings.TrimSuffix(strings.TrimPrefix(framed, framingPrefix), kittyRestore))
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want more than one", len(chunks))
	}

	var payload strings.Builder
	for i, chunk := range chunks {
		if len(chunk.payload) > chunkSize {
			t.Errorf("chunk %d payload length = %d, want <= %d", i, len(chunk.payload), chunkSize)
		}

		wantControls := "m=1"
		if i == 0 {
			wantControls = "a=T,f=100,i=42,c=20,r=10,q=2,m=1"
		}
		if i == len(chunks)-1 {
			wantControls = "m=0"
			if i == 0 {
				wantControls = "a=T,f=100,i=42,c=20,r=10,q=2,m=0"
			}
		}
		if chunk.controls != wantControls {
			t.Errorf("chunk %d controls = %q, want %q", i, chunk.controls, wantControls)
		}
		payload.WriteString(chunk.payload)
	}

	pngBytes, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatalf("decode concatenated base64 payload: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode transmitted PNG: %v", err)
	}
	wantBounds := image.Rect(0, 0, source.Bounds().Dx(), source.Bounds().Dy())
	if got := decoded.Bounds(); got != wantBounds {
		t.Fatalf("decoded bounds = %v, want %v", got, wantBounds)
	}
	for y := 0; y < wantBounds.Dy(); y++ {
		for x := 0; x < wantBounds.Dx(); x++ {
			got := color.NRGBAModel.Convert(decoded.At(x, y))
			want := color.NRGBAModel.Convert(source.At(x+source.Bounds().Min.X, y+source.Bounds().Min.Y))
			if got != want {
				t.Fatalf("decoded pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestTransmitPNGRejectsInvalidInputWithoutWriting(t *testing.T) {
	var typedNil *image.RGBA
	tests := []struct {
		name      string
		imageID   uint32
		rectangle image.Rectangle
		source    image.Image
	}{
		{name: "zero image ID", imageID: 0, rectangle: image.Rect(0, 0, 1, 1), source: image.NewRGBA(image.Rect(0, 0, 1, 1))},
		{name: "empty rectangle", imageID: 1, rectangle: image.Rect(0, 0, 0, 1), source: image.NewRGBA(image.Rect(0, 0, 1, 1))},
		{name: "negative rectangle X", imageID: 1, rectangle: image.Rect(-1, 0, 1, 1), source: image.NewRGBA(image.Rect(0, 0, 1, 1))},
		{name: "negative rectangle Y", imageID: 1, rectangle: image.Rect(0, -1, 1, 1), source: image.NewRGBA(image.Rect(0, 0, 1, 1))},
		{name: "nil source", imageID: 1, rectangle: image.Rect(0, 0, 1, 1), source: nil},
		{name: "typed nil source", imageID: 1, rectangle: image.Rect(0, 0, 1, 1), source: typedNil},
		{name: "empty source", imageID: 1, rectangle: image.Rect(0, 0, 1, 1), source: image.NewRGBA(image.Rect(0, 0, 0, 1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := TransmitPNG(&output, tt.imageID, tt.rectangle, tt.source); err == nil {
				t.Fatal("TransmitPNG() error = nil, want validation error")
			}
			if output.Len() != 0 {
				t.Fatalf("TransmitPNG() wrote %d bytes for invalid input", output.Len())
			}
		})
	}
}

func TestDeleteImageRejectsZeroIDWithoutWriting(t *testing.T) {
	var output bytes.Buffer
	if err := DeleteImage(&output, 0); err == nil {
		t.Fatal("DeleteImage() error = nil, want validation error")
	}
	if output.Len() != 0 {
		t.Fatalf("DeleteImage() wrote %d bytes for invalid input", output.Len())
	}
}

func TestTransmitPNGRejectsNilWriterBeforeReadingSource(t *testing.T) {
	var typedNil *bytes.Buffer
	writers := []struct {
		name   string
		writer io.Writer
	}{
		{name: "nil interface", writer: nil},
		{name: "typed nil", writer: typedNil},
	}

	for _, tt := range writers {
		t.Run(tt.name, func(t *testing.T) {
			source := &boundsCountingImage{Image: image.NewRGBA(image.Rect(0, 0, 1, 1))}
			err := callWithoutPanic(t, func() error {
				return TransmitPNG(tt.writer, 1, image.Rect(0, 0, 1, 1), source)
			})
			if err == nil {
				t.Fatal("TransmitPNG() error = nil, want nil writer error")
			}
			if source.boundsCalls != 0 {
				t.Fatalf("TransmitPNG() called source.Bounds %d times before rejecting writer", source.boundsCalls)
			}
		})
	}
}

func TestDeleteImageRejectsNilWriter(t *testing.T) {
	var typedNil *bytes.Buffer
	writers := []struct {
		name   string
		writer io.Writer
	}{
		{name: "nil interface", writer: nil},
		{name: "typed nil", writer: typedNil},
	}

	for _, tt := range writers {
		t.Run(tt.name, func(t *testing.T) {
			err := callWithoutPanic(t, func() error {
				return DeleteImage(tt.writer, 1)
			})
			if err == nil {
				t.Fatal("DeleteImage() error = nil, want nil writer error")
			}
		})
	}
}

func TestTransmitPNGRestoresCursorAfterPositionFailure(t *testing.T) {
	positionErr := errors.New("position failed")
	writer := &recordingWriter{
		fail: func(p []byte) error {
			if bytes.HasPrefix(p, []byte("\x1b[")) {
				return positionErr
			}
			return nil
		},
	}

	err := TransmitPNG(writer, 1, image.Rect(0, 0, 1, 1), image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if !errors.Is(err, positionErr) {
		t.Fatalf("TransmitPNG() error = %v, want position error", err)
	}
	if !writer.attempted(kittyRestore) {
		t.Fatal("TransmitPNG() did not attempt cursor restore after position failure")
	}
}

func TestTransmitPNGRestoresCursorAfterChunkFailure(t *testing.T) {
	chunkErr := errors.New("chunk failed")
	writer := &recordingWriter{
		fail: func(p []byte) error {
			if bytes.HasPrefix(p, []byte(kittyAPCStart)) {
				return chunkErr
			}
			return nil
		},
	}

	err := TransmitPNG(writer, 1, image.Rect(0, 0, 1, 1), image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if !errors.Is(err, chunkErr) {
		t.Fatalf("TransmitPNG() error = %v, want chunk error", err)
	}
	if !writer.attempted(kittyRestore) {
		t.Fatal("TransmitPNG() did not attempt cursor restore after chunk failure")
	}
}

func TestTransmitPNGReturnsRestoreError(t *testing.T) {
	restoreErr := errors.New("restore failed")
	writer := &recordingWriter{
		fail: func(p []byte) error {
			if string(p) == kittyRestore {
				return restoreErr
			}
			return nil
		},
	}

	err := TransmitPNG(writer, 1, image.Rect(0, 0, 1, 1), image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if !errors.Is(err, restoreErr) {
		t.Fatalf("TransmitPNG() error = %v, want restore error", err)
	}
}

type parsedChunk struct {
	controls string
	payload  string
}

type boundsCountingImage struct {
	image.Image
	boundsCalls int
}

func (i *boundsCountingImage) Bounds() image.Rectangle {
	i.boundsCalls++
	return i.Image.Bounds()
}

func callWithoutPanic(t *testing.T, call func() error) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("call panicked: %v", recovered)
		}
	}()
	return call()
}

func parseChunks(t *testing.T, framed string) []parsedChunk {
	t.Helper()
	var chunks []parsedChunk
	for len(framed) > 0 {
		if !strings.HasPrefix(framed, kittyAPCStart) {
			t.Fatalf("chunk framing starts with %q, want APC start", truncate(framed, 20))
		}
		framed = strings.TrimPrefix(framed, kittyAPCStart)
		end := strings.Index(framed, kittyAPCEnd)
		if end < 0 {
			t.Fatal("chunk framing has no APC terminator")
		}
		body := framed[:end]
		controls, payload, ok := strings.Cut(body, ";")
		if !ok {
			t.Fatalf("chunk body %q has no payload separator", truncate(body, 80))
		}
		chunks = append(chunks, parsedChunk{controls: controls, payload: payload})
		framed = framed[end+len(kittyAPCEnd):]
	}
	return chunks
}

func noisyImage(bounds image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(bounds)
	var value uint32 = 0x9e3779b9
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			value ^= value << 13
			value ^= value >> 17
			value ^= value << 5
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(value), G: uint8(value >> 8), B: uint8(value >> 16), A: 0xff,
			})
		}
	}
	return img
}

func truncate(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length]
}

type recordingWriter struct {
	mu     sync.Mutex
	writes [][]byte
	fail   func([]byte) error
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	copyOfP := bytes.Clone(p)
	w.writes = append(w.writes, copyOfP)
	if w.fail != nil {
		if err := w.fail(copyOfP); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *recordingWriter) attempted(value string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, write := range w.writes {
		if string(write) == value {
			return true
		}
	}
	return false
}

func (w *recordingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(bytes.Join(w.writes, nil))
}

var _ io.Writer = (*recordingWriter)(nil)
