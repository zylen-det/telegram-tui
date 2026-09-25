package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

// ChatActionMenuItem is one capability-aware row in the chat-list action modal.
type ChatActionMenuItem struct {
	Action      Action
	Label       string
	Destructive bool
}

func ChatActionMenuItems(chat domain.Chat, menu *ChatActionMenuState) []ChatActionMenuItem {
	if menu == nil || chat.ID == 0 || menu.ChatID != chat.ID {
		return nil
	}
	if menu.Confirming != NoAction {
		return []ChatActionMenuItem{
			{Action: CancelChatAction, Label: "Cancel"},
			{Action: ConfirmChatAction, Label: confirmationLabel(menu.Confirming), Destructive: true},
		}
	}
	items := []ChatActionMenuItem{
		{Action: OpenChat, Label: "Open chat"},
		{Action: ViewChatInfo, Label: chatInfoLabel(chat)},
	}
	if chat.IsArchived {
		items = append(items, ChatActionMenuItem{Action: UnarchiveChat, Label: "Unarchive"})
	} else {
		items = append(items, ChatActionMenuItem{Action: ArchiveChat, Label: "Archive"})
	}
	if chat.IsPinned {
		items = append(items, ChatActionMenuItem{Action: UnpinChat, Label: "Unpin"})
	} else {
		items = append(items, ChatActionMenuItem{Action: PinChat, Label: "Pin"})
	}
	if chat.Muted {
		items = append(items, ChatActionMenuItem{Action: UnmuteChat, Label: "Unmute"})
	} else {
		items = append(items, ChatActionMenuItem{Action: MuteChat, Label: "Mute"})
	}
	if chat.UnreadCount > 0 || chat.UnreadMentionCount > 0 || chat.IsMarkedUnread {
		items = append(items, ChatActionMenuItem{Action: MarkChatRead, Label: "Mark as read"})
	} else {
		items = append(items, ChatActionMenuItem{Action: MarkChatUnread, Label: "Mark as unread"})
	}
	if chat.CanDeleteForSelf || chat.CanDeleteForAll {
		items = append(items, ChatActionMenuItem{Action: ClearChatHistory, Label: "Clear history", Destructive: true})
		if chat.Kind == domain.ChatPrivate {
			items = append(items, ChatActionMenuItem{Action: DeleteConversation, Label: "Delete conversation", Destructive: true})
		}
	}
	if chat.Kind == domain.ChatBasicGroup || chat.Kind == domain.ChatSupergroup || chat.Kind == domain.ChatChannel {
		if chat.IsMember {
			label := "Leave group"
			if chat.Kind == domain.ChatChannel {
				label = "Leave channel"
			}
			items = append(items, ChatActionMenuItem{Action: LeaveChat, Label: label, Destructive: true})
		} else {
			label := "Join group"
			if chat.Kind == domain.ChatChannel {
				label = "Join channel"
			}
			items = append(items, ChatActionMenuItem{Action: JoinChat, Label: label})
		}
		if chat.CanDeleteForAll {
			label := "Delete group"
			if chat.Kind == domain.ChatChannel {
				label = "Delete channel"
			}
			items = append(items, ChatActionMenuItem{Action: DeleteChat, Label: label, Destructive: true})
		}
	}
	return items
}

func chatInfoLabel(chat domain.Chat) string {
	switch chat.Kind {
	case domain.ChatBasicGroup, domain.ChatSupergroup:
		return "View group"
	case domain.ChatChannel:
		return "View channel"
	default:
		return "View profile"
	}
}

func confirmationLabel(action Action) string {
	switch action {
	case ClearChatHistory:
		return "Confirm clear history"
	case LeaveChat:
		return "Confirm leave"
	case DeleteConversation, DeleteChat:
		return "Confirm delete"
	default:
		return "Confirm"
	}
}

func openChatActionMenu(state *State) []Effect {
	chatID, ok := focusedChatID(*state)
	if !ok || state.Focus != FocusChats {
		return nil
	}
	state.ChatActions = &ChatActionMenuState{ChatID: chatID, PreviousFocus: state.Focus}
	state.Focus = FocusChatActions
	return nil
}

func reduceChatActionMenu(state *State, event ActionReceived) []Effect {
	menu := state.ChatActions
	if menu == nil || (event.ChatID != 0 && event.ChatID != menu.ChatID) {
		return nil
	}
	chatIndex := chatIndex(state.Chats, menu.ChatID)
	if chatIndex < 0 {
		state.Focus = menu.PreviousFocus
		state.ChatActions = nil
		return nil
	}
	items := ChatActionMenuItems(state.Chats[chatIndex], menu)
	if menu.Working {
		if event.Action == Close {
			state.Focus = menu.PreviousFocus
			state.ChatActions = nil
		}
		return nil
	}
	switch event.Action {
	case Close:
		if menu.Confirming != NoAction {
			menu.Confirming = NoAction
			menu.Selected = 0
			return nil
		}
		state.Focus = menu.PreviousFocus
		state.ChatActions = nil
		return nil
	case SelectNext, SelectPrevious:
		if len(items) == 0 {
			return nil
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		menu.Selected = (menu.Selected + delta + len(items)) % len(items)
		return nil
	case Activate:
		if len(items) == 0 {
			return nil
		}
		menu.Selected = max(0, min(menu.Selected, len(items)-1))
		event.Action = items[menu.Selected].Action
		return reduceChatActionMenu(state, event)
	case CancelChatAction:
		menu.Confirming = NoAction
		menu.Selected = 0
		return nil
	case ConfirmChatAction:
		if menu.Confirming == NoAction {
			return nil
		}
		action := menu.Confirming
		menu.Confirming = NoAction
		return executeChatAction(state, action)
	case OpenChat:
		state.Focus = menu.PreviousFocus
		state.ChatActions = nil
		return openChatFromList(state, menu.ChatID)
	case ViewChatInfo:
		state.Focus = menu.PreviousFocus
		state.ChatActions = nil
		state.FocusBeforeInfo = state.Focus
		state.DetailsOpen = true
		state.DetailsChatID = menu.ChatID
		state.DetailsSelected = 0
		state.Focus = FocusDetails
		clampDetailsSelection(state)
		return nil
	}
	for _, item := range items {
		if item.Action != event.Action {
			continue
		}
		if item.Destructive {
			menu.Confirming = item.Action
			menu.Selected = 0
			return nil
		}
		return executeChatAction(state, item.Action)
	}
	return nil
}

func openActiveChatFromList(state *State) []Effect {
	chatID, ok := focusedChatID(*state)
	if !ok {
		return nil
	}
	return openChatFromList(state, chatID)
}

func openChatFromList(state *State, chatID domain.ChatID) []Effect {
	commands := selectChatForOpen(state, chatID)
	activeID, ok := activeChatID(*state)
	if !ok || activeID != chatID {
		return commands
	}
	if state.Chats[state.SelectedChat].IsForum {
		focus := state.Focus
		commands = append(commands, activateAllTopics(state, chatID)...)
		state.Focus = focus
		return commands
	}
	commands = append(commands, requestHistoryIfAbsent(state, chatID)...)
	commands = append(commands, requestMissingMessageAvatars(state, state.Messages[chatID])...)
	return commands
}

// selectChatForOpen commits the focused chat as the selected conversation.
// Reopening the already-selected chat does not churn TDLib's open-chat state.
func selectChatForOpen(state *State, chatID domain.ChatID) []Effect {
	index := chatIndex(state.Chats, chatID)
	if index < 0 {
		return nil
	}
	state.FocusedChat = index
	if activeID, ok := activeChatID(*state); ok && activeID == chatID {
		if state.DetailsOpen {
			state.DetailsChatID = chatID
			clampDetailsSelection(state)
		}
		return nil
	}
	return reduceAction(state, ActionReceived{Action: SelectChat, ChatID: chatID})
}

func executeChatAction(state *State, action Action) []Effect {
	menu := state.ChatActions
	if menu == nil {
		return nil
	}
	telegramAction, ok := telegramChatAction(action)
	if !ok {
		return nil
	}
	requestID := allocateRequestID(state)
	menu.RequestID = requestID
	menu.Working = true
	return []Effect{ApplyChatActionCommand{RequestID: requestID, ChatID: menu.ChatID, Action: telegramAction}}
}

func telegramChatAction(action Action) (telegram.ChatAction, bool) {
	switch action {
	case MarkChatRead:
		return telegram.ChatActionMarkRead, true
	case MarkChatUnread:
		return telegram.ChatActionMarkUnread, true
	case MuteChat:
		return telegram.ChatActionMute, true
	case UnmuteChat:
		return telegram.ChatActionUnmute, true
	case PinChat:
		return telegram.ChatActionPin, true
	case UnpinChat:
		return telegram.ChatActionUnpin, true
	case ArchiveChat:
		return telegram.ChatActionArchive, true
	case UnarchiveChat:
		return telegram.ChatActionUnarchive, true
	case ClearChatHistory:
		return telegram.ChatActionClearHistory, true
	case DeleteConversation:
		return telegram.ChatActionDeleteConversation, true
	case DeleteChat:
		return telegram.ChatActionDeleteChat, true
	case LeaveChat:
		return telegram.ChatActionLeaveChat, true
	case JoinChat:
		return telegram.ChatActionJoinChat, true
	default:
		return 0, false
	}
}

func reduceChatActionApplied(state *State, event ChatActionApplied) []Effect {
	menu := state.ChatActions
	if menu == nil || !menu.Working || menu.RequestID != event.RequestID || menu.ChatID != event.ChatID {
		return nil
	}
	index := chatIndex(state.Chats, event.ChatID)
	if index < 0 {
		state.Focus = menu.PreviousFocus
		state.ChatActions = nil
		return nil
	}
	chat := &state.Chats[index]
	label := "Chat updated"
	removeFromList := false
	deleteData := false
	var commands []Effect
	switch event.Action {
	case telegram.ChatActionMarkRead:
		chat.UnreadCount, chat.UnreadMentionCount, chat.IsMarkedUnread = 0, 0, false
		label = "Chat marked as read"
	case telegram.ChatActionMarkUnread:
		chat.IsMarkedUnread = true
		label = "Chat marked as unread"
	case telegram.ChatActionMute:
		chat.Muted = true
		label = "Notifications muted"
	case telegram.ChatActionUnmute:
		chat.Muted = false
		label = "Notifications unmuted"
	case telegram.ChatActionPin:
		chat.IsPinned = true
		label = "Chat pinned"
	case telegram.ChatActionUnpin:
		chat.IsPinned = false
		label = "Chat unpinned"
	case telegram.ChatActionArchive:
		chat.IsArchived = true
		label = "Chat archived"
		removeFromList = true
		commands = append(commands, CloseChatCommand{ChatID: event.ChatID})
	case telegram.ChatActionUnarchive:
		chat.IsArchived = false
		label = "Chat unarchived"
	case telegram.ChatActionClearHistory:
		state.Messages[event.ChatID] = nil
		delete(state.History, event.ChatID)
		chat.LastMessage = ""
		chat.LastMessageAt = 0
		label = "History cleared"
	case telegram.ChatActionDeleteConversation:
		label = "Conversation deleted"
		removeFromList = true
		deleteData = true
		commands = append(commands, CloseChatCommand{ChatID: event.ChatID})
	case telegram.ChatActionDeleteChat:
		label = "Chat deleted"
		removeFromList = true
		deleteData = true
		commands = append(commands, CloseChatCommand{ChatID: event.ChatID})
	case telegram.ChatActionLeaveChat:
		chat.IsMember = false
		label = "Left chat"
		removeFromList = true
		commands = append(commands, CloseChatCommand{ChatID: event.ChatID})
	case telegram.ChatActionJoinChat:
		chat.IsMember = true
		label = "Joined chat"
	}
	if removeFromList {
		removeChatFromMainList(state, event.ChatID, deleteData)
	}
	state.Focus = menu.PreviousFocus
	state.ChatActions = nil
	setToast(state, domain.AppError{Message: label}, 2*time.Second)
	return commands
}

func removeChatFromMainList(state *State, chatID domain.ChatID, deleteData bool) {
	index := chatIndex(state.Chats, chatID)
	if index >= 0 {
		selectedID, hadSelected := activeChatID(*state)
		focusedID, hadFocused := focusedChatID(*state)
		selectedRemoved := hadSelected && selectedID == chatID
		focusedRemoved := hadFocused && focusedID == chatID

		state.Chats = append(state.Chats[:index], state.Chats[index+1:]...)
		if state.DetailsChatID == chatID {
			state.DetailsChatID = 0
			state.DetailsOpen = false
			if state.Focus == FocusDetails {
				state.Focus = state.FocusBeforeInfo
			}
		}
		if len(state.Chats) == 0 {
			state.FocusedChat = -1
			state.SelectedChat = 0
			state.SelectedMessageChat = 0
			state.SelectedMessage = 0
		} else {
			if selectedRemoved || !hadSelected {
				state.SelectedChat = min(index, len(state.Chats)-1)
				selectNewestMessage(state, state.Chats[state.SelectedChat].ID)
			} else {
				preserveChatSelection(state, selectedID)
			}
			if focusedRemoved || !hadFocused {
				state.FocusedChat = min(index, len(state.Chats)-1)
			} else {
				preserveChatFocus(state, focusedID)
			}
		}
	}
	if deleteData {
		delete(state.Messages, chatID)
		delete(state.History, chatID)
		delete(state.Drafts, chatID)
		delete(state.DraftReplies, chatID)
		delete(state.DraftDates, chatID)
		delete(state.DraftSync, chatID)
		delete(state.Thumbnails, chatID)
	}
}

func reduceChatActionFailed(state *State, event ChatActionFailed) []Effect {
	menu := state.ChatActions
	if menu == nil || !menu.Working || menu.RequestID != event.RequestID || menu.ChatID != event.ChatID {
		return nil
	}
	menu.Working = false
	setToast(state, event.Error, 3*time.Second)
	return nil
}

func chatActionError(action telegram.ChatAction) domain.AppError {
	message := "Could not update chat"
	switch action {
	case telegram.ChatActionMarkRead, telegram.ChatActionMarkUnread:
		message = "Could not change read state"
	case telegram.ChatActionMute, telegram.ChatActionUnmute:
		message = "Could not change notifications"
	case telegram.ChatActionPin, telegram.ChatActionUnpin:
		message = "Could not change pinned state"
	case telegram.ChatActionArchive, telegram.ChatActionUnarchive:
		message = "Could not move chat"
	case telegram.ChatActionClearHistory:
		message = "Could not clear history"
	case telegram.ChatActionDeleteConversation:
		message = "Could not delete conversation"
	case telegram.ChatActionDeleteChat:
		message = "Could not delete chat"
	case telegram.ChatActionLeaveChat:
		message = "Could not leave chat"
	case telegram.ChatActionJoinChat:
		message = "Could not join chat"
	}
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "update chat", Message: message}
}
