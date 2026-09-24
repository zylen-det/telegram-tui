package frontend

import (
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestCloudDraftLoadsIntoComposerAndReplyState(t *testing.T) {
	state := InitialState()
	state.ChatRequestID = 4
	state.NextRequestID = 5
	page := telegram.ChatPage{Done: true, Chats: []domain.Chat{{
		ID:      9,
		Title:   "Chat",
		CanSend: true,
		Order:   10,
		Draft:   domain.Draft{Text: "cloud draft", ReplyToMessageID: 44, Date: 123},
	}}}

	commands := updateState(&state, ChatsLoaded{RequestID: 4, Page: page})
	got := state
	if got.Drafts[9] != "cloud draft" || got.DraftReplies[9] != 44 || got.DraftDates[9] != 123 {
		t.Fatalf("loaded draft state = text:%q reply:%d date:%d", got.Drafts[9], got.DraftReplies[9], got.DraftDates[9])
	}
	if got.ReplyTarget == nil || got.ReplyTarget.ChatID != 9 || got.ReplyTarget.MessageID != 44 || got.ReplyTarget.Preview != "Reply draft" {
		t.Fatalf("restored reply target = %#v", got.ReplyTarget)
	}
	if syncState := got.DraftSync[9]; syncState.Dirty || syncState.Pending || syncState.Draft != page.Chats[0].Draft {
		t.Fatalf("loaded sync state = %#v", syncState)
	}
	for _, command := range commands {
		if _, saves := command.(SaveDraft); saves {
			t.Fatal("loading a cloud draft wrote it back")
		}
	}

	updateState(&got, MessagesLoaded{RequestID: got.History[9].RequestID, ChatID: 9, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 44, ChatID: 9, Kind: domain.MessageText, SenderName: "Sender", Text: "reply body"}}}})
	if got.ReplyTarget == nil || got.ReplyTarget.Sender != "Sender" || got.ReplyTarget.Preview != "reply body" {
		t.Fatalf("hydrated reply target = %#v", got.ReplyTarget)
	}
}

func TestLocalDraftProtectsAgainstStaleUpdateThenAcceptsRemoteChanges(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer

	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "local draft"})
	local := state
	if len(commands) != 1 {
		t.Fatalf("local commands = %#v", commands)
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.ChatID != 9 || save.Text != "local draft" || save.RequestID == 0 {
		t.Fatalf("save command = %#v", commands[0])
	}

	updateState(&local, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "old cloud", Date: 10}}})
	stale := local
	if stale.Drafts[9] != "local draft" || !stale.DraftSync[9].Dirty {
		t.Fatalf("stale update replaced local draft: %#v", stale.DraftSync[9])
	}

	updateState(&stale, TelegramEvent{Value: &telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "local draft", Date: 20}}})
	confirmed := stale
	if confirmed.Drafts[9] != "local draft" || confirmed.DraftSync[9].Dirty || confirmed.DraftDates[9] != 20 {
		t.Fatalf("matching update did not confirm draft: %#v", confirmed.DraftSync[9])
	}

	updateState(&confirmed, DraftSaved{RequestID: save.RequestID, ChatID: 9, Date: 21})
	saved := confirmed
	if saved.DraftSync[9].Pending || saved.DraftDates[9] != 21 {
		t.Fatalf("save result = %#v", saved.DraftSync[9])
	}
	updateState(&saved, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "other device", Date: 30}}})
	remote := saved
	if remote.Drafts[9] != "other device" || remote.DraftDates[9] != 30 {
		t.Fatalf("clean remote update = %#v", remote.DraftSync[9])
	}
}

func TestSwitchingChatsReleasesLocalDraftGuardAfterQueuedSave(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer
	state.Chats = append(state.Chats, domain.Chat{ID: 10, CanSend: true})
	updateState(&state, ComposerValueChanged{ChatID: 9, Value: "local draft"})
	if !state.DraftSync[9].Dirty {
		t.Fatal("local draft was not guarded")
	}

	updateState(&state, ActionReceived{Action: SelectChat, ChatID: 10})
	switched := state
	if switched.DraftSync[9].Dirty {
		t.Fatal("chat switch retained the active local-edit guard")
	}
	requestID := switched.DraftSync[9].RequestID
	updateState(&switched, DraftSaved{RequestID: requestID, ChatID: 9, Date: 20})
	updateState(&switched, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "other device", Date: 30}}})
	remote := switched
	if remote.Drafts[9] != "other device" {
		t.Fatalf("inactive remote draft = %q", remote.Drafts[9])
	}
}

func TestReplyAndSendDraftCommandsCarryLatestIdentity(t *testing.T) {
	fixture := func() State {
		state := selectedWritableState()
		state.Focus = FocusConversation
		state.Drafts[9] = "draft"
		state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
		state.SelectedMessageChat, state.SelectedMessage = 9, 22
		return state
	}
	replying := fixture()
	commands := updateState(&replying, ActionReceived{Action: ReplyMessage})
	if len(commands) != 1 {
		t.Fatalf("reply commands = %#v", commands)
	}
	save := commands[0].(SaveDraft)
	if save.Text != "draft" || save.ReplyToMessageID != 22 || replying.DraftReplies[9] != 22 {
		t.Fatalf("reply save = %#v state=%#v", save, replying.ReplyTarget)
	}

	// Cancel and submit are independent branches from the replying state,
	// so each rebuilds its own fixture.
	cancelled := fixture()
	updateState(&cancelled, ActionReceived{Action: ReplyMessage})
	commands = updateState(&cancelled, ActionReceived{Action: CancelReply})
	if len(commands) != 1 || commands[0].(SaveDraft).ReplyToMessageID != 0 || commands[0].(SaveDraft).Text != "draft" || cancelled.ReplyTarget != nil {
		t.Fatalf("cancel reply = commands:%#v target:%#v", commands, cancelled.ReplyTarget)
	}

	submitted := fixture()
	updateState(&submitted, ActionReceived{Action: ReplyMessage})
	commands = updateState(&submitted, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})
	if len(commands) != 2 {
		t.Fatalf("submit commands = %#v", commands)
	}
	if send := commands[0].(SendText); send.Text != "draft" || send.ReplyToMessageID != 22 {
		t.Fatalf("send command = %#v", send)
	}
	if clear := commands[1].(SaveDraft); clear.Text != "" || clear.ReplyToMessageID != 0 {
		t.Fatalf("clear command = %#v", clear)
	}
	if submitted.Drafts[9] != "" || submitted.DraftReplies[9] != 0 || submitted.ReplyTarget != nil {
		t.Fatalf("submitted draft state was not cleared")
	}
}

func TestDraftSaveFailureIsCurrentSafeAndNonDestructive(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer
	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "private draft text"})
	save := commands[0].(SaveDraft)

	staleCommands := updateState(&state, DraftSaveFailed{RequestID: save.RequestID + 1, ChatID: 9, Error: domain.AppError{Message: "private raw failure"}})
	if len(staleCommands) != 0 || state.Toast != nil || state.Drafts[9] != "private draft text" || state.DraftSync[9].RequestID != save.RequestID || !state.DraftSync[9].Pending {
		t.Fatalf("stale draft failure changed pending save: draft=%q sync=%#v effects=%#v", state.Drafts[9], state.DraftSync[9], staleCommands)
	}
	updateState(&state, DraftSaveFailed{RequestID: save.RequestID, ChatID: 9, Error: domain.AppError{Message: "private raw failure"}})
	failed := state
	if failed.Drafts[9] != "private draft text" || failed.Toast == nil || failed.Toast.Message != "Could not sync draft" || strings.Contains(failed.Toast.Message, "private") {
		t.Fatalf("current draft failure = draft:%q toast:%#v", failed.Drafts[9], failed.Toast)
	}
}
