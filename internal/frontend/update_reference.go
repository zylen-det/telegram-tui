package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

// A reference jump belongs to the open message menu. Closing the menu cancels
// the jump: late context results can no longer change the conversation.
func beginReferencedMessageJump(state *State) []Effect {
	menu := state.MessageMenu
	if menu == nil || menu.JumpRequestID != 0 || menu.ReferencedMessageID <= 0 {
		return nil
	}
	chatID, active := activeChatID(*state)
	message, found := messageByIdentity(*state, menu.ChatID, menu.MessageID)
	if !active || chatID != menu.ChatID || !found || !message.HasReply || message.ReplyToMessageID != menu.ReferencedMessageID {
		return nil
	}
	requestID := allocateRequestID(state)
	menu.JumpRequestID = requestID
	// Load against the chat so references across forum topics can be found too.
	return []Effect{LoadSearchMessageContext{RequestID: requestID, ChatID: chatID, MessageID: menu.ReferencedMessageID}}
}

func referenceJumpMatches(state State, requestID uint64, chatID domain.ChatID, messageID domain.MessageID) bool {
	menu := state.MessageMenu
	activeID, active := activeChatID(state)
	return menu != nil && menu.JumpRequestID != 0 && menu.JumpRequestID == requestID &&
		menu.ChatID == chatID && menu.ReferencedMessageID == messageID && active && activeID == chatID
}

func reduceReferencedMessageLoaded(state *State, event SearchMessageContextLoaded) []Effect {
	if !referenceJumpMatches(*state, event.RequestID, event.ChatID, event.MessageID) || event.TopicID != 0 {
		return nil
	}
	var target domain.Message
	for _, message := range event.Page.Messages {
		if message.ChatID == event.ChatID && message.ID == event.MessageID {
			target = message
			break
		}
	}
	if target.ID == 0 {
		return reduceReferencedMessageFailed(state, SearchMessageContextFailed{RequestID: event.RequestID, ChatID: event.ChatID, MessageID: event.MessageID})
	}
	merged := mergeMessages(state.Messages[event.ChatID], event.Page.Messages)
	merged = limitMessagesAround(merged, event.MessageID, maxMessagesPerChat)
	state.Messages[event.ChatID] = merged

	// A reference outside the currently visible topic must still be visible.
	if state.Chats[state.SelectedChat].IsForum && !state.ShowAll[event.ChatID] && state.SelectedTopics[event.ChatID] != target.TopicID {
		state.ShowAll[event.ChatID] = true
		delete(state.SelectedTopics, event.ChatID)
		state.ReplyTarget = nil
		state.EditTarget = nil
		state.CommandMenu = nil
	}
	visible := visibleConversationMessages(*state, event.ChatID)
	index := messageIndex(visible, event.MessageID)
	state.SelectedMessageChat, state.SelectedMessage = event.ChatID, event.MessageID
	key, topicActive := activeTopicKey(*state)
	_, topicKnown := visibleConversationTopic(*state, event.ChatID)
	if topicActive && topicKnown {
		history := state.TopicHistory[key]
		history.Loading = false
		history.Error = nil
		history.OldestID = visible[0].ID
		history.ViewOffset = len(visible) - 1 - index
		history.FollowSelection = true
		state.TopicHistory[key] = history
	} else {
		history := state.History[event.ChatID]
		history.Loading = false
		history.Error = nil
		history.OldestID = visible[0].ID
		history.ViewOffset = len(visible) - 1 - index
		history.FollowSelection = true
		state.History[event.ChatID] = history
	}
	state.MessageMenu = nil
	state.Focus = FocusConversation
	commands := requestMissingMessageAvatars(state, event.Page.Messages)
	commands = append(commands, requestMissingThumbnails(state, event.Page.Messages)...)
	return commands
}

func reduceReferencedMessageFailed(state *State, event SearchMessageContextFailed) []Effect {
	if !referenceJumpMatches(*state, event.RequestID, event.ChatID, event.MessageID) || event.TopicID != 0 {
		return nil
	}
	state.MessageMenu.JumpRequestID = 0
	setToast(state, domain.AppError{Kind: domain.ErrorNetwork, Op: "load referenced message", Message: "Could not open referenced message"}, 4*time.Second)
	return nil
}
