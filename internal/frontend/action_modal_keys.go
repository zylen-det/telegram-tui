package frontend

import tea "charm.land/bubbletea/v2"

// actionModalShortcut is shared by the action row paint and keyboard routing.
// Uppercase keys distinguish destructive/global-looking actions from their
// lowercase counterparts; j/k and q stay reserved for list navigation.
func actionModalShortcut(action Action) string {
	switch action {
	case ViewMessageMedia:
		return "v"
	case ReplyMessage, MarkChatRead, MarkChatUnread:
		return "r"
	case GoToReferencedMessage:
		return "g"
	case ForwardMessageSource:
		return "f"
	case EditMessage:
		return "e"
	case CopyMessage:
		return "y" // yank
	case ViewUserInfo, ViewChatInfo:
		return "i"
	case ReactMessage:
		return "a"
	case PinMessage:
		return "p"
	case OpenChat:
		return "o"
	case DeleteMessage, DeleteConversation, DeleteChat:
		return "d"
	case DeleteForEveryone:
		return "D"
	case ArchiveChat, UnarchiveChat:
		return "a"
	case PinChat, UnpinChat:
		return "p"
	case MuteChat, UnmuteChat:
		return "m"
	case ClearChatHistory:
		return "c"
	case LeaveChat:
		return "l"
	case JoinChat:
		return "J" // lowercase j moves selection
	case CancelChatAction:
		return "c"
	case ConfirmChatAction:
		return "y" // yes
	default:
		return ""
	}
}

// mapActionModalKey consumes every key while either action menu is open, so
// unrecognized keys never fall through to pane/global bindings. Only rows
// currently displayed by the capability-aware menu can be activated.
func mapActionModalKey(state State, msg tea.KeyPressMsg) (ActionReceived, bool) {
	if !(state.MessageMenu != nil && state.Focus == FocusModal ||
		state.ChatActions != nil && state.Focus == FocusChatActions) {
		return ActionReceived{}, false
	}
	key := msg.Key()
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod == tea.ModCtrl && (key.Code == 'c' || key.Code == 'C') {
		return actionReceived(Quit)
	}
	if key.Mod != 0 && key.Mod != tea.ModShift {
		return ActionReceived{}, true
	}
	if key.Code == tea.KeyEscape {
		return actionReceived(Close)
	}
	if key.Mod == 0 {
		if received, ok := mapListNavigationKey(state.Focus, key); ok {
			return received, true
		}
	}
	var rows []modalRowSpec
	if state.ChatActions != nil && state.Focus == FocusChatActions {
		if index := chatIndex(state.Chats, state.ChatActions.ChatID); index >= 0 {
			rows = chatActionRows(state.Chats[index], state.ChatActions)
		}
	} else {
		rows = messageActionRows(state.MessageMenu)
	}
	for _, row := range rows {
		if rowSelectable(row) && row.Key != "" && row.Key == keyText(key) {
			return row.Action, true
		}
	}
	return ActionReceived{}, true
}
