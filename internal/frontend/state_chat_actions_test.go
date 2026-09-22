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
	opened, commands := updateState(state, ActionReceived{Action: OpenChatActionMenu})
	if len(commands) != 0 || opened.ChatActions == nil || opened.Focus != FocusChatActions || opened.ChatActions.ChatID != 9 {
		t.Fatalf("opened = %#v commands=%#v", opened.ChatActions, commands)
	}
	items := ChatActionMenuItems(opened.Chats[0], opened.ChatActions)
	if len(items) == 0 || items[0].Action != OpenChat || items[0].Label != "Open chat" {
		t.Fatalf("items = %#v", items)
	}
	activated, commands := updateState(opened, ActionReceived{Action: Activate})
	if activated.ChatActions != nil || activated.Focus != FocusConversation {
		t.Fatalf("activated = focus %v menu %#v", activated.Focus, activated.ChatActions)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	if load, ok := commands[0].(LoadMessages); !ok || load.ChatID != 9 {
		t.Fatalf("open command = %#v", commands[0])
	}
}

func TestChatListActionsTargetFocusedChatWhileConversationStaysSelected(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 1, Title: "Selected", Kind: domain.ChatPrivate})
	state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Focused", Kind: domain.ChatPrivate, Muted: false})
	state.FocusedChat = 1
	state.SelectedChat = 0
	state.Messages[1] = []domain.Message{{ID: 11, ChatID: 1, Text: "selected conversation"}}

	focused, commands := updateState(state, ActionReceived{Action: SelectPrevious})
	if len(commands) != 0 || focused.FocusedChat != 0 || focused.SelectedChat != 0 {
		t.Fatalf("focus-only navigation = focused:%d selected:%d commands:%#v", focused.FocusedChat, focused.SelectedChat, commands)
	}

	state.FocusedChat = 1
	opened, commands := updateState(state, ActionReceived{Action: OpenChatActionMenu})
	if len(commands) != 0 || opened.ChatActions == nil || opened.ChatActions.ChatID != 2 || opened.SelectedChat != 0 {
		t.Fatalf("focused menu = %#v selected:%d commands:%#v", opened.ChatActions, opened.SelectedChat, commands)
	}
	working, commands := updateState(opened, ActionReceived{Action: MuteChat})
	if len(commands) != 1 || working.SelectedChat != 0 {
		t.Fatalf("focused action changed conversation: selected:%d commands:%#v", working.SelectedChat, commands)
	}
	command, ok := commands[0].(ApplyChatActionCommand)
	if !ok || command.ChatID != 2 || command.Action != telegram.ChatActionMute {
		t.Fatalf("focused action command = %#v", commands[0])
	}

	model := Select(working, nil)
	if model.ActiveChat.ID != 1 || model.ChatActions == nil || len(displayedChatActionRows(model)) == 0 {
		t.Fatalf("projection lost selected conversation or focused menu: active=%d menu=%#v", model.ActiveChat.ID, model.ChatActions)
	}

	infoMenu, _ := updateState(state, ActionReceived{Action: OpenChatActionMenu})
	infoMenu.ChatActions.Selected = 1
	info, commands := updateState(infoMenu, ActionReceived{Action: Activate})
	infoModel := Select(info, nil)
	if len(commands) != 0 || info.SelectedChat != 0 || info.DetailsChatID != 2 || infoModel.ActiveChat.ID != 1 || infoModel.DetailsChat.ID != 2 {
		t.Fatalf("focused info changed conversation: selected=%d details=%d active=%d detail=%d commands:%#v", info.SelectedChat, info.DetailsChatID, infoModel.ActiveChat.ID, infoModel.DetailsChat.ID, commands)
	}

	reopened, _ := updateState(state, ActionReceived{Action: OpenChatActionMenu})
	selected, commands := updateState(reopened, ActionReceived{Action: Activate})
	if selected.SelectedChat != 1 || selected.FocusedChat != 1 || selected.Focus != FocusConversation || len(commands) == 0 {
		t.Fatalf("open focused chat = focused:%d selected:%d focus:%v commands:%#v", selected.FocusedChat, selected.SelectedChat, selected.Focus, commands)
	}
	if active := Select(selected, nil).ActiveChat.ID; active != 2 {
		t.Fatalf("conversation active chat = %d, want focused chat 2", active)
	}
}

func TestRemovingFocusedChatKeepsSelectedConversation(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 1, Title: "Selected", Kind: domain.ChatPrivate})
	state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Focused", Kind: domain.ChatPrivate})
	state.SelectedChat = 0
	state.FocusedChat = 1
	opened, _ := updateState(state, ActionReceived{Action: OpenChatActionMenu})
	working, commands := updateState(opened, ActionReceived{Action: ArchiveChat})
	request := commands[0].(ApplyChatActionCommand)

	applied, commands := updateState(working, ChatActionApplied{RequestID: request.RequestID, ChatID: 2, Action: telegram.ChatActionArchive})
	if len(applied.Chats) != 1 || applied.Chats[0].ID != 1 || applied.SelectedChat != 0 || applied.FocusedChat != 0 {
		t.Fatalf("archive focused chat = chats:%#v selected:%d focused:%d", applied.Chats, applied.SelectedChat, applied.FocusedChat)
	}
	if len(commands) != 1 || commands[0] != (CloseChatCommand{ChatID: 2}) {
		t.Fatalf("archive commands = %#v", commands)
	}
}

func TestChatListOpenActionBypassesActions(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Group", Kind: domain.ChatSupergroup, IsMember: true})
	state.Layout = LayoutNarrow

	opened, commands := updateState(state, ActionReceived{Action: OpenChat})
	if opened.Focus != FocusConversation || opened.ChatActions != nil {
		t.Fatalf("opened = focus %v menu %#v", opened.Focus, opened.ChatActions)
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
	state, _ := updateState(chatActionState(domain.Chat{ID: 9, Muted: false, CanDeleteForSelf: true}), ActionReceived{Action: Activate})
	working, commands := updateState(state, ActionReceived{Action: MuteChat})
	if len(commands) != 1 || !working.ChatActions.Working {
		t.Fatalf("mute start = %#v commands=%#v", working.ChatActions, commands)
	}
	command, ok := commands[0].(ApplyChatActionCommand)
	if !ok || command.ChatID != 9 || command.Action != telegram.ChatActionMute || command.RequestID == 0 {
		t.Fatalf("mute command = %#v", commands[0])
	}
	stale, _ := updateState(working, ChatActionApplied{RequestID: command.RequestID + 1, ChatID: 9, Action: telegram.ChatActionMute})
	if stale.ChatActions == nil || !stale.ChatActions.Working || stale.Chats[0].Muted {
		t.Fatalf("stale result changed state = %#v", stale)
	}
	applied, _ := updateState(working, ChatActionApplied{RequestID: command.RequestID, ChatID: 9, Action: telegram.ChatActionMute})
	if applied.ChatActions != nil || !applied.Chats[0].Muted || applied.Toast == nil || applied.Toast.Message != "Notifications muted" {
		t.Fatalf("applied = %#v", applied)
	}

	confirming, commands := updateState(state, ActionReceived{Action: ClearChatHistory})
	if len(commands) != 0 || confirming.ChatActions.Confirming != ClearChatHistory {
		t.Fatalf("confirming = %#v commands=%#v", confirming.ChatActions, commands)
	}
	items := ChatActionMenuItems(confirming.Chats[0], confirming.ChatActions)
	if len(items) != 2 || items[0].Action != CancelChatAction || items[1].Action != ConfirmChatAction {
		t.Fatalf("confirmation items = %#v", items)
	}
	confirmed, commands := updateState(confirming, ActionReceived{Action: ConfirmChatAction})
	if len(commands) != 1 || !confirmed.ChatActions.Working {
		t.Fatalf("confirmed = %#v commands=%#v", confirmed.ChatActions, commands)
	}
	if action := commands[0].(ApplyChatActionCommand).Action; action != telegram.ChatActionClearHistory {
		t.Fatalf("confirmed action = %v", action)
	}
}

func TestChatActionFailureRemainsOpenAndSanitized(t *testing.T) {
	state, _ := updateState(chatActionState(domain.Chat{ID: 9}), ActionReceived{Action: Activate})
	working, commands := updateState(state, ActionReceived{Action: ArchiveChat})
	request := commands[0].(ApplyChatActionCommand)
	failed, _ := updateState(working, ChatActionFailed{RequestID: request.RequestID, ChatID: 9, Action: request.Action, Error: chatActionError(request.Action)})
	if failed.ChatActions == nil || failed.ChatActions.Working || failed.Toast == nil || failed.Toast.Message != "Could not move chat" || failed.Toast.Cause != nil {
		t.Fatalf("failed = %#v", failed)
	}
}
