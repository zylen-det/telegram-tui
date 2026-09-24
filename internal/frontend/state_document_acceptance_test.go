package frontend

import (
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
	commands := updateState(&state, ActionReceived{Action: PhotoSendSubmit, At: now})
	want := []Effect{
		SendDocument{RequestID: 30, LocalID: -7, ChatID: 9, LocalPath: "/tmp/archive.tar.gz", Caption: "document caption", ReplyToMessageID: 51},
		SaveDraft{RequestID: 31, ChatID: 9},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	message := state.Messages[9][0]
	if message.ID != -7 || message.Kind != domain.MessageDocument || message.Text != "document caption" || message.FileName != "archive.tar.gz" || !message.Outgoing || message.SendState != domain.SendPending || message.Media.File.LocalPath != "/tmp/archive.tar.gz" || !message.Media.File.Downloaded || !message.HasReply || message.ReplyToMessageID != 51 {
		t.Fatalf("optimistic message = %#v", message)
	}
	if state.SelectedMessage != -7 || state.SelectedMessageChat != 9 || state.PhotoSend != nil || state.Drafts[9] != "" || state.ReplyTarget != nil || state.Focus != FocusConversation || state.DocumentSendRequests[-7] != 30 {
		t.Fatalf("post-submit state = %#v", state)
	}

	photo := documentSendState("/tmp/pic.JPG")
	photoCommands := updateState(&photo, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := photoCommands[0].(SendPhoto); !ok {
		t.Fatalf("photo command = %T", photoCommands[0])
	}
	video := documentSendState("/tmp/clip.mp4")
	videoCommands := updateState(&video, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := videoCommands[0].(SendVideo); !ok {
		t.Fatalf("video command = %T", videoCommands[0])
	}
	audio := documentSendState("/tmp/song.mp3")
	audioCommands := updateState(&audio, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := audioCommands[0].(SendAudio); !ok {
		t.Fatalf("audio command = %T", audioCommands[0])
	}
}

func TestDocumentQueueCorrelationFailureRetryAndTerminalReconciliation(t *testing.T) {
	// updateState mutates in place, so each independent scenario below starts
	// from its own freshly submitted fixture.
	submit := func() State {
		state := documentSendState("/tmp/archive.tar.gz")
		updateState(&state, ActionReceived{Action: PhotoSendSubmit, At: time.Unix(100, 0)})
		return state
	}

	staleEvents := []DocumentQueued{
		{RequestID: 29, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -8, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -7, ChatID: 8, Message: domain.Message{ID: -20, ChatID: 8, Kind: domain.MessageDocument}},
		{RequestID: 30, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessagePhoto}},
	}
	for _, event := range staleEvents {
		stale := submit()
		staleCommands := updateState(&stale, event)
		if len(staleCommands) != 0 || !reflect.DeepEqual(stale, submit()) {
			t.Fatalf("stale queue result mutated state for %#v: %#v %#v", event, stale, staleCommands)
		}
	}

	collisionFixture := func() State {
		state := submit()
		state.Messages[9] = append(state.Messages[9], domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageText, Text: "existing"})
		return state
	}
	collision := collisionFixture()
	collisionCommands := updateState(&collision, DocumentQueued{RequestID: 30, LocalID: -7, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument}})
	if len(collisionCommands) != 0 || !reflect.DeepEqual(collision, collisionFixture()) {
		t.Fatalf("queued ID collision mutated state: %#v %#v", collision, collisionCommands)
	}

	queuedMessage := domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageDocument, Outgoing: true, SendState: domain.SendPending}
	queued := submit()
	updateState(&queued, DocumentQueued{RequestID: 30, LocalID: -7, ChatID: 9, Message: queuedMessage})
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
		staleFailure := submit()
		staleFailureCommands := updateState(&staleFailure, event)
		if len(staleFailureCommands) != 0 || !reflect.DeepEqual(staleFailure, submit()) {
			t.Fatalf("stale failure mutated state for %#v: %#v %#v", event, staleFailure, staleFailureCommands)
		}
	}
	failed := submit()
	updateState(&failed, DocumentQueueFailed{RequestID: 30, LocalID: -7, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)})
	if failed.Messages[9][0].SendState != domain.SendFailed || failed.Messages[9][0].Failure == nil || failed.DocumentSendRequests[-7] != 0 {
		t.Fatalf("queue failure = %#v", failed)
	}
	beforeTooEarly := cloneState(failed)
	commands := updateState(&failed, ActionReceived{Action: Retry, MessageID: -7, At: time.Unix(101, 0)})
	if len(commands) != 0 || !reflect.DeepEqual(failed, beforeTooEarly) || len(failed.DocumentSendRequests) != 0 {
		t.Fatalf("retry before RetryAt changed message or ledger: state=%#v effects=%#v", failed, commands)
	}
	// The too-early retry is asserted to be a full no-op, so the valid retry
	// continues on the same state.
	commands = updateState(&failed, ActionReceived{Action: Retry, MessageID: -7, At: time.Unix(102, 0)})
	wantRetry := []Effect{SendDocument{RequestID: 32, LocalID: -7, ChatID: 9, LocalPath: "/tmp/archive.tar.gz", Caption: "document caption", ReplyToMessageID: 51}}
	if !reflect.DeepEqual(commands, wantRetry) || failed.Messages[9][0].SendState != domain.SendPending || failed.DocumentSendRequests[-7] != 32 {
		t.Fatalf("retry = %#v commands=%#v", failed.Messages[9][0], commands)
	}

	succeeded := submit()
	updateState(&succeeded, TelegramEvent{ReceivedAt: time.Unix(103, 0), Value: telegram.MessageSendSucceeded{OldID: -7, Message: domain.Message{ID: 80, ChatID: 9, Kind: domain.MessageDocument, Text: "document caption", Outgoing: true, SendState: domain.SendSucceeded}}})
	if succeeded.Messages[9][0].ID != 80 || succeeded.Messages[9][0].SendState != domain.SendSucceeded || succeeded.Messages[9][0].Media.File.LocalPath != "/tmp/archive.tar.gz" || succeeded.SelectedMessage != 80 {
		t.Fatalf("terminal success = %#v", succeeded.Messages[9])
	}
	terminalFailed := submit()
	updateState(&terminalFailed, TelegramEvent{ReceivedAt: time.Unix(104, 0), Value: telegram.MessageSendFailed{OldID: -7, Message: domain.Message{ID: -2, ChatID: 9, Kind: domain.MessageDocument}, Error: failure}})
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
	cmds := updateState(&state, ActionReceived{Action: Retry, MessageID: -8, At: time.Unix(10, 0)})
	want := []Effect{SendDocument{RequestID: 15, LocalID: -8, ChatID: 9, LocalPath: "/tmp/report.pdf", Caption: "caption"}}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %#v, want %#v", cmds, want)
	}
	if state.Messages[9][0].SendState != domain.SendPending || state.DocumentSendRequests[-8] != 15 {
		t.Fatalf("state = %#v", state)
	}
}
