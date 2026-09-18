package telegram

import (
	"context"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeChatSettingsLoadAndMutations(t *testing.T) {
	chatID := domain.ChatID(7)
	fake := NewFake(FakeData{ChatSettings: map[domain.ChatID]ChatSettings{
		chatID: {Kind: domain.ChatSupergroup, Title: "old", Description: "about", SlowModeDelay: 30},
	}})
	if err := fake.SetChatTitle(context.Background(), chatID, "new"); err != nil {
		t.Fatal(err)
	}
	if err := fake.SetChatDescription(context.Background(), chatID, "updated"); err != nil {
		t.Fatal(err)
	}
	if err := fake.SetChatSlowModeDelay(context.Background(), chatID, 300); err != nil {
		t.Fatal(err)
	}
	got, err := fake.LoadChatSettings(context.Background(), chatID)
	if err != nil || got.Title != "new" || got.Description != "updated" || got.SlowModeDelay != 300 {
		t.Fatalf("settings=%#v err=%v", got, err)
	}
	if err := fake.SetChatSlowModeDelay(context.Background(), chatID, 1); err == nil {
		t.Fatal("invalid delay accepted")
	}
}
