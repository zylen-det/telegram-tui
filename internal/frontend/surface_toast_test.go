package frontend

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
)

// toastCanvas composes a full-size styles.Base root with the toast layer onto
// a canvas sized to the model viewport.
func toastCanvas(model ViewModel, surface surfaceResult) *lipgloss.Canvas {
	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(model.Width).Height(model.Height).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor)
}

func toastModel(width, height int, kind, message string) ViewModel {
	var toast *domain.AppError
	if message != "" || kind != "" {
		toast = &domain.AppError{Kind: domain.ErrorKind(kind), Message: message}
	}
	return ViewModel{Width: width, Height: height, Toast: toast}
}

func TestToastLayerNilToastAndEmptyViewport(t *testing.T) {
	styles := newRenderStyles(false)
	for _, model := range []ViewModel{
		{Width: 80, Height: 24},
		{Width: 0, Height: 0},
		{Width: 80, Height: 0},
		{Width: 0, Height: 24},
	} {
		surface := buildToastLayer(model, styles)
		if surface.Layer != nil {
			t.Errorf("%v: nil/empty toast should have no layer", model)
		}
		if surface.Cursor.Visible {
			t.Errorf("%v: nil/empty toast should have hidden cursor", model)
		}
		if len(surface.Interactions) != 0 {
			t.Errorf("%v: nil/empty toast should have no interactions", model)
		}
	}
}

func TestToastLayerASCIIExactFrameAndContent(t *testing.T) {
	model := toastModel(80, 24, "", "Message copied")
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, model.Width, model.Height))
	surface := buildToastLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("toast layer is nil")
	}
	if !surface.Rect.Eq(layout.Frame) {
		t.Errorf("surface rect = %v, want %v", surface.Rect, layout.Frame)
	}
	if got, want := surface.Layer.GetX(), layout.Frame.Min.X; got != want {
		t.Errorf("layer X = %d, want %d", got, want)
	}
	if got, want := surface.Layer.GetY(), layout.Frame.Min.Y; got != want {
		t.Errorf("layer Y = %d, want %d", got, want)
	}
	if got, want := surface.Layer.Width(), layout.Frame.Dx(); got != want {
		t.Errorf("layer width = %d, want %d", got, want)
	}
	if got, want := surface.Layer.Height(), layout.Frame.Dy(); got != want {
		t.Errorf("layer height = %d, want %d", got, want)
	}
	// Bottom-right Max stays exactly one cell inside the viewport corner.
	if got, want := surface.Rect.Max, image.Pt(model.Width-1, model.Height-1); got != want {
		t.Errorf("rect Max = %v, want %v", got, want)
	}
}

func TestToastLayerRoundedBorderCornersAndPalette(t *testing.T) {
	model := toastModel(80, 24, "", "Message copied")
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, model.Width, model.Height))
	surface := buildToastLayer(model, styles)
	canvas := toastCanvas(model, surface)

	for _, corner := range []struct {
		x, y int
		want string
	}{
		{layout.Frame.Min.X, layout.Frame.Min.Y, "╭"},
		{layout.Frame.Max.X - 1, layout.Frame.Min.Y, "╮"},
		{layout.Frame.Min.X, layout.Frame.Max.Y - 1, "╰"},
		{layout.Frame.Max.X - 1, layout.Frame.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(corner.x, corner.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) is nil", corner.x, corner.y)
		}
		if got := cell.Content; got != corner.want {
			t.Errorf("corner (%d,%d) = %q, want %q", corner.x, corner.y, got, corner.want)
		}
		if got := colorOf(cell.Style.Fg); got != rgba(borderColor) {
			t.Errorf("corner (%d,%d) foreground = %v, want %v", corner.x, corner.y, got, rgba(borderColor))
		}
	}
	// A left edge interior cell is a vertical border.
	if cell := canvas.CellAt(layout.Frame.Min.X, layout.Frame.Min.Y+1); cell.Content != "│" {
		t.Errorf("left edge cell = %q, want │", cell.Content)
	}
}

func TestToastLayerMessageVisibleAndClipped(t *testing.T) {
	model := toastModel(80, 24, "", "Message copied")
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, model.Width, model.Height))
	surface := buildToastLayer(model, styles)
	canvas := toastCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Message copied") {
		t.Errorf("toast content missing message, got %q", text)
	}
	// Content stays one line: the content row has no wrapped second row.
	contentRow := layout.Content.Min.Y
	if got := strings.Count(text, "\n"); got != model.Height-1 {
		t.Errorf("rendered lines = %d, want %d (no wrapping)", got+1, model.Height)
	}
	// The content cell at the left edge of content holds the message start.
	cell := canvas.CellAt(layout.Content.Min.X, contentRow)
	if cell == nil || !strings.Contains(cell.Content, "M") {
		t.Errorf("content start cell = %q, want to contain M", cell.Content)
	}
}

func TestToastLayerKindPrefixAndWarningForeground(t *testing.T) {
	model := toastModel(80, 24, "network", "Temporarily offline")
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: "[network] Temporarily offline"}).Layout(image.Rect(0, 0, model.Width, model.Height))
	surface := buildToastLayer(model, styles)
	canvas := toastCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "[network] Temporarily offline") {
		t.Errorf("toast content missing kind prefix, got %q", text)
	}
	// The first content cell carries the warning foreground.
	cell := canvas.CellAt(layout.Content.Min.X, layout.Content.Min.Y)
	if cell == nil {
		t.Fatal("content cell is nil")
	}
	if got := colorOf(cell.Style.Fg); got != rgba(warningColor) {
		t.Errorf("toast content foreground = %v, want %v", got, rgba(warningColor))
	}
}

func TestToastLayerErrorForegroundSelection(t *testing.T) {
	styles := newRenderStyles(false)
	for _, tc := range []struct {
		kind string
		want color.RGBA
	}{
		{string(domain.ErrorInternal), rgba(errorColor)},
		{string(domain.ErrorStorage), rgba(errorColor)},
		{string(domain.ErrorNetwork), rgba(warningColor)},
		{string(domain.ErrorRateLimit), rgba(warningColor)},
		{"", rgba(warningColor)},
	} {
		model := toastModel(80, 24, tc.kind, "Something happened")
		surface := buildToastLayer(model, styles)
		canvas := toastCanvas(model, surface)
		// Match the production text construction so the layout geometry aligns.
		toastText := "Something happened"
		if tc.kind != "" {
			toastText = "[" + tc.kind + "] Something happened"
		}
		layout := (components.Toast{Text: toastText}).Layout(image.Rect(0, 0, model.Width, model.Height))
		// The first interior content cell carries the text foreground.
		cell := canvas.CellAt(layout.Content.Min.X, layout.Content.Min.Y)
		if cell == nil {
			t.Fatalf("%q: content cell is nil", tc.kind)
		}
		if got := colorOf(cell.Style.Fg); got != tc.want {
			t.Errorf("kind %q foreground = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestToastLayerMixedGraphemeWidthFrame(t *testing.T) {
	text := "a👨‍👩‍👧‍👦界🔥"
	model := toastModel(80, 24, "", text)
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: text}).Layout(image.Rect(0, 0, model.Width, model.Height))
	surface := buildToastLayer(model, styles)
	if !surface.Rect.Eq(layout.Frame) {
		t.Errorf("surface rect = %v, want %v", surface.Rect, layout.Frame)
	}
	canvas := toastCanvas(model, surface)
	// Content remains a single line and the full message is visible.
	rendered := plainText(canvas.Render())
	if !strings.Contains(rendered, text) {
		t.Errorf("toast content missing mixed text %q, got %q", text, rendered)
	}
	if got := strings.Count(rendered, "\n"); got != model.Height-1 {
		t.Errorf("rendered lines = %d, want %d (content one line)", got+1, model.Height)
	}
}

func TestToastLayerTinyBoundsSafe(t *testing.T) {
	styles := newRenderStyles(false)
	for _, w := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		for _, h := range []int{1, 2, 3} {
			model := toastModel(w, h, "", "Message copied")
			surface := buildToastLayer(model, styles)
			if surface.Layer == nil {
				t.Fatalf("%dx%d: tiny toast should still produce a layer", w, h)
			}
			canvas := toastCanvas(model, surface)
			bounds := canvas.Bounds()
			if bounds.Dx() != w || bounds.Dy() != h {
				t.Errorf("%dx%d: canvas bounds = %v", w, h, bounds)
			}
			// The layer must stay within the viewport.
			if surface.Layer.GetX() < 0 || surface.Layer.GetY() < 0 ||
				surface.Layer.GetX()+surface.Layer.Width() > w ||
				surface.Layer.GetY()+surface.Layer.Height() > h {
				t.Errorf("%dx%d: layer escapes viewport: X=%d Y=%d W=%d H=%d",
					w, h, surface.Layer.GetX(), surface.Layer.GetY(), surface.Layer.Width(), surface.Layer.Height())
			}
		}
	}
}

func TestToastLayerNoInteractionsIDsOrCursor(t *testing.T) {
	styles := newRenderStyles(false)
	for _, model := range []ViewModel{
		toastModel(80, 24, "", "Message copied"),
		toastModel(80, 24, "network", "Temporarily offline"),
		{Width: 80, Height: 24},
		toastModel(3, 2, "", "x"),
	} {
		surface := buildToastLayer(model, styles)
		if len(surface.Interactions) != 0 {
			t.Errorf("%v: toast should have no interactions", model)
		}
		if surface.Cursor.Visible {
			t.Errorf("%v: toast should have hidden cursor", model)
		}
		if surface.Layer != nil && surface.Layer.GetID() != "" {
			t.Errorf("%v: toast layer should have no ID", model)
		}
	}
}

func TestToastLayerFloatsOverBaseWithoutReservingGeometry(t *testing.T) {
	// Compose a base label separately, then compose the toast over the same
	// full-size root. The base geometry outside the toast frame is unchanged.
	model := toastModel(80, 24, "", "Message copied")
	styles := newRenderStyles(false)
	layout := (components.Toast{Text: "Message copied"}).Layout(image.Rect(0, 0, model.Width, model.Height))

	base := styles.Base.Width(model.Width).Height(model.Height).Render("")
	baseRoot := lipgloss.NewLayer(base).X(0).Y(0).Z(zFrame)
	baseCompositor := lipgloss.NewCompositor(baseRoot)
	baseCanvas := lipgloss.NewCanvas(model.Width, model.Height).Compose(baseCompositor)

	surface := buildToastLayer(model, styles)
	toastRoot := lipgloss.NewLayer(base).X(0).Y(0).Z(zFrame)
	toastRoot.AddLayers(surface.Layer)
	toastCompositor := lipgloss.NewCompositor(toastRoot)
	toastCanvas := lipgloss.NewCanvas(model.Width, model.Height).Compose(toastCompositor)

	// A cell outside the toast frame must be identical between the two.
	outside := image.Pt(0, 0)
	before := baseCanvas.CellAt(outside.X, outside.Y)
	after := toastCanvas.CellAt(outside.X, outside.Y)
	if before == nil || after == nil {
		t.Fatal("outside cell is nil")
	}
	if before.Content != after.Content || colorOf(before.Style.Bg) != colorOf(after.Style.Bg) {
		t.Errorf("base cell outside frame changed: before=%q/%v after=%q/%v",
			before.Content, colorOf(before.Style.Bg), after.Content, colorOf(after.Style.Bg))
	}
	// The toast frame itself is drawn.
	if cell := toastCanvas.CellAt(layout.Frame.Min.X, layout.Frame.Min.Y); cell == nil || cell.Content != "╭" {
		t.Errorf("toast frame corner missing over base")
	}
}
