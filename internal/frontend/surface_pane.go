package frontend

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
)

// buildPane builds one rounded pane root at the absolute rect.Min with a
// single pane interaction covering the exact rect. The root carries the given
// ID only when non-empty; an empty ID produces a root without an ID and no
// interaction. A non-empty title is placed intrinsically on the top border row
// so the remaining border cells stay intact.
func buildPane(rect image.Rectangle, title string, focused bool, id string, focus app.Focus, styles renderStyles) surfaceResult {
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	borderStyle := styles.Border
	if focused {
		borderStyle = styles.FocusedBorder
	}
	content := styles.Panel.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderStyle.GetForeground()).
		BorderBackground(borderStyle.GetBackground()).
		Width(rect.Dx()).
		Height(rect.Dy()).
		Render("")

	root := lipgloss.NewLayer(content).X(rect.Min.X).Y(rect.Min.Y).Z(zPane)
	if id != "" {
		root.ID(id)
	}

	var interactions []layerInteraction
	if id != "" {
		interactions = []layerInteraction{{
			ID:    id,
			Rect:  rect,
			Z:     zPane,
			Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: focus},
		}}
	}

	if title != "" && rect.Dx() > 4 {
		available := rect.Dx() - 3
		clipped := ansi.Truncate(title, available, "")
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(clipped)).X(2).Y(0).Z(zContent))
	}

	return surfaceResult{
		Layer:        root,
		Rect:         rect,
		Interactions: interactions,
		Cursor:       renderCursor{X: -1, Y: -1},
	}
}
