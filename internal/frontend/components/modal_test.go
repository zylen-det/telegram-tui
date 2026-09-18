package components

import (
	"image"
	"testing"
)

func TestActionModalCentersClipsAndPublishesListRows(t *testing.T) {
	layout := (Modal{Title: "Message actions", Items: []Item{{Label: "Copy"}}, Selected: 0}).Layout(image.Rect(0, 0, 80, 24))
	if layout.Frame != image.Rect(26, 9, 54, 15) {
		t.Fatalf("centered frame = %v", layout.Frame)
	}
	if len(layout.Rows) != 1 || layout.Rows[0].Rect.Empty() || !layout.Rows[0].Selected {
		t.Fatalf("rows = %#v", layout.Rows)
	}
	if !layout.Close.In(layout.Frame) {
		t.Fatalf("close = %v outside frame %v", layout.Close, layout.Frame)
	}

	wide := (Modal{Items: []Item{{Label: "Result"}}, Width: 64}).Layout(image.Rect(0, 0, 100, 24))
	if wide.Frame.Dx() != 64 || wide.Frame.Min.X != 18 {
		t.Fatalf("preferred-width frame = %v", wide.Frame)
	}

	narrow := (Modal{Items: []Item{{Label: "Copy"}}}).Layout(image.Rect(0, 0, 5, 3))
	if !narrow.Frame.In(image.Rect(0, 0, 5, 3)) {
		t.Fatalf("narrow frame escaped bounds: %v", narrow.Frame)
	}
	for _, row := range narrow.Rows {
		if !row.Rect.In(narrow.Frame) {
			t.Fatalf("narrow row escaped frame: %v", row.Rect)
		}
	}
}
