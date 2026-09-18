package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestAudioChooserClassificationAndOptimisticSubmit(t *testing.T) {
	for _, path := range []string{"a.mp3", "a.M4A", "a.AaC", "a.flac", "a.WAV", "a.ogg", "a.OPUS"} {
		if !isAudioPath(path) {
			t.Errorf("isAudioPath(%q) = false", path)
		}
	}
	for _, path := range []string{"clip.MP4", "photo.jpg", "song.mp3.tmp"} {
		if isAudioPath(path) {
			t.Errorf("isAudioPath(%q) = true", path)
		}
	}
	if !isVideoPath("clip.MP4") || isVideoPath("song.mp3") {
		t.Fatal("existing Video classification regressed")
	}

	state := audioSendState("/tmp/song.MP3")
	now := time.Unix(100, 0)
	got, commands := Reduce(state, ActionReceived{Action: PhotoSendSubmit, At: now})
	want := []Command{
		SendAudio{RequestID: 10, LocalID: -1, ChatID: 9, LocalPath: "/tmp/song.MP3", Caption: "composer caption", ReplyToMessageID: 44},
		SaveDraft{RequestID: 11, ChatID: 9},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	message := got.Messages[9][0]
	if message.ID != -1 || message.Kind != domain.MessageAudio || message.Text != "composer caption" || !message.Outgoing || message.SendState != domain.SendPending || message.Media.File.LocalPath != "/tmp/song.MP3" || !message.Media.File.Downloaded || !message.HasReply || message.ReplyToMessageID != 44 {
		t.Fatalf("optimistic message = %#v", message)
	}
	if got.SelectedMessage != -1 || got.SelectedMessageChat != 9 || got.PhotoSend != nil || got.Drafts[9] != "" || got.ReplyTarget != nil || got.Focus != FocusConversation || got.AudioSendRequests[-1] != 10 {
		t.Fatalf("post-submit state = %#v", got)
	}

	unknown := audioSendState("/tmp/photo.unknown")
	_, unknownCommands := Reduce(unknown, ActionReceived{Action: PhotoSendSubmit, At: now})
	if _, ok := unknownCommands[0].(SendDocument); !ok {
		t.Fatalf("unknown-path fallback command = %T", unknownCommands[0])
	}
}

func TestAudioQueueCorrelationFailureRetryAndTerminalReconciliation(t *testing.T) {
	pending, _ := Reduce(audioSendState("/tmp/song.mp3"), ActionReceived{Action: PhotoSendSubmit, At: time.Unix(100, 0)})

	stale, staleCommands := Reduce(pending, AudioQueued{RequestID: 9, LocalID: -1, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageAudio}})
	if len(staleCommands) != 0 || !reflect.DeepEqual(stale, pending) {
		t.Fatalf("stale queue result mutated state: %#v %#v", stale, staleCommands)
	}

	queuedMessage := domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageAudio, Outgoing: true, SendState: domain.SendPending}
	queued, _ := Reduce(pending, AudioQueued{RequestID: 10, LocalID: -1, ChatID: 9, Message: queuedMessage})
	if len(queued.Messages[9]) != 1 || queued.Messages[9][0].ID != -20 || queued.Messages[9][0].Text != "composer caption" || queued.Messages[9][0].Media.File.LocalPath != "/tmp/song.mp3" || queued.SelectedMessage != -20 {
		t.Fatalf("queued replacement = %#v", queued)
	}

	failure := domain.AppError{Kind: domain.ErrorNetwork, Op: "send audio", Message: "Audio send failed", RetryAfter: time.Second}
	failed, _ := Reduce(pending, AudioQueueFailed{RequestID: 10, LocalID: -1, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)})
	if failed.Messages[9][0].SendState != domain.SendFailed || failed.Messages[9][0].Failure == nil || failed.AudioSendRequests[-1] != 0 {
		t.Fatalf("queue failure = %#v", failed)
	}
	beforeTooEarly := cloneState(failed)
	tooEarly, commands := Reduce(failed, ActionReceived{Action: Retry, MessageID: -1, At: time.Unix(101, 0)})
	if len(commands) != 0 || !reflect.DeepEqual(tooEarly, beforeTooEarly) {
		t.Fatal("retry before RetryAt mutated state")
	}
	retried, commands := Reduce(failed, ActionReceived{Action: Retry, MessageID: -1, At: time.Unix(102, 0)})
	wantRetry := []Command{SendAudio{RequestID: 12, LocalID: -1, ChatID: 9, LocalPath: "/tmp/song.mp3", Caption: "composer caption", ReplyToMessageID: 44}}
	if !reflect.DeepEqual(commands, wantRetry) || retried.Messages[9][0].SendState != domain.SendPending || retried.AudioSendRequests[-1] != 12 {
		t.Fatalf("retry = %#v commands=%#v", retried.Messages[9][0], commands)
	}

	succeeded, _ := Reduce(pending, TelegramEvent{ReceivedAt: time.Unix(103, 0), Value: telegram.MessageSendSucceeded{OldID: -1, Message: domain.Message{ID: 80, ChatID: 9, Kind: domain.MessageAudio, Text: "composer caption", Outgoing: true, SendState: domain.SendSucceeded}}})
	if succeeded.Messages[9][0].ID != 80 || succeeded.Messages[9][0].SendState != domain.SendSucceeded || succeeded.Messages[9][0].Media.File.LocalPath != "/tmp/song.mp3" || succeeded.SelectedMessage != 80 {
		t.Fatalf("terminal success = %#v", succeeded)
	}
	terminalFailed, _ := Reduce(pending, TelegramEvent{ReceivedAt: time.Unix(104, 0), Value: telegram.MessageSendFailed{OldID: -1, Message: domain.Message{ID: -2, ChatID: 9, Kind: domain.MessageAudio}, Error: failure}})
	if terminalFailed.Messages[9][0].ID != -2 || terminalFailed.Messages[9][0].SendState != domain.SendFailed || terminalFailed.Messages[9][0].Media.File.LocalPath != "/tmp/song.mp3" || terminalFailed.SelectedMessage != -2 {
		t.Fatalf("terminal failure = %#v", terminalFailed)
	}
}

func TestAudioRetryValidatesPathBeforeMutation(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.Focus = FocusConversation
	state.Messages[9] = []domain.Message{{ID: -1, ChatID: 9, Kind: domain.MessageAudio, SendState: domain.SendFailed}}
	state.SelectedMessageChat, state.SelectedMessage = 9, -1
	before := cloneState(state)
	got, commands := Reduce(state, ActionReceived{Action: Retry, MessageID: -1, At: time.Now()})
	if len(commands) != 0 || !reflect.DeepEqual(got, before) {
		t.Fatalf("missing-path retry mutated state: %#v %#v", got, commands)
	}
}

func TestAudioHandlerPassesFullRequestAndCorrelatesResultAndError(t *testing.T) {
	request := telegram.SendAudioRequest{ChatID: 9, LocalPath: "/tmp/song.mp3", Caption: "caption", ReplyToMessageID: 44}
	client := &handlerClient{sent: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageAudio}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, SendAudio{RequestID: 8, LocalID: -1, ChatID: request.ChatID, LocalPath: request.LocalPath, Caption: request.Caption, ReplyToMessageID: request.ReplyToMessageID})
	if client.audioSendRequest != request || len(events) != 1 {
		t.Fatalf("request/events = %#v/%#v", client.audioSendRequest, events)
	}
	queued, ok := events[0].(AudioQueued)
	if !ok || queued.RequestID != 8 || queued.LocalID != -1 || queued.ChatID != 9 || queued.Message.ID != -20 {
		t.Fatalf("queued = %#v", events[0])
	}

	client.err = errors.New("private transport detail")
	events = collectHandlerEvents(handler, SendAudio{RequestID: 9, LocalID: -1, ChatID: 9, LocalPath: "/tmp/song.mp3"})
	failed, ok := events[0].(AudioQueueFailed)
	if !ok || failed.RequestID != 9 || failed.LocalID != -1 || failed.ChatID != 9 || failed.Error.Op != "send audio" || strings.Contains(failed.Error.Error(), "private transport detail") {
		t.Fatalf("failed = %#v", events[0])
	}
}

func TestAudioExternalOpenReceivedOutgoingPendingStaleFailureAndRetry(t *testing.T) {
	for _, outgoing := range []bool{false, true} {
		state := audioOpenState(outgoing)
		if outgoing {
			state.Messages[9][0].Media.File = domain.MediaFileRef{ID: 702, UniqueID: "audio-local", Downloaded: true, LocalPath: "/tmp/local.mp3"}
		}
		menu, _ := Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
		if menu.MessageMenu == nil || menu.MessageMenu.MediaFile != state.Messages[9][0].Media.File || menu.MessageMenu.MediaKind != domain.MessageAudio {
			t.Fatalf("outgoing=%t menu=%#v", outgoing, menu.MessageMenu)
		}
		started, commands := Reduce(menu, ActionReceived{Action: ViewMessageMedia})
		want := []Command{OpenMessageMediaFile{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Audio", File: state.Messages[9][0].Media.File}}
		if !reflect.DeepEqual(commands, want) || started.Modal != nil || started.Toast == nil || started.Toast.Message != "Opening audio…" || started.AudioOpenPending[77] != 41 {
			t.Fatalf("outgoing=%t started=%#v commands=%#v", outgoing, started, commands)
		}
		duplicate, duplicateCommands := Reduce(started, ActionReceived{Action: ViewMessageMedia})
		if len(duplicateCommands) != 0 || !reflect.DeepEqual(duplicate, started) {
			t.Fatal("double dispatch was not suppressed")
		}
		pendingMenu, _ := Reduce(started, ActionReceived{Action: OpenMessageActionMenu})
		if pendingMenu.MessageMenu == nil || pendingMenu.MessageMenu.MediaFile != (domain.MediaFileRef{}) {
			t.Fatal("pending Audio action stayed visible")
		}

		stale, _ := Reduce(started, MessageMediaOpened{RequestID: 40, ChatID: 9, MessageID: 77, Title: "Audio", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/stale.mp3"}})
		if !reflect.DeepEqual(stale, started) {
			t.Fatal("stale Audio success mutated state")
		}
		openedFile := state.Messages[9][0].Media.File
		openedFile.Downloaded = true
		openedFile.LocalPath = "/tmp/song.mp3"
		succeeded, _ := Reduce(started, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Audio", File: openedFile})
		if succeeded.Toast == nil || succeeded.Toast.Message != "Audio opened" || succeeded.Messages[9][0].Media.File != openedFile || len(succeeded.AudioOpenPending) != 0 {
			t.Fatalf("Audio success = %#v", succeeded)
		}

		private := "private audio failure"
		failed, _ := Reduce(started, MessageMediaOpenFailed{RequestID: 41, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Op: "open message media", Message: "Could not open audio", Cause: errors.New(private)}})
		if failed.Toast == nil || failed.Toast.Message != "Could not open audio" || strings.Contains(failed.Toast.Error(), private) || len(failed.AudioOpenPending) != 0 {
			t.Fatalf("Audio failure = %#v", failed.Toast)
		}
		retryMenu, _ := Reduce(failed, ActionReceived{Action: OpenMessageActionMenu})
		retried, retryCommands := Reduce(retryMenu, ActionReceived{Action: ViewMessageMedia})
		if len(retryCommands) != 1 || retried.Toast == nil || retried.Toast.Message != "Opening audio…" {
			t.Fatalf("Audio retry = %#v %#v", retried, retryCommands)
		}
	}
}

func TestAudioExternalOpenHandlerDownloadsAndLaunchesSystemPlayer(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened")
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nprintf '%s' \"$1\" > "+marker+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	client := &handlerClient{avatarFile: telegram.LocalFile{Path: "/tmp/downloaded-song.mp3"}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 91, ChatID: 9, MessageID: 77, Title: "Audio", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	opened, ok := events[0].(MessageMediaOpened)
	if !ok || opened.Title != "Audio" || opened.File.LocalPath != "/tmp/downloaded-song.mp3" || !opened.File.Downloaded {
		t.Fatalf("opened = %#v", events[0])
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "/tmp/downloaded-song.mp3" {
		t.Fatalf("opener marker = %q err=%v", contents, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nprintf 'private launcher output' >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	client.avatarFile.Path = "/private/downloaded-song.mp3"
	launchFailed := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 92, ChatID: 9, MessageID: 77, Title: "Audio", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
	if len(launchFailed) != 1 {
		t.Fatalf("launch failure events = %#v", launchFailed)
	}
	openFailure, ok := launchFailed[0].(MessageMediaOpenFailed)
	if !ok || openFailure.Error.Message != "Could not open audio" || openFailure.Error.Cause != nil || strings.Contains(openFailure.Error.Error(), "private") {
		t.Fatalf("launch failure = %#v", launchFailed[0])
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledHandler := &Handler{client: client, workCtx: ctx}
	canceled := collectHandlerEvents(canceledHandler, OpenMessageMediaFile{RequestID: 93, ChatID: 9, MessageID: 77, Title: "Audio", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
	if len(canceled) != 1 {
		t.Fatalf("canceled events = %#v", canceled)
	}
	failed, ok := canceled[0].(MessageMediaOpenFailed)
	if !ok || failed.Error.Message != "Could not open audio" || failed.Error.Cause != nil {
		t.Fatalf("canceled failure = %#v", canceled[0])
	}
}

func audioSendState(path string) State {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusPhotoSend
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	state.NextLocalID = -1
	state.Drafts[9] = "composer caption"
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 44}
	state.PhotoSend = &PhotoSendState{ChatID: 9, Input: []rune(path), PreviousFocus: FocusConversation}
	return state
}

func audioOpenState(outgoing bool) State {
	state := InitialState()
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.NextRequestID = 40
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.Messages[9] = []domain.Message{{ID: 77, ChatID: 9, Kind: domain.MessageAudio, Outgoing: outgoing, FileName: "song.mp3", Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 701, UniqueID: "audio-main", CanDownload: true}, MIMEType: "audio/mpeg"}}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 77
	return state
}
