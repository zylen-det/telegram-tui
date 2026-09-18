package app

import (
	"errors"
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
	opened, commands := Reduce(state, ActionReceived{Action: OpenMembers})
	if opened.Members == nil || opened.Members.ChatID != 9 || opened.Focus != FocusMembers {
		t.Fatalf("open = %#v focus=%v", opened.Members, opened.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	command, ok := commands[0].(LoadMembers)
	if !ok || command.RequestID != 10 || command.ChatID != 9 || command.Cursor != (telegram.MemberCursor{Limit: pageSize}) {
		t.Fatalf("open command = %#v", commands[0])
	}
	return opened
}

func TestOpenMembersRequiresDetailsAndGroupKind(t *testing.T) {
	for _, kind := range []domain.ChatKind{domain.ChatBasicGroup, domain.ChatSupergroup, domain.ChatChannel} {
		openMembersForTest(t, membersBaseState(kind))
	}
	private := membersBaseState(domain.ChatPrivate)
	unchanged, commands := Reduce(private, ActionReceived{Action: OpenMembers})
	if unchanged.Members != nil || len(commands) != 0 {
		t.Fatalf("private open = %#v commands=%#v", unchanged.Members, commands)
	}
	notDetails := membersBaseState(domain.ChatSupergroup)
	notDetails.Focus = FocusConversation
	unchanged, commands = Reduce(notDetails, ActionReceived{Action: OpenMembers})
	if unchanged.Members != nil || len(commands) != 0 {
		t.Fatalf("non-details open = %#v commands=%#v", unchanged.Members, commands)
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
	loaded, commands := Reduce(opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: page})
	if len(commands) != 0 || loaded.Members.Loading || loaded.Members.Done || loaded.Members.NextOffset != 3 || loaded.Members.TotalCount != 5 {
		t.Fatalf("loaded = %#v commands=%#v", loaded.Members, commands)
	}
	if len(loaded.Members.Results) != 2 || loaded.Members.Results[0].User.ID != 1 || loaded.Members.Results[1].User.ID != 2 {
		t.Fatalf("results = %#v", loaded.Members.Results)
	}
	stale, staleCommands := Reduce(loaded, MembersLoaded{RequestID: 11, ChatID: 9, Page: page})
	if len(staleCommands) != 0 || !reflect.DeepEqual(stale, loaded) {
		t.Fatal("stale member page mutated state")
	}
}

func TestMembersNavigationPaginatesWithSourceOffset(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1}}, {User: domain.User{ID: 2}}},
		TotalCount: 10,
		NextOffset: 4,
	}})
	paged, commands := Reduce(loaded, ActionReceived{Action: SelectNext})
	if paged.Members.Selected != 1 || !paged.Members.Loading || len(commands) != 1 {
		t.Fatalf("paged = %#v commands=%#v", paged.Members, commands)
	}
	command, ok := commands[0].(LoadMembers)
	if !ok || command.RequestID != 11 || command.Cursor.Offset != 4 || command.Cursor.Limit != pageSize {
		t.Fatalf("pagination command = %#v", commands[0])
	}
}

func TestMembersEmptyFilteredPageContinuesAndNoProgressStops(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	continued, commands := Reduce(opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{TotalCount: 10, NextOffset: 5}})
	if len(commands) != 1 || !continued.Members.Loading {
		t.Fatalf("continued = %#v commands=%#v", continued.Members, commands)
	}
	command := commands[0].(LoadMembers)
	if command.Cursor.Offset != 5 || command.RequestID != 11 {
		t.Fatalf("continue command = %#v", command)
	}
	stopped, commands := Reduce(continued, MembersLoaded{RequestID: 11, ChatID: 9, Page: telegram.MemberPage{TotalCount: 10, NextOffset: 5}})
	if len(commands) != 0 || !stopped.Members.Done || stopped.Members.Loading {
		t.Fatalf("stopped = %#v commands=%#v", stopped.Members, commands)
	}
}

func TestMembersSelectionFailureCloseAndClone(t *testing.T) {
	opened := openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: 10, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "One"}}, {User: domain.User{ID: 2, Name: "Two"}}},
		TotalCount: 2,
		NextOffset: 2,
		Done:       true,
	}})
	selected, _ := Reduce(loaded, ActionReceived{Action: SelectMember, ChatID: 9, UserID: 2})
	if selected.Members.Selected != 1 {
		t.Fatalf("selected = %d", selected.Members.Selected)
	}
	cloned := cloneReducerState(selected)
	cloned.Members.Results[0].User.Name = "mutated"
	if selected.Members.Results[0].User.Name != "One" {
		t.Fatal("member state clone aliases results")
	}
	closed, _ := Reduce(selected, ActionReceived{Action: Close})
	if closed.Members != nil || closed.Focus != FocusDetails || closed.Drafts[9] != "keep draft" {
		t.Fatalf("closed = members=%#v focus=%v draft=%q", closed.Members, closed.Focus, closed.Drafts[9])
	}

	opened = openMembersForTest(t, membersBaseState(domain.ChatSupergroup))
	failed, _ := Reduce(opened, MembersLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Message: "private raw error"}})
	if failed.Members.Error == nil || failed.Members.Error.Message != "Could not load members" || failed.Members.Loading {
		t.Fatalf("failure = %#v", failed.Members)
	}
}

func TestHandlerLoadMembersExactRequestAndSafeFailure(t *testing.T) {
	client := &handlerClient{memberPage: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Member"}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadMembers{RequestID: 42, ChatID: 9, Cursor: telegram.MemberCursor{Offset: 5, Limit: 25}})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	loaded, ok := events[0].(MembersLoaded)
	if !ok || loaded.RequestID != 42 || loaded.ChatID != 9 || len(loaded.Page.Members) != 1 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.memberChat != 9 || client.memberCursor != (telegram.MemberCursor{Offset: 5, Limit: 25}) {
		t.Fatalf("member call = %v %#v", client.memberChat, client.memberCursor)
	}

	client.err = errors.New("secret member detail")
	events = collectHandlerEvents(handler, LoadMembers{RequestID: 43, ChatID: 9})
	failed, ok := events[0].(MembersLoadFailed)
	if !ok || failed.RequestID != 43 || failed.ChatID != 9 || failed.Error.Message != "Could not load members" || failed.Error.Cause != nil {
		t.Fatalf("failure event = %#v", events[0])
	}
}
