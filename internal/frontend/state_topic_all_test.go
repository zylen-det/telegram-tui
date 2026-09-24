package frontend

import (
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestActivateAllTopicsSetsShowAllAndRequestsChatHistory(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A"), testTopic(102, "B")},
		Selected:  0,
	}
	// Seed some existing messages so the selection picks the newest.
	state.Messages[7] = []domain.Message{
		{ID: 1, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "a"},
		{ID: 2, ChatID: 7, TopicID: 102, Kind: domain.MessageText, Text: "b"},
	}

	commands := updateState(&state, ActionReceived{Action: Activate})

	// ALL mode is enabled.
	if !state.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", state.ShowAll)
	}
	// SelectedTopics is cleared.
	if _, exists := state.SelectedTopics[7]; exists {
		t.Fatalf("SelectedTopics not cleared: %#v", state.SelectedTopics)
	}
	// Topic list is closed.
	if state.Topics != nil {
		t.Fatalf("Topics not closed: %#v", state.Topics)
	}
	// Focus is conversation.
	if state.Focus != FocusConversation {
		t.Fatalf("focus = %v", state.Focus)
	}
	// Reply/edit/menu cleared.
	if state.ReplyTarget != nil || state.EditTarget != nil || state.MessageMenu != nil {
		t.Fatalf("targets not cleared: %v %v %v", state.ReplyTarget, state.EditTarget, state.MessageMenu)
	}
	// Selection picks the newest message (ID 2).
	if state.SelectedMessageChat != 7 || state.SelectedMessage != 2 {
		t.Fatalf("selection = %d/%d", state.SelectedMessageChat, state.SelectedMessage)
	}
	// A LoadMessages command is emitted for the whole chat (TopicID 0).
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	load, ok := commands[0].(LoadMessages)
	if !ok {
		t.Fatalf("command = %#v", commands[0])
	}
	if load.TopicID != 0 || load.ChatID != 7 || load.RequestID != 10 {
		t.Fatalf("LoadMessages = %#v", load)
	}
	// History is created with Loading=true.
	if history := state.History[7]; !history.Loading || history.RequestID != 10 {
		t.Fatalf("history = %#v", history)
	}
}

func TestActivateAllTopicsIdempotentWhenHistoryExists(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A")},
		Selected:  0,
	}
	// Pre-existing history: no new LoadMessages should be emitted.
	state.History[7] = HistoryState{Loading: false, Done: true}

	commands := updateState(&state, ActionReceived{Action: Activate})

	if !state.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", state.ShowAll)
	}
	if len(commands) != 0 {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	// Existing history is untouched.
	if state.History[7] != (HistoryState{Loading: false, Done: true}) {
		t.Fatalf("history = %#v", state.History[7])
	}
}

func TestSelectAllMessagesActionActivatesAllMode(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A")},
		Selected:  1,
	}

	commands := updateState(&state, ActionReceived{Action: SelectAllMessages})

	if !state.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", state.ShowAll)
	}
	if state.Topics != nil {
		t.Fatalf("Topics not closed: %#v", state.Topics)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestActivateRowZeroActivatesAllMode(t *testing.T) {
	// Row 0 = ALL pseudo-row. SelectNext from row 0 to row 1, then
	// SelectPrevious back to row 0 and Activate should enter ALL mode.
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A"), testTopic(102, "B")},
		Selected:  1,
	}
	// Move to row 0 (ALL).
	updateState(&state, ActionReceived{Action: SelectPrevious})
	if state.Topics.Selected != 0 {
		t.Fatalf("selected = %d", state.Topics.Selected)
	}
	// Activate row 0 → ALL mode.
	updateState(&state, ActionReceived{Action: Activate})
	if !state.ShowAll[7] {
		t.Fatalf("ShowAll not set after Activate row 0: %#v", state.ShowAll)
	}
}

func TestSelectTopicClampIncludesAllRow(t *testing.T) {
	// With 2 topics, valid Selected range is [0, 2] (0=ALL, 1..2=topics).
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A"), testTopic(102, "B")},
		Selected:  2,
	}
	// SelectNext from last topic → stays at 2.
	updateState(&state, ActionReceived{Action: SelectNext})
	if state.Topics.Selected != 2 {
		t.Fatalf("next at last: selected=%d", state.Topics.Selected)
	}
	// SelectPrevious to row 1.
	updateState(&state, ActionReceived{Action: SelectPrevious})
	if state.Topics.Selected != 1 {
		t.Fatalf("previous: selected=%d", state.Topics.Selected)
	}
	// SelectPrevious to row 0 (ALL).
	updateState(&state, ActionReceived{Action: SelectPrevious})
	if state.Topics.Selected != 0 {
		t.Fatalf("previous to all: selected=%d", state.Topics.Selected)
	}
}

func TestGeneralTopicIDFallsBackToOne(t *testing.T) {
	state := topicsBaseState(t)
	// No topics tracked → fallback to 1.
	if id := generalTopicID(state, 7); id != 1 {
		t.Fatalf("fallback = %d", id)
	}
	// Track a General topic with non-1 ID.
	trackTopic(&state, domain.ForumTopic{ID: 99, ChatID: 7, IsGeneral: true})
	if id := generalTopicID(state, 7); id != 99 {
		t.Fatalf("general = %d", id)
	}
}

func TestShowAllActiveReturnsTopicKey(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true

	key, ok := showAllActive(state)
	if !ok {
		t.Fatal("showAllActive = false, want true")
	}
	if key != (topicKey{ChatID: 7, TopicID: 0}) {
		t.Fatalf("key = %#v", key)
	}

	// Deactivate.
	state.ShowAll[7] = false
	if _, ok := showAllActive(state); ok {
		t.Fatal("showAllActive = true after deactivation")
	}
}

func TestShowAllBlocksActiveTopicKey(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true
	state.SelectedTopics[7] = 101
	trackTopic(&state, domain.ForumTopic{ID: 101, ChatID: 7, Name: "A"})

	if _, ok := activeTopicKey(state); ok {
		t.Fatal("activeTopicKey should be false when ShowAll is active")
	}
}

func TestAllModeDraftsUseTopicKeyZero(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer

	commands := updateState(&state, ComposerValueChanged{ChatID: 7, Value: "all draft"})

	key := topicKey{ChatID: 7, TopicID: 0}
	if state.TopicDrafts[key] != "all draft" {
		t.Fatalf("ALL draft = %q", state.TopicDrafts[key])
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.TopicID != 0 || save.ChatID != 7 || save.Text != "all draft" {
		t.Fatalf("save command = %#v", commands[0])
	}
	if syncState := state.TopicDraftSync[key]; !syncState.Pending || !syncState.Dirty {
		t.Fatalf("sync state = %#v", syncState)
	}
}

func TestAllModeSubmitSendsToGeneralTopicAndClearsAllDraft(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}
	state.TopicDrafts[allKey] = "all text"

	// Track a General topic with ID 101.
	trackTopic(&state, domain.ForumTopic{ID: 101, ChatID: 7, IsGeneral: true})

	commands := updateState(&state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})

	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	send, ok := commands[0].(SendText)
	if !ok || send.TopicID != 101 || send.ChatID != 7 || send.Text != "all text" {
		t.Fatalf("send command = %#v", commands[0])
	}
	clear, ok := commands[1].(SaveDraft)
	if !ok || clear.TopicID != 0 || clear.ChatID != 7 || clear.Text != "" {
		t.Fatalf("clear command = %#v", commands[1])
	}
	if state.TopicDrafts[allKey] != "" {
		t.Fatalf("ALL draft not cleared: %q", state.TopicDrafts[allKey])
	}
}

func TestAllModeSubmitFallsBackToGeneralTopicIDOne(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}
	state.TopicDrafts[allKey] = "all text"

	// No General topic tracked → fallback to 1.
	// Remove all tracked topics.
	state.ForumTopics[7] = nil

	commands := updateState(&state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})

	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	send, ok := commands[0].(SendText)
	if !ok || send.TopicID != 1 {
		t.Fatalf("send command TopicID = %d, want 1", send.TopicID)
	}
}

func TestAllModeDraftSavedAckRoutesToAllKey(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}
	commands := updateState(&state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	save := commands[0].(SaveDraft)

	updateState(&state, DraftSaved{RequestID: save.RequestID, ChatID: 7, TopicID: 0, Date: 123})
	if state.TopicDraftDates[allKey] != 123 {
		t.Fatalf("ack dates = %#v", state.TopicDraftDates)
	}
	if syncState := state.TopicDraftSync[allKey]; syncState.Pending || syncState.Draft.Date != 123 {
		t.Fatalf("ack sync state = %#v", syncState)
	}
	// Chat-level draft dates untouched.
	if state.DraftDates[7] != 0 {
		t.Fatalf("chat-level dates touched: %#v", state.DraftDates)
	}
}

func TestAllModeDraftSaveFailedSanitizedToast(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}
	commands := updateState(&state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	save := commands[0].(SaveDraft)

	updateState(&state, DraftSaveFailed{RequestID: save.RequestID, ChatID: 7, TopicID: 0, Error: domain.AppError{Message: "private raw failure"}})
	if state.Toast == nil || state.Toast.Message != "Could not sync draft" {
		t.Fatalf("failure toast = %#v", state.Toast)
	}
	if syncState := state.TopicDraftSync[allKey]; syncState.Pending {
		t.Fatalf("failure sync state = %#v", syncState)
	}
	if state.TopicDrafts[allKey] != "all draft" {
		t.Fatalf("failure cleared the local ALL draft: %q", state.TopicDrafts[allKey])
	}
}

func TestAllModeReplyBeginAndRestoreCarryTopicIDZero(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusConversation
	allKey := topicKey{ChatID: 7, TopicID: 0}
	// A General-topic message (TopicID 0) is the reply target.
	state.Messages[7] = []domain.Message{{ID: 55, ChatID: 7, TopicID: 0, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
	state.SelectedMessageChat, state.SelectedMessage = 7, 55

	commands := updateState(&state, ActionReceived{Action: ReplyMessage})
	if len(commands) != 1 {
		t.Fatalf("reply commands = %#v", commands)
	}
	save := commands[0].(SaveDraft)
	if save.TopicID != 0 || save.ChatID != 7 || save.ReplyToMessageID != 55 {
		t.Fatalf("reply save = %#v", save)
	}
	if state.ReplyTarget == nil || state.ReplyTarget.TopicID != 0 || state.ReplyTarget.MessageID != 55 {
		t.Fatalf("reply target = %#v", state.ReplyTarget)
	}
	if state.TopicDraftReplies[allKey] != 55 {
		t.Fatalf("reply draft map = %#v", state.TopicDraftReplies)
	}

	// Cancel reply.
	commands = updateState(&state, ActionReceived{Action: CancelReply})
	if len(commands) != 1 {
		t.Fatalf("cancel commands = %#v", commands)
	}
	if clear := commands[0].(SaveDraft); clear.TopicID != 0 || clear.ReplyToMessageID != 0 {
		t.Fatalf("cancel save = %#v", clear)
	}
	if state.ReplyTarget != nil || state.TopicDraftReplies[allKey] != 0 {
		t.Fatalf("cancel state = target:%#v replies:%#v", state.ReplyTarget, state.TopicDraftReplies)
	}

	// Restore reply from persisted ALL draft.
	hydrated := topicDraftState(t)
	hydrated.ShowAll[7] = true
	delete(hydrated.SelectedTopics, 7)
	hydrated.TopicDraftReplies[allKey] = 55
	hydrated.Messages[7] = []domain.Message{{ID: 55, ChatID: 7, TopicID: 0, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
	restoreActiveDraftReply(&hydrated)
	if hydrated.ReplyTarget == nil || hydrated.ReplyTarget.TopicID != 0 || hydrated.ReplyTarget.MessageID != 55 || hydrated.ReplyTarget.Sender != "Sender" {
		t.Fatalf("hydrated reply target = %#v", hydrated.ReplyTarget)
	}

	// A foreign-topic message is also a valid ALL reply target; its TopicID
	// normalizes to the ALL key and the send still goes to General.
	foreign := topicDraftState(t)
	foreign.ShowAll[7] = true
	delete(foreign.SelectedTopics, 7)
	foreign.Focus = FocusConversation
	foreign.Messages[7] = []domain.Message{{ID: 77, ChatID: 7, TopicID: 5, Kind: domain.MessageText, SenderName: "Other", Text: "foreign"}}
	foreign.SelectedMessageChat, foreign.SelectedMessage = 7, 77
	foreignCommands := updateState(&foreign, ActionReceived{Action: ReplyMessage})
	if len(foreignCommands) != 1 {
		t.Fatalf("foreign reply commands = %#v", foreignCommands)
	}
	if save := foreignCommands[0].(SaveDraft); save.TopicID != 0 || save.ReplyToMessageID != 77 {
		t.Fatalf("foreign reply save = %#v", save)
	}
	if foreign.ReplyTarget == nil || foreign.ReplyTarget.TopicID != 0 || foreign.ReplyTarget.MessageID != 77 {
		t.Fatalf("foreign reply target = %#v", foreign.ReplyTarget)
	}
	if foreign.TopicDraftReplies[allKey] != 77 {
		t.Fatalf("foreign reply draft map = %#v", foreign.TopicDraftReplies)
	}
}

func TestActivateTopicDisablesShowAll(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{testTopic(101, "A")},
		Selected:  1,
	}

	updateState(&state, ActionReceived{Action: Activate})

	if state.ShowAll[7] {
		t.Fatalf("ShowAll not cleared: %#v", state.ShowAll)
	}
	if state.SelectedTopics[7] != 101 {
		t.Fatalf("SelectedTopics = %#v", state.SelectedTopics)
	}
}

func TestMessageSearchAllowedInAllMode(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusConversation

	updateState(&state, ActionReceived{Action: OpenMessageSearch})

	if state.MessageSearch == nil {
		t.Fatal("MessageSearch not opened in ALL mode")
	}
	// TopicID should be 0 (whole-chat search).
	if state.MessageSearch.TopicID != 0 {
		t.Fatalf("TopicID = %d, want 0", state.MessageSearch.TopicID)
	}
}

func TestMessageSearchBlockedWithoutSelectionOrAll(t *testing.T) {
	state := topicsBaseState(t)
	// No selected topic, no ALL mode.
	state.Focus = FocusConversation

	updateState(&state, ActionReceived{Action: OpenMessageSearch})

	if state.MessageSearch != nil {
		t.Fatal("MessageSearch opened without selected topic or ALL mode")
	}
}

func TestChatSearchJumpDisablesShowAll(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true

	// Simulate jumping from a chat search to a message in chat 7 while ALL
	// mode is active; the landing logic must disable ALL mode.
	msg := domain.Message{ID: 42, ChatID: 7, TopicID: 0, Kind: domain.MessageText, Text: "msg"}
	updateState(&state, ActionReceived{Action: SelectChat, ChatID: 7})
	openChatFromMessageResult(&state, msg)
	if state.ShowAll[7] {
		t.Fatalf("ShowAll not disabled after chat search jump: %#v", state.ShowAll)
	}
}

func TestAllModeDraftsPersistAcrossTopicSelections(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}

	// Type in ALL mode.
	updateState(&state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	if state.TopicDrafts[allKey] != "all draft" {
		t.Fatalf("ALL draft = %q", state.TopicDrafts[allKey])
	}

	// Switch to a specific topic (disable ALL).
	state.ShowAll[7] = false
	state.SelectedTopics[7] = 101

	// Type in topic mode.
	updateState(&state, ComposerValueChanged{ChatID: 7, Value: "topic draft"})
	topicKey := topicKey{ChatID: 7, TopicID: 101}
	if state.TopicDrafts[topicKey] != "topic draft" {
		t.Fatalf("topic draft = %q", state.TopicDrafts[topicKey])
	}

	// Switch back to ALL mode.
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)

	// The ALL draft should still be there.
	if state.TopicDrafts[allKey] != "all draft" {
		t.Fatalf("ALL draft lost: %q", state.TopicDrafts[allKey])
	}
}
