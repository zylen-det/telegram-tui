package frontend

import (
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type photoSendModalData struct {
	Path string
}

func buildPhotoSendModalLayer(bounds image.Rectangle, data photoSendModalData, styles renderStyles, photoPathView ...string) surfaceResult {
	if bounds.Empty() || bounds.Dx() < 20 || bounds.Dy() < 8 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	frameWidth := min(56, bounds.Dx())
	frameHeight := min(8, bounds.Dy())

	frameX := bounds.Min.X + (bounds.Dx()-frameWidth)/2
	frameY := bounds.Min.Y + (bounds.Dy()-frameHeight)/2
	frame := image.Rect(frameX, frameY, frameX+frameWidth, frameY+frameHeight).Intersect(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	// Frame root at absolute frame.Min, no ID.
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

	// Title: literal "Send file" at local (2,0), styles.Title, truncated.
	titleText := "Send file"
	titleWidth := frame.Dx() - 4
	if titleWidth > 0 {
		clipped := ansi.Truncate(titleText, titleWidth, "")
		if clipped != "" {
			root.AddLayers(lipgloss.NewLayer(styles.Title.Render(clipped)).
				X(2).Y(0).Z(zModalContent))
		}
	}

	// Close control: absolute cell at [frame.Max.X-2, frame.Min.Y], one cell.
	closeAbs := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1)
	closeAbs = closeAbs.Intersect(frame)
	if !closeAbs.Empty() {
		closeLocal := closeAbs.Sub(frame.Min)
		closeContent := renderLine(styles.Accent, "\u00d7", 1)
		interactions = append(interactions, addInteractive(
			root, frame.Min, closeLocal, "photo-send:close", zModalControl, closeContent,
			ActionReceived{Action: Close}, ActionReceived{}, ActionReceived{},
		))
	}

	// Label: literal "Local media/document path" at local (2,2), clipped to frame.Dx()-4.
	labelWidth := frame.Dx() - 4
	if labelWidth > 0 {
		label := "Local media/document path"
		clipped := ansi.Truncate(label, labelWidth, "")
		if clipped != "" {
			root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(clipped)).
				X(2).Y(2).Z(zModalContent))
		}
	}

	// Input: shared photoSendInputRect geometry, one row.
	inputAbs := photoSendInputRect(bounds)
	inputWidth := max(1, inputAbs.Dx())

	var inputContent string
	var cursor renderCursor
	if len(photoPathView) > 0 {
		// Production typed injection: content comes only from the clipped Huh
		// View (even when empty); no manual path fallback, no terminal cursor.
		inputContent = renderLine(styles.Panel, clipPhotoPathInputView(photoPathView[0], inputWidth), inputWidth)
		cursor = renderCursor{X: -1, Y: -1}
	} else {
		// Standalone/direct compatibility: manual trailing path and absolute cursor.
		visiblePath := trailingCells(data.Path, inputWidth-1)
		inputContent = renderLine(styles.Panel, visiblePath, inputWidth)
		cursorX := inputAbs.Min.X + displayWidth(visiblePath)
		cursorY := inputAbs.Min.Y
		// Ensure cursor stays inside input rect.
		if cursorX > inputAbs.Max.X-1 {
			cursorX = inputAbs.Max.X - 1
		}
		if cursorX < inputAbs.Min.X {
			cursorX = inputAbs.Min.X
		}
		cursor = renderCursor{X: cursorX, Y: cursorY, Visible: true}
	}
	inputInteraction := addInteractive(
		root, frame.Min, inputAbs.Sub(frame.Min), "photo-send:input", zModalControl,
		inputContent,
		ActionReceived{}, ActionReceived{}, ActionReceived{},
	)
	interactions = append(interactions, inputInteraction)

	// Controls at local Y=5 (absolute frame.Min.Y+5).
	const controlsLocalY = 5
	controlsAbsY := frame.Min.Y + controlsLocalY
	if controlsAbsY >= frame.Max.Y-1 {
		return surfaceResult{
			Layer:        root,
			Rect:         frame,
			Interactions: interactions,
			Cursor:       cursor,
			IsModal:      true,
		}
	}

	// Cancel: local rect image.Rect(2,5,10,6), literal "[Cancel]".
	cancelLocal := image.Rect(2, controlsLocalY, 10, controlsLocalY+1)
	cancelAbs := cancelLocal.Add(frame.Min).Intersect(frame)
	if !cancelAbs.Empty() {
		cancelLocalForRender := cancelAbs.Sub(frame.Min)
		cancelContent := renderLine(styles.Accent, "[Cancel]", cancelLocalForRender.Dx())
		interactions = append(interactions, addInteractive(
			root, frame.Min, cancelLocalForRender, "photo-send:cancel", zModalControl,
			cancelContent,
			ActionReceived{Action: Close}, ActionReceived{}, ActionReceived{},
		))
	}

	// Send: width 6 ending at local frame.Dx()-2.
	sendLocal := image.Rect(frame.Dx()-8, controlsLocalY, frame.Dx()-2, controlsLocalY+1)
	sendAbs := sendLocal.Add(frame.Min).Intersect(frame)
	if !sendAbs.Empty() {
		sendLocalForRender := sendAbs.Sub(frame.Min)
		sendEnabled := strings.TrimSpace(data.Path) != ""
		if sendEnabled {
			if sendLocalForRender.Dx() >= 6 {
				sendContent := renderLine(styles.Accent, "[Send]", 6)
				interactions = append(interactions, addInteractive(
					root, frame.Min, sendLocalForRender, "photo-send:submit", zModalControl,
					sendContent,
					ActionReceived{Action: PhotoSendSubmit},
					ActionReceived{}, ActionReceived{},
				))
			} else {
				// Muted render when too narrow, no ID/interaction.
				root.AddLayers(lipgloss.NewLayer(styles.Muted.Render("[Send]")).
					X(sendLocalForRender.Min.X).Y(sendLocalForRender.Min.Y).Z(zModalControl))
			}
		} else {
			// Blank: styles.Muted visual, no ID, no interaction.
			root.AddLayers(lipgloss.NewLayer(styles.Muted.Render("[Send]")).
				X(sendLocalForRender.Min.X).Y(sendLocalForRender.Min.Y).Z(zModalControl))
		}
	}

	return surfaceResult{
		Layer:        root,
		Rect:         frame,
		Interactions: interactions,
		Cursor:       cursor,
		IsModal:      true,
	}
}

// clipPhotoPathInputView reduces an injected Huh Photo path editor View to the
// visible one-row input: it keeps only the exact first line and truncates it
// to width cells with ANSI safety. A non-positive width or empty view yields
// ""; no second or third row may ever appear.
func clipPhotoPathInputView(view string, width int) string {
	if width <= 0 || view == "" {
		return ""
	}
	first := strings.SplitN(view, "\n", 2)[0]
	return ansi.Truncate(first, width, "")
}

// photoSendInputRect computes the Photo send modal input rect from the modal
// frame bounds. Returns the empty rect for frames smaller than the modal
// minimum (Dx<20 or Dy<8). Otherwise the frame is width=min(56,Dx),
// height=min(8,Dy), centered within bounds, then the local input rect
// (2,3,frame.Dx()-2,4) is translated and intersected.
func photoSendInputRect(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() || bounds.Dx() < 20 || bounds.Dy() < 8 {
		return image.Rectangle{}
	}

	frameWidth := min(56, bounds.Dx())
	frameHeight := min(8, bounds.Dy())

	frameX := bounds.Min.X + (bounds.Dx()-frameWidth)/2
	frameY := bounds.Min.Y + (bounds.Dy()-frameHeight)/2
	frame := image.Rect(frameX, frameY, frameX+frameWidth, frameY+frameHeight).Intersect(bounds)
	if frame.Empty() {
		return image.Rectangle{}
	}

	inputLocal := image.Rect(2, 3, frame.Dx()-2, 4)
	return inputLocal.Add(frame.Min).Intersect(frame)
}
