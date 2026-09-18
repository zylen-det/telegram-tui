package testutil

import (
	"image"
	"testing"

	gotui "github.com/metaspartan/gotui/v5"
)

func TestBufferTextUsesBufferOriginAndTrimsRight(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(4, 7, 8, 9))
	buffer.SetCell(gotui.NewCell('A'), image.Pt(4, 7))
	buffer.SetCell(gotui.NewCell('界'), image.Pt(6, 8))

	if got, want := BufferText(buffer), "A\n  界\n"; got != want {
		t.Fatalf("BufferText() = %q, want %q", got, want)
	}
}

func TestBufferTextHandlesNilAndEmptyBuffers(t *testing.T) {
	for _, buffer := range []*gotui.Buffer{nil, gotui.NewBuffer(image.Rectangle{})} {
		if got := BufferText(buffer); got != "" {
			t.Fatalf("BufferText() = %q, want empty", got)
		}
	}
}
