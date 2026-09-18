package components

import (
	"image"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestToastComponentBottomRightPlacementAndClipping(t *testing.T) {
	layout := (Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, 80, 24))
	if layout.Frame.Max != image.Pt(79, 23) {
		t.Fatalf("toast bottom-right = %v, want (79,23)", layout.Frame.Max)
	}
	if layout.Frame.Dx() != len("Message copied")+4 || layout.Frame.Dy() != 3 {
		t.Fatalf("toast frame = %v", layout.Frame)
	}

	narrow := (Toast{Text: "a message too wide"}).Layout(image.Rect(0, 0, 8, 2))
	if !narrow.Frame.In(image.Rect(0, 0, 8, 2)) || narrow.Content.Empty() {
		t.Fatalf("clipped toast = %#v", narrow)
	}
}

func TestToastWideGraphemesCountAsTwoCells(t *testing.T) {
	for _, text := range []string{"❤️", "👨‍👩‍👧‍👦", "👍", "🔥", "界"} {
		if got := ansi.StringWidth(text); got != 2 {
			t.Errorf("StringWidth(%q) = %d, want 2", text, got)
		}
		layout := (Toast{Text: text}).Layout(image.Rect(0, 0, 80, 24))
		if got, want := layout.Frame.Dx(), 2+4; got != want {
			t.Errorf("frame width for %q = %d, want %d", text, got, want)
		}
		if layout.Frame.Max != image.Pt(79, 23) {
			t.Errorf("frame Max for %q = %v, want (79,23)", text, layout.Frame.Max)
		}
	}
}

func TestToastMixedTextFrameWidthFormula(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	for _, text := range []string{"a", "a界b", "👨‍👩‍👧‍👦x🔥"} {
		layout := (Toast{Text: text}).Layout(bounds)
		want := min(bounds.Dx(), max(3, ansi.StringWidth(text)+4))
		if got := layout.Frame.Dx(); got != want {
			t.Errorf("frame width for %q = %d, want %d", text, got, want)
		}
	}
}

func TestToastBottomRightMaxExact(t *testing.T) {
	layout := (Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, 80, 24))
	if layout.Frame.Max != image.Pt(79, 23) {
		t.Errorf("frame Max = %v, want (79,23)", layout.Frame.Max)
	}
	if layout.Content.Max != image.Pt(78, 22) {
		t.Errorf("content Max = %v, want (78,22)", layout.Content.Max)
	}
}

func TestToastTinyBoundsSafeContained(t *testing.T) {
	for _, bounds := range []image.Rectangle{
		image.Rect(0, 0, 1, 1),
		image.Rect(0, 0, 2, 2),
		image.Rect(0, 0, 3, 3),
		image.Rect(0, 0, 4, 2),
		image.Rect(0, 0, 8, 2),
		image.Rect(0, 0, 0, 0),
	} {
		layout := (Toast{Text: "Message copied"}).Layout(bounds)
		if !layout.Frame.In(bounds) {
			t.Errorf("%v: frame %v not contained in bounds", bounds, layout.Frame)
		}
		if !layout.Content.In(bounds) {
			t.Errorf("%v: content %v not contained in bounds", bounds, layout.Content)
		}
		if layout.Frame.Dx() < 0 || layout.Frame.Dy() < 0 {
			t.Errorf("%v: negative frame dims %v", bounds, layout.Frame)
		}
	}
}
