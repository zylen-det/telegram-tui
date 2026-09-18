//go:build tdlib

package telegram

import (
	"context"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
)

func TestAdapterApplyChatActionBuildsExactCoreRequests(t *testing.T) {
	newAdapter := func() (*Adapter, *dataTransport) {
		transport := &dataTransport{chatByID: map[int64]*td.Chat{
			9: {
				Id:          9,
				LastMessage: &td.Message{Id: 77, ChatId: 9},
				NotificationSettings: &td.ChatNotificationSettings{
					UseDefaultMuteFor: true, UseDefaultSound: true, SoundId: 42,
					UseDefaultShowPreview: true,
				},
			},
		}}
		return adapterForDataTests(transport), transport
	}

	adapter, transport := newAdapter()
	if err := adapter.ApplyChatAction(context.Background(), ChatActionRequest{ChatID: 9, Action: ChatActionMarkRead}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if transport.viewMessagesRequest == nil || transport.viewMessagesRequest.ChatId != 9 || len(transport.viewMessagesRequest.MessageIds) != 1 || transport.viewMessagesRequest.MessageIds[0] != 77 || !transport.viewMessagesRequest.ForceRead {
		t.Fatalf("view messages request = %#v", transport.viewMessagesRequest)
	}
	if transport.readMentionsRequest == nil || transport.readReactionsRequest == nil || transport.toggleChatUnreadRequest == nil || transport.toggleChatUnreadRequest.IsMarkedAsUnread {
		t.Fatalf("mark read ancillary requests missing: mentions=%#v reactions=%#v unread=%#v", transport.readMentionsRequest, transport.readReactionsRequest, transport.toggleChatUnreadRequest)
	}

	adapter, transport = newAdapter()
	if err := adapter.ApplyChatAction(context.Background(), ChatActionRequest{ChatID: 9, Action: ChatActionMute}); err != nil {
		t.Fatalf("mute: %v", err)
	}
	settings := transport.setChatNotificationRequest.NotificationSettings
	if settings == nil || settings.UseDefaultMuteFor || settings.MuteFor <= 366*24*60*60 || !settings.UseDefaultSound || settings.SoundId != 42 || !settings.UseDefaultShowPreview {
		t.Fatalf("mute settings = %#v", settings)
	}

	tests := []struct {
		action ChatAction
		check  func(*dataTransport) bool
	}{
		{ChatActionMarkUnread, func(t *dataTransport) bool {
			return t.toggleChatUnreadRequest != nil && t.toggleChatUnreadRequest.IsMarkedAsUnread
		}},
		{ChatActionUnmute, func(t *dataTransport) bool {
			return t.setChatNotificationRequest != nil && !t.setChatNotificationRequest.NotificationSettings.UseDefaultMuteFor && t.setChatNotificationRequest.NotificationSettings.MuteFor == 0
		}},
		{ChatActionPin, func(t *dataTransport) bool {
			_, ok := t.toggleChatPinnedRequest.ChatList.(*td.ChatListMain)
			return t.toggleChatPinnedRequest != nil && ok && t.toggleChatPinnedRequest.IsPinned
		}},
		{ChatActionUnpin, func(t *dataTransport) bool {
			return t.toggleChatPinnedRequest != nil && !t.toggleChatPinnedRequest.IsPinned
		}},
		{ChatActionArchive, func(t *dataTransport) bool {
			_, ok := t.addChatToListRequest.ChatList.(*td.ChatListArchive)
			return t.addChatToListRequest != nil && ok
		}},
		{ChatActionUnarchive, func(t *dataTransport) bool {
			_, ok := t.addChatToListRequest.ChatList.(*td.ChatListMain)
			return t.addChatToListRequest != nil && ok
		}},
		{ChatActionClearHistory, func(t *dataTransport) bool {
			return t.deleteChatHistoryRequest != nil && !t.deleteChatHistoryRequest.RemoveFromChatList && !t.deleteChatHistoryRequest.Revoke
		}},
		{ChatActionDeleteConversation, func(t *dataTransport) bool {
			return t.deleteChatHistoryRequest != nil && t.deleteChatHistoryRequest.RemoveFromChatList && !t.deleteChatHistoryRequest.Revoke
		}},
		{ChatActionDeleteChat, func(t *dataTransport) bool { return t.deleteChatRequest != nil }},
		{ChatActionLeaveChat, func(t *dataTransport) bool { return t.leaveChatRequest != nil }},
		{ChatActionJoinChat, func(t *dataTransport) bool { return t.joinChatRequest != nil }},
	}
	for _, test := range tests {
		adapter, transport = newAdapter()
		if err := adapter.ApplyChatAction(context.Background(), ChatActionRequest{ChatID: 9, Action: test.action}); err != nil {
			t.Fatalf("action %d: %v", test.action, err)
		}
		if !test.check(transport) {
			t.Fatalf("action %d built wrong request: %#v", test.action, transport)
		}
	}
}

func TestNormalizerMapsAndUpdatesChatActionState(t *testing.T) {
	n := newNormalizer()
	n.update(&td.UpdateSupergroup{Supergroup: &td.Supergroup{Id: 5, Status: &td.ChatMemberStatusMember{}}})
	updates := n.update(&td.UpdateNewChat{Chat: &td.Chat{
		Id:                      9,
		Type:                    &td.ChatTypeSupergroup{SupergroupId: 5},
		Positions:               []*td.ChatPosition{{List: &td.ChatListMain{}, Order: 100, IsPinned: true}},
		ChatLists:               []td.ChatList{&td.ChatListMain{}},
		IsMarkedAsUnread:        true,
		CanBeDeletedOnlyForSelf: true,
	}})
	chat := updates[0].(ChatUpserted).Chat
	if !chat.IsPinned || chat.IsArchived || !chat.IsMarkedUnread || !chat.CanDeleteForSelf || !chat.IsMember {
		t.Fatalf("normalized chat action fields = %#v", chat)
	}

	updates = n.update(&td.UpdateChatPosition{ChatId: 9, Position: &td.ChatPosition{List: &td.ChatListArchive{}, Order: 90}})
	chat = updates[0].(ChatUpserted).Chat
	if !chat.IsArchived {
		t.Fatalf("archive update = %#v", chat)
	}
	updates = n.update(&td.UpdateChatIsMarkedAsUnread{ChatId: 9, IsMarkedAsUnread: false})
	if updates[0].(ChatUpserted).Chat.IsMarkedUnread {
		t.Fatal("marked-unread update did not clear state")
	}
	updates = n.update(&td.UpdateSupergroup{Supergroup: &td.Supergroup{Id: 5, Status: &td.ChatMemberStatusLeft{}}})
	if updates[0].(ChatUpserted).Chat.IsMember {
		t.Fatal("membership update did not clear state")
	}
}
