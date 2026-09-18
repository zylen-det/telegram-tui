//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
)

func TestAdapterSearchPublicChatUsesExactUsernameAndNormalizes(t *testing.T) {
	transport := &dataTransport{publicChat: tdChat(99, 300)}
	transport.publicChat.Title = "BotFather"
	chat, err := adapterForDataTests(transport).SearchPublicChat(context.Background(), "BotFather")
	if err != nil {
		t.Fatal(err)
	}
	request := transport.searchPublicChatRequest
	if request == nil || request.Username != "BotFather" {
		t.Fatalf("request = %#v", request)
	}
	if chat.ID != 99 || chat.Title != "BotFather" {
		t.Fatalf("chat = %#v", chat)
	}
	// The private-chat fixture has no username mapping, so the adapter retains
	// the requested username as a display fallback.
	if chat.Username != "BotFather" {
		t.Fatalf("username fallback = %q", chat.Username)
	}
}

func TestAdapterSearchPublicChatNilAndErrorAreSafe(t *testing.T) {
	transport := &dataTransport{publicChat: nil}
	empty, err := adapterForDataTests(transport).SearchPublicChat(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("nil error = %v", err)
	}
	if empty.ID != 0 {
		t.Fatalf("nil chat = %#v", empty)
	}

	transport = &dataTransport{publicChat: &td.Chat{}, err: errors.New("private transport detail")}
	_, err = adapterForDataTests(transport).SearchPublicChat(context.Background(), "BotFather")
	if err == nil || err.Error() == "private transport detail" {
		t.Fatalf("error leaked raw cause: %v", err)
	}

	zeroFixture := tdChat(0, 0)
	zeroFixture.Title = "zero"
	transport = &dataTransport{publicChat: zeroFixture}
	zero, err := adapterForDataTests(transport).SearchPublicChat(context.Background(), "BotFather")
	if err != nil {
		t.Fatalf("zero error = %v", err)
	}
	if zero.ID != 0 {
		t.Fatalf("zero chat = %#v", zero)
	}

}

func TestAdapterSearchPublicChatsHydratesAndSorts(t *testing.T) {
	transport := &dataTransport{
		publicChats: &td.Chats{ChatIds: []int64{7, 9}},
		chatByID: map[int64]*td.Chat{
			7: tdChat(7, 100),
			9: tdChat(9, 300),
		},
	}
	chats, err := adapterForDataTests(transport).SearchPublicChats(context.Background(), "bot")
	if err != nil {
		t.Fatal(err)
	}
	if transport.searchPublicChatsRequest == nil || transport.searchPublicChatsRequest.Query != "bot" {
		t.Fatalf("request = %#v", transport.searchPublicChatsRequest)
	}
	if len(chats) != 2 || chats[0].ID != 9 || chats[1].ID != 7 {
		t.Fatalf("chats = %#v", chats)
	}
}

func TestAdapterSearchAllMessagesUsesMainListAndLimit(t *testing.T) {
	transport := &dataTransport{
		foundMessages: &td.FoundMessages{
			TotalCount: 2,
			Messages:   []*td.Message{tdTextMessage(30, 9, 1, 30, "needle new"), tdTextMessage(20, 8, 1, 20, "needle old")},
		},
	}
	page, err := adapterForDataTests(transport).SearchAllMessages(context.Background(), "needle", 10)
	if err != nil {
		t.Fatal(err)
	}
	request := transport.searchMessagesRequest
	if request == nil || request.Query != "needle" || request.Limit != 10 {
		t.Fatalf("request = %#v", request)
	}
	if len(page.Messages) != 2 || page.Messages[0].ChatID != 9 || page.Messages[1].ChatID != 8 || !page.Done {
		t.Fatalf("page = %#v", page)
	}
}
