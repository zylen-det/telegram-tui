package frontend

import (
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func reducerBranchCloneState() State {
	state := InitialState()
	state.Layout = LayoutWide
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9, Title: "active"}, {ID: 10, Title: "other"}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, Text: "active", SentAt: time.Unix(1, 0)}}
	state.Messages[10] = []domain.Message{{ID: 2, ChatID: 10, Text: "other", SentAt: time.Unix(2, 0)}}
	state.SelectedMessageChat = 9
	state.SelectedMessage = 1
	state.History[9] = HistoryState{}
	return state
}

func TestScalarReducerEventsDoNotCloneCollectionBranches(t *testing.T) {
	input := reducerBranchCloneState()
	got, commands := updateState(input, TerminalFocusChanged{Focused: false})
	if len(commands) != 0 || got.TerminalFocused {
		t.Fatalf("terminal focus result = %v, commands=%#v", got.TerminalFocused, commands)
	}
	if &got.Messages[9][0] != &input.Messages[9][0] || &got.Chats[0] != &input.Chats[0] {
		t.Fatal("scalar event cloned an untouched collection branch")
	}
}

func TestMessageEditedCopiesOnlyAffectedChatMessages(t *testing.T) {
	input := reducerBranchCloneState()
	before := cloneReducerState(input)
	editedAt := time.Unix(50, 0)

	got, commands := updateState(input, TelegramEvent{Value: telegram.MessageEdited{ChatID: 9, MessageID: 1, EditedAt: editedAt}})

	if len(commands) != 0 || !got.Messages[9][0].EditedAt.Equal(editedAt) {
		t.Fatalf("edited message = %#v, commands=%#v", got.Messages[9][0], commands)
	}
	if &got.Messages[9][0] == &input.Messages[9][0] {
		t.Fatal("affected chat retained the input message backing slice")
	}
	if &got.Messages[10][0] != &input.Messages[10][0] {
		t.Fatal("unaffected chat message slice was cloned")
	}
	requireInputUnchanged(t, "message edited", input, before)
}

func TestChatUpsertCopiesDraftAndChatBranchesWithoutMessages(t *testing.T) {
	input := reducerBranchCloneState()
	input.Drafts[9] = "old"
	input.DraftSync[9] = DraftSyncState{Draft: domain.Draft{Text: "old"}}
	before := cloneReducerState(input)
	updated := input.Chats[0]
	updated.Title = "renamed"
	updated.Draft = domain.Draft{Text: "cloud", Date: 5}

	got, commands := updateState(input, TelegramEvent{Value: telegram.ChatUpserted{Chat: updated}})

	if got.Chats[chatIndex(got.Chats, 9)].Title != "renamed" || got.Drafts[9] != "cloud" {
		t.Fatalf("chat upsert result: chat=%#v draft=%q", got.Chats, got.Drafts[9])
	}
	if len(commands) != 0 || &got.Messages[9][0] != &input.Messages[9][0] {
		t.Fatalf("commands=%#v or messages branch was cloned", commands)
	}
	requireInputUnchanged(t, "chat upsert", input, before)
}

func TestMessagesLoadedCopiesOnlyTargetAndLoadTrackingBranches(t *testing.T) {
	input := reducerBranchCloneState()
	input.History[9] = HistoryState{RequestID: 44, Loading: true}
	before := cloneReducerState(input)
	page := telegram.MessagePage{Messages: []domain.Message{{ID: 3, ChatID: 9, Text: "loaded", SentAt: time.Unix(3, 0)}}, Done: true}

	got, commands := updateState(input, MessagesLoaded{RequestID: 44, ChatID: 9, Page: page})

	if messageIndex(got.Messages[9], 3) < 0 || got.History[9].Loading || !got.History[9].Done {
		t.Fatalf("messages loaded result: messages=%#v history=%#v", got.Messages[9], got.History[9])
	}
	if len(commands) != 0 || &got.Messages[10][0] != &input.Messages[10][0] {
		t.Fatalf("commands=%#v or unrelated message branch was cloned", commands)
	}
	requireInputUnchanged(t, "messages loaded", input, before)
}

func TestMessageUpsertCopiesOnlyBranchesItUpdates(t *testing.T) {
	input := reducerBranchCloneState()
	input.History[9] = HistoryState{ViewOffset: 2}
	before := cloneReducerState(input)
	message := domain.Message{ID: 3, ChatID: 9, Text: "new", SentAt: time.Unix(3, 0)}

	got, commands := updateState(input, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})

	if len(commands) != 0 || messageIndex(got.Messages[9], 3) < 0 {
		t.Fatalf("upsert messages = %#v, commands=%#v", got.Messages[9], commands)
	}
	if &got.Messages[10][0] != &input.Messages[10][0] {
		t.Fatal("upsert cloned an unaffected chat message slice")
	}
	requireInputUnchanged(t, "message upsert", input, before)
}

func TestChatNavigationCopiesOnlyMutableSelectionBranches(t *testing.T) {
	input := reducerBranchCloneState()
	input.Focus = FocusChats
	input.DraftSync[9] = DraftSyncState{Dirty: true}
	before := cloneReducerState(input)

	got, commands := updateState(input, ActionReceived{Action: SelectNext})

	if got.SelectedChat != 1 {
		t.Fatalf("selected chat = %d, want 1", got.SelectedChat)
	}
	if len(commands) != 3 {
		t.Fatalf("commands = %#v, want load/close/open", commands)
	}
	if &got.Messages[9][0] != &input.Messages[9][0] || &got.Chats[0] != &input.Chats[0] {
		t.Fatal("chat navigation cloned an untouched chat or message branch")
	}
	if got.DraftSync[9].Dirty {
		t.Fatal("previous chat draft guard was not released")
	}
	requireInputUnchanged(t, "chat navigation", input, before)
}

func TestMessageSelectionNavigationCopiesHistoryOnly(t *testing.T) {
	input := reducerBranchCloneState()
	input.Messages[9] = append(input.Messages[9], domain.Message{ID: 3, ChatID: 9, Text: "newer", SentAt: time.Unix(3, 0)})
	input.SelectedMessage = 3
	before := cloneReducerState(input)

	got, commands := updateState(input, ActionReceived{Action: SelectPreviousMessage})

	if len(commands) != 0 || got.SelectedMessage != 1 || got.History[9].ViewOffset == input.History[9].ViewOffset {
		t.Fatalf("selection=%d history=%#v commands=%#v", got.SelectedMessage, got.History[9], commands)
	}
	if &got.Messages[9][0] != &input.Messages[9][0] || &got.Chats[0] != &input.Chats[0] {
		t.Fatal("message navigation cloned an untouched chat or message branch")
	}
	requireInputUnchanged(t, "message navigation", input, before)
}
