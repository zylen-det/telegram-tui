package app

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
	opened, commands := Reduce(state, ActionReceived{Action: Activate})
	if len(commands) != 0 || opened.ChatActions == nil || opened.Focus != FocusChatActions || opened.ChatActions.ChatID != 9 {
		t.Fatalf("opened = %#v commands=%#v", opened.ChatActions, commands)
	}
	items := ChatActionMenuItems(opened.Chats[0], opened.ChatActions)
	if len(items) == 0 || items[0].Action != OpenChatFromMenu || items[0].Label != "Open chat" {
		t.Fatalf("items = %#v", items)
	}
	activated, commands := Reduce(opened, ActionReceived{Action: Activate})
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
	state, _ := Reduce(chatActionState(domain.Chat{ID: 9, Muted: false, CanDeleteForSelf: true}), ActionReceived{Action: Activate})
	working, commands := Reduce(state, ActionReceived{Action: MuteChat})
	if len(commands) != 1 || !working.ChatActions.Working {
		t.Fatalf("mute start = %#v commands=%#v", working.ChatActions, commands)
	}
	command, ok := commands[0].(ApplyChatActionCommand)
	if !ok || command.ChatID != 9 || command.Action != telegram.ChatActionMute || command.RequestID == 0 {
		t.Fatalf("mute command = %#v", commands[0])
	}
	stale, _ := Reduce(working, ChatActionApplied{RequestID: command.RequestID + 1, ChatID: 9, Action: telegram.ChatActionMute})
	if stale.ChatActions == nil || !stale.ChatActions.Working || stale.Chats[0].Muted {
		t.Fatalf("stale result changed state = %#v", stale)
	}
	applied, _ := Reduce(working, ChatActionApplied{RequestID: command.RequestID, ChatID: 9, Action: telegram.ChatActionMute})
	if applied.ChatActions != nil || !applied.Chats[0].Muted || applied.Toast == nil || applied.Toast.Message != "Notifications muted" {
		t.Fatalf("applied = %#v", applied)
	}

	confirming, commands := Reduce(state, ActionReceived{Action: ClearChatHistory})
	if len(commands) != 0 || confirming.ChatActions.Confirming != ClearChatHistory {
		t.Fatalf("confirming = %#v commands=%#v", confirming.ChatActions, commands)
	}
	items := ChatActionMenuItems(confirming.Chats[0], confirming.ChatActions)
	if len(items) != 2 || items[0].Action != CancelChatAction || items[1].Action != ConfirmChatAction {
		t.Fatalf("confirmation items = %#v", items)
	}
	confirmed, commands := Reduce(confirming, ActionReceived{Action: ConfirmChatAction})
	if len(commands) != 1 || !confirmed.ChatActions.Working {
		t.Fatalf("confirmed = %#v commands=%#v", confirmed.ChatActions, commands)
	}
	if action := commands[0].(ApplyChatActionCommand).Action; action != telegram.ChatActionClearHistory {
		t.Fatalf("confirmed action = %v", action)
	}
}

func TestChatActionFailureRemainsOpenAndSanitized(t *testing.T) {
	state, _ := Reduce(chatActionState(domain.Chat{ID: 9}), ActionReceived{Action: Activate})
	working, commands := Reduce(state, ActionReceived{Action: ArchiveChat})
	request := commands[0].(ApplyChatActionCommand)
	failed, _ := Reduce(working, ChatActionFailed{RequestID: request.RequestID, ChatID: 9, Action: request.Action, Error: chatActionError(request.Action)})
	if failed.ChatActions == nil || failed.ChatActions.Working || failed.Toast == nil || failed.Toast.Message != "Could not move chat" || failed.Toast.Cause != nil {
		t.Fatalf("failed = %#v", failed)
	}
}
