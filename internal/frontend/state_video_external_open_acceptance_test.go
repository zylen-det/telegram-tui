package frontend

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestVideoExternalOpenAcceptance_EligibleReceivedVideoActionAndNoDoubleDispatch(t *testing.T) {
	file := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true}
	state := videoExternalOpenAcceptanceState(file, false)

	propertyCommands := updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	menuState := state
	if len(propertyCommands) != 1 {
		t.Fatalf("message-property commands = %#v, want one", propertyCommands)
	}
	if menuState.MessageMenu == nil || !reflect.DeepEqual(menuState.MessageMenu.MediaFile, file) {
		t.Fatalf("eligible received Video menu = %#v, want exact main file", menuState.MessageMenu)
	}

	commands := updateState(&menuState, ActionReceived{Action: ViewMessageMedia})
	started := menuState
	wantCommand := []Effect{OpenMessageMediaFile{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: file}}
	if !reflect.DeepEqual(commands, wantCommand) {
		t.Fatalf("Video open commands = %#v, want %#v", commands, wantCommand)
	}
	if started.MessageMenu != nil || started.Modal != nil || started.Focus != FocusConversation {
		t.Fatalf("Video open should close the action menu without a terminal modal: focus=%v menu=%#v modal=%#v", started.Focus, started.MessageMenu, started.Modal)
	}
	if started.Toast == nil || started.Toast.Message != "Opening video…" {
		t.Fatalf("starting toast = %#v, want Opening video…", started.Toast)
	}

	duplicateCommands := updateState(&started, ActionReceived{Action: ViewMessageMedia})
	if len(duplicateCommands) != 0 || started.VideoOpenPending[77] != 41 || started.Toast == nil || started.Toast.Message != "Opening video…" {
		t.Fatalf("duplicate activation changed pending open: state=%#v commands=%#v", started, duplicateCommands)
	}

	updateState(&started, ActionReceived{Action: OpenMessageActionMenu})
	pendingMenu := started
	if pendingMenu.MessageMenu == nil {
		t.Fatal("message action menu did not reopen while Video launch was pending")
	}
	if pendingMenu.MessageMenu.MediaFile != (domain.MediaFileRef{}) {
		t.Fatalf("pending Video exposed a second open action: %#v", pendingMenu.MessageMenu.MediaFile)
	}
	pendingCommands := updateState(&pendingMenu, ActionReceived{Action: ViewMessageMedia})
	if len(pendingCommands) != 0 || pendingMenu.VideoOpenPending[77] != 41 || pendingMenu.MessageMenu == nil || pendingMenu.MessageMenu.MediaFile != (domain.MediaFileRef{}) {
		t.Fatalf("pending Video double-dispatched: state=%#v commands=%#v", pendingMenu, pendingCommands)
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
			updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
			opened := state
			visible := opened.MessageMenu != nil && opened.MessageMenu.MediaFile != (domain.MediaFileRef{})
			if visible != test.want {
				t.Fatalf("open action visible = %t, want %t; menu=%#v", visible, test.want, opened.MessageMenu)
			}
		})
	}

	photo := videoExternalOpenAcceptanceState(remote, false)
	photo.Messages[9][0].Kind = domain.MessagePhoto
	updateState(&photo, ActionReceived{Action: OpenMessageActionMenu})
	openedPhoto := photo
	if openedPhoto.MessageMenu == nil || !reflect.DeepEqual(openedPhoto.MessageMenu.MediaFile, remote) {
		t.Fatal("existing received-Photo View image eligibility regressed")
	}
}

func TestVideoExternalOpenAcceptance_CorrelationStaleResultsRetryAndToasts(t *testing.T) {
	file := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true}
	menuState := videoExternalOpenAcceptanceState(file, false)
	updateState(&menuState, ActionReceived{Action: OpenMessageActionMenu})
	updateState(&menuState, ActionReceived{Action: ViewMessageMedia})
	started := menuState

	for _, staleEvent := range []Event{
		MessageMediaOpened{RequestID: 40, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-request.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 8, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-chat.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 78, Title: "Video", File: domain.MediaFileRef{ID: 701, Downloaded: true, LocalPath: "/tmp/wrong-message.mp4"}},
		MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: domain.MediaFileRef{ID: 999, Downloaded: true, LocalPath: "/tmp/wrong-file.mp4"}},
		MessageMediaOpenFailed{RequestID: 40, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Message: "Could not open video"}},
	} {
		commands := updateState(&started, staleEvent)
		if len(commands) != 0 || started.VideoOpenPending[77] != 41 || started.Messages[9][0].Media.File != file || started.Toast == nil || started.Toast.Message != "Opening video…" {
			t.Fatalf("stale %T changed pending open: state=%#v commands=%#v", staleEvent, started, commands)
		}
	}

	openedFile := domain.MediaFileRef{ID: 701, UniqueID: "video-main", CanDownload: true, Downloaded: true, LocalPath: "/tmp/video.mp4"}
	commands := updateState(&started, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: openedFile})
	succeeded := started
	if len(commands) != 0 || !reflect.DeepEqual(succeeded.Messages[9][0].Media.File, openedFile) {
		t.Fatalf("matched success = file:%#v commands:%#v", succeeded.Messages[9][0].Media.File, commands)
	}
	if succeeded.Toast == nil || succeeded.Toast.Message != "Video opened" || succeeded.Modal != nil {
		t.Fatalf("success presentation = toast:%#v modal:%#v", succeeded.Toast, succeeded.Modal)
	}
	duplicateCommands := updateState(&succeeded, MessageMediaOpened{RequestID: 41, ChatID: 9, MessageID: 77, Title: "Video", File: openedFile})
	if len(duplicateCommands) != 0 || len(succeeded.VideoOpenPending) != 0 || succeeded.Messages[9][0].Media.File != openedFile || succeeded.Toast == nil || succeeded.Toast.Message != "Video opened" {
		t.Fatalf("duplicate success changed completed Video state: %#v effects=%#v", succeeded, duplicateCommands)
	}

	menuState2 := videoExternalOpenAcceptanceState(file, false)
	updateState(&menuState2, ActionReceived{Action: OpenMessageActionMenu})
	updateState(&menuState2, ActionReceived{Action: ViewMessageMedia})
	started2 := menuState2
	private := "private-platform-cause"
	commands = updateState(&started2, MessageMediaOpenFailed{RequestID: 41, ChatID: 9, MessageID: 77, Error: domain.AppError{Kind: domain.ErrorMedia, Op: "open message media", Message: "Could not open video", Cause: errors.New(private)}})
	failed := started2
	if len(commands) != 0 || failed.Toast == nil || failed.Toast.Message != "Could not open video" || strings.Contains(failed.Toast.Error(), private) {
		t.Fatalf("matched failure = toast:%#v commands:%#v", failed.Toast, commands)
	}
	updateState(&failed, ActionReceived{Action: OpenMessageActionMenu})
	retryMenu := failed
	if retryMenu.MessageMenu == nil || !reflect.DeepEqual(retryMenu.MessageMenu.MediaFile, file) {
		t.Fatalf("failure did not restore manual retry action: %#v", retryMenu.MessageMenu)
	}
	retryCommands := updateState(&retryMenu, ActionReceived{Action: ViewMessageMedia})
	retried := retryMenu
	if len(retryCommands) != 1 || retried.Toast == nil || retried.Toast.Message != "Opening video…" {
		t.Fatalf("manual retry = state:%#v commands:%#v", retried, retryCommands)
	}
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
