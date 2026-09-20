package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
	"testing"
)

func TestDetailsActionItemsDynamic(t *testing.T) {
	chat := domain.Chat{ID: 1, Kind: domain.ChatSupergroup, CanRestrictMembers: true}
	items := DetailsActionItems(chat)
	if len(items) != 4 || items[1].Label != "Members" || items[2].Label != "Group permissions" || items[3].Label != "Chat settings" {
		t.Fatalf("items=%#v", items)
	}
	chat.CanManageInviteLinks = true
	items = DetailsActionItems(chat)
	if len(items) != 5 || items[2].Label != "Invite links" || items[4].Label != "Chat settings" {
		t.Fatalf("items=%#v", items)
	}
}

func TestAdministrationPermissionToggleAndSave(t *testing.T) {
	state := State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 1, Kind: domain.ChatSupergroup, CanRestrictMembers: true}}}
	state, cmd := updateState(state, ActionReceived{Action: OpenGroupPermissions})
	if state.Administration == nil || len(cmd) != 1 {
		t.Fatalf("open state=%#v cmd=%#v", state.Administration, cmd)
	}
	id := state.Administration.RequestID
	state, _ = updateState(state, AdministrationLoaded{RequestID: id, ChatID: 1, Snapshot: telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup, CanRestrictMembers: true}})
	state, _ = updateState(state, ActionReceived{Action: ToggleAdministrationItem, AdminIndex: 0})
	if !state.Administration.EditedPermissions.CanSendBasicMessages {
		t.Fatal("toggle failed")
	}
	state, cmd = updateState(state, ActionReceived{Action: SaveAdministration})
	if len(cmd) != 1 || !state.Administration.Working {
		t.Fatalf("save state=%#v cmd=%#v", state.Administration, cmd)
	}
}

func TestAdministrationStaleAndSanitizedFailure(t *testing.T) {
	state := State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 1, Kind: domain.ChatSupergroup, CanRestrictMembers: true}}}
	state, _ = updateState(state, ActionReceived{Action: OpenGroupPermissions})
	id := state.Administration.RequestID
	state, _ = updateState(state, AdministrationLoadFailed{RequestID: id + 1, ChatID: 1})
	if state.Administration.Error != nil {
		t.Fatal("stale failure applied")
	}
	state, _ = updateState(state, AdministrationLoadFailed{RequestID: id, ChatID: 1})
	if state.Administration.Error == nil || state.Administration.Error.Message == "" {
		t.Fatal("failure not sanitized")
	}
}
