package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func topicDraftState(t *testing.T) State {
	t.Helper()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusComposer
	state.Chats = []domain.Chat{{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	state.SelectedTopics[7] = 101
	state.ForumTopics[7] = map[domain.TopicID]domain.ForumTopic{
		101: {ID: 101, ChatID: 7, Name: "A"},
		102: {ID: 102, ChatID: 7, Name: "B"},
	}
	return state
}

func TestTopicComposerTypingRoutesToTopicDrafts(t *testing.T) {
	state := topicDraftState(t)
	key := topicKey{ChatID: 7, TopicID: 101}

	got, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "topic draft"})

	if got.TopicDrafts[key] != "topic draft" {
		t.Fatalf("topic draft = %q", got.TopicDrafts[key])
	}
	if len(got.Drafts) != 0 {
		t.Fatalf("chat-level drafts touched: %#v", got.Drafts)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.TopicID != 101 || save.ChatID != 7 || save.Text != "topic draft" || save.ReplyToMessageID != 0 || save.RequestID != 10 {
		t.Fatalf("save command = %#v", commands[0])
	}
	if syncState := got.TopicDraftSync[key]; !syncState.Pending || !syncState.Dirty {
		t.Fatalf("sync state = %#v", syncState)
	}
	if got.ForumTopics[7][101].Draft.Text != "topic draft" {
		t.Fatalf("forum topic snapshot = %#v", got.ForumTopics[7][101].Draft)
	}
}

func TestForumWithoutSelectedTopicComposerNoOp(t *testing.T) {
	state := topicDraftState(t)
	delete(state.SelectedTopics, 7)

	typing, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "orphan"})
	if len(commands) != 0 || !reflect.DeepEqual(typing, state) {
		t.Fatalf("orphan typing changed state: commands=%#v drafts=%#v", commands, typing.Drafts)
	}

	submitted, commands := Reduce(state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})
	if len(commands) != 0 || !reflect.DeepEqual(submitted, state) {
		t.Fatalf("orphan submit changed state: commands=%#v", commands)
	}
}

func TestTopicSubmitCarriesTopicIDClearsTopicDraftPreservesChatDrafts(t *testing.T) {
	state := topicDraftState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	state.TopicDrafts[key] = "topic text"
	state.Drafts[7] = "chat level"

	got, commands := Reduce(state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})

	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	send, ok := commands[0].(SendText)
	if !ok || send.TopicID != 101 || send.ChatID != 7 || send.Text != "topic text" || send.ReplyToMessageID != 0 || send.LocalID == 0 {
		t.Fatalf("send command = %#v", commands[0])
	}
	clear, ok := commands[1].(SaveDraft)
	if !ok || clear.TopicID != 101 || clear.ChatID != 7 || clear.Text != "" || clear.ReplyToMessageID != 0 {
		t.Fatalf("clear command = %#v", commands[1])
	}
	if got.TopicDrafts[key] != "" {
		t.Fatalf("topic draft not cleared: %q", got.TopicDrafts[key])
	}
	if got.Drafts[7] != "chat level" {
		t.Fatalf("chat-level draft was clobbered: %q", got.Drafts[7])
	}
	if got.SelectedMessage != send.LocalID {
		t.Fatalf("selected message = %d, want %d", got.SelectedMessage, send.LocalID)
	}
	messages := got.Messages[7]
	if len(messages) == 0 || messages[len(messages)-1].TopicID != 101 {
		t.Fatalf("sent message = %#v", messages)
	}
}

func TestTopicDraftSavedAckUpdatesDatesAndSnapshotIgnoresStale(t *testing.T) {
	state := topicDraftState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "topic draft"})
	save := commands[0].(SaveDraft)

	ack, _ := Reduce(state, DraftSaved{RequestID: save.RequestID, ChatID: 7, TopicID: 101, Date: 123})
	if ack.TopicDraftDates[key] != 123 {
		t.Fatalf("ack dates = %#v", ack.TopicDraftDates)
	}
	if syncState := ack.TopicDraftSync[key]; syncState.Pending || syncState.Draft.Date != 123 {
		t.Fatalf("ack sync state = %#v", syncState)
	}
	if ack.ForumTopics[7][101].Draft.Date != 123 {
		t.Fatalf("forum topic snapshot date = %#v", ack.ForumTopics[7][101].Draft)
	}
	if ack.DraftDates[7] != 0 {
		t.Fatalf("chat-level draft date touched: %#v", ack.DraftDates)
	}

	stale, _ := Reduce(ack, DraftSaved{RequestID: save.RequestID + 1, ChatID: 7, TopicID: 101, Date: 999})
	if stale.TopicDraftDates[key] != 123 || stale.ForumTopics[7][101].Draft.Date != 123 {
		t.Fatalf("stale ack applied: dates=%#v snapshot=%#v", stale.TopicDraftDates, stale.ForumTopics[7][101].Draft)
	}
}

func TestTopicDraftSaveFailedSanitizedToast(t *testing.T) {
	state := topicDraftState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "topic draft"})
	save := commands[0].(SaveDraft)

	failed, _ := Reduce(state, DraftSaveFailed{RequestID: save.RequestID, ChatID: 7, TopicID: 101, Error: domain.AppError{Message: "private raw failure"}})
	if failed.Toast == nil || failed.Toast.Message != "Could not sync draft" || strings.Contains(failed.Toast.Error(), "private") {
		t.Fatalf("failure toast = %#v", failed.Toast)
	}
	if syncState := failed.TopicDraftSync[key]; syncState.Pending {
		t.Fatalf("failure sync state = %#v", syncState)
	}
	if failed.TopicDrafts[key] != "topic draft" {
		t.Fatalf("failure cleared the local topic draft: %q", failed.TopicDrafts[key])
	}
}

func TestTopicReplyBeginAndRestoreCarryTopicID(t *testing.T) {
	state := topicDraftState(t)
	state.Focus = FocusConversation
	key := topicKey{ChatID: 7, TopicID: 101}
	state.Messages[7] = []domain.Message{{ID: 55, ChatID: 7, TopicID: 101, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
	state.SelectedMessageChat, state.SelectedMessage = 7, 55

	replying, commands := Reduce(state, ActionReceived{Action: ReplyMessage})
	if len(commands) != 1 {
		t.Fatalf("reply commands = %#v", commands)
	}
	save := commands[0].(SaveDraft)
	if save.TopicID != 101 || save.ChatID != 7 || save.ReplyToMessageID != 55 {
		t.Fatalf("reply save = %#v", save)
	}
	if replying.ReplyTarget == nil || replying.ReplyTarget.TopicID != 101 || replying.ReplyTarget.MessageID != 55 {
		t.Fatalf("reply target = %#v", replying.ReplyTarget)
	}
	if replying.TopicDraftReplies[key] != 55 || replying.DraftReplies[7] != 0 {
		t.Fatalf("reply draft maps = %#v / %#v", replying.TopicDraftReplies, replying.DraftReplies)
	}

	cancelled, commands := Reduce(replying, ActionReceived{Action: CancelReply})
	if len(commands) != 1 {
		t.Fatalf("cancel commands = %#v", commands)
	}
	if clear := commands[0].(SaveDraft); clear.TopicID != 101 || clear.ReplyToMessageID != 0 {
		t.Fatalf("cancel save = %#v", clear)
	}
	if cancelled.ReplyTarget != nil || cancelled.TopicDraftReplies[key] != 0 {
		t.Fatalf("cancel state = target:%#v replies:%#v", cancelled.ReplyTarget, cancelled.TopicDraftReplies)
	}

	hydrated := topicDraftState(t)
	hydrated.TopicDraftReplies[key] = 55
	hydrated.Messages[7] = []domain.Message{{ID: 55, ChatID: 7, TopicID: 101, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
	restoreActiveDraftReply(&hydrated)
	if hydrated.ReplyTarget == nil || hydrated.ReplyTarget.TopicID != 101 || hydrated.ReplyTarget.MessageID != 55 || hydrated.ReplyTarget.Sender != "Sender" {
		t.Fatalf("hydrated reply target = %#v", hydrated.ReplyTarget)
	}

	fallback := topicDraftState(t)
	fallback.TopicDraftReplies[key] = 55
	restoreActiveDraftReply(&fallback)
	if fallback.ReplyTarget == nil || fallback.ReplyTarget.ChatID != 7 || fallback.ReplyTarget.TopicID != 101 || fallback.ReplyTarget.MessageID != 55 || fallback.ReplyTarget.Sender != "Message" || fallback.ReplyTarget.Preview != "Reply draft" {
		t.Fatalf("fallback reply target = %#v", fallback.ReplyTarget)
	}
}

func TestTopicDraftsAreIsolatedAcrossTopics(t *testing.T) {
	state := topicDraftState(t)
	keyA := topicKey{ChatID: 7, TopicID: 101}
	keyB := topicKey{ChatID: 7, TopicID: 102}

	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "draft A"})
	if save := commands[0].(SaveDraft); save.TopicID != 101 || save.Text != "draft A" {
		t.Fatalf("topic A save = %#v", save)
	}
	state.SelectedTopics[7] = 102
	state, commands = Reduce(state, ComposerValueChanged{ChatID: 7, Value: "draft B"})
	if save := commands[0].(SaveDraft); save.TopicID != 102 || save.Text != "draft B" {
		t.Fatalf("topic B save = %#v", save)
	}
	if state.TopicDrafts[keyA] != "draft A" || state.TopicDrafts[keyB] != "draft B" {
		t.Fatalf("topic drafts = %#v", state.TopicDrafts)
	}
	if state.ForumTopics[7][101].Draft.Text != "draft A" || state.ForumTopics[7][102].Draft.Text != "draft B" {
		t.Fatalf("forum snapshots = %#v", state.ForumTopics[7])
	}
}

func TestOrdinaryChatDraftBehaviorUnchanged(t *testing.T) {
	state := topicDraftState(t)
	state.Chats = []domain.Chat{{ID: 7, Title: "Team", CanSend: true}}

	state, commands := Reduce(state, ComposerValueChanged{ChatID: 7, Value: "chat draft"})
	if state.Drafts[7] != "chat draft" {
		t.Fatalf("chat draft = %q", state.Drafts[7])
	}
	if state.TopicDrafts[topicKey{ChatID: 7}] != "" || len(state.TopicDrafts) != 0 {
		t.Fatalf("topic drafts touched: %#v", state.TopicDrafts)
	}
	if save := commands[0].(SaveDraft); save.TopicID != 0 || save.ChatID != 7 || save.Text != "chat draft" {
		t.Fatalf("save command = %#v", save)
	}

	got, commands := Reduce(state, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})
	if len(commands) != 2 {
		t.Fatalf("submit commands = %#v", commands)
	}
	send := commands[0].(SendText)
	if send.TopicID != 0 || send.ChatID != 7 || send.Text != "chat draft" {
		t.Fatalf("send command = %#v", send)
	}
	if got.Drafts[7] != "" || got.SelectedMessage != send.LocalID {
		t.Fatalf("submit state = drafts:%q selected:%d", got.Drafts[7], got.SelectedMessage)
	}
}
