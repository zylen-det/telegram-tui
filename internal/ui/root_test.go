package ui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/testutil"
)

func TestAuthorizationPromptDrawsVisibleInputFieldAndAPIIDHelp(t *testing.T) {
	model := rootFixture(100, 24)
	model.Focus = app.FocusAuth
	model.Prompt = &app.PromptState{Prompt: auth.Prompt{Kind: auth.PromptAPIID, Label: "Telegram API ID"}}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	text := testutil.BufferText(buffer)
	for _, want := range []string{"Telegram API ID", "my.telegram.org/apps", "Type here…", "Enter: continue"} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, text)
		}
	}
}

func TestRootWideChatGolden(t *testing.T) {
	model := rootFixture(140, 30)
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)

	assertGolden(t, "wide-chat.txt", testutil.BufferText(buffer))
}

func TestRootApprovedLayoutGoldens(t *testing.T) {
	tests := []struct {
		name   string
		model  ViewModel
		golden string
	}{
		{name: "wide chat", model: rootFixture(140, 30), golden: "wide-chat.txt"},
		{name: "wide details", model: detailsFixture(), golden: "wide-details.txt"},
		{name: "narrow chat", model: rootFixture(70, 22), golden: "narrow-chat.txt"},
		{name: "too small", model: rootFixture(59, 17), golden: "too-small.txt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertModelGolden(t, test.model, test.golden)
		})
	}
}

func TestRootFocusedStateGoldens(t *testing.T) {
	tests := []struct {
		name   string
		model  ViewModel
		golden string
	}{
		{name: "chat loading", model: chatsStateFixture("loading"), golden: "chat-loading.txt"},
		{name: "chat loaded empty", model: chatsStateFixture("empty"), golden: "chat-empty.txt"},
		{name: "chat safe error", model: chatsStateFixture("error"), golden: "chat-error.txt"},
		{name: "cell aware text", model: textFixture(), golden: "text-cases.txt"},
		{name: "multiline draft", model: draftFixture(), golden: "multiline-draft.txt"},
		{name: "group boundaries", model: groupBoundaryFixture(), golden: "group-boundaries.txt"},
		{name: "missing avatars", model: missingAvatarFixture(), golden: "missing-avatars.txt"},
		{name: "modal loading", model: modalFixture(true, nil), golden: "modal-loading.txt"},
		{name: "modal error", model: modalFixture(false, &domain.AppError{Kind: domain.ErrorMedia, Message: "Image unavailable"}), golden: "modal-error.txt"},
		{name: "read only channel", model: readOnlyFixture(), golden: "read-only-channel.txt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _ := assertModelGolden(t, test.model, test.golden)
			assertHitsBounded(t, root.Hits(), image.Rect(0, 0, test.model.Width, test.model.Height))
		})
	}
}

func assertModelGolden(t *testing.T, model ViewModel, golden string) (*Root, *gotui.Buffer) {
	t.Helper()
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	assertGolden(t, golden, testutil.BufferText(buffer))
	return root, buffer
}

func assertHitsBounded(t *testing.T, hits HitMap, bounds image.Rectangle) {
	t.Helper()
	for index, hit := range hits {
		if hit.Rect.Empty() || !hit.Rect.In(bounds) {
			t.Errorf("hit %d rect = %v, want nonempty and inside %v", index, hit.Rect, bounds)
		}
	}
}

func rootFixture(width, height int) ViewModel {
	chatAvatar := fixtureAvatar(6, 3, color.NRGBA{R: 35, G: 115, B: 160, A: 255}, color.NRGBA{R: 18, G: 62, B: 82, A: 255})
	groupAvatar := fixtureAvatar(4, 2, color.NRGBA{R: 135, G: 80, B: 145, A: 255}, color.NRGBA{R: 68, G: 38, B: 75, A: 255})
	active := domain.Chat{
		ID: 9, Kind: domain.ChatSupergroup, Title: "Weekend 開發群", Username: "weekend_dev",
		Avatar: domain.AvatarRef{UniqueID: "weekend"}, LastMessage: "明天見 👋", LastMessageAt: localFixtureUnix(14, 35), UnreadCount: 3, CanSend: true,
	}
	model := ViewModel{
		Width: width, Height: height, Focus: app.FocusConversation, Connection: domain.ConnectionOnline,
		Chats: []ChatRow{
			{Chat: domain.Chat{ID: 7, Kind: domain.ChatPrivate, Title: "Mina Chen", Avatar: domain.AvatarRef{UniqueID: "mina"}, LastMessage: "Ship it", LastMessageAt: localFixtureUnix(13, 2), Muted: true, CanSend: true}, AvatarKey: "mina:chat-list", Avatar: chatAvatar},
			{Chat: active, Selected: true, AvatarKey: "weekend:chat-list", Avatar: chatAvatar},
		},
		ActiveChat: active,
		Groups: []RenderedMessageGroup{
			{MessageGroup: MessageGroup{Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 31}, SenderName: "Iris 林", SenderAvatar: domain.AvatarRef{UniqueID: "iris"}, ShowAvatar: true, Messages: []domain.Message{
				{ID: 1, ChatID: 9, SenderName: "Iris 林", SentAt: fixtureTime(14, 30), Kind: domain.MessageText, Text: "早安，今天來處理 CJK。"},
				{ID: 2, ChatID: 9, SenderName: "Iris 林", SentAt: fixtureTime(14, 31), Kind: domain.MessageText, Text: "第二則連發訊息只有一個頭像。"},
				{ID: 3, ChatID: 9, SenderName: "Iris 林", SentAt: fixtureTime(14, 32), Kind: domain.MessageText, Text: "Emoji 👩‍💻 looks good."},
			}}, AvatarKey: "iris:message-group", Avatar: groupAvatar},
			{MessageGroup: MessageGroup{Messages: []domain.Message{{ID: 4, ChatID: 9, SentAt: fixtureTime(14, 34), Kind: domain.MessageText, Text: "收到，我會更新。", Outgoing: true}}}},
		},
		Draft: "準備送出的 draft",
	}
	model.Layout = ComputeLayout(width, height, false, model.Focus)
	return model
}

func detailsFixture() ViewModel {
	model := rootFixture(140, 30)
	model.DetailsOpen = true
	model.Focus = app.FocusDetails
	model.Layout = ComputeLayout(model.Width, model.Height, true, model.Focus)
	return model
}

func chatsStateFixture(state string) ViewModel {
	model := rootFixture(100, 24)
	model.Chats = nil
	model.ActiveChat = domain.Chat{}
	model.Groups = nil
	model.Draft = ""
	switch state {
	case "loading":
		model.ChatsLoading = true
	case "empty":
		model.ChatsLoaded = true
	case "error":
		model.ChatsLoaded = true
		model.ChatsError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "Could not load chats"}
	}
	return model
}

func textFixture() ViewModel {
	model := rootFixture(100, 24)
	model.Groups = []RenderedMessageGroup{
		{MessageGroup: MessageGroup{SenderName: "文字測試", ShowAvatar: true, Messages: []domain.Message{
			{ID: 10, SentAt: fixtureTime(9, 1), Kind: domain.MessageText, Text: "中文內容與 English"},
			{ID: 11, SentAt: fixtureTime(9, 2), Kind: domain.MessageText, Text: "Emoji 👩‍💻 and e\u0301 combining"},
			{ID: 12, SentAt: fixtureTime(9, 3), Kind: domain.MessageText, Text: "averyveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryverylongtoken"},
		}}, AvatarKey: "text:message-group", Avatar: fixtureAvatar(4, 2, color.NRGBA{R: 20, G: 170, B: 120, A: 255}, color.NRGBA{R: 8, G: 70, B: 52, A: 255})},
	}
	return model
}

func draftFixture() ViewModel {
	model := rootFixture(100, 24)
	model.Focus = app.FocusComposer
	model.Draft = "first line\n第二行 👋"
	model.Layout = ComputeLayout(model.Width, model.Height, false, model.Focus)
	return model
}

func groupBoundaryFixture() ViewModel {
	model := rootFixture(100, 24)
	avatar := fixtureAvatar(4, 2, color.NRGBA{R: 190, G: 80, B: 60, A: 255}, color.NRGBA{R: 85, G: 30, B: 22, A: 255})
	model.Groups = []RenderedMessageGroup{
		{MessageGroup: MessageGroup{SenderName: "Iris", ShowAvatar: true, Messages: []domain.Message{
			{ID: 20, SentAt: fixtureTime(10, 0), Kind: domain.MessageText, Text: "first run"},
			{ID: 21, SentAt: fixtureTime(10, 1), Kind: domain.MessageText, Text: "same run"},
		}}, AvatarKey: "iris:message-group", Avatar: avatar},
		{MessageGroup: MessageGroup{SenderName: "Iris", ShowAvatar: true, Messages: []domain.Message{
			{ID: 22, SentAt: fixtureTime(10, 9), Kind: domain.MessageText, Text: "new boundary"},
		}}, AvatarKey: "iris:message-group", Avatar: avatar},
	}
	return model
}

func missingAvatarFixture() ViewModel {
	model := rootFixture(100, 24)
	model.Chats[0].Avatar = pixel.Avatar{}
	model.Chats[1].Avatar = pixel.Avatar{}
	model.Groups[0].Avatar = pixel.Avatar{}
	return model
}

func modalFixture(loading bool, modalError *domain.AppError) ViewModel {
	model := detailsFixture()
	model.Width, model.Height = 100, 24
	model.Focus = app.FocusModal
	model.Layout = ComputeLayout(model.Width, model.Height, true, model.Focus)
	model.Modal = &app.ModalState{Title: "Weekend 開發群", Loading: loading, Error: modalError, PreviousFocus: app.FocusDetails}
	return model
}

func readOnlyFixture() ViewModel {
	model := rootFixture(100, 24)
	model.ActiveChat.Kind = domain.ChatChannel
	model.ActiveChat.Title = "Release channel"
	model.ActiveChat.CanSend = false
	model.Chats[1].Chat = model.ActiveChat
	model.Groups = []RenderedMessageGroup{{MessageGroup: MessageGroup{Messages: []domain.Message{{ID: 30, SentAt: fixtureTime(16, 0), Kind: domain.MessageText, Text: "Read-only announcement"}}}}}
	model.Draft = "must not be rendered"
	return model
}

func fixtureAvatar(width, height int, foreground, background color.NRGBA) pixel.Avatar {
	avatar := pixel.Avatar{Width: width, Height: height, Cells: make([]pixel.Cell, width*height)}
	for index := range avatar.Cells {
		avatar.Cells[index] = pixel.Cell{Rune: '▀', Foreground: foreground, Background: background}
	}
	return avatar
}

func fixtureTime(hour, minute int) time.Time {
	return time.Date(2026, time.July, 20, hour, minute, 0, 0, time.Local)
}

func localFixtureUnix(hour, minute int) int64 {
	return fixtureTime(hour, minute).Unix()
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if got != string(want) {
		t.Fatalf("render does not match %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
