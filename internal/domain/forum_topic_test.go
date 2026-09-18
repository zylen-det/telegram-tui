package domain

import (
	"testing"
)

func TestForumTopicStateIsExplicitAndComparable(t *testing.T) {
	general := ForumTopic{
		ID:          1,
		ChatID:      42,
		Name:        "General",
		IconColor:   0x8832f2,
		IsGeneral:   true,
		UnreadCount: 3,
		Order:       7,
		Draft:       Draft{Text: "draft"},
	}
	byID := map[TopicID]ForumTopic{general.ID: general}
	if got := byID[general.ID]; got != general {
		t.Fatalf("topic = %#v", got)
	}
	if general.IsClosed || general.IsHidden || general.IsPinned || general.UnreadMentionCount != 0 {
		t.Fatalf("topic = %#v", general)
	}
	general.IsClosed = true
	general.UnreadMentionCount = 2
	if !general.IsClosed || general.UnreadMentionCount != 2 {
		t.Fatalf("topic = %#v", general)
	}

	var zero ForumTopic
	if zero.ID != 0 || zero.ChatID != 0 || zero.LastMessage != "" || zero.LastMessageAt != 0 || zero.Order != 0 {
		t.Fatalf("zero topic = %#v", zero)
	}
}

func TestChatAndMessageCarryForumState(t *testing.T) {
	chat := Chat{ID: 9, Kind: ChatSupergroup, IsForum: true}
	if !chat.IsForum {
		t.Fatalf("chat = %#v", chat)
	}

	message := Message{ID: 3, ChatID: 9, TopicID: 5}
	if message.TopicID != 5 {
		t.Fatalf("message = %#v", message)
	}
	plain := Message{ID: 4, ChatID: 9}
	if plain.TopicID != 0 {
		t.Fatal("default message topic is not zero")
	}
}
