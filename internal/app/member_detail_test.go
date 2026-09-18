package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/platform"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func openDetailForTest(t *testing.T, userID domain.UserID) State {
	t.Helper()
	state := membersBaseState(domain.ChatSupergroup)
	opened, _ := Reduce(state, ActionReceived{Action: OpenMembers})
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: opened.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members: []domain.ChatMember{
			{User: domain.User{ID: 1, Name: "Ada Lovelace", Username: "ada"}, Role: domain.ChatMemberRoleOwner},
			{User: domain.User{ID: 2, Name: "Bob"}},
		},
		TotalCount: 2,
		NextOffset: 2,
		Done:       true,
	}})
	detailed, commands := Reduce(loaded, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: userID})
	if detailed.Members == nil || detailed.Members.Detail == nil {
		t.Fatalf("detail not opened for user %d", userID)
	}
	if len(commands) != 0 {
		t.Fatalf("open detail commands = %#v", commands)
	}
	return detailed
}

func TestMemberDetailOpensViaActivateAndBacksOutWithClose(t *testing.T) {
	state := membersBaseState(domain.ChatSupergroup)
	opened, _ := Reduce(state, ActionReceived{Action: OpenMembers})
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: opened.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Username: "ada"}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	menuOpened, _ := Reduce(loaded, ActionReceived{Action: Activate})
	detail := menuOpened.Members.Detail
	if detail == nil || detail.UserID != 1 || detail.Name != "Ada" || detail.Username != "ada" || detail.Role != 0 {
		t.Fatalf("detail = %#v", detail)
	}
	if menuOpened.Focus != FocusMembers {
		t.Fatalf("focus = %v, want FocusMembers (single modal)", menuOpened.Focus)
	}
	// Close backs out to the list, keeping the modal open.
	backed, _ := Reduce(menuOpened, ActionReceived{Action: Close})
	if backed.Members == nil || backed.Members.Detail != nil {
		t.Fatalf("after close = %#v", backed.Members)
	}
	if backed.Focus != FocusMembers {
		t.Fatalf("focus after back = %v", backed.Focus)
	}
	// Close from the list closes the modal.
	closed, _ := Reduce(backed, ActionReceived{Action: Close})
	if closed.Members != nil || closed.Focus != FocusDetails {
		t.Fatalf("closed = members=%#v focus=%v", closed.Members, closed.Focus)
	}
}

func TestMemberDetailNavigationWrapsBackAndActions(t *testing.T) {
	state := openDetailForTest(t, 1)
	// Detail rows: Back + 5 actions.
	moved, _ := Reduce(state, ActionReceived{Action: SelectPrevious})
	if moved.Members.Detail.Selected != 5 {
		t.Fatalf("wrap previous = %d, want 5", moved.Members.Detail.Selected)
	}
	moved, _ = Reduce(moved, ActionReceived{Action: SelectNext})
	if moved.Members.Detail.Selected != 0 {
		t.Fatalf("wrap next = %d, want 0", moved.Members.Detail.Selected)
	}
	// Activate on Back returns to the list.
	backed, _ := Reduce(state, ActionReceived{Action: Activate})
	if backed.Members.Detail != nil {
		t.Fatalf("back activate = %#v", backed.Members.Detail)
	}
	// Explicit back action with wrong chat is ignored.
	ignored, _ := Reduce(state, ActionReceived{Action: CloseMemberDetail, ChatID: 8})
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
	opened, _ := Reduce(state, ActionReceived{Action: OpenMembers})
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: opened.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Avatar: ref}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	detailed, _ := Reduce(loaded, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	viewed, commands := Reduce(detailed, ActionReceived{Action: ViewMemberAvatar})
	if viewed.Modal == nil || viewed.Focus != FocusModal {
		t.Fatalf("modal = %#v focus = %v", viewed.Modal, viewed.Focus)
	}
	if viewed.Modal.PreviousFocus != FocusMembers || viewed.Modal.Title != "Ada" || viewed.Modal.Ref != ref {
		t.Fatalf("modal = %#v", viewed.Modal)
	}
	if viewed.Members == nil || viewed.Members.Detail == nil {
		t.Fatal("members detail should survive under the avatar modal")
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if _, ok := commands[0].(OpenAvatar); !ok {
		t.Fatalf("command = %#v, want OpenAvatar", commands[0])
	}
	// Closing the avatar modal returns to the member detail.
	closed, _ := Reduce(viewed, ActionReceived{Action: Close})
	if closed.Modal != nil || closed.Focus != FocusMembers || closed.Members.Detail == nil {
		t.Fatalf("closed = modal=%#v focus=%v detail=%#v", closed.Modal, closed.Focus, closed.Members.Detail)
	}
	// Avatarless members expose no view action.
	plain, _ := Reduce(openDetailForTest(t, 2), ActionReceived{Action: ViewMemberAvatar})
	if plain.Modal != nil {
		t.Fatalf("avatarless view opened modal: %#v", plain.Modal)
	}
}

func TestMemberDetailCopyContactBlockFlow(t *testing.T) {
	state := openDetailForTest(t, 1)
	// Activate on the Copy row (index 1) issues the copy command.
	withCopy, _ := Reduce(state, ActionReceived{Action: SelectNext})
	copied, commands := Reduce(withCopy, ActionReceived{Action: Activate})
	if len(commands) != 1 || !copied.Members.Detail.Working {
		t.Fatalf("copy = working=%v commands=%#v", copied.Members.Detail.Working, commands)
	}
	copyCommand, ok := commands[0].(CopyMemberUsernameCommand)
	if !ok || copyCommand.Text != "@ada" || copyCommand.UserID != 1 {
		t.Fatalf("copy command = %#v", commands[0])
	}
	// Navigation blocked while working.
	navigated, navCommands := Reduce(copied, ActionReceived{Action: SelectNext})
	if len(navCommands) != 0 || navigated.Members.Detail.Selected != 1 {
		t.Fatalf("nav while working = %d %#v", navigated.Members.Detail.Selected, navCommands)
	}
	done, _ := Reduce(copied, MemberUsernameCopied{RequestID: copyCommand.RequestID, ChatID: 9, UserID: 1})
	if done.Members.Detail == nil || done.Members.Detail.Working || done.Toast == nil || done.Toast.Message != "Username copied" {
		t.Fatalf("copied = detail=%#v toast=%#v", done.Members.Detail, done.Toast)
	}
	stale, staleCommands := Reduce(done, MemberUsernameCopied{RequestID: 999, ChatID: 9, UserID: 1})
	if len(staleCommands) != 0 || !reflect.DeepEqual(stale, done) {
		t.Fatal("stale copy result mutated state")
	}

	adding, commands := Reduce(openDetailForTest(t, 1), ActionReceived{Action: AddMemberContact})
	add, ok := commands[0].(AddMemberContactCommand)
	if !ok || add.FirstName != "Ada" || add.LastName != "Lovelace" {
		t.Fatalf("add command = %#v", commands[0])
	}
	_ = adding
	added, _ := Reduce(adding, MemberContactChanged{RequestID: add.RequestID, ChatID: 9, UserID: 1, Added: true})
	if added.Members.Detail == nil || added.Toast == nil || added.Toast.Message != "Added to contacts" {
		t.Fatalf("added = detail=%#v toast=%#v", added.Members.Detail, added.Toast)
	}

	blocking, commands := Reduce(openDetailForTest(t, 2), ActionReceived{Action: BlockMember})
	block := commands[0].(SetMemberBlockedCommand)
	if !block.Blocked || block.UserID != 2 {
		t.Fatalf("block command = %#v", commands[0])
	}
	blocked, _ := Reduce(blocking, MemberBlockChanged{RequestID: block.RequestID, ChatID: 9, UserID: 2, Blocked: true})
	if blocked.Members.Detail == nil || blocked.Toast.Message != "User blocked" {
		t.Fatalf("blocked = detail=%#v toast=%#v", blocked.Members.Detail, blocked.Toast)
	}

	unblocking, commands := Reduce(openDetailForTest(t, 2), ActionReceived{Action: UnblockMember})
	unblock := commands[0].(SetMemberBlockedCommand)
	failed, _ := Reduce(unblocking, MemberBlockFailed{RequestID: unblock.RequestID, ChatID: 9, UserID: 2, Blocked: false, Error: domain.AppError{Message: "raw"}})
	if failed.Members.Detail == nil || failed.Toast == nil || failed.Toast.Message != "Could not unblock user" {
		t.Fatalf("unblock failure = detail=%#v toast=%#v", failed.Members.Detail, failed.Toast)
	}

	removing, commands := Reduce(openDetailForTest(t, 2), ActionReceived{Action: RemoveMemberContact})
	remove := commands[0].(RemoveMemberContactCommand)
	removeFailed, _ := Reduce(removing, MemberContactFailed{RequestID: remove.RequestID, ChatID: 9, UserID: 2, Added: false, Error: domain.AppError{Message: "x"}})
	if removeFailed.Members.Detail == nil || removeFailed.Toast.Message != "Could not remove contact" {
		t.Fatalf("remove failure = detail=%#v toast=%#v", removeFailed.Members.Detail, removeFailed.Toast)
	}
}

func TestMemberDetailAvatarRequestedOnceAndCached(t *testing.T) {
	ref := domain.AvatarRef{FileID: 11, UniqueID: "avatar-1"}
	state := membersBaseState(domain.ChatSupergroup)
	opened, _ := Reduce(state, ActionReceived{Action: OpenMembers})
	loaded, _ := Reduce(opened, MembersLoaded{RequestID: opened.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 1, Name: "Ada", Avatar: ref}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	detailed, commands := Reduce(loaded, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	if detailed.Members.Detail.AvatarKey != "avatar-1:chat-list" {
		t.Fatalf("avatar key = %q", detailed.Members.Detail.AvatarKey)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands = %#v", commands)
	}
	render, ok := commands[0].(RenderAvatar)
	if !ok || render.Key != "avatar-1:chat-list" || render.Ref != ref {
		t.Fatalf("render command = %#v", commands[0])
	}
	// Reopening while cached issues no duplicate request.
	again, commands := Reduce(detailed, ActionReceived{Action: CloseMemberDetail, ChatID: 9})
	reopened, commands := Reduce(again, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 1})
	if len(commands) != 0 {
		t.Fatalf("cached reopen commands = %#v", commands)
	}
	_ = reopened
	// Member without an avatar requests nothing.
	plain, _ := Reduce(membersBaseState(domain.ChatSupergroup), ActionReceived{Action: OpenMembers})
	plainLoaded, _ := Reduce(plain, MembersLoaded{RequestID: plain.Members.RequestID, ChatID: 9, Page: telegram.MemberPage{
		Members:    []domain.ChatMember{{User: domain.User{ID: 2, Name: "Bob"}}},
		TotalCount: 1,
		NextOffset: 1,
		Done:       true,
	}})
	noAvatar, commands := Reduce(plainLoaded, ActionReceived{Action: OpenMemberDetail, ChatID: 9, UserID: 2})
	if noAvatar.Members.Detail.AvatarKey != "" || len(commands) != 0 {
		t.Fatalf("avatarless detail = key=%q commands=%#v", noAvatar.Members.Detail.AvatarKey, commands)
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
	opened, commands := Reduce(state, ActionReceived{Action: ViewUserInfo})
	load := commands[0].(LoadUserInfo)
	loaded, _ := Reduce(opened, UserInfoLoaded{RequestID: load.RequestID, ChatID: 9, UserID: 7, User: domain.User{ID: 7, Name: "Ada"}})
	// Keyboard Enter with the ‹ Back row selected must close the modal,
	// not fall back to an empty list shell.
	if loaded.Members.Detail.Selected != 0 {
		t.Fatalf("detail selection = %d, want 0", loaded.Members.Detail.Selected)
	}
	closed, _ := Reduce(loaded, ActionReceived{Action: Activate})
	if closed.Members != nil || closed.Focus != FocusConversation {
		t.Fatalf("back-enter = members=%#v focus=%v", closed.Members, closed.Focus)
	}
	// List mode keeps the old behavior: back returns to the member list.
	listed := openDetailForTest(t, 1)
	backed, _ := Reduce(listed, ActionReceived{Action: Activate})
	if backed.Members == nil || backed.Members.Detail != nil {
		t.Fatalf("list back-enter = %#v", backed.Members)
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

func TestMemberDetailCloneDoesNotAlias(t *testing.T) {
	state := openDetailForTest(t, 1)
	cloned := cloneReducerState(state)
	cloned.Members.Detail.Selected = 4
	cloned.Members.Detail.Name = "mutated"
	if state.Members.Detail.Selected != 0 || state.Members.Detail.Name != "Ada Lovelace" {
		t.Fatal("detail clone aliases state")
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
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9, Title: "Group"}}
	state.Messages[9] = []domain.Message{
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi", Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 7}, SenderName: "Ada"},
		{ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "ch", Sender: domain.SenderRef{Kind: domain.SenderChat, ID: 9}},
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	opened, _ := Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
	if opened.MessageMenu == nil || opened.MessageMenu.UserID != 7 {
		t.Fatalf("menu user = %#v", opened.MessageMenu)
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 3
	opened, _ = Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
	if opened.MessageMenu == nil || opened.MessageMenu.UserID != 0 {
		t.Fatalf("chat sender menu user = %#v", opened.MessageMenu)
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
	opened, commands := Reduce(state, ActionReceived{Action: ViewUserInfo})
	if opened.MessageMenu != nil || opened.Members == nil || !opened.Members.Single || opened.Focus != FocusMembers {
		t.Fatalf("opened = menu=%#v members=%#v focus=%v", opened.MessageMenu, opened.Members, opened.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	load, ok := commands[0].(LoadUserInfo)
	if !ok || load.ChatID != 9 || load.UserID != 7 {
		t.Fatalf("command = %#v", commands[0])
	}
	loaded, commands := Reduce(opened, UserInfoLoaded{
		RequestID: load.RequestID, ChatID: 9, UserID: 7,
		User: domain.User{ID: 7, Name: "Ada", Username: "ada", Avatar: domain.AvatarRef{UniqueID: "u7"}},
	})
	detail := loaded.Members.Detail
	if detail == nil || detail.Name != "Ada" || detail.Username != "ada" || detail.AvatarKey != "u7:chat-list" {
		t.Fatalf("detail = %#v", detail)
	}
	if len(commands) != 1 {
		t.Fatalf("avatar commands = %#v", commands)
	}
	if _, ok := commands[0].(RenderAvatar); !ok {
		t.Fatalf("command = %#v, want RenderAvatar", commands[0])
	}
	// Stale load ignored.
	stale, staleCommands := Reduce(loaded, UserInfoLoaded{RequestID: 999, ChatID: 9, UserID: 7, User: domain.User{ID: 7}})
	if len(staleCommands) != 0 || !reflect.DeepEqual(stale, loaded) {
		t.Fatal("stale user info mutated state")
	}
	// Esc in single mode closes the whole modal.
	closed, _ := Reduce(loaded, ActionReceived{Action: Close})
	if closed.Members != nil || closed.Focus != FocusConversation {
		t.Fatalf("closed = members=%#v focus=%v", closed.Members, closed.Focus)
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
	opened, commands := Reduce(state, ActionReceived{Action: ViewUserInfo})
	load := commands[0].(LoadUserInfo)
	failed, _ := Reduce(opened, UserInfoLoadFailed{RequestID: load.RequestID, ChatID: 9, UserID: 7, Error: domain.AppError{Message: "raw"}})
	if failed.Members.Error == nil || failed.Members.Error.Message != "Could not load user info" || failed.Members.Detail != nil {
		t.Fatalf("failed = %#v", failed.Members)
	}
}

func TestHandlerLoadUserInfoExactAndSafeFailure(t *testing.T) {
	client := &handlerClient{loadUser: domain.User{ID: 7, Name: "Ada"}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadUserInfo{RequestID: 46, ChatID: 9, UserID: 7})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	loaded, ok := events[0].(UserInfoLoaded)
	if !ok || loaded.RequestID != 46 || loaded.User.ID != 7 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.loadUserID != 7 {
		t.Fatalf("load user call = %v", client.loadUserID)
	}
	client.err = errors.New("secret user detail")
	events = collectHandlerEvents(handler, LoadUserInfo{RequestID: 47, ChatID: 9, UserID: 7})
	failed, ok := events[0].(UserInfoLoadFailed)
	if !ok || failed.Error.Message != "Could not load user info" || failed.Error.Cause != nil {
		t.Fatalf("failure event = %#v", events[0])
	}
}

func TestHandlerMemberCopyContactBlockExactAndSafeFailure(t *testing.T) {
	clipboard := &platform.FakeClipboard{}
	client := &handlerClient{}
	handler := NewHandler(context.Background(), &handlerResolver{}, func(_ config.Runtime, _ auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, auth.NewBroker(), &handlerAvatarRenderer{}, clipboard)
	handler.client = client
	handler.startOnce.Do(func() { close(handler.startBegan) })

	events := collectHandlerEvents(handler, CopyMemberUsernameCommand{RequestID: 41, ChatID: 9, UserID: 7, Text: "@ada"})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	copied, ok := events[0].(MemberUsernameCopied)
	if !ok || copied.RequestID != 41 || copied.UserID != 7 {
		t.Fatalf("event = %#v", events[0])
	}
	if clipboard.Writes != 1 {
		t.Fatalf("clipboard writes = %d, want 1", clipboard.Writes)
	}

	events = collectHandlerEvents(handler, AddMemberContactCommand{RequestID: 42, ChatID: 9, UserID: 7, FirstName: "Ada", LastName: "L"})
	changed, ok := events[0].(MemberContactChanged)
	if !ok || changed.RequestID != 42 || !changed.Added {
		t.Fatalf("event = %#v", events[0])
	}
	if client.contactRequest != (telegram.AddContactRequest{UserID: 7, FirstName: "Ada", LastName: "L"}) {
		t.Fatalf("contact call = %#v", client.contactRequest)
	}

	events = collectHandlerEvents(handler, SetMemberBlockedCommand{RequestID: 43, ChatID: 9, UserID: 7, Blocked: true})
	blocked, ok := events[0].(MemberBlockChanged)
	if !ok || blocked.RequestID != 43 || !blocked.Blocked {
		t.Fatalf("event = %#v", events[0])
	}

	clipboard.Err = errors.New("raw clipboard detail")
	events = collectHandlerEvents(handler, CopyMemberUsernameCommand{RequestID: 44, ChatID: 9, UserID: 7, Text: "@ada"})
	copyFailed, ok := events[0].(MemberUsernameCopyFailed)
	if !ok || copyFailed.Error.Message != "Could not copy username" || copyFailed.Error.Cause != nil {
		t.Fatalf("failure event = %#v", events[0])
	}
	client.err = errors.New("secret block detail")
	events = collectHandlerEvents(handler, SetMemberBlockedCommand{RequestID: 45, ChatID: 9, UserID: 7, Blocked: false})
	blockFailed, ok := events[0].(MemberBlockFailed)
	if !ok || blockFailed.Blocked || blockFailed.Error.Message != "Could not unblock user" || blockFailed.Error.Cause != nil {
		t.Fatalf("failure event = %#v", events[0])
	}
}
