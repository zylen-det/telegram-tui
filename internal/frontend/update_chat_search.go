package frontend

import (
	"sort"
	"strings"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func openChatSearch(state *State) []Effect {
	if state.Focus != FocusChats || state.ChatSearch != nil {
		return nil
	}
	state.ChatSearch = &ChatSearchState{PreviousFocus: state.Focus}
	state.Focus = FocusChatSearchInput
	return nil
}

func reduceChatSearchValueChanged(state *State, event ChatSearchValueChanged) []Effect {
	active := state.ChatSearch
	if active == nil || state.Focus != FocusChatSearchInput {
		return nil
	}
	search := active
	input := make([]rune, 0, len(event.Value))
	for _, value := range event.Value {
		if value != '\r' && value != '\n' {
			input = append(input, value)
		}
	}
	search.Input = input
	query := strings.TrimSpace(string(search.Input))
	stripped := strings.TrimPrefix(query, "@")
	search.Query = stripped

	if stripped == "" {
		search.Query = ""
		search.LocalChats = nil
		search.PublicChats = nil
		search.GlobalMessages = nil
		search.PublicLoading = false
		search.MessagesLoading = false
		search.PublicError = nil
		search.MessagesError = nil
		search.Selected = 0
		search.Submitted = false
		return nil
	}

	// Synchronous local filter, capped for live preview.
	lowered := strings.ToLower(search.Query)
	matches := make([]domain.Chat, 0, len(state.Chats))
	for _, chat := range state.Chats {
		if strings.Contains(strings.ToLower(chat.Title), lowered) ||
			(chat.Username != "" && strings.Contains(strings.ToLower(chat.Username), lowered)) {
			matches = append(matches, chat)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Order == matches[j].Order {
			return matches[i].ID > matches[j].ID
		}
		return matches[i].Order > matches[j].Order
	})
	if len(matches) > 5 {
		matches = matches[:5]
	}
	search.LocalChats = matches

	// One live generation for both remote sections so stale results die together.
	requestID := allocateRequestID(state)
	search.RequestID = requestID
	search.PublicLoading = true
	search.MessagesLoading = true
	search.PublicError = nil
	search.MessagesError = nil
	search.PublicChats = nil
	search.GlobalMessages = nil
	search.Selected = 0
	search.Submitted = true
	return []Effect{
		SearchPublicChatsCommand{RequestID: requestID, Query: search.Query},
		SearchAllMessagesCommand{RequestID: requestID, Query: search.Query, Limit: 10},
	}
}

func reduceChatSearchAction(state *State, event ActionReceived) []Effect {
	search := state.ChatSearch
	if search == nil {
		return nil
	}
	switch event.Action {
	case Close:
		state.Focus = search.PreviousFocus
		state.ChatSearch = nil
		return nil
	case OpenChatSearch:
		if state.Focus != FocusChatSearchResults {
			return nil
		}
		search.RequestID = 0
		search.PublicLoading = false
		search.MessagesLoading = false
		search.PublicError = nil
		search.MessagesError = nil
		search.Submitted = false
		state.Focus = FocusChatSearchInput
		return nil
	case SubmitChatSearch:
		// Explicit search button / legacy submit: same as Activate in input.
		if state.Focus != FocusChatSearchInput {
			return nil
		}
		return activateChatSearchSelection(state)
	case SelectNext, SelectPrevious:
		if (state.Focus != FocusChatSearchInput && state.Focus != FocusChatSearchResults) || len(flattenChatSearchResults(search)) == 0 {
			return nil
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		count := len(flattenChatSearchResults(search))
		search.Selected = max(0, min(count-1, search.Selected+delta))
		return nil
	case SelectChat:
		if state.Focus != FocusChatSearchInput && state.Focus != FocusChatSearchResults {
			return nil
		}
		for _, chat := range flattenChatSearchChats(search) {
			if chat.ID != 0 && chat.ID == event.ChatID {
				return openChatFromSearchResult(state, chat)
			}
		}
	case SelectMessage:
		if state.Focus != FocusChatSearchInput && state.Focus != FocusChatSearchResults {
			return nil
		}
		for _, msg := range search.GlobalMessages {
			if msg.ChatID == event.ChatID && msg.ID == event.MessageID {
				return openChatFromMessageResult(state, msg)
			}
		}
	case Activate:
		if state.Focus != FocusChatSearchInput && state.Focus != FocusChatSearchResults {
			return nil
		}
		return activateChatSearchSelection(state)
	}
	return nil
}

// activateChatSearchSelection activates the flattened selection, or falls back
// to an exact @username lookup when there is nothing to activate.
func activateChatSearchSelection(state *State) []Effect {
	search := state.ChatSearch
	if search == nil {
		return nil
	}
	flattened := flattenChatSearchResults(search)
	if search.Selected >= 0 && search.Selected < len(flattened) {
		switch r := flattened[search.Selected].(type) {
		case chatResult:
			return openChatFromSearchResult(state, r.Chat)
		case messageResult:
			return openChatFromMessageResult(state, r.Message)
		}
	}
	username := strings.TrimSpace(string(search.Input))
	username = strings.TrimPrefix(username, "@")
	if username == "" {
		return nil
	}
	requestID := allocateRequestID(state)
	search.RequestID = requestID
	search.Query = username
	search.PublicLoading = true
	search.MessagesLoading = true
	search.PublicError = nil
	search.MessagesError = nil
	search.PublicChats = nil
	search.GlobalMessages = nil
	search.Selected = 0
	search.Submitted = true
	state.Focus = FocusChatSearchResults
	return []Effect{
		SearchPublicChatCommand{RequestID: requestID, Username: username},
		SearchPublicChatsCommand{RequestID: requestID, Query: username},
		SearchAllMessagesCommand{RequestID: requestID, Query: username, Limit: 10},
	}
}

func reducePublicChatSearched(state *State, event PublicChatSearched) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	if event.Chat.ID == 0 {
		search.PublicChats = nil
	} else {
		search.PublicChats = []domain.Chat{event.Chat}
	}
	search.PublicLoading = false
	search.PublicError = nil
	clampChatSearchSelection(search)
	return nil
}

func reducePublicChatSearchFailed(state *State, event PublicChatSearchFailed) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	failure := chatSearchError()
	search.PublicError = &failure
	search.PublicLoading = false
	search.PublicChats = nil
	clampChatSearchSelection(search)
	return nil
}

func reducePublicChatsSearched(state *State, event PublicChatsSearched) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	chats := make([]domain.Chat, 0, len(event.Chats))
	for _, chat := range event.Chats {
		if chat.ID != 0 {
			chats = append(chats, chat)
		}
	}
	if len(chats) > 5 {
		chats = chats[:5]
	}
	search.PublicChats = chats
	search.PublicLoading = false
	search.PublicError = nil
	clampChatSearchSelection(search)
	return nil
}

func reducePublicChatsSearchFailed(state *State, event PublicChatsSearchFailed) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Op: "search public chats", Message: "Could not search public chats"}
	search.PublicError = &failure
	search.PublicLoading = false
	search.PublicChats = nil
	clampChatSearchSelection(search)
	return nil
}

func reduceAllMessagesSearched(state *State, event AllMessagesSearched) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	messages := make([]domain.Message, 0, len(event.Messages))
	for _, msg := range event.Messages {
		if msg.ChatID != 0 && msg.ID != 0 {
			messages = append(messages, cloneDomainMessage(msg))
		}
	}
	if len(messages) > 5 {
		messages = messages[:5]
	}
	search.GlobalMessages = messages
	search.MessagesLoading = false
	search.MessagesError = nil
	clampChatSearchSelection(search)
	return nil
}

func reduceAllMessagesSearchFailed(state *State, event AllMessagesSearchFailed) []Effect {
	search := state.ChatSearch
	if search == nil || search.RequestID != event.RequestID {
		return nil
	}
	failure := domain.AppError{Kind: domain.ErrorNetwork, Op: "search all messages", Message: "Could not search messages"}
	search.MessagesError = &failure
	search.MessagesLoading = false
	search.GlobalMessages = nil
	clampChatSearchSelection(search)
	return nil
}

func chatSearchError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "search public chat", Message: "Could not find public chat"}
}

// chatResult represents a chat-based search result.
type chatResult struct {
	Chat domain.Chat
}

// messageResult represents a message-based search result.
type messageResult struct {
	Message domain.Message
}

// chatResultWithID is an interface that both chatResult and messageResult satisfy.
type chatResultWithID interface {
	chatID() domain.ChatID
}

func (r chatResult) chatID() domain.ChatID    { return r.Chat.ID }
func (r messageResult) chatID() domain.ChatID { return r.Message.ChatID }

// flattenChatSearchResults assembles the unified results list in display order:
// local chats, global messages, public chats.
func flattenChatSearchResults(search *ChatSearchState) []chatResultWithID {
	return buildChatSearchResults(search, nil)
}

// buildChatSearchResults assembles the unified results list from local chats,
// global messages, and public chats. allChats is unused and kept for
// backward compatibility with existing callers.
func buildChatSearchResults(search *ChatSearchState, _ []domain.Chat) []chatResultWithID {
	if search == nil {
		return nil
	}
	results := make([]chatResultWithID, 0, len(search.LocalChats)+len(search.GlobalMessages)+len(search.PublicChats))
	for _, chat := range search.LocalChats {
		if chat.ID != 0 {
			results = append(results, chatResult{Chat: chat})
		}
	}
	for _, msg := range search.GlobalMessages {
		if msg.ChatID != 0 && msg.ID != 0 {
			results = append(results, messageResult{Message: msg})
		}
	}
	for _, chat := range search.PublicChats {
		if chat.ID != 0 {
			results = append(results, chatResult{Chat: chat})
		}
	}
	return results
}

func flattenChatSearchChats(search *ChatSearchState) []domain.Chat {
	if search == nil {
		return nil
	}
	chats := make([]domain.Chat, 0, len(search.LocalChats)+len(search.PublicChats))
	chats = append(chats, search.LocalChats...)
	chats = append(chats, search.PublicChats...)
	return chats
}

func clampChatSearchSelection(search *ChatSearchState) {
	count := len(flattenChatSearchResults(search))
	if count == 0 {
		search.Selected = 0
		return
	}
	search.Selected = max(0, min(count-1, search.Selected))
}

// openChatFromSearchResult opens the chat identified by the search result
// and navigates back to the previous focus.
func openChatFromSearchResult(state *State, chat domain.Chat) []Effect {
	previousID, hadPrevious := activeChatID(*state)
	focusedID, hadFocused := focusedChatID(*state)
	upsertChat(&state.Chats, chat)
	sortChats(state.Chats)
	if hadPrevious {
		preserveChatSelection(state, previousID)
	}
	if hadFocused {
		preserveChatFocus(state, focusedID)
	}

	state.ChatSearch = nil
	state.Focus = FocusChats
	commands := reduceAction(state, ActionReceived{Action: SelectChat, ChatID: chat.ID})
	if chatIndex(state.Chats, chat.ID) < 0 {
		return nil
	}
	openCommands := openActiveChatFromList(state)
	commands = append(commands, openCommands...)
	commands = append(commands, requestMissingChatAvatars(state, []domain.Chat{chat})...)
	return commands
}

// openChatFromMessageResult selects the containing chat and jumps to the
// message context so the user lands on the exact matched message.
func openChatFromMessageResult(state *State, msg domain.Message) []Effect {
	previousID, hadPrevious := activeChatID(*state)
	focusedID, hadFocused := focusedChatID(*state)
	chat := findChatByID(state.Chats, msg.ChatID)
	if chat.ID == 0 {
		chat = domain.Chat{ID: msg.ChatID}
	}
	upsertChat(&state.Chats, chat)
	sortChats(state.Chats)
	if hadPrevious {
		preserveChatSelection(state, previousID)
	}
	if hadFocused {
		preserveChatFocus(state, focusedID)
	}

	state.ChatSearch = nil
	state.Focus = FocusChats
	commands := reduceAction(state, ActionReceived{Action: SelectChat, ChatID: msg.ChatID})
	if chatIndex(state.Chats, msg.ChatID) < 0 {
		return nil
	}
	// Landing from a chat search targets a single message: leave ALL mode.
	if state.ShowAll == nil {
		state.ShowAll = make(map[domain.ChatID]bool)
	}
	state.ShowAll[msg.ChatID] = false
	if msg.TopicID != 0 {
		// The matched message lives in a forum topic: require a forum chat,
		// select the topic, and jump through the topic-scoped context load.
		if !state.Chats[state.SelectedChat].IsForum {
			return nil
		}
		if state.ForumTopics == nil {
			state.ForumTopics = make(map[domain.ChatID]map[domain.TopicID]domain.ForumTopic)
		}
		if state.ForumTopics[msg.ChatID] == nil {
			state.ForumTopics[msg.ChatID] = make(map[domain.TopicID]domain.ForumTopic)
		}
		if _, tracked := state.ForumTopics[msg.ChatID][msg.TopicID]; !tracked {
			state.ForumTopics[msg.ChatID][msg.TopicID] = domain.ForumTopic{ID: msg.TopicID, ChatID: msg.ChatID, Name: "Topic"}
		}
		if state.SelectedTopics == nil {
			state.SelectedTopics = make(map[domain.ChatID]domain.TopicID)
		}
		state.SelectedTopics[msg.ChatID] = msg.TopicID
		requestID := allocateRequestID(state)
		state.MessageSearch = &MessageSearchState{
			RequestID:     requestID,
			ChatID:        msg.ChatID,
			TopicID:       msg.TopicID,
			Query:         "",
			PreviousFocus: FocusConversation,
			Loading:       true,
			Submitted:     true,
			JumpMessageID: msg.ID,
		}
		state.Focus = FocusConversation
		commands = append(commands, LoadSearchMessageContext{
			RequestID: requestID,
			ChatID:    msg.ChatID,
			TopicID:   msg.TopicID,
			MessageID: msg.ID,
		})
		return commands
	}
	requestID := allocateRequestID(state)
	state.MessageSearch = &MessageSearchState{
		RequestID:     requestID,
		ChatID:        msg.ChatID,
		Query:         "",
		PreviousFocus: FocusConversation,
		Loading:       true,
		Submitted:     true,
		JumpMessageID: msg.ID,
	}
	state.Focus = FocusConversation
	commands = append(commands, LoadSearchMessageContext{
		RequestID: requestID,
		ChatID:    msg.ChatID,
		MessageID: msg.ID,
	})
	return commands
}

// findChatByID finds a chat by ID in the slice, or returns an empty Chat.
func findChatByID(chats []domain.Chat, id domain.ChatID) domain.Chat {
	for _, chat := range chats {
		if chat.ID == id {
			return chat
		}
	}
	return domain.Chat{}
}
