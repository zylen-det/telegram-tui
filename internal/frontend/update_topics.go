package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

// openTopics opens the forum-topic list for the active chat. It only runs
// from the chat list or conversation focus and only for forum chats.
func openTopics(state *State) []Effect {
	chatID, ok := activeChatID(*state)
	if !ok || !state.Chats[state.SelectedChat].IsForum {
		return nil
	}
	if state.Focus != FocusChats && state.Focus != FocusConversation {
		return nil
	}
	requestID := allocateRequestID(state)
	state.Topics = &TopicListState{
		RequestID:     requestID,
		ChatID:        chatID,
		PreviousFocus: state.Focus,
		Loading:       true,
	}
	state.Focus = FocusTopics
	return []Effect{LoadTopics{
		RequestID: requestID,
		ChatID:    chatID,
		Cursor:    telegram.TopicCursor{Limit: pageSize},
	}}
}

// reduceTopicsAction owns every key while the topic list is open. It mirrors
// the members modal key ownership: the list never closes on its own, the
// user selects a row to leave.
func reduceTopicsAction(state *State, event ActionReceived) []Effect {
	topics := state.Topics
	if topics == nil {
		return nil
	}
	switch event.Action {
	case Close:
		state.Focus = topics.PreviousFocus
		state.Topics = nil
	case SelectNext, SelectPrevious:
		// Row 0 is the ALL pseudo-row, so the clamp bound is len(Results).
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		topics.Selected = max(0, min(len(topics.Results), topics.Selected+delta))
		if topics.Selected == len(topics.Results) && !topics.Done && !topics.Loading {
			return requestNextTopicsPage(state)
		}
	case Activate:
		index := max(0, min(topics.Selected, len(topics.Results)))
		if index == 0 {
			chatID, _ := activeChatID(*state)
			return activateAllTopics(state, chatID)
		}
		if index > len(topics.Results) {
			return nil
		}
		return activateTopic(state, topics.Results[index-1])
	case SelectAllMessages:
		chatID := topics.ChatID
		if state.SelectedChat < 0 || state.SelectedChat >= len(state.Chats) || state.Chats[state.SelectedChat].ID != chatID || !state.Chats[state.SelectedChat].IsForum {
			return nil
		}
		return activateAllTopics(state, chatID)
	case SelectTopic:
		if event.ChatID != topics.ChatID {
			return nil
		}
		if event.TopicID == 0 {
			return activateAllTopics(state, topics.ChatID)
		}
		for index := range topics.Results {
			if topics.Results[index].ID == event.TopicID {
				topics.Selected = index + 1
				return activateTopic(state, topics.Results[index])
			}
		}
	}
	return nil
}

// generalTopicID resolves the forum's General topic ID: the tracked topic
// flagged IsGeneral, falling back to TDLib's General convention of 1.
func generalTopicID(state State, chatID domain.ChatID) domain.TopicID {
	for topicID, topic := range state.ForumTopics[chatID] {
		if topic.IsGeneral {
			return topicID
		}
	}
	return 1
}

// showAllActive reports the active chat's ALL-messages key. TopicID 0 is
// reserved for the ALL pseudo-row: real topics start at 1 and chat-level
// drafts use separate maps, so the key never collides.
func showAllActive(state State) (topicKey, bool) {
	chatID, ok := activeChatID(state)
	if !ok {
		return topicKey{}, false
	}
	if !state.Chats[state.SelectedChat].IsForum || !state.ShowAll[chatID] {
		return topicKey{}, false
	}
	return topicKey{ChatID: chatID, TopicID: 0}, true
}

// activateAllTopics closes the topic list and switches the forum chat into
// ALL-messages mode. The ALL draft at TopicID 0 persists; nothing is seeded
// or deleted here. History reuses the shared chat history path.
func activateAllTopics(state *State, chatID domain.ChatID) []Effect {
	if state.ShowAll == nil {
		state.ShowAll = make(map[domain.ChatID]bool)
	}
	state.ShowAll[chatID] = true
	if state.SelectedTopics == nil {
		state.SelectedTopics = make(map[domain.ChatID]domain.TopicID)
	}
	delete(state.SelectedTopics, chatID)
	state.ReplyTarget = nil
	state.EditTarget = nil
	state.MessageMenu = nil
	state.CommandMenu = nil
	state.Topics = nil
	state.Focus = FocusConversation
	if id := newestMessageID(state.Messages[chatID]); id != 0 {
		state.SelectedMessageChat = chatID
		state.SelectedMessage = id
	} else {
		state.SelectedMessageChat = 0
		state.SelectedMessage = 0
	}
	if _, exists := state.History[chatID]; !exists {
		requestID := allocateRequestID(state)
		state.History[chatID] = HistoryState{Loading: true, RequestID: requestID}
		return []Effect{LoadMessages{RequestID: requestID, ChatID: chatID, Cursor: telegram.MessageCursor{Limit: pageSize}}}
	}
	return nil
}

func requestNextTopicsPage(state *State) []Effect {
	topics := state.Topics
	if topics == nil || topics.Loading || topics.Done {
		return nil
	}
	requestID := allocateRequestID(state)
	topics.RequestID = requestID
	topics.Loading = true
	topics.Error = nil
	return []Effect{LoadTopics{
		RequestID: requestID,
		ChatID:    topics.ChatID,
		Cursor:    topics.NextCursor,
	}}
}

// activateTopic closes the topic list and makes the given topic the selected
// topic of its forum chat.
func activateTopic(state *State, topic domain.ForumTopic) []Effect {
	if state.ForumTopics == nil {
		state.ForumTopics = make(map[domain.ChatID]map[domain.TopicID]domain.ForumTopic)
	}
	if state.ForumTopics[topic.ChatID] == nil {
		state.ForumTopics[topic.ChatID] = make(map[domain.TopicID]domain.ForumTopic)
	}
	state.ForumTopics[topic.ChatID][topic.ID] = topic
	if state.ShowAll == nil {
		state.ShowAll = make(map[domain.ChatID]bool)
	}
	state.ShowAll[topic.ChatID] = false
	if state.SelectedTopics == nil {
		state.SelectedTopics = make(map[domain.ChatID]domain.TopicID)
	}
	state.SelectedTopics[topic.ChatID] = topic.ID
	seedTopicDraft(state, topicKey{ChatID: topic.ChatID, TopicID: topic.ID}, topic.Draft)
	state.ReplyTarget = nil
	state.EditTarget = nil
	state.MessageMenu = nil
	state.CommandMenu = nil
	state.Topics = nil
	state.Focus = FocusConversation

	if id := newestTopicMessageID(state.Messages[topic.ChatID], topic.ID); id != 0 {
		state.SelectedMessageChat = topic.ChatID
		state.SelectedMessage = id
	} else {
		state.SelectedMessageChat = 0
		state.SelectedMessage = 0
	}

	key := topicKey{ChatID: topic.ChatID, TopicID: topic.ID}
	if _, exists := state.TopicHistory[key]; !exists {
		requestID := allocateRequestID(state)
		state.TopicHistory[key] = HistoryState{Loading: true, RequestID: requestID}
		return []Effect{LoadMessages{
			RequestID: requestID,
			ChatID:    topic.ChatID,
			TopicID:   topic.ID,
			Cursor:    telegram.MessageCursor{Limit: pageSize},
		}}
	}
	return nil
}

func seedTopicDraft(state *State, key topicKey, draft domain.Draft) {
	if state.TopicDrafts == nil {
		state.TopicDrafts = make(map[topicKey]string)
	}
	if state.TopicDraftReplies == nil {
		state.TopicDraftReplies = make(map[topicKey]domain.MessageID)
	}
	if state.TopicDraftDates == nil {
		state.TopicDraftDates = make(map[topicKey]int64)
	}
	state.TopicDrafts[key] = draft.Text
	if draft.ReplyToMessageID > 0 {
		state.TopicDraftReplies[key] = draft.ReplyToMessageID
	} else {
		delete(state.TopicDraftReplies, key)
	}
	if draft.Date > 0 {
		state.TopicDraftDates[key] = draft.Date
	} else {
		delete(state.TopicDraftDates, key)
	}
}

func newestTopicMessageID(messages []domain.Message, topicID domain.TopicID) domain.MessageID {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].TopicID == topicID {
			return messages[index].ID
		}
	}
	return 0
}

func reduceTopicsLoaded(state *State, event TopicsLoaded) []Effect {
	topics := state.Topics
	if topics == nil || topics.RequestID != event.RequestID || topics.ChatID != event.ChatID {
		return nil
	}
	seen := make(map[domain.TopicID]int, len(topics.Results)+len(event.Page.Topics))
	for index, topic := range topics.Results {
		seen[topic.ID] = index
	}
	for _, topic := range event.Page.Topics {
		if topic.ID == 0 {
			continue
		}
		if index, exists := seen[topic.ID]; exists {
			topics.Results[index] = topic
			continue
		}
		seen[topic.ID] = len(topics.Results)
		topics.Results = append(topics.Results, topic)
	}
	// The ALL pseudo-row is index 0, so the selection clamps to
	// len(Results), not len(Results)-1.
	topics.Selected = max(0, min(len(topics.Results), topics.Selected))
	topics.NextCursor = telegram.TopicCursor{
		OffsetDate:      event.Page.NextOffsetDate,
		OffsetMessageID: event.Page.NextOffsetMessageID,
		OffsetTopicID:   event.Page.NextOffsetTopicID,
		Limit:           pageSize,
	}
	topics.TotalCount = event.Page.TotalCount
	topics.Done = event.Page.Done
	topics.Loading = false
	topics.Error = nil
	topics.Selected = max(0, min(len(topics.Results)-1, topics.Selected))
	state.Focus = FocusTopics
	return nil
}

func reduceTopicsLoadFailed(state *State, event TopicsLoadFailed) []Effect {
	topics := state.Topics
	if topics == nil || topics.RequestID != event.RequestID || topics.ChatID != event.ChatID {
		return nil
	}
	failure := event.Error
	topics.Error = &failure
	topics.Loading = false
	state.Focus = FocusTopics
	return nil
}

// reduceTopicMessagesLoaded mirrors reduceMessagesLoaded for topic-scoped
// loads. Messages merge into the shared chat slice; only TopicHistory tracks
// the topic request.
func reduceTopicMessagesLoaded(state *State, event MessagesLoaded) []Effect {
	key := topicKey{ChatID: event.ChatID, TopicID: event.TopicID}
	history, exists := state.TopicHistory[key]
	if !exists {
		activeID, active := activeChatID(*state)
		if !active || activeID != event.ChatID || state.SelectedTopics[event.ChatID] != event.TopicID {
			return nil
		}
		history = HistoryState{RequestID: event.RequestID}
	} else if history.RequestID != event.RequestID {
		return nil
	}
	history.Loading = false
	history.Error = nil
	history.Done = event.Page.Done
	if id := topicOldestMessageID(state.Messages[event.ChatID], event.TopicID); id != 0 {
		history.OldestID = id
	}
	history.ViewOffset = clampOffset(history.ViewOffset, len(state.Messages[event.ChatID]))
	if state.TopicHistory == nil {
		state.TopicHistory = make(map[topicKey]HistoryState)
	}
	state.TopicHistory[key] = history
	if activeID, active := activeChatID(*state); active && activeID == event.ChatID && state.SelectedMessage == 0 {
		if id := newestTopicMessageID(state.Messages[event.ChatID], event.TopicID); id != 0 {
			state.SelectedMessage = id
			state.SelectedMessageChat = event.ChatID
		}
	}
	commands := requestMissingMessageAvatars(state, event.Page.Messages)
	commands = append(commands, requestMissingThumbnails(state, event.Page.Messages)...)
	return commands
}

func topicOldestMessageID(messages []domain.Message, topicID domain.TopicID) domain.MessageID {
	for _, message := range messages {
		if message.TopicID == topicID {
			return message.ID
		}
	}
	return 0
}

func reduceForumTopicInfoChanged(state *State, update telegram.ForumTopicInfoChanged) []Effect {
	topic := update.Topic
	if topic.ChatID == 0 || topic.ID == 0 {
		return nil
	}
	if chatIndex(state.Chats, topic.ChatID) >= 0 || state.ForumTopics[topic.ChatID][topic.ID].ID != 0 {
		if state.ForumTopics == nil {
			state.ForumTopics = make(map[domain.ChatID]map[domain.TopicID]domain.ForumTopic)
		}
		byTopic := state.ForumTopics[topic.ChatID]
		if byTopic == nil {
			byTopic = make(map[domain.TopicID]domain.ForumTopic)
			state.ForumTopics[topic.ChatID] = byTopic
		}
		existing := byTopic[topic.ID]
		existing.ID = topic.ID
		existing.ChatID = topic.ChatID
		existing.Name = topic.Name
		existing.IconColor = topic.IconColor
		existing.IsGeneral = topic.IsGeneral
		existing.IsClosed = topic.IsClosed
		existing.IsHidden = topic.IsHidden
		byTopic[topic.ID] = existing
	}
	if topics := state.Topics; topics != nil && topics.ChatID == topic.ChatID {
		for index := range topics.Results {
			if topics.Results[index].ID != topic.ID {
				continue
			}
			row := topics.Results[index]
			row.Name = topic.Name
			row.IconColor = topic.IconColor
			row.IsGeneral = topic.IsGeneral
			row.IsClosed = topic.IsClosed
			row.IsHidden = topic.IsHidden
			topics.Results[index] = row
		}
	}
	return nil
}

func reduceForumTopicStateChanged(state *State, update telegram.ForumTopicStateChanged) []Effect {
	chatID, topicID := update.ChatID, update.TopicID
	if chatID == 0 || topicID == 0 {
		return nil
	}
	key := topicKey{ChatID: chatID, TopicID: topicID}
	applyDraft := topicDraftMergeAllowed(*state, key, update.Draft)
	merged := false
	if byTopic := state.ForumTopics[chatID]; byTopic != nil {
		if entry, tracked := byTopic[topicID]; tracked {
			entry.IsPinned = update.IsPinned
			entry.UnreadMentionCount = update.UnreadMentionCount
			if applyDraft {
				entry.Draft = update.Draft
			}
			byTopic[topicID] = entry
			merged = true
		}
	}
	if topics := state.Topics; topics != nil && topics.ChatID == chatID {
		for index := range topics.Results {
			if topics.Results[index].ID != topicID {
				continue
			}
			row := topics.Results[index]
			row.IsPinned = update.IsPinned
			row.UnreadMentionCount = update.UnreadMentionCount
			if applyDraft {
				row.Draft = update.Draft
			}
			topics.Results[index] = row
			merged = true
			break
		}
	}
	if merged {
		applyTopicCloudDraft(state, key, update.Draft)
	}
	return nil
}

// topicDraftMergeAllowed mirrors the applyCloudDraft stale guard: a locally
// dirty or pending topic draft blocks the cloud draft unless the text and
// reply are identical.
func topicDraftMergeAllowed(state State, key topicKey, incoming domain.Draft) bool {
	syncState := state.TopicDraftSync[key]
	if !(syncState.Dirty || syncState.Pending) {
		return true
	}
	local := domain.Draft{
		Text:             state.TopicDrafts[key],
		ReplyToMessageID: state.TopicDraftReplies[key],
	}
	return sameDraftContent(local, incoming)
}

// applyTopicCloudDraft mirrors applyCloudDraft for topic drafts. A locally
// dirty or pending topic draft with different content blocks the cloud
// draft.
func applyTopicCloudDraft(state *State, key topicKey, draft domain.Draft) {
	syncState := state.TopicDraftSync[key]
	local := domain.Draft{
		Text:             state.TopicDrafts[key],
		ReplyToMessageID: state.TopicDraftReplies[key],
	}
	if (syncState.Dirty || syncState.Pending) && !sameDraftContent(local, draft) {
		return
	}
	if state.TopicDrafts == nil {
		state.TopicDrafts = make(map[topicKey]string)
	}
	if state.TopicDraftReplies == nil {
		state.TopicDraftReplies = make(map[topicKey]domain.MessageID)
	}
	if state.TopicDraftDates == nil {
		state.TopicDraftDates = make(map[topicKey]int64)
	}
	state.TopicDrafts[key] = draft.Text
	if draft.ReplyToMessageID > 0 {
		state.TopicDraftReplies[key] = draft.ReplyToMessageID
	} else {
		delete(state.TopicDraftReplies, key)
	}
	if draft.Date > 0 {
		state.TopicDraftDates[key] = draft.Date
	} else {
		delete(state.TopicDraftDates, key)
	}
	syncState.Draft = draft
	syncState.Dirty = false
	state.TopicDraftSync[key] = syncState
}

// visibleConversationTopic is the topic whose messages the conversation
// displays. ALL mode and untracked topics show the whole chat.
func visibleConversationTopic(state State, chatID domain.ChatID) (domain.ForumTopic, bool) {
	if state.ShowAll[chatID] {
		return domain.ForumTopic{}, false
	}
	topic, ok := state.ForumTopics[chatID][state.SelectedTopics[chatID]]
	return topic, ok
}

func visibleConversationMessages(state State, chatID domain.ChatID) []domain.Message {
	messages := state.Messages[chatID]
	_, known := visibleConversationTopic(state, chatID)
	if !known {
		return messages
	}
	visible := make([]domain.Message, 0)
	topicID := state.SelectedTopics[chatID]
	for _, message := range messages {
		if message.TopicID == topicID {
			visible = append(visible, message)
		}
	}
	return visible
}

// activeTopicKey reports the active topic key: the active chat is a forum
// with a selected topic. A forum without a selected topic has no key.
func activeTopicKey(state State) (topicKey, bool) {
	chatID, ok := activeChatID(state)
	if !ok {
		return topicKey{}, false
	}
	if !state.Chats[state.SelectedChat].IsForum {
		return topicKey{}, false
	}
	if state.ShowAll[chatID] {
		return topicKey{}, false
	}
	topicID := state.SelectedTopics[chatID]
	if topicID == 0 {
		return topicKey{}, false
	}
	return topicKey{ChatID: chatID, TopicID: topicID}, true
}

// forumTopicClosed reports whether the active chat is a forum and its
// selected topic is tracked and closed. In ALL mode the send target is the
// General topic, so a closed General topic blocks ALL-mode sending. Unknown
// topics never block sending.
func forumTopicClosed(state State, chatID domain.ChatID) bool {
	if state.SelectedChat < 0 || state.SelectedChat >= len(state.Chats) {
		return false
	}
	chat := state.Chats[state.SelectedChat]
	if !chat.IsForum || chat.ID != chatID {
		return false
	}
	if state.ShowAll[chatID] {
		topic, known := state.ForumTopics[chatID][generalTopicID(state, chatID)]
		return known && topic.IsClosed
	}
	topicID := state.SelectedTopics[chatID]
	if topicID == 0 {
		return false
	}
	topic, known := state.ForumTopics[chatID][topicID]
	return known && topic.IsClosed
}
