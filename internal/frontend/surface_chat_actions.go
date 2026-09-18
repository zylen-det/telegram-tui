package frontend

import (
	"image"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func chatActionFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	return centeredSurfaceRectangle(bounds, min(48, bounds.Dx()), min(14, bounds.Dy())).Intersect(bounds)
}

func buildChatActionLayer(model ui.ViewModel, styles renderStyles, selectorView string) surfaceResult {
	menu := model.ChatActions
	if menu == nil || model.ActiveChat.ID == 0 || menu.ChatID != model.ActiveChat.ID {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	title := "Chat actions"
	if menu.Confirming != app.NoAction {
		title = "Confirm action"
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	return buildListModalWidth(bounds, title, displayedChatActionRows(model), styles, chatActionFrame(bounds).Dx(), selectorView)
}

// displayedChatActionRows is the chat action modal's single row source for one
// view model.
func displayedChatActionRows(model ui.ViewModel) []modalRowSpec {
	return chatActionRows(model.ActiveChat, model.ChatActions)
}

// chatActionRows is the single row source for the chat action modal: one row
// per generated app.ChatActionMenuItems entry in exact order plus the trailing
// Working informational row. app.ChatActionMenuItems yields nothing without a
// matching active chat, so a mismatched menu has no rows to render or select.
func chatActionRows(chat domain.Chat, menu *app.ChatActionMenuState) []modalRowSpec {
	if menu == nil {
		return nil
	}
	items := app.ChatActionMenuItems(chat, menu)
	rows := make([]modalRowSpec, 0, len(items)+1)
	for index, item := range items {
		rows = append(rows, modalRowSpec{
			ID:       "chat-action:" + item.Label,
			Label:    item.Label,
			Selected: index == menu.Selected,
			Action:   app.ActionReceived{Action: item.Action, ChatID: menu.ChatID},
		})
	}
	if menu.Working {
		rows = append(rows, modalRowSpec{Label: "Working..."})
	}
	return rows
}
