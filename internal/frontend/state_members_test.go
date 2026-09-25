package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func membersBaseState(kind domain.ChatKind) State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusDetails
	state.DetailsOpen = true
	state.Chats = []domain.Chat{{ID: 9, Kind: kind, Title: "Group"}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	state.Drafts[9] = "keep draft"
	return state
}

func openMembersForTest(t *testing.T, state State) State {
	t.Helper()
	commands := updateState(&state, ActionReceived{Action: OpenMembers})
	if state.Members == nil || state.Members.ChatID != 9 || state.Focus != FocusMembers {
		t.Fatalf("open = %#v focus=%v", state.Members, state.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	command, ok := commands[0].(LoadMembers)
	if !ok || command.RequestID != 10 || command.ChatID != 9 || command.Cursor != (telegram.MemberCursor{Limit: pageSize}) {
		t.Fatalf("open command = %#v", commands[0])
	}
	return state
}

func TestOpenMembersRequiresDetailsAndGroupKind(t *testing.T) {
	for _, kind := range []domain.ChatKind{domain.ChatBasicGroup, domain.ChatSupergroup, domain.ChatChannel} {
		openMembersForTest(t, membersBaseState(kind))
	}
	private := membersBaseState(domain.ChatPrivate)
	commands := updateState(&private, ActionReceived{Action: OpenMembers})
	if private.Members != nil || len(commands) != 0 {
		t.Fatalf("private open = %#v commands=%#v", private.Members, commands)
	}
	notDetails := membersBaseState(domain.ChatSupergroup)
	notDetails.Focus = FocusConversation
	commands = updateState(&notDetails, ActionReceived{Action: OpenMembers})
	if notDetails.Members != nil || len(commands) != 0 {
		t.Fatalf("non-details open = %#v commands=%#v", notDetails.Members, commands)
	}
}

func TestMembersLoadedDeduplicatesAndRejectsStaleResults(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	page := telegram.MemberPage{
		Members: []domain.ChatMember{
			{User: domain.User{ID: 1, Name: "Owner"}, Role: domain.ChatMemberRoleOwner},
			{User: domain.User{ID: 1, Name: "Duplicate"}},
			{User: domain.User{ID: 2, Name: "Member"}},
		},
		TotalCount: 5,
		NextOffset: 3,
	}
	commands := updateState(&opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: page})
	if len(commands) != 0 || opened.Members.Loading || opened.Members.Done || opened.Members.NextOffset != 3 || opened.Members.TotalCount != 5 {
		t.Fatalf("loaded = %#v commands=%#v", opened.Members, commands)
	}
	if len(opened.Members.Results) != 2 || opened.Members.Results[0].User.ID != 1 || opened.Members.Results[1].User.ID != 2 {
		t.Fatalf("results = %#v", opened.Members.Results)
	}
	want := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	updateState(&want, MembersLoaded{RequestID: 10, ChatID: 9, Page: page})
	staleCommands := updateState(&opened, MembersLoaded{RequestID: 11, ChatID: 9, Page: page})
	if len(staleCommands) != 0 || !reflect.DeepEqual(opened, want) {
		t.Fatalf("stale member page changed state: %#v effects=%#v", opened.Members, staleCommands)
	}
}

func TestMembersSelectionPaginatesWithSourceOffset(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	updateState(&opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1}}, {User: domain.User{ID: 2}}},
		TotalCount: 10,
		NextOffset: 4,
	}})
	commands := updateState(&opened, ActionReceived{Action: SelectNext})
	if opened.Members.Selected != 1 || !opened.Members.Loading || len(commands) != 1 {
		t.Fatalf("paged = %#v commands=%#v", opened.Members, commands)
	}
	command, ok := commands[0].(LoadMembers)
	if !ok || command.RequestID != 11 || command.Cursor.Offset != 4 || command.Cursor.Limit != pageSize {
		t.Fatalf("pagination command = %#v", commands[0])
	}
}

func TestMembersEmptyFilteredPageContinuesAndNoProgressStops(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	commands := updateState(&opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{TotalCount: 10, NextOffset: 5}})
	if len(commands) != 1 || !opened.Members.Loading {
		t.Fatalf("continued = %#v commands=%#v", opened.Members, commands)
	}
	command := commands[0].(LoadMembers)
	if command.Cursor.Offset != 5 || command.RequestID != 11 {
		t.Fatalf("continue command = %#v", command)
	}
	commands = updateState(&opened, MembersLoaded{RequestID: 11, ChatID: 9, Page: telegram.MemberPage{TotalCount: 10, NextOffset: 5}})
	if len(commands) != 0 || !opened.Members.Done || opened.Members.Loading {
		t.Fatalf("stopped = %#v commands=%#v", opened.Members, commands)
	}
}

func TestMembersSelectionFailureCloseAndClone(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	updateState(&opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "One"}}, {User: domain.User{ID: 2, Name: "Two"}}},
		TotalCount: 2,
		NextOffset: 2,
		Done:       true,
	}})
	updateState(&opened, ActionReceived{Action: SelectNext})
	if opened.Members.Selected != 1 {
		t.Fatalf("selected = %d", opened.Members.Selected)
	}
	updateState(&opened, ActionReceived{Action: Close})
	if opened.Members != nil || opened.Focus != FocusDetails || opened.Drafts[9] != "keep draft" {
		t.Fatalf("closed = members=%#v focus=%v draft=%q", opened.Members, opened.Focus, opened.Drafts[9])
	}

	opened = openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	updateState(&opened, MembersLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Message: "private raw error"}})
	if opened.Members.Error == nil || opened.Members.Error.Message != "Could not load members" || opened.Members.Loading {
		t.Fatalf("failure = %#v", opened.Members)
	}
}
