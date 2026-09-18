package ui

import (
	"image"

	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
)

func drawModal(buffer *gotui.Buffer, model ViewModel, hits *HitMap) {
	if model.Modal == nil || buffer == nil {
		return
	}
	bounds := terminalBounds(buffer, model)
	if bounds.Empty() {
		return
	}
	dimRectangle(buffer, bounds)
	width := max(4, bounds.Dx()*80/100)
	height := max(4, bounds.Dy()*80/100)
	frame := centeredRectangle(bounds, width, height)
	inner := drawRoundedBlock(buffer, frame, model.Modal.Title, true)
	if inner.Empty() {
		return
	}

	closeRect := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1).Intersect(frame)
	outside := []image.Rectangle{
		image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, frame.Min.Y),
		image.Rect(bounds.Min.X, frame.Max.Y, bounds.Max.X, bounds.Max.Y),
		image.Rect(bounds.Min.X, frame.Min.Y, frame.Min.X, frame.Max.Y),
		image.Rect(frame.Max.X, frame.Min.Y, bounds.Max.X, frame.Max.Y),
	}
	for _, rectangle := range outside {
		appendHit(hits, Hit{Rect: rectangle, Click: app.ActionReceived{Action: app.Close}})
	}
	if !closeRect.Empty() {
		drawClipped(buffer, closeRect.Min, closeRect.Dx(), "×", accentStyle)
		appendHit(hits, Hit{Rect: closeRect, Click: app.ActionReceived{Action: app.Close}})
	}

	content := insetRectangle(inner, 1)
	if content.Empty() {
		content = inner
	}
	switch {
	case model.Modal.Loading:
		drawCenteredText(buffer, image.Rect(content.Min.X, content.Min.Y+content.Dy()/2, content.Max.X, content.Min.Y+content.Dy()/2+1), "Loading image...", mutedStyle)
	case model.Modal.Error != nil:
		y := content.Min.Y + max(0, content.Dy()/2-1)
		drawCenteredText(buffer, image.Rect(content.Min.X, y, content.Max.X, min(content.Max.Y, y+1)), safeErrorText(model.Modal.Error), errorStyle)
		retryText := "Retry"
		retryY := min(content.Max.Y-1, y+2)
		retryWidth := min(len(retryText), content.Dx())
		retryX := content.Min.X + (content.Dx()-retryWidth)/2
		retryRect := image.Rect(retryX, retryY, retryX+retryWidth, retryY+1).Intersect(content)
		if !retryRect.Empty() {
			drawClipped(buffer, retryRect.Min, retryRect.Dx(), retryText, accentStyle)
			appendHit(hits, Hit{Rect: retryRect, Click: app.ActionReceived{Action: app.Retry}})
		}
	}
}
