package frontend

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// buildMediaModalLayer builds the media modal as a real Lipgloss layer surface
// and the Kitty overlay request describing the same content rectangle. Geometry
// is owned entirely by modalSurfaceRectangles; the returned Frame and Content
// are the sole geometry authority for both the text shell and the overlay.
//
// The frame, title, and close always render. Content text is shown only for the
// loading and error shells; a ready modal keeps the text shell empty so the
// Kitty image fills the content rectangle. Outside-frame rectangles are virtual
// close interactions with no visual Layer.
func buildMediaModalLayer(model ui.ViewModel, styles renderStyles) (surfaceResult, overlayRequest) {
	if model.Modal == nil || model.Width <= 0 || model.Height <= 0 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}, overlayRequest{}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	frame, content := modalSurfaceRectangles(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}, overlayRequest{}
	}

	request := overlayRequest{
		Path:       model.Modal.Path,
		Bounds:     content,
		Columns:    model.Width,
		Rows:       model.Height,
		SelectedID: int64(model.ActiveChat.ID),
	}
	ready := model.Modal.Path != "" &&
		!model.Modal.Loading &&
		model.Modal.Error == nil &&
		!content.Empty() &&
		model.Width >= minimumWidth &&
		model.Height >= minimumHeight &&
		model.Prompt == nil &&
		model.Focus != app.FocusAuth
	request.Ready = ready

	// Frame root: absolute at Frame.Min, no ID. Use the actual rounded Style
	// box or degrade to an exact Panel box when the border geometry is too small.
	var rootContent string
	if frame.Dx() >= 3 && frame.Dy() >= 3 {
		rootContent = styles.Panel.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.FocusedBorder.GetForeground()).
			BorderBackground(styles.FocusedBorder.GetBackground()).
			Width(frame.Dx()).
			Height(frame.Dy()).
			Render("")
	} else {
		rootContent = renderEmptyBox(styles.Panel, frame.Dx(), frame.Dy())
	}
	root := lipgloss.NewLayer(rootContent).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)

	var interactions []layerInteraction

	// Title: intrinsic child at local (2,0), clipped strictly before the close
	// cell so it never overwrites the top-right border/close corner.
	titleWidth := frame.Dx() - 4
	if titleWidth > 0 && model.Modal.Title != "" {
		clipped := ansi.Truncate(model.Modal.Title, titleWidth, "")
		if clipped != "" {
			root.AddLayers(lipgloss.NewLayer(styles.Title.Render(clipped)).X(2).Y(0).Z(zModalContent))
		}
	}

	// Close control: exact 1-cell at the top-right interior, click app.Close.
	closeLocal := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1).
		Intersect(frame).Sub(frame.Min)
	if !closeLocal.Empty() {
		closeContent := renderLine(styles.Accent, "×", 1)
		interactions = append(interactions, addInteractive(
			root, frame.Min, closeLocal, "media:close", zModalControl, closeContent,
			app.ActionReceived{Action: app.Close}, app.ActionReceived{}, app.ActionReceived{},
		))
	}

	// Outside-frame virtual close interactions: four absolute rectangles covering
	// the viewport around the frame, published as layerInteractions only (no
	// visual Layer). Rect stays absolute.
	outside := []struct {
		id   string
		rect image.Rectangle
	}{
		{"media:outside:top", image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, frame.Min.Y)},
		{"media:outside:bottom", image.Rect(bounds.Min.X, frame.Max.Y, bounds.Max.X, bounds.Max.Y)},
		{"media:outside:left", image.Rect(bounds.Min.X, frame.Min.Y, frame.Min.X, frame.Max.Y)},
		{"media:outside:right", image.Rect(frame.Max.X, frame.Min.Y, bounds.Max.X, frame.Max.Y)},
	}
	for _, region := range outside {
		region.rect = region.rect.Intersect(bounds)
		if region.rect.Empty() {
			continue
		}
		interactions = append(interactions, layerInteraction{
			ID:      region.id,
			Rect:    region.rect,
			Z:       zModalControl,
			Click:   app.ActionReceived{Action: app.Close},
			Virtual: true,
		})
	}

	// Loading/error content shell, all intrinsic single-line text centered within
	// the exact Content rectangle. A ready/path-empty nonloading state adds no
	// content text.
	if !content.Empty() {
		switch {
		case model.Modal.Loading:
			text := "Loading image..."
			textWidth := min(displayWidth(text), content.Dx())
			x := content.Min.X + centeredX(content.Dx(), textWidth)
			y := content.Min.Y + content.Dy()/2
			root.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, text, textWidth)).
				X(x - frame.Min.X).Y(y - frame.Min.Y).Z(zModalContent))
		case model.Modal.Error != nil:
			message := model.Modal.Error.Message
			messageWidth := min(displayWidth(message), content.Dx())
			messageX := content.Min.X + centeredX(content.Dx(), messageWidth)
			y := content.Min.Y + max(0, content.Dy()/2-1)
			root.AddLayers(lipgloss.NewLayer(renderLine(styles.Error, message, messageWidth)).
				X(messageX - frame.Min.X).Y(y - frame.Min.Y).Z(zModalContent))

			// Retry: fixed exact visual interaction, clipped/centered within Content.
			retryText := "Retry"
			retryWidth := min(displayWidth(retryText), content.Dx())
			retryY := min(content.Max.Y-1, y+2)
			if retryWidth > 0 && retryY >= content.Min.Y && retryY < content.Max.Y {
				retryX := content.Min.X + centeredX(content.Dx(), retryWidth)
				retryLocal := image.Rect(retryX, retryY, retryX+retryWidth, retryY+1).Sub(frame.Min)
				if !retryLocal.Empty() {
					interactions = append(interactions, addInteractive(
						root, frame.Min, retryLocal, "media:retry", zModalControl,
						renderLine(styles.Accent, retryText, retryWidth),
						app.ActionReceived{Action: app.Retry}, app.ActionReceived{}, app.ActionReceived{},
					))
				}
			}
		}
	}

	return surfaceResult{
		Layer:        root,
		Rect:         frame,
		Interactions: interactions,
		Cursor:       renderCursor{X: -1, Y: -1},
		IsModal:      true,
	}, request
}
