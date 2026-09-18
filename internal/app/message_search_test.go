package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func searchBaseState() State {
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
		{ID: 90, ChatID: 9, Kind: domain.MessageText, Text: "old"},
		{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "new"},
	}
	state.History[9] = HistoryState{Loading: false, Done: true, OldestID: 90}
	return state
}

func openSearch(t *testing.T, state State) State {
	t.Helper()
	opened, commands := Reduce(state, ActionReceived{Action: OpenMessageSearch})
	if opened.MessageSearch == nil || opened.MessageSearch.ChatID != 9 || opened.Focus != FocusSearchInput {
		t.Fatalf("open = %#v %#v", opened.MessageSearch, opened.Focus)
	}
	if len(commands) != 0 {
		t.Fatalf("open commands = %#v", commands)
	}
	return opened
}

func submitSearch(t *testing.T, opened State, input string) (State, []Command) {
	t.Helper()
	withInput, _ := Reduce(opened, MessageSearchValueChanged{ChatID: 9, Value: input})
	submitted, commands := Reduce(withInput, ActionReceived{Action: SubmitMessageSearch})
	return submitted, commands
}

func TestMessageSearchOpenRequiresConversationFocus(t *testing.T) {
	state := searchBaseState()
	state.Focus = FocusChats
	unchanged, commands := Reduce(state, ActionReceived{Action: OpenMessageSearch})
	if unchanged.MessageSearch != nil || len(commands) != 0 {
		t.Fatalf("open from chats = %#v %#v", unchanged.MessageSearch, commands)
	}
	opened := openSearch(t, searchBaseState())
	if opened.MessageSearch.PreviousFocus != FocusConversation {
		t.Fatalf("previous focus = %v", opened.MessageSearch.PreviousFocus)
	}
}

func TestMessageSearchValueChangedScopesToActiveChat(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	changed, _ := Reduce(opened, MessageSearchValueChanged{ChatID: 9, Value: "needle"})
	if string(changed.MessageSearch.Input) != "needle" {
		t.Fatalf("input = %q", string(changed.MessageSearch.Input))
	}
	wrongChat, _ := Reduce(opened, MessageSearchValueChanged{ChatID: 8, Value: "other"})
	if string(wrongChat.MessageSearch.Input) != "" {
		t.Fatalf("wrong chat mutated input: %q", string(wrongChat.MessageSearch.Input))
	}
}

func TestMessageSearchSubmitTrimsAndEmitsExactCommand(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, commands := submitSearch(t, opened, "  needle  ")
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	cmd, ok := commands[0].(SearchChatMessages)
	if !ok || cmd.RequestID != 10 || cmd.ChatID != 9 || cmd.Query != "needle" || cmd.Cursor.Limit != pageSize {
		t.Fatalf("search command = %#v", commands[0])
	}
	if !submitted.MessageSearch.Loading || !submitted.MessageSearch.Submitted || submitted.Focus != FocusSearchResults || submitted.MessageSearch.Query != "needle" || len(submitted.MessageSearch.Results) != 0 {
		t.Fatalf("submitted = %#v", submitted.MessageSearch)
	}
	if submitted.Drafts[9] != "keep draft" {
		t.Fatalf("draft lost: %q", submitted.Drafts[9])
	}
	blank, blankCommands := submitSearch(t, opened, "   ")
	if len(blankCommands) != 0 || blank.MessageSearch != nil && blank.MessageSearch.Submitted {
		t.Fatalf("blank submit = %#v %#v", blank.MessageSearch, blankCommands)
	}
	_ = reflect.DeepEqual
}

func TestMessageSearchResultsAppendDedupAndStaleGuards(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	first := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "new", SenderName: "a"},
			{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old", SenderName: "b"},
		},
		NextFromMessageID: 10,
		TotalCount:        3,
	}
	got, _ := Reduce(submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: first})
	if len(got.MessageSearch.Results) != 2 || got.MessageSearch.Done || got.MessageSearch.Loading || got.Focus != FocusSearchResults {
		t.Fatalf("first page = %#v", got.MessageSearch)
	}
	// Duplicate + wrong chat + cross-chat message are ignored/deduped.
	second := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "dup"},
			{ID: 10, ChatID: 9, Kind: domain.MessageText, Text: "older"},
			{ID: 99, ChatID: 8, Kind: domain.MessageText, Text: "other chat"},
		},
		NextFromMessageID: 0,
		TotalCount:        3,
		Done:              true,
	}
	paged, cmds := Reduce(got, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: second})
	if len(paged.MessageSearch.Results) != 3 || paged.MessageSearch.Results[2].ID != 10 || !paged.MessageSearch.Done {
		t.Fatalf("second page = %#v cmds=%#v", paged.MessageSearch, cmds)
	}
	for _, stale := range []Event{
		ChatMessagesSearched{RequestID: 11, ChatID: 9, Page: first},
		ChatMessagesSearched{RequestID: 10, ChatID: 8, Page: first},
	} {
		before := cloneReducerState(paged)
		after, afterCmds := Reduce(paged, stale)
		if len(afterCmds) != 0 || !reflect.DeepEqual(after, before) {
			t.Fatalf("stale result mutated state: %#v", stale)
		}
	}
}

func TestMessageSearchPaginationOnlyFromLastSelection(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	page := telegram.MessageSearchPage{
		Messages: []domain.Message{
			{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "a"},
			{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "b"},
		},
		NextFromMessageID: 10,
	}
	loaded, _ := Reduce(submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: page})
	// Moving onto the last row paginates once.
	paged, pagedCmds := Reduce(loaded, ActionReceived{Action: SelectNext})
	if paged.MessageSearch.Selected != 1 {
		t.Fatalf("move = %#v", paged.MessageSearch)
	}
	if len(pagedCmds) != 1 {
		t.Fatalf("expected pagination command, got %#v", pagedCmds)
	}
	paginateCmd, ok := pagedCmds[0].(SearchChatMessages)
	if !ok || paginateCmd.Cursor.FromMessageID != 10 || paginateCmd.Query != "needle" || !paged.MessageSearch.Loading {
		t.Fatalf("paginate command = %#v", pagedCmds[0])
	}
}

func TestMessageSearchFailureIsFixedAndSafe(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	failed, _ := Reduce(submitted, ChatMessagesSearchFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "x", Message: "raw"}})
	if failed.MessageSearch.Loading || failed.MessageSearch.Error == nil || failed.MessageSearch.Error.Message != "Could not search messages" {
		t.Fatalf("failure = %#v", failed.MessageSearch)
	}
	if failed.Focus != FocusSearchResults {
		t.Fatalf("focus = %v", failed.Focus)
	}
	before := cloneReducerState(failed)
	after, cmds := Reduce(failed, ChatMessagesSearchFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds) != 0 || !reflect.DeepEqual(after, before) {
		t.Fatal("stale failure mutated state")
	}
}

func TestMessageSearchJumpLoadsContextAndClearsOverlay(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	page := telegram.MessageSearchPage{
		Messages:          []domain.Message{{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "target"}},
		NextFromMessageID: 0,
		Done:              true,
	}
	loaded, _ := Reduce(submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: page})
	jumping, cmds := Reduce(loaded, ActionReceived{Action: Activate})
	if len(cmds) != 1 || jumping.MessageSearch.JumpMessageID != 20 || !jumping.MessageSearch.Loading {
		t.Fatalf("jump = %#v %#v", jumping.MessageSearch, cmds)
	}
	ctxCmd, ok := cmds[0].(LoadSearchMessageContext)
	if !ok || ctxCmd.ChatID != 9 || ctxCmd.MessageID != 20 {
		t.Fatalf("context command = %#v", cmds[0])
	}
	contextPage := telegram.MessagePage{Messages: []domain.Message{
		{ID: 10, ChatID: 9, Kind: domain.MessageText, Text: "old", SentAt: time.Unix(10, 0)},
		{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "target", SentAt: time.Unix(20, 0)},
		{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "new", SentAt: time.Unix(30, 0)},
	}}
	landed, _ := Reduce(jumping, SearchMessageContextLoaded{RequestID: jumping.MessageSearch.RequestID, ChatID: 9, MessageID: 20, Page: contextPage})
	if landed.MessageSearch != nil || landed.Focus != FocusConversation || landed.SelectedMessage != 20 || landed.SelectedMessageChat != 9 {
		t.Fatalf("landed = %#v focus=%v sel=%v", landed.MessageSearch, landed.Focus, landed.SelectedMessage)
	}
	if landed.History[9].ViewOffset != 1 {
		t.Fatalf("view offset = %d", landed.History[9].ViewOffset)
	}
	if landed.Drafts[9] != "keep draft" {
		t.Fatalf("draft lost after jump")
	}
}

func TestMessageSearchContextFailureKeepsOverlayAndToasts(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	loaded, _ := Reduce(submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: telegram.MessageSearchPage{
		Messages: []domain.Message{{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "t"}},
		Done:     true,
	}})
	jumping, _ := Reduce(loaded, ActionReceived{Action: Activate})
	failed, _ := Reduce(jumping, SearchMessageContextFailed{RequestID: jumping.MessageSearch.RequestID, ChatID: 9, MessageID: 20, Error: domain.AppError{Message: "raw"}})
	if failed.MessageSearch == nil || failed.MessageSearch.JumpMessageID != 0 || failed.MessageSearch.Loading || failed.Focus != FocusSearchResults {
		t.Fatalf("context failure = %#v", failed.MessageSearch)
	}
	if failed.MessageSearch.Error == nil || failed.MessageSearch.Error.Message != "Could not open search result" || failed.Toast == nil {
		t.Fatalf("error/toast = %#v %#v", failed.MessageSearch.Error, failed.Toast)
	}
}

func TestMessageSearchClosePreservesDraftAndSelectChatClears(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	closed, _ := Reduce(opened, ActionReceived{Action: Close})
	if closed.MessageSearch != nil || closed.Focus != FocusConversation || closed.Drafts[9] != "keep draft" {
		t.Fatalf("close = %#v", closed)
	}
	reopened := openSearch(t, searchBaseState())
	reopened.Chats = append(reopened.Chats, domain.Chat{ID: 8, Title: "other"})
	switched, cmds := Reduce(reopened, ActionReceived{Action: SelectChat, ChatID: 8})
	if switched.MessageSearch != nil {
		t.Fatalf("select chat must clear search: %#v", switched.MessageSearch)
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

func TestHandlerSearchChatMessagesExactRequestAndSafeFailure(t *testing.T) {
	client := &handlerClient{searchPage: telegram.MessageSearchPage{
		Messages:          []domain.Message{{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "hit"}},
		NextFromMessageID: 10,
		TotalCount:        1,
	}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, SearchChatMessages{
		RequestID: 10, ChatID: 9, Query: "needle",
		Cursor: telegram.MessageSearchCursor{FromMessageID: 40, Limit: 25},
	})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	searched, ok := events[0].(ChatMessagesSearched)
	if !ok || searched.RequestID != 10 || searched.ChatID != 9 || len(searched.Page.Messages) != 1 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.searchChat != 9 || client.searchQuery != "needle" || client.searchCursor.FromMessageID != 40 || client.searchCursor.Limit != 25 {
		t.Fatalf("search call = %v %q %#v", client.searchChat, client.searchQuery, client.searchCursor)
	}
	client.err = errors.New("secret query detail")
	events = collectHandlerEvents(handler, SearchChatMessages{RequestID: 11, ChatID: 9, Query: "secret", Cursor: telegram.MessageSearchCursor{}})
	failed, ok := events[0].(ChatMessagesSearchFailed)
	if !ok || failed.RequestID != 11 || failed.ChatID != 9 {
		t.Fatalf("failure event = %#v", events[0])
	}
	if failed.Error.Message != "Could not search messages" || failed.Error.Cause != nil {
		t.Fatalf("failure error = %#v", failed.Error)
	}
}

func TestHandlerLoadSearchMessageContextExactAndSafeFailure(t *testing.T) {
	client := &handlerClient{messagePage: telegram.MessagePage{Messages: []domain.Message{{ID: 20, ChatID: 9}}}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadSearchMessageContext{RequestID: 12, ChatID: 9, MessageID: 20})
	loaded, ok := events[0].(SearchMessageContextLoaded)
	if !ok || loaded.RequestID != 12 || loaded.ChatID != 9 || loaded.MessageID != 20 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.contextChat != 9 || client.contextMessage != 20 {
		t.Fatalf("context call = %v %v", client.contextChat, client.contextMessage)
	}
	client.err = errors.New("secret context detail")
	events = collectHandlerEvents(handler, LoadSearchMessageContext{RequestID: 13, ChatID: 9, MessageID: 20})
	failed, ok := events[0].(SearchMessageContextFailed)
	if !ok || failed.RequestID != 13 || failed.ChatID != 9 || failed.MessageID != 20 {
		t.Fatalf("failure event = %#v", events[0])
	}
	if failed.Error.Message != "Could not open search result" || failed.Error.Cause != nil {
		t.Fatalf("failure error = %#v", failed.Error)
	}
}

func TestMessageSearchCloneDoesNotAlias(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	withInput, _ := Reduce(opened, MessageSearchValueChanged{ChatID: 9, Value: "ab"})
	cloned := cloneReducerState(withInput)
	cloned.MessageSearch.Input[0] = 'X'
	cloned.MessageSearch.Results = append(cloned.MessageSearch.Results, domain.Message{ID: 1})
	if string(withInput.MessageSearch.Input) != "ab" || len(withInput.MessageSearch.Results) != 0 {
		t.Fatal("clone aliased search state")
	}
}
