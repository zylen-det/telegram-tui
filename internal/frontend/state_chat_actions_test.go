package frontend

import (
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func chatActionState(chat domain.Chat) State {
	state := InitialState()
	state.Layout = LayoutWide
	state.Focus = FocusChats
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{chat}
	state.SelectedChat = 0
	return state
}

func TestChatListEnterOpensActionsAndOpenChatIsFirst(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Group", Kind: domain.ChatSupergroup, IsMember: true})
	commands := updateState(&state, ActionReceived{Action: OpenChatActionMenu})
	if len(commands) != 0 || state.ChatActions == nil || state.Focus != FocusChatActions || state.ChatActions.ChatID != 9 {
		t.Fatalf("opened = %#v commands=%#v", state.ChatActions, commands)
	}
	items := ChatActionMenuItems(state.Chats[0], state.ChatActions)
	if len(items) == 0 || items[0].Action != OpenChat || items[0].Label != "Open chat" {
		t.Fatalf("items = %#v", items)
	}
	commands = updateState(&state, ActionReceived{Action: Activate})
	if state.ChatActions != nil || state.Focus != FocusConversation {
		t.Fatalf("activated = focus %v menu %#v", state.Focus, state.ChatActions)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	if load, ok := commands[0].(LoadMessages); !ok || load.ChatID != 9 {
		t.Fatalf("open command = %#v", commands[0])
	}
}

func TestChatListActionsTargetFocusedChatWhileConversationStaysSelected(t *testing.T) {
	fixture := func() State {
		state := chatActionState(domain.Chat{ID: 1, Title: "Selected", Kind: domain.ChatPrivate})
		state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Focused", Kind: domain.ChatPrivate, Muted: false})
		state.FocusedChat = 1
		state.SelectedChat = 0
		state.Messages[1] = []domain.Message{{ID: 11, ChatID: 1, Text: "selected conversation"}}
		return state
	}

	focused := fixture()
	commands := updateState(&focused, ActionReceived{Action: SelectPrevious})
	if len(commands) != 0 || focused.FocusedChat != 0 || focused.SelectedChat != 0 {
		t.Fatalf("focus-only navigation = focused:%d selected:%d commands:%#v", focused.FocusedChat, focused.SelectedChat, commands)
	}

	// updateState mutates in place, so each branch rebuilds its own fixture.
	opened := fixture()
	commands = updateState(&opened, ActionReceived{Action: OpenChatActionMenu})
	if len(commands) != 0 || opened.ChatActions == nil || opened.ChatActions.ChatID != 2 || opened.SelectedChat != 0 {
		t.Fatalf("focused menu = %#v selected:%d commands=%#v", opened.ChatActions, opened.SelectedChat, commands)
	}
	commands = updateState(&opened, ActionReceived{Action: MuteChat})
	if len(commands) != 1 || opened.SelectedChat != 0 {
		t.Fatalf("focused action changed conversation: selected:%d commands:%#v", opened.SelectedChat, commands)
	}
	command, ok := commands[0].(ApplyChatActionCommand)
	if !ok || command.ChatID != 2 || command.Action != telegram.ChatActionMute {
		t.Fatalf("focused action command = %#v", commands[0])
	}

	model := Select(opened, nil)
	if model.ActiveChat.ID != 1 || model.ChatActions == nil || len(displayedChatActionRows(model)) == 0 {
		t.Fatalf("projection lost selected conversation or focused menu: active=%d menu=%#v", model.ActiveChat.ID, model.ChatActions)
	}

	infoMenu := fixture()
	updateState(&infoMenu, ActionReceived{Action: OpenChatActionMenu})
	infoMenu.ChatActions.Selected = 1
	commands = updateState(&infoMenu, ActionReceived{Action: Activate})
	infoModel := Select(infoMenu, nil)
	if len(commands) != 0 || infoMenu.SelectedChat != 0 || infoMenu.DetailsChatID != 2 || infoModel.ActiveChat.ID != 1 || infoModel.DetailsChat.ID != 2 {
		t.Fatalf("focused info changed conversation: selected=%d details=%d active=%d detail=%d commands:%#v", infoMenu.SelectedChat, infoMenu.DetailsChatID, infoModel.ActiveChat.ID, infoModel.DetailsChat.ID, commands)
	}

	reopened := fixture()
	updateState(&reopened, ActionReceived{Action: OpenChatActionMenu})
	commands = updateState(&reopened, ActionReceived{Action: Activate})
	if reopened.SelectedChat != 1 || reopened.FocusedChat != 1 || reopened.Focus != FocusConversation || len(commands) == 0 {
		t.Fatalf("open focused chat = focused:%d selected:%d focus:%v commands:%#v", reopened.FocusedChat, reopened.SelectedChat, reopened.Focus, commands)
	}
	if active := Select(reopened, nil).ActiveChat.ID; active != 2 {
		t.Fatalf("conversation active chat = %d, want focused chat 2", active)
	}
}

func TestRemovingFocusedChatKeepsSelectedConversation(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 1, Title: "Selected", Kind: domain.ChatPrivate})
	state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Focused", Kind: domain.ChatPrivate})
	state.SelectedChat = 0
	state.FocusedChat = 1
	updateState(&state, ActionReceived{Action: OpenChatActionMenu})
	commands := updateState(&state, ActionReceived{Action: ArchiveChat})
	request := commands[0].(ApplyChatActionCommand)

	commands = updateState(&state, ChatActionApplied{RequestID: request.RequestID, ChatID: 2, Action: telegram.ChatActionArchive})
	if len(state.Chats) != 1 || state.Chats[0].ID != 1 || state.SelectedChat != 0 || state.FocusedChat != 0 {
		t.Fatalf("archive focused chat = chats:%#v selected:%d focused:%d", state.Chats, state.SelectedChat, state.FocusedChat)
	}
	if len(commands) != 1 || commands[0] != (CloseChatCommand{ChatID: 2}) {
		t.Fatalf("archive commands = %#v", commands)
	}
}

func TestChatListOpenActionBypassesActions(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Group", Kind: domain.ChatSupergroup, IsMember: true})
	state.Layout = LayoutNarrow

	commands := updateState(&state, ActionReceived{Action: OpenChat})
	if state.Focus != FocusConversation || state.ChatActions != nil {
		t.Fatalf("opened = focus %v menu %#v", state.Focus, state.ChatActions)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	if load, ok := commands[0].(LoadMessages); !ok || load.ChatID != 9 {
		t.Fatalf("open command = %#v", commands[0])
	}
}

func TestChatActionRowsAreCapabilityAndStateAware(t *testing.T) {
	chat := domain.Chat{
		ID: 9, Kind: domain.ChatChannel, IsMember: true, Muted: true,
		IsPinned: true, IsArchived: true, IsMarkedUnread: true,
		CanDeleteForSelf: true, CanDeleteForAll: true,
	}
	menu := &ChatActionMenuState{ChatID: 9}
	items := ChatActionMenuItems(chat, menu)
	labels := make(map[string]bool, len(items))
	for _, item := range items {
		labels[item.Label] = true
	}
	for _, label := range []string{"View channel", "Unarchive", "Unpin", "Unmute", "Mark as read", "Clear history", "Leave channel", "Delete channel"} {
		if !labels[label] {
			t.Fatalf("missing %q in %#v", label, items)
		}
	}
}

func TestChatActionDispatchConfirmationAndStaleSafety(t *testing.T) {
	opened := func() State {
		state := chatActionState(domain.Chat{ID: 9, Muted: false, CanDeleteForSelf: true})
		updateState(&state, ActionReceived{Action: Activate})
		return state
	}
	working := opened()
	commands := updateState(&working, ActionReceived{Action: MuteChat})
	if len(commands) != 1 || !working.ChatActions.Working {
		t.Fatalf("mute start = %#v commands=%#v", working.ChatActions, commands)
	}
	command, ok := commands[0].(ApplyChatActionCommand)
	if !ok || command.ChatID != 9 || command.Action != telegram.ChatActionMute || command.RequestID == 0 {
		t.Fatalf("mute command = %#v", commands[0])
	}
	updateState(&working, ChatActionApplied{RequestID: command.RequestID + 1, ChatID: 9, Action: telegram.ChatActionMute})
	if working.ChatActions == nil || !working.ChatActions.Working || working.Chats[0].Muted {
		t.Fatalf("stale result changed state = %#v", working)
	}
	updateState(&working, ChatActionApplied{RequestID: command.RequestID, ChatID: 9, Action: telegram.ChatActionMute})
	if working.ChatActions != nil || !working.Chats[0].Muted || working.Toast == nil || working.Toast.Message != "Notifications muted" {
		t.Fatalf("applied = %#v", working)
	}

	// The confirmation flow starts from a freshly opened menu, independent
	// of the mute branch above.
	confirming := opened()
	commands = updateState(&confirming, ActionReceived{Action: ClearChatHistory})
	if len(commands) != 0 || confirming.ChatActions.Confirming != ClearChatHistory {
		t.Fatalf("confirming = %#v commands=%#v", confirming.ChatActions, commands)
	}
	items := ChatActionMenuItems(confirming.Chats[0], confirming.ChatActions)
	if len(items) != 2 || items[0].Action != CancelChatAction || items[1].Action != ConfirmChatAction {
		t.Fatalf("confirmation items = %#v", items)
	}
	commands = updateState(&confirming, ActionReceived{Action: ConfirmChatAction})
	if len(commands) != 1 || !confirming.ChatActions.Working {
		t.Fatalf("confirmed = %#v commands=%#v", confirming.ChatActions, commands)
	}
	if action := commands[0].(ApplyChatActionCommand).Action; action != telegram.ChatActionClearHistory {
		t.Fatalf("confirmed action = %v", action)
	}
}

func TestChatActionFailureRemainsOpenAndSanitized(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9})
	updateState(&state, ActionReceived{Action: Activate})
	commands := updateState(&state, ActionReceived{Action: ArchiveChat})
	request := commands[0].(ApplyChatActionCommand)
	updateState(&state, ChatActionFailed{RequestID: request.RequestID, ChatID: 9, Action: request.Action, Error: chatActionError(request.Action)})
	if state.ChatActions == nil || state.ChatActions.Working || state.Toast == nil || state.Toast.Message != "Could not move chat" || state.Toast.Cause != nil {
		t.Fatalf("failed = %#v", state)
	}
}
