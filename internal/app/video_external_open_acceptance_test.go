package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestVideoExternalOpenAcceptance_EligibleReceivedVideoActionAndNoDoubleDispatch(t *testing.T) {
	file := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true}
	state := videoExternalOpenAcceptanceState(file, false)

	menuState, propertyCommands := Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
	if len(propertyCommands) != 1 {
		t.Fatalf("message-property commands = %#v, want one", propertyCommands)
	}
	if menuState.MessageMenu == nil || !reflect.DeepEqual(menuState.MessageMenu.MediaFile, file) {
		t.Fatalf("eligible received Video menu = %#v, want exact main file", menuState.MessageMenu)
	}

	started, commands := Reduce(menuState, ActionReceived{Action: ViewMessageMedia})
	wantCommand := []Command{OpenMessageMediaFile{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: file}}
	if !reflect.DeepEqual(commands, wantCommand) {
		t.Fatalf("Video open commands = %#v, want %#v", commands, wantCommand)
	}
	if started.MessageMenu != nil || started.Modal != nil || started.Focus != FocusConversation {
		t.Fatalf("Video open should close the action menu without a terminal modal: focus=%v menu=%#v modal=%#v", started.Focus, started.MessageMenu, started.Modal)
	}
	if started.Toast == nil || started.Toast.Message != "Opening video…" {
		t.Fatalf("starting toast = %#v, want Opening video…", started.Toast)
	}

	duplicate, duplicateCommands := Reduce(started, ActionReceived{Action: ViewMessageMedia})
	if len(duplicateCommands) != 0 || !reflect.DeepEqual(duplicate, started) {
		t.Fatalf("duplicate activation changed state or dispatched: state=%#v commands=%#v", duplicate, duplicateCommands)
	}

	pendingMenu, _ := Reduce(started, ActionReceived{Action: OpenMessageActionMenu})
	if pendingMenu.MessageMenu == nil {
		t.Fatal("message action menu did not reopen while Video launch was pending")
	}
	if pendingMenu.MessageMenu.MediaFile != (domain.MediaFileRef{}) {
		t.Fatalf("pending Video exposed a second open action: %#v", pendingMenu.MessageMenu.MediaFile)
	}
	pendingAfterAction, pendingCommands := Reduce(pendingMenu, ActionReceived{Action: ViewMessageMedia})
	if len(pendingCommands) != 0 || !reflect.DeepEqual(pendingAfterAction, pendingMenu) {
		t.Fatalf("pending Video double-dispatched: state=%#v commands=%#v", pendingAfterAction, pendingCommands)
	}
}

func TestVideoExternalOpenAcceptance_VisibilityEligibility(t *testing.T) {
	remote := domain.MediaFileRef{ID: 701, CanDownload: true}
	local := domain.MediaFileRef{ID: 702, Downloaded: true, LocalPath: "/tmp/local.mp4"}
	for _, test := range []struct {
		name     string
		kind     domain.MessageKind
		outgoing bool
		file     domain.MediaFileRef
		want     bool
	}{
		{name: "received remote Video", kind: domain.MessageVideo, file: remote, want: true},
		{name: "received local Video", kind: domain.MessageVideo, file: local, want: true},
		{name: "unavailable Video", kind: domain.MessageVideo, file: domain.MediaFileRef{}},
		{name: "outgoing Video", kind: domain.MessageVideo, outgoing: true, file: remote, want: true},
		{name: "text", kind: domain.MessageText, file: remote},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := videoExternalOpenAcceptanceState(test.file, test.outgoing)
			state.Messages[9][0].Kind = test.kind
			opened, _ := Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
			visible := opened.MessageMenu != nil && opened.MessageMenu.MediaFile != (domain.MediaFileRef{})
			if visible != test.want {
				t.Fatalf("open action visible = %t, want %t; menu=%#v", visible, test.want, opened.MessageMenu)
			}
		})
	}

	photo := videoExternalOpenAcceptanceState(remote, false)
	photo.Messages[9][0].Kind = domain.MessagePhoto
	openedPhoto, _ := Reduce(photo, ActionReceived{Action: OpenMessageActionMenu})
	if openedPhoto.MessageMenu == nil || !reflect.DeepEqual(openedPhoto.MessageMenu.MediaFile, remote) {
		t.Fatal("existing received-Photo View image eligibility regressed")
	}
}

func TestVideoExternalOpenAcceptance_CorrelationStaleResultsRetryAndToasts(t *testing.T) {
	file := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true}
	menuState, _ := Reduce(videoExternalOpenAcceptanceState(file, false), ActionReceived{Action: OpenMessageActionMenu})
	started, _ := Reduce(menuState, ActionReceived{Action: ViewMessageMedia})

	for _, staleEvent := range []Event{
		MessageMediaOpened{RequestID: 40, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-request.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 8, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-chat.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 78, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-message.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 999, Downloaded: true, LocalPath: "/tmp/wrong-file.mp4"}},
		MessageMediaOpenFailed{RequestID: 40, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Message: "Could not open video"}},
	} {
		got, commands := Reduce(started, staleEvent)
		if len(commands) != 0 || !reflect.DeepEqual(got, started) {
			t.Fatalf("stale %T changed state or emitted commands: state=%#v commands=%#v", staleEvent, got, commands)
		}
	}

	openedFile := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true, Downloaded: true, LocalPath: "/tmp/video.mp4"}
	succeeded, commands := Reduce(started, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: openedFile})
	if len(commands) != 0 || !reflect.DeepEqual(succeeded.Messages[9][0].Media.File, openedFile) {
		t.Fatalf("matched success = file:%#v commands:%#v", succeeded.Messages[9][0].Media.File, commands)
	}
	if succeeded.Toast == nil || succeeded.Toast.Message != "Video opened" || succeeded.Modal != nil {
		t.Fatalf("success presentation = toast:%#v modal:%#v", succeeded.Toast, succeeded.Modal)
	}
	duplicateSuccess, _ := Reduce(succeeded, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: openedFile})
	if !reflect.DeepEqual(duplicateSuccess, succeeded) {
		t.Fatal("duplicate success relaunched or rewrote completed Video state")
	}

	menuState2, _ := Reduce(videoExternalOpenAcceptanceState(file, false), ActionReceived{Action: OpenMessageActionMenu})
	started2, _ := Reduce(menuState2, ActionReceived{Action: ViewMessageMedia})
	private := "private-platform-cause"
	failed, commands := Reduce(started2, MessageMediaOpenFailed{RequestID: 41, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Op: "open message media", Message: "Could not open video", Cause: errors.New(private)}})
	if len(commands) != 0 || failed.Toast == nil || failed.Toast.Message != "Could not open video" || strings.Contains(failed.Toast.Error(), private) {
		t.Fatalf("matched failure = toast:%#v commands:%#v", failed.Toast, commands)
	}
	retryMenu, _ := Reduce(failed, ActionReceived{Action: OpenMessageActionMenu})
	if retryMenu.MessageMenu == nil || !reflect.DeepEqual(retryMenu.MessageMenu.MediaFile, file) {
		t.Fatalf("failure did not restore manual retry action: %#v", retryMenu.MessageMenu)
	}
	retried, retryCommands := Reduce(retryMenu, ActionReceived{Action: ViewMessageMedia})
	if len(retryCommands) != 1 || retried.Toast == nil || retried.Toast.Message != "Opening video…" {
		t.Fatalf("manual retry = state:%#v commands:%#v", retried, retryCommands)
	}
}

func TestVideoExternalOpenAcceptance_HandlerDownloadsThenLaunchesOnceAndSanitizesFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "opened")
		writeVideoAcceptanceOpener(t, dir, "printf '%s' \"$1\" > "+strconv.Quote(marker)+"\nexit 0\n")
		t.Setenv("PATH", dir)
		client := &handlerClient{avatarFile: telegram.LocalFile{Path: "/tmp/downloaded-video.mp4"}}
		handler := newTestHandler(t, client, &handlerAvatarRenderer{})
		events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 91, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
		if len(events) != 1 {
			t.Fatalf("events = %#v, want one", events)
		}
		opened, ok := events[0].(MessageMediaOpened)
		if !ok || opened.RequestID != 91 || opened.ChatID != 9 || opened.MessageID != 77 || opened.Title != "Video" || opened.File.LocalPath != "/tmp/downloaded-video.mp4" || !opened.File.Downloaded {
			t.Fatalf("opened event = %#v", events[0])
		}
		markerBytes, err := os.ReadFile(marker)
		if err != nil || string(markerBytes) != "/tmp/downloaded-video.mp4" {
			t.Fatalf("external launch marker = %q, err=%v", string(markerBytes), err)
		}
		if calls := client.mediaCalls; len(calls) != 1 || !reflect.DeepEqual(calls[0], domain.MediaFileRef{ID: 701, CanDownload: true}) {
			t.Fatalf("DownloadMedia calls = %#v, want exact one", calls)
		}
	})

	t.Run("external launcher failure", func(t *testing.T) {
		dir := t.TempDir()
		private := "private-launcher-output"
		writeVideoAcceptanceOpener(t, dir, "printf '"+private+"' >&2\nexit 7\n")
		t.Setenv("PATH", dir)
		client := &handlerClient{avatarFile: telegram.LocalFile{Path: "/private/downloaded-video.mp4"}}
		handler := newTestHandler(t, client, &handlerAvatarRenderer{})
		events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 92, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
		if len(events) != 1 {
			t.Fatalf("events = %#v, want one", events)
		}
		failed, ok := events[0].(MessageMediaOpenFailed)
		if !ok || failed.RequestID != 92 || failed.ChatID != 9 || failed.MessageID != 77 || failed.Error.Kind != domain.ErrorMedia || failed.Error.Message != "Could not open video" {
			t.Fatalf("failure event = %#v", events[0])
		}
		for _, secret := range []string{private, "/private/downloaded-video.mp4"} {
			if strings.Contains(failed.Error.Error(), secret) || (failed.Error.Cause != nil && strings.Contains(failed.Error.Cause.Error(), secret)) {
				t.Fatalf("failure exposed %q: %#v", secret, failed.Error)
			}
		}
	})

	t.Run("canceled before download never launches", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "must-not-exist")
		writeVideoAcceptanceOpener(t, dir, "printf launched > "+strconv.Quote(marker)+"\nexit 0\n")
		t.Setenv("PATH", dir)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := &handlerClient{avatarFile: telegram.LocalFile{Path: "/tmp/video.mp4"}}
		handler := &Handler{client: client, workCtx: ctx}
		events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 93, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, CanDownload: true}})
		if len(events) != 1 {
			t.Fatalf("events = %#v, want one safe failure", events)
		}
		if _, ok := events[0].(MessageMediaOpenFailed); !ok {
			t.Fatalf("canceled event = %T, want MessageMediaOpenFailed", events[0])
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canceled command launched external opener: %v", err)
		}
	})
}

func videoExternalOpenAcceptanceState(file domain.MediaFileRef, outgoing bool) State {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.NextRequestID = 40
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.Messages[9] = []domain.Message{{
		ID: 77, ChatID: 9, Kind: domain.MessageVideo, Outgoing: outgoing,
		FileName: "clip.mp4", Media: domain.MessageMedia{File: file, MIMEType: "video/mp4"},
	}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 77
	return state
}

func writeVideoAcceptanceOpener(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatalf("write xdg-open fixture: %v", err)
	}
}
