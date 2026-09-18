package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func pinnedBaseState() State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9, Title: "chat"}}
	state.SelectedChat = 0
	state.SelectedMessageChat = 9
	state.SelectedMessage = 100
	state.NextRequestID = 10
	state.Drafts[9] = "keep draft"
	state.Messages[9] = []domain.Message{
		{ID: 90, ChatID: 9, Kind: domain.MessageText, Text: "old", SentAt: time.Unix(90, 0)},
		{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "new", SentAt: time.Unix(100, 0)},
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", SentAt: time.Unix(200, 0), Pinned: true},
		{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", SentAt: time.Unix(300, 0), Pinned: true},
	}
	return state
}

func openPinned(t *testing.T, state State) State {
	t.Helper()
	opened, commands := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	if opened.PinnedMessages == nil || opened.PinnedMessages.ChatID != 9 || opened.Focus != FocusPinnedResults {
		t.Fatalf("open = %#v %#v", opened.PinnedMessages, opened.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands count = %d, want 1", len(commands))
	}
	cmd, ok := commands[0].(LoadPinnedMessages)
	if !ok || cmd.ChatID != 9 || cmd.RequestID != 10 {
		t.Fatalf("open command = %#v", cmd)
	}
	return opened
}

func TestOpenPinnedRequiresConversationFocus(t *testing.T) {
	state := pinnedBaseState()
	state.Focus = FocusChats
	unchanged, commands := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	if unchanged.PinnedMessages != nil || len(commands) != 0 {
		t.Fatalf("open from chats = %#v %#v", unchanged.PinnedMessages, commands)
	}
	opened := openPinned(t, pinnedBaseState())
	if opened.PinnedMessages.PreviousFocus != FocusConversation {
		t.Fatalf("previous focus = %v", opened.PinnedMessages.PreviousFocus)
	}
}

func TestOpenPinnedAllocatesRequestIDAndSetsLoading(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 42
	opened, commands := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	if opened.PinnedMessages == nil {
		t.Fatal("pinned messages not opened")
	}
	if opened.PinnedMessages.RequestID != 42 {
		t.Fatalf("request ID = %d, want 42", opened.PinnedMessages.RequestID)
	}
	if !opened.PinnedMessages.Loading {
		t.Fatal("loading should be true")
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
}

func TestPinnedMessagesLoadedAppendsAndDeduplicates(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 100
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	// Simulate first page load.
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned3", SentAt: time.Unix(500, 0)},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "pinned4", SentAt: time.Unix(400, 0)},
		},
		NextFromMessageID: 350,
		TotalCount:        5,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 100, ChatID: 9, Page: first})
	if len(loaded.PinnedMessages.Results) != 2 {
		t.Fatalf("first page results count = %d, want 2", len(loaded.PinnedMessages.Results))
	}
	if loaded.PinnedMessages.Results[0].ID != 500 || loaded.PinnedMessages.Results[1].ID != 400 {
		t.Fatalf("first page order = [%d, %d], want [500, 400]",
			loaded.PinnedMessages.Results[0].ID, loaded.PinnedMessages.Results[1].ID)
	}
	if loaded.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", loaded.PinnedMessages.Selected)
	}
	if loaded.PinnedMessages.Done {
		t.Fatal("done should be false after first page")
	}
	if loaded.PinnedMessages.NextFromMessageID != 350 {
		t.Fatalf("nextFromMessageID = %d, want 350", loaded.PinnedMessages.NextFromMessageID)
	}
	// Second page with duplicate + new message.
	second := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "pinned4-dup"},
			{ID: 350, ChatID: 9, Kind: domain.MessageText, Text: "pinned5", SentAt: time.Unix(350, 0)},
		},
		NextFromMessageID: 0,
		TotalCount:        5,
		Done:              true,
	}
	paged, cmds := Reduce(loaded, PinnedMessagesLoaded{RequestID: 100, ChatID: 9, Page: second})
	if len(paged.PinnedMessages.Results) != 3 {
		t.Fatalf("second page results count = %d, want 3", len(paged.PinnedMessages.Results))
	}
	if paged.PinnedMessages.Results[2].ID != 350 {
		t.Fatalf("third result ID = %d, want 350", paged.PinnedMessages.Results[2].ID)
	}
	if !paged.PinnedMessages.Done {
		t.Fatal("done should be true after second page")
	}
	if len(cmds) != 0 {
		t.Fatalf("unexpected commands: %#v", cmds)
	}
}

func TestPinnedMessagesStaleRequestIgnored(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages:   []domain.Message{{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned"}},
		TotalCount: 1,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Stale request ID should be ignored.
	stale, staleCmds := Reduce(loaded, PinnedMessagesLoaded{RequestID: 11, ChatID: 9, Page: first})
	if !reflect.DeepEqual(stale, loaded) {
		t.Fatalf("stale request mutated state")
	}
	if len(staleCmds) != 0 {
		t.Fatalf("unexpected stale commands: %#v", staleCmds)
	}
	// Wrong chat should also be ignored.
	wrongChat, wrongCmds := Reduce(loaded, PinnedMessagesLoaded{RequestID: 10, ChatID: 8, Page: first})
	if !reflect.DeepEqual(wrongChat, loaded) {
		t.Fatalf("wrong chat mutated state")
	}
	if len(wrongCmds) != 0 {
		t.Fatalf("unexpected wrong-chat commands: %#v", wrongCmds)
	}
}

func TestPinnedMessagesJumpMessageGuard(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned"},
	}}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Simulate a jump in progress.
	loaded.PinnedMessages.JumpMessageID = 500
	loaded.PinnedMessages.Loading = true
	// New search results during a jump should be ignored.
	stale, _ := Reduce(loaded, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	if !reflect.DeepEqual(stale, loaded) {
		t.Fatalf("jump in progress should ignore new results")
	}
}

func TestPinnedMessagesSelectMessageSetsSelected(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "b"},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "c"},
		},
		TotalCount: 3,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Select middle message.
	selected, _ := Reduce(loaded, ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 400})
	if selected.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1", selected.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesSelectNextPaginates(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "b"},
		},
		NextFromMessageID: 300,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Move to last row to trigger pagination.
	paged, cmds := Reduce(loaded, ActionReceived{Action: SelectNext})
	if paged.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1", paged.PinnedMessages.Selected)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected pagination command, got %#v", cmds)
	}
	pagCmd, ok := cmds[0].(LoadPinnedMessages)
	if !ok || pagCmd.Cursor.FromMessageID != 300 || pagCmd.RequestID != 11 {
		t.Fatalf("pagination command = %#v", cmds[0])
	}
	if !paged.PinnedMessages.Loading {
		t.Fatal("should be loading after pagination trigger")
	}
}

func TestPinnedMessagesFailureIsFixedAndSafe(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	failed, _ := Reduce(opened, PinnedMessagesLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "x", Message: "raw"}})
	if failed.PinnedMessages.Loading {
		t.Fatal("should not be loading after failure")
	}
	if failed.PinnedMessages.Error == nil {
		t.Fatal("error should be set")
	}
	if failed.PinnedMessages.Error.Message != "Could not load pinned messages" {
		t.Fatalf("error message = %q, want safe message", failed.PinnedMessages.Error.Message)
	}
	if failed.Focus != FocusPinnedResults {
		t.Fatalf("focus = %v, want FocusPinnedResults", failed.Focus)
	}
	// Stale failure should be ignored.
	before := cloneReducerState(failed)
	after, afterCmds := Reduce(failed, PinnedMessagesLoadFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(afterCmds) != 0 || !reflect.DeepEqual(after, before) {
		t.Fatal("stale failure mutated state")
	}
}

func TestPinnedMessagesActivateLoadsContext(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages:   []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target"}},
		TotalCount: 1,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	jumping, cmds := Reduce(loaded, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	ctxCmd, ok := cmds[0].(LoadPinnedMessageContext)
	if !ok || ctxCmd.ChatID != 9 || ctxCmd.MessageID != 200 || ctxCmd.RequestID != 11 {
		t.Fatalf("context command = %#v", cmds[0])
	}
	if jumping.PinnedMessages.JumpMessageID != 200 {
		t.Fatalf("jumpMessageID = %d, want 200", jumping.PinnedMessages.JumpMessageID)
	}
	if !jumping.PinnedMessages.Loading {
		t.Fatal("should be loading during context request")
	}
}

func TestPinnedMessageContextLoadedClearsOverlayAndRestoresFocus(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target", SentAt: time.Unix(200, 0)},
	}}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	jumping, cmds := Reduce(loaded, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	ctxPage := telegram.MessagePage{Messages: []domain.Message{
		{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "old", SentAt: time.Unix(100, 0)},
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target", SentAt: time.Unix(200, 0)},
	}}
	landed, _ := Reduce(jumping, PinnedMessageContextLoaded{
		RequestID: jumping.PinnedMessages.RequestID, ChatID: 9, MessageID: 200, Page: ctxPage,
	})
	if landed.PinnedMessages != nil {
		t.Fatalf("pinned should be nil after context load: %#v", landed.PinnedMessages)
	}
	if landed.Focus != FocusConversation {
		t.Fatalf("focus = %v, want FocusConversation", landed.Focus)
	}
	if landed.SelectedMessage != 200 || landed.SelectedMessageChat != 9 {
		t.Fatalf("selection = chat=%v msg=%d", landed.SelectedMessageChat, landed.SelectedMessage)
	}
	if landed.Drafts[9] != "keep draft" {
		t.Fatal("draft lost after context load")
	}
}

func TestPinnedMessageContextFailedRestoresOverlayWithToast(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target"},
	}}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	jumping, cmds := Reduce(loaded, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	failed, _ := Reduce(jumping, PinnedMessageContextFailed{
		RequestID: jumping.PinnedMessages.RequestID, ChatID: 9, MessageID: 200,
		Error: domain.AppError{Message: "raw"},
	})
	if failed.PinnedMessages == nil {
		t.Fatal("pinned should be present after context failure")
	}
	if failed.PinnedMessages.JumpMessageID != 0 {
		t.Fatalf("jumpMessageID = %d, want 0", failed.PinnedMessages.JumpMessageID)
	}
	if failed.PinnedMessages.Error == nil || failed.PinnedMessages.Error.Message != "Could not open pinned message" {
		t.Fatalf("error = %#v", failed.PinnedMessages.Error)
	}
	if failed.Toast == nil {
		t.Fatal("toast should be set on context failure")
	}
	if failed.Toast.Message != "Could not open pinned message" {
		t.Fatalf("toast message = %q", failed.Toast.Message)
	}
}

func TestPinnedMessagesCloseRestoresFocus(t *testing.T) {
	opened := openPinned(t, pinnedBaseState())
	closed, _ := Reduce(opened, ActionReceived{Action: Close})
	if closed.PinnedMessages != nil {
		t.Fatalf("pinned should be nil: %#v", closed.PinnedMessages)
	}
	if closed.Focus != FocusConversation {
		t.Fatalf("focus = %v, want FocusConversation", closed.Focus)
	}
	if closed.Drafts[9] != "keep draft" {
		t.Fatal("draft lost on close")
	}
}

func TestPinnedMessagesLivePinUpdatesResults(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", SentAt: time.Unix(200, 0)},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// New pin arrives for message 400.
	loaded.Messages[9] = append(loaded.Messages[9],
		domain.Message{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "newPinned", SentAt: time.Unix(400, 0)})
	pinned, _ := Reduce(loaded, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 400, Pinned: true}})
	if len(pinned.PinnedMessages.Results) != 3 {
		t.Fatalf("results count = %d, want 3", len(pinned.PinnedMessages.Results))
	}
	if pinned.PinnedMessages.Results[0].ID != 400 {
		t.Fatalf("first result = %d, want 400", pinned.PinnedMessages.Results[0].ID)
	}
}

func TestPinnedMessagesLiveUnpinRemovesResult(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", SentAt: time.Unix(200, 0)},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Unpin message 200.
	unpinned, _ := Reduce(loaded, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(unpinned.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(unpinned.PinnedMessages.Results))
	}
	if unpinned.PinnedMessages.Results[0].ID != 300 {
		t.Fatalf("remaining result = %d, want 300", unpinned.PinnedMessages.Results[0].ID)
	}
	if unpinned.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", unpinned.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesLiveUnpinLastResultSetsSelectedZero(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned", SentAt: time.Unix(200, 0)},
		},
		TotalCount: 1,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Unpin the only result.
	unpinned, _ := Reduce(loaded, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(unpinned.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(unpinned.PinnedMessages.Results))
	}
	if unpinned.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0 (not -1)", unpinned.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesCloneDoesNotAlias(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	opened, _ := Reduce(state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages:   []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"}},
		TotalCount: 1,
	}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	cloned := cloneReducerState(loaded)
	cloned.PinnedMessages.Results[0].Text = "modified"
	cloned.PinnedMessages.Results = append(cloned.PinnedMessages.Results,
		domain.Message{ID: 999, ChatID: 9, Kind: domain.MessageText, Text: "new"})
	if loaded.PinnedMessages.Results[0].Text != "a" {
		t.Fatal("clone aliased message text")
	}
	if len(loaded.PinnedMessages.Results) != 1 {
		t.Fatalf("clone aliased results length: %d", len(loaded.PinnedMessages.Results))
	}
}

func TestPinnedMessagesSelectChatCleared(t *testing.T) {
	opened := openPinned(t, pinnedBaseState())
	first := telegram.MessageSearchPage{Messages: []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"}}}
	loaded, _ := Reduce(opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	loaded.Chats = append(loaded.Chats, domain.Chat{ID: 8, Title: "other"})
	switched, cmds := Reduce(loaded, ActionReceived{Action: SelectChat, ChatID: 8})
	if switched.PinnedMessages != nil {
		t.Fatalf("pinned should be cleared: %#v", switched.PinnedMessages)
	}
	found := false
	for _, cmd := range cmds {
		if open, ok := cmd.(OpenChatCommand); ok && open.ChatID == 8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("select chat commands = %#v", cmds)
	}
}

func TestHandlerLoadPinnedMessagesExactRequestAndSafeFailure(t *testing.T) {
	client := &handlerClient{searchPage: telegram.MessageSearchPage{
		Messages:          []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned"}},
		NextFromMessageID: 100,
		TotalCount:        1,
	}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadPinnedMessages{
		RequestID: 42, ChatID: 9, Cursor: telegram.MessageSearchCursor{FromMessageID: 50, Limit: 25},
	})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	pinned, ok := events[0].(PinnedMessagesLoaded)
	if !ok || pinned.RequestID != 42 || pinned.ChatID != 9 || len(pinned.Page.Messages) != 1 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.searchChat != 9 || client.searchCursor.FromMessageID != 50 || client.searchCursor.Limit != 25 {
		t.Fatalf("pinned search call = %v %#v", client.searchChat, client.searchCursor)
	}
	// Safe failure.
	client.err = errors.New("secret pinned detail")
	events = collectHandlerEvents(handler, LoadPinnedMessages{RequestID: 43, ChatID: 9, Cursor: telegram.MessageSearchCursor{}})
	failed, ok := events[0].(PinnedMessagesLoadFailed)
	if !ok || failed.RequestID != 43 || failed.ChatID != 9 {
		t.Fatalf("failure event = %#v", events[0])
	}
	if failed.Error.Message != "Could not load pinned messages" || failed.Error.Cause != nil {
		t.Fatalf("failure error = %#v", failed.Error)
	}
}

func TestHandlerLoadPinnedMessageContextExactAndSafeFailure(t *testing.T) {
	client := &handlerClient{messagePage: telegram.MessagePage{Messages: []domain.Message{{ID: 200, ChatID: 9}}}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadPinnedMessageContext{RequestID: 50, ChatID: 9, MessageID: 200})
	loaded, ok := events[0].(PinnedMessageContextLoaded)
	if !ok || loaded.RequestID != 50 || loaded.ChatID != 9 || loaded.MessageID != 200 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.contextChat != 9 || client.contextMessage != 200 {
		t.Fatalf("context call = %v %v", client.contextChat, client.contextMessage)
	}
	client.err = errors.New("secret context detail")
	events = collectHandlerEvents(handler, LoadPinnedMessageContext{RequestID: 51, ChatID: 9, MessageID: 200})
	failed, ok := events[0].(PinnedMessageContextFailed)
	if !ok || failed.RequestID != 51 || failed.ChatID != 9 || failed.MessageID != 200 {
		t.Fatalf("failure event = %#v", events[0])
	}
	if failed.Error.Message != "Could not open pinned message" || failed.Error.Cause != nil {
		t.Fatalf("failure error = %#v", failed.Error)
	}
}

func TestPinnedMessagesTotalCountOnInsertBeforeSelection(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{ChatID: 9, Selected: 1}
	state.PinnedMessages.Results = []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"},
		{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "b", SentAt: time.Unix(300, 0)},
	}
	state.PinnedMessages.TotalCount = 2
	// New pin 100 inserts before selection (index 1).
	state.Messages[9] = append(state.Messages[9],
		domain.Message{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "newest", SentAt: time.Unix(100, 0)})
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 100, Pinned: true}})
	if got.PinnedMessages.TotalCount != 3 {
		t.Fatalf("total count = %d, want 3", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 2 {
		t.Fatalf("selected index = %d, want 2 (shifted by insert)", got.PinnedMessages.Selected)
	}
	if got.PinnedMessages.Results[2].ID != 300 {
		t.Fatalf("selected message ID = %d, want 300", got.PinnedMessages.Results[2].ID)
	}
}

func TestPinnedMessagesUnpinBeforeSelectionPreservesIdentity(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{ChatID: 9, Selected: 2}
	state.PinnedMessages.Results = []domain.Message{
		{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "a"},
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "b"},
		{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "c", SentAt: time.Unix(300, 0)},
	}
	state.PinnedMessages.TotalCount = 3
	// Unpin message 100 (before selected).
	unpinned, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 100, Pinned: false}})
	if unpinned.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", unpinned.PinnedMessages.TotalCount)
	}
	if unpinned.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1 (preserved identity of msg 300)", unpinned.PinnedMessages.Selected)
	}
	if unpinned.PinnedMessages.Results[1].ID != 300 {
		t.Fatalf("selected message ID = %d, want 300", unpinned.PinnedMessages.Results[1].ID)
	}
}

func TestPinnedMessagesSelectedRowRemovalFallback(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{ChatID: 9, Selected: 0}
	state.PinnedMessages.Results = []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "only", SentAt: time.Unix(200, 0)},
	}
	state.PinnedMessages.TotalCount = 1
	// Unpin the selected message.
	unpinned, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if unpinned.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", unpinned.PinnedMessages.TotalCount)
	}
	if unpinned.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", unpinned.PinnedMessages.Selected)
	}
	if len(unpinned.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(unpinned.PinnedMessages.Results))
	}
}

func TestPinnedMessagesLoadFailedIgnoredDuringContextJump(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID: 9, RequestID: 10, Selected: 0,
		JumpMessageID: 200,
		Loading:       true,
		Results: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target"},
		},
	}
	// A list-load failure during a context jump should be ignored.
	ignored, cmds := Reduce(state, PinnedMessagesLoadFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds) != 0 || !reflect.DeepEqual(ignored, state) {
		t.Fatalf("should be ignored: %#v %#v", ignored, cmds)
	}
	// A matching request-id failure during jump should also be ignored.
	ignored2, cmds2 := Reduce(state, PinnedMessagesLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds2) != 0 || !reflect.DeepEqual(ignored2, state) {
		t.Fatalf("should be ignored: %#v %#v", ignored2, cmds2)
	}
}

func TestPinnedMessagesNewPinIntoEmptyResults(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:     9,
		Results:    []domain.Message{},
		TotalCount: 0,
		Selected:   0,
	}
	state.Messages[9] = append(state.Messages[9],
		domain.Message{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "first", SentAt: time.Unix(500, 0)})
	// New pin into empty results: no panic, one result, count 1, selected 0.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 500, Pinned: true}})
	if len(got.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.Results[0].ID != 500 {
		t.Fatalf("result[0] ID = %d, want 500", got.PinnedMessages.Results[0].ID)
	}
	if got.PinnedMessages.TotalCount != 1 {
		t.Fatalf("total count = %d, want 1", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesUnrelatedUnpinEmptyResults(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:     9,
		Results:    []domain.Message{},
		TotalCount: 0,
		Selected:   0,
	}
	// Unrelated unpin with empty results: no panic, count unchanged.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(got.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesUnrelatedUnpinNonemptyResults(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:   9,
		Selected: 0,
		Results: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "b", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	// Unrelated unpin (message 999 not in results): count and selected identity unchanged.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(got.PinnedMessages.Results) != 2 {
		t.Fatalf("results count = %d, want 2", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.PinnedMessages.Selected)
	}
	if got.PinnedMessages.Results[0].ID != 200 {
		t.Fatalf("result[0] ID = %d, want 200", got.PinnedMessages.Results[0].ID)
	}
}

func TestPinnedMessagesDuplicatePinCountUnchanged(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:   9,
		Selected: 0,
		Results: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned", SentAt: time.Unix(200, 0)},
		},
		TotalCount: 1,
	}
	state.Messages[9] = append(state.Messages[9],
		domain.Message{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned-dup", SentAt: time.Unix(200, 0)})
	// Duplicate pin update: count unchanged, result updated.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: true}})
	if len(got.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.TotalCount != 1 {
		t.Fatalf("total count = %d, want 1 (duplicate should not increment)", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesUnpinInvalidSelectedIndex(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:   9,
		Selected: -1, // Invalid index
		Results: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "b", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	// Unrelated unpin with invalid Selected (-1): no panic, unchanged count/results.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(got.PinnedMessages.Results) != 2 {
		t.Fatalf("results count = %d, want 2", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", got.PinnedMessages.TotalCount)
	}
	// Selected should be safely clamped to a valid range.
	if got.PinnedMessages.Selected < 0 || got.PinnedMessages.Selected >= len(got.PinnedMessages.Results) {
		t.Fatalf("selected = %d, want clamped to 0..%d", got.PinnedMessages.Selected, len(got.PinnedMessages.Results)-1)
	}
}

func TestPinnedMessagesUnpinSelectedTooLarge(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:   9,
		Selected: 100, // Way out of range
		Results: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"},
		},
		TotalCount: 1,
	}
	// Unpin the only message with out-of-range Selected.
	got, _ := Reduce(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(got.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(got.PinnedMessages.Results))
	}
	if got.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", got.PinnedMessages.TotalCount)
	}
	if got.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.PinnedMessages.Selected)
	}
}
