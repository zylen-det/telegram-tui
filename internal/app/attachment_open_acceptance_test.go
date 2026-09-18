package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestAttachmentHandlerDownloadsAndLaunchesEachTitle(t *testing.T) {
	for _, title := range []string{"File", "Animation", "Voice note", "Video note"} {
		t.Run(title, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "opened")
			writeVideoAcceptanceOpener(t, dir, "printf '%s' \"$1\" > "+strconv.Quote(marker)+"\nexit 0\n")
			t.Setenv("PATH", dir)
			input := domain.MediaFileRef{ID: 701, CanDownload: true}
			client := &handlerClient{avatarFile: telegram.LocalFile{Path: "/tmp/attachment.bin"}}
			handler := newTestHandler(t, client, &handlerAvatarRenderer{})

			events := collectHandlerEvents(handler, OpenMessageMediaFile{
				RequestID: 91, ChatID: 9, MessageID: 77, Title: title, File: input,
			})
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one", events)
			}
			opened, ok := events[0].(MessageMediaOpened)
			if !ok || opened.Title != title || !opened.File.Downloaded || opened.File.LocalPath != "/tmp/attachment.bin" {
				t.Fatalf("opened event = %#v", events[0])
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "/tmp/attachment.bin" {
				t.Fatalf("external launch marker = %q, err=%v", data, err)
			}
			if len(client.mediaCalls) != 1 || !reflect.DeepEqual(client.mediaCalls[0], input) {
				t.Fatalf("DownloadMedia calls = %#v", client.mediaCalls)
			}
		})
	}
}

func TestAttachmentPendingIsChatScopedAndFailureChecksCurrentFile(t *testing.T) {
	state := InitialState()
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9}, {ID: 8}}
	state.Messages[9] = []domain.Message{{
		ID: 77, ChatID: 9, Kind: domain.MessageDocument,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 901, CanDownload: true}},
	}}
	state.Messages[8] = []domain.Message{{
		ID: 77, ChatID: 8, Kind: domain.MessageDocument,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 801, CanDownload: true}},
	}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 77

	menu9, _ := Reduce(state, ActionReceived{Action: OpenMessageActionMenu})
	started9, commands9 := Reduce(menu9, ActionReceived{Action: ViewMessageMedia})
	if len(commands9) != 1 {
		t.Fatalf("chat 9 commands = %#v", commands9)
	}
	request9 := commands9[0].(OpenMessageMediaFile).RequestID

	changedFile := cloneReducerState(started9)
	changedFile.Messages[9][0].Media.File = domain.MediaFileRef{ID: 999, CanDownload: true}
	staleFailure, commands := Reduce(changedFile, MessageMediaOpenFailed{
		RequestID: request9, ChatID: 9, MessageID: 77,
		Error: domain.AppError{Kind: domain.ErrorInternal, Message: "private raw failure"},
	})
	if len(commands) != 0 || !reflect.DeepEqual(staleFailure, changedFile) {
		t.Fatalf("failure for replaced file changed state: commands=%#v", commands)
	}

	started9.SelectedChat = 1
	started9.SelectedMessageChat, started9.SelectedMessage = 8, 77
	menu8, _ := Reduce(started9, ActionReceived{Action: OpenMessageActionMenu})
	if menu8.MessageMenu == nil || menu8.MessageMenu.MediaFile.ID != 801 {
		t.Fatalf("chat 8 open action hidden by chat 9 pending: %#v", menu8.MessageMenu)
	}
	startedBoth, commands8 := Reduce(menu8, ActionReceived{Action: ViewMessageMedia})
	if len(commands8) != 1 || len(startedBoth.AttachmentOpenPending) != 2 {
		t.Fatalf("chat-scoped pending = %#v, commands=%#v", startedBoth.AttachmentOpenPending, commands8)
	}
}
