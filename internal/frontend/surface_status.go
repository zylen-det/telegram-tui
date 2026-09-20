package frontend

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// buildStatusLayer builds the status bar surface: an exact root box with an
// intrinsic brand, title, and connection text. The rect is the status layout
// intersected with the viewport; an empty result returns a zero surface.
func buildStatusLayer(model ViewModel, styles renderStyles) surfaceResult {
	rect := model.Layout.Status.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	rootContent := styles.Base.Width(rect.Dx()).Height(rect.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(rect.Min.X).Y(rect.Min.Y).Z(zStatus)

	connection := connectionText(model.Connection)
	connectionX := rect.Max.X - displayWidth(connection) - 1
	connectionVisible := connectionX > rect.Min.X

	// Brand.
	brand := "telegram-tui"
	brandStyle := styles.Accent
	brandRight := rect.Max.X
	if connectionVisible {
		brandRight = connectionX
	}
	brandWidth := max(0, brandRight-(rect.Min.X+1))
	if clippedBrand := ansi.Truncate(brand, brandWidth, ""); clippedBrand != "" {
		root.AddLayers(lipgloss.NewLayer(brandStyle.Render(clippedBrand)).X(1).Y(0).Z(zContent))
	}

	// Connection text.
	if connectionVisible {
		connectionStyle := styles.Muted
		switch model.Connection {
		case domain.ConnectionOnline:
			connectionStyle = connectionStyle.Foreground(rgba(accentColor))
		case domain.ConnectionOffline:
			connectionStyle = connectionStyle.Foreground(rgba(offlineColor))
		case domain.ConnectionReconnecting:
			connectionStyle = connectionStyle.Foreground(rgba(warningColor))
		}
		root.AddLayers(lipgloss.NewLayer(connectionStyle.Render(connection)).X(connectionX - rect.Min.X).Y(0).Z(zContent))
	}

	// Title.
	title := model.ActiveChat.Title
	if title == "" {
		title = "Chats"
	}
	centeredX := rect.Min.X + centeredX(rect.Dx(), displayWidth(title))
	left := rect.Min.X + displayWidth("telegram-tui") + 3
	right := connectionX - 2
	titleX := centeredX
	if titleX < left {
		titleX = left
	}
	if titleX < right {
		available := right - titleX
		clipped := ansi.Truncate(title, available, "")
		root.AddLayers(lipgloss.NewLayer(styles.Base.Render(clipped)).X(titleX - rect.Min.X).Y(0).Z(zContent))
	}

	return surfaceResult{
		Layer:        root,
		Rect:         rect,
		Interactions: nil,
		Cursor:       renderCursor{X: -1, Y: -1},
	}
}
