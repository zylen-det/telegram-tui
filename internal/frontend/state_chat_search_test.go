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

func TestChatSearchOpenAndSubmitExactUsername(t *testing.T) {
	state := chatSearchBaseState()
	opened, commands := updateState(state, ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || opened.ChatSearch == nil || opened.Focus != FocusChatSearchInput || opened.ChatSearch.PreviousFocus != FocusChats {
		t.Fatalf("open = state %#v commands %#v", opened.ChatSearch, commands)
	}
	changed, _ := updateState(opened, ChatSearchValueChanged{Value: "  @BotFather\n"})
	if got := string(changed.ChatSearch.Input); got != "  @BotFather" {
		t.Fatalf("input = %q", got)
	}
	submitted, commands := updateState(changed, ActionReceived{Action: SubmitChatSearch})
	if len(commands) != 3 {
		t.Fatalf("commands = %#v", commands)
	}
	if submitted.Focus != FocusChatSearchResults || !submitted.ChatSearch.PublicLoading || !submitted.ChatSearch.MessagesLoading || !submitted.ChatSearch.Submitted || submitted.ChatSearch.Query != "BotFather" {
		t.Fatalf("submitted = %#v focus=%v", submitted.ChatSearch, submitted.Focus)
	}
	if submitted.Drafts[7] != "keep draft" {
		t.Fatal("chat search changed the existing draft")
	}

	blank, _ := updateState(opened, ChatSearchValueChanged{Value: " @ "})
	blank, blankCommands := updateState(blank, ActionReceived{Action: SubmitChatSearch})
	if len(blankCommands) != 0 || blank.ChatSearch.Submitted {
		t.Fatalf("blank submit = %#v %#v", blank.ChatSearch, blankCommands)
	}
}

func TestChatSearchOpensOnlyFromChatsAndCloses(t *testing.T) {
	state := chatSearchBaseState()
	state.Focus = FocusConversation
	unchanged, commands := updateState(state, ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || unchanged.ChatSearch != nil {
		t.Fatalf("conversation open = %#v %#v", unchanged.ChatSearch, commands)
	}

	opened, _ := updateState(chatSearchBaseState(), ActionReceived{Action: OpenChatSearch})
	closed, commands := updateState(opened, ActionReceived{Action: Close})
	if len(commands) != 0 || closed.ChatSearch != nil || closed.Focus != FocusChats {
		t.Fatalf("close = %#v focus=%v commands=%#v", closed.ChatSearch, closed.Focus, commands)
	}
}

func TestChatSearchResultsStaleGuardAndEditQuery(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "@botfather"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	found := domain.Chat{ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather", CanSend: true}

	// Stale guard: event with different RequestID should be ignored.
	before := cloneReducerState(submitted)
	stale, commands := updateState(submitted, PublicChatsSearched{RequestID: submitted.ChatSearch.RequestID + 99, Chats: []domain.Chat{found}})
	if len(commands) != 0 || !reflect.DeepEqual(stale, before) {
		t.Fatal("stale result mutated chat search")
	}

	// Loaded: event with matching RequestID should update state.
	requestID := submitted.ChatSearch.RequestID
	loaded, _ := updateState(submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{found}})
	if loaded.ChatSearch.PublicLoading || len(loaded.ChatSearch.PublicChats) != 1 || loaded.ChatSearch.PublicChats[0].ID != 99 {
		t.Fatalf("loaded = %#v", loaded.ChatSearch)
	}
	edited, commands := updateState(loaded, ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || edited.Focus != FocusChatSearchInput || edited.ChatSearch.Submitted || edited.ChatSearch.RequestID != 0 || string(edited.ChatSearch.Input) != "@botfather" {
		t.Fatalf("edit query = %#v focus=%v commands=%#v", edited.ChatSearch, edited.Focus, commands)
	}
	staleAfterEdit, _ := updateState(edited, PublicChatSearchFailed{RequestID: 10, Error: domain.AppError{Message: "raw"}})
	if !reflect.DeepEqual(staleAfterEdit, edited) {
		t.Fatal("invalidated failure mutated edited query")
	}
}

func TestChatSearchActivationAddsSelectsAndOpensChat(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "@botfather"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	found := domain.Chat{
		ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather", CanSend: true,
		Avatar: domain.AvatarRef{FileID: 4, UniqueID: "bot-avatar"},
	}
	requestID := submitted.ChatSearch.RequestID
	// Send both events to clear loading flags.
	loaded, _ := updateState(submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{found}})
	loaded, _ = updateState(loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{}})
	activated, commands := updateState(loaded, ActionReceived{Action: Activate})
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
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "botfather"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	requestID := submitted.ChatSearch.RequestID
	loaded, _ := updateState(submitted, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{{ID: 99, Title: "BotFather"}}})
	loaded, _ = updateState(loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{}})
	unchanged, commands := updateState(loaded, ActionReceived{Action: SelectChat, ChatID: 100})
	if len(commands) != 0 || unchanged.ChatSearch == nil {
		t.Fatalf("wrong result = %#v %#v", unchanged.ChatSearch, commands)
	}
	selected, commands := updateState(loaded, ActionReceived{Action: SelectChat, ChatID: 99})
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
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	changed, _ := updateState(opened, ChatSearchValueChanged{Value: "tele"})
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
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	changed, _ := updateState(opened, ChatSearchValueChanged{Value: "tele"})
	// Now clear.
	cleared, _ := updateState(changed, ChatSearchValueChanged{Value: ""})
	search := cleared.ChatSearch
	if len(search.LocalChats) != 0 {
		t.Fatalf("expected empty local chats, got %v", search.LocalChats)
	}
	if search.PublicChats != nil || search.GlobalMessages != nil {
		t.Fatal("expected PublicChats/GlobalMessages to be nil when blank")
	}
}

func TestChatSearchPublicChatsSearched(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "test"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	publicChats := []domain.Chat{
		{ID: 100, Title: "Public Group", Username: "publicgroup", Order: 20},
	}
	requestID := submitted.ChatSearch.RequestID
	loaded, _ := updateState(submitted, PublicChatsSearched{RequestID: requestID, Chats: publicChats})
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
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "test"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	requestID := submitted.ChatSearch.RequestID
	failed, _ := updateState(submitted, PublicChatsSearchFailed{RequestID: requestID, Error: domain.AppError{Message: "network error"}})
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
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "needle"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	messages := []domain.Message{
		{ID: 1, ChatID: 7, Kind: domain.MessageText, Text: "This has a needle", SentAt: time.Now()},
	}
	requestID := submitted.ChatSearch.RequestID
	loaded, _ := updateState(submitted, AllMessagesSearched{RequestID: requestID, Messages: messages})
	if loaded.ChatSearch.MessagesLoading {
		t.Fatal("MessagesLoading should be false after search")
	}
	if len(loaded.ChatSearch.GlobalMessages) != 1 || loaded.ChatSearch.GlobalMessages[0].ChatID != 7 {
		t.Fatalf("global messages = %v", loaded.ChatSearch.GlobalMessages)
	}
}

func TestChatSearchAllMessagesSearchFailed(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	opened, _ = updateState(opened, ChatSearchValueChanged{Value: "test"})
	submitted, _ := updateState(opened, ActionReceived{Action: SubmitChatSearch})
	requestID := submitted.ChatSearch.RequestID
	failed, _ := updateState(submitted, AllMessagesSearchFailed{RequestID: requestID, Error: domain.AppError{Message: "search error"}})
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
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	typed, commands := updateState(opened, ChatSearchValueChanged{Value: "tele"})
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
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	typed, _ := updateState(opened, ChatSearchValueChanged{Value: "tele"})
	requestID := typed.ChatSearch.RequestID
	loaded, _ := updateState(typed, PublicChatsSearched{RequestID: requestID, Chats: []domain.Chat{{ID: 99, Title: "Pub"}}})
	if loaded.Focus != FocusChatSearchInput || loaded.ChatSearch.PublicLoading || len(loaded.ChatSearch.PublicChats) != 1 {
		t.Fatalf("public arrival = focus %v search %#v", loaded.Focus, loaded.ChatSearch)
	}
	loaded, _ = updateState(loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{{ID: 5, ChatID: 7, Kind: domain.MessageText, Text: "hi"}}})
	if loaded.Focus != FocusChatSearchInput || loaded.ChatSearch.MessagesLoading || len(loaded.ChatSearch.GlobalMessages) != 1 {
		t.Fatalf("messages arrival = focus %v search %#v", loaded.Focus, loaded.ChatSearch)
	}
}

func TestChatSearchMessageActivationJumpsToContext(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	typed, _ := updateState(opened, ChatSearchValueChanged{Value: "needle"})
	requestID := typed.ChatSearch.RequestID
	loaded, _ := updateState(typed, PublicChatsSearched{RequestID: requestID, Chats: nil})
	msg := domain.Message{ID: 20, ChatID: 7, Kind: domain.MessageText, Text: "needle hit", SentAt: time.Now()}
	loaded, _ = updateState(loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{msg}})
	activated, commands := updateState(loaded, ActionReceived{Action: Activate})
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
	landed, _ := updateState(activated, SearchMessageContextLoaded{RequestID: activated.MessageSearch.RequestID, ChatID: 7, MessageID: 20, Page: page})
	if landed.MessageSearch != nil || landed.Focus != FocusConversation || landed.SelectedMessage != 20 || landed.SelectedMessageChat != 7 {
		t.Fatalf("landed = search %#v focus %v sel %v", landed.MessageSearch, landed.Focus, landed.SelectedMessage)
	}
}

func TestChatSearchMouseMessageSelectionJumps(t *testing.T) {
	state := chatSearchBaseState()
	opened, _ := updateState(state, ActionReceived{Action: OpenChatSearch})
	typed, _ := updateState(opened, ChatSearchValueChanged{Value: "needle"})
	requestID := typed.ChatSearch.RequestID
	loaded, _ := updateState(typed, PublicChatsSearched{RequestID: requestID, Chats: nil})
	loaded, _ = updateState(loaded, AllMessagesSearched{RequestID: requestID, Messages: []domain.Message{{ID: 20, ChatID: 7, Kind: domain.MessageText, Text: "hit"}}})
	selected, commands := updateState(loaded, ActionReceived{Action: SelectMessage, ChatID: 7, MessageID: 20})
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
