package frontend

import (
	"image"
	"strings"
	"testing"
)

func TestParseKittySourceDims(t *testing.T) {
	width, height, ok := parseKittySourceDims("\x1b_Ga=T,f=100,i=7,s=64,v=32,c=20,r=5,q=2;AAAA\x1b\\")
	if !ok || width != 64 || height != 32 {
		t.Fatalf("parse = (%d,%d,%v), want (64,32,true)", width, height, ok)
	}
	if _, _, ok := parseKittySourceDims("plain text"); ok {
		t.Fatal("plain text parsed, want false")
	}
	if _, _, ok := parseKittySourceDims("\x1b_Ga=T,f=100,i=7,q=2;AAAA\x1b\\"); ok {
		t.Fatal("missing s/v parsed, want false")
	}
}

func TestRewriteKittyTransmit(t *testing.T) {
	transmit := "\x1b_Ga=T,f=100,i=100,s=64,v=64,c=20,r=8,q=2;AAAA\x1b\\"
	rewritten, err := rewriteKittyTransmit(transmit, 42, 20, 3, 0, 40, 64, 24)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"i=42", "c=20", "r=3", "x=0", "y=40", "w=64", "h=24", "q=2", "s=64", "v=64", "a=T", "f=100",
	} {
		if !strings.Contains(rewritten, want) {
			t.Errorf("rewritten %q missing %q", rewritten, want)
		}
	}
	if !strings.HasSuffix(rewritten, ";AAAA\x1b\\") {
		t.Errorf("payload not preserved: %q", rewritten)
	}
	if strings.Contains(rewritten, "i=100") {
		t.Errorf("old id not replaced: %q", rewritten)
	}

	// Multi-chunk transmit: only the first APC control is rewritten, the
	// remaining chunks pass through untouched.
	multi := "\x1b_Ga=T,f=100,i=100,s=64,v=64,c=20,r=8,q=2,C=1,1;AAAA\x1b\\\x1b_Gm=1,q=2;BBBB\x1b\\\x1b_Gm=0,q=2;CCCC\x1b\\"
	multiRewritten, err := rewriteKittyTransmit(multi, 7, 10, 2, 4, 8, 12, 16)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"i=7", "c=10", "r=2", "x=4", "y=8", "w=12", "h=16", "\x1b_Gm=1,q=2;BBBB\x1b\\", "\x1b_Gm=0,q=2;CCCC\x1b\\"} {
		if !strings.Contains(multiRewritten, want) {
			t.Errorf("multi rewritten %q missing %q", multiRewritten, want)
		}
	}
	if strings.Contains(multiRewritten, "C=1,1") == false {
		t.Errorf("first control chunk marker dropped: %q", multiRewritten)
	}
}

func TestRewriteKittyTransmitErrors(t *testing.T) {
	if _, err := rewriteKittyTransmit("no control", 1, 2, 3, 4, 5, 6, 7); err == nil {
		t.Fatal("no-control transmit rewrote without error")
	}
	if _, err := rewriteKittyTransmit("\x1b_Ga=T,f=100", 1, 2, 3, 4, 5, 6, 7); err == nil {
		t.Fatal("unterminated control rewrote without error")
	}
}

func TestSubtractRect(t *testing.T) {
	base := image.Rect(0, 0, 10, 10)
	cases := []struct {
		name  string
		cover image.Rectangle
		want  []image.Rectangle
	}{
		{"no overlap", image.Rect(20, 20, 30, 30), []image.Rectangle{base}},
		{"above", image.Rect(2, -5, 8, -1), []image.Rectangle{base}},
		{"below", image.Rect(2, 15, 8, 20), []image.Rectangle{base}},
		{"full cover", image.Rect(-1, -1, 11, 11), nil},
		{"middle band splits", image.Rect(3, 3, 7, 7), []image.Rectangle{
			image.Rect(0, 0, 10, 3),
			image.Rect(0, 3, 3, 7),
			image.Rect(7, 3, 10, 7),
			image.Rect(0, 7, 10, 10),
		}},
		{"left edge", image.Rect(0, 2, 6, 8), []image.Rectangle{
			image.Rect(0, 0, 10, 2),
			image.Rect(6, 2, 10, 8),
			image.Rect(0, 8, 10, 10),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := subtractRect(base, tc.cover)
			if len(got) != len(tc.want) {
				t.Fatalf("regions = %v, want %v", got, tc.want)
			}
			for i := range got {
				if !got[i].Eq(tc.want[i]) {
					t.Errorf("region %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestDerivedInlineID(t *testing.T) {
	a := derivedInlineID(100, 0)
	b := derivedInlineID(100, 1)
	c := derivedInlineID(101, 0)
	if a == b || a == c || b == c {
		t.Fatalf("derived ids collide: %d %d %d", a, b, c)
	}
	if derivedInlineID(100, 0) != a {
		t.Fatal("derived id not deterministic")
	}
}

func TestClipInlinePlacementsViewport(t *testing.T) {
	transmit := "\x1b_Ga=T,f=100,i=1,s=64,v=64,c=20,r=8,q=2;AAAA\x1b\\"
	full := inlinePlacement{ImageID: 1, X: 5, Y: 5, Width: 20, Height: 8, Text: transmit}

	// Fully visible: unchanged with the original id and transmit.
	out := clipInlinePlacements([]inlinePlacement{full}, image.Rect(0, 0, 100, 100))
	if len(out) != 1 || out[0] != full {
		t.Fatalf("fully visible = %#v, want original", out)
	}

	// Fully outside: dropped.
	out = clipInlinePlacements([]inlinePlacement{full}, image.Rect(0, 0, 4, 4))
	if len(out) != 0 {
		t.Fatalf("outside placements = %d, want 0", len(out))
	}

	// Top half clipped (image scrolled down so its lower half stays in view):
	// one cropped placement at the visible origin.
	out = clipInlinePlacements([]inlinePlacement{full}, image.Rect(0, 0, 100, 9))
	if len(out) != 1 {
		t.Fatalf("partial placements = %d, want 1", len(out))
	}
	cropped := out[0]
	if cropped.X != 5 || cropped.Y != 5 || cropped.Width != 20 || cropped.Height != 4 {
		t.Fatalf("cropped = %#v, want {X:5 Y:5 W:20 H:4}", cropped)
	}
	if cropped.ImageID != derivedInlineID(1, 0) {
		t.Fatalf("cropped id = %d, want derived(1,0)", cropped.ImageID)
	}
	for _, want := range []string{"y=0", "h=32", "c=20", "r=4"} {
		if !strings.Contains(cropped.Text, want) {
			t.Fatalf("cropped transmit %q missing %q", cropped.Text, want)
		}
	}

	// Unparseable transmit on a partial placement: dropped safely.
	stub := inlinePlacement{ImageID: 2, X: 5, Y: 5, Width: 20, Height: 8, Text: "stub"}
	out = clipInlinePlacements([]inlinePlacement{stub}, image.Rect(0, 0, 100, 6))
	if len(out) != 0 {
		t.Fatalf("stub placements = %d, want 0", len(out))
	}
}

func TestClipInlinePlacementsModalOverlay(t *testing.T) {
	transmit := "\x1b_Ga=T,f=100,i=1,s=64,v=64,c=20,r=8,q=2;AAAA\x1b\\"
	full := inlinePlacement{ImageID: 1, X: 5, Y: 5, Width: 20, Height: 8, Text: transmit}
	viewport := image.Rect(0, 0, 100, 100)

	// Fully covered by the modal frame: dropped entirely.
	cover := image.Rect(0, 0, 100, 100)
	if out := clipInlinePlacementsWithOverlay([]inlinePlacement{full}, viewport, cover); len(out) != 0 {
		t.Fatalf("covered placements = %d, want 0", len(out))
	}

	// Modal frame cuts a centered band out of the image: up to four sub-bands
	// with distinct derived ids and cropped transmits.
	modal := image.Rect(10, 6, 12, 12)
	out := clipInlinePlacementsWithOverlay([]inlinePlacement{full}, viewport, modal)
	if len(out) < 2 {
		t.Fatalf("modal placements = %d, want >= 2", len(out))
	}
	seen := make(map[uint32]bool)
	for i, placement := range out {
		if seen[placement.ImageID] {
			t.Fatalf("duplicate derived id %d", placement.ImageID)
		}
		seen[placement.ImageID] = true
		if placement.X < 0 || placement.Y < 0 || !placement.rect().In(viewport) {
			t.Errorf("placement %d escapes viewport: %#v", i, placement)
		}
		if placement.rect().Overlaps(modal) {
			t.Errorf("placement %d overlaps modal: %v", i, placement.rect())
		}
		if !strings.Contains(placement.Text, "x=") || !strings.Contains(placement.Text, "h=") {
			t.Errorf("placement %d transmit not cropped: %q", i, placement.Text)
		}
	}

	// Non-overlapping modal: the placement is returned unchanged.
	apart := image.Rect(50, 50, 60, 60)
	if out := clipInlinePlacementsWithOverlay([]inlinePlacement{full}, viewport, apart); len(out) != 1 || out[0] != full {
		t.Fatalf("apart modal placements = %#v, want original", out)
	}
}

// TestClipInlinePlacementsPaneThenModal asserts the composeApplication chain:
// a pane-cropped placement (derived id) fed into the modal overlay clip keeps
// distinct ids and never overlaps the modal.
func TestClipInlinePlacementsPaneThenModal(t *testing.T) {
	transmit := "\x1b_Ga=T,f=100,i=1,s=64,v=64,c=20,r=8,q=2;AAAA\x1b\\"
	full := inlinePlacement{ImageID: 1, X: 5, Y: -5, Width: 20, Height: 8, Text: transmit}

	pane := image.Rect(0, 0, 100, 4)
	paned := clipInlinePlacements([]inlinePlacement{full}, pane)
	if len(paned) != 1 || paned[0].Y != 0 || paned[0].ImageID == 1 {
		t.Fatalf("pane clip = %#v, want one 4-row crop at y=0", paned)
	}

	modal := image.Rect(10, 1, 15, 3)
	out := clipInlinePlacementsWithOverlay(paned, pane, modal)
	if len(out) < 2 {
		t.Fatalf("modal placements = %d, want >= 2", len(out))
	}
	seen := make(map[uint32]bool)
	for _, placement := range out {
		if seen[placement.ImageID] {
			t.Fatalf("duplicate id %d", placement.ImageID)
		}
		seen[placement.ImageID] = true
		if placement.ImageID == paned[0].ImageID {
			t.Fatalf("modal clip reused the pane-cropped id %d", placement.ImageID)
		}
		if placement.rect().Overlaps(modal) {
			t.Errorf("placement overlaps modal: %v", placement.rect())
		}
	}
}
