package app

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
	state, _ = Reduce(state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 5})
	if !state.Administration.EditedRights.CanInviteUsers {
		t.Fatal("own invite right could not be enabled")
	}
	state, _ = Reduce(state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 6})
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
	state, _ = Reduce(state, ActionReceived{Action: SelectAdministrationAction, ChatID: 9, UserID: 2, AdminIndex: 4})
	if state.Administration.Mode != AdministrationRestrictionsEditor || !state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatalf("restriction editor did not load current restrictions: %#v", state.Administration)
	}
	state, _ = Reduce(state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 3})
	if state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatal("existing extra photo permission could not be removed")
	}
	state, _ = Reduce(state, ActionReceived{Action: ToggleAdministrationItem, ChatID: 9, UserID: 2, AdminIndex: 3})
	if state.Administration.EditedPermissions.CanSendPhotos {
		t.Fatal("restriction editor enabled permission forbidden by group")
	}
}

func TestMemberAdministrationRestoresModalAndRejectsStaleAction(t *testing.T) {
	base := State{Focus: FocusMembers, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanRestrictMembers: true}},
		Members: &MembersState{ChatID: 9, Detail: &MemberDetail{UserID: 2, Name: "Ada", CanManageInChat: true}}}
	opened, commands := Reduce(base, ActionReceived{Action: OpenMemberAdministration, ChatID: 9, UserID: 2})
	if opened.Members != nil || opened.Administration == nil || len(commands) != 2 {
		t.Fatalf("member admin did not replace modal: members=%#v admin=%#v commands=%#v", opened.Members, opened.Administration, commands)
	}
	backed, _ := Reduce(opened, ActionReceived{Action: Close})
	if backed.Administration != nil || backed.Members == nil || backed.Members.Detail == nil || backed.Focus != FocusMembers {
		t.Fatalf("closing did not restore detail: %#v", backed)
	}
	request := opened.Administration.RequestID
	opened, _ = Reduce(opened, AdministrationLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup, CanRestrictMembers: true}})
	opened, _ = Reduce(opened, MemberAdministrationLoaded{RequestID: request, ChatID: 9, UserID: 2, Status: telegram.MemberAdministrationStatus{Role: domain.ChatMemberRoleMember}})
	opened, _ = Reduce(opened, ActionReceived{Action: SelectAdministrationAction, ChatID: 9, UserID: 2, AdminIndex: 7})
	if opened.Administration.Mode != AdministrationConfirmation {
		t.Fatal("ban did not enter confirmation")
	}
	working, commands := Reduce(opened, ActionReceived{Action: ConfirmAdministrationAction, ChatID: 9, UserID: 2})
	if len(commands) != 1 || !working.Administration.Working {
		t.Fatalf("ban did not start: %#v %#v", working.Administration, commands)
	}
	actionRequest := working.Administration.RequestID
	stale, _ := Reduce(working, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 3, Action: telegram.MemberAdministrationBan})
	if stale.Administration == nil || !stale.Administration.Working {
		t.Fatal("wrong user action completed modal")
	}
	stale, _ = Reduce(working, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 2, Action: telegram.MemberAdministrationRemove})
	if stale.Administration == nil || !stale.Administration.Working {
		t.Fatal("wrong action completed modal")
	}
	refreshed, commands := Reduce(working, MemberAdministrationApplied{RequestID: actionRequest, ChatID: 9, UserID: 2, Action: telegram.MemberAdministrationBan})
	if refreshed.Administration != nil || refreshed.Members == nil || refreshed.Members.Detail != nil || refreshed.Members.Notice != "Member updated" || refreshed.Focus != FocusMembers || len(commands) != 1 {
		t.Fatalf("success did not refresh member list: members=%#v admin=%#v commands=%#v", refreshed.Members, refreshed.Administration, commands)
	}
}

func TestAdministrationLoadFailureRetriesWithoutLeakingOldResults(t *testing.T) {
	base := State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanRestrictMembers: true}}}
	opened, _ := Reduce(base, ActionReceived{Action: OpenGroupPermissions})
	first := opened.Administration.RequestID
	failed, _ := Reduce(opened, AdministrationLoadFailed{RequestID: first, ChatID: 9})
	if failed.Administration.Error == nil || failed.Administration.Error.Message != "Could not load administration" {
		t.Fatalf("failure message = %#v", failed.Administration.Error)
	}
	if items := AdministrationMenuItems(failed.Administration); len(items) != 1 || items[0].Action.Action != Retry {
		t.Fatalf("failed editor offered editable rows: %#v", items)
	}
	retrying, commands := Reduce(failed, ActionReceived{Action: Retry, ChatID: 9})
	if retrying.Administration.RequestID == first || !retrying.Administration.Loading || len(commands) != 1 {
		t.Fatalf("retry did not reload: %#v %#v", retrying.Administration, commands)
	}
	stale, _ := Reduce(retrying, AdministrationLoaded{RequestID: first, ChatID: 9, Snapshot: telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup, CanRestrictMembers: true}})
	if stale.Administration.Snapshot != nil || !stale.Administration.Loading {
		t.Fatal("old result replaced retry state")
	}
}
