package app

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

	reduced, commands := Reduce(state, ActionReceived{Action: Activate})

	// ALL mode is enabled.
	if !reduced.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", reduced.ShowAll)
	}
	// SelectedTopics is cleared.
	if _, exists := reduced.SelectedTopics[7]; exists {
		t.Fatalf("SelectedTopics not cleared: %#v", reduced.SelectedTopics)
	}
	// Topic list is closed.
	if reduced.Topics != nil {
		t.Fatalf("Topics not closed: %#v", reduced.Topics)
	}
	// Focus is conversation.
	if reduced.Focus != FocusConversation {
		t.Fatalf("focus = %v", reduced.Focus)
	}
	// Reply/edit/menu cleared.
	if reduced.ReplyTarget != nil || reduced.EditTarget != nil || reduced.MessageMenu != nil {
		t.Fatalf("targets not cleared: %v %v %v", reduced.ReplyTarget, reduced.EditTarget, reduced.MessageMenu)
	}
	// Selection picks the newest message (ID 2).
	if reduced.SelectedMessageChat != 7 || reduced.SelectedMessage != 2 {
		t.Fatalf("selection = %d/%d", reduced.SelectedMessageChat, reduced.SelectedMessage)
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
	if history := reduced.History[7]; !history.Loading || history.RequestID != 10 {
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

	reduced, commands := Reduce(state, ActionReceived{Action: Activate})

	if !reduced.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", reduced.ShowAll)
	}
	if len(commands) != 0 {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	// Existing history is untouched.
	if reduced.History[7] != (HistoryState{Loading: false, Done: true}) {
		t.Fatalf("history = %#v", reduced.History[7])
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

	reduced, commands := Reduce(state, ActionReceived{Action: SelectAllMessages})

	if !reduced.ShowAll[7] {
		t.Fatalf("ShowAll not set: %#v", reduced.ShowAll)
	}
	if reduced.Topics != nil {
		t.Fatalf("Topics not closed: %#v", reduced.Topics)
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
	reduced, _ := Reduce(state, ActionReceived{Action: SelectPrevious})
	if reduced.Topics.Selected != 0 {
		t.Fatalf("selected = %d", reduced.Topics.Selected)
	}
	// Activate row 0 → ALL mode.
	reduced, _ = Reduce(reduced, ActionReceived{Action: Activate})
	if !reduced.ShowAll[7] {
		t.Fatalf("ShowAll not set after Activate row 0: %#v", reduced.ShowAll)
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
	reduced, _ := Reduce(state, ActionReceived{Action: SelectNext})
	if reduced.Topics.Selected != 2 {
		t.Fatalf("next at last: selected=%d", reduced.Topics.Selected)
	}
	// SelectPrevious to row 1.
	reduced, _ = Reduce(reduced, ActionReceived{Action: SelectPrevious})
	if reduced.Topics.Selected != 1 {
		t.Fatalf("previous: selected=%d", reduced.Topics.Selected)
	}
	// SelectPrevious to row 0 (ALL).
	reduced, _ = Reduce(reduced, ActionReceived{Action: SelectPrevious})
	if reduced.Topics.Selected != 0 {
		t.Fatalf("previous to all: selected=%d", reduced.Topics.Selected)
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

	got, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "all draft"})

	key := topicKey{ChatID: 7, TopicID: 0}
	if got.TopicDrafts[key] != "all draft" {
		t.Fatalf("ALL draft = %q", got.TopicDrafts[key])
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.TopicID != 0 || save.ChatID != 7 || save.Text != "all draft" {
		t.Fatalf("save command = %#v", commands[0])
	}
	if syncState := got.TopicDraftSync[key]; !syncState.Pending || !syncState.Dirty {
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

	got, commands := Reduce(state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})

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
	if got.TopicDrafts[allKey] != "" {
		t.Fatalf("ALL draft not cleared: %q", got.TopicDrafts[allKey])
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

	_, commands := Reduce(state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})

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
	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	save := commands[0].(SaveDraft)

	ack, _ := Reduce(state, DraftSaved{RequestID: save.RequestID, ChatID: 7, TopicID: 0, Date: 123})
	if ack.TopicDraftDates[allKey] != 123 {
		t.Fatalf("ack dates = %#v", ack.TopicDraftDates)
	}
	if syncState := ack.TopicDraftSync[allKey]; syncState.Pending || syncState.Draft.Date != 123 {
		t.Fatalf("ack sync state = %#v", syncState)
	}
	// Chat-level draft dates untouched.
	if ack.DraftDates[7] != 0 {
		t.Fatalf("chat-level dates touched: %#v", ack.DraftDates)
	}
}

func TestAllModeDraftSaveFailedSanitizedToast(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}
	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	save := commands[0].(SaveDraft)

	failed, _ := Reduce(state, DraftSaveFailed{RequestID: save.RequestID, ChatID: 7, TopicID: 0, Error: domain.AppError{Message: "private raw failure"}})
	if failed.Toast == nil || failed.Toast.Message != "Could not sync draft" {
		t.Fatalf("failure toast = %#v", failed.Toast)
	}
	if syncState := failed.TopicDraftSync[allKey]; syncState.Pending {
		t.Fatalf("failure sync state = %#v", syncState)
	}
	if failed.TopicDrafts[allKey] != "all draft" {
		t.Fatalf("failure cleared the local ALL draft: %q", failed.TopicDrafts[allKey])
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

	replying, commands := Reduce(state, ActionReceived{Action: ReplyMessage})
	if len(commands) != 1 {
		t.Fatalf("reply commands = %#v", commands)
	}
	save := commands[0].(SaveDraft)
	if save.TopicID != 0 || save.ChatID != 7 || save.ReplyToMessageID != 55 {
		t.Fatalf("reply save = %#v", save)
	}
	if replying.ReplyTarget == nil || replying.ReplyTarget.TopicID != 0 || replying.ReplyTarget.MessageID != 55 {
		t.Fatalf("reply target = %#v", replying.ReplyTarget)
	}
	if replying.TopicDraftReplies[allKey] != 55 {
		t.Fatalf("reply draft map = %#v", replying.TopicDraftReplies)
	}

	// Cancel reply.
	cancelled, commands := Reduce(replying, ActionReceived{Action: CancelReply})
	if len(commands) != 1 {
		t.Fatalf("cancel commands = %#v", commands)
	}
	if clear := commands[0].(SaveDraft); clear.TopicID != 0 || clear.ReplyToMessageID != 0 {
		t.Fatalf("cancel save = %#v", clear)
	}
	if cancelled.ReplyTarget != nil || cancelled.TopicDraftReplies[allKey] != 0 {
		t.Fatalf("cancel state = target:%#v replies:%#v", cancelled.ReplyTarget, cancelled.TopicDraftReplies)
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
	replyingForeign, foreignCommands := Reduce(foreign, ActionReceived{Action: ReplyMessage})
	if len(foreignCommands) != 1 {
		t.Fatalf("foreign reply commands = %#v", foreignCommands)
	}
	if save := foreignCommands[0].(SaveDraft); save.TopicID != 0 || save.ReplyToMessageID != 77 {
		t.Fatalf("foreign reply save = %#v", save)
	}
	if replyingForeign.ReplyTarget == nil || replyingForeign.ReplyTarget.TopicID != 0 || replyingForeign.ReplyTarget.MessageID != 77 {
		t.Fatalf("foreign reply target = %#v", replyingForeign.ReplyTarget)
	}
	if replyingForeign.TopicDraftReplies[allKey] != 77 {
		t.Fatalf("foreign reply draft map = %#v", replyingForeign.TopicDraftReplies)
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

	reduced, _ := Reduce(state, ActionReceived{Action: Activate})

	if reduced.ShowAll[7] {
		t.Fatalf("ShowAll not cleared: %#v", reduced.ShowAll)
	}
	if reduced.SelectedTopics[7] != 101 {
		t.Fatalf("SelectedTopics = %#v", reduced.SelectedTopics)
	}
}

func TestMessageSearchAllowedInAllMode(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusConversation

	reduced, _ := Reduce(state, ActionReceived{Action: OpenMessageSearch})

	if reduced.MessageSearch == nil {
		t.Fatal("MessageSearch not opened in ALL mode")
	}
	// TopicID should be 0 (whole-chat search).
	if reduced.MessageSearch.TopicID != 0 {
		t.Fatalf("TopicID = %d, want 0", reduced.MessageSearch.TopicID)
	}
}

func TestMessageSearchBlockedWithoutSelectionOrAll(t *testing.T) {
	state := topicsBaseState(t)
	// No selected topic, no ALL mode.
	state.Focus = FocusConversation

	reduced, _ := Reduce(state, ActionReceived{Action: OpenMessageSearch})

	if reduced.MessageSearch != nil {
		t.Fatal("MessageSearch opened without selected topic or ALL mode")
	}
}

func TestChatSearchJumpDisablesShowAll(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true

	// Simulate jumping from a chat search to a message in chat 7.
	msg := domain.Message{ID: 42, ChatID: 7, TopicID: 0, Kind: domain.MessageText, Text: "msg"}
	reduced, _ := Reduce(state, ActionReceived{Action: SelectChat, ChatID: 7})
	_ = reduced
	// Directly call the landing logic.
	state2 := state
	state2.ShowAll[7] = true
	opened, _ := openChatFromMessageResult(state2, msg)
	if opened.ShowAll[7] {
		t.Fatalf("ShowAll not disabled after chat search jump: %#v", opened.ShowAll)
	}
}

func TestAllModeDraftsPersistAcrossTopicSelections(t *testing.T) {
	state := topicDraftState(t)
	state.ShowAll[7] = true
	delete(state.SelectedTopics, 7)
	state.Focus = FocusComposer
	allKey := topicKey{ChatID: 7, TopicID: 0}

	// Type in ALL mode.
	state, _ = Reduce(state, ComposerValueChanged{ChatID: 7, Value: "all draft"})
	if state.TopicDrafts[allKey] != "all draft" {
		t.Fatalf("ALL draft = %q", state.TopicDrafts[allKey])
	}

	// Switch to a specific topic (disable ALL).
	state.ShowAll[7] = false
	state.SelectedTopics[7] = 101

	// Type in topic mode.
	state, _ = Reduce(state, ComposerValueChanged{ChatID: 7, Value: "topic draft"})
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

func TestCloneReducerStateCopiesShowAll(t *testing.T) {
	state := topicsBaseState(t)
	state.ShowAll[7] = true

	clone := cloneReducerState(state)

	if !clone.ShowAll[7] {
		t.Fatal("ShowAll not copied to clone")
	}
	// Mutating the clone should not affect the original.
	clone.ShowAll[7] = false
	if !state.ShowAll[7] {
		t.Fatal("mutating clone ShowAll affected original")
	}
}
