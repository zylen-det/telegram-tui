package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func forumFixture() FakeData {
	return FakeData{
		Chats: []domain.Chat{{ID: 7, Title: "forum", IsForum: true, CanSend: true}},
		Messages: map[domain.ChatID][]domain.Message{
			7: {
				{ID: 1, ChatID: 7, Kind: domain.MessageText, Text: "general"},
				{ID: 2, ChatID: 7, TopicID: 5, Kind: domain.MessageText, Text: "topic five"},
				{ID: 3, ChatID: 7, TopicID: 5, Kind: domain.MessageText, Text: "pinned five", Pinned: true},
				{ID: 4, ChatID: 7, TopicID: 7, Kind: domain.MessageText, Text: "topic seven"},
			},
		},
		Topics: map[domain.ChatID][]domain.ForumTopic{
			7: {
				{ID: 4, ChatID: 7, Name: "fourth"},
				{ID: 5, ChatID: 7, Name: "fifth", UnreadCount: 2},
				{ID: 7, ChatID: 7, Name: "seventh"},
			},
		},
	}
}

func TestFakeLoadTopicsPagesByOffsetAndOwnsResults(t *testing.T) {
	fake := NewFake(forumFixture())
	page, err := fake.LoadTopics(context.Background(), 7, TopicCursor{Limit: 2})
	if err != nil {
		t.Fatalf("LoadTopics() error = %v", err)
	}
	if len(page.Topics) != 2 || page.Topics[0].ID != 4 || page.Topics[1].ID != 5 || page.TotalCount != 3 || page.Done {
		t.Fatalf("first topic page = %#v", page)
	}
	if page.Topics[1].UnreadCount != 2 || page.NextOffsetTopicID != 7 {
		t.Fatalf("topic state/offset = %#v/%d", page.Topics[1], page.NextOffsetTopicID)
	}
	page.Topics[0].Name = "mutated"
	rest, err := fake.LoadTopics(context.Background(), 7, TopicCursor{OffsetTopicID: 7, Limit: 2})
	if err != nil {
		t.Fatalf("continuation LoadTopics() error = %v", err)
	}
	if len(rest.Topics) != 1 || rest.Topics[0].ID != 7 || rest.Topics[0].Name != "seventh" || !rest.Done {
		t.Fatalf("continuation topic page = %#v", rest)
	}
	fresh, err := fake.LoadTopics(context.Background(), 7, TopicCursor{})
	if err != nil || fresh.Topics[0].Name != "fourth" {
		t.Fatalf("fixture topic regressed = %#v, error = %v", fresh.Topics, err)
	}

	failed := NewFake(FakeData{TopicsError: errors.New("opaque topics failure")})
	if _, err := failed.LoadTopics(context.Background(), 7, TopicCursor{}); !errors.Is(err, failed.data.TopicsError) {
		t.Fatalf("configured topics error = %v", err)
	}
}

func TestFakeLoadMessagesTopicCursorFiltersTopic(t *testing.T) {
	fake := NewFake(forumFixture())
	all, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil || len(all.Messages) != 4 {
		t.Fatalf("zero-topic page = %#v, error = %v", all, err)
	}
	page, err := fake.LoadMessages(context.Background(), 7, MessageCursor{TopicID: 5})
	if err != nil {
		t.Fatalf("topic LoadMessages() error = %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 2 || page.Messages[1].ID != 3 || !page.Done {
		t.Fatalf("topic 5 page = %#v", page)
	}
	none, err := fake.LoadMessages(context.Background(), 7, MessageCursor{TopicID: 9})
	if err != nil || len(none.Messages) != 0 || !none.Done {
		t.Fatalf("empty topic page = %#v, error = %v", none, err)
	}
}

func TestFakeSearchTopicCursorFiltersTopic(t *testing.T) {
	fake := NewFake(forumFixture())
	page, err := fake.SearchChatMessages(context.Background(), 7, "five", MessageSearchCursor{TopicID: 5})
	if err != nil {
		t.Fatalf("topic search error = %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 3 || page.Messages[1].ID != 2 || page.TotalCount != 2 {
		t.Fatalf("topic search page = %#v", page)
	}
	pinned, err := fake.SearchPinnedMessages(context.Background(), 7, MessageSearchCursor{TopicID: 5})
	if err != nil {
		t.Fatalf("topic pinned search error = %v", err)
	}
	if len(pinned.Messages) != 1 || pinned.Messages[0].ID != 3 {
		t.Fatalf("topic pinned page = %#v", pinned)
	}
	cross, err := fake.SearchPinnedMessages(context.Background(), 7, MessageSearchCursor{TopicID: 7})
	if err != nil || len(cross.Messages) != 0 {
		t.Fatalf("cross-topic pinned page = %#v, error = %v", cross, err)
	}
}

func TestFakeLoadTopicMessageContext(t *testing.T) {
	fake := NewFake(forumFixture())
	page, err := fake.LoadTopicMessageContext(context.Background(), 7, 5, 3)
	if err != nil {
		t.Fatalf("topic context error = %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 2 || page.Messages[1].ID != 3 {
		t.Fatalf("topic context page = %#v", page)
	}
	if _, err := fake.LoadTopicMessageContext(context.Background(), 7, 7, 3); err == nil {
		t.Fatal("cross-topic context did not report the missing target")
	}
	if _, err := fake.LoadTopicMessageContext(context.Background(), 7, 5, 99); err == nil {
		t.Fatal("unknown target context did not fail")
	}
}

func TestFakeSetDraftTopicDraft(t *testing.T) {
	fake := NewFake(forumFixture())
	if err := fake.SetDraft(context.Background(), SetDraftRequest{ChatID: 7, TopicID: 5, Text: "topic draft"}); err != nil {
		t.Fatalf("topic SetDraft() error = %v", err)
	}
	page, _ := fake.LoadTopics(context.Background(), 7, TopicCursor{})
	if page.Topics[1].Draft.Text != "topic draft" || page.Topics[0].Draft.Text != "" {
		t.Fatalf("topic drafts = %#v", page.Topics)
	}
	chats, _ := fake.LoadChats(context.Background(), ChatCursor{})
	if chats.Chats[0].Draft.Text != "" {
		t.Fatalf("topic draft leaked into the chat draft: %#v", chats.Chats[0].Draft)
	}

	if err := fake.SetDraft(context.Background(), SetDraftRequest{ChatID: 7, Text: "chat draft"}); err != nil {
		t.Fatalf("chat SetDraft() error = %v", err)
	}
	chats, _ = fake.LoadChats(context.Background(), ChatCursor{})
	if chats.Chats[0].Draft.Text != "chat draft" {
		t.Fatalf("chat draft = %#v", chats.Chats[0].Draft)
	}
}

func TestFakeSendPreservesTopicID(t *testing.T) {
	fake := NewFake(forumFixture())
	text, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, TopicID: 5, Text: "hello"})
	if err != nil || text.TopicID != 5 || text.ChatID != 7 {
		t.Fatalf("topic text send = %#v, error = %v", text, err)
	}
	photo, err := fake.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 7, TopicID: 5, LocalPath: "/tmp/photo.jpg", Caption: "photo"})
	if err != nil || photo.TopicID != 5 || photo.Kind != domain.MessagePhoto {
		t.Fatalf("topic photo send = %#v, error = %v", photo, err)
	}
	plain, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "general"})
	if err != nil || plain.TopicID != 0 {
		t.Fatalf("general text send = %#v, error = %v", plain, err)
	}
	page, _ := fake.LoadMessages(context.Background(), 7, MessageCursor{TopicID: 5})
	if len(page.Messages) != 4 || page.Messages[2].ID != text.ID || page.Messages[3].ID != photo.ID {
		t.Fatalf("sent topic messages = %#v", page.Messages)
	}
	if page.Messages[3].Media.File.LocalPath != "/tmp/photo.jpg" || !page.Messages[3].Media.File.Downloaded {
		t.Fatalf("sent photo media = %#v", page.Messages[3].Media)
	}
}
