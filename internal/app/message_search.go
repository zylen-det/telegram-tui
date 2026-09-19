package app

import (
	"strings"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func openMessageSearch(state State) (State, []Command) {
	chatID, ok := activeChatID(state)
	if !ok || state.Focus != FocusConversation {
		return state, nil
	}
	chat := state.Chats[state.SelectedChat]
	if chat.IsForum && state.SelectedTopics[chatID] == 0 && !state.ShowAll[chatID] {
		return state, nil
	}
	topicID := domain.TopicID(0)
	if chat.IsForum {
		// ALL mode keeps TopicID 0 (whole-chat search); otherwise a specific
		// topic is selected.
		topicID = state.SelectedTopics[chatID]
	}
	state.MessageSearch = &MessageSearchState{ChatID: chatID, TopicID: topicID, PreviousFocus: state.Focus}
	state.Focus = FocusSearchInput
	return state, nil
}

func reduceMessageSearchValueChanged(state State, event MessageSearchValueChanged) (State, []Command) {
	active := state.MessageSearch
	if active == nil || state.Focus != FocusSearchInput || active.ChatID != event.ChatID {
		return state, nil
	}
	// Copy-on-write: clone the owned MessageSearchState so the caller's State
	// keeps the previous query and result slices.
	search := *active
	search.Input = []rune(event.Value)
	state.MessageSearch = &search
	return state, nil
}

func reduceMessageSearchAction(state State, event ActionReceived) (State, []Command) {
	search := state.MessageSearch
	if search == nil {
		return state, nil
	}
	switch event.Action {
	case Close:
		state.Focus = search.PreviousFocus
		state.MessageSearch = nil
		return state, nil
	case SelectChat:
		state.Focus = FocusConversation
		state.MessageSearch = nil
		return reduceAction(state, event)
	case OpenMessageSearch:
		// From results, '/' returns to the query and invalidates in-flight work.
		search.RequestID = allocateRequestID(&state)
		search.Loading = false
		search.JumpMessageID = 0
		search.Error = nil
		state.Focus = FocusSearchInput
		return state, nil
	case SubmitMessageSearch:
		if state.Focus != FocusSearchInput {
			return state, nil
		}
		query := strings.TrimSpace(string(search.Input))
		if query == "" {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		search.RequestID = requestID
		search.Query = query
		search.Loading = true
		search.Error = nil
		search.Results = nil
		search.Selected = 0
		search.NextFromMessageID = 0
		search.TotalCount = 0
		search.Done = false
		search.Submitted = true
		search.JumpMessageID = 0
		state.Focus = FocusSearchResults
		return state, []Command{SearchChatMessages{
			RequestID: requestID,
			ChatID:    search.ChatID,
			TopicID:   search.TopicID,
			Query:     query,
			Cursor:    telegram.MessageSearchCursor{Limit: pageSize},
		}}
	case SelectMessage:
		if state.Focus != FocusSearchResults || event.ChatID != search.ChatID {
			return state, nil
		}
		for index := range search.Results {
			if search.Results[index].ID == event.MessageID {
				search.Selected = index
				return maybePaginateMessageSearch(state)
			}
		}
		return state, nil
	case SelectNext, SelectPrevious:
		if state.Focus != FocusSearchResults || len(search.Results) == 0 || search.JumpMessageID != 0 {
			return state, nil
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		search.Selected = max(0, min(len(search.Results)-1, search.Selected+delta))
		return maybePaginateMessageSearch(state)
	case Activate:
		if state.Focus != FocusSearchResults || search.Loading || search.Selected < 0 || search.Selected >= len(search.Results) {
			return state, nil
		}
		target := search.Results[search.Selected].ID
		if target == 0 {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		search.RequestID = requestID
		search.JumpMessageID = target
		search.Loading = true
		search.Error = nil
		return state, []Command{LoadSearchMessageContext{RequestID: requestID, ChatID: search.ChatID, TopicID: search.TopicID, MessageID: target}}
	}
	return state, nil
}

func maybePaginateMessageSearch(state State) (State, []Command) {
	search := state.MessageSearch
	if search == nil || search.Loading || search.Done || search.NextFromMessageID == 0 || len(search.Results) == 0 || search.Selected != len(search.Results)-1 {
		return state, nil
	}
	requestID := allocateRequestID(&state)
	search.RequestID = requestID
	search.Loading = true
	search.Error = nil
	return state, []Command{SearchChatMessages{
		RequestID: requestID,
		ChatID:    search.ChatID,
		TopicID:   search.TopicID,
		Query:     search.Query,
		Cursor: telegram.MessageSearchCursor{
			FromMessageID: search.NextFromMessageID,
			Limit:         pageSize,
		},
	}}
}

func reduceChatMessagesSearched(state State, event ChatMessagesSearched) (State, []Command) {
	search := state.MessageSearch
	if search == nil || search.RequestID != event.RequestID || search.ChatID != event.ChatID || search.TopicID != event.TopicID || search.JumpMessageID != 0 {
		return state, nil
	}
	seen := make(map[domain.MessageID]struct{}, len(search.Results)+len(event.Page.Messages))
	for _, message := range search.Results {
		seen[message.ID] = struct{}{}
	}
	for _, message := range event.Page.Messages {
		if message.ChatID != search.ChatID || message.TopicID != event.TopicID || message.ID == 0 {
			continue
		}
		if _, exists := seen[message.ID]; exists {
			continue
		}
		seen[message.ID] = struct{}{}
		search.Results = append(search.Results, cloneDomainMessage(message))
	}
	search.NextFromMessageID = event.Page.NextFromMessageID
	search.TotalCount = event.Page.TotalCount
	search.Done = event.Page.Done || event.Page.NextFromMessageID == 0
	search.Loading = false
	search.Error = nil
	if len(search.Results) == 0 {
		search.Selected = 0
	} else {
		search.Selected = max(0, min(len(search.Results)-1, search.Selected))
	}
	state.Focus = FocusSearchResults
	return state, nil
}

func reduceChatMessagesSearchFailed(state State, event ChatMessagesSearchFailed) (State, []Command) {
	search := state.MessageSearch
	if search == nil || search.RequestID != event.RequestID || search.ChatID != event.ChatID || search.TopicID != event.TopicID || search.JumpMessageID != 0 {
		return state, nil
	}
	failure := messageSearchError()
	search.Error = &failure
	search.Loading = false
	state.Focus = FocusSearchResults
	return state, nil
}

func reduceSearchMessageContextLoaded(state State, event SearchMessageContextLoaded) (State, []Command) {
	search := state.MessageSearch
	if search == nil || search.RequestID != event.RequestID || search.ChatID != event.ChatID || search.TopicID != event.TopicID || search.JumpMessageID != event.MessageID {
		return state, nil
	}
	found := false
	for _, message := range event.Page.Messages {
		if message.ChatID == event.ChatID && message.TopicID == event.TopicID && message.ID == event.MessageID {
			found = true
			break
		}
	}
	if !found {
		return reduceSearchMessageContextFailed(state, SearchMessageContextFailed{
			RequestID: event.RequestID,
			ChatID:    event.ChatID,
			MessageID: event.MessageID,
			Error:     messageContextError(),
		})
	}
	merged := mergeMessages(state.Messages[event.ChatID], event.Page.Messages)
	merged = limitMessagesAround(merged, event.MessageID, maxMessagesPerChat)
	state.Messages[event.ChatID] = merged
	index := messageIndex(merged, event.MessageID)
	state.SelectedMessageChat = event.ChatID
	state.SelectedMessage = event.MessageID
	history := state.History[event.ChatID]
	history.Loading = false
	history.Error = nil
	if len(merged) > 0 {
		history.OldestID = merged[0].ID
	}
	history.ViewOffset = max(0, len(merged)-1-index)
	state.History[event.ChatID] = history
	state.Focus = FocusConversation
	state.MessageSearch = nil
	commands := requestMissingMessageAvatars(&state, event.Page.Messages)
	commands = append(commands, requestMissingThumbnails(&state, event.Page.Messages)...)
	return state, commands
}

func reduceSearchMessageContextFailed(state State, event SearchMessageContextFailed) (State, []Command) {
	search := state.MessageSearch
	if search == nil || search.RequestID != event.RequestID || search.ChatID != event.ChatID || search.TopicID != event.TopicID || search.JumpMessageID != event.MessageID {
		return state, nil
	}
	failure := messageContextError()
	search.Error = &failure
	search.Loading = false
	search.JumpMessageID = 0
	state.Focus = FocusSearchResults
	setToast(&state, failure, 4*time.Second)
	return state, nil
}

func limitMessagesAround(messages []domain.Message, target domain.MessageID, limit int) []domain.Message {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	index := messageIndex(messages, target)
	if index < 0 {
		return messages[len(messages)-limit:]
	}
	start := max(0, index-limit/2)
	end := min(len(messages), start+limit)
	start = max(0, end-limit)
	return append([]domain.Message(nil), messages[start:end]...)
}

func openPinnedMessages(state State) (State, []Command) {
	chatID, ok := activeChatID(state)
	if !ok || state.Focus != FocusConversation {
		return state, nil
	}
	chat := state.Chats[state.SelectedChat]
	if chat.IsForum && state.SelectedTopics[chatID] == 0 && !state.ShowAll[chatID] {
		return state, nil
	}
	topicID := domain.TopicID(0)
	if chat.IsForum {
		topicID = state.SelectedTopics[chatID]
	}
	requestID := allocateRequestID(&state)
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:            chatID,
		TopicID:           topicID,
		RequestID:         requestID,
		PreviousFocus:     state.Focus,
		Loading:           true,
		Error:             nil,
		Results:           nil,
		Selected:          0,
		NextFromMessageID: 0,
		TotalCount:        0,
		Done:              false,
		JumpMessageID:     0,
	}
	state.Focus = FocusPinnedResults
	return state, []Command{LoadPinnedMessages{
		RequestID: requestID,
		ChatID:    chatID,
		TopicID:   topicID,
		Cursor:    telegram.MessageSearchCursor{Limit: pageSize},
	}}
}

func reducePinnedMessagesAction(state State, event ActionReceived) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil {
		return state, nil
	}
	switch event.Action {
	case Close:
		state.Focus = pinned.PreviousFocus
		state.PinnedMessages = nil
		return state, nil
	case SelectChat:
		state.Focus = FocusConversation
		state.PinnedMessages = nil
		return reduceAction(state, event)
	case SelectNext, SelectPrevious:
		if state.Focus != FocusPinnedResults || pinned.Loading || len(pinned.Results) == 0 || pinned.JumpMessageID != 0 {
			return state, nil
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		pinned.Selected = max(0, min(len(pinned.Results)-1, pinned.Selected+delta))
		return maybePaginatePinnedMessages(state)
	case Activate:
		if state.Focus != FocusPinnedResults || pinned.Loading || pinned.Selected < 0 || pinned.Selected >= len(pinned.Results) {
			return state, nil
		}
		target := pinned.Results[pinned.Selected].ID
		if target == 0 {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		pinned.RequestID = requestID
		pinned.JumpMessageID = target
		pinned.Loading = true
		pinned.Error = nil
		return state, []Command{LoadPinnedMessageContext{RequestID: requestID, ChatID: pinned.ChatID, TopicID: pinned.TopicID, MessageID: target}}
	case SelectMessage:
		if state.Focus != FocusPinnedResults || event.ChatID != pinned.ChatID {
			return state, nil
		}
		for index := range pinned.Results {
			if pinned.Results[index].ID == event.MessageID {
				pinned.Selected = index
				return maybePaginatePinnedMessages(state)
			}
		}
		return state, nil
	}
	return state, nil
}

func maybePaginatePinnedMessages(state State) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil || pinned.Loading || pinned.Done || pinned.NextFromMessageID == 0 || len(pinned.Results) == 0 || pinned.Selected != len(pinned.Results)-1 {
		return state, nil
	}
	requestID := allocateRequestID(&state)
	pinned.RequestID = requestID
	pinned.Loading = true
	pinned.Error = nil
	return state, []Command{LoadPinnedMessages{
		RequestID: requestID,
		ChatID:    pinned.ChatID,
		TopicID:   pinned.TopicID,
		Cursor: telegram.MessageSearchCursor{
			FromMessageID: pinned.NextFromMessageID,
			Limit:         pageSize,
		},
	}}
}

func reducePinnedMessagesLoaded(state State, event PinnedMessagesLoaded) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil || pinned.RequestID != event.RequestID || pinned.ChatID != event.ChatID || pinned.TopicID != event.TopicID || pinned.JumpMessageID != 0 {
		return state, nil
	}
	seen := make(map[domain.MessageID]struct{}, len(pinned.Results)+len(event.Page.Messages))
	for _, message := range pinned.Results {
		seen[message.ID] = struct{}{}
	}
	for _, message := range event.Page.Messages {
		if message.ChatID != pinned.ChatID || message.TopicID != event.TopicID || message.ID == 0 {
			continue
		}
		if _, exists := seen[message.ID]; exists {
			continue
		}
		seen[message.ID] = struct{}{}
		pinned.Results = append(pinned.Results, cloneDomainMessage(message))
	}
	pinned.NextFromMessageID = event.Page.NextFromMessageID
	pinned.TotalCount = event.Page.TotalCount
	pinned.Done = event.Page.Done || event.Page.NextFromMessageID == 0
	pinned.Loading = false
	pinned.Error = nil
	if len(pinned.Results) == 0 {
		pinned.Selected = 0
	} else {
		pinned.Selected = max(0, min(len(pinned.Results)-1, pinned.Selected))
	}
	state.Focus = FocusPinnedResults
	return state, nil
}

func reducePinnedMessagesLoadFailed(state State, event PinnedMessagesLoadFailed) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil || pinned.RequestID != event.RequestID || pinned.ChatID != event.ChatID || pinned.TopicID != event.TopicID || pinned.JumpMessageID != 0 {
		return state, nil
	}
	failure := pinnedSearchError()
	pinned.Error = &failure
	pinned.Loading = false
	state.Focus = FocusPinnedResults
	return state, nil
}

func reducePinnedMessageContextLoaded(state State, event PinnedMessageContextLoaded) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil || pinned.RequestID != event.RequestID || pinned.ChatID != event.ChatID || pinned.TopicID != event.TopicID || pinned.JumpMessageID != event.MessageID {
		return state, nil
	}
	found := false
	for _, message := range event.Page.Messages {
		if message.ChatID == event.ChatID && message.TopicID == event.TopicID && message.ID == event.MessageID {
			found = true
			break
		}
	}
	if !found {
		return reducePinnedMessageContextFailed(state, PinnedMessageContextFailed{
			RequestID: event.RequestID,
			ChatID:    event.ChatID,
			MessageID: event.MessageID,
			Error:     messageContextError(),
		})
	}
	merged := mergeMessages(state.Messages[event.ChatID], event.Page.Messages)
	merged = limitMessagesAround(merged, event.MessageID, maxMessagesPerChat)
	state.Messages[event.ChatID] = merged
	index := messageIndex(merged, event.MessageID)
	state.SelectedMessageChat = event.ChatID
	state.SelectedMessage = event.MessageID
	history := state.History[event.ChatID]
	history.Loading = false
	history.Error = nil
	if len(merged) > 0 {
		history.OldestID = merged[0].ID
	}
	history.ViewOffset = max(0, len(merged)-1-index)
	state.History[event.ChatID] = history
	state.Focus = FocusConversation
	state.PinnedMessages = nil
	commands := requestMissingMessageAvatars(&state, event.Page.Messages)
	commands = append(commands, requestMissingThumbnails(&state, event.Page.Messages)...)
	return state, commands
}

func pinnedSearchError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load pinned messages", Message: "Could not load pinned messages"}
}

func pinnedContextError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load pinned message context", Message: "Could not open pinned message"}
}

func reducePinnedMessageContextFailed(state State, event PinnedMessageContextFailed) (State, []Command) {
	pinned := state.PinnedMessages
	if pinned == nil || pinned.RequestID != event.RequestID || pinned.ChatID != event.ChatID || pinned.TopicID != event.TopicID || pinned.JumpMessageID != event.MessageID {
		return state, nil
	}
	failure := pinnedContextError()
	pinned.Error = &failure
	pinned.Loading = false
	pinned.JumpMessageID = 0
	state.Focus = FocusPinnedResults
	setToast(&state, failure, 4*time.Second)
	return state, nil
}
