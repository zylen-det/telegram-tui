package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// statusCanvas composes a compositor rooted at (0,0) containing the status
// layer onto a canvas sized to the model viewport.
func statusCanvas(model ViewModel, surface surfaceResult) *lipgloss.Canvas {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor)
}

func statusModel(width int) ViewModel {
	return ViewModel{
		Width:      width,
		Height:     18,
		Layout:     ComputeLayout(width, 18, false, 0),
		Connection: domain.ConnectionOnline,
	}
}

func TestStatusExactRectAndBounds(t *testing.T) {
	for _, width := range []int{60, 100, 120} {
		model := statusModel(width)
		styles := newRenderStyles(false)
		surface := buildStatusLayer(model, styles)
		if surface.Layer == nil {
			t.Fatalf("%d: status layer is nil", width)
		}
		want := image.Rect(0, 0, width, 1)
		if !surface.Rect.Eq(want) {
			t.Errorf("%d: status rect = %v, want %v", width, surface.Rect, want)
		}
		if got := surface.Layer.GetX(); got != want.Min.X {
			t.Errorf("%d: layer X = %d, want %d", width, got, want.Min.X)
		}
		if got := surface.Layer.GetY(); got != want.Min.Y {
			t.Errorf("%d: layer Y = %d, want %d", width, got, want.Min.Y)
		}
		if got := surface.Layer.Width(); got != want.Dx() {
			t.Errorf("%d: layer width = %d, want %d", width, got, want.Dx())
		}
		if got := surface.Layer.Height(); got != want.Dy() {
			t.Errorf("%d: layer height = %d, want %d", width, got, want.Dy())
		}
	}
}

func TestStatusBrandFallbackAndActiveTitle(t *testing.T) {
	styles := newRenderStyles(false)

	// Fallback title "Chats" when no active chat.
	model := statusModel(100)
	model.ActiveChat = domain.Chat{}
	canvas := statusCanvas(model, buildStatusLayer(model, styles))
	text := plainText(canvas.Render())
	if !strings.Contains(text, "telegram-tui") {
		t.Error("status missing brand")
	}
	if !strings.Contains(text, "Chats") {
		t.Error("status missing fallback title")
	}

	// Active chat title.
	model.ActiveChat = domain.Chat{Title: "Weekend"}
	canvas = statusCanvas(model, buildStatusLayer(model, styles))
	if !strings.Contains(plainText(canvas.Render()), "Weekend") {
		t.Error("status missing active title")
	}
}

func TestStatusConnectionTextAndColors(t *testing.T) {
	cases := []struct {
		state domain.ConnectionState
		text  string
		color rgb
	}{
		{domain.ConnectionOnline, "online", accentColor},
		{domain.ConnectionOffline, "offline", offlineColor},
		{domain.ConnectionReconnecting, "reconnecting", warningColor},
		{domain.ConnectionWaiting, "waiting", mutedTextColor},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			model := statusModel(120)
			model.Connection = tc.state
			styles := newRenderStyles(false)
			canvas := statusCanvas(model, buildStatusLayer(model, styles))
			if !strings.Contains(plainText(canvas.Render()), tc.text) {
				t.Errorf("status missing %q", tc.text)
			}
			// The connection text is right-aligned at connectionX.
			connectionX := model.Width - displayWidth(tc.text) - 1
			cell := canvas.CellAt(connectionX, 0)
			if cell == nil {
				t.Fatalf("CellAt(%d,0) is nil", connectionX)
			}
			if got := colorOf(cell.Style.Fg); got != rgba(tc.color) {
				t.Errorf("connection foreground = %v, want %v", got, rgba(tc.color))
			}
		})
	}
}

func TestStatusLongTitleClippedWithinReservations(t *testing.T) {
	model := statusModel(60)
	model.ActiveChat = domain.Chat{Title: strings.Repeat("x", 200)}
	styles := newRenderStyles(false)
	canvas := statusCanvas(model, buildStatusLayer(model, styles))

	connection := "online"
	connectionX := model.Width - displayWidth(connection) - 1
	left := displayWidth("telegram-tui") + 3
	right := connectionX - 2

	// The title must not overwrite the brand at x=1.
	if cell := canvas.CellAt(1, 0); cell.Content != "telegram-tui"[0:1] {
		t.Errorf("brand cell overwritten by long title: %q", cell.Content)
	}
	// The title must not overwrite the connection text.
	if cell := canvas.CellAt(connectionX, 0); cell.Content != "o" {
		t.Errorf("connection cell overwritten by long title: %q", cell.Content)
	}
	// Title starts at left and is clipped to fit within right-left cells.
	for x := left; x < right; x++ {
		cell := canvas.CellAt(x, 0)
		if cell == nil {
			t.Fatalf("CellAt(%d,0) is nil", x)
		}
		if cell.Content != "x" {
			t.Errorf("title cell (%d,0) = %q, want x", x, cell.Content)
		}
	}
	// The cell just before the connection reservation is not a title cell.
	if cell := canvas.CellAt(right, 0); cell.Content == "x" {
		t.Errorf("title overflowed into reservation at x=%d", right)
	}
}

func TestStatusNoInteractionsOrIDs(t *testing.T) {
	model := statusModel(100)
	styles := newRenderStyles(false)
	surface := buildStatusLayer(model, styles)
	if len(surface.Interactions) != 0 {
		t.Errorf("status interactions = %d, want 0", len(surface.Interactions))
	}
	if surface.Layer.GetID() != "" {
		t.Errorf("status layer ID = %q, want empty", surface.Layer.GetID())
	}
	if surface.Cursor.Visible {
		t.Error("status should have hidden cursor")
	}
}

func TestStatusEmptyIntersectionZero(t *testing.T) {
	model := statusModel(100)
	// A status layout outside the viewport yields an empty intersection.
	model.Layout.Status = image.Rect(0, 0, 0, 0)
	styles := newRenderStyles(false)
	surface := buildStatusLayer(model, styles)
	if surface.Layer != nil {
		t.Errorf("empty status should have no layer, got %v", surface.Layer)
	}
	if surface.Cursor.Visible {
		t.Error("empty status should have hidden cursor")
	}
}

func TestStatusNarrowBrandStaysWithinRect(t *testing.T) {
	for _, test := range []struct {
		width int
		want  string
	}{
		{width: 5, want: " tele"},
		{width: 10, want: " teonline"},
	} {
		model := ViewModel{
			Width:      test.width,
			Height:     1,
			Layout:     ViewLayout{Status: image.Rect(0, 0, test.width, 1)},
			Connection: domain.ConnectionOnline,
		}
		surface := buildStatusLayer(model, newRenderStyles(false))
		if surface.Layer == nil {
			t.Fatalf("width %d: narrow status layer is nil", test.width)
		}
		root := lipgloss.NewLayer(lipgloss.NewStyle().Width(test.width).Height(1).Render("")).X(0).Y(0).Z(zFrame)
		root.AddLayers(surface.Layer)
		compositor := lipgloss.NewCompositor(root)
		if got := compositor.Bounds(); got != image.Rect(0, 0, test.width, 1) {
			t.Errorf("width %d: compositor bounds = %v", test.width, got)
		}
		if got := plainText(compositor.Render()); got != test.want {
			t.Errorf("width %d: status = %q, want %q", test.width, got, test.want)
		}
	}
}
