package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func documentSendState(path string) State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusPhotoSend
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 30
	state.NextLocalID = -7
	state.Drafts[9] = "document caption"
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 51}
	state.PhotoSend = &PhotoSendState{ChatID: 9, Input: []rune(path), PreviousFocus: FocusConversation}
	return state
}

func TestDocumentChooserClassificationAndOptimisticSubmit(t *testing.T) {
	for _, path := range []string{"archive.tar.gz", "notes.txt", "data", "report.PDF", "file.unknownext"} {
		if isPhotoPath(path) || isVideoPath(path) || isAudioPath(path) {
			t.Fatalf("path %q must classify as an unknown document", path)
		}
	}
	for _, path := range []string{"pic.jpg", "pic.JPEG", "pic.png", "pic.gif", "pic.webp", "pic.bmp"} {
		if !isPhotoPath(path) {
			t.Fatalf("isPhotoPath(%q) = false", path)
		}
	}

	state := documentSendState("/tmp/archive.tar.gz")
	now := time.Unix(100, 0)
	got, commands := Reduce(state, ActionReceived{Action: PhotoSendSubmit, At: now})
	want := []Command{
		SendDocument{RequestID: 30, LocalID: -7, ChatID: 9, LocalPath: "/tmp/archive.tar.gz", Caption: "document caption", ReplyToMessageID: 51},
		SaveDraft{RequestID: 31, ChatID: 9},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	message := got.Messages[9][0]
	if message.ID != -7 || message.Kind != domain.MessageDocument || message.Text != "document caption" || message.FileName != "archive.tar.gz" || !message.Outgoing || message.SendState != domain.SendPending || message.Media.File.LocalPath != "/tmp/archive.tar.gz" || !message.Media.File.Downloaded || !message.HasReply || message.ReplyToMessageID != 51 {
		t.Fatalf("optimistic message = %#v", message)
	}
	if got.SelectedMessage != -7 || got.SelectedMessageChat != 9 || got.PhotoSend != nil || got.Drafts[9] != "" || got.ReplyTarget != nil || got.Focus != FocusConversation || got.DocumentSendRequests[-7] != 30 {
		t.Fatalf("post-submit state = %#v", got)
	}

	photo := documentSendState("/tmp/pic.JPG")
	_, photoCommands := Reduce(photo, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := photoCommands[0].(SendPhoto); !ok {
		t.Fatalf("photo command = %T", photoCommands[0])
	}
	video := documentSendState("/tmp/clip.mp4")
	_, videoCommands := Reduce(video, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := videoCommands[0].(SendVideo); !ok {
		t.Fatalf("video command = %T", videoCommands[0])
	}
	audio := documentSendState("/tmp/song.mp3")
	_, audioCommands := Reduce(audio, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := audioCommands[0].(SendAudio); !ok {
		t.Fatalf("audio command = %T", audioCommands[0])
	}
}

func TestDocumentQueueCorrelationFailureRetryAndTerminalReconciliation(t *testing.T) {
	pending, _ := Reduce(documentSendState("/tmp/archive.tar.gz"), ActionReceived{Action: PhotoSendSubmit, At: time.Unix(100, 0)})

	staleEvents := []DocumentQueued{
		{RequestID: 29, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -8, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -7, ChatID: 8, Message: domain.Message{ID: -20, ChatID: 8, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessagePhoto}},
	}
	for _, event := range staleEvents {
		stale, staleCommands := Reduce(pending, event)
		if len(staleCommands) != 0 || !reflect.DeepEqual(stale, pending) {
			t.Fatalf("stale queue result mutated state for %#v: %#v %#v", event, stale, staleCommands)
		}
	}

	collision := cloneReducerState(pending)
	collision.Messages[9] = append(collision.Messages[9], domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageText, Text: "existing"})
	collisionResult, collisionCommands := Reduce(collision, DocumentQueued{RequestID: 30, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}})
	if len(collisionCommands) != 0 || !reflect.DeepEqual(collisionResult, collision) {
		t.Fatalf("queued ID collision mutated state: %#v %#v", collisionResult, collisionCommands)
	}

	queuedMessage := domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument, Outgoing: true, SendState: domain.SendPending}
	queued, _ := Reduce(pending, DocumentQueued{RequestID: 30, LocalID: -7, ChatID: 9, Message: queuedMessage})
	if len(queued.Messages[9]) != 1 || queued.Messages[9][0].ID != -20 || queued.Messages[9][0].Text != "document caption" || queued.Messages[9][0].FileName != "archive.tar.gz" || queued.Messages[9][0].Media.File.LocalPath != "/tmp/archive.tar.gz" || queued.SelectedMessage != -20 {
		t.Fatalf("queued replacement = %#v", queued.Messages[9])
	}
	if _, stillTracked := queued.DocumentSendRequests[-7]; stillTracked {
		t.Fatal("queued document still tracked in ledger")
	}

	failure := domain.AppError{Kind: domain.ErrorNetwork, Op: "send document", Message: "Document send failed", RetryAfter: time.Second}
	for _, event := range []DocumentQueueFailed{
		{RequestID: 29, LocalID: -7, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)},
		{RequestID: 30, LocalID: -8, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)},
		{RequestID: 30, LocalID: -7, ChatID: 8, Error: failure, FailedAt: time.Unix(101, 0)},
	} {
		staleFailure, staleFailureCommands := Reduce(pending, event)
		if len(staleFailureCommands) != 0 || !reflect.DeepEqual(staleFailure, pending) {
			t.Fatalf("stale failure mutated state for %#v: %#v %#v", event, staleFailure, staleFailureCommands)
		}
	}
	failed, _ := Reduce(pending, DocumentQueueFailed{RequestID: 30, LocalID: -7, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)})
	if failed.Messages[9][0].SendState != domain.SendFailed || failed.Messages[9][0].Failure == nil || failed.DocumentSendRequests[-7] != 0 {
		t.Fatalf("queue failure = %#v", failed)
	}
	beforeTooEarly := cloneState(failed)
	tooEarly, commands := Reduce(failed, ActionReceived{Action: Retry, MessageID: -7, At: time.Unix(101, 0)})
	if len(commands) != 0 || !reflect.DeepEqual(tooEarly, beforeTooEarly) {
		t.Fatal("retry before RetryAt mutated state")
	}
	retried, commands := Reduce(failed, ActionReceived{Action: Retry, MessageID: -7, At: time.Unix(102, 0)})
	wantRetry := []Command{SendDocument{RequestID: 32, LocalID: -7, ChatID: 9, LocalPath: "/tmp/archive.tar.gz", Caption: "document caption", ReplyToMessageID: 51}}
	if !reflect.DeepEqual(commands, wantRetry) || retried.Messages[9][0].SendState != domain.SendPending || retried.DocumentSendRequests[-7] != 32 {
		t.Fatalf("retry = %#v commands=%#v", retried.Messages[9][0], commands)
	}

	succeeded, _ := Reduce(pending, TelegramEvent{ReceivedAt: time.Unix(103, 0), Value: telegram.MessageSendSucceeded{OldID: -7, Message: domain.Message{ID: 80, ChatID: 9, Kind: domain.MessageDocument, Text: "document caption", Outgoing: true, SendState: domain.SendSucceeded}}})
	if succeeded.Messages[9][0].ID != 80 || succeeded.Messages[9][0].SendState != domain.SendSucceeded || succeeded.Messages[9][0].Media.File.LocalPath != "/tmp/archive.tar.gz" || succeeded.SelectedMessage != 80 {
		t.Fatalf("terminal success = %#v", succeeded.Messages[9])
	}
	terminalFailed, _ := Reduce(pending, TelegramEvent{ReceivedAt: time.Unix(104, 0), Value: telegram.MessageSendFailed{OldID: -7, Message: domain.Message{ID: -2, ChatID: 9, Kind: domain.MessageDocument}, Error: failure}})
	if terminalFailed.Messages[9][0].ID != -2 || terminalFailed.Messages[9][0].SendState != domain.SendFailed || terminalFailed.Messages[9][0].Media.File.LocalPath != "/tmp/archive.tar.gz" || terminalFailed.SelectedMessage != -2 {
		t.Fatalf("terminal failure = %#v", terminalFailed.Messages[9])
	}
	if terminalFailed.Messages[9][0].Failure == nil || terminalFailed.Messages[9][0].Failure.Op != "send document" {
		t.Fatalf("terminal failure op = %#v", terminalFailed.Messages[9][0].Failure)
	}
	if !strings.Contains(terminalFailed.Messages[9][0].Failure.Error(), "send document") {
		t.Fatalf("terminal failure text = %q", terminalFailed.Messages[9][0].Failure.Error())
	}
}

func TestHandlerSendDocumentMapsExactRequestAndSafeFailure(t *testing.T) {
	request := SendDocument{
		RequestID: 61, LocalID: -7, ChatID: 9, LocalPath: "/tmp/report.pdf",
		Caption: "document caption", ReplyToMessageID: 51,
	}
	queuedMessage := domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}
	client := &handlerClient{sent: queuedMessage}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, request)
	wantRequest := telegram.SendDocumentRequest{
		ChatID: 9, LocalPath: "/tmp/report.pdf", Caption: "document caption", ReplyToMessageID: 51,
	}
	if !reflect.DeepEqual(client.documentSendRequest, wantRequest) || len(events) != 1 {
		t.Fatalf("request=%#v events=%#v", client.documentSendRequest, events)
	}
	queued, ok := events[0].(DocumentQueued)
	if !ok || queued.RequestID != 61 || queued.LocalID != -7 || queued.ChatID != 9 || !reflect.DeepEqual(queued.Message, queuedMessage) {
		t.Fatalf("queued event = %#v", events[0])
	}

	private := "/private/report.pdf transport detail"
	failedClient := &handlerClient{err: errors.New(private)}
	failedHandler := newTestHandler(t, failedClient, &handlerAvatarRenderer{})
	failedEvents := collectHandlerEvents(failedHandler, request)
	if len(failedEvents) != 1 {
		t.Fatalf("failure events = %#v", failedEvents)
	}
	failed, ok := failedEvents[0].(DocumentQueueFailed)
	if !ok || failed.RequestID != 61 || failed.LocalID != -7 || failed.ChatID != 9 || failed.Error.Op != "send document" || failed.Error.Message != "Could not send document" || strings.Contains(failed.Error.Error(), private) {
		t.Fatalf("safe failure = %#v", failedEvents[0])
	}

	unavailable := NewHandler(nil, nil, nil, nil, nil)
	unavailableEvents := collectHandlerEvents(unavailable, request)
	if len(unavailableEvents) != 1 {
		t.Fatalf("unavailable events = %#v", unavailableEvents)
	}
	unavailableFailure, ok := unavailableEvents[0].(DocumentQueueFailed)
	if !ok || unavailableFailure.Error.Op != "send document" || unavailableFailure.Error.Message != "Could not send document" {
		t.Fatalf("unavailable failure = %#v", unavailableEvents[0])
	}
}

func TestDocumentRetryValidatesPathBeforeMutation(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.NextRequestID = 15
	state.SelectedChat = 0
	state.SelectedMessage = -8
	state.Messages[9] = []domain.Message{{
		ID: -8, ChatID: 9, Kind: domain.MessageDocument,
		Text: "caption", Outgoing: true, SendState: domain.SendFailed,
		Failure: &domain.AppError{Kind: domain.ErrorNetwork, Message: "network", RetryAfter: 0},
		Media:   domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/tmp/report.pdf"}},
		RetryAt: time.Unix(1, 0),
	}}
	got, cmds := Reduce(state, ActionReceived{Action: Retry, MessageID: -8, At: time.Unix(10, 0)})
	want := []Command{SendDocument{RequestID: 15, LocalID: -8, ChatID: 9, LocalPath: "/tmp/report.pdf", Caption: "caption"}}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %#v, want %#v", cmds, want)
	}
	if got.Messages[9][0].SendState != domain.SendPending || got.DocumentSendRequests[-8] != 15 {
		t.Fatalf("state = %#v", got)
	}
}
