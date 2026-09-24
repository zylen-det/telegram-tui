package frontend

import (
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestAdministrationRightsAndRestrictionsRespectLiveLimits(t *testing.T) {
	snapshot := &telegram.AdministrationSnapshot{
		Kind: domain.ChatSupergroup, CanPromoteMembers: true, CanRestrictMembers: true,
		OwnRights:          telegram.AdministratorRights{CanInviteUsers: true},
		DefaultPermissions: telegram.ChatPermissions{CanSendBasicMessages: true},
	}
	state := State{Focus: FocusAdministration, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup}},
		Administration: &AdministrationState{ChatID: 9, UserID: 2, Mode: AdministrationAdminRightsEditor, Snapshot: snapshot, MemberStatus: &telegram.MemberAdministrationStatus{Role: domain.ChatMemberRoleMember}}}
	items := AdministrationMenuItems(state.Administration)
	for _, item := range items {
		if strings.Contains(item.Label, "Manage topics") {
			t.Fatal("forum-only right appeared in non-forum group")
		}
	}
	updateState(&state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 5})
	if !state.Administration.EditedRights.CanInviteUsers {
		t.Fatal("own invite right could not be enabled")
	}
	updateState(&state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 6})
	if state.Administration.EditedRights.CanRestrictMembers {
		t.Fatal("right beyond current admin rights was enabled")
	}
	state.Administration.IsForum = true
	foundTopics := false
	for _, item := range AdministrationMenuItems(state.Administration) {
		foundTopics = foundTopics || strings.Contains(item.Label, "Manage topics")
	}
	if !foundTopics {
		t.Fatal("forum topic right hidden in forum")
	}
	state.Administration.Mode = AdministrationMemberMenu
	state.Administration.MemberStatus = &telegram.MemberAdministrationStatus{Role: domain.ChatMemberRoleRestricted, Permissions: telegram.ChatPermissions{CanSendPhotos: true}}
	updateState(&state, ActionReceived{Action: SelectAdministrationAction, ChatID: 9, UserID: 2, AdminIndex: 4})
	if state.Administration.Mode != AdministrationRestrictionsEditor || !state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatalf("restriction editor did not load current restrictions: %#v", state.Administration)
	}
	updateState(&state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 3})
	if state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatal("existing extra photo permission could not be removed")
	}
	updateState(&state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 3})
	if state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatal("restriction editor enabled permission forbidden by group")
	}
}

func TestMemberAdministrationRestoresModalAndRejectsStaleAction(t *testing.T) {
	memberFixture := func() State {
		return State{Focus: FocusMembers, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanRestrictMembers: true}},
			Members: &MembersState{ChatID: 9, Detail: &MemberDetail{UserID: 2, Name: "Ada", CanManageInChat: true}}}
	}
	state := memberFixture()
	commands := updateState(&state, ActionReceived{Action: OpenMemberAdministration, ChatID: 9, UserID: 2})
	if state.Members != nil || state.Administration == nil || len(commands) != 2 {
		t.Fatalf("member admin did not replace modal: members=%#v admin=%#v commands=%#v", state.Members, state.Administration, commands)
	}
	request := state.Administration.RequestID

	// Closing restores the member detail. updateState mutates in place, so
	// this branch opens its own independent copy of the modal.
	closing := memberFixture()
	updateState(&closing, ActionReceived{Action: OpenMemberAdministration, ChatID: 9, UserID: 2})
	updateState(&closing, ActionReceived{Action: Close})
	if closing.Administration != nil || closing.Members == nil || closing.Members.Detail == nil || closing.Members.Detail.UserID != 2 || closing.Focus != FocusMembers {
		t.Fatalf("closing did not restore detail: %#v", closing)
	}

	updateState(&state, AdministrationLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup, CanRestrictMembers: true}})
	updateState(&state, MemberAdministrationLoaded{RequestID: request, ChatID: 9, UserID: 2, Status: telegram.MemberAdministrationStatus{Role: domain.ChatMemberRoleMember}})
	updateState(&state, ActionReceived{Action: SelectAdministrationAction, ChatID: 9, UserID: 2, AdminIndex: 7})
	if state.Administration.Mode != AdministrationConfirmation {
		t.Fatal("ban did not enter confirmation")
	}
	commands = updateState(&state, ActionReceived{Action: ConfirmAdministrationAction, ChatID: 9, UserID: 2})
	if len(commands) != 1 || !state.Administration.Working {
		t.Fatalf("ban did not start: %#v %#v", state.Administration, commands)
	}
	actionRequest := state.Administration.RequestID
	updateState(&state, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 3, Action: telegram.MemberAdministrationBan})
	if state.Administration == nil || !state.Administration.Working {
		t.Fatal("wrong user action completed modal")
	}
	updateState(&state, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 2, Action: telegram.MemberAdministrationRemove})
	if state.Administration == nil || !state.Administration.Working {
		t.Fatal("wrong action completed modal")
	}
	commands = updateState(&state, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 2, Action: telegram.MemberAdministrationBan})
	if state.Administration != nil || state.Members == nil || state.Members.Detail != nil || state.Members.Notice != "Member updated" || state.Focus != FocusMembers || len(commands) != 1 {
		t.Fatalf("success did not refresh member list: members=%#v admin=%#v commands=%#v", state.Members, state.Administration, commands)
	}
}

func TestAdministrationLoadFailureRetriesWithoutLeakingOldResults(t *testing.T) {
	state := State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanRestrictMembers: true}}}
	updateState(&state, ActionReceived{Action: OpenGroupPermissions})
	first := state.Administration.RequestID
	updateState(&state, AdministrationLoadFailed{RequestID: first, ChatID: 9})
	if state.Administration.Error == nil || state.Administration.Error.Message != "Could not load administration" {
		t.Fatalf("failure message = %#v", state.Administration.Error)
	}
	if items := AdministrationMenuItems(state.Administration); len(items) != 1 || items[0].Action.Action != Retry {
		t.Fatalf("failed editor offered editable rows: %#v", items)
	}
	commands := updateState(&state, ActionReceived{Action: Retry, ChatID: 9})
	if state.Administration.RequestID == first || !state.Administration.Loading || len(commands) != 1 {
		t.Fatalf("retry did not reload: %#v %#v", state.Administration, commands)
	}
	updateState(&state, AdministrationLoaded{RequestID: first, ChatID: 9, Snapshot: telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup, CanRestrictMembers: true}})
	if state.Administration.Snapshot != nil || !state.Administration.Loading {
		t.Fatal("old result replaced retry state")
	}
}
