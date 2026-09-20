package frontend

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
)

// buildToastLayer builds the floating toast notification surface. It anchors
// the toast one cell from the bottom-right of the viewport and renders a
// rounded panel with the message text. The layer has no interactions and no
// cursor.
func buildToastLayer(model ViewModel, styles renderStyles) surfaceResult {
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if model.Toast == nil || bounds.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	text := model.Toast.Message
	if model.Toast.Kind != "" {
		text = fmt.Sprintf("[%s] %s", model.Toast.Kind, text)
	}

	layout := (components.Toast{Text: text}).Layout(bounds)
	if layout.Frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	// Frame: a fresh root at the exact absolute frame Min, no ID/interactions.
	var rootContent string
	if layout.Frame.Dx() >= 3 && layout.Frame.Dy() >= 3 {
		rootContent = styles.Panel.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Border.GetForeground()).
			BorderBackground(styles.Border.GetBackground()).
			Width(layout.Frame.Dx()).
			Height(layout.Frame.Dy()).
			Render("")
	} else {
		rootContent = renderEmptyBox(styles.Panel, layout.Frame.Dx(), layout.Frame.Dy())
	}
	root := lipgloss.NewLayer(rootContent).X(layout.Frame.Min.X).Y(layout.Frame.Min.Y).Z(zToastFrame)

	// Text leaf: one intrinsic child at local content coordinates, clipped to
	// the content width before styling. Omit when content/text is empty.
	contentWidth := layout.Content.Dx()
	if contentWidth > 0 && text != "" {
		style := styles.Warning
		if model.Toast.Kind == domain.ErrorInternal || model.Toast.Kind == domain.ErrorStorage {
			style = styles.Error
		}
		clipped := ansi.Truncate(text, contentWidth, "")
		root.AddLayers(lipgloss.NewLayer(style.Render(clipped)).
			X(layout.Content.Min.X - layout.Frame.Min.X).
			Y(layout.Content.Min.Y - layout.Frame.Min.Y).
			Z(zToastContent))
	}

	return surfaceResult{
		Layer:  root,
		Rect:   layout.Frame,
		Cursor: renderCursor{X: -1, Y: -1},
	}
}
