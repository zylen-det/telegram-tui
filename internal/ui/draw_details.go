package ui

import (
	"image"

	"github.com/mattn/go-runewidth"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
)

func drawDetails(buffer *gotui.Buffer, model ViewModel, hits *HitMap) {
	rectangle := model.Layout.Details.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	inner := drawRoundedBlock(buffer, rectangle, "Info", model.Focus == app.FocusDetails)
	appendHit(hits, Hit{
		Rect:  rectangle,
		Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusDetails},
	})
	if inner.Empty() {
		return
	}

	closeRect := image.Rect(rectangle.Max.X-2, rectangle.Min.Y, rectangle.Max.X-1, rectangle.Min.Y+1).Intersect(rectangle)
	if !closeRect.Empty() {
		drawClipped(buffer, closeRect.Min, closeRect.Dx(), "×", accentStyle)
		appendHit(hits, Hit{Rect: closeRect, Click: app.ActionReceived{Action: app.ToggleDetails}})
	}
	if model.ActiveChat.ID == 0 {
		y := inner.Min.Y + inner.Dy()/2
		drawCenteredText(buffer, image.Rect(inner.Min.X, y, inner.Max.X, y+1), "No conversation", mutedStyle)
		return
	}

	avatarWidth := min(12, inner.Dx())
	avatarHeight := min(6, inner.Dy())
	avatarX := inner.Min.X + (inner.Dx()-avatarWidth)/2
	avatarRect := image.Rect(avatarX, inner.Min.Y, avatarX+avatarWidth, inner.Min.Y+avatarHeight)
	row := activeChatRow(model)
	drawAvatar(buffer, avatarRect, row.Avatar, row.AvatarKey, model.ActiveChat.Title)
	if row.AvatarError != nil && avatarRect.Dx() == 12 && avatarRect.Dy() == 6 {
		appendHit(hits, Hit{Rect: avatarRect, Click: app.ActionReceived{Action: app.Retry, AvatarKey: row.AvatarKey}})
	}

	y := avatarRect.Max.Y + 1
	if y < inner.Max.Y {
		drawCenteredText(buffer, image.Rect(inner.Min.X, y, inner.Max.X, y+1), model.ActiveChat.Title, emphasisStyle)
		y++
	}
	if model.ActiveChat.Username != "" && y < inner.Max.Y {
		drawCenteredText(buffer, image.Rect(inner.Min.X, y, inner.Max.X, y+1), "@"+model.ActiveChat.Username, mutedStyle)
		y++
	}
	if y < inner.Max.Y {
		action := "View image"
		actionWidth := min(runewidth.StringWidth(action), inner.Dx())
		x := inner.Min.X + (inner.Dx()-actionWidth)/2
		actionRect := image.Rect(x, y, x+actionWidth, y+1)
		drawClipped(buffer, actionRect.Min, actionRect.Dx(), action, accentStyle)
		appendHit(hits, Hit{Rect: actionRect, Click: app.ActionReceived{Action: app.Activate}})
	}
}

func activeChatRow(model ViewModel) ChatRow {
	for _, row := range model.Chats {
		if row.Chat.ID == model.ActiveChat.ID {
			return row
		}
	}
	return ChatRow{}
}

func drawCenteredText(buffer *gotui.Buffer, rectangle image.Rectangle, text string, style gotui.Style) {
	rectangle = rectangle.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	width := min(runewidth.StringWidth(text), rectangle.Dx())
	x := rectangle.Min.X + (rectangle.Dx()-width)/2
	drawClipped(buffer, image.Pt(x, rectangle.Min.Y), rectangle.Max.X-x, text, style)
}
