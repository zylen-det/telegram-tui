package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func openDetailForTest(t *testing.T, userID domain.UserID) State {
	t.Helper()
	state := membersBaseState(domain.ChatSupergroup)
	updateState(&state, ActionReceived{Action: OpenMembers})
	updateState(&state, MembersLoaded{RequestID: state.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members: []domain.ChatMember{
			{User: domain.User{ID: 1, Name: "Ada Lovelace", Username: "ada"}, Role: domain.ChatMemberRoleOwner},
			{User: domain.User{ID: 2, Name: "Bob"}},
		},
		TotalCount: 2,
		NextOffset: 2,
		Done:       true,
	}})
	commands := updateState(&state, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: userID})
	if state.Members == nil || state.Members.Detail == nil {
		t.Fatalf("detail not opened for user %d", userID)
	}
	if len(commands) != 0 {
		t.Fatalf("open detail commands = %#v", commands)
	}
	return state
}

func TestMemberDetailOpensViaActivateAndBacksOutWithClose(t *testing.T) {
	state := membersBaseState(domain.ChatSupergroup)
	updateState(&state, ActionReceived{Action: OpenMembers})
	updateState(&state, MembersLoaded{RequestID: state.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Username: "ada"}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	updateState(&state, ActionReceived{Action: Activate})
	detail := state.Members.Detail
	if detail == nil || detail.UserID != 1 || detail.Name != "Ada" || detail.Username != "ada" || detail.Role != 0 {
		t.Fatalf("detail = %#v", detail)
	}
	if state.Focus != FocusMembers {
		t.Fatalf("focus = %v, want FocusMembers (single modal)", state.Focus)
	}
	// Close backs out to the list, keeping the modal open.
	updateState(&state, ActionReceived{Action: Close})
	if state.Members == nil || state.Members.Detail != nil {
		t.Fatalf("after close = %#v", state.Members)
	}
	if state.Focus != FocusMembers {
		t.Fatalf("focus after back = %v", state.Focus)
	}
	// Close from the list closes the modal.
	updateState(&state, ActionReceived{Action: Close})
	if state.Members != nil || state.Focus != FocusDetails {
		t.Fatalf("closed = members=%#v focus=%v", state.Members, state.Focus)
	}
}

func TestMemberDetailNavigationWrapsBackAndActions(t *testing.T) {
	// updateState mutates in place, so each scenario starts from its own
	// freshly opened detail.
	moved := openDetailForTest(t, 1)
	// Detail rows: Back + 5 actions.
	updateState(&moved, ActionReceived{Action: SelectPrevious})
	if moved.Members.Detail.Selected != 5 {
		t.Fatalf("wrap previous = %d, want 5", moved.Members.Detail.Selected)
	}
	updateState(&moved, ActionReceived{Action: SelectNext})
	if moved.Members.Detail.Selected != 0 {
		t.Fatalf("wrap next = %d, want 0", moved.Members.Detail.Selected)
	}
	// Activate on Back returns to the list.
	backed := openDetailForTest(t, 1)
	updateState(&backed, ActionReceived{Action: Activate})
	if backed.Members.Detail != nil {
		t.Fatalf("back activate = %#v", backed.Members.Detail)
	}
	// Explicit back action with wrong chat is ignored.
	ignored := openDetailForTest(t, 1)
	updateState(&ignored, ActionReceived{Action: CloseMemberDetail, ChatID: 8})
	if ignored.Members.Detail == nil {
		t.Fatal("wrong-chat back cleared the detail")
	}
}

func TestMemberDetailActionsOrder(t *testing.T) {
	full := MemberDetailActions(&MemberDetail{Username: "ada", Avatar: domain.AvatarRef{UniqueID: "a1"}})
	want := []Action{ViewMemberAvatar, CopyMemberUsername, AddMemberContact, RemoveMemberContact, BlockMember, UnblockMember}
	if !reflect.DeepEqual(full, want) {
		t.Fatalf("actions full = %v, want %v", full, want)
	}
	withUsername := MemberDetailActions(&MemberDetail{Username: "ada"})
	want = []Action{CopyMemberUsername, AddMemberContact, RemoveMemberContact, BlockMember, UnblockMember}
	if !reflect.DeepEqual(withUsername, want) {
		t.Fatalf("actions with username = %v, want %v", withUsername, want)
	}
	without := MemberDetailActions(&MemberDetail{})
	want = []Action{AddMemberContact, RemoveMemberContact, BlockMember, UnblockMember}
	if !reflect.DeepEqual(without, want) {
		t.Fatalf("actions without username = %v, want %v", without, want)
	}
	if MemberDetailActions(nil) != nil {
		t.Fatal("nil detail actions should be nil")
	}
}

func TestMemberDetailViewAvatarOpensModalOverMembers(t *testing.T) {
	ref := domain.AvatarRef{FileID: 11, UniqueID: "avatar-1"}
	state := membersBaseState(domain.ChatSupergroup)
	updateState(&state, ActionReceived{Action: OpenMembers})
	updateState(&state, MembersLoaded{RequestID: state.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Avatar: ref}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	updateState(&state, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	commands := updateState(&state, ActionReceived{Action: ViewMemberAvatar})
	if state.Modal == nil || state.Focus != FocusModal {
		t.Fatalf("modal = %#v focus = %v", state.Modal, state.Focus)
	}
	if state.Modal.PreviousFocus != FocusMembers || state.Modal.Title != "Ada" || state.Modal.Ref != ref {
		t.Fatalf("modal = %#v", state.Modal)
	}
	if state.Members == nil || state.Members.Detail == nil {
		t.Fatal("members detail should survive under the avatar modal")
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if _, ok := commands[0].(OpenAvatar); !ok {
		t.Fatalf("command = %#v, want OpenAvatar", commands[0])
	}
	// Closing the avatar modal returns to the member detail.
	updateState(&state, ActionReceived{Action: Close})
	if state.Modal != nil || state.Focus != FocusMembers || state.Members.Detail == nil {
		t.Fatalf("closed = modal=%#v focus=%v detail=%#v", state.Modal, state.Focus, state.Members.Detail)
	}
	// Avatarless members expose no view action.
	plain := openDetailForTest(t, 2)
	updateState(&plain, ActionReceived{Action: ViewMemberAvatar})
	if plain.Modal != nil {
		t.Fatalf("avatarless view opened modal: %#v", plain.Modal)
	}
}

func TestMemberDetailCopyContactBlockFlow(t *testing.T) {
	state := openDetailForTest(t, 1)
	// Activate on the Copy row (index 1) issues the copy command.
	updateState(&state, ActionReceived{Action: SelectNext})
	commands := updateState(&state, ActionReceived{Action: Activate})
	if len(commands) != 1 || !state.Members.Detail.Working {
		t.Fatalf("copy = working=%v commands=%#v", state.Members.Detail.Working, commands)
	}
	copyCommand, ok := commands[0].(CopyMemberUsernameCommand)
	if !ok || copyCommand.Text != "@ada" || copyCommand.UserID != 1 {
		t.Fatalf("copy command = %#v", commands[0])
	}
	// Navigation blocked while working.
	navCommands := updateState(&state, ActionReceived{Action: SelectNext})
	if len(navCommands) != 0 || state.Members.Detail.Selected != 1 {
		t.Fatalf("nav while working = %d %#v", state.Members.Detail.Selected, navCommands)
	}
	updateState(&state, MemberUsernameCopied{RequestID: copyCommand.RequestID, ChatID: 9, UserID: 1})
	if state.Members.Detail == nil || state.Members.Detail.Working || state.Toast == nil || state.Toast.Message != "Username copied" {
		t.Fatalf("copied = detail=%#v toast=%#v", state.Members.Detail, state.Toast)
	}
	// A stale result must not alter the completed operation.
	detail := *state.Members.Detail
	toast := *state.Toast
	staleCommands := updateState(&state, MemberUsernameCopied{RequestID: 999, ChatID: 9, UserID: 1})
	if len(staleCommands) != 0 || !reflect.DeepEqual(state.Members.Detail, &detail) || !reflect.DeepEqual(state.Toast, &toast) {
		t.Fatalf("stale copy result replaced completed operation: detail=%#v toast=%#v effects=%#v", state.Members.Detail, state.Toast, staleCommands)
	}

	adding := openDetailForTest(t, 1)
	commands = updateState(&adding, ActionReceived{Action: AddMemberContact})
	add, ok := commands[0].(AddMemberContactCommand)
	if !ok || add.FirstName != "Ada" || add.LastName != "Lovelace" {
		t.Fatalf("add command = %#v", commands[0])
	}
	updateState(&adding, MemberContactChanged{RequestID: add.RequestID, ChatID: 9, UserID: 1, Added: true})
	if adding.Members.Detail == nil || adding.Toast == nil || adding.Toast.Message != "Added to contacts" {
		t.Fatalf("added = detail=%#v toast=%#v", adding.Members.Detail, adding.Toast)
	}

	blocking := openDetailForTest(t, 2)
	commands = updateState(&blocking, ActionReceived{Action: BlockMember})
	block := commands[0].(SetMemberBlockedCommand)
	if !block.Blocked || block.UserID != 2 {
		t.Fatalf("block command = %#v", commands[0])
	}
	updateState(&blocking, MemberBlockChanged{RequestID: block.RequestID, ChatID: 9, UserID: 2, Blocked: true})
	if blocking.Members.Detail == nil || blocking.Toast.Message != "User blocked" {
		t.Fatalf("blocked = detail=%#v toast=%#v", blocking.Members.Detail, blocking.Toast)
	}

	unblocking := openDetailForTest(t, 2)
	commands = updateState(&unblocking, ActionReceived{Action: UnblockMember})
	unblock := commands[0].(SetMemberBlockedCommand)
	updateState(&unblocking, MemberBlockFailed{RequestID: unblock.RequestID, ChatID: 9, UserID: 2, Blocked: false, Error: domain.AppError{Message: "raw"}})
	if unblocking.Members.Detail == nil || unblocking.Toast == nil || unblocking.Toast.Message != "Could not unblock user" {
		t.Fatalf("unblock failure = detail=%#v toast=%#v", unblocking.Members.Detail, unblocking.Toast)
	}

	removing := openDetailForTest(t, 2)
	commands = updateState(&removing, ActionReceived{Action: RemoveMemberContact})
	remove := commands[0].(RemoveMemberContactCommand)
	updateState(&removing, MemberContactFailed{RequestID: remove.RequestID, ChatID: 9, UserID: 2, Added: false, Error: domain.AppError{Message: "x"}})
	if removing.Members.Detail == nil || removing.Toast.Message != "Could not remove contact" {
		t.Fatalf("remove failure = detail=%#v toast=%#v", removing.Members.Detail, removing.Toast)
	}
}

func TestMemberDetailAvatarRequestedOnceAndCached(t *testing.T) {
	ref := domain.AvatarRef{FileID: 11, UniqueID: "avatar-1"}
	state := membersBaseState(domain.ChatSupergroup)
	updateState(&state, ActionReceived{Action: OpenMembers})
	updateState(&state, MembersLoaded{RequestID: state.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Avatar: ref}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	commands := updateState(&state, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	if state.Members.Detail.AvatarKey != "avatar-1:chat-list" {
		t.Fatalf("avatar key = %q", state.Members.Detail.AvatarKey)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	render, ok := commands[0].(RenderAvatar)
	if !ok || render.Key != "avatar-1:chat-list" || render.Ref != ref {
		t.Fatalf("render command = %#v", commands[0])
	}
	// Reopening while cached issues no duplicate request.
	updateState(&state, ActionReceived{Action: CloseMemberDetail, ChatID: 9})
	commands = updateState(&state, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	if len(commands) != 0 {
		t.Fatalf("cached reopen commands = %#v", commands)
	}
	if state.Members.Detail == nil || state.Members.Detail.AvatarKey != "avatar-1:chat-list" {
		t.Fatalf("reopened detail = %#v", state.Members.Detail)
	}
	// Member without an avatar requests nothing.
	plain := membersBaseState(domain.ChatSupergroup)
	updateState(&plain, ActionReceived{Action: OpenMembers})
	updateState(&plain, MembersLoaded{RequestID: plain.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 2, Name: "Bob"}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	commands = updateState(&plain, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 2})
	if plain.Members.Detail.AvatarKey != "" || len(commands) != 0 {
		t.Fatalf("avatarless detail = key=%q commands=%#v", plain.Members.Detail.AvatarKey, commands)
	}
}

func TestSingleDetailEnterOnBackClosesModal(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 7}},
	}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, UserID: 7, PreviousFocus: FocusConversation}
	commands := updateState(&state, ActionReceived{Action: ViewUserInfo})
	load := commands[0].(LoadUserInfo)
	updateState(&state, UserInfoLoaded{RequestID: load.RequestID, ChatID: 9, UserID: 7, User: domain.User{ID: 7, Name: "Ada"}})
	detail := state.Members.Detail
	if detail == nil || detail.Selected != 0 {
		t.Fatalf("detail = %+v, want selection 0", detail)
	}
	// Keyboard Enter with the ‹ Back row selected must close the modal,
	// not fall back to an empty list shell.
	updateState(&state, ActionReceived{Action: Activate})
	if state.Members != nil || state.Focus != FocusConversation {
		t.Fatalf("back-enter = members=%#v focus=%v", state.Members, state.Focus)
	}
	// List mode keeps the old behavior: back returns to the member list.
	listed := openDetailForTest(t, 1)
	updateState(&listed, ActionReceived{Action: Activate})
	if listed.Members == nil || listed.Members.Detail != nil {
		t.Fatalf("list back-enter = %#v", listed.Members)
	}
}

func TestSplitMemberNameFallbacks(t *testing.T) {
	first, last := splitMemberName("Ada Lovelace", "ada")
	if first != "Ada" || last != "Lovelace" {
		t.Fatalf("split = %q %q", first, last)
	}
	first, last = splitMemberName("", "ada")
	if first != "ada" || last != "" {
		t.Fatalf("username fallback = %q %q", first, last)
	}
	first, last = splitMemberName("", "")
	if first != "Telegram" || last != "" {
		t.Fatalf("generic fallback = %q %q", first, last)
	}
}

func TestMessageMenuOffersUserInfoAfterCopyForUserSenders(t *testing.T) {
	caps := domain.MessageCapabilities{Reply: true, Forward: true, Edit: true, Copy: true, Pin: true, DeleteForSelf: true, DeleteForAll: true}
	menu := &MessageActionMenu{Capabilities: caps, CanReact: true, UserID: 7}
	if got := actionMenuItemCount(menu); got != 9 {
		t.Fatalf("actionMenuItemCount = %d, want 9", got)
	}
	want := []Action{ReplyMessage, ForwardMessageSource, EditMessage, CopyMessage, ViewUserInfo, ReactMessage, PinMessage, DeleteMessage, DeleteForEveryone}
	for index, action := range want {
		probe := &MessageActionMenu{Capabilities: caps, CanReact: true, UserID: 7, Selected: index}
		if got := selectedMenuAction(probe); got != action {
			t.Fatalf("selectedMenuAction at %d = %v, want %v", index, got, action)
		}
		selectMessageMenuAction(probe, action)
		if probe.Selected != index {
			t.Fatalf("selectMessageMenuAction(%v) = %d, want %d", action, probe.Selected, index)
		}
	}
	anonymous := &MessageActionMenu{Capabilities: caps, CanReact: true}
	if got := actionMenuItemCount(anonymous); got != 8 {
		t.Fatalf("count without sender = %d, want 8", got)
	}
}

func TestOpenMessageActionMenuCapturesUserSender(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9, Title: "Group"}}
		state.Messages[9] = []domain.Message{
			{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi", Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 7}, SenderName: "Ada"},
			{ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "ch", Sender: domain.SenderRef{Kind: domain.SenderChat, ID: 9}},
		}
		return state
	}
	// Each menu open starts from a fresh conversation without a menu.
	state := fixture()
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || state.MessageMenu.UserID != 7 {
		t.Fatalf("menu user = %#v", state.MessageMenu)
	}
	state = fixture()
	state.SelectedMessageChat, state.SelectedMessage = 9, 3
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || state.MessageMenu.UserID != 0 {
		t.Fatalf("chat sender menu user = %#v", state.MessageMenu)
	}
}

func TestViewUserInfoOpensSingleModalAndLoadsUser(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9, Title: "Group"}}
	state.Messages[9] = []domain.Message{
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi", Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 7}, SenderName: "Ada"},
	}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, UserID: 7, Capabilities: domain.MessageCapabilities{Copy: true}, PreviousFocus: FocusConversation}
	commands := updateState(&state, ActionReceived{Action: ViewUserInfo})
	if state.MessageMenu != nil || state.Members == nil || !state.Members.Single || state.Focus != FocusMembers {
		t.Fatalf("opened = menu=%#v members=%#v focus=%v", state.MessageMenu, state.Members, state.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	load, ok := commands[0].(LoadUserInfo)
	if !ok || load.ChatID != 9 || load.UserID != 7 {
		t.Fatalf("command = %#v", commands[0])
	}
	commands = updateState(&state, UserInfoLoaded{
		RequestID: load.RequestID, ChatID: 9, UserID: 7,
		User: domain.User{ID: 7, Name: "Ada", Username: "ada", Avatar: domain.AvatarRef{UniqueID: "u7"}},
	})
	detail := state.Members.Detail
	if detail == nil || detail.Name != "Ada" || detail.Username != "ada" || detail.AvatarKey != "u7:chat-list" {
		t.Fatalf("detail = %#v", detail)
	}
	if len(commands) != 1 {
		t.Fatalf("avatar commands = %#v", commands)
	}
	if _, ok := commands[0].(RenderAvatar); !ok {
		t.Fatalf("command = %#v, want RenderAvatar", commands[0])
	}
	// Stale load ignored: the current detail content survives.
	before := *detail
	staleCommands := updateState(&state, UserInfoLoaded{RequestID: 999, ChatID: 9, UserID: 7, User: domain.User{ID: 7}})
	if len(staleCommands) != 0 || !reflect.DeepEqual(state.Members.Detail, &before) {
		t.Fatalf("stale user info replaced current detail: %#v effects=%#v", state.Members.Detail, staleCommands)
	}
	// Esc in single mode closes the whole modal.
	updateState(&state, ActionReceived{Action: Close})
	if state.Members != nil || state.Focus != FocusConversation {
		t.Fatalf("closed = members=%#v focus=%v", state.Members, state.Focus)
	}
}

func TestUserInfoLoadFailedShowsErrorRow(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 7}},
	}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, UserID: 7, PreviousFocus: FocusConversation}
	commands := updateState(&state, ActionReceived{Action: ViewUserInfo})
	load := commands[0].(LoadUserInfo)
	updateState(&state, UserInfoLoadFailed{RequestID: load.RequestID, ChatID: 9, UserID: 7, Error: domain.AppError{Message: "raw"}})
	if state.Members.Error == nil || state.Members.Error.Message != "Could not load user info" || state.Members.Detail != nil {
		t.Fatalf("failed = %#v", state.Members)
	}
}
