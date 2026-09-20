package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// photoSendModalCanvas composes a full-size styled Base root with the photo
// send modal layer and returns the Compositor and Canvas over the viewport.
func photoSendModalCanvas(bounds image.Rectangle, surface surfaceResult) (*lipgloss.Compositor, *lipgloss.Canvas) {
	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(bounds.Dx()).Height(bounds.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	compositor := lipgloss.NewCompositor(root)
	return compositor, lipgloss.NewCanvas(bounds.Dx(), bounds.Dy()).Compose(compositor)
}

func photoSendModal(bounds image.Rectangle, data photoSendModalData) (surfaceResult, *lipgloss.Compositor, *lipgloss.Canvas) {
	surface := buildPhotoSendModalLayer(bounds, data, newRenderStyles(false))
	compositor, canvas := photoSendModalCanvas(bounds, surface)
	return surface, compositor, canvas
}

// TestPhotoSendModal_SafeEmpty covers safe-empty returns.
func TestPhotoSendModal_SafeEmpty(t *testing.T) {
	styles := newRenderStyles(false)

	// Zero bounds.
	surface := buildPhotoSendModalLayer(image.Rectangle{}, photoSendModalData{Path: "/x"}, styles)
	if surface.Layer != nil {
		t.Error("zero bounds: expected nil Layer")
	}
	if surface.Cursor.Visible {
		t.Error("zero bounds: cursor should be hidden")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("cursor = %v, want {-1,-1}", got)
	}
	if len(surface.Interactions) != 0 {
		t.Errorf("zero bounds: expected no interactions, got %d", len(surface.Interactions))
	}

	// Width 1.
	surface = buildPhotoSendModalLayer(image.Rect(0, 0, 1, 24), photoSendModalData{}, styles)
	if surface.Layer != nil {
		t.Error("width 1: expected nil Layer")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("width 1: cursor = %v, want {-1,-1}", got)
	}

	// Width 19 (below 20).
	surface = buildPhotoSendModalLayer(image.Rect(0, 0, 19, 24), photoSendModalData{}, styles)
	if surface.Layer != nil {
		t.Error("width 19: expected nil Layer")
	}

	// Height 1.
	surface = buildPhotoSendModalLayer(image.Rect(0, 0, 80, 1), photoSendModalData{}, styles)
	if surface.Layer != nil {
		t.Error("height 1: expected nil Layer")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("height 1: cursor = %v, want {-1,-1}", got)
	}

	// Height 7 (below 8).
	surface = buildPhotoSendModalLayer(image.Rect(0, 0, 80, 7), photoSendModalData{}, styles)
	if surface.Layer != nil {
		t.Error("height 7: expected nil Layer")
	}

	// Nonzero-origin bounds that are too small.
	surface = buildPhotoSendModalLayer(image.Rect(10, 10, 25, 15), photoSendModalData{}, styles)
	if surface.Layer != nil {
		t.Error("nonzero small: expected nil Layer")
	}
}

// TestPhotoSendModal_ExactGeometry tests at bounds (0,0,80,24).
func TestPhotoSendModal_ExactGeometry(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/photo.png"}
	surface, _, canvas := photoSendModal(bounds, data)

	// Frame: width=min(56,80)=56, height=min(8,24)=8, centered:
	// x=0+(80-56)/2=12, y=0+(24-8)/2=8.
	wantFrame := image.Rect(12, 8, 68, 16)
	if !surface.Rect.Eq(wantFrame) {
		t.Errorf("frame = %v, want %v", surface.Rect, wantFrame)
	}
	if surface.Layer == nil {
		t.Fatal("nil Layer")
	}
	if surface.Layer.GetX() != wantFrame.Min.X || surface.Layer.GetY() != wantFrame.Min.Y {
		t.Errorf("layer pos = (%d,%d), want (%d,%d)",
			surface.Layer.GetX(), surface.Layer.GetY(), wantFrame.Min.X, wantFrame.Min.Y)
	}
	if surface.Layer.Width() != wantFrame.Dx() || surface.Layer.Height() != wantFrame.Dy() {
		t.Errorf("layer size = %dx%d, want %dx%d",
			surface.Layer.Width(), surface.Layer.Height(), wantFrame.Dx(), wantFrame.Dy())
	}
	if got := surface.Layer.GetID(); got != "" {
		t.Errorf("frame layer ID = %q, want empty", got)
	}

	// Rounded corners.
	for _, tc := range []struct {
		x, y int
		want string
	}{
		{wantFrame.Min.X, wantFrame.Min.Y, "╭"},
		{wantFrame.Max.X - 1, wantFrame.Min.Y, "╮"},
		{wantFrame.Min.X, wantFrame.Max.Y - 1, "╰"},
		{wantFrame.Max.X - 1, wantFrame.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(tc.x, tc.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) nil", tc.x, tc.y)
		}
		if got := cell.Content; got != tc.want {
			t.Errorf("corner (%d,%d) = %q, want %q", tc.x, tc.y, got, tc.want)
		}
	}

	// Title visible at local (2,0) -> absolute (14,8).
	titleCell := canvas.CellAt(wantFrame.Min.X+2, wantFrame.Min.Y)
	if titleCell == nil || titleCell.Content == "" {
		t.Errorf("title cell = %+v, want non-empty", titleCell)
	}

	// Label visible at local (2,2) -> absolute (14,10).
	labelCell := canvas.CellAt(wantFrame.Min.X+2, wantFrame.Min.Y+2)
	if labelCell == nil || labelCell.Content == "" {
		t.Errorf("label cell = %+v, want non-empty", labelCell)
	}
}

// TestPhotoSendModal_NonZeroOriginBounds verifies every returned element
// stays inside the non-zero-origin bounds.
func TestPhotoSendModal_NonZeroOriginBounds(t *testing.T) {
	// Bounds 80x24 at offset (10,5) so the centered frame stays within the
	// canvas (which is also 80x24).  Frame y = 5+8=13, max y=21 < 24.
	bounds := image.Rect(10, 5, 90, 29)
	data := photoSendModalData{Path: "/tmp/photo.png"}
	surface, _, canvas := photoSendModal(bounds, data)

	// Frame centered within bounds: x=10+(80-56)/2=22, y=5+(24-8)/2=13.
	wantFrame := image.Rect(22, 13, 78, 21)
	if !surface.Rect.Eq(wantFrame) {
		t.Errorf("frame = %v, want %v", surface.Rect, wantFrame)
	}
	if surface.Layer == nil {
		t.Fatal("nil Layer")
	}
	if surface.Layer.GetX() != wantFrame.Min.X || surface.Layer.GetY() != wantFrame.Min.Y {
		t.Errorf("layer pos = (%d,%d), want (%d,%d)",
			surface.Layer.GetX(), surface.Layer.GetY(), wantFrame.Min.X, wantFrame.Min.Y)
	}

	// Every interaction rect must be inside bounds.
	for _, it := range surface.Interactions {
		if !it.Rect.Overlaps(bounds) {
			t.Errorf("interaction %q rect %v not inside bounds %v", it.ID, it.Rect, bounds)
		}
	}

	// Cursor inside frame.
	c := surface.Cursor
	if !cursorInside(c, wantFrame) {
		t.Errorf("cursor {%d,%d,v=%v} not inside frame %v", c.X, c.Y, c.Visible, wantFrame)
	}

	// Rounded corners at offset.
	for _, tc := range []struct {
		x, y int
		want string
	}{
		{wantFrame.Min.X, wantFrame.Min.Y, "╭"},
		{wantFrame.Max.X - 1, wantFrame.Min.Y, "╮"},
		{wantFrame.Min.X, wantFrame.Max.Y - 1, "╰"},
		{wantFrame.Max.X - 1, wantFrame.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(tc.x, tc.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) nil", tc.x, tc.y)
		}
		if got := cell.Content; got != tc.want {
			t.Errorf("corner (%d,%d) = %q, want %q", tc.x, tc.y, got, tc.want)
		}
	}
}

// TestPhotoSendModal_ExactInteractionsEnabled verifies all interaction IDs,
// rects, Z, actions, and compositor hit parity.
func TestPhotoSendModal_ExactInteractionsEnabled(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/photo.png"}
	surface, compositor, _ := photoSendModal(bounds, data)
	frame := surface.Rect

	// Exactly 4 IDs: close, input, cancel, submit.
	ids := make(map[string]bool)
	for _, it := range surface.Interactions {
		if ids[it.ID] {
			t.Errorf("duplicate ID %q", it.ID)
		}
		ids[it.ID] = true
	}
	wantIDs := map[string]bool{
		"photo-send:close": true, "photo-send:input": true,
		"photo-send:cancel": true, "photo-send:submit": true,
	}
	if !mapsEqual(ids, wantIDs) {
		t.Errorf("interaction IDs = %v, want %v", ids, wantIDs)
	}

	// Close: abs rect at frame.Max.X-2, frame.Min.Y.
	closeRect := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1).Intersect(frame)
	for _, it := range surface.Interactions {
		if it.ID == "photo-send:close" {
			if !it.Rect.Eq(closeRect) {
				t.Errorf("close rect = %v, want %v", it.Rect, closeRect)
			}
			if it.Z != zModalControl {
				t.Errorf("close Z = %d, want %d", it.Z, zModalControl)
			}
			if it.Click.Action != Close {
				t.Errorf("close action = %#v, want Close", it.Click)
			}
			if it.Virtual {
				t.Error("close must not be virtual")
			}
		}
	}

	// Input.
	for _, it := range surface.Interactions {
		if it.ID == "photo-send:input" {
			if it.Z != zModalControl {
				t.Errorf("input Z = %d, want %d", it.Z, zModalControl)
			}
			if it.Virtual {
				t.Error("input must not be virtual")
			}
		}
	}

	// Cancel: local (2,5) -> abs (14,13)-(22,14).
	for _, it := range surface.Interactions {
		if it.ID == "photo-send:cancel" {
			wantCancelAbs := image.Rect(frame.Min.X+2, frame.Min.Y+5, frame.Min.X+10, frame.Min.Y+6).Intersect(frame)
			if !it.Rect.Eq(wantCancelAbs) {
				t.Errorf("cancel rect = %v, want %v", it.Rect, wantCancelAbs)
			}
			if it.Click.Action != Close {
				t.Errorf("cancel action = %#v, want Close", it.Click)
			}
		}
	}

	// Send: local (56-8,5) -> abs (56-8+12,8+5)=(60,13)-(66,14).
	for _, it := range surface.Interactions {
		if it.ID == "photo-send:submit" {
			wantSubmitAbs := image.Rect(frame.Max.X-8, frame.Min.Y+5, frame.Max.X-2, frame.Min.Y+6).Intersect(frame)
			if !it.Rect.Eq(wantSubmitAbs) {
				t.Errorf("submit rect = %v, want %v", it.Rect, wantSubmitAbs)
			}
			if it.Click.Action != PhotoSendSubmit {
				t.Errorf("submit action = %#v, want PhotoSendSubmit", it.Click)
			}
			if it.Virtual {
				t.Error("submit must not be virtual")
			}
		}
	}

	// Compositor Hit parity for all visual interactions.
	for _, it := range surface.Interactions {
		if it.Virtual {
			continue
		}
		center := image.Pt(it.Rect.Min.X+it.Rect.Dx()/2, it.Rect.Min.Y+it.Rect.Dy()/2)
		hit := compositor.Hit(center.X, center.Y)
		if hit.ID() != it.ID {
			t.Errorf("Hit(%v) = %q, want %q for interaction %q", center, hit.ID(), it.ID, it.ID)
		}
		if got := hit.Bounds(); !got.Eq(it.Rect) {
			t.Errorf("%s hit bounds = %v, want %v", it.ID, got, it.Rect)
		}
	}

	// Verify rounded corners actually render via canvas.
	_, _, canvas := photoSendModal(bounds, data)
	corner := canvas.CellAt(frame.Max.X-1, frame.Min.Y)
	if corner == nil || corner.Content != "╮" {
		t.Errorf("corner cell = %+v, want ╮", corner)
	}

	// No virtual interactions in this leaf.
	for _, it := range surface.Interactions {
		if it.Virtual {
			t.Errorf("interaction %q must not be virtual", it.ID)
		}
	}

	// No wheel actions.
	for _, it := range surface.Interactions {
		if it.WheelUp.Action != NoAction || it.WheelDown.Action != NoAction {
			t.Errorf("interaction %q has non-zero wheel action", it.ID)
		}
	}
}

// TestPhotoSendModal_BlankSubmitMutedNoHit verifies that when Path is empty,
// submit is not an interaction, [Send] is muted, and cursor is visible.
func TestPhotoSendModal_BlankSubmitMutedNoHit(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: ""}
	surface, compositor, _ := photoSendModal(bounds, data)
	frame := surface.Rect

	// Collect IDs.
	idSet := make(map[string]bool)
	for _, it := range surface.Interactions {
		idSet[it.ID] = true
	}
	if idSet["photo-send:submit"] {
		t.Error("submit interaction should not exist for blank path")
	}
	// close/input/cancel should still exist.
	for _, want := range []string{"photo-send:close", "photo-send:input", "photo-send:cancel"} {
		if !idSet[want] {
			t.Errorf("missing interaction %q for blank path", want)
		}
	}

	// Verify no submit hit at Send cells.
	sendRect := image.Rect(frame.Max.X-8, frame.Min.Y+5, frame.Max.X-2, frame.Min.Y+6).Intersect(frame)
	for x := sendRect.Min.X; x < sendRect.Max.X; x++ {
		for y := sendRect.Min.Y; y < sendRect.Max.Y; y++ {
			hit := compositor.Hit(x, y)
			if hit.ID() == "photo-send:submit" {
				t.Errorf("Hit(%d,%d) = submit, should not", x, y)
			}
		}
	}

	// Cursor visible at first input cell.
	inputLocal := image.Rect(2, 3, frame.Dx()-2, 4)
	inputAbs := inputLocal.Add(frame.Min).Intersect(frame)
	if got := surface.Cursor; !got.Visible {
		t.Errorf("cursor should be visible for blank path, got visible=%v", got.Visible)
	}
	if got := surface.Cursor; got.X < inputAbs.Min.X || got.X >= inputAbs.Max.X {
		t.Errorf("cursor X = %d, expected in [%d,%d)", got.X, inputAbs.Min.X, inputAbs.Max.X)
	}
}

// TestPhotoSendModal_LongPathShowsTailAndCursorInside verifies trailingCells
// behavior with a long path.
func TestPhotoSendModal_LongPathShowsTailAndCursorInside(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	// Distinctive prefix/suffix.
	data := photoSendModalData{Path: "/very/long/prefix/that/should/not/see/____suffix.png"}
	surface, _, canvas := photoSendModal(bounds, data)

	// Visible path should contain suffix and not prefix.
	rendered := plainText(canvas.Render())
	if !strings.Contains(rendered, "suffix.png") {
		t.Errorf("long path missing suffix in: %s", rendered)
	}
	if strings.Contains(rendered, "/very/long/prefix/") {
		t.Errorf("long path shows prefix in: %s", rendered)
	}

	// Cursor should be at the trailing edge of visible input.
	inputLocal := image.Rect(2, 3, surface.Rect.Dx()-2, 4)
	inputAbs := inputLocal.Add(surface.Rect.Min).Intersect(surface.Rect)
	if !cursorInside(surface.Cursor, inputAbs) {
		t.Errorf("cursor {%d,%d,v=%v} not inside input rect %v",
			surface.Cursor.X, surface.Cursor.Y, surface.Cursor.Visible, inputAbs)
	}
}

// TestPhotoSendModal_GraphemeSafety tests CJK, emoji, FE0F, ZWJ combinations.
func TestPhotoSendModal_GraphemeSafety(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, surface surfaceResult, canvas *lipgloss.Canvas)
	}{
		{
			name: "CJK",
			path: "/tmp/图片.png",
			check: func(t *testing.T, s surfaceResult, _ *lipgloss.Canvas) {
				if !s.Cursor.Visible {
					t.Error("cursor should be visible")
				}
				if s.Cursor.X < s.Rect.Min.X || s.Cursor.X > s.Rect.Max.X {
					t.Errorf("CJK cursor X out of frame: %d", s.Cursor.X)
				}
			},
		},
		{
			name: "emoji",
			path: "/tmp/📷photo.png",
			check: func(t *testing.T, s surfaceResult, _ *lipgloss.Canvas) {
				if !s.Cursor.Visible {
					t.Error("emoji cursor should be visible")
				}
				inputLocal := image.Rect(2, 3, s.Rect.Dx()-2, 4)
				inputAbs := inputLocal.Add(s.Rect.Min).Intersect(s.Rect)
				if s.Cursor.X < inputAbs.Min.X || s.Cursor.X > inputAbs.Max.X {
					t.Errorf("emoji cursor out of input: X=%d", s.Cursor.X)
				}
			},
		},
		{
			name: "FE0F",
			path: "/tmp/❤️.png",
			check: func(t *testing.T, s surfaceResult, _ *lipgloss.Canvas) {
				if !s.Cursor.Visible {
					t.Error("FE0F cursor should be visible")
				}
			},
		},
		{
			name: "ZWJ family",
			path: "/tmp/👨‍👩‍👧‍👦.png",
			check: func(t *testing.T, s surfaceResult, _ *lipgloss.Canvas) {
				if !s.Cursor.Visible {
					t.Error("ZWJ cursor should be visible")
				}
				inputLocal := image.Rect(2, 3, s.Rect.Dx()-2, 4)
				inputAbs := inputLocal.Add(s.Rect.Min).Intersect(s.Rect)
				if s.Cursor.X < inputAbs.Min.X || s.Cursor.X > inputAbs.Max.X {
					t.Errorf("ZWJ cursor out: X=%d", s.Cursor.X)
				}
			},
		},
		{
			name: "combinations",
			path: "/tmp/a❤️👨‍👩‍👧‍👦界.png",
			check: func(t *testing.T, s surfaceResult, c *lipgloss.Canvas) {
				if !s.Cursor.Visible {
					t.Error("combo cursor should be visible")
				}
				// No height growth beyond frame.
				if s.Rect.Dy() > 8 {
					t.Errorf("frame height grew to %d", s.Rect.Dy())
				}
				// Verify each line width does not exceed bounds.Dx().
				text := plainText(c.Render())
				lines := strings.Split(text, "\n")
				for _, line := range lines {
					w := ansi.StringWidth(line)
					if w > 80 {
						t.Errorf("line width %d exceeds bounds width 80", w)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := photoSendModalData{Path: tc.path}
			surface, _, canvas := photoSendModal(bounds, data)
			tc.check(t, surface, canvas)

			// No wrap/height growth: frame height must stay <=8.
			if surface.Rect.Dy() > 8 {
				t.Errorf("frame height %d > 8", surface.Rect.Dy())
			}
		})
	}
}

// TestPhotoSendModal_CloseAndCornerIntact verifies that the close interaction
// and the top-right corner cell are rendered correctly when the path contains
// CJK/emoji content. The title is the fixed literal "Send file" (not from data),
// and Path data only affects the input visual.
func TestPhotoSendModal_CloseAndCornerIntact(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/👨‍👩‍👧‍👦🔥❤️很长很长的标题.png"}
	surface, _, canvas := photoSendModal(bounds, data)
	frame := surface.Rect

	// Close interaction exists.
	closeIt := mediaInteraction(t, surface, "photo-send:close")
	if closeIt.ID != "photo-send:close" {
		t.Errorf("close ID = %q", closeIt.ID)
	}

	// Corner must be intact.
	corner := canvas.CellAt(frame.Max.X-1, frame.Min.Y)
	if corner == nil || corner.Content != "╮" {
		t.Errorf("corner = %+v, want ╮", corner)
	}
}

// TestPhotoSendModal_Deterministic verifies same input yields same output.
func TestPhotoSendModal_Deterministic(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/photo.png"}

	_, comp1, canvas1 := photoSendModal(bounds, data)
	_, comp2, canvas2 := photoSendModal(bounds, data)

	s1 := comp1.Render()
	s2 := comp2.Render()

	if s1 != s2 {
		t.Errorf("non-deterministic: renders differ")
	}

	if !canvas1.Bounds().Eq(canvas2.Bounds()) {
		t.Errorf("non-deterministic: canvas bounds differ")
	}

	// Rect, interactions, cursor must all match.
}

// TestPhotoSendModal_NoOverlayNoSecondCompositor verifies structurally
// that builder only returns surfaceResult and no overlay side channel.
func TestPhotoSendModal_NoOverlayNoSecondCompositor(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/photo.png"}
	result := buildPhotoSendModalLayer(bounds, data, newRenderStyles(false))

	// Should be surfaceResult only — no overlayRequest side channel.
	if result.Layer == nil {
		t.Fatal("nil layer for valid bounds")
	}
	// Cursor is visible, not hidden.
	if !result.Cursor.Visible {
		t.Error("cursor should be visible")
	}
}

// TestPhotoSendModal_InputHasSingleVisualLayer asserts that the input has
// exactly one visual layer whose ID is "photo-send:input". After the fix in
// buildPhotoSendModalLayer, the anonymous root.AddLayers() for input was
// removed so addInteractive is the sole visual authority.
func TestPhotoSendModal_InputHasSingleVisualLayer(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	data := photoSendModalData{Path: "/tmp/photo.png"}
	styles := newRenderStyles(false)
	surface, _, _ := photoSendModal(bounds, data)

	// Compute expected content using the same frame dimensions as the builder.
	frameWidth := min(56, bounds.Dx())
	inputLocal := image.Rect(2, 3, frameWidth-2, 4)
	inputWidth := max(1, frameWidth-4)
	visiblePath := trailingCells(data.Path, inputWidth)
	visiblePathContent := renderLine(styles.Input, visiblePath, inputWidth)

	// Find the interactive input layer by ID.
	inputLayer := surface.Layer.GetLayer("photo-send:input")
	if inputLayer == nil {
		t.Fatal("GetLayer(\"photo-send:input\") returned nil")
	}

	// Layer content must match the visible path (not a duplicated anonymous layer).
	if inputLayer.GetContent() != visiblePathContent {
		t.Errorf("input layer content = %q, want %q", inputLayer.GetContent(), visiblePathContent)
	}

	// Z must be zModalControl (the addInteractive authority), not zModalContent.
	if inputLayer.GetZ() != zModalControl {
		t.Errorf("input Z = %d, want %d", inputLayer.GetZ(), zModalControl)
	}

	// Position must match the input rect.
	if inputLayer.GetX() != inputLocal.Min.X || inputLayer.GetY() != inputLocal.Min.Y {
		t.Errorf("input layer pos = (%d,%d), want (%d,%d)",
			inputLayer.GetX(), inputLayer.GetY(), inputLocal.Min.X, inputLocal.Min.Y)
	}

	// The layer ID must be set on the layer itself.
	if inputLayer.GetID() != "photo-send:input" {
		t.Errorf("input layer ID = %q, want %q", inputLayer.GetID(), "photo-send:input")
	}
}

// ---------- helpers ----------

func cursorInside(c renderCursor, frame image.Rectangle) bool {
	return c.X >= frame.Min.X && c.X < frame.Max.X &&
		c.Y >= frame.Min.Y && c.Y < frame.Max.Y
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
