package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

const chatRowHeight = 4

func drawChats(buffer *gotui.Buffer, model ViewModel, hits *HitMap) {
	rectangle := model.Layout.Chats.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	inner := drawRoundedBlock(buffer, rectangle, "Chats", model.Focus == app.FocusChats)
	appendHit(hits, Hit{
		Rect:  rectangle,
		Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusChats},
	})
	if inner.Empty() {
		return
	}

	rowsTop := inner.Min.Y
	switch {
	case model.ChatsError != nil:
		drawClipped(buffer, inner.Min, inner.Dx(), safeErrorText(model.ChatsError), errorStyle)
		if len(model.Chats) == 0 {
			return
		}
		rowsTop++
	case model.ChatsLoading:
		drawClipped(buffer, inner.Min, inner.Dx(), "Loading chats...", mutedStyle)
		if len(model.Chats) == 0 {
			return
		}
		rowsTop++
	case model.ChatsLoaded && len(model.Chats) == 0:
		drawClipped(buffer, inner.Min, inner.Dx(), "No chats", mutedStyle)
		return
	}

	capacity := max(0, (inner.Max.Y-rowsTop)/chatRowHeight)
	if capacity == 0 {
		return
	}
	selectedIndex := -1
	for index, row := range model.Chats {
		if row.Selected {
			selectedIndex = index
			break
		}
	}
	start := 0
	if selectedIndex >= capacity {
		start = selectedIndex - capacity + 1
	}
	end := min(len(model.Chats), start+capacity)
	for index := start; index < end; index++ {
		visibleIndex := index - start
		row := model.Chats[index]
		y := rowsTop + visibleIndex*chatRowHeight
		rowRect := image.Rect(inner.Min.X, y, inner.Max.X, y+chatRowHeight)
		if row.Selected {
			buffer.Fill(gotui.NewCell(' ', gotui.NewStyle(textColor, selectedColor)), rowRect)
		}

		avatarRect := image.Rect(rowRect.Min.X, rowRect.Min.Y, rowRect.Min.X+6, rowRect.Min.Y+3)
		drawAvatar(buffer, avatarRect, row.Avatar, row.AvatarKey, row.Chat.Title)
		textX := min(rowRect.Max.X, avatarRect.Max.X+1)
		textWidth := max(0, rowRect.Max.X-textX)
		rowStyle := panelStyle
		rowMutedStyle := mutedStyle
		rowTitleStyle := emphasisStyle
		if row.Selected {
			rowStyle.Bg = selectedColor
			rowMutedStyle.Bg = selectedColor
			rowTitleStyle.Bg = selectedColor
		}
		drawClipped(buffer, image.Pt(textX, y), textWidth, row.Chat.Title, rowTitleStyle)
		drawClipped(buffer, image.Pt(textX, y+1), textWidth, row.Chat.LastMessage, rowStyle)
		drawClipped(buffer, image.Pt(textX, y+2), textWidth, chatMetadata(row.Chat.LastMessageAt, row.Chat.UnreadCount, row.Chat.Muted), rowMutedStyle)

		appendHit(hits, Hit{
			Rect:      rowRect,
			Click:     app.ActionReceived{Action: app.SelectChat, ChatID: row.Chat.ID},
			WheelUp:   app.ActionReceived{Action: app.SelectPrevious},
			WheelDown: app.ActionReceived{Action: app.SelectNext},
		})
		if row.AvatarError != nil {
			appendHit(hits, Hit{
				Rect:  avatarRect,
				Click: app.ActionReceived{Action: app.Retry, AvatarKey: row.AvatarKey},
			})
			drawClipped(buffer, image.Pt(avatarRect.Min.X, avatarRect.Max.Y-1), avatarRect.Dx(), "Retry", gotui.NewStyle(errorColor, panelColor, tcell.AttrBold))
		}
	}
}

func chatMetadata(timestamp int64, unread int, muted bool) string {
	parts := make([]string, 0, 3)
	if timestamp != 0 {
		parts = append(parts, time.Unix(timestamp, 0).In(time.Local).Format("15:04"))
	}
	if unread > 0 {
		parts = append(parts, fmt.Sprintf("[%d]", unread))
	}
	if muted {
		parts = append(parts, "mute")
	}
	return strings.Join(parts, " ")
}

func safeErrorText(appError *domain.AppError) string {
	if appError == nil {
		return ""
	}
	return appError.Message
}

func appendHit(hits *HitMap, hit Hit) {
	if hits != nil && !hit.Rect.Empty() {
		*hits = append(*hits, hit)
	}
}
