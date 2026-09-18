package telegram

import (
	"context"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeApplyChatActionRecordsAndMutatesState(t *testing.T) {
	fake := NewFake(FakeData{
		Chats:    []domain.Chat{{ID: 9, UnreadCount: 3, IsMember: true}},
		Messages: map[domain.ChatID][]domain.Message{9: {{ID: 1, ChatID: 9}}},
	})
	for _, action := range []ChatAction{ChatActionMarkRead, ChatActionMute, ChatActionPin, ChatActionArchive, ChatActionLeaveChat} {
		if err := fake.ApplyChatAction(context.Background(), ChatActionRequest{ChatID: 9, Action: action}); err != nil {
			t.Fatalf("action %d: %v", action, err)
		}
	}
	want := []ChatActionRequest{
		{ChatID: 9, Action: ChatActionMarkRead}, {ChatID: 9, Action: ChatActionMute},
		{ChatID: 9, Action: ChatActionPin}, {ChatID: 9, Action: ChatActionArchive},
		{ChatID: 9, Action: ChatActionLeaveChat},
	}
	if got := fake.ChatActionCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
	page, err := fake.LoadChats(context.Background(), ChatCursor{})
	if err != nil || len(page.Chats) != 1 {
		t.Fatalf("load = %#v, %v", page, err)
	}
	chat := page.Chats[0]
	if chat.UnreadCount != 0 || !chat.Muted || !chat.IsPinned || !chat.IsArchived || chat.IsMember {
		t.Fatalf("mutated chat = %#v", chat)
	}
}
