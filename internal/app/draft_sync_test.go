package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
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

	got, commands := Reduce(state, ChatsLoaded{RequestID: 4, Page: page})
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

	got, _ = Reduce(got, MessagesLoaded{RequestID: got.History[9].RequestID, ChatID: 9, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 44, ChatID: 9, Kind: domain.MessageText, SenderName: "Sender", Text: "reply body"}}}})
	if got.ReplyTarget == nil || got.ReplyTarget.Sender != "Sender" || got.ReplyTarget.Preview != "reply body" {
		t.Fatalf("hydrated reply target = %#v", got.ReplyTarget)
	}
}

func TestLocalDraftProtectsAgainstStaleUpdateThenAcceptsRemoteChanges(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer

	local, commands := Reduce(state, ComposerValueChanged{ChatID: 9, Value: "local draft"})
	if len(commands) != 1 {
		t.Fatalf("local commands = %#v", commands)
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.ChatID != 9 || save.Text != "local draft" || save.RequestID == 0 {
		t.Fatalf("save command = %#v", commands[0])
	}

	stale, _ := Reduce(local, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "old cloud", Date: 10}}})
	if stale.Drafts[9] != "local draft" || !stale.DraftSync[9].Dirty {
		t.Fatalf("stale update replaced local draft: %#v", stale.DraftSync[9])
	}

	confirmed, _ := Reduce(stale, TelegramEvent{Value: &telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "local draft", Date: 20}}})
	if confirmed.Drafts[9] != "local draft" || confirmed.DraftSync[9].Dirty || confirmed.DraftDates[9] != 20 {
		t.Fatalf("matching update did not confirm draft: %#v", confirmed.DraftSync[9])
	}

	saved, _ := Reduce(confirmed, DraftSaved{RequestID: save.RequestID, ChatID: 9, Date: 21})
	if saved.DraftSync[9].Pending || saved.DraftDates[9] != 21 {
		t.Fatalf("save result = %#v", saved.DraftSync[9])
	}
	remote, _ := Reduce(saved, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "other device", Date: 30}}})
	if remote.Drafts[9] != "other device" || remote.DraftDates[9] != 30 {
		t.Fatalf("clean remote update = %#v", remote.DraftSync[9])
	}
}

func TestSwitchingChatsReleasesLocalDraftGuardAfterQueuedSave(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusComposer
	state.Chats = append(state.Chats, domain.Chat{ID: 10, CanSend: true})
	state, _ = Reduce(state, ComposerValueChanged{ChatID: 9, Value: "local draft"})
	if !state.DraftSync[9].Dirty {
		t.Fatal("local draft was not guarded")
	}

	switched, _ := Reduce(state, ActionReceived{Action: SelectChat, ChatID: 10})
	if switched.DraftSync[9].Dirty {
		t.Fatal("chat switch retained the active local-edit guard")
	}
	requestID := switched.DraftSync[9].RequestID
	switched, _ = Reduce(switched, DraftSaved{RequestID: requestID, ChatID: 9, Date: 20})
	remote, _ := Reduce(switched, TelegramEvent{Value: telegram.DraftChanged{ChatID: 9, Draft: domain.Draft{Text: "other device", Date: 30}}})
	if remote.Drafts[9] != "other device" {
		t.Fatalf("inactive remote draft = %q", remote.Drafts[9])
	}
}

func TestReplyAndSendDraftCommandsCarryLatestIdentity(t *testing.T) {
	state := selectedWritableState()
	state.Focus = FocusConversation
	state.Drafts[9] = "draft"
	state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, SenderName: "Sender", Text: "body"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 22

	replying, commands := Reduce(state, ActionReceived{Action: ReplyMessage})
	if len(commands) != 1 {
		t.Fatalf("reply commands = %#v", commands)
	}
	save := commands[0].(SaveDraft)
	if save.Text != "draft" || save.ReplyToMessageID != 22 || replying.DraftReplies[9] != 22 {
		t.Fatalf("reply save = %#v state=%#v", save, replying.ReplyTarget)
	}

	cancelled, commands := Reduce(replying, ActionReceived{Action: CancelReply})
	if len(commands) != 1 || commands[0].(SaveDraft).ReplyToMessageID != 0 || commands[0].(SaveDraft).Text != "draft" || cancelled.ReplyTarget != nil {
		t.Fatalf("cancel reply = commands:%#v target:%#v", commands, cancelled.ReplyTarget)
	}

	submitted, commands := Reduce(replying, ActionReceived{Action: ComposerSubmit, At: time.Unix(100, 0)})
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
	state, commands := Reduce(state, ComposerValueChanged{ChatID: 9, Value: "private draft text"})
	save := commands[0].(SaveDraft)

	stale, _ := Reduce(state, DraftSaveFailed{RequestID: save.RequestID + 1, ChatID: 9, Error: domain.AppError{Message: "private raw failure"}})
	if stale.Toast != nil || !reflect.DeepEqual(stale, state) {
		t.Fatal("stale draft failure changed state")
	}
	failed, _ := Reduce(state, DraftSaveFailed{RequestID: save.RequestID, ChatID: 9, Error: domain.AppError{Message: "private raw failure"}})
	if failed.Drafts[9] != "private draft text" || failed.Toast == nil || failed.Toast.Message != "Could not sync draft" || strings.Contains(failed.Toast.Message, "private") {
		t.Fatalf("current draft failure = draft:%q toast:%#v", failed.Drafts[9], failed.Toast)
	}
}

func TestHandlerDraftSaveMapsExactRequestAndSafeFailure(t *testing.T) {
	client := &handlerClient{}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	handler.now = func() time.Time { return time.Unix(123, 0) }
	command := SaveDraft{RequestID: 7, ChatID: 9, Text: "private draft", ReplyToMessageID: 44}
	events := collectHandlerEvents(handler, command)
	if client.draftRequest != (telegram.SetDraftRequest{ChatID: 9, Text: "private draft", ReplyToMessageID: 44}) {
		t.Fatalf("draft request = %#v", client.draftRequest)
	}
	if !reflect.DeepEqual(events, []Event{DraftSaved{RequestID: 7, ChatID: 9, Date: 123}}) {
		t.Fatalf("save events = %#v", events)
	}

	client.err = errors.New("private draft transport detail")
	events = collectHandlerEvents(handler, SaveDraft{RequestID: 8, ChatID: 9, Text: "another private draft"})
	failed, ok := events[0].(DraftSaveFailed)
	if !ok || failed.RequestID != 8 || failed.ChatID != 9 || failed.Error.Message != "Could not sync draft" || failed.Error.Cause != nil || strings.Contains(failed.Error.Error(), "private") {
		t.Fatalf("failure event = %#v", events[0])
	}
}

type blockingDraftClient struct {
	*handlerClient
	mu      sync.Mutex
	calls   []telegram.SetDraftRequest
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingDraftClient) SetDraft(ctx context.Context, request telegram.SetDraftRequest) error {
	c.mu.Lock()
	c.calls = append(c.calls, request)
	first := len(c.calls) == 1
	c.mu.Unlock()
	if first {
		c.once.Do(func() { close(c.started) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestHandlerCoalescesConcurrentDraftSavesPerChatLatestWins(t *testing.T) {
	client := &blockingDraftClient{
		handlerClient: &handlerClient{},
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := make(chan Event, 3)
	done := make(chan struct{})
	go func() {
		handler.Handle(context.Background(), SaveDraft{RequestID: 1, ChatID: 9, Text: "first"}, func(event Event) { events <- event })
		close(done)
	}()
	<-client.started
	handler.Handle(context.Background(), SaveDraft{RequestID: 2, ChatID: 9, Text: "second"}, func(event Event) { events <- event })
	handler.Handle(context.Background(), SaveDraft{RequestID: 3, ChatID: 9, Text: "latest"}, func(event Event) { events <- event })
	close(client.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coalesced draft saves")
	}

	client.mu.Lock()
	calls := append([]telegram.SetDraftRequest(nil), client.calls...)
	client.mu.Unlock()
	want := []telegram.SetDraftRequest{{ChatID: 9, Text: "first"}, {ChatID: 9, Text: "latest"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("draft calls = %#v, want %#v", calls, want)
	}
	close(events)
	var requestIDs []uint64
	for event := range events {
		requestIDs = append(requestIDs, event.(DraftSaved).RequestID)
	}
	if !reflect.DeepEqual(requestIDs, []uint64{1, 3}) {
		t.Fatalf("save event request IDs = %#v", requestIDs)
	}
}
