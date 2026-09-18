package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/zylen-det/telegram-tui/internal/app"
)

func paneCanvas(size image.Point, surface surfaceResult) *lipgloss.Canvas {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(size.X).Height(size.Y).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(size.X, size.Y).Compose(compositor)
}

func TestPaneExactComposedBoundsAndRoundedCorners(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(2, 3, 22, 9)
	surface := buildPane(rect, "Chats", true, "chats", app.FocusChats, styles)
	if surface.Layer == nil {
		t.Fatal("pane layer is nil")
	}
	// Root content dimensions match the rect.
	if got, want := surface.Layer.Width(), rect.Dx(); got != want {
		t.Errorf("pane layer width = %d, want %d", got, want)
	}
	if got, want := surface.Layer.Height(), rect.Dy(); got != want {
		t.Errorf("pane layer height = %d, want %d", got, want)
	}
	if got, want := surface.Layer.GetX(), rect.Min.X; got != want {
		t.Errorf("pane layer X = %d, want %d", got, want)
	}
	if got, want := surface.Layer.GetY(), rect.Min.Y; got != want {
		t.Errorf("pane layer Y = %d, want %d", got, want)
	}
	if got, want := surface.Layer.GetZ(), zPane; got != want {
		t.Errorf("pane layer Z = %d, want %d", got, want)
	}

	// Compose onto a canvas and check rounded corners at the exact rect.
	canvas := paneCanvas(image.Pt(30, 12), surface)
	for _, corner := range []struct {
		x, y int
		want string
	}{
		{rect.Min.X, rect.Min.Y, "╭"},
		{rect.Max.X - 1, rect.Min.Y, "╮"},
		{rect.Min.X, rect.Max.Y - 1, "╰"},
		{rect.Max.X - 1, rect.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(corner.x, corner.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) is nil", corner.x, corner.y)
		}
		if got := cell.Content; got != corner.want {
			t.Errorf("corner (%d,%d) = %q, want %q", corner.x, corner.y, got, corner.want)
		}
	}
	// A left edge interior cell is a vertical border.
	if cell := canvas.CellAt(rect.Min.X, rect.Min.Y+1); cell.Content != "│" {
		t.Errorf("left edge cell = %q, want │", cell.Content)
	}
	// A top border interior cell is a horizontal border (past the title).
	if cell := canvas.CellAt(rect.Min.X+8, rect.Min.Y); cell.Content != "─" {
		t.Errorf("top border cell = %q, want ─", cell.Content)
	}
}

func TestPaneTitleCellsStyledAndBorderPreserved(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 20, 5)
	surface := buildPane(rect, "Chats", true, "chats", app.FocusChats, styles)
	canvas := paneCanvas(image.Pt(20, 5), surface)

	// Title begins at local (2,0) absolute (2,0).
	for x := 2; x < 2+len("Chats"); x++ {
		cell := canvas.CellAt(x, 0)
		if cell == nil {
			t.Fatalf("CellAt(%d,0) is nil", x)
		}
		if got := colorOf(cell.Style.Fg); got != rgba(textColor) {
			t.Errorf("title cell (%d,0) foreground = %v, want %v", x, got, rgba(textColor))
		}
		if cell.Style.Attrs&uv.AttrBold == 0 {
			t.Errorf("title cell (%d,0) is not bold", x)
		}
	}
	// The cell immediately before the title remains a border cell.
	cell := canvas.CellAt(1, 0)
	if got := cell.Content; got != "─" {
		t.Errorf("cell before title = %q, want ─", got)
	}
}

func TestPaneFocusedUnfocusedBorderColors(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 12, 4)

	focused := buildPane(rect, "", true, "f", app.FocusChats, styles)
	fCanvas := paneCanvas(image.Pt(12, 4), focused)
	if got := colorOf(fCanvas.CellAt(0, 0).Style.Fg); got != rgba(focusedBorderColor) {
		t.Errorf("focused border foreground = %v, want %v", got, rgba(focusedBorderColor))
	}

	unfocused := buildPane(rect, "", false, "u", app.FocusChats, styles)
	uCanvas := paneCanvas(image.Pt(12, 4), unfocused)
	if got := colorOf(uCanvas.CellAt(0, 0).Style.Fg); got != rgba(borderColor) {
		t.Errorf("unfocused border foreground = %v, want %v", got, rgba(borderColor))
	}
}

func TestPaneHitIDBoundsMatchInteraction(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(3, 2, 18, 8)
	surface := buildPane(rect, "Chats", true, "pane-chats", app.FocusChats, styles)
	if len(surface.Interactions) != 1 {
		t.Fatalf("interactions = %d, want 1", len(surface.Interactions))
	}
	interaction := surface.Interactions[0]
	if interaction.ID != "pane-chats" {
		t.Errorf("interaction ID = %q, want pane-chats", interaction.ID)
	}
	if !interaction.Rect.Eq(rect) {
		t.Errorf("interaction rect = %v, want %v", interaction.Rect, rect)
	}
	if interaction.Z != zPane {
		t.Errorf("interaction Z = %d, want %d", interaction.Z, zPane)
	}
	if interaction.Click.Action != app.FocusPane || interaction.Click.TargetFocus != app.FocusChats {
		t.Errorf("interaction click = %#v, want FocusPane/FocusChats", interaction.Click)
	}

	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(30).Height(12).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)

	point := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	hit := compositor.Hit(point.X, point.Y)
	if hit.ID() != interaction.ID {
		t.Errorf("Hit(%v) = %q, want %q", point, hit.ID(), interaction.ID)
	}
	if got := hit.Bounds(); !got.Eq(interaction.Rect) {
		t.Errorf("hit bounds = %v, want %v", got, interaction.Rect)
	}
}

func TestPaneEmptyIDNoInteraction(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 20, 5)
	surface := buildPane(rect, "Chats", true, "", app.FocusChats, styles)
	if surface.Layer == nil {
		t.Fatal("pane layer is nil")
	}
	if surface.Layer.GetID() != "" {
		t.Errorf("empty-ID pane layer ID = %q, want empty", surface.Layer.GetID())
	}
	if len(surface.Interactions) != 0 {
		t.Errorf("empty-ID pane interactions = %d, want 0", len(surface.Interactions))
	}
}

func TestPaneTinyRectSafe(t *testing.T) {
	styles := newRenderStyles(false)
	for _, rect := range []image.Rectangle{
		{Min: image.Pt(0, 0), Max: image.Pt(1, 1)},
		{Min: image.Pt(0, 0), Max: image.Pt(2, 1)},
		{Min: image.Pt(0, 0), Max: image.Pt(1, 2)},
		{Min: image.Pt(0, 0), Max: image.Pt(0, 0)},
		{Min: image.Pt(5, 5), Max: image.Pt(5, 5)},
	} {
		surface := buildPane(rect, "Chats", true, "tiny", app.FocusChats, styles)
		if surface.Layer != nil {
			t.Errorf("%v: tiny rect should have no layer, got %v", rect, surface.Layer)
		}
		if len(surface.Interactions) != 0 {
			t.Errorf("%v: tiny rect should have no interactions", rect)
		}
		if surface.Cursor.Visible {
			t.Errorf("%v: tiny rect should have hidden cursor", rect)
		}
	}
}

func TestPaneTitleClippedWithinAvailableWidth(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 10, 5)
	surface := buildPane(rect, "ThisIsALongTitle", true, "p", app.FocusChats, styles)
	canvas := paneCanvas(image.Pt(10, 5), surface)
	// Available title width is rect.Dx()-3 = 7. The clipped title must not
	// overwrite the top-right corner at x=9.
	if cell := canvas.CellAt(9, 0); cell.Content != "╮" {
		t.Errorf("top-right corner = %q, want ╮", cell.Content)
	}
	// The clipped title is present and fits.
	text := plainText(canvas.Render())
	if !strings.Contains(text, "ThisIsA") {
		t.Errorf("clipped title missing prefix, got %q", text)
	}
}
