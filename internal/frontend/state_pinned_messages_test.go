package frontend

import (
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
	commands := updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	if state.PinnedMessages == nil || state.PinnedMessages.ChatID != 9 || state.Focus != FocusPinnedResults {
		t.Fatalf("open = %#v %#v", state.PinnedMessages, state.Focus)
	}
	if len(commands) != 1 {
		t.Fatalf("open commands count = %d, want 1", len(commands))
	}
	cmd, ok := commands[0].(LoadPinnedMessages)
	if !ok || cmd.ChatID != 9 || cmd.RequestID != 10 {
		t.Fatalf("open command = %#v", cmd)
	}
	return state
}

func TestOpenPinnedRequiresConversationFocus(t *testing.T) {
	state := pinnedBaseState()
	state.Focus = FocusChats
	commands := updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	if state.PinnedMessages != nil || len(commands) != 0 {
		t.Fatalf("open from chats = %#v %#v", state.PinnedMessages, commands)
	}
	opened := openPinned(t, pinnedBaseState())
	if opened.PinnedMessages.PreviousFocus != FocusConversation {
		t.Fatalf("previous focus = %v", opened.PinnedMessages.PreviousFocus)
	}
}

func TestOpenPinnedAllocatesRequestIDAndSetsLoading(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 42
	commands := updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	if state.PinnedMessages == nil {
		t.Fatal("pinned messages not opened")
	}
	if state.PinnedMessages.RequestID != 42 {
		t.Fatalf("request ID = %d, want 42", state.PinnedMessages.RequestID)
	}
	if !state.PinnedMessages.Loading {
		t.Fatal("loading should be true")
	}
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
}

func TestPinnedMessagesLoadedAppendsAndDeduplicates(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 100
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	// Simulate first page load.
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned3", SentAt: time.Unix(500, 0)},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "pinned4", SentAt: time.Unix(400, 0)},
		},
		NextFromMessageID: 350,
		TotalCount:        5,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 100, ChatID: 9, Page: first})
	if len(state.PinnedMessages.Results) != 2 {
		t.Fatalf("first page results count = %d, want 2", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Results[0].ID != 500 || state.PinnedMessages.Results[1].ID != 400 {
		t.Fatalf("first page order = [%d, %d], want [500, 400]",
			state.PinnedMessages.Results[0].ID, state.PinnedMessages.Results[1].ID)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
	}
	if state.PinnedMessages.Done {
		t.Fatal("done should be false after first page")
	}
	if state.PinnedMessages.NextFromMessageID != 350 {
		t.Fatalf("nextFromMessageID = %d, want 350", state.PinnedMessages.NextFromMessageID)
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
	cmds := updateState(&state, PinnedMessagesLoaded{RequestID: 100, ChatID: 9, Page: second})
	if len(state.PinnedMessages.Results) != 3 {
		t.Fatalf("second page results count = %d, want 3", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Results[2].ID != 350 {
		t.Fatalf("third result ID = %d, want 350", state.PinnedMessages.Results[2].ID)
	}
	if !state.PinnedMessages.Done {
		t.Fatal("done should be true after second page")
	}
	if len(cmds) != 0 {
		t.Fatalf("unexpected commands: %#v", cmds)
	}
}

func TestPinnedMessagesStaleRequestIgnored(t *testing.T) {
	first := telegram.MessageSearchPage{
		Messages:   []domain.Message{{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned"}},
		TotalCount: 1,
	}
	loaded := pinnedBaseState()
	loaded.NextRequestID = 10
	updateState(&loaded, ActionReceived{Action: OpenPinnedMessages})
	updateState(&loaded, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	want := pinnedBaseState()
	want.NextRequestID = 10
	updateState(&want, ActionReceived{Action: OpenPinnedMessages})
	updateState(&want, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	for _, event := range []PinnedMessagesLoaded{
		{RequestID: 11, ChatID: 9, Page: first},
		{RequestID: 10, ChatID: 8, Page: first},
	} {
		commands := updateState(&loaded, event)
		view := loaded.PinnedMessages
		if len(commands) != 0 || !reflect.DeepEqual(loaded, want) {
			t.Fatalf("stale pinned page changed active results: event=%#v view=%#v effects=%#v", event, view, commands)
		}
	}
}

func TestPinnedMessagesJumpMessageGuard(t *testing.T) {
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "pinned"},
	}}
	withJump := func() State {
		state := pinnedBaseState()
		state.NextRequestID = 10
		updateState(&state, ActionReceived{Action: OpenPinnedMessages})
		updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
		// Simulate a jump in progress.
		state.PinnedMessages.JumpMessageID = 500
		state.PinnedMessages.Loading = true
		return state
	}
	loaded := withJump()
	// New search results during a jump should be ignored.
	commands := updateState(&loaded, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	if len(commands) != 0 || !reflect.DeepEqual(loaded, withJump()) {
		t.Fatalf("jump accepted new pinned page: view=%#v effects=%#v", loaded.PinnedMessages, commands)
	}
}

func TestPinnedMessagesSelectMessageSetsSelected(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "b"},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "c"},
		},
		TotalCount: 3,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Select middle message.
	updateState(&state, ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 400})
	if state.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1", state.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesSelectNextPaginates(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 500, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "b"},
		},
		NextFromMessageID: 300,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Move to last row to trigger pagination.
	cmds := updateState(&state, ActionReceived{Action: SelectNext})
	if state.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1", state.PinnedMessages.Selected)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected pagination command, got %#v", cmds)
	}
	pagCmd, ok := cmds[0].(LoadPinnedMessages)
	if !ok || pagCmd.Cursor.FromMessageID != 300 || pagCmd.RequestID != 11 {
		t.Fatalf("pagination command = %#v", cmds[0])
	}
	if !state.PinnedMessages.Loading {
		t.Fatal("should be loading after pagination trigger")
	}
}

func TestPinnedMessagesFailureIsFixedAndSafe(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	updateState(&state, PinnedMessagesLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "x", Message: "raw"}})
	if state.PinnedMessages.Loading {
		t.Fatal("should not be loading after failure")
	}
	if state.PinnedMessages.Error == nil {
		t.Fatal("error should be set")
	}
	if state.PinnedMessages.Error.Message != "Could not load pinned messages" {
		t.Fatalf("error message = %q, want safe message", state.PinnedMessages.Error.Message)
	}
	if state.Focus != FocusPinnedResults {
		t.Fatalf("focus = %v, want FocusPinnedResults", state.Focus)
	}
	// Stale failure (wrong request ID) should be ignored. The comparison
	// fixture is rebuilt independently because updateState mutates in place.
	failedFixture := func() State {
		state := pinnedBaseState()
		state.NextRequestID = 10
		updateState(&state, ActionReceived{Action: OpenPinnedMessages})
		updateState(&state, PinnedMessagesLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "x", Message: "raw"}})
		return state
	}
	afterCmds := updateState(&state, PinnedMessagesLoadFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(afterCmds) != 0 || !reflect.DeepEqual(state, failedFixture()) {
		t.Fatal("stale failure mutated state")
	}
}

func TestPinnedMessagesActivateLoadsContext(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages:   []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target"}},
		TotalCount: 1,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	cmds := updateState(&state, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	ctxCmd, ok := cmds[0].(LoadPinnedMessageContext)
	if !ok || ctxCmd.ChatID != 9 || ctxCmd.MessageID != 200 || ctxCmd.RequestID != 11 {
		t.Fatalf("context command = %#v", cmds[0])
	}
	if state.PinnedMessages.JumpMessageID != 200 {
		t.Fatalf("jumpMessageID = %d, want 200", state.PinnedMessages.JumpMessageID)
	}
	if !state.PinnedMessages.Loading {
		t.Fatal("should be loading during context request")
	}
}

func TestPinnedMessageContextLoadedClearsOverlayAndRestoresFocus(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target", SentAt: time.Unix(200, 0)},
	}}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	cmds := updateState(&state, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	ctxPage := telegram.MessagePage{Messages: []domain.Message{
		{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "old", SentAt: time.Unix(100, 0)},
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target", SentAt: time.Unix(200, 0)},
	}}
	updateState(&state, PinnedMessageContextLoaded{
		RequestID: state.PinnedMessages.RequestID, ChatID: 9, MessageID: 200, Page: ctxPage,
	})
	if state.PinnedMessages != nil {
		t.Fatalf("pinned should be nil after context load: %#v", state.PinnedMessages)
	}
	if state.Focus != FocusConversation {
		t.Fatalf("focus = %v, want FocusConversation", state.Focus)
	}
	if state.SelectedMessage != 200 || state.SelectedMessageChat != 9 {
		t.Fatalf("selection = chat=%v msg=%d", state.SelectedMessageChat, state.SelectedMessage)
	}
	if state.Drafts[9] != "keep draft" {
		t.Fatal("draft lost after context load")
	}
}

func TestPinnedMessageContextFailedRestoresOverlayWithToast(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{Messages: []domain.Message{
		{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "target"},
	}}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	cmds := updateState(&state, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate commands = %#v", cmds)
	}
	updateState(&state, PinnedMessageContextFailed{
		RequestID: state.PinnedMessages.RequestID, ChatID: 9, MessageID: 200,
		Error: domain.AppError{Message: "raw"},
	})
	if state.PinnedMessages == nil {
		t.Fatal("pinned should be present after context failure")
	}
	if state.PinnedMessages.JumpMessageID != 0 {
		t.Fatalf("jumpMessageID = %d, want 0", state.PinnedMessages.JumpMessageID)
	}
	if state.PinnedMessages.Error == nil || state.PinnedMessages.Error.Message != "Could not open pinned message" {
		t.Fatalf("error = %#v", state.PinnedMessages.Error)
	}
	if state.Toast == nil {
		t.Fatal("toast should be set on context failure")
	}
	if state.Toast.Message != "Could not open pinned message" {
		t.Fatalf("toast message = %q", state.Toast.Message)
	}
}

func TestPinnedMessagesCloseRestoresFocus(t *testing.T) {
	opened := openPinned(t, pinnedBaseState())
	updateState(&opened, ActionReceived{Action: Close})
	if opened.PinnedMessages != nil {
		t.Fatalf("pinned should be nil: %#v", opened.PinnedMessages)
	}
	if opened.Focus != FocusConversation {
		t.Fatalf("focus = %v, want FocusConversation", opened.Focus)
	}
	if opened.Drafts[9] != "keep draft" {
		t.Fatal("draft lost on close")
	}
}

func TestPinnedMessagesLivePinUpdatesResults(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", SentAt: time.Unix(200, 0)},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// New pin arrives for message 400.
	state.Messages[9] = append(state.Messages[9],
		domain.Message{ID: 400, ChatID: 9, Kind: domain.MessageText, Text: "newPinned", SentAt: time.Unix(400, 0)})
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 400, Pinned: true}})
	if len(state.PinnedMessages.Results) != 3 {
		t.Fatalf("results count = %d, want 3", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Results[0].ID != 400 {
		t.Fatalf("first result = %d, want 400", state.PinnedMessages.Results[0].ID)
	}
}

func TestPinnedMessagesLiveUnpinRemovesResult(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", SentAt: time.Unix(200, 0)},
			{ID: 300, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", SentAt: time.Unix(300, 0)},
		},
		TotalCount: 2,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Unpin message 200.
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(state.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Results[0].ID != 300 {
		t.Fatalf("remaining result = %d, want 300", state.PinnedMessages.Results[0].ID)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesLiveUnpinLastResultSetsSelectedZero(t *testing.T) {
	state := pinnedBaseState()
	state.NextRequestID = 10
	updateState(&state, ActionReceived{Action: OpenPinnedMessages})
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "pinned", SentAt: time.Unix(200, 0)},
		},
		TotalCount: 1,
	}
	updateState(&state, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	// Unpin the only result.
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(state.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0 (not -1)", state.PinnedMessages.Selected)
	}
}

func TestPinnedMessagesSelectChatCleared(t *testing.T) {
	opened := openPinned(t, pinnedBaseState())
	first := telegram.MessageSearchPage{Messages: []domain.Message{{ID: 200, ChatID: 9, Kind: domain.MessageText, Text: "a"}}}
	updateState(&opened, PinnedMessagesLoaded{RequestID: 10, ChatID: 9, Page: first})
	opened.Chats = append(opened.Chats, domain.Chat{ID: 8, Title: "other"})
	cmds := updateState(&opened, ActionReceived{Action: SelectChat, ChatID: 8})
	if opened.PinnedMessages != nil {
		t.Fatalf("pinned should be cleared: %#v", opened.PinnedMessages)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 100, Pinned: true}})
	if state.PinnedMessages.TotalCount != 3 {
		t.Fatalf("total count = %d, want 3", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 2 {
		t.Fatalf("selected index = %d, want 2 (shifted by insert)", state.PinnedMessages.Selected)
	}
	if state.PinnedMessages.Results[2].ID != 300 {
		t.Fatalf("selected message ID = %d, want 300", state.PinnedMessages.Results[2].ID)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 100, Pinned: false}})
	if state.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 1 {
		t.Fatalf("selected = %d, want 1 (preserved identity of msg 300)", state.PinnedMessages.Selected)
	}
	if state.PinnedMessages.Results[1].ID != 300 {
		t.Fatalf("selected message ID = %d, want 300", state.PinnedMessages.Results[1].ID)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if state.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
	}
	if len(state.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(state.PinnedMessages.Results))
	}
}

func TestPinnedMessagesLoadFailedIgnoredDuringContextJump(t *testing.T) {
	jumpFixture := func() State {
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
		return state
	}
	state := jumpFixture()
	// A list-load failure during a context jump should be ignored. The
	// comparison fixture is rebuilt independently because updateState
	// mutates in place.
	cmds := updateState(&state, PinnedMessagesLoadFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds) != 0 || !reflect.DeepEqual(state, jumpFixture()) {
		t.Fatalf("should be ignored: %#v %#v", state, cmds)
	}
	// A matching request-id failure during jump should also be ignored.
	cmds2 := updateState(&state, PinnedMessagesLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds2) != 0 || !reflect.DeepEqual(state, jumpFixture()) {
		t.Fatalf("should be ignored: %#v %#v", state, cmds2)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 500, Pinned: true}})
	if len(state.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.Results[0].ID != 500 {
		t.Fatalf("result[0] ID = %d, want 500", state.PinnedMessages.Results[0].ID)
	}
	if state.PinnedMessages.TotalCount != 1 {
		t.Fatalf("total count = %d, want 1", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(state.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(state.PinnedMessages.Results) != 2 {
		t.Fatalf("results count = %d, want 2", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
	}
	if state.PinnedMessages.Results[0].ID != 200 {
		t.Fatalf("result[0] ID = %d, want 200", state.PinnedMessages.Results[0].ID)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: true}})
	if len(state.PinnedMessages.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.TotalCount != 1 {
		t.Fatalf("total count = %d, want 1 (duplicate should not increment)", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 999, Pinned: false}})
	if len(state.PinnedMessages.Results) != 2 {
		t.Fatalf("results count = %d, want 2", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2", state.PinnedMessages.TotalCount)
	}
	// Selected should be safely clamped to a valid range.
	if state.PinnedMessages.Selected < 0 || state.PinnedMessages.Selected >= len(state.PinnedMessages.Results) {
		t.Fatalf("selected = %d, want clamped to 0..%d", state.PinnedMessages.Selected, len(state.PinnedMessages.Results)-1)
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
	updateState(&state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 9, MessageID: 200, Pinned: false}})
	if len(state.PinnedMessages.Results) != 0 {
		t.Fatalf("results count = %d, want 0", len(state.PinnedMessages.Results))
	}
	if state.PinnedMessages.TotalCount != 0 {
		t.Fatalf("total count = %d, want 0", state.PinnedMessages.TotalCount)
	}
	if state.PinnedMessages.Selected != 0 {
		t.Fatalf("selected = %d, want 0", state.PinnedMessages.Selected)
	}
}
