package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAttachmentPendingIsChatScopedAndFailureChecksCurrentFile(t *testing.T) {
	openChat9 := func() (State, uint64) {
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
		updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
		commands := updateState(&state, ActionReceived{Action: ViewMessageMedia})
		if len(commands) != 1 {
			t.Fatalf("chat 9 commands = %#v", commands)
		}
		return state, commands[0].(OpenMessageMediaFile).RequestID
	}

	started9, request9 := openChat9()

	// Once chat 9's file is replaced, the in-flight open failure is stale and
	// must leave the state untouched.
	changedFile, _ := openChat9()
	changedFile.Messages[9][0].Media.File = domain.MediaFileRef{ID: 999, CanDownload: true}
	replacedFile, _ := openChat9()
	replacedFile.Messages[9][0].Media.File = domain.MediaFileRef{ID: 999, CanDownload: true}
	commands := updateState(&changedFile, MessageMediaOpenFailed{
		RequestID: request9, ChatID: 9, MessageID: 77,
		Error: domain.AppError{Kind: domain.ErrorInternal, Message: "private raw failure"},
	})
	if len(commands) != 0 || !reflect.DeepEqual(changedFile, replacedFile) {
		t.Fatalf("failure for replaced file changed state: commands=%#v", commands)
	}

	started9.SelectedChat = 1
	started9.SelectedMessageChat, started9.SelectedMessage = 8, 77
	updateState(&started9, ActionReceived{Action: OpenMessageActionMenu})
	if started9.MessageMenu == nil || started9.MessageMenu.MediaFile.ID != 801 {
		t.Fatalf("chat 8 open action hidden by chat 9 pending: %#v", started9.MessageMenu)
	}
	commands8 := updateState(&started9, ActionReceived{Action: ViewMessageMedia})
	if len(commands8) != 1 || len(started9.AttachmentOpenPending) != 2 {
		t.Fatalf("chat-scoped pending = %#v, commands=%#v", started9.AttachmentOpenPending, commands8)
	}
}
