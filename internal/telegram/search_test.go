package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeSearchChatMessagesPaginationOrderAndContext(t *testing.T) {
	messages := []domain.Message{
		{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "needle old", SentAt: time.Unix(1, 0)},
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "other", SentAt: time.Unix(2, 0)},
		{ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "Needle middle", SentAt: time.Unix(3, 0)},
		{ID: 4, ChatID: 9, Kind: domain.MessageText, Text: "needle new", SentAt: time.Unix(4, 0)},
	}
	client := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{9: messages}})
	first, err := client.SearchChatMessages(context.Background(), 9, " NEEDLE ", MessageSearchCursor{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 2 || first.Messages[0].ID != 4 || first.Messages[1].ID != 3 || first.NextFromMessageID != 1 || first.Done || first.TotalCount != 3 {
		t.Fatalf("first page = %#v", first)
	}
	second, err := client.SearchChatMessages(context.Background(), 9, "needle", MessageSearchCursor{FromMessageID: first.NextFromMessageID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 1 || second.Messages[0].ID != 1 || !second.Done || second.NextFromMessageID != 0 {
		t.Fatalf("second page = %#v", second)
	}
	contextPage, err := client.LoadMessageContext(context.Background(), 9, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(contextPage.Messages) != 4 || contextPage.Messages[0].ID != 1 || contextPage.Messages[3].ID != 4 {
		t.Fatalf("context page = %#v", contextPage)
	}
}

func TestFakeSearchAndContextErrors(t *testing.T) {
	private := errors.New("private query and transport detail")
	client := NewFake(FakeData{SearchError: private, ContextError: private})
	if _, err := client.SearchChatMessages(context.Background(), 9, "secret", MessageSearchCursor{}); !errors.Is(err, private) {
		t.Fatalf("search error = %v", err)
	}
	if _, err := client.LoadMessageContext(context.Background(), 9, 1); !errors.Is(err, private) {
		t.Fatalf("context error = %v", err)
	}
}

func TestFakePinnedMessagesFilteringAndReverseOrder(t *testing.T) {
	messages := []domain.Message{
		{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "unpinned1", Pinned: false, SentAt: time.Unix(1, 0)},
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", Pinned: true, SentAt: time.Unix(2, 0)},
		{ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "unpinned2", Pinned: false, SentAt: time.Unix(3, 0)},
		{ID: 4, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", Pinned: true, SentAt: time.Unix(4, 0)},
	}
	client := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{9: messages}})
	results, err := client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Messages) != 2 {
		t.Fatalf("expected 2 pinned messages, got %d", len(results.Messages))
	}
	if results.Messages[0].ID != 4 || results.Messages[1].ID != 2 {
		t.Fatalf("expected [4, 2], got [%d, %d]", results.Messages[0].ID, results.Messages[1].ID)
	}
	if results.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", results.TotalCount)
	}
}

func TestFakePinnedMessagesPagination(t *testing.T) {
	messages := []domain.Message{
		{ID: 4, ChatID: 9, Kind: domain.MessageText, Text: "pinned4", Pinned: true, SentAt: time.Unix(4, 0)},
		{ID: 3, ChatID: 9, Kind: domain.MessageText, Text: "pinned3", Pinned: true, SentAt: time.Unix(3, 0)},
		{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "pinned2", Pinned: true, SentAt: time.Unix(2, 0)},
		{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "pinned1", Pinned: true, SentAt: time.Unix(1, 0)},
	}
	client := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{9: messages}})
	first, err := client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 2 || first.Messages[0].ID != 4 || first.Messages[1].ID != 3 {
		t.Fatalf("first page = %#v", first)
	}
	if first.Done || first.NextFromMessageID != 2 {
		t.Fatalf("expected Done=false, NextFromMessageID=2, got %#v", first)
	}
	second, err := client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{FromMessageID: first.NextFromMessageID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 2 || second.Messages[0].ID != 2 || second.Messages[1].ID != 1 {
		t.Fatalf("second page = %#v", second)
	}
	if !second.Done || second.NextFromMessageID != 0 {
		t.Fatalf("expected Done=true, NextFromMessageID=0, got %#v", second)
	}
}

func TestFakePinnedMessagesDefaultAndClampedLimit(t *testing.T) {
	messages := []domain.Message{}
	for i := 65; i >= 6; i-- {
		messages = append(messages, domain.Message{ID: domain.MessageID(i), ChatID: 9, Kind: domain.MessageText, Text: "pinned", Pinned: true, SentAt: time.Unix(int64(i), 0)})
	}
	client := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{9: messages}})
	// Default limit (0) should become 50
	results, err := client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Messages) != 50 {
		t.Fatalf("got %d messages with limit 0, want 50", len(results.Messages))
	}
	// Invalid limit (>100) should be clamped to default 50.
	results, err = client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Messages) != 50 {
		t.Fatalf("got %d messages with limit 200, want 50", len(results.Messages))
	}
}

func TestFakePinnedMessagesError(t *testing.T) {
	private := errors.New("pinned search failure")
	client := NewFake(FakeData{
		Messages:          map[domain.ChatID][]domain.Message{9: {{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "hello", Pinned: true}}},
		PinnedSearchError: private,
	})
	_, err := client.SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{})
	if !errors.Is(err, private) {
		t.Fatalf("pinned search error = %v, want %v", err, private)
	}
	// Normal search should still work
	results, err := client.SearchChatMessages(context.Background(), 9, "hello", MessageSearchCursor{})
	if err != nil {
		t.Fatalf("normal search error = %v", err)
	}
	if len(results.Messages) != 1 {
		t.Fatalf("expected 1 message from normal search, got %d", len(results.Messages))
	}
}
