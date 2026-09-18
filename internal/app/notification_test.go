package app

import (
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestDesktopNotificationForEligibleBlurredMessage(t *testing.T) {
	state := InitialState()
	state.TerminalFocused = false
	state.Chats = []domain.Chat{{ID: 1, Title: "  Project\nchat  "}}
	message := domain.Message{ID: 100, ChatID: 1, Kind: domain.MessageText, Text: " Hello\nthere ", SenderName: " Alice  Example "}

	_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	if len(commands) != 1 {
		t.Fatalf("commands = %#v, want one notification", commands)
	}
	notification, ok := commands[0].(ShowDesktopNotification)
	if !ok {
		t.Fatalf("command = %T, want ShowDesktopNotification", commands[0])
	}
	if notification.Title != "Project chat" || notification.Body != "Alice Example: Hello there" {
		t.Fatalf("notification = %#v", notification)
	}
}

func TestDesktopNotificationMediaCaptionAndEmptySender(t *testing.T) {
	state := InitialState()
	state.TerminalFocused = false
	state.Chats = []domain.Chat{{ID: 2, Title: "Media"}}
	message := domain.Message{ID: 200, ChatID: 2, Kind: domain.MessagePhoto, Text: " nice\n sunset "}

	_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	if len(commands) != 1 {
		t.Fatalf("commands = %#v, want one notification", commands)
	}
	notification := commands[0].(ShowDesktopNotification)
	if notification.Body != "[Photo] nice sunset" {
		t.Fatalf("body = %q", notification.Body)
	}
}

func TestDesktopNotificationSuppression(t *testing.T) {
	baseMessage := domain.Message{ID: 10, ChatID: 1, Kind: domain.MessageText, Text: "hello", SenderName: "Alice"}
	tests := []struct {
		name    string
		state   State
		message domain.Message
	}{
		{name: "focused", state: func() State { s := notificationTestState(); s.TerminalFocused = true; return s }(), message: baseMessage},
		{name: "muted", state: func() State { s := notificationTestState(); s.Chats[0].Muted = true; return s }(), message: baseMessage},
		{name: "outgoing", state: notificationTestState(), message: func() domain.Message { m := baseMessage; m.Outgoing = true; return m }()},
		{name: "service flag", state: notificationTestState(), message: func() domain.Message { m := baseMessage; m.Service = true; return m }()},
		{name: "service kind", state: notificationTestState(), message: func() domain.Message { m := baseMessage; m.Kind = domain.MessageService; return m }()},
		{name: "unknown chat", state: notificationTestState(), message: func() domain.Message { m := baseMessage; m.ChatID = 99; return m }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, commands := Reduce(test.state, TelegramEvent{Value: telegram.MessageUpserted{Message: test.message}})
			for _, command := range commands {
				if _, ok := command.(ShowDesktopNotification); ok {
					t.Fatalf("suppressed message emitted notification: %#v", commands)
				}
			}
		})
	}
}

func TestDesktopNotificationSuppressesReupsert(t *testing.T) {
	state := notificationTestState()
	message := domain.Message{ID: 10, ChatID: 1, Kind: domain.MessageText, Text: "first", SenderName: "Alice"}
	state, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	if len(commands) != 1 {
		t.Fatalf("first upsert commands = %#v", commands)
	}
	message.Text = "updated"
	_, commands = Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	for _, command := range commands {
		if _, ok := command.(ShowDesktopNotification); ok {
			t.Fatalf("re-upsert emitted notification: %#v", commands)
		}
	}
}

func TestDesktopNotificationBodyIsRuneBounded(t *testing.T) {
	state := notificationTestState()
	message := domain.Message{
		ID: 10, ChatID: 1, Kind: domain.MessageText,
		SenderName: strings.Repeat("界", 150),
		Text:       strings.Repeat("🙂", 300),
	}
	_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	notification := commands[0].(ShowDesktopNotification)
	body := []rune(notification.Body)
	if len(body) != notificationBodyLimit || body[len(body)-1] != '…' {
		t.Fatalf("bounded body = runes:%d suffix:%q", len(body), string(body[len(body)-1:]))
	}
}

func TestDesktopNotificationAppendsAfterMediaWork(t *testing.T) {
	state := notificationTestState()
	message := domain.Message{
		ID: 10, ChatID: 1, Kind: domain.MessagePhoto, SenderName: "Alice",
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 7, CanDownload: true}},
	}
	_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: message}})
	if len(commands) != 2 {
		t.Fatalf("commands = %#v, want media work then notification", commands)
	}
	if _, ok := commands[0].(DownloadThumbnail); !ok {
		t.Fatalf("first command = %T, want DownloadThumbnail", commands[0])
	}
	if _, ok := commands[1].(ShowDesktopNotification); !ok {
		t.Fatalf("last command = %T, want ShowDesktopNotification", commands[1])
	}
}

func TestTerminalFocusChangedPointerIsNormalized(t *testing.T) {
	state, _ := Reduce(InitialState(), &TerminalFocusChanged{Focused: false})
	if state.TerminalFocused {
		t.Fatal("pointer focus event did not update state")
	}
}

func notificationTestState() State {
	state := InitialState()
	state.TerminalFocused = false
	state.Chats = []domain.Chat{{ID: 1, Title: "Chat"}}
	return state
}
