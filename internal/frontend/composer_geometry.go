package frontend

import (
	"image"

	"github.com/zylen-det/telegram-tui/internal/ui"
)

// composerSurfaceRect returns the absolute viewport rectangle occupied by the
// composer surface inside the conversation pane.
//
// It intersects the conversation layout with the viewport, then computes the
// inner rect (one-cell border inset), and takes the bottom three inner rows.
// Returns a zero rect when the conversation pane is too small or outside the
// viewport.
func composerSurfaceRect(model ui.ViewModel) image.Rectangle {
	rect := model.Layout.Conversation.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return image.Rectangle{}
	}

	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	if inner.Empty() {
		return image.Rectangle{}
	}

	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	return image.Rect(inner.Min.X, composerTop, inner.Max.X, inner.Max.Y)
}

// composerTextRect returns the absolute viewport rectangle available for
// draft text inside the composer, taking into account banner rows and control
// geometry (Send + optional Photo).
//
// Returns zero rect when there is no active chat, the chat is read-only,
// the rect has no usable width after X=1, or there are no available rows.
type composerControlLayout struct {
	sendX      int
	photoX     int
	stickerX   int
	hasPhoto   bool
	hasSticker bool
}

// composerControls computes the exact horizontal control geometry shared by
// the surface and controlled-editor rectangle.
func composerControls(width int, interactive bool) composerControlLayout {
	controls := composerControlLayout{sendX: max(0, width-displayWidth(sendText)-1)}
	if !interactive {
		return controls
	}
	photoW := displayWidth(photoText)
	photoX := controls.sendX - photoW - 1
	if photoX < 1 || photoX+photoW > controls.sendX {
		return controls
	}
	controls.photoX = photoX
	controls.hasPhoto = true

	stickerW := displayWidth(stickerText)
	stickerX := photoX - stickerW - 1
	if stickerX >= 1 && stickerX+stickerW <= photoX {
		controls.stickerX = stickerX
		controls.hasSticker = true
	}
	return controls
}

func composerTextRect(model ui.ViewModel, rect image.Rectangle) image.Rectangle {
	rect = rect.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Empty() {
		return image.Rectangle{}
	}

	width := rect.Dx()
	height := rect.Dy()

	// No active chat or read-only → no text area.
	if model.ActiveChat.ID == 0 || !model.ActiveChat.CanSend {
		return image.Rectangle{}
	}

	// Need at least one cell to the right of X=1 for text.
	if width <= 1 {
		return image.Rectangle{}
	}

	contentTop := 0

	// Reply banner consumes one row.
	if model.ReplyTarget != nil && model.ReplyTarget.ChatID == model.ActiveChat.ID && contentTop < height {
		contentTop++
	}

	// Edit banner consumes one row, plus optional "Edit failed" row.
	if model.EditTarget != nil && model.EditTarget.ChatID == model.ActiveChat.ID && contentTop < height {
		contentTop++
		if model.EditTarget.Error != nil && contentTop < height-1 {
			contentTop++
		}
	}

	// Available rows from contentTop to the bottom edge.
	availableRows := max(0, height-contentTop)
	if availableRows <= 0 {
		return image.Rectangle{}
	}

	controls := composerControls(width, true)
	leftmostControlX := controls.sendX
	if controls.hasPhoto {
		leftmostControlX = controls.photoX
	}
	if controls.hasSticker {
		leftmostControlX = controls.stickerX
	}
	draftWidth := max(1, leftmostControlX-1)
	if draftWidth <= 0 {
		return image.Rectangle{}
	}

	textHeight := min(2, availableRows)

	return image.Rect(rect.Min.X+1, rect.Min.Y+contentTop, rect.Min.X+1+draftWidth, rect.Min.Y+contentTop+textHeight)
}
