package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeSearchPublicChatMatchesCaseInsensitiveWithOptionalAt(t *testing.T) {
	client := NewFake(FakeData{
		PublicChats: map[string]domain.Chat{
			"botfather": {ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather"},
		},
	})
	for _, query := range []string{"botfather", "BotFather", "@botfather", "  @BOTFATHER  "} {
		chat, err := client.SearchPublicChat(context.Background(), query)
		if err != nil {
			t.Fatalf("query %q error = %v", query, err)
		}
		if chat.ID != 99 || chat.Username != "BotFather" {
			t.Fatalf("query %q = %#v", query, chat)
		}
	}
	missing, err := client.SearchPublicChat(context.Background(), "@unknown")
	if err != nil {
		t.Fatalf("missing error = %v", err)
	}
	if missing.ID != 0 {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestFakeSearchPublicChatErrorAndCloneIsolation(t *testing.T) {
	configured := errors.New("private public lookup detail")
	client := NewFake(FakeData{SearchPublicChatErr: configured})
	if _, err := client.SearchPublicChat(context.Background(), "botfather"); !errors.Is(err, configured) {
		t.Fatalf("error = %v, want %v", err, configured)
	}

	original := FakeData{
		PublicChats: map[string]domain.Chat{
			"botfather": {ID: 99, Title: "BotFather"},
		},
	}
	client = NewFake(original)
	original.PublicChats["botfather"] = domain.Chat{ID: 1}
	original.PublicChats["other"] = domain.Chat{ID: 2}
	chat, err := client.SearchPublicChat(context.Background(), "@BotFather")
	if err != nil || chat.ID != 99 {
		t.Fatalf("isolated lookup = (%#v, %v)", chat, err)
	}
	// The late-added key must not leak into the fake.
	missing, err := client.SearchPublicChat(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	if missing.ID != 0 {
		t.Fatalf("leaked key = %#v", missing)
	}
}
