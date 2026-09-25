package frontend

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestInitialStateOwnsReducerIDsAndCollections(t *testing.T) {
	state := InitialState()
	if state.NextRequestID != 1 || state.NextLocalID != -1 {
		t.Fatalf("IDs = (%d, %d), want (1, -1)", state.NextRequestID, state.NextLocalID)
	}
	if state.History == nil || state.Messages == nil || state.Drafts == nil || state.Avatars == nil {
		t.Fatalf("collections not initialized: %#v", state)
	}
}

func TestMessageSelectionUsesChatScopedIdentityAndSurvivesMatchingUpdate(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}, {ID: 10}}
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9}, {ID: 2, ChatID: 9}}
	state.Messages[10] = []domain.Message{{ID: 2, ChatID: 10}}
	updateState(&state, ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 2})
	if state.SelectedMessageChat != 9 || state.SelectedMessage != 2 {
		t.Fatalf("selection identity = (%d, %d), want (9, 2)", state.SelectedMessageChat, state.SelectedMessage)
	}
	updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 2, ChatID: 9}}})
	if state.SelectedMessageChat != 9 || state.SelectedMessage != 2 {
		t.Fatal("matching update cleared selection")
	}
	updateState(&state, ActionReceived{Action: SelectChat, ChatID: 10})
	if state.SelectedMessageChat != 10 || state.SelectedMessage != 2 {
		t.Fatalf("chat change selection identity = (%d, %d)", state.SelectedMessageChat, state.SelectedMessage)
	}
}

func TestMessageActionMenuIsCapabilityAwareAndCopyCommandIsContentOpaqueToState(t *testing.T) {
	openMenu := func() State {
		menu := InitialState()
		menu.Focus = FocusConversation
		menu.Chats = []domain.Chat{{ID: 9}}
		menu.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "private"}}
		menu.SelectedMessageChat, menu.SelectedMessage = 9, 2
		updateState(&menu, ActionReceived{Action: OpenMessageActionMenu})
		return menu
	}
	state := openMenu()
	if state.MessageMenu == nil || state.MessageMenu.ChatID != 9 || state.MessageMenu.MessageID != 2 || !state.MessageMenu.Capabilities.Copy {
		t.Fatalf("menu metadata = %#v", state.MessageMenu)
	}
	commands := updateState(&state, ActionReceived{Action: CopyMessage})
	if state.MessageMenu != nil || len(commands) != 1 {
		t.Fatalf("copy transition = menu:%#v command-count:%d", state.MessageMenu, len(commands))
	}
	if command, ok := commands[0].(WriteClipboard); !ok || command.Text != "private" {
		t.Fatalf("copy command = %#v", commands[0])
	}
	state = openMenu()
	commands = updateState(&state, ActionReceived{Action: Close})
	if state.MessageMenu != nil || state.Focus != FocusConversation || len(commands) != 0 {
		t.Fatalf("close = focus:%v menu:%#v effects:%#v", state.Focus, state.MessageMenu, commands)
	}
}

func TestMessageSelectionClearsWhenSelectedMessageDisappears(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2}
	updateState(&state, MessagesLoaded{ChatID: 9, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 1, ChatID: 9}}, Done: true}})
	if state.SelectedMessage != 0 || state.SelectedMessageChat != 0 || state.MessageMenu != nil {
		t.Fatal("missing selected message was retained")
	}
}

func TestMessageKeyboardNavigationAndMenuIdentityRemainAtomic(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "first"}, {ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "second"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 1

	updateState(&state, ActionReceived{Action: SelectNextMessage})
	if state.SelectedChat != 0 || state.SelectedMessage != 2 || state.SelectedMessageChat != 9 {
		t.Fatal("message navigation changed wrong identity")
	}
	updateState(&state, ActionReceived{Action: SelectPreviousMessage})
	if state.SelectedMessage != 1 {
		t.Fatal("previous message navigation failed")
	}

	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || state.MessageMenu.MessageID != 1 {
		t.Fatalf("menu target = %#v", state.MessageMenu)
	}
	state.SelectedMessage = 2 // menu target must survive a later selection change
	commands := updateState(&state, ActionReceived{Action: CopyMessage})
	if len(commands) != 1 {
		t.Fatal("menu identity did not produce copy command")
	}
	command, ok := commands[0].(WriteClipboard)
	if !ok || command.Text != "first" {
		t.Fatalf("copy did not use menu-owned message: %#v", commands[0])
	}
}

func TestMessageNavigationViewportFollowsSelectionAndStopsAtEndpoints(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9},
		{ID: 2, ChatID: 9},
		{ID: 3, ChatID: 9},
		{ID: 4, ChatID: 9},
		{ID: 5, ChatID: 9},
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 5

	newest := state
	newest.History = map[domain.ChatID]HistoryState{9: {}}
	commands := updateState(&state, ActionReceived{Action: SelectPreviousMessage})
	if len(commands) != 0 || state.SelectedMessage != 4 || state.History[9].ViewOffset != 1 || !state.History[9].FollowSelection {
		t.Fatalf("first older navigation = selected:%d offset:%d commands:%#v", state.SelectedMessage, state.History[9].ViewOffset, commands)
	}
	commands = updateState(&state, ActionReceived{Action: SelectPreviousMessage})
	if len(commands) != 0 || state.SelectedMessage != 3 || state.History[9].ViewOffset != 2 || !state.History[9].FollowSelection {
		t.Fatalf("second older navigation = selected:%d offset:%d commands:%#v", state.SelectedMessage, state.History[9].ViewOffset, commands)
	}
	commands = updateState(&state, ActionReceived{Action: SelectNextMessage})
	if len(commands) != 0 || state.SelectedMessage != 4 || state.History[9].ViewOffset != 1 || !state.History[9].FollowSelection {
		t.Fatalf("newer navigation = selected:%d offset:%d commands:%#v", state.SelectedMessage, state.History[9].ViewOffset, commands)
	}
	commands = updateState(&state, ActionReceived{Action: PageDown})
	if len(commands) != 0 || state.History[9].FollowSelection {
		t.Fatalf("manual pagination retained selection follow: %#v %#v", state.History[9], commands)
	}

	oldest := newest
	oldest.SelectedMessage = 1
	oldest.History = map[domain.ChatID]HistoryState{9: {ViewOffset: 4}}
	commands = updateState(&oldest, ActionReceived{Action: SelectPreviousMessage})
	if len(commands) != 0 || oldest.SelectedMessage != 1 || oldest.History[9].ViewOffset != 4 {
		t.Fatalf("oldest endpoint = selected:%d offset:%d commands:%#v", oldest.SelectedMessage, oldest.History[9].ViewOffset, commands)
	}

	commands = updateState(&newest, ActionReceived{Action: SelectNextMessage})
	if len(commands) != 0 || newest.SelectedMessage != 5 || newest.History[9].ViewOffset != 0 {
		t.Fatalf("newest endpoint = selected:%d offset:%d commands:%#v", newest.SelectedMessage, newest.History[9].ViewOffset, commands)
	}
}

func TestAuthoritativeMessageActionsLoadAndRejectStaleResults(t *testing.T) {
	base := func() State {
		state := InitialState()
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "private"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		return state
	}
	state := base()
	commands := updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || !state.MessageMenu.Loading || !state.MessageMenu.Capabilities.Copy || state.MessageMenu.Capabilities.Reply || len(commands) != 1 {
		t.Fatalf("opened = %#v commands=%#v", state.MessageMenu, commands)
	}
	request, ok := commands[0].(GetMessageProperties)
	if !ok || request.ChatID != 9 || request.MessageID != 2 || request.RequestID != state.MessageMenu.RequestID {
		t.Fatalf("request = %#v", commands[0])
	}
	updateState(&state, MessagePropertiesLoaded{RequestID: request.RequestID, ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true, Edit: true}})
	if state.MessageMenu == nil || state.MessageMenu.Loading || !state.MessageMenu.Capabilities.Reply || !state.MessageMenu.Capabilities.Edit {
		t.Fatalf("loaded = %#v", state.MessageMenu)
	}
	updateState(&state, MessagePropertiesLoaded{RequestID: request.RequestID + 1, ChatID: 9, MessageID: 2})
	if state.MessageMenu == nil || !state.MessageMenu.Capabilities.Edit {
		t.Fatal("stale result replaced snapshot")
	}

	closed := base()
	updateState(&closed, ActionReceived{Action: OpenMessageActionMenu})
	if closed.MessageMenu == nil || !closed.MessageMenu.Loading {
		t.Fatal("close branch did not start with a loading menu")
	}
	updateState(&closed, ActionReceived{Action: Close})
	updateState(&closed, MessagePropertiesLoaded{RequestID: request.RequestID, ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Edit: true}})
	if closed.MessageMenu != nil {
		t.Fatal("closed modal accepted result")
	}
}

func TestAuthoritativeMessageActionsFailureIsSafe(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	commands := updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	opened := state
	request := commands[0].(GetMessageProperties)
	updateState(&opened, MessagePropertiesLoadFailed{RequestID: request.RequestID, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "raw secret-body 9 2"}})
	failed := opened
	if failed.MessageMenu == nil || failed.MessageMenu.Loading || !failed.MessageMenu.Capabilities.Copy || failed.MessageMenu.Capabilities.Reply || failed.MessageMenu.Capabilities.Edit || failed.MessageMenu.Error == nil || failed.MessageMenu.Error.Message != "Could not load message actions" {
		t.Fatalf("failed = %#v", failed.MessageMenu)
	}
}

func TestAuthoritativeMessageActionsCloseWhenTargetIsEvicted(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = make([]domain.Message, 0, maxMessagesPerChat)
	for index := 1; index <= maxMessagesPerChat; index++ {
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: domain.MessageID(index), ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(int64(index), 0)})
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 1
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	opened := state
	if opened.MessageMenu == nil || !opened.MessageMenu.Loading {
		t.Fatal("authoritative action menu did not open")
	}
	updateState(&opened, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(999, 0)}}})
	evicted := opened
	if evicted.MessageMenu != nil || evicted.SelectedMessage == 1 {
		t.Fatal("evicted target retained action-menu identity")
	}
}

func TestMessageMenuTracksOptimisticIdentityReplacement(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: -1, ChatID: 9, Kind: domain.MessageText, Outgoing: true, SendState: domain.SendPending}}
	state.SelectedMessageChat, state.SelectedMessage = 9, -1
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: -1, Capabilities: state.Messages[9][0].Capabilities()}

	updateState(&state, TextQueued{LocalID: -1, Message: domain.Message{ID: -2, ChatID: 9, Kind: domain.MessageText}})
	if state.SelectedMessage != -2 || state.MessageMenu == nil || state.MessageMenu.MessageID != -2 {
		t.Fatal("queued replacement left stale menu identity")
	}
	updateState(&state, TelegramEvent{Value: telegram.MessageSendSucceeded{OldID: -2, Message: domain.Message{ID: 100, ChatID: 9, Kind: domain.MessageText}}})
	if state.SelectedMessage != 100 || state.MessageMenu == nil || state.MessageMenu.MessageID != 100 {
		t.Fatal("durable replacement left stale menu identity")
	}
}

func TestCopyMessageResultUsesOnlyConstantSafeToast(t *testing.T) {
	state := InitialState()
	updateState(&state, ClipboardWritten{})
	if state.Toast == nil || state.Toast.Message != "Message copied" {
		t.Fatalf("success toast = %#v", state.Toast)
	}
	failure := domain.AppError{Kind: domain.ErrorInternal, Message: "Could not copy message", Cause: errors.New("private raw cause")}
	updateState(&state, ClipboardWriteFailed{Error: failure})
	if state.Toast == nil || state.Toast.Message != "Could not copy message" {
		t.Fatalf("failure toast = %#v", state.Toast)
	}
}

func TestToastLifecycleReplacesAndRejectsStaleExpiry(t *testing.T) {
	state := InitialState()
	updateState(&state, ClipboardWritten{})
	if state.Toast == nil || state.ToastGeneration == 0 || state.ToastDuration != 2*time.Second {
		t.Fatalf("success toast = %#v", state.Toast)
	}
	firstGeneration := state.ToastGeneration
	updateState(&state, OperationFailed{Error: domain.AppError{Kind: domain.ErrorNetwork, Message: "Try again"}})
	if state.Toast == nil || state.ToastGeneration == firstGeneration || state.ToastDuration != 4*time.Second {
		t.Fatalf("replacement toast = %#v", state.Toast)
	}
	secondGeneration := state.ToastGeneration
	updateState(&state, ToastExpired{Generation: firstGeneration})
	if state.Toast == nil || state.Toast.Message != "Try again" || state.ToastGeneration != secondGeneration {
		t.Fatal("stale expiry changed replacement toast")
	}
	updateState(&state, ToastExpired{Generation: secondGeneration})
	if state.Toast != nil {
		t.Fatalf("current expiry retained toast = %#v", state.Toast)
	}
}

func TestActionModalListNavigationAndActivation(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "private"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || state.Focus != FocusModal || state.MessageMenu.Selected != 0 {
		t.Fatalf("opened action modal = focus:%v menu:%#v", state.Focus, state.MessageMenu)
	}
	updateState(&state, MessagePropertiesLoaded{RequestID: state.MessageMenu.RequestID, ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Reply: true, Copy: true}})
	if state.MessageMenu.Selected != 1 {
		t.Fatal("authoritative actions did not preserve selected Copy action")
	}
	updateState(&state, ActionReceived{Action: SelectPrevious})
	if state.MessageMenu.Selected != 0 {
		t.Fatalf("two-row navigation selected %d", state.MessageMenu.Selected)
	}
	commands := updateState(&state, ActionReceived{Action: Activate})
	if state.MessageMenu != nil || state.Focus != FocusComposer || state.ReplyTarget == nil || len(commands) != 1 {
		t.Fatalf("reply activation = focus:%v menu-present:%t reply-present:%t commands:%d", state.Focus, state.MessageMenu != nil, state.ReplyTarget != nil, len(commands))
	}
}

func TestReplyTargetClearsWhenTargetDisappears(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = make([]domain.Message, 0, maxMessagesPerChat)
	state.Messages[9] = append(state.Messages[9], domain.Message{ID: 22, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(1, 0)})
	for index := 1; index < maxMessagesPerChat; index++ {
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: domain.MessageID(22 + index), ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(int64(index+1), 0)})
	}
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 22}
	updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(999, 0)}}})
	got := state
	if got.ReplyTarget != nil {
		t.Fatal("disappeared reply target was retained")
	}
}

func TestReplyTargetLifecycleAndSendReply(t *testing.T) {
	// updateState mutates in place, so each branch rebuilds its fixture.
	replyingFixture := func() State {
		state := selectedWritableState()
		state.Focus = FocusConversation
		state.Drafts[9] = "draft"
		state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, SenderName: "Sender", Text: "opaque"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 22
		updateState(&state, ActionReceived{Action: ReplyMessage})
		return state
	}
	replying := replyingFixture()
	if replying.ReplyTarget == nil || replying.ReplyTarget.ChatID != 9 || replying.ReplyTarget.MessageID != 22 || replying.Focus != FocusComposer || replying.Drafts[9] != "draft" {
		t.Fatal("reply target was not established atomically")
	}
	cancelled := replyingFixture()
	updateState(&cancelled, ActionReceived{Action: Close})
	if cancelled.ReplyTarget != nil || cancelled.Focus != FocusComposer || cancelled.Drafts[9] != "draft" {
		t.Fatal("first composer escape did not cancel reply only")
	}

	submitted := replyingFixture()
	commands := updateState(&submitted, ActionReceived{Action: ComposerSubmit, At: time.Unix(1, 0)})
	if submitted.ReplyTarget != nil || len(commands) != 2 || submitted.Messages[9][1].ReplyToMessageID != 22 {
		t.Fatal("reply submit did not persist identity")
	}
	if commands[0].(SendText).ReplyToMessageID != 22 {
		t.Fatal("reply command lost target identity")
	}
}

func TestComposerEscapeCancelsReplyThenDraftThenLeaves(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer
	state.Drafts[9] = "draft"
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 22}

	updateState(&state, ActionReceived{Action: Close})
	if state.ReplyTarget != nil || state.Drafts[9] != "draft" || state.Focus != FocusComposer {
		t.Fatal("first escape did not cancel reply only")
	}
	updateState(&state, ActionReceived{Action: Close})
	if state.Drafts[9] != "" || state.Focus != FocusComposer {
		t.Fatal("second escape did not cancel draft only")
	}
	updateState(&state, ActionReceived{Action: Close})
	if state.Focus != FocusConversation {
		t.Fatal("third escape did not leave empty composer")
	}
}

func TestReadyRefreshesChatsWithoutDiscardingCache(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 10
	state.Chats = []domain.Chat{{ID: 7, Title: "cached"}}
	state.Messages[7] = []domain.Message{{ID: 11, ChatID: 7, Text: "cached message"}}

	commands := updateState(&state, TelegramEvent{Value: telegram.Ready{}, ReceivedAt: time.Unix(1, 0)})
	got := state
	if !got.ChatsLoading || got.ChatRequestID != 10 || got.NextRequestID != 11 {
		t.Fatalf("chat request state = %#v", got)
	}
	if got.Chats[0].Title != "cached" || got.Messages[7][0].Text != "cached message" {
		t.Fatal("Ready discarded cached content")
	}
	assertCommands(t, commands, []Effect{LoadChats{RequestID: 10, Cursor: telegram.ChatCursor{Limit: 50}}})
}

func TestChatsLoadedSortsPreservesSelectionAndStartsHistoryAndAvatars(t *testing.T) {
	state := InitialState()
	state.ChatRequestID = 4
	state.NextRequestID = 5
	state.ChatsLoading = true
	state.Chats = []domain.Chat{{ID: 7}, {ID: 9}}
	state.SelectedChat = 0

	page := telegram.ChatPage{Done: true, Chats: []domain.Chat{
		{ID: 7, Title: "Mina", Order: 20, Avatar: domain.AvatarRef{UniqueID: "mina"}},
		{ID: 9, Title: "Team", Order: 20, Avatar: domain.AvatarRef{UniqueID: "team"}},
		{ID: 3, Title: "Old", Order: 10},
	}}
	commands := updateState(&state, ChatsLoaded{RequestID: 4, Page: page})
	got := state
	if got.ChatsLoading || !got.ChatsLoaded || got.ChatsError != nil {
		t.Fatalf("chat load indicators = %#v", got)
	}
	if ids := chatIDs(got.Chats); !reflect.DeepEqual(ids, []domain.ChatID{9, 7, 3}) {
		t.Fatalf("chat order = %#v", ids)
	}
	if got.Chats[got.SelectedChat].ID != 7 {
		t.Fatalf("selected chat = %#v", got.Chats[got.SelectedChat])
	}
	history := got.History[7]
	if !history.Loading || history.RequestID != 5 || got.NextRequestID != 6 {
		t.Fatalf("history request = %#v, next = %d", history, got.NextRequestID)
	}
	assertCommands(t, commands, []Effect{
		LoadMessages{RequestID: 5, ChatID: 7, Cursor: telegram.MessageCursor{Limit: 50}},
		RenderAvatar{Key: "mina:chat-list", Ref: page.Chats[0].Avatar, Role: avatar.RoleChatList},
		RenderAvatar{Key: "team:chat-list", Ref: page.Chats[1].Avatar, Role: avatar.RoleChatList},
	})
}

func TestStaleOldChatMessagesFillCacheWithoutActiveSideEffects(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 1}, {ID: 2}}
	state.SelectedChat = 1
	state.SelectedMessage = 22
	state.Focus = FocusComposer
	state.History[1] = HistoryState{Loading: true, RequestID: 11}
	state.History[2] = HistoryState{Loading: true, RequestID: 12, ViewOffset: 7}
	activeBefore := state.History[2]

	commands := updateState(&state, MessagesLoaded{
		RequestID: 11,
		ChatID:    1,
		Page: telegram.MessagePage{Messages: []domain.Message{{
			ID: 10, ChatID: 1, Text: "old", SenderAvatar: domain.AvatarRef{UniqueID: "old-sender"},
		}}},
	})
	got := state
	if got.Messages[1][0].Text != "old" {
		t.Fatalf("old chat cache = %#v", got.Messages[1])
	}
	if got.SelectedChat != 1 || got.SelectedMessage != 22 || got.Focus != FocusComposer || got.History[2] != activeBefore {
		t.Fatalf("active view changed: %#v", got)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "old failure"}
	commands = updateState(&state, MessagesLoadFailed{RequestID: 11, ChatID: 1, Error: failure})
	failed := state
	if failed.Toast != nil || failed.History[2] != activeBefore || len(commands) != 0 {
		t.Fatalf("stale failure changed active indicators = (%#v, %#v)", failed, commands)
	}
}

func TestSelectingCachedOldChatRequestsItsMissingSenderAvatars(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 1}, {ID: 2}}
	state.SelectedChat = 1
	state.History[1] = HistoryState{Done: true}
	ref := domain.AvatarRef{UniqueID: "old-sender"}
	state.Messages[1] = []domain.Message{{ID: 10, ChatID: 1, SenderAvatar: ref}}

	commands := updateState(&state, ActionReceived{Action: SelectChat, ChatID: 1})
	got := state
	if got.SelectedChat != 0 || !got.Avatars["old-sender:message-group"].Loading {
		t.Fatalf("selected cached chat = %#v", got)
	}
	assertCommands(t, commands, []Effect{CloseChatCommand{ChatID: 2}, OpenChatCommand{ChatID: 1}, RenderAvatar{Key: "old-sender:message-group", Ref: ref, Role: avatar.RoleMessageGroup}})
}

func TestOptimisticSendQueueFailureAndCooldownRetry(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	state := selectedWritableState()
	state.NextRequestID = 7
	state.Drafts[9] = "hello"

	commands := updateState(&state, ActionReceived{Action: ComposerSubmit, At: now})
	submitted := state
	if submitted.Drafts[9] != "" || submitted.NextLocalID != -2 || len(submitted.Messages[9]) != 1 {
		t.Fatalf("submitted state = %#v", submitted)
	}
	pending := submitted.Messages[9][0]
	if pending.ID != -1 || pending.Text != "hello" || pending.SendState != domain.SendPending || !pending.Outgoing {
		t.Fatalf("pending message = %#v", pending)
	}
	assertCommands(t, commands, []Effect{
		SendText{RequestID: 7, LocalID: -1, ChatID: 9, Text: "hello"},
		SaveDraft{RequestID: 8, ChatID: 9},
	})

	failure := domain.AppError{Kind: domain.ErrorRateLimit, Message: "wait", RetryAfter: 5 * time.Second}
	updateState(&submitted, TextQueueFailed{RequestID: 7, LocalID: -1, Error: failure, FailedAt: now})
	failed := submitted
	message := failed.Messages[9][0]
	if message.Text != "hello" || message.SendState != domain.SendFailed || message.RetryAt != now.Add(5*time.Second) {
		t.Fatalf("failed message = %#v", message)
	}
	failed.SelectedMessage = -1

	commands = updateState(&failed, ActionReceived{Action: Retry, MessageID: -1, At: now.Add(4 * time.Second)})
	before := failed
	if before.Messages[9][0].SendState != domain.SendFailed || len(commands) != 0 {
		t.Fatalf("early retry = (%#v, %#v)", before.Messages[9][0], commands)
	}
	commands = updateState(&failed, ActionReceived{Action: Retry, MessageID: -1, At: now.Add(5 * time.Second)})
	retried := failed
	if retried.Messages[9][0].SendState != domain.SendPending || retried.Messages[9][0].Failure != nil {
		t.Fatalf("retried message = %#v", retried.Messages[9][0])
	}
	assertCommands(t, commands, []Effect{SendText{RequestID: 9, LocalID: -1, ChatID: 9, Text: "hello"}})
}

func TestQueuedTemporaryReplyIdentitySurvivesFailureAndSuccessReplacement(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	state := selectedWritableState()
	state.Messages[9] = []domain.Message{{ID: -1, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendPending, HasReply: true, ReplyToMessageID: 22}}
	temporary := domain.Message{ID: -44, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendPending}
	updateState(&state, TextQueued{RequestID: 3, LocalID: -1, Message: temporary})
	queued := state
	if got := queued.Messages[9][0]; got.ReplyToMessageID != 22 || !got.HasReply {
		t.Fatal("queued replacement lost reply identity")
	}

	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "offline", RetryAfter: 3 * time.Second}
	updateState(&queued, TelegramEvent{Value: telegram.MessageSendFailed{OldID: -44, Message: temporary, Error: failure}, ReceivedAt: now})
	failed := queued
	if got := failed.Messages[9][0]; got.ReplyToMessageID != 22 || !got.HasReply {
		t.Fatal("failed replacement lost reply identity")
	}

	durable := domain.Message{ID: 88, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendSucceeded}
	updateState(&failed, TelegramEvent{Value: telegram.MessageSendSucceeded{OldID: -44, Message: durable}, ReceivedAt: now.Add(time.Second)})
	succeeded := failed
	if got := succeeded.Messages[9][0]; got.ReplyToMessageID != 22 || !got.HasReply {
		t.Fatal("durable replacement lost reply identity")
	}
}

func TestQueuedTemporaryMessageThenTDLibFailureAndSuccessReplaceAtomically(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	state := selectedWritableState()
	state.Messages[9] = []domain.Message{{ID: -1, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendPending}}
	temporary := domain.Message{ID: -44, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendPending}
	updateState(&state, TextQueued{RequestID: 3, LocalID: -1, Message: temporary})
	queued := state
	if len(queued.Messages[9]) != 1 || queued.Messages[9][0].ID != -44 {
		t.Fatalf("queued messages = %#v", queued.Messages[9])
	}

	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "offline", RetryAfter: 3 * time.Second}
	failedMessage := temporary
	failedMessage.Text = ""
	updateState(&queued, TelegramEvent{Value: telegram.MessageSendFailed{OldID: -44, Message: failedMessage, Error: failure}, ReceivedAt: now})
	failed := queued
	if got := failed.Messages[9][0]; got.ID != -44 || got.Text != "hello" || got.SendState != domain.SendFailed || got.RetryAt != now.Add(3*time.Second) {
		t.Fatalf("TDLib failure = %#v", got)
	}

	durable := domain.Message{ID: 88, ChatID: 9, Text: "hello", Outgoing: true, SendState: domain.SendSucceeded}
	updateState(&failed, TelegramEvent{Value: telegram.MessageSendSucceeded{OldID: -44, Message: durable}, ReceivedAt: now.Add(time.Second)})
	succeeded := failed
	if !reflect.DeepEqual(succeeded.Messages[9], []domain.Message{durable}) {
		t.Fatalf("durable messages = %#v", succeeded.Messages[9])
	}
}

func TestPaginationUsesPerChatNewestRelativeOffsets(t *testing.T) {
	state := selectedWritableState()
	state.Height = 30
	state.NextRequestID = 20
	for id := domain.MessageID(1); id <= 12; id++ {
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: id, ChatID: 9, SentAt: time.Unix(int64(id), 0)})
	}
	state.History[9] = HistoryState{OldestID: 1}

	commands := updateState(&state, ActionReceived{Action: PageUp})
	paged := state
	if paged.History[9].ViewOffset != 11 || !paged.History[9].Loading || paged.History[9].RequestID != 20 {
		t.Fatalf("paged history = %#v", paged.History[9])
	}
	assertCommands(t, commands, []Effect{LoadMessages{RequestID: 20, ChatID: 9, Cursor: telegram.MessageCursor{FromMessageID: 1, Limit: 50}}})

	updateState(&paged, MessagesLoaded{RequestID: 20, ChatID: 9, Page: telegram.MessagePage{Done: true, Messages: []domain.Message{{ID: -1, ChatID: 9, SentAt: time.Unix(-1, 0)}, {ID: 1, ChatID: 9, SentAt: time.Unix(1, 0)}}}})
	loaded := paged
	if loaded.History[9].Loading || !loaded.History[9].Done || loaded.History[9].OldestID != -1 {
		t.Fatalf("loaded history = %#v", loaded.History[9])
	}
	if len(loaded.Messages[9]) != 13 || loaded.Messages[9][0].ID != -1 {
		t.Fatalf("deduplicated messages = %#v", loaded.Messages[9])
	}
	updateState(&loaded, ActionReceived{Action: PageDown})
	down := loaded
	if down.History[9].ViewOffset != 0 {
		t.Fatalf("page down offset = %d", down.History[9].ViewOffset)
	}
}

func TestMessageUpsertUpdatesPreviewAndKeepsScrolledViewportAnchored(t *testing.T) {
	state := selectedWritableState()
	state.History[9] = HistoryState{ViewOffset: 4}
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, SentAt: time.Unix(1, 0)}}
	incoming := domain.Message{ID: 2, ChatID: 9, Text: "new", SentAt: time.Unix(2, 0), SenderAvatar: domain.AvatarRef{UniqueID: "sender"}}

	commands := updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: incoming}, ReceivedAt: time.Unix(3, 0)})
	got := state
	if got.History[9].ViewOffset != 5 || got.Chats[0].LastMessage != "new" || got.Chats[0].LastMessageAt != 2 {
		t.Fatalf("updated state = %#v", got)
	}
	assertCommands(t, commands, []Effect{RenderAvatar{Key: "sender:message-group", Ref: incoming.SenderAvatar, Role: avatar.RoleMessageGroup}})
}

func TestDetailsAvatarModalOpenFailureAndRetry(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusDetails
	state.DetailsOpen = true
	state.NextRequestID = 30
	state.Chats[0].Title = "Mina"
	state.Chats[0].Avatar = domain.AvatarRef{UniqueID: "small", OriginalUniqueID: "large"}

	commands := updateState(&state, ActionReceived{Action: Activate})
	opened := state
	if opened.Modal == nil || !opened.Modal.Loading || opened.Modal.RequestID != 30 || opened.Focus != FocusModal || opened.Modal.PreviousFocus != FocusDetails {
		t.Fatalf("modal shell = %#v", opened)
	}
	assertCommands(t, commands, []Effect{OpenAvatar{RequestID: 30, Title: "Mina", Ref: state.Chats[0].Avatar}})

	failure := domain.AppError{Kind: domain.ErrorMedia, Message: "try again"}
	updateState(&opened, AvatarOpenFailed{RequestID: 30, Error: failure})
	failed := opened
	if failed.Modal == nil || failed.Modal.Loading || failed.Modal.Error == nil {
		t.Fatalf("failed modal = %#v", failed.Modal)
	}
	commands = updateState(&failed, ActionReceived{Action: Retry})
	retried := failed
	if retried.Modal.RequestID != 31 || !retried.Modal.Loading || retried.Modal.Error != nil || retried.Focus != FocusModal || retried.Modal.Path != "" {
		t.Fatalf("retried modal = %#v", retried.Modal)
	}
	assertCommands(t, commands, []Effect{OpenAvatar{RequestID: 31, Title: "Mina", Ref: state.Chats[0].Avatar}})

	updateState(&retried, AvatarOpened{RequestID: 31, Title: "Mina", Path: "/tmp/mina.jpg"})
	ready := retried
	if ready.Modal.Loading || ready.Modal.Path != "/tmp/mina.jpg" || ready.Modal.Error != nil {
		t.Fatalf("ready modal = %#v", ready.Modal)
	}
	staleCommands := updateState(&ready, AvatarOpened{RequestID: 30, Title: "old", Path: "/tmp/old.jpg"})
	if len(staleCommands) != 0 || ready.Modal == nil || ready.Modal.RequestID != 31 || ready.Modal.Title != "Mina" || ready.Modal.Path != "/tmp/mina.jpg" || ready.Focus != FocusModal {
		t.Fatalf("stale avatar result changed modal = %#v effects=%#v", ready.Modal, staleCommands)
	}
}

func TestAvatarRenderFailureUsesPlaceholderAndRetryRetainsRequest(t *testing.T) {
	state := InitialState()
	ref := domain.AvatarRef{UniqueID: "mina"}
	state.Avatars["mina:chat-list"] = AvatarState{Loading: true, Ref: ref, Role: avatar.RoleChatList}
	failure := domain.AppError{Kind: domain.ErrorMedia, Message: "bad image"}
	updateState(&state, AvatarRenderFailed{Key: "mina:chat-list", Error: failure})
	failed := state
	avatarState := failed.Avatars["mina:chat-list"]
	if avatarState.Loading || avatarState.Error == nil || avatarState.Cells.Width != 6 || avatarState.Cells.Height != 3 {
		t.Fatalf("failed avatar = %#v", avatarState)
	}
	commands := updateState(&failed, ActionReceived{Action: Retry, AvatarKey: "mina:chat-list"})
	retried := failed
	if !retried.Avatars["mina:chat-list"].Loading || retried.Avatars["mina:chat-list"].Error != nil {
		t.Fatalf("retried avatar = %#v", retried.Avatars["mina:chat-list"])
	}
	assertCommands(t, commands, []Effect{RenderAvatar{Key: "mina:chat-list", Ref: ref, Role: avatar.RoleChatList}})
}

func TestStartupFailureQuitsOnceAndLaterInputIsIgnored(t *testing.T) {
	state := InitialState()
	failure := domain.AppError{Kind: domain.ErrorConfiguration, Message: "configure account"}
	commands := updateState(&state, StartupFailed{Error: failure})
	failed := state
	if !failed.Quitting || failed.Fatal == nil || failed.Fatal.Message != "configure account" {
		t.Fatalf("failed state = %#v", failed)
	}
	assertCommands(t, commands, []Effect{BeginShutdown{}})
	commands = updateState(&failed, ActionReceived{Action: Quit, Rune: 'x'})
	if len(commands) != 0 || !failed.Quitting || failed.Fatal == nil || failed.Fatal.Message != "configure account" {
		t.Fatalf("input after quit changed fatal state: state=%#v effects=%#v", failed, commands)
	}
}

func TestChatLoadFailureAndStaleResultsRetainCachedChats(t *testing.T) {
	state := InitialState()
	state.ChatRequestID = 8
	state.ChatsLoading = true
	state.Chats = []domain.Chat{{ID: 7, Title: "cached"}}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "try again"}

	updateState(&state, ChatsLoadFailed{RequestID: 7, Error: failure})
	stale := state
	if !stale.ChatsLoading || stale.ChatsError != nil {
		t.Fatalf("stale failure changed indicators = %#v", stale)
	}
	updateState(&state, ChatsLoadFailed{RequestID: 8, Error: failure})
	current := state
	if current.ChatsLoading || current.ChatsError == nil || current.Chats[0].Title != "cached" {
		t.Fatalf("current failure state = %#v", current)
	}
	updateState(&current, ChatsLoaded{RequestID: 7, Page: telegram.ChatPage{Chats: []domain.Chat{{ID: 99}}}})
	ignored := current
	if !reflect.DeepEqual(ignored.Chats, current.Chats) {
		t.Fatalf("stale chat success replaced cache = %#v", ignored.Chats)
	}
}

func TestChatUpsertSortsPreservesSelectedIDAndRequestsOnlyNewAvatar(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 7, Order: 10}, {ID: 9, Order: 20}}
	state.SelectedChat = 0
	state.FocusedChat = 1
	state.Avatars["existing:chat-list"] = AvatarState{Loading: true}
	updated := domain.Chat{ID: 7, Title: "Mina", Order: 30, Avatar: domain.AvatarRef{UniqueID: "new"}}

	commands := updateState(&state, TelegramEvent{Value: telegram.ChatUpserted{Chat: updated}})
	got := state
	if got.Chats[0].ID != 7 || got.SelectedChat != 0 || got.FocusedChat != 1 || got.Chats[0].Title != "Mina" {
		t.Fatalf("upserted chats = %#v, selected=%d focused=%d", got.Chats, got.SelectedChat, got.FocusedChat)
	}
	assertCommands(t, commands, []Effect{RenderAvatar{Key: "new:chat-list", Ref: updated.Avatar, Role: avatar.RoleChatList}})
	commands = updateState(&got, TelegramEvent{Value: telegram.ChatUpserted{Chat: updated}})
	if len(commands) != 0 {
		t.Fatalf("duplicate avatar commands = %#v", commands)
	}
}

func TestSelectionFocusAndCloseActionsRespectLayoutAndDrafts(t *testing.T) {
	state := InitialState()
	state.Layout = LayoutWide
	state.Focus = FocusChats
	state.Chats = []domain.Chat{{ID: 1}, {ID: 2}}
	state.Drafts[1] = "one"
	state.Drafts[2] = "two"
	state.History[1] = HistoryState{Done: true}

	commands := updateState(&state, ActionReceived{Action: SelectNext})
	focused := state
	if focused.FocusedChat != 1 || focused.SelectedChat != 0 || focused.Drafts[1] != "one" || focused.Drafts[2] != "two" {
		t.Fatalf("focused state = %#v", focused)
	}
	if len(commands) != 0 {
		t.Fatalf("focus navigation commands = %#v, want none", commands)
	}
	updateState(&focused, ActionReceived{Action: Activate})
	menu := focused
	if menu.Focus != FocusChatActions || menu.ChatActions == nil || menu.ChatActions.ChatID != 2 {
		t.Fatalf("wide activate did not open focused chat actions: %#v", menu.ChatActions)
	}
	commands = updateState(&menu, ActionReceived{Action: Activate})
	activated := menu
	if activated.Focus != FocusChats || activated.SelectedChat != 1 || len(commands) == 0 {
		t.Fatalf("wide activate = focus:%v selected:%d commands:%#v", activated.Focus, activated.SelectedChat, commands)
	}
	updateState(&activated, ActionReceived{Action: FocusPane, TargetFocus: FocusConversation})
	updateState(&activated, ActionReceived{Action: FocusPane, TargetFocus: FocusComposer})
	composer := activated
	if composer.Focus != FocusComposer {
		t.Fatalf("focus composer = %v", composer.Focus)
	}
	updateState(&composer, ActionReceived{Action: Close})
	closed := composer
	if closed.Focus != FocusComposer || closed.Drafts[2] != "" {
		t.Fatalf("first composer close should cancel draft only: %#v", closed)
	}
	updateState(&closed, ActionReceived{Action: Close})
	if closed.Focus != FocusConversation {
		t.Fatalf("second composer close focus = %v", closed.Focus)
	}
	closed.Layout = LayoutNarrow
	updateState(&closed, ActionReceived{Action: Close})
	chatList := closed
	if chatList.Focus != FocusChats {
		t.Fatalf("narrow close focus = %v", chatList.Focus)
	}
}

func TestUnreadAndMentionNavigationWrapsFocusedChatWithoutSelecting(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.Layout = LayoutWide
		state.Focus = FocusChats
		state.Chats = []domain.Chat{
			{ID: 1, UnreadCount: 1, UnreadMentionCount: 1},
			{ID: 2},
			{ID: 3, UnreadCount: 2},
			{ID: 4, UnreadMentionCount: 2},
		}
		state.SelectedChat = 0
		return state
	}

	for _, test := range []struct {
		name   string
		start  int
		action Action
		wantID domain.ChatID
	}{
		{name: "next unread", start: 0, action: SelectNextUnread, wantID: 3},
		{name: "unread wraps", start: 2, action: SelectNextUnread, wantID: 1},
		{name: "next mention", start: 0, action: SelectNextMention, wantID: 4},
		{name: "mention wraps", start: 3, action: SelectNextMention, wantID: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Each branch starts from its own freshly built fixture because
			// updateState mutates the state it is given.
			got := fixture()
			got.FocusedChat = test.start
			gotCommands := updateState(&got, ActionReceived{Action: test.action})
			want := fixture()
			want.FocusedChat = test.start
			wantCommands := updateState(&want, ActionReceived{Action: FocusChat, ChatID: test.wantID})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotCommands, wantCommands) {
				t.Fatalf("navigation result = (%#v, %#v), want ordinary focus (%#v, %#v)", got, gotCommands, want, wantCommands)
			}
		})
	}
}

func TestUnreadAndMentionNavigationNoMatchShowsToast(t *testing.T) {
	for _, test := range []struct {
		action  Action
		message string
	}{
		{action: SelectNextUnread, message: "No unread chats"},
		{action: SelectNextMention, message: "No unread mentions"},
	} {
		state := InitialState()
		state.Layout = LayoutWide
		state.Focus = FocusChats
		state.Chats = []domain.Chat{{ID: 1, UnreadCount: 1, UnreadMentionCount: 1}, {ID: 2}}
		state.SelectedChat = 0

		commands := updateState(&state, ActionReceived{Action: test.action})
		if state.SelectedChat != 0 || len(commands) != 0 {
			t.Fatalf("action %v with current-only match navigated or emitted commands: selected=%d, commands=%#v", test.action, state.SelectedChat, commands)
		}
		if state.Toast == nil || state.Toast.Message != test.message || state.Toast.Kind != "" || state.ToastDuration != 2*time.Second || state.ToastGeneration == 0 {
			t.Fatalf("action %v toast = %#v, duration=%v, generation=%d", test.action, state.Toast, state.ToastDuration, state.ToastGeneration)
		}

		state.Focus = FocusConversation
		before := cloneState(state)
		commands = updateState(&state, ActionReceived{Action: test.action})
		if !reflect.DeepEqual(state, before) || len(commands) != 0 {
			t.Fatalf("action %v outside Chats focus changed state or emitted commands: %#v, %#v", test.action, state, commands)
		}
	}
}

func TestFocusPaneAndCycleOnlyVisitVisiblePanes(t *testing.T) {
	tests := []struct {
		name      string
		state     State
		action    ActionReceived
		wantFocus Focus
	}{
		{name: "narrow chat cannot focus hidden conversation", state: State{Layout: LayoutNarrow, Focus: FocusChats}, action: ActionReceived{Action: FocusPane, TargetFocus: FocusConversation}, wantFocus: FocusChats},
		{name: "normal cycles chats to conversation", state: State{Layout: LayoutNormal, Focus: FocusChats}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusConversation},
		{name: "wide reverse wraps chats to conversation", state: State{Layout: LayoutWide, Focus: FocusChats}, action: ActionReceived{Action: FocusPrevious}, wantFocus: FocusConversation},
		{name: "wide conversation wraps to chats without info", state: State{Layout: LayoutWide, Focus: FocusConversation}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusChats},
		{name: "wide composer next returns to conversation", state: State{Layout: LayoutWide, Focus: FocusComposer}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusConversation},
		{name: "wide composer previous returns to conversation", state: State{Layout: LayoutWide, Focus: FocusComposer}, action: ActionReceived{Action: FocusPrevious}, wantFocus: FocusConversation},
		{name: "wide info chats next to conversation", state: State{Layout: LayoutWide, Focus: FocusChats, DetailsOpen: true}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusConversation},
		{name: "wide info chats previous wraps to info", state: State{Layout: LayoutWide, Focus: FocusChats, DetailsOpen: true}, action: ActionReceived{Action: FocusPrevious}, wantFocus: FocusDetails},
		{name: "wide info conversation next to info", state: State{Layout: LayoutWide, Focus: FocusConversation, DetailsOpen: true}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusDetails},
		{name: "wide info previous to conversation", state: State{Layout: LayoutWide, Focus: FocusDetails, DetailsOpen: true}, action: ActionReceived{Action: FocusPrevious}, wantFocus: FocusConversation},
		{name: "wide info next wraps to chats", state: State{Layout: LayoutWide, Focus: FocusDetails, DetailsOpen: true}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusChats},
		{name: "normal details traps visible focus", state: State{Layout: LayoutNormal, Focus: FocusDetails, DetailsOpen: true}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusDetails},
		{name: "narrow details traps visible focus", state: State{Layout: LayoutNarrow, Focus: FocusDetails, DetailsOpen: true}, action: ActionReceived{Action: FocusPrevious}, wantFocus: FocusDetails},
		{name: "narrow conversation wraps to chats", state: State{Layout: LayoutNarrow, Focus: FocusConversation}, action: ActionReceived{Action: FocusNext}, wantFocus: FocusChats},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := InitialState()
			state.Layout, state.Focus, state.DetailsOpen = test.state.Layout, test.state.Focus, test.state.DetailsOpen
			updateState(&state, test.action)
			got := state
			if got.Focus != test.wantFocus {
				t.Fatalf("Focus = %v, want %v", got.Focus, test.wantFocus)
			}
		})
	}
}

func TestComposerEditingAndSubmitGuardsArePerChat(t *testing.T) {
	state := selectedWritableState()
	state.Drafts[9] = "界"
	updateState(&state, ActionReceived{Rune: '面'})
	edited := state
	updateState(&edited, ActionReceived{Action: ComposerNewline})
	updateState(&edited, ActionReceived{Action: ComposerBackspace})
	if edited.Drafts[9] != "界面" {
		t.Fatalf("edited draft = %q", edited.Drafts[9])
	}

	for _, test := range []struct {
		name      string
		configure func(*State)
	}{
		{name: "whitespace", configure: func(state *State) { state.Drafts[9] = " \n " }},
		{name: "offline", configure: func(state *State) { state.Drafts[9] = "hello"; state.Connection = domain.ConnectionOffline }},
		{name: "read only", configure: func(state *State) { state.Drafts[9] = "hello"; state.Chats[0].CanSend = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := selectedWritableState()
			test.configure(&candidate)
			commands := updateState(&candidate, ActionReceived{Action: ComposerSubmit})
			got := candidate
			if len(commands) != 0 || !reflect.DeepEqual(got.Messages[9], candidate.Messages[9]) || got.Drafts[9] != candidate.Drafts[9] {
				t.Fatalf("guard result = (%#v, %#v)", got, commands)
			}
		})
	}
}

func TestCurrentAndStaleMessageLoadFailuresUpdateOnlyMatchingHistory(t *testing.T) {
	state := selectedWritableState()
	state.History[9] = HistoryState{Loading: true, RequestID: 4, ViewOffset: 3}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "retry history"}
	updateState(&state, MessagesLoadFailed{RequestID: 3, ChatID: 9, Error: failure})
	stale := state
	if stale.History[9] != state.History[9] || stale.Toast != nil {
		t.Fatalf("stale failure = %#v", stale)
	}
	updateState(&state, MessagesLoadFailed{RequestID: 4, ChatID: 9, Error: failure})
	current := state
	if current.History[9].Loading || current.History[9].ViewOffset != 3 || current.Toast == nil || !current.Toast.Retryable() {
		t.Fatalf("current failure = %#v", current)
	}
}

func TestLoadedMessagesRenderEachMissingGroupAvatarOnce(t *testing.T) {
	state := selectedWritableState()
	state.History[9] = HistoryState{Loading: true, RequestID: 5}
	ref := domain.AvatarRef{UniqueID: "iris"}
	page := telegram.MessagePage{Messages: []domain.Message{
		{ID: 1, ChatID: 9, SenderAvatar: ref},
		{ID: 2, ChatID: 9, SenderAvatar: ref},
	}}
	commands := updateState(&state, MessagesLoaded{RequestID: 5, ChatID: 9, Page: page})
	got := state
	if len(commands) != 1 {
		t.Fatalf("avatar commands = %#v", commands)
	}
	assertCommands(t, commands, []Effect{RenderAvatar{Key: "iris:message-group", Ref: ref, Role: avatar.RoleMessageGroup}})
	if !got.Avatars["iris:message-group"].Loading {
		t.Fatal("avatar was not marked loading")
	}
}

func TestActivateFailedSelectedMessageHonorsCooldownWithoutChangingFocus(t *testing.T) {
	now := time.Unix(50, 0)
	state := selectedWritableState()
	state.Focus = FocusConversation
	state.SelectedMessage = -4
	state.Messages[9] = []domain.Message{{ID: -4, ChatID: 9, Text: "retry me", Outgoing: true, SendState: domain.SendFailed, RetryAt: now.Add(time.Second)}}

	commands := updateState(&state, ActionReceived{Action: Activate, At: now})
	early := state
	if early.Focus != FocusConversation || early.Messages[9][0].SendState != domain.SendFailed || len(commands) != 0 {
		t.Fatalf("early activate = (%#v, %#v)", early, commands)
	}
	commands = updateState(&state, ActionReceived{Action: Activate, At: now.Add(time.Second)})
	ready := state
	if ready.Focus != FocusConversation || ready.Messages[9][0].SendState != domain.SendPending || len(commands) != 1 {
		t.Fatalf("ready activate = (%#v, %#v)", ready, commands)
	}
}

func TestResizeClampsEachChatOffsetAndMessageLimitPreservesOutgoingFailures(t *testing.T) {
	state := selectedWritableState()
	state.Chats = append(state.Chats, domain.Chat{ID: 10})
	state.History[9] = HistoryState{ViewOffset: 99}
	state.History[10] = HistoryState{ViewOffset: 4}
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9}, {ID: 2, ChatID: 9}}
	state.Messages[10] = []domain.Message{{ID: 3, ChatID: 10}}
	updateState(&state, Resized{Width: 100, Height: 22})
	resized := state
	if resized.History[9].ViewOffset != 1 || resized.History[10].ViewOffset != 0 {
		t.Fatalf("clamped histories = %#v", resized.History)
	}

	for id := domain.MessageID(1); id <= maxMessagesPerChat+5; id++ {
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: id + 100, ChatID: 9, SentAt: time.Unix(int64(id), 0)})
	}
	failed := domain.Message{ID: -1, ChatID: 9, Text: "keep", Outgoing: true, SendState: domain.SendFailed, SentAt: time.Unix(-1, 0)}
	state.Messages[9] = append(state.Messages[9], failed)
	updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 9999, ChatID: 9, SentAt: time.Unix(9999, 0)}}})
	limited := state
	if len(limited.Messages[9]) != maxMessagesPerChat || messageIndex(limited.Messages[9], -1) < 0 {
		t.Fatalf("limited history length = %d, failed index = %d", len(limited.Messages[9]), messageIndex(limited.Messages[9], -1))
	}
}

func TestHistoryFailureRetainsErrorAndRetryUsesOldestCursor(t *testing.T) {
	state := selectedWritableState()
	state.NextRequestID = 20
	state.History[9] = HistoryState{Loading: true, RequestID: 7, OldestID: 55, ViewOffset: 3}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "offline"}

	updateState(&state, MessagesLoadFailed{RequestID: 7, ChatID: 9, Error: failure})
	failed := state
	if failed.History[9].Error == nil || failed.History[9].Loading {
		t.Fatalf("history failure was not retained: %#v", failed.History[9])
	}
	commands := updateState(&failed, ActionReceived{Action: Retry})
	retried := failed
	history := retried.History[9]
	if !history.Loading || history.Error != nil || history.RequestID != 20 {
		t.Fatalf("history retry state = %#v", history)
	}
	if len(commands) != 1 {
		t.Fatalf("retry commands = %#v", commands)
	}
	command, ok := commands[0].(LoadMessages)
	if !ok || command.ChatID != 9 || command.Cursor.FromMessageID != 55 || command.Cursor.Limit != pageSize {
		t.Fatalf("retry command = %#v", commands[0])
	}
}

func TestAvatarFailurePlaceholderUsesDisplayLabel(t *testing.T) {
	state := InitialState()
	chat := domain.Chat{ID: 9, Title: "週末", Avatar: domain.AvatarRef{UniqueID: "AQAD-file-id"}}
	key := avatar.CacheKey(chat.Avatar, avatar.RoleChatList)
	state.Avatars[key] = AvatarState{Loading: true, Ref: chat.Avatar, Role: avatar.RoleChatList, Label: chat.Title}
	updateState(&state, AvatarRenderFailed{Key: key, Error: domain.AppError{Kind: domain.ErrorMedia}})
	got := state
	cells := got.Avatars[key].Cells
	center := (cells.Height/2)*cells.Width + cells.Width/2
	if cells.Cells[center].Rune != '週' {
		t.Fatalf("placeholder initial = %q, want 週", cells.Cells[center].Rune)
	}
}

func TestPaginationBeyondBoundKeepsNewlyLoadedOlderPage(t *testing.T) {
	state := selectedWritableState()
	messages := make([]domain.Message, 0, maxMessagesPerChat)
	for id := 101; id <= 600; id++ {
		messages = append(messages, domain.Message{ID: domain.MessageID(id), ChatID: 9, SentAt: time.Unix(int64(id), 0)})
	}
	state.Messages[9] = messages
	state.History[9] = HistoryState{Loading: true, RequestID: 7, OldestID: 101}
	older := make([]domain.Message, 0, 50)
	for id := 51; id <= 100; id++ {
		older = append(older, domain.Message{ID: domain.MessageID(id), ChatID: 9, SentAt: time.Unix(int64(id), 0)})
	}

	updateState(&state, MessagesLoaded{RequestID: 7, ChatID: 9, Page: telegram.MessagePage{Messages: older, Done: false}})
	got := state

	if len(got.Messages[9]) != maxMessagesPerChat {
		t.Fatalf("message count = %d, want %d", len(got.Messages[9]), maxMessagesPerChat)
	}
	if got.Messages[9][0].ID != 51 || got.History[9].OldestID != 51 {
		t.Fatalf("oldest retained message/history = %d/%d, want 51/51", got.Messages[9][0].ID, got.History[9].OldestID)
	}
	if messageIndex(got.Messages[9], 600) >= 0 {
		t.Fatal("newest edge was not trimmed after loading older history")
	}
}

func TestSendResultReplacementIsScopedToMessageChat(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.Messages[1] = []domain.Message{{ID: -30, ChatID: 1, Text: "first", SendState: domain.SendPending}}
		state.Messages[2] = []domain.Message{{ID: -30, ChatID: 2, Text: "second", SendState: domain.SendPending}}
		return state
	}

	succeeded := fixture()
	updateState(&succeeded, TelegramEvent{Value: telegram.MessageSendSucceeded{
		OldID:   -30,
		Message: domain.Message{ID: 200, ChatID: 2, Text: "second", SendState: domain.SendSucceeded},
	}})
	if messageIndex(succeeded.Messages[1], -30) < 0 || messageIndex(succeeded.Messages[2], 200) < 0 || messageIndex(succeeded.Messages[2], -30) >= 0 {
		t.Fatalf("success replacement crossed chat boundary: %#v", succeeded.Messages)
	}

	failed := fixture()
	updateState(&failed, TelegramEvent{Value: telegram.MessageSendFailed{
		OldID:   -30,
		Message: domain.Message{ID: -31, ChatID: 2, Text: "second"},
		Error:   domain.AppError{Kind: domain.ErrorNetwork},
	}, ReceivedAt: time.Unix(20, 0)})
	if failed.Messages[1][0].SendState != domain.SendPending || failed.Messages[2][0].ID != -31 || failed.Messages[2][0].SendState != domain.SendFailed {
		t.Fatalf("failure replacement crossed chat boundary: %#v", failed.Messages)
	}
}

func TestMessageEditLifecyclePreservesDraftAndRetriesWithoutOptimisticMutation(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.Drafts[9] = "ordinary"
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "original"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	commands := updateState(&state, ActionReceived{Action: EditMessage})
	opened := state
	if opened.MessageMenu == nil || !opened.MessageMenu.Loading || len(commands) != 1 {
		t.Fatal("edit shortcut did not open authoritative menu")
	}
	request := commands[0].(GetMessageProperties)
	updateState(&opened, MessagePropertiesLoaded{RequestID: request.RequestID, ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Edit: true}})
	loaded := opened
	if loaded.MessageMenu == nil || selectedMenuAction(loaded.MessageMenu) != EditMessage {
		t.Fatal("authorized edit shortcut did not select Edit")
	}
	updateState(&loaded, ActionReceived{Action: Activate})
	editing := loaded
	if editing.EditTarget == nil || editing.EditTarget.Buffer != "original" || editing.Drafts[9] != "ordinary" || editing.Focus != FocusComposer {
		t.Fatal("edit mode did not isolate draft")
	}
	updateState(&editing, ActionReceived{Action: NoAction, Rune: 'x'})
	typed := editing
	if typed.EditTarget.Buffer != "originalx" || typed.Drafts[9] != "ordinary" || typed.Messages[9][0].Text != "original" {
		t.Fatal("edit input mutated draft or body")
	}
	commands = updateState(&typed, ActionReceived{Action: ComposerSubmit})
	submitting := typed
	if len(commands) != 1 || !submitting.EditTarget.Submitting || submitting.Messages[9][0].Text != "original" {
		t.Fatal("edit submit optimistic or absent")
	}
	edit := commands[0].(EditText)
	updateState(&submitting, TextEditFailed{RequestID: edit.RequestID, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "secret"}})
	failed := submitting
	if failed.EditTarget == nil || failed.EditTarget.Submitting || failed.EditTarget.Buffer != "originalx" || failed.EditTarget.Error == nil || failed.EditTarget.Error.Message != "Edit failed" || failed.EditTarget.Error.Cause != nil {
		t.Fatal("failure did not retain safe retry")
	}
	commands = updateState(&failed, ActionReceived{Action: ComposerSubmit})
	retry := failed
	if retry.EditTarget == nil || len(commands) != 1 {
		t.Fatal("retry unavailable")
	}
	updateState(&retry, TextEdited{RequestID: edit.RequestID, ChatID: 9, MessageID: 2, Message: domain.Message{ID: 2, ChatID: 9, Text: "stale"}})
	stale := retry
	if stale.EditTarget == nil || stale.Messages[9][0].Text != "original" {
		t.Fatal("stale result applied")
	}
}

func TestMessageEditCancelGuardsAndSuccessMergeMetadata(t *testing.T) {
	original := domain.Message{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "original", SenderName: "sender", Outgoing: true, SentAt: time.Unix(10, 0)}
	newEdit := func(buffer string) State {
		state := InitialState()
		state.Focus = FocusComposer
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.Drafts[9] = "ordinary"
		state.Messages[9] = []domain.Message{original}
		state.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "original", Buffer: buffer}
		return state
	}
	state := newEdit("original")
	commands := updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) != 0 || state.EditTarget == nil {
		t.Fatal("unchanged reached transport")
	}
	state.EditTarget.Buffer = "  \n"
	commands = updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) != 0 || state.EditTarget == nil {
		t.Fatal("empty reached transport")
	}
	updateState(&state, ActionReceived{Action: Close})
	if state.EditTarget != nil || state.Drafts[9] != "ordinary" || state.Focus != FocusComposer {
		t.Fatal("Escape did not cancel edit")
	}

	state = newEdit("changed")
	commands = updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) != 1 {
		t.Fatalf("edit command = %#v", commands)
	}
	request := commands[0].(EditText)
	updateState(&state, TextEdited{RequestID: request.RequestID, ChatID: 9, MessageID: 2, Message: domain.Message{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "changed", EditedAt: time.Unix(20, 0)}})
	got := state.Messages[9][0]
	if state.EditTarget != nil || got.Text != "changed" || got.SenderName != "sender" || !got.Outgoing || got.SentAt != original.SentAt || got.EditedAt.IsZero() || state.Drafts[9] != "ordinary" {
		t.Fatal("success did not preserve metadata")
	}
}

func TestMessageEditCancelsOnChatSwitchExternalChangeAndDisappearance(t *testing.T) {
	base := InitialState()
	base.Focus = FocusComposer
	base.Chats = []domain.Chat{{ID: 9}, {ID: 10}}
	base.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-original"}}
	base.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "opaque-original", Buffer: "opaque-buffer"}

	updateState(&base, ActionReceived{Action: SelectChat, ChatID: 10})
	switched := base
	if switched.EditTarget != nil {
		t.Fatal("chat switch retained edit target")
	}
	updateState(&base, TelegramEvent{Value: telegram.MessageContentUpdated{ChatID: 9, MessageID: 2, Kind: domain.MessageText, Text: "opaque-external"}})
	externallyChanged := base
	if externallyChanged.EditTarget != nil {
		t.Fatal("external content update retained edit target")
	}

	base.Messages[9] = make([]domain.Message, 0, maxMessagesPerChat)
	for index := 1; index <= maxMessagesPerChat; index++ {
		base.Messages[9] = append(base.Messages[9], domain.Message{ID: domain.MessageID(index), ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(int64(index), 0)})
	}
	base.EditTarget = &EditTarget{ChatID: 9, MessageID: 1, Original: "opaque-original", Buffer: "opaque-buffer"}
	updateState(&base, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(999, 0)}}})
	evicted := base
	if evicted.EditTarget != nil {
		t.Fatal("disappeared edit target was retained")
	}

	base.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-original"}}
	base.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "opaque-original", Buffer: "opaque-buffer"}
	updateState(&base, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 2, ChatID: 9, Kind: domain.MessagePhoto}}})
	replaced := base
	if replaced.EditTarget != nil {
		t.Fatal("non-editable replacement retained edit target")
	}

	base.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-original"}}
	base.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "opaque-original", Buffer: "opaque-buffer"}
	updateState(&base, TelegramEvent{Value: telegram.MessagesDeleted{ChatID: 9, MessageIDs: []domain.MessageID{2}}})
	deleted := base
	if deleted.EditTarget != nil || messageIndex(deleted.Messages[9], 2) >= 0 {
		t.Fatal("external deletion retained edit target or message")
	}

	base.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-original"}, {ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "opaque-newest"}}
	base.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "opaque-original", Buffer: "opaque-buffer"}
	updateState(&base, TelegramEvent{Value: telegram.MessagesDeleted{ChatID: 9, MessageIDs: []domain.MessageID{2}, FromCache: true}})
	evicted = base
	if evicted.EditTarget == nil || messageIndex(evicted.Messages[9], 2) < 0 || messageIndex(evicted.Messages[9], 3) < 0 {
		t.Fatal("cache eviction removed valid messages or edit target")
	}
}

func TestDeleteMessageActivatesWithinModalUsesSavedIdentityAndRevokeFalse(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 1 // prove modal identity wins over selection
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, DeleteForSelf: true}}

	commands := updateState(&state, ActionReceived{Action: DeleteMessage})
	deleting := state
	if len(commands) != 1 {
		t.Fatalf("delete commands = %d", len(commands))
	}
	command, ok := commands[0].(DeleteMessageCommand)
	if !ok || command.ChatID != 9 || command.MessageID != 2 || command.Revoke {
		t.Fatalf("delete command = %#v", commands[0])
	}
	// No optimistic removal before transport success.
	if len(deleting.Messages[9]) != 1 || deleting.MessageMenu == nil {
		t.Fatal("optimistic removal or closed menu before success")
	}
	if deleting.MessageMenu.RequestID != command.RequestID {
		t.Fatal("in-flight delete not tracked by identity")
	}
}

func TestDeleteMessageSuccessRemovesExactMessageReconcilesSelectionAndClearsTargets(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(1, 0)},
		{ID: 2, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(2, 0)},
		{ID: 3, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(3, 0)},
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, DeleteForSelf: true}}
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 2, Sender: "x", Preview: "y"}
	state.EditTarget = &EditTarget{ChatID: 9, MessageID: 2, Original: "o", Buffer: "b"}

	commands := updateState(&state, ActionReceived{Action: DeleteMessage})
	deleting := state
	request := commands[0].(DeleteMessageCommand).RequestID
	updateState(&deleting, MessageDeleted{RequestID: request, ChatID: 9, MessageID: 2})
	deleted := deleting

	if len(deleted.Messages[9]) != 2 || messageIndex(deleted.Messages[9], 2) >= 0 {
		t.Fatal("success did not remove exactly the deleted message")
	}
	if deleted.SelectedMessage != 3 || deleted.SelectedMessageChat != 9 {
		t.Fatal("selection did not reconcile to nearest remaining message")
	}
	if deleted.MessageMenu != nil || deleted.ReplyTarget != nil || deleted.EditTarget != nil {
		t.Fatal("deleted message retained menu or reply/edit target")
	}
	if deleted.Focus != FocusConversation {
		t.Fatalf("focus = %v, want conversation", deleted.Focus)
	}
}

func TestDeleteMessageSuccessWhenMessageAlreadyAbsentIsIdempotent(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 3, ChatID: 9, Kind: domain.MessageText}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 3
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForSelf: true}}
	updateState(&state, MessageDeleted{RequestID: 77, ChatID: 9, MessageID: 2})
	deleting := state
	if deleting.MessageMenu == nil || len(deleting.Messages[9]) != 1 {
		t.Fatal("stale success applied")
	}
}

func TestDeleteMessageFailureRetainsMessageAndShowsConstantToast(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, DeleteForSelf: true}}

	commands := updateState(&state, ActionReceived{Action: DeleteMessage})
	deleting := state
	request := commands[0].(DeleteMessageCommand).RequestID
	updateState(&deleting, MessageDeleteFailed{RequestID: request, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "raw secret-body 9 2"}})
	failed := deleting
	if messageIndex(failed.Messages[9], 2) < 0 {
		t.Fatal("failure removed the message")
	}
	if failed.Toast == nil || failed.Toast.Message != "Delete failed" || failed.Toast.Cause != nil || strings.Contains(failed.Toast.Message, "secret-body") || strings.Contains(failed.Toast.Message, "9") || strings.Contains(failed.Toast.Message, "2") {
		t.Fatalf("failure toast = %#v", failed.Toast)
	}
	if failed.MessageMenu == nil || failed.Focus != FocusModal {
		t.Fatalf("failure changed menu or focus: menu:%t focus:%v", failed.MessageMenu != nil, failed.Focus)
	}
}

func TestDeleteForEveryoneIssuesRevokeDeleteAndRequiresDeleteForAll(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForAll: true}}
	commands := updateState(&state, ActionReceived{Action: DeleteForEveryone})
	if len(commands) != 1 {
		t.Fatal("revoke delete command missing")
	}
	command := commands[0].(DeleteMessageCommand)
	if !command.Revoke || command.ChatID != 9 || command.MessageID != 2 {
		t.Fatalf("revoke command = %#v", command)
	}

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForSelf: true}}
	commands = updateState(&state, ActionReceived{Action: DeleteForEveryone})
	selfOnly := state
	if len(commands) != 0 {
		t.Fatal("delete for everyone issued without DeleteForAll capability")
	}
	if selfOnly.MessageMenu == nil {
		t.Fatal("unauthorized delete closed the menu")
	}

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForAll: true}}
	commands = updateState(&state, ActionReceived{Action: DeleteMessage})
	forAllOnly := state
	if len(commands) != 0 {
		t.Fatal("delete for self issued without DeleteForSelf capability")
	}
	if forAllOnly.MessageMenu == nil {
		t.Fatal("unauthorized self-delete closed the menu")
	}
}

func TestDeleteMessageEventsIgnoreStaleAndIdentityMismatch(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, RequestID: 99, Capabilities: domain.MessageCapabilities{Copy: true, DeleteForSelf: true}}

	// Stale success: RequestID does not match the in-flight delete.
	updateState(&state, MessageDeleted{RequestID: 98, ChatID: 9, MessageID: 2})
	stale := state
	if stale.MessageMenu == nil || messageIndex(stale.Messages[9], 2) < 0 {
		t.Fatal("stale success applied")
	}
	// Identity mismatch on failure.
	updateState(&state, MessageDeleteFailed{RequestID: 99, ChatID: 10, MessageID: 2, Error: domain.AppError{Message: "raw 10 2"}})
	mismatched := state
	if mismatched.Toast != nil || mismatched.MessageMenu == nil {
		t.Fatal("identity-mismatched failure applied")
	}
	// Identity mismatch on success.
	updateState(&state, MessageDeleted{RequestID: 99, ChatID: 9, MessageID: 3})
	wrongMessage := state
	if wrongMessage.MessageMenu == nil || messageIndex(wrongMessage.Messages[9], 2) < 0 {
		t.Fatal("identity-mismatched success applied")
	}
	// Closed menu ignores a late, otherwise-matching success.
	updateState(&state, ActionReceived{Action: Close})
	closed := state
	updateState(&closed, MessageDeleted{RequestID: 99, ChatID: 9, MessageID: 2})
	late := closed
	if late.Toast != nil || messageIndex(late.Messages[9], 2) < 0 || late.SelectedMessage != 2 {
		t.Fatal("late success applied after menu closed")
	}
}

func TestDeleteMessageSelectionReconcilesToPredecessorWhenNewestDeleted(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(1, 0)},
		{ID: 2, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(2, 0)},
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForSelf: true}}
	commands := updateState(&state, ActionReceived{Action: DeleteMessage})
	deleting := state
	request := commands[0].(DeleteMessageCommand).RequestID
	updateState(&deleting, MessageDeleted{RequestID: request, ChatID: 9, MessageID: 2})
	deleted := deleting
	if deleted.SelectedMessage != 1 || deleted.SelectedMessageChat != 9 || len(deleted.Messages[9]) != 1 {
		t.Fatalf("newest deletion did not keep predecessor selected: %#v", deleted.Messages[9])
	}
}

func TestDeleteForEveryoneHasSingleRowOrderAndNoOptimisticRemoval(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{DeleteForSelf: true, DeleteForAll: true}}
	commands := updateState(&state, ActionReceived{Action: DeleteForEveryone})
	deleting := state
	if len(commands) != 1 || len(deleting.Messages[9]) != 1 {
		t.Fatal("delete failed to preserve until success")
	}
}

func TestMessageActionMenuCapturesPinnedStateFromSelectedMessage(t *testing.T) {
	fixture := func(pinned bool) State {
		state := InitialState()
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Pinned: pinned}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		return state
	}
	// Each menu open starts from a conversation without an open menu.
	state := fixture(true)
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || !state.MessageMenu.Pinned {
		t.Fatalf("menu did not capture pinned state: %#v", state.MessageMenu)
	}
	state = fixture(false)
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	if state.MessageMenu == nil || state.MessageMenu.Pinned {
		t.Fatalf("menu captured stale pinned state: %#v", state.MessageMenu)
	}
}

func TestPinMessageMenuOrderAndCapabilityGating(t *testing.T) {
	caps := domain.MessageCapabilities{Reply: true, Forward: true, Edit: true, Copy: true, Pin: true, DeleteForSelf: true, DeleteForAll: true}
	menu := &MessageActionMenu{Capabilities: caps, CanReact: true}
	if got := actionMenuItemCount(menu); got != 8 {
		t.Fatalf("actionMenuItemCount = %d, want 8", got)
	}
	for index, want := range []Action{ReplyMessage, ForwardMessageSource, EditMessage, CopyMessage, ReactMessage, PinMessage, DeleteMessage, DeleteForEveryone} {
		menu := &MessageActionMenu{Capabilities: caps, CanReact: true, Selected: index}
		if got := selectedMenuAction(menu); got != want {
			t.Fatalf("selectedMenuAction at index %d = %v, want %v", index, got, want)
		}
		selectMessageMenuAction(menu, want)
		if menu.Selected != index {
			t.Fatalf("selectMessageMenuAction(%v) selected %d, want %d", want, menu.Selected, index)
		}
	}

	withoutPin := domain.MessageCapabilities{Reply: true, Forward: true, Edit: true, Copy: true, DeleteForSelf: true, DeleteForAll: true}
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: withoutPin, CanReact: true}); got != 7 {
		t.Fatalf("count without Pin = %d, want 7", got)
	}
	if got := selectedMenuAction(&MessageActionMenu{Capabilities: withoutPin, CanReact: true, Selected: 5}); got != DeleteMessage {
		t.Fatalf("Copy+React+Delete selection without Pin = %v", got)
	}
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: caps, CanReact: true, Loading: true}); got != 7 {
		t.Fatalf("count while loading = %d, want 7", got)
	}
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: caps, CanReact: true, Error: &domain.AppError{Message: "x"}}); got != 7 {
		t.Fatalf("count on error = %d, want 7", got)
	}
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: caps, CanReact: false}); got != 7 {
		t.Fatalf("count without local React = %d, want 7", got)
	}
}

func TestPinMessageActivationResolvesUnpinFromMenuPinned(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 1 // prove menu identity wins
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}

	commands := updateState(&state, ActionReceived{Action: PinMessage})
	pinning := state
	if len(commands) != 1 {
		t.Fatalf("pin commands = %d", len(commands))
	}
	command, ok := commands[0].(PinMessageCommand)
	if !ok || command.ChatID != 9 || command.MessageID != 2 || command.Unpin {
		t.Fatalf("pin command = %#v", commands[0])
	}
	if len(pinning.Messages[9]) != 1 || pinning.MessageMenu == nil || pinning.Messages[9][0].Pinned {
		t.Fatal("optimistic mutation or closed menu before success")
	}
	if pinning.MessageMenu.RequestID != command.RequestID {
		t.Fatal("in-flight pin not tracked by identity")
	}

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Pinned: true, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}
	commands = updateState(&state, ActionReceived{Action: PinMessage})
	unpinning := state
	unpin := commands[0].(PinMessageCommand)
	if !unpin.Unpin || unpin.ChatID != 9 || unpin.MessageID != 2 || unpinning.Messages[9][0].Pinned {
		t.Fatalf("unpin command = %#v", commands[0])
	}
}

func TestPinMessageRequiresCapabilityAndLivingIdentity(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true}, PreviousFocus: FocusConversation}
	commands := updateState(&state, ActionReceived{Action: PinMessage})
	gated := state
	if len(commands) != 0 || gated.MessageMenu == nil {
		t.Fatal("pin issued without Pin capability")
	}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 99, Capabilities: domain.MessageCapabilities{Pin: true}, PreviousFocus: FocusConversation}
	commands = updateState(&state, ActionReceived{Action: PinMessage})
	missing := state
	if len(commands) != 0 {
		t.Fatal("pin issued for a missing identity")
	}
	if missing.MessageMenu == nil {
		t.Fatal("missing identity closed the menu")
	}
}

func TestPinMessageSuccessSetsPinnedAndShowsToast(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}

	commands := updateState(&state, ActionReceived{Action: PinMessage})
	pinning := state
	request := commands[0].(PinMessageCommand).RequestID
	updateState(&pinning, MessagePinChanged{RequestID: request, ChatID: 9, MessageID: 2, Pinned: true})
	changed := pinning
	if !changed.Messages[9][0].Pinned {
		t.Fatal("success did not set Pinned")
	}
	if changed.MessageMenu != nil || changed.Focus != FocusConversation {
		t.Fatalf("success kept menu or focus: menu:%t focus:%v", changed.MessageMenu != nil, changed.Focus)
	}
	if changed.Toast == nil || changed.Toast.Message != "Message pinned" || changed.Toast.Cause != nil || strings.Contains(changed.Toast.Message, "opaque-body") || strings.Contains(changed.Toast.Message, "9") || strings.Contains(changed.Toast.Message, "2") {
		t.Fatalf("pin toast = %#v", changed.Toast)
	}

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Pinned: true, Capabilities: domain.MessageCapabilities{Pin: true}}
	state.Messages[9][0].Pinned = true
	commands = updateState(&state, ActionReceived{Action: PinMessage})
	unpinning := state
	request = commands[0].(PinMessageCommand).RequestID
	updateState(&unpinning, MessagePinChanged{RequestID: request, ChatID: 9, MessageID: 2, Pinned: false})
	unpinned := unpinning
	if unpinned.Messages[9][0].Pinned {
		t.Fatal("unpin success did not clear Pinned")
	}
	if unpinned.Toast == nil || unpinned.Toast.Message != "Message unpinned" || unpinned.MessageMenu != nil {
		t.Fatalf("unpin result = toast:%#v menu:%#v", unpinned.Toast, unpinned.MessageMenu)
	}
}

func TestPinMessageFailureShowsConstantToastAndClosesMenu(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}

	commands := updateState(&state, ActionReceived{Action: PinMessage})
	pinning := state
	request := commands[0].(PinMessageCommand).RequestID
	updateState(&pinning, MessagePinFailed{RequestID: request, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "raw secret-body 9 2"}})
	failed := pinning
	if messageIndex(failed.Messages[9], 2) < 0 || failed.Messages[9][0].Pinned {
		t.Fatal("failure mutated the message")
	}
	if failed.Toast == nil || failed.Toast.Message != "Pin failed" || failed.Toast.Cause != nil || strings.Contains(failed.Toast.Message, "secret-body") || strings.Contains(failed.Toast.Message, "9") || strings.Contains(failed.Toast.Message, "2") {
		t.Fatalf("failure toast = %#v", failed.Toast)
	}
	if failed.MessageMenu != nil || failed.Focus != FocusConversation {
		t.Fatalf("failure kept menu/focus: menu:%t focus:%v", failed.MessageMenu != nil, failed.Focus)
	}

	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Pinned: true, Capabilities: domain.MessageCapabilities{Pin: true}}
	commands = updateState(&state, ActionReceived{Action: PinMessage})
	unpinning := state
	request = commands[0].(PinMessageCommand).RequestID
	updateState(&unpinning, MessagePinFailed{RequestID: request, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "raw"}})
	unpinFailed := unpinning
	if unpinFailed.Toast == nil || unpinFailed.Toast.Message != "Unpin failed" || unpinFailed.Focus != FocusConversation {
		t.Fatalf("unpin failure toast/focus = %#v / %v", unpinFailed.Toast, unpinFailed.Focus)
	}
}

func TestPinMessageEventsIgnoreStaleAndIdentityMismatch(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, RequestID: 99, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}

	updateState(&state, MessagePinChanged{RequestID: 98, ChatID: 9, MessageID: 2, Pinned: true})
	stale := state
	if stale.MessageMenu == nil || stale.Messages[9][0].Pinned {
		t.Fatal("stale success applied")
	}
	updateState(&state, MessagePinFailed{RequestID: 99, ChatID: 10, MessageID: 2, Error: domain.AppError{Message: "raw 10 2"}})
	mismatched := state
	if mismatched.Toast != nil || mismatched.MessageMenu == nil {
		t.Fatal("identity-mismatched failure applied")
	}
	updateState(&state, MessagePinChanged{RequestID: 99, ChatID: 9, MessageID: 3, Pinned: true})
	wrongMessage := state
	if wrongMessage.MessageMenu == nil || wrongMessage.Messages[9][0].Pinned {
		t.Fatal("identity-mismatched success applied")
	}
	updateState(&state, ActionReceived{Action: Close})
	closed := state
	updateState(&closed, MessagePinChanged{RequestID: 99, ChatID: 9, MessageID: 2, Pinned: true})
	late := closed
	if late.Toast != nil || late.Messages[9][0].Pinned {
		t.Fatal("late success applied after menu closed")
	}
}

func TestMessagePinnedUpdatedReconcilesInPlace(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
		return state
	}
	got := fixture()
	updateState(&got, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 2, Pinned: true}})
	if !got.Messages[9][0].Pinned {
		t.Fatal("external pin update not applied in place")
	}
	updateState(&got, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 2, Pinned: false}})
	if got.Messages[9][0].Pinned {
		t.Fatal("external unpin update not applied in place")
	}
	pointer := fixture()
	updateState(&pointer, TelegramEvent{Value: &telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 2, Pinned: true}})
	if !pointer.Messages[9][0].Pinned {
		t.Fatal("pointer external pin update not normalized")
	}
	unknown := fixture()
	updateState(&unknown, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 99, Pinned: true}})
	if unknown.Messages[9][0].Pinned {
		t.Fatal("unknown identity pin applied")
	}
}

func TestCanonicalMessageMenuOrderIsReplyForwardEditCopyReactDelete(t *testing.T) {
	caps := domain.MessageCapabilities{Reply: true, Forward: true, Edit: true, Copy: true, DeleteForSelf: true, DeleteForAll: true}
	menu := &MessageActionMenu{Capabilities: caps, CanReact: true}
	if got := actionMenuItemCount(menu); got != 7 {
		t.Fatalf("actionMenuItemCount = %d, want 7", got)
	}
	for index, want := range []Action{ReplyMessage, ForwardMessageSource, EditMessage, CopyMessage, ReactMessage, DeleteMessage, DeleteForEveryone} {
		menu := &MessageActionMenu{Capabilities: caps, CanReact: true, Selected: index}
		if got := selectedMenuAction(menu); got != want {
			t.Fatalf("selectedMenuAction at index %d = %v, want %v", index, got, want)
		}
		selectMessageMenuAction(menu, want)
		if menu.Selected != index {
			t.Fatalf("selectMessageMenuAction(%v) selected %d, want %d", want, menu.Selected, index)
		}
	}
}

func TestForwardMenuActionIsCapabilityGated(t *testing.T) {
	base := domain.MessageCapabilities{Reply: true, Copy: true}
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: base}); got != 2 {
		t.Fatalf("count without Forward = %d, want 2", got)
	}
	gated := base
	gated.Forward = true
	if got := actionMenuItemCount(&MessageActionMenu{Capabilities: gated}); got != 3 {
		t.Fatalf("count with Forward = %d, want 3", got)
	}
	if got := selectedMenuAction(&MessageActionMenu{Capabilities: base, Selected: 1}); got != CopyMessage {
		t.Fatalf("Copy selection without Forward = %v", got)
	}
	if got := selectedMenuAction(&MessageActionMenu{Capabilities: gated, Selected: 1}); got != ForwardMessageSource {
		t.Fatalf("Forward selection = %v", got)
	}
}

func TestForwardMenuActivationVerifiesIdentityAndOpensPicker(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}, {ID: 10, Title: "Dest"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 1 // prove menu identity wins
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Forward: true}, PreviousFocus: FocusConversation}
	commands := updateState(&state, ActionReceived{Action: ForwardMessageSource})
	opened := state
	if len(commands) != 0 {
		t.Fatalf("picker open commands = %d, want 0", len(commands))
	}
	if opened.MessageMenu != nil {
		t.Fatal("message menu retained while picker open")
	}
	if opened.Focus != FocusForwardPicker {
		t.Fatalf("focus = %v, want FocusForwardPicker", opened.Focus)
	}
	if opened.ForwardPicker == nil || opened.ForwardPicker.SourceChatID != 9 || opened.ForwardPicker.SourceMessageID != 2 || opened.ForwardPicker.SelectedChat != 0 || opened.ForwardPicker.RequestID == 0 {
		t.Fatalf("picker = %#v", opened.ForwardPicker)
	}
}

func TestForwardMenuActivationRequiresCapabilityAndLivingIdentity(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true}, PreviousFocus: FocusConversation}
	updateState(&state, ActionReceived{Action: ForwardMessageSource})
	gated := state
	if gated.ForwardPicker != nil {
		t.Fatal("picker opened without Forward capability")
	}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 99, Capabilities: domain.MessageCapabilities{Forward: true}, PreviousFocus: FocusConversation}
	updateState(&state, ActionReceived{Action: ForwardMessageSource})
	missing := state
	if missing.ForwardPicker != nil {
		t.Fatal("picker opened for a missing source identity")
	}
}

func TestForwardPickerNavigationAndConfirmEmitsExactCommand(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 10, Title: "A"}, {ID: 11, Title: "B"}, {ID: 12, Title: "C"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0, RequestID: 42}

	updateState(&state, ActionReceived{Action: SelectNext})
	next := state
	if next.ForwardPicker == nil || next.ForwardPicker.SelectedChat != 1 {
		t.Fatalf("SelectNext selected %d", next.ForwardPicker.SelectedChat)
	}
	updateState(&next, ActionReceived{Action: SelectPrevious})
	if next.ForwardPicker.SelectedChat != 0 {
		t.Fatalf("SelectPrevious selected %d", next.ForwardPicker.SelectedChat)
	}
	updateState(&next, ActionReceived{Action: SelectNext})
	updateState(&next, ActionReceived{Action: SelectNext})
	if next.ForwardPicker.SelectedChat != 2 {
		t.Fatalf("wrap selection = %d, want 2", next.ForwardPicker.SelectedChat)
	}

	commands := updateState(&next, ActionReceived{Action: Activate})
	confirmed := next
	if len(commands) != 1 {
		t.Fatalf("confirm commands = %d, want 1", len(commands))
	}
	command, ok := commands[0].(ForwardMessageCommand)
	if !ok || command.RequestID != 42 || command.SourceChatID != 9 || command.SourceMessageID != 2 || command.DestinationChatID != 12 {
		t.Fatalf("forward command = %#v", commands[0])
	}
	if confirmed.ForwardPicker == nil {
		t.Fatal("picker closed before the result arrived")
	}
}

func TestForwardPickerCancelRestoresFocusAndClearsPicker(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 9}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0, RequestID: 7}
	updateState(&state, ActionReceived{Action: Close})
	got := state
	if got.ForwardPicker != nil || got.Focus != FocusConversation {
		t.Fatalf("cancel result = picker:%#v focus:%v", got.ForwardPicker, got.Focus)
	}
}

func TestForwardSuccessClosesPickerShowsToastAndDoesNotOptimisticallyInsert(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}, {ID: 10, Title: "Dest"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1, RequestID: 42}
	updateState(&state, MessageForwarded{RequestID: 42, DestinationChatID: 10})
	got := state
	if got.ForwardPicker != nil || got.Focus != FocusConversation {
		t.Fatalf("success focus = picker:%#v focus:%v", got.ForwardPicker, got.Focus)
	}
	if got.Toast == nil || got.Toast.Message != "Message forwarded" || got.Toast.Cause != nil {
		t.Fatalf("success toast = %#v", got.Toast)
	}
	if len(got.Messages[10]) != 0 {
		t.Fatal("forward inserted an optimistic message into the destination chat")
	}
}

func TestForwardFailureClosesPickerAndShowsConstantToast(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}, {ID: 10, Title: "Dest"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1, RequestID: 42}
	updateState(&state, MessageForwardFailed{RequestID: 42, DestinationChatID: 10, Error: domain.AppError{Message: "raw secret-body 9 10 42"}})
	got := state
	if got.ForwardPicker != nil || got.Focus != FocusConversation {
		t.Fatalf("failure focus = picker:%#v focus:%v", got.ForwardPicker, got.Focus)
	}
	if got.Toast == nil || got.Toast.Message != "Forward failed" || got.Toast.Cause != nil || strings.Contains(got.Toast.Message, "secret-body") || strings.Contains(got.Toast.Message, "9") || strings.Contains(got.Toast.Message, "10") || strings.Contains(got.Toast.Message, "42") {
		t.Fatalf("failure toast = %#v", got.Toast)
	}
	if messageIndex(got.Messages[9], 2) < 0 {
		t.Fatal("failure removed the source message")
	}
}

func TestForwardPickerEventsIgnoreStaleAndIdentityMismatch(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 9}, {ID: 10}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1, RequestID: 42}

	updateState(&state, MessageForwarded{RequestID: 41, DestinationChatID: 10})
	stale := state
	if stale.ForwardPicker == nil || stale.Toast != nil {
		t.Fatal("stale success applied")
	}
	updateState(&state, MessageForwardFailed{RequestID: 42, DestinationChatID: 9, Error: domain.AppError{Message: "raw"}})
	wrongDestination := state
	if wrongDestination.ForwardPicker == nil || wrongDestination.Toast != nil {
		t.Fatal("identity-mismatched failure applied")
	}
	updateState(&state, ActionReceived{Action: Close})
	late := state
	updateState(&late, MessageForwarded{RequestID: 42, DestinationChatID: 10})
	if late.Toast != nil {
		t.Fatal("late success applied after picker closed")
	}
}

func TestForwardPickerClearedOnChatSwitchAndSourceDisappearance(t *testing.T) {
	state := InitialState()
	state.Focus = FocusForwardPicker
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}, {ID: 10, Title: "Dest"}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(1, 0)}}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0, RequestID: 42}
	updateState(&state, ActionReceived{Action: SelectChat, ChatID: 10})
	switched := state
	if switched.ForwardPicker != nil {
		t.Fatal("chat switch retained picker")
	}

	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0, RequestID: 42}
	updateState(&state, TelegramEvent{Value: telegram.MessagesDeleted{ChatID: 9, MessageIDs: []domain.MessageID{2}}})
	deleted := state
	if deleted.ForwardPicker != nil || deleted.Focus != FocusConversation {
		t.Fatalf("deleted source retained picker: %#v focus:%v", deleted.ForwardPicker, deleted.Focus)
	}

	state.Messages[9] = make([]domain.Message, 0, maxMessagesPerChat)
	for index := 1; index <= maxMessagesPerChat; index++ {
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: domain.MessageID(index), ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(int64(index), 0)})
	}
	state.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 1, SelectedChat: 0, RequestID: 43}
	updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, SentAt: time.Unix(999, 0)}}})
	evicted := state
	if evicted.ForwardPicker != nil {
		t.Fatal("evicted source message retained picker")
	}
}

func TestReactLocalCapabilityGatingAtMenuOpen(t *testing.T) {
	flags := func(canReact bool, id domain.MessageID, service bool, send domain.SendState) State {
		state := InitialState()
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9, CanReact: canReact}}
		state.Messages[9] = []domain.Message{{ID: id, ChatID: 9, Kind: domain.MessageText, Service: service, SendState: send}}
		state.SelectedMessageChat, state.SelectedMessage = 9, id
		return state
	}

	open := func(state State) bool {
		updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
		opened := state
		return opened.MessageMenu != nil && opened.MessageMenu.CanReact
	}

	if open(flags(true, 2, false, domain.SendNone)) != true {
		t.Fatal("React not granted for a durable outbound-capable message")
	}
	if open(flags(false, 2, false, domain.SendNone)) {
		t.Fatal("React granted without chat.CanReact")
	}
	if open(flags(true, 0, false, domain.SendNone)) {
		t.Fatal("React granted to a non-durable message ID")
	}
	if open(flags(true, 2, true, domain.SendNone)) {
		t.Fatal("React granted to a service message")
	}
	if open(flags(true, 2, false, domain.SendPending)) {
		t.Fatal("React granted to a pending message")
	}
	if open(flags(true, 2, false, domain.SendFailed)) {
		t.Fatal("React granted to a failed message")
	}
}

func TestReactMenuActivationVerifiesIdentityAndOpensPicker(t *testing.T) {
	base := func() State {
		state := InitialState()
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9, CanReact: true}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 1 // prove menu identity wins
		state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, CanReact: true, Capabilities: domain.MessageCapabilities{Copy: true}, PreviousFocus: FocusConversation}
		return state
	}
	// updateState mutates in place, so each gating branch rebuilds the base.
	opened := base()
	commands := updateState(&opened, ActionReceived{Action: ReactMessage})
	if len(commands) != 0 {
		t.Fatalf("picker open commands = %d, want 0", len(commands))
	}
	if opened.MessageMenu != nil {
		t.Fatal("message menu retained while reaction picker open")
	}
	if opened.Focus != FocusReactionPicker {
		t.Fatalf("focus = %v, want FocusReactionPicker", opened.Focus)
	}
	if opened.ReactionPicker == nil || opened.ReactionPicker.ChatID != 9 || opened.ReactionPicker.MessageID != 2 || opened.ReactionPicker.Selected != 0 || opened.ReactionPicker.RequestID == 0 {
		t.Fatalf("picker = %#v", opened.ReactionPicker)
	}

	gated := base()
	gated.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, CanReact: false, Capabilities: domain.MessageCapabilities{Copy: true}, PreviousFocus: FocusConversation}
	updateState(&gated, ActionReceived{Action: ReactMessage})
	if gated.ReactionPicker != nil {
		t.Fatal("picker opened without local React capability")
	}
	loading := base()
	loading.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, CanReact: true, Loading: true, Capabilities: domain.MessageCapabilities{Copy: true}}
	updateState(&loading, ActionReceived{Action: ReactMessage})
	if loading.ReactionPicker != nil {
		t.Fatal("picker opened while menu loading")
	}
	missing := base()
	missing.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 99, CanReact: true, Capabilities: domain.MessageCapabilities{Copy: true}}
	updateState(&missing, ActionReceived{Action: ReactMessage})
	if missing.ReactionPicker != nil {
		t.Fatal("picker opened for a missing source identity")
	}
}

func TestReactionPickerNavigationConfirmAndCancel(t *testing.T) {
	state := InitialState()
	state.Focus = FocusReactionPicker
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, Selected: 0, RequestID: 42}

	updateState(&state, ActionReceived{Action: SelectNext})
	next := state
	if next.ReactionPicker == nil || next.ReactionPicker.Selected != 1 {
		t.Fatalf("SelectNext selected %d", next.ReactionPicker.Selected)
	}
	updateState(&next, ActionReceived{Action: SelectPrevious})
	if next.ReactionPicker.Selected != 0 {
		t.Fatalf("SelectPrevious selected %d", next.ReactionPicker.Selected)
	}
	for index := 0; index < len(ReactionPalette); index++ {
		updateState(&next, ActionReceived{Action: SelectNext})
	}
	if next.ReactionPicker.Selected != 0 {
		t.Fatalf("wrap-around selection = %d, want 0", next.ReactionPicker.Selected)
	}

	commands := updateState(&next, ActionReceived{Action: Activate})
	confirmed := next
	if len(commands) != 1 {
		t.Fatalf("confirm commands = %d, want 1", len(commands))
	}
	command, ok := commands[0].(ReactToMessage)
	if !ok || command.RequestID != 42 || command.ChatID != 9 || command.MessageID != 2 || command.Emoji != ReactionPalette[0] {
		t.Fatalf("react command = %#v", commands[0])
	}
	if command.Remove {
		t.Fatal("add-path erroneously removed")
	}
	if confirmed.ReactionPicker == nil {
		t.Fatal("picker closed before the result arrived")
	}

	updateState(&state, ActionReceived{Action: Close})
	cancelled := state
	if cancelled.ReactionPicker != nil || cancelled.Focus != FocusConversation {
		t.Fatalf("cancel result = picker:%#v focus:%v", cancelled.ReactionPicker, cancelled.Focus)
	}
}

func TestReactionPickerToggleAddVsRemoveFromChosen(t *testing.T) {
	base := InitialState()
	base.Focus = FocusReactionPicker
	base.Chats = []domain.Chat{{ID: 9}}
	base.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Reactions: []domain.MessageReaction{{Emoji: ReactionPalette[0], Count: 1, Chosen: true}}}}

	// Selected index 0 (👍) is already chosen -> removal.
	state := base
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 42, Selected: 0}
	commands := updateState(&state, ActionReceived{Action: Activate})
	removeCommand, ok := commands[0].(ReactToMessage)
	if !ok {
		t.Fatalf("command type = %T", commands[0])
	}
	if !removeCommand.Remove || removeCommand.Emoji != ReactionPalette[0] {
		t.Fatalf("chosen emoji command = %#v (want remove %q)", removeCommand, ReactionPalette[0])
	}

	// different emoji (index 1) not chosen -> addition.
	state = base
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 43, Selected: 1}
	commands = updateState(&state, ActionReceived{Action: Activate})
	addCommand := commands[0].(ReactToMessage)
	if addCommand.Remove || addCommand.Emoji != ReactionPalette[1] {
		t.Fatalf("unchosen command = %#v (want add %q)", addCommand, ReactionPalette[1])
	}

	// A different chosen emoji (index 2) but picking index 1 still adds.
	chosenOther := base
	chosenOther.Messages[9][0].Reactions = []domain.MessageReaction{{Emoji: ReactionPalette[2], Count: 1, Chosen: true}}
	chosenOther.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 44, Selected: 1}
	commands = updateState(&chosenOther, ActionReceived{Action: Activate})
	otherAdd := commands[0].(ReactToMessage)
	if otherAdd.Remove || otherAdd.Emoji != ReactionPalette[1] {
		t.Fatalf("unselected chosen emoji made add into remove: %#v", otherAdd)
	}

	// Picked emoji present but explicitly Chosen=false -> addition, not removal.
	chosenFalse := base
	chosenFalse.Messages[9][0].Reactions = []domain.MessageReaction{{Emoji: ReactionPalette[0], Count: 1, Chosen: false}}
	chosenFalse.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 45, Selected: 0}
	commands = updateState(&chosenFalse, ActionReceived{Action: Activate})
	falseAdd := commands[0].(ReactToMessage)
	if falseAdd.Remove || falseAdd.Emoji != ReactionPalette[0] {
		t.Fatalf("unchosen present emoji made add into remove: %#v", falseAdd)
	}
}

func TestReactionPickerIdentityIsExactAndCommandUsesSavedIdentity(t *testing.T) {
	state := InitialState()
	state.Focus = FocusReactionPicker
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 42}

	commands := updateState(&state, ActionReceived{Action: Activate})
	confirmed := state
	command := commands[0].(ReactToMessage)
	if len(confirmed.Messages[9]) != 1 || command.ChatID != 9 || command.MessageID != 2 || command.RequestID != 42 {
		t.Fatalf("picker identity not preserved: %#v", commands[0])
	}
}

func TestReactionSuccessClosesPickerShowsConstantToastWithoutOptimisticMutation(t *testing.T) {
	state := InitialState()
	state.Focus = FocusReactionPicker
	state.Chats = []domain.Chat{{ID: 9}}
	base := []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body", Reactions: []domain.MessageReaction{{Emoji: "👍", Count: 1, Chosen: false}}}}
	state.Messages[9] = append([]domain.Message(nil), base...)
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 5}

	updateState(&state, ReactionChanged{RequestID: 5, ChatID: 9, MessageID: 2, Emoji: "👍", Removed: false})
	added := state
	if added.ReactionPicker != nil || added.Focus != FocusConversation {
		t.Fatalf("success focus = picker:%#v focus:%v", added.ReactionPicker, added.Focus)
	}
	if added.Toast == nil || added.Toast.Message != "Reaction added" || added.Toast.Cause != nil || strings.Contains(added.Toast.Message, "secret-body") || strings.Contains(added.Toast.Message, "9") || strings.Contains(added.Toast.Message, "2") {
		t.Fatalf("add toast = %#v", added.Toast)
	}
	if !reflect.DeepEqual(added.Messages[9][0].Reactions, base[0].Reactions) {
		t.Fatal("success optimistically mutated reaction counts")
	}

	removedState := state
	removedState.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 6}
	updateState(&removedState, ReactionChanged{RequestID: 6, ChatID: 9, MessageID: 2, Emoji: "👍", Removed: true})
	removed := removedState
	if removed.Toast == nil || removed.Toast.Message != "Reaction removed" {
		t.Fatalf("remove toast = %#v", removed.Toast)
	}
}

func TestReactionFailureShowsConstantToastNoCauseAndClosesPicker(t *testing.T) {
	state := InitialState()
	state.Focus = FocusReactionPicker
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "secret-body"}}
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 5}
	updateState(&state, ReactionFailed{RequestID: 5, ChatID: 9, MessageID: 2, Error: domain.AppError{Message: "secret-body 9 2"}})
	failed := state
	if failed.ReactionPicker != nil || failed.Focus != FocusConversation {
		t.Fatalf("failure focus = picker:%#v focus:%v", failed.ReactionPicker, failed.Focus)
	}
	if failed.Toast == nil || failed.Toast.Message != "Reaction failed" || failed.Toast.Cause != nil || strings.Contains(failed.Toast.Message, "secret-body") || strings.Contains(failed.Toast.Message, "9") || strings.Contains(failed.Toast.Message, "2") {
		t.Fatalf("failure toast = %#v", failed.Toast)
	}
	if messageIndex(failed.Messages[9], 2) < 0 {
		t.Fatal("failure removed the message")
	}
}

func TestReactionEventsIgnoreStaleAndIdentityMismatch(t *testing.T) {
	state := InitialState()
	state.Focus = FocusReactionPicker
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
	state.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, RequestID: 42}

	updateState(&state, ReactionChanged{RequestID: 41, ChatID: 9, MessageID: 2, Emoji: "👍"})
	stale := state
	if stale.ReactionPicker == nil || stale.Toast != nil {
		t.Fatal("stale success applied")
	}
	updateState(&state, ReactionFailed{RequestID: 42, ChatID: 10, MessageID: 2, Error: domain.AppError{Message: "raw"}})
	wrongChat := state
	if wrongChat.ReactionPicker == nil || wrongChat.Toast != nil {
		t.Fatal("identity-mismatched failure applied")
	}
	updateState(&state, ReactionChanged{RequestID: 42, ChatID: 9, MessageID: 3, Emoji: "👍"})
	wrongMessage := state
	if wrongMessage.ReactionPicker == nil || wrongMessage.Toast != nil {
		t.Fatal("identity-mismatched success applied")
	}
	updateState(&state, ActionReceived{Action: Close})
	closed := state
	updateState(&closed, ReactionChanged{RequestID: 42, ChatID: 9, MessageID: 2, Emoji: "👍"})
	late := closed
	if late.Toast != nil {
		t.Fatal("late success applied after picker closed")
	}
}

func TestUpdateMessageReactionsReconcilesInPlaceAndIgnoresUnknownIdentity(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText}}
		return state
	}
	reactions := []domain.MessageReaction{{Emoji: "👍", Count: 3, Chosen: true}, {Emoji: "❤️", Count: 1, Chosen: false}}
	got := fixture()
	updateState(&got, TelegramEvent{Value: telegram.MessageReactionsUpdated{ChatID: 9, MessageID: 2, Reactions: reactions}})
	if !reflect.DeepEqual(got.Messages[9][0].Reactions, reactions) {
		t.Fatalf("reactions = %#v, want %#v", got.Messages[9][0].Reactions, reactions)
	}
	pointer := fixture()
	updateState(&pointer, TelegramEvent{Value: &telegram.MessageReactionsUpdated{ChatID: 9, MessageID: 2, Reactions: reactions}})
	if !reflect.DeepEqual(pointer.Messages[9][0].Reactions, reactions) {
		t.Fatal("pointer reactions update not normalized")
	}
	unknown := fixture()
	updateState(&unknown, TelegramEvent{Value: telegram.MessageReactionsUpdated{ChatID: 9, MessageID: 99, Reactions: reactions}})
	if len(unknown.Messages[9][0].Reactions) != 0 {
		t.Fatal("unknown identity reactions applied")
	}
}

func TestMessageReactionsSurviveReUpsertWithEmptySnapshot(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}

	updateState(&state, TelegramEvent{Value: telegram.MessageReactionsUpdated{
		ChatID:    9,
		MessageID: 2,
		Reactions: []domain.MessageReaction{{Emoji: "👍", Count: 3, Chosen: true}},
	}})
	live := state
	if len(live.Messages[9][0].Reactions) != 1 {
		t.Fatalf("live reaction count = %d, want 1", len(live.Messages[9][0].Reactions))
	}

	updateState(&live, TelegramEvent{Value: telegram.MessageUpserted{
		Message: domain.Message{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"},
	}})
	reupserted := live
	if len(reupserted.Messages[9][0].Reactions) != 1 {
		t.Fatal("same-ID re-upsert dropped live-updated reactions")
	}
	if got := reupserted.Messages[9][0].Reactions[0]; got.Emoji != "👍" || got.Count != 3 || !got.Chosen {
		t.Fatalf("retained reaction = %#v, want chosen thumbs-up with count 3", got)
	}
}

func selectedWritableState() State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusComposer
	state.Chats = []domain.Chat{{ID: 9, Title: "Team", CanSend: true}}
	state.SelectedChat = 0
	return state
}

func chatIDs(chats []domain.Chat) []domain.ChatID {
	ids := make([]domain.ChatID, len(chats))
	for index, chat := range chats {
		ids[index] = chat.ID
	}
	return ids
}

func assertCommands(t *testing.T, got, want []Effect) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestReducerReplacesAndClearsMessageMediaOnContentUpdate(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID:         10,
		ChatID:     9,
		Kind:       domain.MessageText,
		Text:       "initial text",
		Sender:     domain.SenderRef{Kind: domain.SenderUser, ID: 41},
		SenderName: "Alice",
		SentAt:     time.Unix(100, 0),
		EditedAt:   time.Time{},
		Outgoing:   false,
		Pinned:     true,
		Reactions:  []domain.MessageReaction{{Emoji: "👍", Count: 1}},
	}}

	// Apply document content update.
	var commands []Effect
	commands = updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID:    9,
		MessageID: 10,
		Kind:      domain.MessageDocument,
		FileName:  "report.pdf",
		Media: domain.MessageMedia{
			File: domain.MediaFileRef{
				ID:           41,
				UniqueID:     "document-main",
				Size:         5000,
				ExpectedSize: 10000,
				LocalPath:    "/tmp/report.pdf",
				CanDownload:  true,
				Downloaded:   false,
			},
			Thumbnail: domain.MediaFileRef{
				ID:       42,
				UniqueID: "document-thumb",
			},
			MIMEType: "application/pdf",
			Width:    0,
			Height:   0,
			Duration: 0,
		},
	}})
	if len(commands) != 0 {
		t.Fatalf("content update should produce no commands, got %d", len(commands))
	}
	msg := state.Messages[9][0]
	expectedMedia := domain.MessageMedia{
		File: domain.MediaFileRef{
			ID:           41,
			UniqueID:     "document-main",
			Size:         5000,
			ExpectedSize: 10000,
			LocalPath:    "/tmp/report.pdf",
			CanDownload:  true,
			Downloaded:   false,
		},
		Thumbnail: domain.MediaFileRef{
			ID:       42,
			UniqueID: "document-thumb",
		},
		MIMEType: "application/pdf",
		Width:    0,
		Height:   0,
		Duration: 0,
	}
	// Exact Kind/Text/FileName assertions.
	if msg.Kind != domain.MessageDocument {
		t.Fatalf("kind = %v, want %v", msg.Kind, domain.MessageDocument)
	}
	if msg.Text != "" {
		t.Fatalf("text should be empty after non-text update: got %q", msg.Text)
	}
	if msg.FileName != "report.pdf" {
		t.Fatalf("fileName = %q, want %q", msg.FileName, "report.pdf")
	}
	// Exact MessageMedia equality.
	if msg.Media != expectedMedia {
		t.Fatalf("Media mismatch: got %#v, want %#v", msg.Media, expectedMedia)
	}
	// Unrelated fields preserved.
	if msg.ID != 10 {
		t.Fatalf("ID = %d, want 10", msg.ID)
	}
	if msg.ChatID != 9 {
		t.Fatalf("ChatID = %d, want 9", msg.ChatID)
	}
	if msg.Sender != (domain.SenderRef{Kind: domain.SenderUser, ID: 41}) {
		t.Fatalf("sender = %#v, want %#v", msg.Sender, domain.SenderRef{Kind: domain.SenderUser, ID: 41})
	}
	if msg.SenderName != "Alice" {
		t.Fatalf("senderName = %q, want Alice", msg.SenderName)
	}
	if msg.SentAt != time.Unix(100, 0) {
		t.Fatalf("sentAt = %v, want %v", msg.SentAt, time.Unix(100, 0))
	}
	if !msg.EditedAt.IsZero() {
		t.Fatalf("editedAt should be zero: %v", msg.EditedAt)
	}
	if msg.Outgoing {
		t.Fatal("outgoing should be false")
	}
	if !msg.Pinned {
		t.Fatal("pinned should be true")
	}
	if !reflect.DeepEqual(msg.Reactions, []domain.MessageReaction{{Emoji: "👍", Count: 1}}) {
		t.Fatalf("reactions = %#v, want []domain.MessageReaction{{Emoji: 👍, Count: 1}}", msg.Reactions)
	}

	// Apply text replacement — should clear FileName and Media.
	commands = updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID:    9,
		MessageID: 10,
		Kind:      domain.MessageText,
		Text:      "now text only",
	}})
	if len(commands) != 0 {
		t.Fatalf("text update should produce no commands, got %d", len(commands))
	}
	msg = state.Messages[9][0]
	if msg.Kind != domain.MessageText {
		t.Fatalf("kind = %v, want %v", msg.Kind, domain.MessageText)
	}
	if msg.Text != "now text only" {
		t.Fatalf("text = %q, want %q", msg.Text, "now text only")
	}
	if msg.FileName != "" {
		t.Fatalf("fileName should be cleared: %q", msg.FileName)
	}
	if msg.Media != (domain.MessageMedia{}) {
		t.Fatalf("media should be zero: %#v", msg.Media)
	}
	// All unrelated fields survive both updates.
	if msg.ID != 10 {
		t.Fatalf("ID = %d, want 10", msg.ID)
	}
	if msg.ChatID != 9 {
		t.Fatalf("ChatID = %d, want 9", msg.ChatID)
	}
	if msg.Sender != (domain.SenderRef{Kind: domain.SenderUser, ID: 41}) {
		t.Fatalf("sender = %#v, want %#v", msg.Sender, domain.SenderRef{Kind: domain.SenderUser, ID: 41})
	}
	if msg.SenderName != "Alice" {
		t.Fatalf("senderName = %q, want Alice", msg.SenderName)
	}
	if msg.SentAt != time.Unix(100, 0) {
		t.Fatalf("sentAt = %v, want %v", msg.SentAt, time.Unix(100, 0))
	}
	if !msg.EditedAt.IsZero() {
		t.Fatalf("editedAt should be zero: %v", msg.EditedAt)
	}
	if msg.Outgoing {
		t.Fatal("outgoing should be false")
	}
	if !msg.Pinned {
		t.Fatal("pinned should be true")
	}
	if !reflect.DeepEqual(msg.Reactions, []domain.MessageReaction{{Emoji: "👍", Count: 1}}) {
		t.Fatalf("reactions = %#v, want []domain.MessageReaction{{Emoji: 👍, Count: 1}}", msg.Reactions)
	}
}

func TestViewImageRowIsFirstWhenMediaFileIsEligible(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
	}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
		MediaFile:    state.Messages[9][0].Media.File,
	}
	count := actionMenuItemCount(state.MessageMenu)
	if count != 3 {
		t.Fatalf("row-count = %d, want 3", count)
	}
	actions := []Action{ViewMessageMedia, ReplyMessage, CopyMessage}
	for index, action := range actions {
		state.MessageMenu.Selected = index
		if selectedMenuAction(state.MessageMenu) != action {
			t.Fatalf("index %d action = %v, want %v", index, selectedMenuAction(state.MessageMenu), action)
		}
	}
}

func TestViewImageRowDoesNotAppearWhenMediaFileIsEmpty(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "plain"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
		MediaFile:    domain.MediaFileRef{},
	}
	count := actionMenuItemCount(state.MessageMenu)
	if count != 2 {
		t.Fatalf("row-count = %d, want 2", count)
	}
	state.MessageMenu.Selected = 0
	if selectedMenuAction(state.MessageMenu) != ReplyMessage {
		t.Fatalf("selected = %v, want ReplyMessage", selectedMenuAction(state.MessageMenu))
	}
}

func TestMediaModalOpensWithDownloadCommandAndSelectionReset(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 10
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
	}}
	state.Modal = nil
	state.MessageMenu = &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
		PreviousFocus: FocusConversation,
	}
	commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
	open := state
	if open.MessageMenu != nil {
		t.Fatal("action-menu was not cleared")
	}
	if open.Modal == nil || !open.Modal.Loading || open.Modal.RequestID != 10 || open.Modal.Title != "Photo" || open.Modal.PreviousFocus != FocusConversation {
		t.Fatalf("modal shell = %#v", open.Modal)
	}
	if open.Focus != FocusModal {
		t.Fatalf("focus = %v, want FocusModal", open.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
	command, ok := commands[0].(OpenMessageMediaFile)
	if !ok || command.RequestID != 10 || command.ChatID != 9 || command.MessageID != 2 || command.File.ID != 101 {
		t.Fatalf("download command = %#v", command)
	}
}

func TestMediaModalOpensImmediatelyWhenAlreadyDownloaded(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/101.jpg"}},
	}}
	state.MessageMenu = &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		MediaFile:     state.Messages[9][0].Media.File,
		PreviousFocus: FocusDetails,
	}
	state.Modal = nil
	commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
	open := state
	if open.MessageMenu != nil {
		t.Fatal("action-menu was not cleared")
	}
	if open.Modal == nil || open.Modal.Loading || open.Modal.RequestID != 0 || open.Modal.Path != "/tmp/101.jpg" {
		t.Fatalf("modal shell = %#v", open.Modal)
	}
	if open.Modal.Title != "Photo" {
		t.Fatalf("title = %s, want Photo", open.Modal.Title)
	}
	if open.Modal.PreviousFocus != FocusDetails {
		t.Fatalf("previous-focus = %v, want FocusDetails", open.Modal.PreviousFocus)
	}
	if len(commands) != 0 {
		t.Fatalf("commands count = %d, want 0", len(commands))
	}
}

func TestMediaModalRetryReusesDownloadMediaCommand(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 21
	state.Focus = FocusModal
	state.Modal = &ModalState{
		RequestID: 20, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
		Loading:   true, Error: nil,
		PreviousFocus: FocusConversation,
	}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "offline"}
	updateState(&state, MessageMediaOpenFailed{RequestID: 20, ChatID: 9, MessageID: 2, Error: failure})
	failed := state
	if failed.Modal.Loading || failed.Modal.Error == nil {
		t.Fatalf("failed modal = %#v", failed.Modal)
	}
	commands := updateState(&failed, ActionReceived{Action: Retry})
	retried := failed
	if retried.Modal.RequestID != 21 || !retried.Modal.Loading || retried.Modal.Error != nil {
		t.Fatalf("retried modal = %#v", retried.Modal)
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
	command, ok := commands[0].(OpenMessageMediaFile)
	if !ok || command.ChatID != 9 || command.MessageID != 2 || command.File.ID != 101 {
		t.Fatalf("retry command = %#v", command)
	}
}

func TestMediaModalClosesAndRestoresFocus(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Modal = &ModalState{PreviousFocus: FocusDetails}
	updateState(&state, ActionReceived{Action: Close})
	closed := state
	if closed.Modal != nil || closed.Focus != FocusDetails {
		t.Fatalf("close result = focus:%v modal:%v", closed.Focus, closed.Modal)
	}
}

func TestMediaModalIdentityMatchesAndRejectsStaleEvents(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 30
	state.Focus = FocusModal
	state.Modal = &ModalState{
		RequestID: 30, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101},
		Loading:   true, PreviousFocus: FocusConversation,
	}
	updateState(&state, MessageMediaOpened{RequestID: 30, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/101.jpg"}})
	matched := state
	if matched.Modal.Loading || matched.Modal.Path != "/tmp/101.jpg" {
		t.Fatalf("matched modal = %#v", matched.Modal)
	}
	// Re-open a fresh loading modal for stale testing
	fresh := state
	fresh.Modal = &ModalState{
		RequestID: 30, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101},
		Loading:   true, PreviousFocus: FocusConversation,
	}
	updateState(&fresh, MessageMediaOpened{RequestID: 29, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	stale := fresh
	if !stale.Modal.Loading {
		t.Fatal("stale event should not have matched")
	}
	updateState(&fresh, MessageMediaOpened{RequestID: 30, ChatID: 8, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	wrongChat := fresh
	if !wrongChat.Modal.Loading {
		t.Fatal("wrong chat ID should not have matched")
	}
	updateState(&fresh, MessageMediaOpened{RequestID: 30, ChatID: 9, MessageID: 3, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	wrongMessage := fresh
	if !wrongMessage.Modal.Loading {
		t.Fatal("wrong message ID should not have matched")
	}
}

func TestMediaModalSuccessUpdatesLocalMessagePath(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 40
	state.Focus = FocusModal
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101}},
	}}
	state.Modal = &ModalState{
		RequestID: 40, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101},
		Loading:   true, PreviousFocus: FocusConversation,
	}
	updateState(&state, MessageMediaOpened{RequestID: 40, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/101.jpg"}})
	updated := state
	if updated.Modal.Loading || updated.Modal.Path != "/tmp/101.jpg" {
		t.Fatalf("modal = %#v", updated.Modal)
	}
	message := updated.Messages[9][0]
	if !message.Media.File.Downloaded || message.Media.File.LocalPath != "/tmp/101.jpg" {
		t.Fatalf("local message = %#v", message.Media.File)
	}
}

func TestMediaModalFailureDoesNotClearMessageMediaRef(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 50
	state.Focus = FocusModal
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101}},
	}}
	state.Modal = &ModalState{
		RequestID: 50, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101},
		Loading:   true, PreviousFocus: FocusConversation,
	}
	failure := domain.AppError{Kind: domain.ErrorMedia, Message: "Could not open image"}
	updateState(&state, MessageMediaOpenFailed{RequestID: 50, ChatID: 9, MessageID: 2, Error: failure})
	failed := state
	if failed.Modal.Loading || failed.Modal.Error == nil {
		t.Fatalf("failed modal = %#v", failed.Modal)
	}
	message := failed.Messages[9][0]
	if message.Media.File.ID != 101 {
		t.Fatalf("media ref was cleared: %#v", message.Media.File)
	}
}

func TestMediaModalMatchesOnUniqueIDWhenIDIsZero(t *testing.T) {
	fixture := func() State {
		state := InitialState()
		state.NextRequestID = 60
		state.Focus = FocusModal
		state.Modal = &ModalState{
			RequestID: 60, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 0, UniqueID: "unique-101"},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	matched := fixture()
	updateState(&matched, MessageMediaOpened{RequestID: 60, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 0, UniqueID: "unique-101", Downloaded: true, LocalPath: "/tmp/101.jpg"}})
	if matched.Modal.Loading || matched.Modal.Path != "/tmp/101.jpg" {
		t.Fatalf("matched modal = %#v", matched.Modal)
	}
	stale := fixture()
	updateState(&stale, MessageMediaOpened{RequestID: 60, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 0, UniqueID: "unique-202"}})
	if !stale.Modal.Loading {
		t.Fatal("different unique ID cleared loading state")
	}
}

func TestMediaModalRetryPreservesIdentityMatchOnUniqueID(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 71
	state.Focus = FocusModal
	state.Modal = &ModalState{
		RequestID: 70, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101, UniqueID: "unique-101", CanDownload: true},
		Loading:   true, Error: nil, PreviousFocus: FocusConversation,
	}
	failure := domain.AppError{Kind: domain.ErrorMedia, Message: "try again"}
	updateState(&state, MessageMediaOpenFailed{RequestID: 70, ChatID: 9, MessageID: 2, Error: failure})
	failed := state
	commands := updateState(&failed, ActionReceived{Action: Retry})
	retried := failed
	if retried.Modal.RequestID != 71 || retried.Modal.Error != nil {
		t.Fatalf("retried modal = %#v", retried.Modal)
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
	command, ok := commands[0].(OpenMessageMediaFile)
	if !ok || command.File.UniqueID != "unique-101" {
		t.Fatalf("retry command = %#v", command)
	}
}

func TestMediaModalSelectionResetsWhenMenuIsClosedViaClose(t *testing.T) {
	state := InitialState()
	state.MessageMenu = &MessageActionMenu{Selected: 1, MediaFile: domain.MediaFileRef{ID: 101}}
	updateState(&state, ActionReceived{Action: Close})
	closed := state
	if closed.MessageMenu != nil {
		t.Fatal("menu was not closed")
	}
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
	}}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: 2, Selected: 0, MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true}, PreviousFocus: FocusConversation}
	state.NextRequestID = 80
	commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
	open := state
	if open.MessageMenu != nil || len(commands) != 1 {
		t.Fatalf("open result = menu:%v commands:%d", open.MessageMenu, len(commands))
	}
}

func TestMessagePhotoFileEligible(t *testing.T) {
	hasPhoto := domain.Message{Kind: domain.MessagePhoto, Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}}}
	if _, ok := messagePhotoFile(hasPhoto); !ok {
		t.Fatal("photo should be eligible")
	}
	noPhoto := domain.Message{Kind: domain.MessageText, Text: "text"}
	if _, ok := messagePhotoFile(noPhoto); ok {
		t.Fatal("text message should not be eligible")
	}
	emptyFile := domain.Message{Kind: domain.MessagePhoto, Media: domain.MessageMedia{File: domain.MediaFileRef{}}}
	if _, ok := messagePhotoFile(emptyFile); ok {
		t.Fatal("photo with empty file should not be eligible")
	}
}

func TestMediaModalMatchesRejectsWhenModalIsNil(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Modal = nil
	updateState(&state, MessageMediaOpened{RequestID: 90, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	matched := state
	if matched.Modal != nil {
		t.Fatal("nil modal should not have changed")
	}
}

func TestMediaModalIdentityMatchesRejectsWrongRequestID(t *testing.T) {
	state := InitialState()
	state.NextRequestID = 100
	state.Focus = FocusModal
	state.Modal = &ModalState{
		RequestID: 100, Title: "Photo",
		MediaChatID: 9, MediaMessageID: 2,
		MediaFile: domain.MediaFileRef{ID: 101},
		Loading:   true, PreviousFocus: FocusConversation,
	}
	updateState(&state, MessageMediaOpened{RequestID: 99, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101, Downloaded: true}})
	result := state
	if !result.Modal.Loading {
		t.Fatal("wrong request ID should not match")
	}
}

func TestMessagePhotoActionOpensReadyModalFromCompletedLocalFile(t *testing.T) {
	// A1: independently build input state and expected state so no shared maps.
	buildReadyState := func() State {
		state := InitialState()
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     state.Messages[9][0].Media.File,
			PreviousFocus: FocusConversation,
		}
		state.Modal = nil
		return state
	}
	buildExpectedReady := func() State {
		state := InitialState()
		state.Focus = FocusModal
		state.NextRequestID = 1 // InitialState sets this to 1
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.Modal = &ModalState{
			RequestID:      0,
			Title:          "Photo",
			MediaChatID:    9,
			MediaMessageID: 2,
			MediaFile:      domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
			Path:           "/tmp/photo.jpg",
			Loading:        false,
			PreviousFocus:  FocusConversation,
		}
		return state
	}

	state := buildReadyState()
	commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
	opened := state

	// A1: full-state no-op against independently built expected.
	want := buildExpectedReady()
	if !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened = %#v, want %#v", opened, want)
	}
	if len(commands) != 0 {
		t.Fatalf("commands count = %d, want 0", len(commands))
	}
	// DeepEqual the full MediaFileRef in modal.
	wantModalFile := domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"}
	if !reflect.DeepEqual(opened.Modal.MediaFile, wantModalFile) {
		t.Fatalf("modal media file = %#v, want %#v", opened.Modal.MediaFile, wantModalFile)
	}
	// DeepEqual the full message media after action.
	wantMsgMedia := domain.MessageMedia{
		File:     domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
		MIMEType: "image/jpeg",
		Width:    640, Height: 480,
		Thumbnail: domain.MediaFileRef{ID: 102},
	}
	if !reflect.DeepEqual(opened.Messages[9][0].Media, wantMsgMedia) {
		t.Fatalf("message media = %#v, want %#v", opened.Messages[9][0].Media, wantMsgMedia)
	}
}

func TestMessagePhotoActionDownloadsAndUsesMenuOwnedIdentity(t *testing.T) {
	// A2: Mutable selected-message proof — selection change happens BEFORE
	// ViewMessageMedia activation so the test proves menu identity, not mutable
	// selection, drives the download command.
	buildDownloadMenuState := func() State {
		state := InitialState()
		state.NextRequestID = 10
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		// Menu-owning message: chat 9 / msg 2 / file 101
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		// Set selection to a DIFFERENT message BEFORE activation
		state.SelectedMessageChat, state.SelectedMessage = 9, 999
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, Text: "different"})
		// Menu owns 9/2/101
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusDetails,
		}
		state.Modal = nil
		return state
	}
	buildExpectedOpened := func() State {
		state := InitialState()
		state.NextRequestID = 11
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{
			{ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
				Media: domain.MessageMedia{
					File:     domain.MediaFileRef{ID: 101, CanDownload: true},
					MIMEType: "image/jpeg",
					Width:    640, Height: 480,
					Thumbnail: domain.MediaFileRef{ID: 102},
				}},
			{ID: 999, ChatID: 9, Kind: domain.MessageText, Text: "different"},
		}
		state.SelectedMessageChat, state.SelectedMessage = 9, 999
		state.Modal = &ModalState{
			RequestID: 10, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusDetails,
		}
		return state
	}

	state := buildDownloadMenuState()
	commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
	opened := state
	want := buildExpectedOpened()
	if !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened = %#v, want %#v", opened, want)
	}
	if opened.MessageMenu != nil {
		t.Fatal("action-menu was not cleared")
	}
	if opened.Modal == nil || !opened.Modal.Loading {
		t.Fatal("loading modal should be opened")
	}
	if opened.Modal.RequestID != 10 {
		t.Fatalf("request ID = %d, want 10", opened.Modal.RequestID)
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
	wantCommands := []Effect{OpenMessageMediaFile{
		RequestID: 10,
		ChatID:    9,
		MessageID: 2,
		Title:     "Photo",
		File:      domain.MediaFileRef{ID: 101, CanDownload: true},
	}}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", commands, wantCommands)
	}

	// A2 continued: Simulate success — proves menu identity, not mutable selection.
	commands2 := updateState(&opened, MessageMediaOpened{
		RequestID: 10, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
	})
	result := opened
	if len(commands2) != 0 {
		t.Fatalf("success should not emit commands, got %d", len(commands2))
	}
	buildExpectedSuccess := func() State {
		state := InitialState()
		state.NextRequestID = 11
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{
			{ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
				Media: domain.MessageMedia{
					File:     domain.MediaFileRef{ID: 101, CanDownload: true, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
					MIMEType: "image/jpeg",
					Width:    640, Height: 480,
					Thumbnail: domain.MediaFileRef{ID: 102},
				}},
			{ID: 999, ChatID: 9, Kind: domain.MessageText, Text: "different"},
		}
		state.SelectedMessageChat, state.SelectedMessage = 9, 999
		state.Modal = &ModalState{
			RequestID: 10, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
			Path:      "/tmp/photo.jpg", Loading: false, PreviousFocus: FocusDetails,
		}
		return state
	}
	wantSuccess := buildExpectedSuccess()
	if !reflect.DeepEqual(result, wantSuccess) {
		t.Fatalf("success result = %#v, want %#v", result, wantSuccess)
	}

	// A3: Identity mismatch — menu owns file 101, current message has file 999
	// with CanDownload=true (both eligible, only identity differs). No-op.
	buildIdentityMismatchState := func() State {
		state := InitialState()
		state.NextRequestID = 200
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 999, CanDownload: true},
				MIMEType: "image/jpeg", Width: 640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 1000},
			}},
		}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		state.Modal = nil
		return state
	}
	buildExpectedIdentityMismatch := func() State {
		state := InitialState()
		state.NextRequestID = 200
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 999, CanDownload: true},
				MIMEType: "image/jpeg", Width: 640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 1000},
			}},
		}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		return state
	}
	no1 := buildIdentityMismatchState()
	cmds1 := updateState(&no1, ActionReceived{Action: ViewMessageMedia})
	want1 := buildExpectedIdentityMismatch()
	if !reflect.DeepEqual(no1, want1) {
		t.Fatalf("identity mismatch = %#v, want %#v (no-op)", no1, want1)
	}
	if len(cmds1) != 0 {
		t.Fatalf("identity mismatch commands = %d, want 0", len(cmds1))
	}

	// A3: Missing message target — menu owns 9/2 but message 2 does not exist.
	buildMissingState := func() State {
		state := InitialState()
		state.NextRequestID = 200
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessagePhoto}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 1
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		state.Modal = nil
		return state
	}
	buildExpectedMissing := func() State {
		state := InitialState()
		state.NextRequestID = 200
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessagePhoto}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 1
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		return state
	}
	no2 := buildMissingState()
	cmds2 := updateState(&no2, ActionReceived{Action: ViewMessageMedia})
	want2 := buildExpectedMissing()
	if !reflect.DeepEqual(no2, want2) {
		t.Fatalf("missing target = %#v, want %#v (no-op)", no2, want2)
	}
	if len(cmds2) != 0 {
		t.Fatalf("missing target commands = %d, want 0", len(cmds2))
	}

	// A3: Same identity changed Photo→Text.
	buildPhotoTextState := func() State {
		state := InitialState()
		state.NextRequestID = 300
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "now text"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		state.Modal = nil
		return state
	}
	buildExpectedPhotoText := func() State {
		state := InitialState()
		state.NextRequestID = 300
		state.Focus = FocusConversation
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "now text"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 2
		state.MessageMenu = &MessageActionMenu{
			ChatID: 9, MessageID: 2, Pinned: false,
			Capabilities:  domain.MessageCapabilities{Copy: true, Reply: true},
			MediaFile:     domain.MediaFileRef{ID: 101, CanDownload: true},
			PreviousFocus: FocusConversation,
		}
		return state
	}
	no3 := buildPhotoTextState()
	cmds3 := updateState(&no3, ActionReceived{Action: ViewMessageMedia})
	want3 := buildExpectedPhotoText()
	if !reflect.DeepEqual(no3, want3) {
		t.Fatalf("photo→text = %#v, want %#v (no-op)", no3, want3)
	}
	if len(cmds3) != 0 {
		t.Fatalf("photo→text commands = %d, want 0", len(cmds3))
	}

	// A4: Valid success exact full payload.
	buildSuccessState := func() State {
		state := InitialState()
		state.NextRequestID = 400
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}, {ID: 10}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 555},
			SenderName: "TestSender", SenderAvatar: domain.AvatarRef{UniqueID: "sender-av"},
			SentAt: time.Unix(12345, 0), EditedAt: time.Unix(12400, 0), FileName: "selfie.jpg",
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg", Width: 1280, Height: 720, Duration: time.Second * 0,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
			Outgoing: true, HasReply: true, ReplyToMessageID: 1, HasForward: true, Pinned: true,
			Reactions: []domain.MessageReaction{{Emoji: "👍", Count: 1, Chosen: true}},
		}}
		state.Messages[10] = []domain.Message{{ID: 100, ChatID: 10, Kind: domain.MessageText, Text: "other"}}
		state.Modal = &ModalState{
			RequestID: 400, Title: "Photo", MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	buildExpectedSuccessResult := func() State {
		state := InitialState()
		state.NextRequestID = 400
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}, {ID: 10}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 555},
			SenderName: "TestSender", SenderAvatar: domain.AvatarRef{UniqueID: "sender-av"},
			SentAt: time.Unix(12345, 0), EditedAt: time.Unix(12400, 0), FileName: "selfie.jpg",
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
				MIMEType: "image/jpeg", Width: 1280, Height: 720, Duration: time.Second * 0,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
			Outgoing: true, HasReply: true, ReplyToMessageID: 1, HasForward: true, Pinned: true,
			Reactions: []domain.MessageReaction{{Emoji: "👍", Count: 1, Chosen: true}},
		}}
		state.Messages[10] = []domain.Message{{ID: 100, ChatID: 10, Kind: domain.MessageText, Text: "other"}}
		state.Modal = &ModalState{
			RequestID: 400, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
			Path:      "/tmp/photo.jpg", Loading: false, PreviousFocus: FocusConversation,
		}
		return state
	}
	successResult := buildSuccessState()
	successCmds := updateState(&successResult, MessageMediaOpened{
		RequestID: 400, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
	})
	wantSuccessResult := buildExpectedSuccessResult()
	if !reflect.DeepEqual(successResult, wantSuccessResult) {
		t.Fatalf("success result = %#v, want %#v", successResult, wantSuccessResult)
	}
	if len(successCmds) != 0 {
		t.Fatalf("success commands = %d, want 0", len(successCmds))
	}
	if len(successResult.Messages[10]) != 1 || successResult.Messages[10][0].Text != "other" {
		t.Fatal("other chat/message changed on success")
	}

	// A5: Invalid success — Downloaded=false + nonempty LocalPath (completed guard).
	buildInvSuccess1 := func() State {
		state := InitialState()
		state.NextRequestID = 500
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
		}}
		state.Modal = &ModalState{
			RequestID: 500, Title: "Photo", MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	buildExpectedInv1 := func() State {
		state := InitialState()
		state.NextRequestID = 500
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
		}}
		state.Modal = &ModalState{
			RequestID: 500, Title: "Photo", MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	noInv1 := buildInvSuccess1()
	invCmds1 := updateState(&noInv1, MessageMediaOpened{
		RequestID: 500, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, Downloaded: false, LocalPath: "/tmp/not-complete.jpg"},
	})
	wantInv1 := buildExpectedInv1()
	if !reflect.DeepEqual(noInv1, wantInv1) {
		t.Fatalf("invalid success 1 = %#v, want %#v (no-op)", noInv1, wantInv1)
	}
	if len(invCmds1) != 0 {
		t.Fatalf("invalid success 1 commands = %d, want 0", len(invCmds1))
	}

	// A5: Invalid success — Downloaded=true + empty LocalPath (nonempty guard).
	buildInvSuccess2 := func() State {
		state := InitialState()
		state.NextRequestID = 501
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
		}}
		state.Modal = &ModalState{
			RequestID: 501, Title: "Photo", MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	buildExpectedInv2 := func() State {
		state := InitialState()
		state.NextRequestID = 501
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 101, CanDownload: true}},
		}}
		state.Modal = &ModalState{
			RequestID: 501, Title: "Photo", MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	noInv2 := buildInvSuccess2()
	invCmds2 := updateState(&noInv2, MessageMediaOpened{
		RequestID: 501, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: ""},
	})
	wantInv2 := buildExpectedInv2()
	if !reflect.DeepEqual(noInv2, wantInv2) {
		t.Fatalf("invalid success 2 = %#v, want %#v (no-op)", noInv2, wantInv2)
	}
	if len(invCmds2) != 0 {
		t.Fatalf("invalid success 2 commands = %d, want 0", len(invCmds2))
	}

}
func TestMessageMediaDownloadSuccessUpdatesOnlyMatchingAvailability(t *testing.T) {
	// A1: independently build input state and expected state so no shared maps.
	buildSuccessState := func() State {
		state := InitialState()
		state.NextRequestID = 110
		state.Focus = FocusModal
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 110, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	buildExpectedSuccess := func() State {
		state := InitialState()
		state.NextRequestID = 110
		state.Focus = FocusModal
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 110, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
			Path:      "/tmp/photo.jpg", Loading: false, PreviousFocus: FocusConversation,
		}
		return state
	}

	state := buildSuccessState()
	cmds := updateState(&state, MessageMediaOpened{
		RequestID: 110, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
	})
	result := state
	want := buildExpectedSuccess()
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
	if len(cmds) != 0 {
		t.Fatalf("success should emit 0 commands, got %d", len(cmds))
	}
	// DeepEqual the full updated message.
	wantMsg := domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{
			File:     domain.MediaFileRef{ID: 101, CanDownload: true, Downloaded: true, LocalPath: "/tmp/photo.jpg"},
			MIMEType: "image/jpeg",
			Width:    640, Height: 480,
			Thumbnail: domain.MediaFileRef{ID: 102},
		},
	}
	if !reflect.DeepEqual(result.Messages[9][0], wantMsg) {
		t.Fatalf("result message = %#v, want %#v", result.Messages[9][0], wantMsg)
	}
}

func TestMessageMediaDownloadFailureRetryAndStaleResults(t *testing.T) {
	// A1: independently build input state and expected failure state.
	buildFailureState := func() State {
		state := InitialState()
		state.NextRequestID = 120
		state.Focus = FocusModal
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 120, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	buildExpectedFailure := func() State {
		state := InitialState()
		state.NextRequestID = 120
		state.Focus = FocusModal
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 120, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   false,
			Error:     &domain.AppError{Kind: domain.ErrorMedia, Message: "unavailable"},
			Path:      "", PreviousFocus: FocusConversation,
		}
		return state
	}
	wantMsg := domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{
			File:     domain.MediaFileRef{ID: 101, CanDownload: true},
			MIMEType: "image/jpeg",
			Width:    640, Height: 480,
			Thumbnail: domain.MediaFileRef{ID: 102},
		},
	}

	state := buildFailureState()
	failure := domain.AppError{Kind: domain.ErrorMedia, Message: "unavailable"}
	cmds := updateState(&state, MessageMediaOpenFailed{
		RequestID: 120, ChatID: 9, MessageID: 2, Error: failure,
	})
	failed := state
	want := buildExpectedFailure()
	if !reflect.DeepEqual(failed, want) {
		t.Fatalf("failure result = %#v, want %#v", failed, want)
	}
	if len(cmds) != 0 {
		t.Fatalf("failure should emit 0 commands, got %d", len(cmds))
	}
	if !reflect.DeepEqual(failed.Messages[9][0], wantMsg) {
		t.Fatalf("failed message = %#v, want %#v", failed.Messages[9][0], wantMsg)
	}

	// A8: Retry: must clear stale path before re-emitting.
	// Build failed media modal independently with Path set.
	buildRetryState := func() State {
		state := buildExpectedFailure()
		state.Modal.Path = "/tmp/stale-before-retry.jpg"
		state.NextRequestID = 200
		return state
	}
	buildExpectedRetried := func() State {
		state := InitialState()
		state.NextRequestID = 201
		state.Focus = FocusModal
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 200, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}

	retried := buildRetryState()
	retryCmds := updateState(&retried, ActionReceived{Action: Retry})
	wantRetried := buildExpectedRetried()
	if !reflect.DeepEqual(retried, wantRetried) {
		t.Fatalf("retried = %#v, want %#v", retried, wantRetried)
	}
	if len(retryCmds) != 1 {
		t.Fatalf("retry should emit 1 command, got %d", len(retryCmds))
	}
	wantRetryCmd := []Effect{OpenMessageMediaFile{
		RequestID: 200, ChatID: 9, MessageID: 2, Title: "Photo",
		File: domain.MediaFileRef{ID: 101, CanDownload: true},
	}}
	if !reflect.DeepEqual(retryCmds, wantRetryCmd) {
		t.Fatalf("retry commands = %#v, want %#v", retryCmds, wantRetryCmd)
	}
	retryCmd, ok := retryCmds[0].(OpenMessageMediaFile)
	if !ok || retryCmd.RequestID != 200 || retryCmd.ChatID != 9 || retryCmd.MessageID != 2 || retryCmd.File.ID != 101 {
		t.Fatalf("retry command = %#v", retryCmd)
	}

	// Stale success matrix — one identity mismatch per case; all must be no-op.
	for _, tc := range []struct {
		name  string
		event Event
	}{
		// Wrong request: RequestID 119, chat 9, message 2, file 101.
		{name: "stale success/wrong request", event: MessageMediaOpened{
			RequestID: 119, ChatID: 9, MessageID: 2, Title: "Photo",
			File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/stale.jpg"},
		}},
		// Wrong chat: RequestID 200, chat 8, message 2, file 101.
		{name: "stale success/wrong chat", event: MessageMediaOpened{
			RequestID: 200, ChatID: 8, MessageID: 2, Title: "Photo",
			File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/stale.jpg"},
		}},
		// Wrong message: RequestID 200, chat 9, message 3, file 101.
		{name: "stale success/wrong message", event: MessageMediaOpened{
			RequestID: 200, ChatID: 9, MessageID: 3, Title: "Photo",
			File: domain.MediaFileRef{ID: 101, Downloaded: true, LocalPath: "/tmp/stale.jpg"},
		}},
		// Wrong file: RequestID 200, chat 9, message 2, file 999.
		{name: "stale success/wrong file", event: MessageMediaOpened{
			RequestID: 200, ChatID: 9, MessageID: 2, Title: "Photo",
			File: domain.MediaFileRef{ID: 999, Downloaded: true, LocalPath: "/tmp/stale.jpg"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Each stale success starts from a freshly built retried fixture;
			// updateState mutates the state it is given.
			got := buildExpectedRetried()
			want := buildExpectedRetried()
			cmds := updateState(&got, tc.event)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s stale success = %#v, want unchanged retried %#v", tc.name, got, want)
			}
			if len(cmds) != 0 {
				t.Fatalf("%s stale success commands = %#v, want none", tc.name, cmds)
			}
		})
	}

	// A7: Stale failure matrix — one coherent base modal (request=120, chat=9, message=2).
	// Each case fires one mismatch only; all must be no-op.
	buildStaleFailureState := func() State {
		state := InitialState()
		state.NextRequestID = 130
		state.Focus = FocusModal
		state.Chats = []domain.Chat{{ID: 9}}
		state.Messages[9] = []domain.Message{{
			ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{
				File:     domain.MediaFileRef{ID: 101, CanDownload: true},
				MIMEType: "image/jpeg",
				Width:    640, Height: 480,
				Thumbnail: domain.MediaFileRef{ID: 102},
			},
		}}
		state.Modal = &ModalState{
			RequestID: 120, Title: "Photo",
			MediaChatID: 9, MediaMessageID: 2,
			MediaFile: domain.MediaFileRef{ID: 101, CanDownload: true},
			Loading:   true, PreviousFocus: FocusConversation,
		}
		return state
	}
	failureErr := domain.AppError{Kind: domain.ErrorMedia, Message: "unavailable"}
	for _, tc := range []struct {
		name  string
		event Event
	}{
		// Wrong request only — should be no-op (Error remains nil).
		{name: "stale failure/wrong request", event: MessageMediaOpenFailed{RequestID: 119, ChatID: 9, MessageID: 2, Error: failureErr}},
		// Wrong chat only — should be no-op.
		{name: "stale failure/wrong chat", event: MessageMediaOpenFailed{RequestID: 120, ChatID: 8, MessageID: 2, Error: failureErr}},
		// Wrong message only — should be no-op.
		{name: "stale failure/wrong message", event: MessageMediaOpenFailed{RequestID: 120, ChatID: 9, MessageID: 3, Error: failureErr}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildStaleFailureState()
			want := buildStaleFailureState()
			cmds := updateState(&got, tc.event)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s stale failure = %#v, want no-op %#v", tc.name, got, want)
			}
			if len(cmds) != 0 {
				t.Fatalf("%s stale failure commands = %#v, want none", tc.name, cmds)
			}
		})
	}
}

// ============================================================
// PhotoSend lifecycle tests
// ============================================================

func TestPhotoSendOpenGuards(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*State)
	}{
		{name: "valid", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
		}},
		{name: "no chat", configure: func(s *State) { s.Chats = nil }},
		{name: "read only", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: false}}
			s.SelectedChat = 0
		}},
		{name: "offline", configure: func(s *State) {
			s.Connection = domain.ConnectionOffline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
		}},
		{name: "edit session", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
			s.EditTarget = &EditTarget{ChatID: 9, MessageID: 1}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := InitialState()
			tc.configure(&state)
			updateState(&state, ActionReceived{Action: OpenPhotoSend})
			result := state
			if tc.name == "valid" {
				if result.PhotoSend == nil {
					t.Fatal("open did not establish PhotoSend")
				}
				if result.PhotoSend.ChatID != 9 {
					t.Fatalf("chatID = %d, want 9", result.PhotoSend.ChatID)
				}
				if result.Focus != FocusPhotoSend {
					t.Fatalf("focus = %v, want FocusPhotoSend", result.Focus)
				}
			} else {
				if result.PhotoSend != nil {
					t.Fatal("open should be no-op when guards fail")
				}
			}
		})
	}
}

func TestPhotoSendInputCancelPreservesDraftReplyAndPendingLedger(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusConversation
	state.Drafts[9] = "draft content"
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 5}
	state.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-7): 100}
	updateState(&state, ActionReceived{Action: OpenPhotoSend})
	updateState(&state, ActionReceived{Action: NoAction, Rune: '界'})
	entered := state
	updateState(&entered, ActionReceived{Action: NoAction, Rune: '\r'})
	updateState(&entered, ActionReceived{Action: NoAction, Rune: '\n'})
	if len(entered.PhotoSend.Input) != 1 || entered.PhotoSend.Input[0] != '界' {
		t.Fatalf("input after unicode+CR/LF = %q", string(entered.PhotoSend.Input))
	}
	updateState(&entered, ActionReceived{Action: ComposerBackspace})
	backed := entered
	if len(backed.PhotoSend.Input) != 0 {
		t.Fatalf("backspace input = %q", string(backed.PhotoSend.Input))
	}
	updateState(&backed, ActionReceived{Action: Close})
	closed := backed
	if closed.PhotoSend != nil {
		t.Fatal("cancel did not close PhotoSend")
	}
	if closed.Focus != FocusConversation {
		t.Fatalf("focus = %v", closed.Focus)
	}
	if closed.Drafts[9] != "draft content" {
		t.Fatalf("draft changed: %q", closed.Drafts[9])
	}
	if closed.ReplyTarget == nil || closed.ReplyTarget.ChatID != 9 {
		t.Fatal("reply target lost")
	}
	if closed.PhotoSendRequests == nil || len(closed.PhotoSendRequests) != 1 {
		t.Fatal("ledger was cleared on close")
	}
}

func TestPhotoSendSubmitExactOptimisticCommand(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Drafts[9] = "  caption  \n"
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 42}
	state.Messages[9] = []domain.Message{{ID: 10, ChatID: 9, Kind: domain.MessageText}}
	state.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-99): 999}
	updateState(&state, ActionReceived{Action: OpenPhotoSend})
	state.PhotoSend.Input = []rune{' ', '/', 'p', 'a', 't', 'h', '/', 'p', 'i', 'c', '.', 'j', 'p', 'g'}
	cmds := updateState(&state, ActionReceived{Action: PhotoSendSubmit, At: now})
	submitted := state
	if submitted.PhotoSend != nil {
		t.Fatal("submit should close PhotoSend")
	}
	if submitted.Focus != FocusConversation {
		t.Fatalf("focus = %v", submitted.Focus)
	}
	if submitted.Drafts[9] != "" {
		t.Fatal("draft not cleared")
	}
	if submitted.ReplyTarget != nil {
		t.Fatal("reply target not cleared")
	}
	if len(cmds) != 2 {
		t.Fatalf("cmds = %d", len(cmds))
	}
	cmd := cmds[0].(SendPhoto)
	if cmd.LocalPath != " /path/pic.jpg" {
		t.Fatalf("path = %q", cmd.LocalPath)
	}
	if cmd.Caption != "  caption  \n" {
		t.Fatalf("caption = %q", cmd.Caption)
	}
	if cmd.ReplyToMessageID != 42 {
		t.Fatalf("replyTo = %d", cmd.ReplyToMessageID)
	}
	if len(submitted.Messages[9]) != 2 {
		t.Fatalf("messages count = %d", len(submitted.Messages[9]))
	}
	msg := submitted.Messages[9][1]
	if msg.Kind != domain.MessagePhoto || !msg.Outgoing || msg.SendState != domain.SendPending {
		t.Fatalf("msg = %#v", msg)
	}
	if msg.Media.File.LocalPath != " /path/pic.jpg" || !msg.Media.File.Downloaded {
		t.Fatalf("media = %#v", msg.Media.File)
	}
	if msg.ReplyToMessageID != 42 || !msg.HasReply {
		t.Fatalf("reply = %#v", msg)
	}
	if len(submitted.PhotoSendRequests) != 2 {
		t.Fatalf("ledger entries = %d, want 2", len(submitted.PhotoSendRequests))
	}
	if submitted.PhotoSendRequests[msg.ID] != cmd.RequestID {
		t.Fatal("new ledger entry missing")
	}
	if submitted.SelectedMessageChat != 9 || submitted.SelectedMessage != msg.ID {
		t.Fatalf("selection = (%d, %d)", submitted.SelectedMessageChat, submitted.SelectedMessage)
	}
	// nil initial ledger must not panic.
	stateNil := InitialState()
	stateNil.Connection = domain.ConnectionOnline
	stateNil.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	stateNil.SelectedChat = 0
	updateState(&stateNil, ActionReceived{Action: OpenPhotoSend})
	stateNil.PhotoSend.Input = []rune{'/'}
	stateNil.PhotoSendRequests = nil
	updateState(&stateNil, ActionReceived{Action: PhotoSendSubmit})
}

func TestPhotoSendInvalidSubmitFullStateNoOp(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*State)
	}{
		{name: "blank path", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
			updateState(s, ActionReceived{Action: OpenPhotoSend})
		}},
		{name: "active chat mismatch", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}, {ID: 10, CanSend: true}}
			s.SelectedChat = 1
			updateState(s, ActionReceived{Action: OpenPhotoSend})
		}},
		{name: "offline", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
			updateState(s, ActionReceived{Action: OpenPhotoSend})
			s.PhotoSend.Input = []rune{'/'}
			s.Connection = domain.ConnectionOffline
		}},
		{name: "read only", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
			updateState(s, ActionReceived{Action: OpenPhotoSend})
			s.PhotoSend.Input = []rune{'/'}
			s.Chats[0].CanSend = false
		}},
		{name: "edit session", configure: func(s *State) {
			s.Connection = domain.ConnectionOnline
			s.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			s.SelectedChat = 0
			updateState(s, ActionReceived{Action: OpenPhotoSend})
			s.PhotoSend.Input = []rune{'/'}
			s.EditTarget = &EditTarget{ChatID: 9, MessageID: 1, Original: "o", Buffer: "b"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := InitialState()
			state.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-7): 100}
			tc.configure(&state)
			unchanged := InitialState()
			unchanged.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-7): 100}
			tc.configure(&unchanged)
			cmds := updateState(&state, ActionReceived{Action: PhotoSendSubmit})
			if !reflect.DeepEqual(state, unchanged) || len(cmds) != 0 {
				t.Fatalf("invalid submit mutated state")
			}
		})
	}
}

func TestPhotoSendQueuedCorrelationAndSourceFallback(t *testing.T) {
	localID := domain.MessageID(-5)
	now := time.Unix(300, 0).UTC()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusConversation
	state.SelectedMessageChat = 9
	state.SelectedMessage = localID
	state.Messages[9] = []domain.Message{{
		ID: localID, ChatID: 9, Kind: domain.MessagePhoto, Text: "caption",
		Outgoing: true, SendState: domain.SendPending,
		SentAt: now, HasReply: true, ReplyToMessageID: 42,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/path", Downloaded: true}},
	}}
	state.PhotoSendRequests = map[domain.MessageID]uint64{localID: 100}
	state.MessageMenu = &MessageActionMenu{ChatID: 9, MessageID: localID}
	queued := domain.Message{
		ID: 500, ChatID: 9, Kind: domain.MessagePhoto, Outgoing: true,
		SendState: domain.SendSucceeded, Text: "",
	}
	updateState(&state, PhotoQueued{RequestID: 100, LocalID: localID, ChatID: 9, Message: queued})
	got := state
	if got.PhotoSend != nil || got.PhotoSendRequests == nil {
		t.Fatal("PhotoSend should be nil")
	}
	if _, ok := got.PhotoSendRequests[localID]; ok {
		t.Fatal("ledger not deleted")
	}
	msg := got.Messages[9][0]
	if msg.ID != 500 {
		t.Fatalf("id = %d", msg.ID)
	}
	if msg.Text != "caption" {
		t.Fatalf("text = %q", msg.Text)
	}
	if msg.ReplyToMessageID != 42 || !msg.HasReply {
		t.Fatalf("reply = %#v", msg)
	}
	if msg.Media.File.LocalPath != "/path" || !msg.Media.File.Downloaded {
		t.Fatalf("media fallback = %#v", msg.Media.File)
	}
	if got.SelectedMessage != 500 {
		t.Fatalf("selected = %d", got.SelectedMessage)
	}
	if got.MessageMenu == nil || got.MessageMenu.MessageID != 500 {
		t.Fatalf("menu = %#v", got.MessageMenu)
	}
	// Nonempty TDLib path wins over original.
	state2 := InitialState()
	state2.Messages[9] = []domain.Message{{
		ID: localID, ChatID: 9, Kind: domain.MessagePhoto, Text: "cap",
		Outgoing: true, SendState: domain.SendPending,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "", Downloaded: false}},
	}}
	state2.PhotoSendRequests = map[domain.MessageID]uint64{localID: 1}
	queued2 := domain.Message{
		ID: 600, ChatID: 9, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/tdlib/photo.jpg", Downloaded: true}},
	}
	updateState(&state2, PhotoQueued{RequestID: 1, LocalID: localID, ChatID: 9, Message: queued2})
	got2 := state2
	msg2 := got2.Messages[9][0]
	if msg2.Media.File.LocalPath != "/tdlib/photo.jpg" {
		t.Fatalf("path not preserved: %q", msg2.Media.File.LocalPath)
	}
}

func TestPhotoSendQueuedInvalidEventsFullStateNoOp(t *testing.T) {
	localID := domain.MessageID(-1)
	tests := []struct {
		name  string
		setup func(*State)
		event PhotoQueued
	}{
		{name: "stale request ID", setup: func(s *State) { s.PhotoSendRequests[localID] = 999 },
			event: PhotoQueued{RequestID: 1, LocalID: localID, ChatID: 9, Message: domain.Message{ID: 100, Kind: domain.MessagePhoto}}},
		{name: "wrong ChatID", setup: func(s *State) { s.PhotoSendRequests[localID] = 1 },
			event: PhotoQueued{RequestID: 1, LocalID: localID, ChatID: 10, Message: domain.Message{ID: 100, Kind: domain.MessagePhoto}}},
		{name: "wrong kind", setup: func(s *State) {
			s.Messages[9] = []domain.Message{{ID: localID, Kind: domain.MessageText, SendState: domain.SendPending}}
			s.PhotoSendRequests[localID] = 1
		}, event: PhotoQueued{RequestID: 1, LocalID: localID, ChatID: 9, Message: domain.Message{ID: 100, Kind: domain.MessagePhoto}}},
		{name: "returned zero ID", setup: func(s *State) { s.PhotoSendRequests[localID] = 1 },
			event: PhotoQueued{RequestID: 1, LocalID: localID, ChatID: 9, Message: domain.Message{Kind: domain.MessagePhoto}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := InitialState()
			state.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-7): 100}
			if tc.setup != nil {
				tc.setup(&state)
			}
			ledger := state.PhotoSendRequests[localID]
			ledgerCount := len(state.PhotoSendRequests)
			messages := append([]domain.Message(nil), state.Messages[9]...)
			cmds := updateState(&state, tc.event)
			if len(cmds) != 0 || len(state.PhotoSendRequests) != ledgerCount || state.PhotoSendRequests[localID] != ledger || !reflect.DeepEqual(state.Messages[9], messages) || state.NextRequestID != 1 {
				t.Fatalf("%s accepted invalid queue result: messages=%#v ledger=%#v effects=%#v", tc.name, state.Messages[9], state.PhotoSendRequests, cmds)
			}
		})
	}
}

func TestPhotoSendQueueFailureCorrelation(t *testing.T) {
	localID := domain.MessageID(-1)
	now := time.Unix(400, 0).UTC()
	state := InitialState()
	state.Messages[9] = []domain.Message{{ID: localID, Kind: domain.MessagePhoto, SendState: domain.SendPending}}
	state.PhotoSendRequests = map[domain.MessageID]uint64{localID: 100}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "network", RetryAfter: 10 * time.Second}
	updateState(&state, PhotoQueueFailed{RequestID: 100, LocalID: localID, ChatID: 9, Error: failure, FailedAt: now})
	failed := state
	msg := failed.Messages[9][0]
	if msg.SendState != domain.SendFailed || msg.Failure == nil || msg.Failure.Kind != domain.ErrorNetwork {
		t.Fatalf("failure state = %#v", msg.Failure)
	}
	if msg.RetryAt != now.Add(10*time.Second) {
		t.Fatalf("retryAt = %v", msg.RetryAt)
	}
	if _, ok := failed.PhotoSendRequests[localID]; ok {
		t.Fatal("ledger not deleted")
	}
	// Stale full no-op.
	state2 := InitialState()
	state2.Messages[9] = []domain.Message{{ID: 1, Kind: domain.MessagePhoto}}
	state2.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-7): 100}
	staleCommands := updateState(&state2, PhotoQueueFailed{RequestID: 99, LocalID: domain.MessageID(-8), ChatID: 9, Error: failure, FailedAt: now})
	if len(staleCommands) != 0 || state2.Messages[9][0].SendState != 0 || state2.Messages[9][0].Failure != nil || state2.PhotoSendRequests[-7] != 100 {
		t.Fatalf("stale failure changed queued state: messages=%#v ledger=%#v effects=%#v", state2.Messages[9], state2.PhotoSendRequests, staleCommands)
	}
}

func TestRetryMessagePhotoExactAndInvalidFullStateNoOp(t *testing.T) {
	now := time.Unix(500, 0).UTC()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 20
	state.SelectedMessage = -5
	state.Messages[9] = []domain.Message{{
		ID: -5, ChatID: 9, Kind: domain.MessagePhoto,
		Text: "caption", Outgoing: true, SendState: domain.SendFailed,
		Failure: &domain.AppError{Kind: domain.ErrorNetwork, Message: "network", RetryAfter: 0},
		Media:   domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/path/to/photo.jpg"}},
		RetryAt: now.Add(-1 * time.Hour),
	}}
	state.PhotoSendRequests = map[domain.MessageID]uint64{}
	cmds := updateState(&state, ActionReceived{Action: Retry, MessageID: -5, At: now})
	got := state
	if len(cmds) != 1 {
		t.Fatalf("cmds = %d", len(cmds))
	}
	cmd := cmds[0].(SendPhoto)
	if cmd.LocalPath != "/path/to/photo.jpg" {
		t.Fatalf("path = %q", cmd.LocalPath)
	}
	if cmd.Caption != "caption" {
		t.Fatalf("caption = %q", cmd.Caption)
	}
	msg := got.Messages[9][0]
	if msg.SendState != domain.SendPending || msg.Failure != nil || !msg.RetryAt.IsZero() {
		t.Fatalf("state = %#v", msg)
	}
	if got.PhotoSendRequests[msg.ID] != cmd.RequestID {
		t.Fatal("photo ledger missing")
	}
	if got.NextRequestID != 21 {
		t.Fatalf("NextRequestID = %d, want 21", got.NextRequestID)
	}
	// Invalid: missing path → full no-op.
	state2 := InitialState()
	state2.Connection = domain.ConnectionOnline
	state2.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state2.SelectedChat = 0
	state2.SelectedMessage = -5
	state2.Messages[9] = []domain.Message{{
		ID: -5, ChatID: 9, Kind: domain.MessagePhoto,
		Outgoing: true, SendState: domain.SendFailed,
		Media:   domain.MessageMedia{File: domain.MediaFileRef{}},
		RetryAt: now.Add(-1 * time.Hour),
	}}
	state2.PhotoSendRequests = map[domain.MessageID]uint64{}
	cmds2 := updateState(&state2, ActionReceived{Action: Retry, MessageID: -5, At: now})
	if len(cmds2) != 0 || state2.Messages[9][0].SendState != domain.SendFailed || !state2.Messages[9][0].RetryAt.Equal(now.Add(-time.Hour)) || len(state2.PhotoSendRequests) != 0 || state2.NextRequestID != 1 {
		t.Fatalf("missing path retry changed message or ledger: message=%#v ledger=%#v effects=%#v", state2.Messages[9][0], state2.PhotoSendRequests, cmds2)
	}
}

func TestTextRetryRegressionAfterPhotoSupport(t *testing.T) {
	now := time.Unix(600, 0).UTC()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	state.SelectedMessage = -1
	state.Messages[9] = []domain.Message{{
		ID: -1, ChatID: 9, Kind: domain.MessageText, Text: "retry me",
		Outgoing: true, SendState: domain.SendFailed,
		RetryAt: now.Add(-1 * time.Hour),
	}}
	state.PhotoSendRequests = map[domain.MessageID]uint64{domain.MessageID(-99): 999}
	cmds := updateState(&state, ActionReceived{Action: Retry, MessageID: -1, At: now})
	got := state
	if len(cmds) != 1 {
		t.Fatalf("cmds = %d", len(cmds))
	}
	cmd := cmds[0].(SendText)
	if cmd.LocalID != -1 || cmd.ChatID != 9 || cmd.Text != "retry me" {
		t.Fatalf("cmd = %#v", cmd)
	}
	if got.NextRequestID != 11 {
		t.Fatalf("NextRequestID = %d", got.NextRequestID)
	}
	if got.PhotoSendRequests == nil || len(got.PhotoSendRequests) != 1 || got.PhotoSendRequests[domain.MessageID(-99)] != 999 {
		t.Fatal("photo ledger leaked")
	}
}

func TestPhotoSendTerminalReconciliationPreservesCaptionReplyAndSource(t *testing.T) {
	now := time.Unix(700, 0).UTC()
	// Success: incoming Photo with empty fields preserves caption/reply/source path.
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{
		ID: -3, ChatID: 9, Kind: domain.MessagePhoto,
		Text: "original-caption", Outgoing: true, SendState: domain.SendFailed,
		SentAt: now,
		Media:  domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/local/photo.jpg", Downloaded: true}},
	}}
	updateState(&state, TelegramEvent{Value: telegram.MessageSendSucceeded{
		OldID:   -3,
		Message: domain.Message{ID: 100, ChatID: 9, Kind: domain.MessagePhoto, SendState: domain.SendSucceeded},
	}})
	result := state
	msg := result.Messages[9][0]
	if msg.Text != "original-caption" {
		t.Fatalf("caption lost: %q", msg.Text)
	}
	if msg.Media.File.LocalPath != "/local/photo.jpg" || !msg.Media.File.Downloaded {
		t.Fatalf("source media lost: %#v", msg.Media.File)
	}
	// Nonempty incoming path wins over original.
	state2 := InitialState()
	state2.Messages[9] = []domain.Message{{
		ID: -4, ChatID: 9, Kind: domain.MessagePhoto, Text: "cap",
		Outgoing: true, SendState: domain.SendFailed,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "", Downloaded: false}},
	}}
	updateState(&state2, TelegramEvent{Value: telegram.MessageSendSucceeded{
		OldID: -4,
		Message: domain.Message{ID: 101, ChatID: 9, Kind: domain.MessagePhoto,
			Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/tdlib/photo.jpg", Downloaded: true}},
		}}})
	result2 := state2
	msg2 := result2.Messages[9][0]
	if msg2.Media.File.LocalPath != "/tdlib/photo.jpg" {
		t.Fatalf("TDLib path not preserved: %q", msg2.Media.File.LocalPath)
	}
	// Fail: incoming Photo with empty fields preserves caption/reply/source path.
	state3 := InitialState()
	state3.Connection = domain.ConnectionOnline
	state3.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state3.SelectedChat = 0
	state3.Messages[9] = []domain.Message{{
		ID: -5, ChatID: 9, Kind: domain.MessagePhoto,
		Text: "fail-caption", Outgoing: true, SendState: domain.SendFailed,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/local/fail.jpg", Downloaded: true}},
	}}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Message: "timeout", RetryAfter: 30 * time.Second}
	updateState(&state3, TelegramEvent{Value: telegram.MessageSendFailed{
		OldID:   -5,
		Message: domain.Message{Kind: domain.MessagePhoto},
		Error:   failure,
	}})
	result3 := state3
	msg3 := result3.Messages[9][0]
	if msg3.Text != "fail-caption" {
		t.Fatalf("caption lost on fail: %q", msg3.Text)
	}
	if msg3.Media.File.LocalPath != "/local/fail.jpg" || !msg3.Media.File.Downloaded {
		t.Fatalf("source media lost on fail: %#v", msg3.Media.File)
	}
}

func TestThumbnailAutoDownloadTrigger(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	message := domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		SentAt: time.Unix(200, 0),
		Media:  domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, UniqueID: "thumb", CanDownload: true, Downloaded: false}},
	}
	commands := updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}, ReceivedAt: time.Unix(300, 0)})
	got := state
	assertCommands(t, commands, []Effect{DownloadThumbnail{RequestID: 1, ChatID: 9, MessageID: 2, File: domain.MediaFileRef{ID: 7, UniqueID: "thumb", CanDownload: true, Downloaded: false}}})
	msg := got.Messages[9][0]
	if msg.Kind != domain.MessagePhoto {
		t.Fatalf("kind = %v, want MessagePhoto", msg.Kind)
	}
	if msg.Media.Thumbnail.CanDownload != true || msg.Media.Thumbnail.Downloaded {
		t.Fatalf("thumbnail state changed unexpectedly: %#v", msg.Media.Thumbnail)
	}
}

func TestThumbnailDispatchWhenAlreadyDownloadedNotRendered(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	message := domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		SentAt: time.Unix(200, 0),
		Media:  domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: true, LocalPath: "/tmp/thumb.jpg"}},
	}
	commands := updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}, ReceivedAt: time.Unix(300, 0)})
	got := state
	// A pre-downloaded thumbnail still needs to be rendered; the handler
	// short-circuits the download and renders the cached file.
	assertCommands(t, commands, []Effect{DownloadThumbnail{RequestID: 1, ChatID: 9, MessageID: 2, File: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: true, LocalPath: "/tmp/thumb.jpg"}}})
	msg := got.Messages[9][0]
	if msg.Media.Thumbnail.Downloaded != true || msg.Media.Thumbnail.LocalPath != "/tmp/thumb.jpg" {
		t.Fatalf("thumbnail state changed unexpectedly: %#v", msg.Media.Thumbnail)
	}
}

func TestThumbnailNoDispatchWhenAlreadyRendered(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.Thumbnails[9] = map[domain.MessageID]thumbnail.Block{
		2: {Text: "x", Width: 20, Height: 8},
	}
	message := domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		SentAt: time.Unix(200, 0),
		Media:  domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}},
	}
	commands := updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}, ReceivedAt: time.Unix(300, 0)})
	assertCommands(t, commands, []Effect{})
}

func TestThumbnailDownloadForAttachments(t *testing.T) {
	// Documents, animations and video notes are file-backed attachments and
	// request a thumbnail when their thumbnail file is reachable. Voice notes
	// never carry a thumbnail and stay silent.
	run := func(t *testing.T, kind domain.MessageKind, thumb domain.MediaFileRef, want int) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 9}}
		state.SelectedChat = 0
		message := domain.Message{
			ID: 2, ChatID: 9, Kind: kind,
			SentAt: time.Unix(200, 0),
			Media:  domain.MessageMedia{Thumbnail: thumb},
		}
		commands := updateState(&state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}, ReceivedAt: time.Unix(300, 0)})
		if len(commands) != want {
			t.Fatalf("kind=%v commands = %#v, want %d", kind, commands, want)
		}
	}
	t.Run("document", func(t *testing.T) {
		run(t, domain.MessageDocument, domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}, 1)
	})
	t.Run("animation", func(t *testing.T) {
		run(t, domain.MessageAnimation, domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}, 1)
	})
	t.Run("videoNote", func(t *testing.T) {
		run(t, domain.MessageVideoNote, domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}, 1)
	})
	t.Run("voiceNoteNoThumbnail", func(t *testing.T) {
		run(t, domain.MessageVoiceNote, domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}, 0)
	})
}

func TestThumbnailDownloadedUpdatesMessage(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto, SentAt: time.Unix(200, 0),
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}},
	}}
	commands := updateState(&state, ThumbnailDownloaded{RequestID: 1, ChatID: 9, MessageID: 2, File: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: true, LocalPath: "/tmp/thumb-dl.jpg"}})
	updated := state
	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	thumb := updated.Messages[9][0].Media.Thumbnail
	if !thumb.Downloaded || thumb.LocalPath != "/tmp/thumb-dl.jpg" {
		t.Fatalf("thumbnail = %#v, want downloaded with path", thumb)
	}
}

func TestThumbnailDownloadFailedIsSilent(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto, SentAt: time.Unix(200, 0),
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}},
	}}
	commands := updateState(&state, ThumbnailDownloadFailed{RequestID: 1, ChatID: 9, MessageID: 2, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "download thumbnail", Message: "failed"}})
	updated := state
	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	thumb := updated.Messages[9][0].Media.Thumbnail
	if thumb.Downloaded || thumb.LocalPath != "" {
		t.Fatalf("failed download mutated thumbnail: %#v", thumb)
	}
}

func TestMessageContentUpdatedPreservesDownloadedThumbnailState(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto, SentAt: time.Unix(200, 0),
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, Downloaded: true, LocalPath: "/tmp/thumb.jpg"}},
	}}
	updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID: 9, MessageID: 2, Kind: domain.MessagePhoto,
		// TDLib re-emits a pre-download Media with the same thumbnail identity.
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true, Downloaded: false}},
	}})
	updated := state
	thumb := updated.Messages[9][0].Media.Thumbnail
	if !thumb.Downloaded || thumb.LocalPath != "/tmp/thumb.jpg" {
		t.Fatalf("downloaded thumbnail state not preserved across content update: %#v", thumb)
	}
}

// TestMessageContentUpdatedRequestsMaterializedThumbnail asserts that a
// MessageContentUpdated whose media populates a previously absent, downloadable
// thumbnail issues a DownloadThumbnail command. This is what makes an inline
// photo preview appear for a message (e.g. a sent photo reply) whose thumbnail
// file ref is only reported by TDLib after the initial upsert.
func TestMessageContentUpdatedRequestsMaterializedThumbnail(t *testing.T) {
	state := reduceThumbnailTestState()
	// Initial message: a photo with no thumbnail file ref yet.
	state.Messages[9][0] = domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		SenderName: "Mina", SentAt: time.Unix(200, 0),
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{}},
	}

	commands := updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID: 9, MessageID: 2, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 42, CanDownload: true}},
	}})

	var dl *DownloadThumbnail
	for index := range commands {
		if d, ok := commands[index].(DownloadThumbnail); ok {
			dl = &d
		}
	}
	if dl == nil {
		t.Fatalf("no DownloadThumbnail command emitted, got %v", commands)
	}
	if dl.ChatID != 9 || dl.MessageID != 2 || dl.File.ID != 42 {
		t.Fatalf("DownloadThumbnail = %#v", dl)
	}
}

// TestMessageContentUpdatedSkipsAlreadyRenderedThumbnail asserts that a content
// update does not re-request a thumbnail that already has a rendered block.
func TestMessageContentUpdatedSkipsAlreadyRenderedThumbnail(t *testing.T) {
	state := reduceThumbnailTestState()
	state.Messages[9][0] = domain.Message{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto,
		SenderName: "Mina", SentAt: time.Unix(200, 0),
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 42, CanDownload: true}},
	}
	// Block already rendered for this message.
	state.Thumbnails[9][2] = thumbnail.Block{Text: "kitty", Width: 20, Height: 8}

	commands := updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID: 9, MessageID: 2, Kind: domain.MessagePhoto,
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 42, CanDownload: true}},
	}})
	for index := range commands {
		if _, ok := commands[index].(DownloadThumbnail); ok {
			t.Fatalf("re-requested an already-rendered thumbnail: %v", commands)
		}
	}
}

func reduceThumbnailTestState() State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessagePhoto, SenderName: "Mina", SentAt: time.Unix(200, 0)}}
	state.ChatsLoaded = true
	state.Thumbnails[9] = make(map[domain.MessageID]thumbnail.Block)
	return state
}

func TestThumbnailRenderedStoresBlock(t *testing.T) {
	state := InitialState()
	block := thumbnail.Block{Text: "\u2584\u2580", Width: 20, Height: 8}
	chatID := domain.ChatID(42)
	msgID := domain.MessageID(7)
	updateState(&state, ThumbnailRendered{RequestID: 1, ChatID: chatID, MessageID: msgID, Block: block})
	updated := state
	got, ok := updated.Thumbnails[chatID][msgID]
	if !ok {
		t.Fatalf("missing block for chat %d msg %d", chatID, msgID)
	}
	if got != block {
		t.Fatalf("block = %#v, want %#v", got, block)
	}
}

func TestThumbnailRenderedNilMapInitialized(t *testing.T) {
	state := InitialState()
	state.Thumbnails = nil
	chatID := domain.ChatID(10)
	msgID := domain.MessageID(20)
	block := thumbnail.Block{Text: "x", Width: 20, Height: 8}
	updateState(&state, ThumbnailRendered{RequestID: 2, ChatID: chatID, MessageID: msgID, Block: block})
	updated := state
	if updated.Thumbnails == nil {
		t.Fatal("Thumbnails map is nil after reducing ThumbnailRendered")
	}
	got, ok := updated.Thumbnails[chatID][msgID]
	if !ok {
		t.Fatal("missing block for chat/msg")
	}
	if got != block {
		t.Fatalf("block = %#v, want %#v", got, block)
	}
}

func TestAttachmentOpenAcceptance_CorrelationToastsAndStaleGuards(t *testing.T) {
	file := domain.MediaFileRef{ID: 801, UniqueID: "attachment-main", CanDownload: true}
	cases := []struct {
		kind  domain.MessageKind
		title string
	}{
		{domain.MessageDocument, "File"},
		{domain.MessageAnimation, "Animation"},
		{domain.MessageVoiceNote, "Voice note"},
		{domain.MessageVideoNote, "Video note"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			state := InitialState()
			state.Focus = FocusConversation
			state.Connection = domain.ConnectionOnline
			state.NextRequestID = 40
			state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
			state.Messages[9] = []domain.Message{{
				ID: 77, ChatID: 9, Kind: tc.kind,
				FileName: "attachment.bin", Media: domain.MessageMedia{File: file, MIMEType: "application/octet-stream"},
			}}
			state.SelectedMessageChat, state.SelectedMessage = 9, 77

			updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
			menuState := state
			if menuState.MessageMenu == nil || !reflect.DeepEqual(menuState.MessageMenu.MediaFile, file) || menuState.MessageMenu.MediaKind != tc.kind {
				t.Fatalf("menu = %#v, want eligible main file with kind %v", menuState.MessageMenu, tc.kind)
			}
			commands := updateState(&menuState, ActionReceived{Action: ViewMessageMedia})
			started := menuState
			if len(commands) != 1 {
				t.Fatalf("commands = %#v, want one OpenMessageMediaFile", commands)
			}
			openCommand, ok := commands[0].(OpenMessageMediaFile)
			if !ok || openCommand.RequestID != 41 || openCommand.ChatID != 9 || openCommand.MessageID != 77 || openCommand.Title != tc.title || !reflect.DeepEqual(openCommand.File, file) {
				t.Fatalf("open command = %#v", commands[0])
			}
			pendingKey := attachmentOpenKey{ChatID: 9, MessageID: 77}
			pending, pendingOK := started.AttachmentOpenPending[pendingKey]
			if started.Modal != nil || started.MessageMenu != nil || !pendingOK || pending.RequestID != 41 || pending.Kind != tc.kind || pending.Title != tc.title || !reflect.DeepEqual(pending.File, file) {
				t.Fatalf("started = modal:%#v menu:%#v pending:%#v", started.Modal, started.MessageMenu, started.AttachmentOpenPending)
			}
			if started.Toast == nil || started.Toast.Message != "Opening "+strings.ToLower(tc.title)+"…" {
				t.Fatalf("opening toast = %#v", started.Toast)
			}

			duplicateCommands := updateState(&started, ActionReceived{Action: ViewMessageMedia})
			if len(duplicateCommands) != 0 || started.AttachmentOpenPending[pendingKey].RequestID != 41 || started.MessageMenu != nil || started.Toast == nil || started.Toast.Message != "Opening "+strings.ToLower(tc.title)+"…" {
				t.Fatalf("duplicate activation changed pending open: state=%#v effects=%#v", started, duplicateCommands)
			}

			openedFile := domain.MediaFileRef{ID: 801, UniqueID: "attachment-main", Downloaded: true, LocalPath: "/tmp/attachment.bin"}
			staleEvents := []Event{
				MessageMediaOpened{RequestID: 40, ChatID: 9, MessageID: 77, Title: tc.title, File: openedFile},
				MessageMediaOpened{RequestID: 41, ChatID: 8, MessageID: 77, Title: tc.title, File: openedFile},
				MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 78, Title: tc.title, File: openedFile},
				MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: tc.title, File: domain.MediaFileRef{ID: 999, Downloaded: true, LocalPath: "/tmp/other.bin"}},
				MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: openedFile},
				MessageMediaOpenFailed{RequestID: 40, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Message: "Could not open file"}},
			}
			for _, stale := range staleEvents {
				staleCommands := updateState(&started, stale)
				if len(staleCommands) != 0 || started.AttachmentOpenPending[pendingKey].RequestID != 41 || started.Messages[9][0].Media.File != file || started.Toast == nil || started.Toast.Message != "Opening "+strings.ToLower(tc.title)+"…" {
					t.Fatalf("stale %T changed pending open: state=%#v effects=%#v", stale, started, staleCommands)
				}
			}

			successCommands := updateState(&started, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: tc.title, File: openedFile})
			succeeded := started
			_, successPending := succeeded.AttachmentOpenPending[pendingKey]
			if len(successCommands) != 0 || !reflect.DeepEqual(succeeded.Messages[9][0].Media.File, openedFile) || successPending {
				t.Fatalf("success = file:%#v pending:%#v commands:%#v", succeeded.Messages[9][0].Media.File, succeeded.AttachmentOpenPending, successCommands)
			}
			if succeeded.Toast == nil || succeeded.Toast.Message != tc.title+" opened" {
				t.Fatalf("success toast = %#v", succeeded.Toast)
			}

			updateState(&succeeded, ActionReceived{Action: OpenMessageActionMenu})
			retryMenu := succeeded
			reopenedCommands := updateState(&retryMenu, ActionReceived{Action: ViewMessageMedia})
			reopened := retryMenu
			if len(reopenedCommands) != 1 {
				t.Fatalf("reopen commands = %#v, want one", reopenedCommands)
			}
			failedCommands := updateState(&reopened, MessageMediaOpenFailed{RequestID: 43, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorInternal, Message: "private /tmp/path raw failure", Cause: errors.New("private cause")}})
			failed := reopened
			_, failurePending := failed.AttachmentOpenPending[pendingKey]
			if len(failedCommands) != 0 || failurePending {
				t.Fatalf("failure = commands:%#v pending:%#v", failedCommands, failed.AttachmentOpenPending)
			}
			if failed.Toast == nil || failed.Toast.Kind != domain.ErrorMedia || failed.Toast.Message != "Could not open "+strings.ToLower(tc.title) || failed.Toast.Cause != nil {
				t.Fatalf("failure toast = %#v", failed.Toast)
			}
			updateState(&failed, ActionReceived{Action: OpenMessageActionMenu})
			retryMenu2 := failed
			if retryMenu2.MessageMenu == nil || !reflect.DeepEqual(retryMenu2.MessageMenu.MediaFile, openedFile) {
				t.Fatalf("failure did not restore open action: %#v", retryMenu2.MessageMenu)
			}
		})
	}
}
