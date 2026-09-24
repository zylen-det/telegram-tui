package frontend

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func chatSearchBaseState() State {
	state := InitialState()
	state.Focus = FocusChats
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 7, Title: "Existing", Order: 100, CanSend: true}}
	state.SelectedChat = 0
	state.Drafts[7] = "keep draft"
	state.NextRequestID = 10
	return state
}

// Each scenario starts with its own mutable search state.
func submittedChatSearch(query string) State {
	state := chatSearchBaseState()
	updateState(&state, ActionReceived{Action: OpenChatSearch})
	updateState(&state, ChatSearchValueChanged{Value: query})
	updateState(&state, ActionReceived{Action: SubmitChatSearch})
	return state
}

func TestChatSearchOpenAndSubmitExactUsername(t *testing.T) {
	state := chatSearchBaseState()
	commands := updateState(&state, ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || state.ChatSearch == nil || state.Focus != FocusChatSearchInput || state.ChatSearch.PreviousFocus != FocusChats {
		t.Fatalf("open = state %#v commands %#v", state.ChatSearch, commands)
	}
	updateState(&state, ChatSearchValueChanged{Value: "  @BotFather\n"})
	if got := string(state.ChatSearch.Input); got != "  @BotFather" {
		t.Fatalf("input = %q", got)
	}
	commands = updateState(&state, ActionReceived{Action: SubmitChatSearch})
	if len(commands) != 3 {
		t.Fatalf("commands = %#v", commands)
	}
	if state.Focus != FocusChatSearchResults || !state.ChatSearch.PublicLoading || !state.ChatSearch.MessagesLoading || !state.ChatSearch.Submitted || state.ChatSearch.Query != "BotFather" {
		t.Fatalf("submitted = %#v focus=%v", state.ChatSearch, state.Focus)
	}
	if state.Drafts[7] != "keep draft" {
		t.Fatal("chat search changed the existing draft")
	}

	// A blank submit branches from the opened search, so rebuild that fixture
	// instead of mutating the submitted state above.
	blank := chatSearchBaseState()
	updateState(&blank, ActionReceived{Action: OpenChatSearch})
	updateState(&blank, ChatSearchValueChanged{Value: " @ "})
	blankCommands := updateState(&blank, ActionReceived{Action: SubmitChatSearch})
	if len(blankCommands) != 0 || blank.ChatSearch.Submitted {
		t.Fatalf("blank submit = %#v %#v", blank.ChatSearch, blankCommands)
	}
}

func TestChatSearchOpensOnlyFromChatsAndCloses(t *testing.T) {
	state := chatSearchBaseState()
	state.Focus = FocusConversation
	commands := updateState(&state, ActionReceived{Action: OpenChatSearch})
	unchanged := state
	if len(commands) != 0 || unchanged.ChatSearch != nil {
		t.Fatalf("conversation open = %#v %#v", unchanged.ChatSearch, commands)
	}

	opened := chatSearchBaseState()
	updateState(&opened, ActionReceived{Action: OpenChatSearch})
	commands = updateState(&opened, ActionReceived{Action: Close})
	closed := opened
	if len(commands) != 0 || closed.ChatSearch != nil || closed.Focus != FocusChats {
		t.Fatalf("close = %#v focus=%v commands=%#v", closed.ChatSearch, closed.Focus, commands)
	}
}

func TestChatSearchResultsStaleGuardAndEditQuery(t *testing.T) {
	submitted := submittedChatSearch("@botfather")
	found := domain.Chat{ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather", CanSend: true}

	requestID := submitted.ChatSearch.RequestID
	want := submittedChatSearch("@botfather")
	commands := updateState(&submitted, PublicChatsSearched{RequestID: requestID + 99, Chats: []domain.Chat{found}})
	if len(commands) != 0 || !reflect.DeepEqual(submitted, want) {
		t.Fatalf("stale result changed active search: %#v effects=%#v", submitted.ChatSearch, commands)
	}

	updateState(&submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{found}})
	if submitted.ChatSearch.PublicLoading || len(submitted.ChatSearch.PublicChats) != 1 || submitted.ChatSearch.PublicChats[0].ID != 99 {
		t.Fatalf("loaded = %#v", submitted.ChatSearch)
	}
	commands = updateState(&submitted, ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || submitted.Focus != FocusChatSearchInput || submitted.ChatSearch.Submitted || submitted.ChatSearch.RequestID != 0 || string(submitted.ChatSearch.Input) != "@botfather" {
		t.Fatalf("edit query = %#v focus=%v commands=%#v", submitted.ChatSearch, submitted.Focus, commands)
	}
	want = submittedChatSearch("@botfather")
	updateState(&want, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{found}})
	updateState(&want, ActionReceived{Action: OpenChatSearch})
	staleCommands := updateState(&submitted, PublicChatSearchFailed{RequestID: requestID, Error: domain.AppError{Message: "raw"}})
	if len(staleCommands) != 0 || !reflect.DeepEqual(submitted, want) {
		t.Fatalf("invalidated failure changed edited query: %#v effects=%#v", submitted.ChatSearch, staleCommands)
	}
}

func TestChatSearchActivationAddsSelectsAndOpensChat(t *testing.T) {
	submitted := submittedChatSearch("@botfather")
	found := domain.Chat{
		ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather", CanSend: true,
		Avatar: domain.AvatarRef{FileID: 4, UniqueID: "bot-avatar"},
	}
	requestID := submitted.ChatSearch.RequestID
	// Send both events to clear loading flags.
	updateState(&submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{found}})
	loaded := submitted
	updateState(&loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{}})
	commands := updateState(&loaded, ActionReceived{Action: Activate})
	activated := loaded
	if activated.ChatSearch != nil || activated.Focus != FocusConversation {
		t.Fatalf("activated search/focus = %#v %v", activated.ChatSearch, activated.Focus)
	}
	index := chatIndex(activated.Chats, 99)
	if index < 0 || activated.SelectedChat != index || activated.Chats[index].Username != "BotFather" {
		t.Fatalf("chat list/selection = %#v selected=%d", activated.Chats, activated.SelectedChat)
	}
	if activated.Drafts[7] != "keep draft" {
		t.Fatal("activation changed existing draft")
	}
	var closedOld, openedNew, loadedHistory, renderedAvatar bool
	for _, command := range commands {
		switch command := command.(type) {
		case CloseChatCommand:
			closedOld = command.ChatID == 7
		case OpenChatCommand:
			openedNew = command.ChatID == 99
		case LoadMessages:
			loadedHistory = command.ChatID == 99
		case RenderAvatar:
			renderedAvatar = command.Ref.UniqueID == "bot-avatar"
		}
	}
	if !closedOld || !openedNew || !loadedHistory || !renderedAvatar {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestChatSearchMouseSelectionRequiresReturnedIdentity(t *testing.T) {
	submitted := submittedChatSearch("botfather")
	requestID := submitted.ChatSearch.RequestID
	updateState(&submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{{ID: 99, Title: "BotFather"}}})
	loaded := submitted
	updateState(&loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{}})
	commands := updateState(&loaded, ActionReceived{Action: SelectChat, ChatID: 100})
	unchanged := loaded
	if len(commands) != 0 || unchanged.ChatSearch == nil {
		t.Fatalf("wrong result = %#v %#v", unchanged.ChatSearch, commands)
	}
	commands = updateState(&loaded, ActionReceived{Action: SelectChat, ChatID: 99})
	selected := loaded
	if selected.ChatSearch != nil || selected.Focus != FocusConversation || chatIndex(selected.Chats, 99) < 0 || len(commands) == 0 {
		t.Fatalf("selected = %#v focus=%v commands=%#v", selected.ChatSearch, selected.Focus, commands)
	}
}

// New tests for the unified global search.

func TestChatSearchValueChangedFiltersLocalChats(t *testing.T) {
	state := chatSearchBaseState()
	state.Chats = append(state.Chats,
		domain.Chat{ID: 10, Title: "Telegram", Username: "telegram", Order: 50},
		domain.Chat{ID: 11, Title: "GitHub", Order: 40},
		domain.Chat{ID: 12, Title: "NoMatch", Order: 30},
	)
	updateState(&state, ActionReceived{Action: OpenChatSearch})
	opened := state
	updateState(&opened, ChatSearchValueChanged{Value: "tele"})
	changed := opened
	search := changed.ChatSearch
	if search == nil {
		t.Fatal("ChatSearch is nil")
	}
	if len(search.LocalChats) != 1 || search.LocalChats[0].Title != "Telegram" {
		t.Fatalf("local chats filter = %v", search.LocalChats)
	}
	if !search.PublicLoading {
		t.Fatal("expected PublicLoading to be true after typing")
	}
}

func TestChatSearchValueChangedClearsWhenBlank(t *testing.T) {
	state := chatSearchBaseState()
	state.Chats = append(state.Chats,
		domain.Chat{ID: 10, Title: "Telegram", Username: "telegram", Order: 50},
	)
	updateState(&state, ActionReceived{Action: OpenChatSearch})
	opened := state
	updateState(&opened, ChatSearchValueChanged{Value: "tele"})
	changed := opened
	// Now clear.
	updateState(&changed, ChatSearchValueChanged{Value: ""})
	cleared := changed
	search := cleared.ChatSearch
	if len(search.LocalChats) != 0 {
		t.Fatalf("expected empty local chats, got %v", search.LocalChats)
	}
	if search.PublicChats != nil || search.GlobalMessages != nil {
		t.Fatal("expected PublicChats/GlobalMessages to be nil when blank")
	}
}

func TestChatSearchPublicChatsSearched(t *testing.T) {
	submitted := submittedChatSearch("test")
	publicChats := []domain.Chat{
		{ID: 100, Title: "Public Group", Username: "publicgroup", Order: 20},
	}
	requestID := submitted.ChatSearch.RequestID
	updateState(&submitted, PublicChatsSearched{RequestID: requestID, Chats: publicChats})
	loaded := submitted
	if loaded.ChatSearch.PublicLoading {
		t.Fatal("PublicLoading should be false after search")
	}
	if len(loaded.ChatSearch.PublicChats) != 1 || loaded.ChatSearch.PublicChats[0].ID != 100 {
		t.Fatalf("public chats = %v", loaded.ChatSearch.PublicChats)
	}
	if loaded.ChatSearch.PublicError != nil {
		t.Fatalf("unexpected error: %v", loaded.ChatSearch.PublicError)
	}
}

func TestChatSearchPublicChatsSearchFailed(t *testing.T) {
	submitted := submittedChatSearch("test")
	requestID := submitted.ChatSearch.RequestID
	updateState(&submitted, PublicChatsSearchFailed{RequestID: requestID, Error: domain.AppError{Message: "network error"}})
	failed := submitted
	if failed.ChatSearch.PublicLoading {
		t.Fatal("PublicLoading should be false after failure")
	}
	if failed.ChatSearch.PublicChats != nil {
		t.Fatal("PublicChats should be nil after failure")
	}
	if failed.ChatSearch.PublicError == nil || !strings.Contains(failed.ChatSearch.PublicError.Message, "Could not search public chats") {
		t.Fatalf("unexpected error: %v", failed.ChatSearch.PublicError)
	}
}

func TestChatSearchAllMessagesSearched(t *testing.T) {
	submitted := submittedChatSearch("needle")
	messages := []domain.Message{
		{ID: 1, ChatID: 7, Kind: domain.MessageText, Text: "This has a needle", SentAt: time.Now()},
	}
	requestID := submitted.ChatSearch.RequestID
	updateState(&submitted, AllMessagesSearched{RequestID: requestID, Messages: messages})
	loaded := submitted
	if loaded.ChatSearch.MessagesLoading {
		t.Fatal("MessagesLoading should be false after search")
	}
	if len(loaded.ChatSearch.GlobalMessages) != 1 || loaded.ChatSearch.GlobalMessages[0].ChatID != 7 {
		t.Fatalf("global messages = %v", loaded.ChatSearch.GlobalMessages)
	}
}

func TestChatSearchAllMessagesSearchFailed(t *testing.T) {
	submitted := submittedChatSearch("test")
	requestID := submitted.ChatSearch.RequestID
	updateState(&submitted, AllMessagesSearchFailed{RequestID: requestID, Error: domain.AppError{Message: "search error"}})
	failed := submitted
	if failed.ChatSearch.MessagesLoading {
		t.Fatal("MessagesLoading should be false after failure")
	}
	if failed.ChatSearch.GlobalMessages != nil {
		t.Fatal("GlobalMessages should be nil after failure")
	}
	if failed.ChatSearch.MessagesError == nil || !strings.Contains(failed.ChatSearch.MessagesError.Message, "Could not search messages") {
		t.Fatalf("unexpected error: %v", failed.ChatSearch.MessagesError)
	}
}

func TestChatSearchLiveTypingEmitsBothAndStaysInInput(t *testing.T) {
	state := chatSearchBaseState()
	state.Chats = append(state.Chats, domain.Chat{ID: 10, Title: "Telegram", Username: "telegram", Order: 50})
	updateState(&state, ActionReceived{Action: OpenChatSearch})
	opened := state
	commands := updateState(&opened, ChatSearchValueChanged{Value: "tele"})
	typed := opened
	search := typed.ChatSearch
	if typed.Focus != FocusChatSearchInput || !search.Submitted {
		t.Fatalf("live focus/submitted = %v %#v", typed.Focus, search)
	}
	if len(search.LocalChats) != 1 || search.LocalChats[0].ID != 10 {
		t.Fatalf("local filter = %#v", search.LocalChats)
	}
	if !search.PublicLoading || !search.MessagesLoading {
		t.Fatalf("loading flags = %v %v", search.PublicLoading, search.MessagesLoading)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	var sawPublic, sawMessages bool
	for _, command := range commands {
		switch command := command.(type) {
		case SearchPublicChatsCommand:
			sawPublic = command.Query == "tele"
		case SearchAllMessagesCommand:
			sawMessages = command.Query == "tele" && command.Limit == 10
		}
	}
	if !sawPublic || !sawMessages {
		t.Fatalf("live commands = %#v", commands)
	}
	if typed.ChatSearch.RequestID == 0 || commands[0].(SearchPublicChatsCommand).RequestID != typed.ChatSearch.RequestID {
		t.Fatalf("request IDs diverged: %#v", commands)
	}
}

func TestChatSearchRemoteArrivalPreservesInputFocus(t *testing.T) {
	state := chatSearchBaseState()
	updateState(&state, ActionReceived{Action: OpenChatSearch})
	opened := state
	updateState(&opened, ChatSearchValueChanged{Value: "tele"})
	typed := opened
	requestID := typed.ChatSearch.RequestID
	updateState(&typed, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{{ID: 99, Title: "Pub"}}})
	loaded := typed
	if loaded.Focus != FocusChatSearchInput || loaded.ChatSearch.PublicLoading || len(loaded.ChatSearch.PublicChats) != 1 {
		t.Fatalf("public arrival = focus %v search %#v", loaded.Focus, loaded.ChatSearch)
	}
	updateState(&loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{{ID: 5, ChatID: 7, Kind: domain.MessageText, Text: "hi"}}})
	if loaded.Focus != FocusChatSearchInput || loaded.ChatSearch.MessagesLoading || len(loaded.ChatSearch.GlobalMessages) != 1 {
		t.Fatalf("messages arrival = focus %v search %#v", loaded.Focus, loaded.ChatSearch)
	}
}

func TestChatSearchMessageActivationJumpsToContext(t *testing.T) {
	typed := chatSearchBaseState()
	updateState(&typed, ActionReceived{Action: OpenChatSearch})
	updateState(&typed, ChatSearchValueChanged{Value: "needle"})
	requestID := typed.ChatSearch.RequestID
	updateState(&typed, PublicChatsSearched{RequestID: requestID, Chats: nil})
	loaded := typed
	msg := domain.Message{ID: 20, ChatID: 7, Kind: domain.MessageText, Text: "needle hit", SentAt: time.Now()}
	updateState(&loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{msg}})
	commands := updateState(&loaded, ActionReceived{Action: Activate})
	activated := loaded
	if activated.ChatSearch != nil {
		t.Fatalf("message activation must clear ChatSearch: %#v", activated.ChatSearch)
	}
	if activated.MessageSearch == nil || activated.MessageSearch.JumpMessageID != 20 || activated.MessageSearch.ChatID != 7 {
		t.Fatalf("jump state = %#v", activated.MessageSearch)
	}
	var sawContext, sawOpen bool
	for _, command := range commands {
		switch command := command.(type) {
		case LoadSearchMessageContext:
			sawContext = command.ChatID == 7 && command.MessageID == 20
		case OpenChatCommand:
			sawOpen = command.ChatID == 7
		}
	}
	if !sawContext || !sawOpen {
		t.Fatalf("jump commands = %#v", commands)
	}
	page := telegram.MessagePage{Messages: []domain.Message{
		{ID: 10, ChatID: 7, Kind: domain.MessageText, Text: "old"},
		{ID: 20, ChatID: 7, Kind: domain.MessageText, Text: "needle hit"},
		{ID: 30, ChatID: 7, Kind: domain.MessageText, Text: "new"},
	}}
	updateState(&activated, SearchMessageContextLoaded{RequestID: activated.MessageSearch.RequestID, ChatID: 7, MessageID: 20, Page: page})
	landed := activated
	if landed.MessageSearch != nil || landed.Focus != FocusConversation || landed.SelectedMessage != 20 || landed.SelectedMessageChat != 7 {
		t.Fatalf("landed = search %#v focus %v sel %v", landed.MessageSearch, landed.Focus, landed.SelectedMessage)
	}
}

func TestChatSearchMouseMessageSelectionJumps(t *testing.T) {
	typed := chatSearchBaseState()
	updateState(&typed, ActionReceived{Action: OpenChatSearch})
	updateState(&typed, ChatSearchValueChanged{Value: "needle"})
	requestID := typed.ChatSearch.RequestID
	updateState(&typed, PublicChatsSearched{RequestID: requestID, Chats: nil})
	loaded := typed
	updateState(&loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{{ID: 20, ChatID: 7, Kind: domain.MessageText, Text: "hit"}}})
	commands := updateState(&loaded, ActionReceived{Action: SelectMessage, ChatID: 7, MessageID: 20})
	selected := loaded
	if selected.ChatSearch != nil || selected.MessageSearch == nil {
		t.Fatalf("mouse jump = search %#v msgsearch %#v", selected.ChatSearch, selected.MessageSearch)
	}
	found := false
	for _, command := range commands {
		if ctx, ok := command.(LoadSearchMessageContext); ok && ctx.ChatID == 7 && ctx.MessageID == 20 {
			found = true
		}
	}
	if !found {
		t.Fatalf("mouse commands = %#v", commands)
	}
}
