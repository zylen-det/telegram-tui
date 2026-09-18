package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestHandlerLoadTopicsSuccessPreservesRequestChatCursorAndPage(t *testing.T) {
	page := telegram.TopicPage{Topics: []domain.ForumTopic{{ID: 5, ChatID: 9, Name: "Announcements"}}, TotalCount: 1, NextOffsetTopicID: 5, Done: true}
	client := &handlerClient{topicPage: page}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	cursor := telegram.TopicCursor{OffsetDate: 9, OffsetMessageID: 7, OffsetTopicID: 4, Limit: 20}

	events := collectHandlerEvents(handler, LoadTopics{RequestID: 31, ChatID: 9, Cursor: cursor})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one", events)
	}
	loaded, ok := events[0].(TopicsLoaded)
	if !ok || loaded.RequestID != 31 || loaded.ChatID != 9 || !reflect.DeepEqual(loaded.Page, page) {
		t.Fatalf("loaded event = %#v", events[0])
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.topicChat != 9 || client.topicCursor != cursor {
		t.Fatalf("topic load call = (%d, %#v)", client.topicChat, client.topicCursor)
	}
}

func TestHandlerLoadTopicsFailureIsSanitizedAndNeverKeepsCause(t *testing.T) {
	private := "raw topic transport detail 7001"
	client := &handlerClient{err: errors.New(private)}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadTopics{RequestID: 32, ChatID: 9})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one", events)
	}
	failed, ok := events[0].(TopicsLoadFailed)
	if !ok || failed.RequestID != 32 || failed.ChatID != 9 {
		t.Fatalf("failure event = %#v", events[0])
	}
	want := domain.AppError{Kind: domain.ErrorNetwork, Op: "load topics", Message: "Could not load topics"}
	if failed.Error != want {
		t.Fatalf("failure error = %#v, want %#v", failed.Error, want)
	}
	if failed.Error.Cause != nil {
		t.Fatalf("failure error kept cause: %#v", failed.Error)
	}

	unavailable := NewHandler(nil, nil, nil, nil, nil)
	events = collectHandlerEvents(unavailable, LoadTopics{RequestID: 33, ChatID: 9})
	failed, ok = events[0].(TopicsLoadFailed)
	if !ok || failed.RequestID != 33 || failed.ChatID != 9 || failed.Error != want {
		t.Fatalf("unavailable failure event = %#v", events[0])
	}
	if strings.Contains(failed.Error.Message, private) {
		t.Fatalf("private detail leaked: %#v", failed.Error)
	}
}

func TestHandlerTopicCommandsSetCursorTopicIDAndPreserveEvents(t *testing.T) {
	topic := domain.TopicID(5)
	page := telegram.MessagePage{Done: true}
	searchPage := telegram.MessageSearchPage{Done: true}
	tests := []struct {
		name        string
		command     Command
		want        Event
		wantMessage telegram.MessageCursor
		wantSearch  telegram.MessageSearchCursor
	}{
		{
			name:        "load messages",
			command:     LoadMessages{RequestID: 41, ChatID: 9, TopicID: topic, Cursor: telegram.MessageCursor{FromMessageID: 88, Limit: 20}},
			want:        MessagesLoaded{RequestID: 41, ChatID: 9, TopicID: topic, Page: page},
			wantMessage: telegram.MessageCursor{FromMessageID: 88, TopicID: topic, Limit: 20},
		},
		{
			name:       "search messages",
			command:    SearchChatMessages{RequestID: 42, ChatID: 9, TopicID: topic, Query: "q", Cursor: telegram.MessageSearchCursor{FromMessageID: 77, Limit: 10}},
			want:       ChatMessagesSearched{RequestID: 42, ChatID: 9, TopicID: topic, Page: searchPage},
			wantSearch: telegram.MessageSearchCursor{FromMessageID: 77, TopicID: topic, Limit: 10},
		},
		{
			name:       "pinned messages",
			command:    LoadPinnedMessages{RequestID: 43, ChatID: 9, TopicID: topic, Cursor: telegram.MessageSearchCursor{Limit: 12}},
			want:       PinnedMessagesLoaded{RequestID: 43, ChatID: 9, TopicID: topic, Page: searchPage},
			wantSearch: telegram.MessageSearchCursor{TopicID: topic, Limit: 12},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &handlerClient{messagePage: page, searchPage: searchPage}
			handler := newTestHandler(t, client, &handlerAvatarRenderer{})
			events := collectHandlerEvents(handler, test.command)
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one", events)
			}
			if !reflect.DeepEqual(events[0], test.want) {
				t.Fatalf("event = %#v, want %#v", events[0], test.want)
			}
			client.mu.Lock()
			defer client.mu.Unlock()
			if test.wantMessage != (telegram.MessageCursor{}) {
				if client.messageChat != 9 || client.messageCursor != test.wantMessage {
					t.Fatalf("load messages call = (%d, %#v), want cursor %#v", client.messageChat, client.messageCursor, test.wantMessage)
				}
			}
			if test.wantSearch != (telegram.MessageSearchCursor{}) {
				if client.searchChat != 9 || client.searchCursor != test.wantSearch {
					t.Fatalf("search/pinned call = (%d, %#v), want cursor %#v", client.searchChat, client.searchCursor, test.wantSearch)
				}
			}
		})
	}
}

func TestHandlerTopicContextCommandsUseTopicContextLoader(t *testing.T) {
	topic := domain.TopicID(5)
	page := telegram.MessagePage{Done: true}

	client := &handlerClient{messagePage: page}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadSearchMessageContext{RequestID: 51, ChatID: 9, TopicID: topic, MessageID: 12})
	if len(events) != 1 {
		t.Fatalf("search context events = %#v", events)
	}
	loaded, ok := events[0].(SearchMessageContextLoaded)
	if !ok || loaded.RequestID != 51 || loaded.ChatID != 9 || loaded.TopicID != topic || loaded.MessageID != 12 || !reflect.DeepEqual(loaded.Page, page) {
		t.Fatalf("search context event = %#v", events[0])
	}
	client.mu.Lock()
	if client.topicContextChat != 9 || client.topicContextID != topic || client.topicContextMessage != 12 {
		t.Fatalf("search topic context call = (%d, %d, %d)", client.topicContextChat, client.topicContextID, client.topicContextMessage)
	}
	if client.contextChat != 0 || client.contextMessage != 0 {
		t.Fatalf("plain context loader was used for topic: (%d, %d)", client.contextChat, client.contextMessage)
	}
	client.mu.Unlock()

	events = collectHandlerEvents(handler, LoadPinnedMessageContext{RequestID: 52, ChatID: 9, TopicID: topic, MessageID: 13})
	if len(events) != 1 {
		t.Fatalf("pinned context events = %#v", events)
	}
	pinned, ok := events[0].(PinnedMessageContextLoaded)
	if !ok || pinned.RequestID != 52 || pinned.ChatID != 9 || pinned.TopicID != topic || pinned.MessageID != 13 || !reflect.DeepEqual(pinned.Page, page) {
		t.Fatalf("pinned context event = %#v", events[0])
	}
	client.mu.Lock()
	if client.topicContextChat != 9 || client.topicContextID != topic || client.topicContextMessage != 13 {
		t.Fatalf("pinned topic context call = (%d, %d, %d)", client.topicContextChat, client.topicContextID, client.topicContextMessage)
	}
	client.mu.Unlock()

	// Zero topics keep the existing plain context loader.
	client.mu.Lock()
	client.topicContextChat, client.topicContextID, client.topicContextMessage = 0, 0, 0
	client.mu.Unlock()
	events = collectHandlerEvents(handler, LoadSearchMessageContext{RequestID: 53, ChatID: 9, MessageID: 14})
	if len(events) != 1 {
		t.Fatalf("zero-topic search context events = %#v", events)
	}
	plain, ok := events[0].(SearchMessageContextLoaded)
	if !ok || plain.TopicID != 0 || plain.MessageID != 14 {
		t.Fatalf("zero-topic search context event = %#v", events[0])
	}
	client.mu.Lock()
	if client.contextChat != 9 || client.contextMessage != 14 {
		t.Fatalf("zero-topic plain context call = (%d, %d)", client.contextChat, client.contextMessage)
	}
	if client.topicContextChat != 0 || client.topicContextID != 0 || client.topicContextMessage != 0 {
		t.Fatalf("topic loader used for zero topic: (%d, %d, %d)", client.topicContextChat, client.topicContextID, client.topicContextMessage)
	}
	client.mu.Unlock()
}

func TestHandlerSaveDraftCapturesTopicIDAndKeepsTopicKeysIndependent(t *testing.T) {
	client := &handlerClient{}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})

	events := collectHandlerEvents(handler, SaveDraft{RequestID: 61, ChatID: 9, TopicID: 5, Text: "draft A"})
	if len(events) != 1 {
		t.Fatalf("draft A events = %#v", events)
	}
	saved, ok := events[0].(DraftSaved)
	if !ok || saved.RequestID != 61 || saved.ChatID != 9 || saved.TopicID != 5 {
		t.Fatalf("draft A event = %#v", events[0])
	}
	client.mu.Lock()
	first := client.draftRequest
	client.mu.Unlock()
	if first != (telegram.SetDraftRequest{ChatID: 9, TopicID: 5, Text: "draft A"}) {
		t.Fatalf("draft A request = %#v", first)
	}

	events = collectHandlerEvents(handler, SaveDraft{RequestID: 62, ChatID: 9, TopicID: 6, Text: "draft B"})
	if len(events) != 1 {
		t.Fatalf("draft B events = %#v", events)
	}
	saved, ok = events[0].(DraftSaved)
	if !ok || saved.RequestID != 62 || saved.ChatID != 9 || saved.TopicID != 6 {
		t.Fatalf("draft B event = %#v", events[0])
	}
	client.mu.Lock()
	second := client.draftRequest
	client.mu.Unlock()
	// Same chat, different topics: each topic key produced its own request
	// with its own topic ID; the first request was not overwritten by the
	// second pending save.
	if second != (telegram.SetDraftRequest{ChatID: 9, TopicID: 6, Text: "draft B"}) {
		t.Fatalf("draft B request = %#v", second)
	}
	if second.TopicID == first.TopicID {
		t.Fatalf("topic keys coalesced: first=%#v second=%#v", first, second)
	}
}

func TestHandlerSendTextCarriesTopicIDAndEmitsQueuedEvent(t *testing.T) {
	client := &handlerClient{sent: domain.Message{ID: -71, ChatID: 9}}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, SendText{RequestID: 71, LocalID: -1, ChatID: 9, TopicID: 5, Text: "topic text"})
	if len(events) != 1 {
		t.Fatalf("send events = %#v", events)
	}
	queued, ok := events[0].(TextQueued)
	if !ok || queued.RequestID != 71 || queued.LocalID != -1 || !reflect.DeepEqual(queued.Message, client.sent) {
		t.Fatalf("send event = %#v", events[0])
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.sendRequest != (telegram.SendTextRequest{ChatID: 9, TopicID: 5, Text: "topic text"}) {
		t.Fatalf("send request = %#v", client.sendRequest)
	}
	if client.sendChat != 9 || client.sendText != "topic text" {
		t.Fatalf("send capture = (%d, %q)", client.sendChat, client.sendText)
	}
}
