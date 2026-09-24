package frontend

import (
	"image"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func chatActionFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	return centeredSurfaceRectangle(bounds, min(54, bounds.Dx()), min(14, bounds.Dy())).Intersect(bounds)
}

func buildChatActionLayer(model ViewModel, styles renderStyles, selectorView string) surfaceResult {
	menu := model.ChatActions
	_, ok := chatActionTarget(model)
	if menu == nil || !ok {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	title := "Chat actions"
	if menu.Confirming != NoAction {
		title = "Confirm action"
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	return buildListModalWidth(bounds, title, displayedChatActionRows(model), styles, chatActionFrame(bounds).Dx(), selectorView)
}

// displayedChatActionRows is the chat action modal's single row source for one
// view model.
func displayedChatActionRows(model ViewModel) []modalRowSpec {
	chat, ok := chatActionTarget(model)
	if !ok {
		return nil
	}
	return chatActionRows(chat, model.ChatActions)
}

func chatActionTarget(model ViewModel) (domain.Chat, bool) {
	if model.ChatActions == nil {
		return domain.Chat{}, false
	}
	for _, row := range model.Chats {
		if row.Chat.ID == model.ChatActions.ChatID {
			return row.Chat, true
		}
	}
	// Keep direct/standalone view models compatible when they only publish the
	// active chat, while production resolves the menu-owned focused identity
	// from Chats above.
	if model.ActiveChat.ID == model.ChatActions.ChatID {
		return model.ActiveChat, true
	}
	return domain.Chat{}, false
}

// chatActionRows is the single row source for the chat action modal: one row
// per generated ChatActionMenuItems entry in exact order plus the trailing
// Working informational row. ChatActionMenuItems yields nothing without a
// matching active chat, so a mismatched menu has no rows to render or select.
func chatActionRows(chat domain.Chat, menu *ChatActionMenuState) []modalRowSpec {
	if menu == nil {
		return nil
	}
	items := ChatActionMenuItems(chat, menu)
	rows := make([]modalRowSpec, 0, len(items)+1)
	for index, item := range items {
		rows = append(rows, modalRowSpec{
			ID:       "chat-action:" + item.Label,
			Label:    item.Label,
			Selected: index == menu.Selected,
			Action:   ActionReceived{Action: item.Action, ChatID: menu.ChatID},
			Key:      actionModalShortcut(item.Action),
		})
	}
	if menu.Working {
		rows = append(rows, modalRowSpec{Label: "Working..."})
	}
	return rows
}
