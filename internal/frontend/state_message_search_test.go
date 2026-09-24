package frontend

import (
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
	commands := updateState(&state, ActionReceived{Action: OpenMessageSearch})
	if state.MessageSearch == nil || state.MessageSearch.ChatID != 9 || state.Focus != FocusSearchInput {
		t.Fatalf("open = %#v %#v", state.MessageSearch, state.Focus)
	}
	if len(commands) != 0 {
		t.Fatalf("open commands = %#v", commands)
	}
	return state
}

func submitSearch(t *testing.T, state State, input string) (State, []Effect) {
	t.Helper()
	updateState(&state, MessageSearchValueChanged{ChatID: 9, Value: input})
	commands := updateState(&state, ActionReceived{Action: SubmitMessageSearch})
	return state, commands
}

func TestMessageSearchOpenRequiresConversationFocus(t *testing.T) {
	state := searchBaseState()
	state.Focus = FocusChats
	commands := updateState(&state, ActionReceived{Action: OpenMessageSearch})
	if state.MessageSearch != nil || len(commands) != 0 {
		t.Fatalf("open from chats = %#v %#v", state.MessageSearch, commands)
	}
	opened := openSearch(t, searchBaseState())
	if opened.MessageSearch.PreviousFocus != FocusConversation {
		t.Fatalf("previous focus = %v", opened.MessageSearch.PreviousFocus)
	}
}

func TestMessageSearchValueChangedScopesToActiveChat(t *testing.T) {
	changed := openSearch(t, searchBaseState())
	updateState(&changed, MessageSearchValueChanged{ChatID: 9, Value: "needle"})
	if string(changed.MessageSearch.Input) != "needle" {
		t.Fatalf("input = %q", string(changed.MessageSearch.Input))
	}
	wrongChat := openSearch(t, searchBaseState())
	updateState(&wrongChat, MessageSearchValueChanged{ChatID: 8, Value: "other"})
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
	blank, blankCommands := submitSearch(t, openSearch(t, searchBaseState()), "   ")
	if len(blankCommands) != 0 || blank.MessageSearch != nil && blank.MessageSearch.Submitted {
		t.Fatalf("blank submit = %#v %#v", blank.MessageSearch, blankCommands)
	}
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
	updateState(&submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: first})
	if len(submitted.MessageSearch.Results) != 2 || submitted.MessageSearch.Done || submitted.MessageSearch.Loading || submitted.Focus != FocusSearchResults {
		t.Fatalf("first page = %#v", submitted.MessageSearch)
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
	cmds := updateState(&submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: second})
	if len(submitted.MessageSearch.Results) != 3 || submitted.MessageSearch.Results[2].ID != 10 || !submitted.MessageSearch.Done {
		t.Fatalf("second page = %#v cmds=%#v", submitted.MessageSearch, cmds)
	}
	for _, stale := range []Event{
		ChatMessagesSearched{RequestID: 11, ChatID: 9, Page: first},
		ChatMessagesSearched{RequestID: 10, ChatID: 8, Page: first},
	} {
		commands := updateState(&submitted, stale)
		search := submitted.MessageSearch
		if len(commands) != 0 || search.RequestID != 10 || search.Loading || !search.Done || len(search.Results) != 3 || search.Results[2].ID != 10 {
			t.Fatalf("stale result replaced active page: search=%#v effects=%#v", search, commands)
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
	updateState(&submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: page})
	// Moving onto the last row paginates once.
	pagedCmds := updateState(&submitted, ActionReceived{Action: SelectNext})
	if submitted.MessageSearch.Selected != 1 {
		t.Fatalf("move = %#v", submitted.MessageSearch)
	}
	if len(pagedCmds) != 1 {
		t.Fatalf("expected pagination command, got %#v", pagedCmds)
	}
	paginateCmd, ok := pagedCmds[0].(SearchChatMessages)
	if !ok || paginateCmd.Cursor.FromMessageID != 10 || paginateCmd.Query != "needle" || !submitted.MessageSearch.Loading {
		t.Fatalf("paginate command = %#v", pagedCmds[0])
	}
}

func TestMessageSearchFailureIsFixedAndSafe(t *testing.T) {
	failed := searchFailureFixture(t)
	if failed.MessageSearch.Loading || failed.MessageSearch.Error == nil || failed.MessageSearch.Error.Message != "Could not search messages" {
		t.Fatalf("failure = %#v", failed.MessageSearch)
	}
	if failed.Focus != FocusSearchResults {
		t.Fatalf("focus = %v", failed.Focus)
	}
	cmds := updateState(&failed, ChatMessagesSearchFailed{RequestID: 11, ChatID: 9, Error: domain.AppError{Message: "stale"}})
	if len(cmds) != 0 || !reflect.DeepEqual(failed, searchFailureFixture(t)) {
		t.Fatal("stale failure mutated state")
	}
}

func searchFailureFixture(t *testing.T) State {
	t.Helper()
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	updateState(&submitted, ChatMessagesSearchFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorInternal, Op: "x", Message: "raw"}})
	return submitted
}

func TestMessageSearchJumpLoadsContextAndClearsOverlay(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	page := telegram.MessageSearchPage{
		Messages:          []domain.Message{{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "target"}},
		NextFromMessageID: 0,
		Done:              true,
	}
	updateState(&submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: page})
	cmds := updateState(&submitted, ActionReceived{Action: Activate})
	if len(cmds) != 1 || submitted.MessageSearch.JumpMessageID != 20 || !submitted.MessageSearch.Loading {
		t.Fatalf("jump = %#v %#v", submitted.MessageSearch, cmds)
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
	updateState(&submitted, SearchMessageContextLoaded{RequestID: submitted.MessageSearch.RequestID, ChatID: 9, MessageID: 20, Page: contextPage})
	if submitted.MessageSearch != nil || submitted.Focus != FocusConversation || submitted.SelectedMessage != 20 || submitted.SelectedMessageChat != 9 {
		t.Fatalf("landed = %#v focus=%v sel=%v", submitted.MessageSearch, submitted.Focus, submitted.SelectedMessage)
	}
	if submitted.History[9].ViewOffset != 1 {
		t.Fatalf("view offset = %d", submitted.History[9].ViewOffset)
	}
	if submitted.Drafts[9] != "keep draft" {
		t.Fatalf("draft lost after jump")
	}
}

func TestMessageSearchContextFailureKeepsOverlayAndToasts(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	submitted, _ := submitSearch(t, opened, "needle")
	updateState(&submitted, ChatMessagesSearched{RequestID: 10, ChatID: 9, Page: telegram.MessageSearchPage{
		Messages: []domain.Message{{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "t"}},
		Done:     true,
	}})
	updateState(&submitted, ActionReceived{Action: Activate})
	updateState(&submitted, SearchMessageContextFailed{RequestID: submitted.MessageSearch.RequestID, ChatID: 9, MessageID: 20, Error: domain.AppError{Message: "raw"}})
	if submitted.MessageSearch == nil || submitted.MessageSearch.JumpMessageID != 0 || submitted.MessageSearch.Loading || submitted.Focus != FocusSearchResults {
		t.Fatalf("context failure = %#v", submitted.MessageSearch)
	}
	if submitted.MessageSearch.Error == nil || submitted.MessageSearch.Error.Message != "Could not open search result" || submitted.Toast == nil {
		t.Fatalf("error/toast = %#v %#v", submitted.MessageSearch.Error, submitted.Toast)
	}
}

func TestMessageSearchClosePreservesDraftAndSelectChatClears(t *testing.T) {
	opened := openSearch(t, searchBaseState())
	updateState(&opened, ActionReceived{Action: Close})
	if opened.MessageSearch != nil || opened.Focus != FocusConversation || opened.Drafts[9] != "keep draft" {
		t.Fatalf("close = %#v", opened)
	}
	reopened := openSearch(t, searchBaseState())
	reopened.Chats = append(reopened.Chats, domain.Chat{ID: 8, Title: "other"})
	cmds := updateState(&reopened, ActionReceived{Action: SelectChat, ChatID: 8})
	if reopened.MessageSearch != nil {
		t.Fatalf("select chat must clear search: %#v", reopened.MessageSearch)
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
