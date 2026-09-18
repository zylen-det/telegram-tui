package telegram

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeLoadsAndSendsDeterministically(t *testing.T) {
	data := FakeData{
		Chats: []domain.Chat{{ID: 7, Title: "Mina", CanSend: true}},
		Messages: map[domain.ChatID][]domain.Message{
			7: {{ID: 1, ChatID: 7, Kind: domain.MessageText, Text: "hello"}},
		},
	}
	first := NewFake(data)
	second := NewFake(data)

	chats, err := first.LoadChats(context.Background(), ChatCursor{})
	if err != nil {
		t.Fatalf("LoadChats() error = %T, want nil", err)
	}
	if len(chats.Chats) != 1 || chats.Chats[0].ID != 7 || !chats.Done {
		t.Fatal("LoadChats() did not return the complete fixture")
	}
	messages, err := first.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil {
		t.Fatalf("LoadMessages() error = %T, want nil", err)
	}
	if len(messages.Messages) != 1 || messages.Messages[0].ID != 1 || !messages.Done {
		t.Fatal("LoadMessages() did not return the complete fixture")
	}

	firstSent, err := first.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: " world "})
	if err != nil {
		t.Fatalf("first SendText() error = %T, want nil", err)
	}
	secondSent, err := second.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: " world "})
	if err != nil {
		t.Fatalf("second SendText() error = %T, want nil", err)
	}
	if !reflect.DeepEqual(firstSent, secondSent) {
		t.Fatal("identical fake clients produced different sent messages")
	}
	if firstSent.ID >= 0 || firstSent.Text != " world " || !firstSent.Outgoing || firstSent.SendState != domain.SendPending {
		t.Fatal("SendText() did not preserve text or mark a negative pending message")
	}
	if firstSent.SentAt.Location() != time.UTC || firstSent.SentAt.IsZero() {
		t.Fatal("SendText() timestamp is not a fixed nonzero UTC time")
	}
}

func TestFakeLoadMembersPagesBoundsAndOwnsResults(t *testing.T) {
	members := []domain.ChatMember{
		{User: domain.User{ID: 1, Name: "Owner"}, Role: domain.ChatMemberRoleOwner},
		{User: domain.User{ID: 2, Name: "Admin"}, Role: domain.ChatMemberRoleAdministrator},
		{User: domain.User{ID: 3, Name: "Member"}},
	}
	fake := NewFake(FakeData{Members: map[domain.ChatID][]domain.ChatMember{7: members}})
	page, err := fake.LoadMembers(context.Background(), 7, MemberCursor{Offset: 1, Limit: 1})
	if err != nil || page.TotalCount != 3 || page.Done || len(page.Members) != 1 || page.Members[0].User.ID != 2 {
		t.Fatalf("LoadMembers page = %#v, error = %v", page, err)
	}
	page.Members[0].User.Name = "mutated"
	again, _ := fake.LoadMembers(context.Background(), 7, MemberCursor{Offset: 1, Limit: 1})
	if again.Members[0].User.Name != "Admin" {
		t.Fatal("LoadMembers retained caller-owned result storage")
	}
	last, err := fake.LoadMembers(context.Background(), 7, MemberCursor{Offset: 2, Limit: 500})
	if err != nil || !last.Done || len(last.Members) != 1 {
		t.Fatalf("last page = %#v, error = %v", last, err)
	}
	all, err := fake.LoadMembers(context.Background(), 7, MemberCursor{Offset: -4})
	if err != nil || !all.Done || len(all.Members) != 3 {
		t.Fatalf("default/clamped page = %#v, error = %v", all, err)
	}

	configured := errors.New("opaque member failure")
	failed := NewFake(FakeData{MembersError: configured})
	if _, err := failed.LoadMembers(context.Background(), 7, MemberCursor{}); !errors.Is(err, configured) {
		t.Fatalf("configured error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.LoadMembers(ctx, 7, MemberCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestFakeSetDraftRecordsUpdatesAndOwnsRequests(t *testing.T) {
	fake := NewFake(FakeData{Chats: []domain.Chat{{ID: 7, Draft: domain.Draft{Text: "old"}}}})
	request := SetDraftRequest{ChatID: 7, Text: "exact draft", ReplyToMessageID: 44}
	if err := fake.SetDraft(context.Background(), request); err != nil {
		t.Fatalf("SetDraft() error = %v", err)
	}
	if got := fake.DraftCalls(); !reflect.DeepEqual(got, []SetDraftRequest{request}) {
		t.Fatalf("DraftCalls() = %#v", got)
	}
	calls := fake.DraftCalls()
	calls[0].Text = "mutated"
	if fake.DraftCalls()[0].Text != "exact draft" {
		t.Fatal("DraftCalls retained caller-owned result storage")
	}
	page, err := fake.LoadChats(context.Background(), ChatCursor{})
	if err != nil || page.Chats[0].Draft.Text != "exact draft" || page.Chats[0].Draft.ReplyToMessageID != 44 {
		t.Fatalf("stored draft = %#v, error = %v", page.Chats[0].Draft, err)
	}

	configured := errors.New("opaque draft failure")
	failed := NewFake(FakeData{Chats: []domain.Chat{{ID: 7}}, DraftError: configured})
	if err := failed.SetDraft(context.Background(), request); !errors.Is(err, configured) {
		t.Fatalf("configured error = %v", err)
	}
	page, _ = failed.LoadChats(context.Background(), ChatCursor{})
	if page.Chats[0].Draft != (domain.Draft{}) || len(failed.DraftCalls()) != 1 {
		t.Fatalf("failed draft mutation/calls = %#v/%#v", page.Chats[0].Draft, failed.DraftCalls())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fake.SetDraft(ctx, SetDraftRequest{ChatID: 7, Text: "must not save"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SetDraft() error = %v", err)
	}
	if len(fake.DraftCalls()) != 1 {
		t.Fatal("canceled SetDraft recorded a call")
	}
}

func TestFakeMessagePropertiesAreExplicitAndIdentityKeyed(t *testing.T) {
	want := domain.MessageCapabilities{Copy: true, Reply: true, Edit: true, DeleteForSelf: true, DeleteForAll: true}
	fake := NewFake(FakeData{MessageProperties: map[MessageIdentity]domain.MessageCapabilities{{ChatID: 7, MessageID: 3}: want}})
	got, err := fake.GetMessageProperties(context.Background(), 7, 3)
	if err != nil || got != want {
		t.Fatalf("GetMessageProperties() = (%#v, %v)", got, err)
	}
	other, err := fake.GetMessageProperties(context.Background(), 8, 3)
	if err != nil || other != (domain.MessageCapabilities{}) {
		t.Fatalf("other identity = (%#v, %v)", other, err)
	}
}

func TestFakeForwardMessageRecordsExactRequestAndConfiguredError(t *testing.T) {
	fake := NewFake(FakeData{})
	if err := fake.ForwardMessage(context.Background(), ForwardMessageRequest{SourceChatID: 7, SourceMessageID: 3, DestinationChatID: 9}); err != nil {
		t.Fatalf("ForwardMessage() error = %v", err)
	}
	if err := fake.ForwardMessage(context.Background(), ForwardMessageRequest{SourceChatID: 8, SourceMessageID: 4, DestinationChatID: 10}); err != nil {
		t.Fatalf("second ForwardMessage() error = %v", err)
	}
	want := []ForwardMessageRequest{
		{SourceChatID: 7, SourceMessageID: 3, DestinationChatID: 9},
		{SourceChatID: 8, SourceMessageID: 4, DestinationChatID: 10},
	}
	if got := fake.ForwardCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ForwardCalls() = %#v, want %#v", got, want)
	}

	configured := errors.New("opaque forward transport detail")
	fake = NewFake(FakeData{ForwardError: configured})
	if err := fake.ForwardMessage(context.Background(), ForwardMessageRequest{SourceChatID: 7, SourceMessageID: 3, DestinationChatID: 9}); !errors.Is(err, configured) {
		t.Fatalf("configured forward error = %v, want %v", err, configured)
	}
	if calls := fake.ForwardCalls(); len(calls) != 1 || calls[0].SourceChatID != 7 || calls[0].SourceMessageID != 3 || calls[0].DestinationChatID != 9 {
		t.Fatalf("failed forward call = %#v", calls)
	}
}

func TestFakeContactBlockRecordsExactRequestsAndConfiguredErrors(t *testing.T) {
	fake := NewFake(FakeData{})
	if err := fake.AddContact(context.Background(), AddContactRequest{UserID: 7, FirstName: "Ada", LastName: "L"}); err != nil {
		t.Fatalf("AddContact() error = %v", err)
	}
	if err := fake.RemoveContact(context.Background(), RemoveContactRequest{UserID: 7}); err != nil {
		t.Fatalf("RemoveContact() error = %v", err)
	}
	if err := fake.SetUserBlocked(context.Background(), SetUserBlockedRequest{UserID: 7, Blocked: true}); err != nil {
		t.Fatalf("SetUserBlocked() error = %v", err)
	}
	if got := fake.ContactCalls(); !reflect.DeepEqual(got, []AddContactRequest{{UserID: 7, FirstName: "Ada", LastName: "L"}}) {
		t.Fatalf("ContactCalls() = %#v", got)
	}
	if got := fake.RemoveContactCalls(); !reflect.DeepEqual(got, []RemoveContactRequest{{UserID: 7}}) {
		t.Fatalf("RemoveContactCalls() = %#v", got)
	}
	if got := fake.BlockCalls(); !reflect.DeepEqual(got, []SetUserBlockedRequest{{UserID: 7, Blocked: true}}) {
		t.Fatalf("BlockCalls() = %#v", got)
	}

	configured := errors.New("opaque contact detail")
	failed := NewFake(FakeData{ContactError: configured})
	if err := failed.AddContact(context.Background(), AddContactRequest{UserID: 7}); !errors.Is(err, configured) {
		t.Fatalf("configured contact error = %v", err)
	}
	blockErr := errors.New("opaque block detail")
	blocked := NewFake(FakeData{BlockError: blockErr})
	if err := blocked.SetUserBlocked(context.Background(), SetUserBlockedRequest{UserID: 7}); !errors.Is(err, blockErr) {
		t.Fatalf("configured block error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fake.AddContact(ctx, AddContactRequest{UserID: 7}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled AddContact() error = %v", err)
	}
}

func TestFakeLoadUserReturnsCloneAndConfiguredErrors(t *testing.T) {
	fake := NewFake(FakeData{Users: map[domain.UserID]domain.User{7: {ID: 7, Name: "Ada", Username: "ada"}}})
	user, err := fake.LoadUser(context.Background(), 7)
	if err != nil || user.ID != 7 || user.Name != "Ada" {
		t.Fatalf("LoadUser() = (%#v, %v)", user, err)
	}
	user.Name = "mutated"
	again, _ := fake.LoadUser(context.Background(), 7)
	if again.Name != "Ada" {
		t.Fatal("LoadUser retained caller-owned result storage")
	}
	if _, err := fake.LoadUser(context.Background(), 8); err == nil {
		t.Fatal("unknown user should fail")
	}
	configured := errors.New("opaque user detail")
	failed := NewFake(FakeData{LoadUserError: configured})
	if _, err := failed.LoadUser(context.Background(), 7); !errors.Is(err, configured) {
		t.Fatalf("configured error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.LoadUser(ctx, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestFakeDeleteMessageRecordsExactRequestAndConfiguredError(t *testing.T) {
	fake := NewFake(FakeData{})
	if err := fake.DeleteMessage(context.Background(), DeleteMessageRequest{ChatID: 7, MessageID: 3, Revoke: false}); err != nil {
		t.Fatalf("DeleteMessage(revoke=false) error = %v", err)
	}
	if err := fake.DeleteMessage(context.Background(), DeleteMessageRequest{ChatID: 8, MessageID: 4, Revoke: true}); err != nil {
		t.Fatalf("DeleteMessage(revoke=true) error = %v", err)
	}
	want := []DeleteMessageRequest{
		{ChatID: 7, MessageID: 3, Revoke: false},
		{ChatID: 8, MessageID: 4, Revoke: true},
	}
	if got := fake.DeleteCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DeleteCalls() = %#v, want %#v", got, want)
	}

	configured := errors.New("opaque delete transport detail")
	fake = NewFake(FakeData{DeleteError: configured})
	if err := fake.DeleteMessage(context.Background(), DeleteMessageRequest{ChatID: 7, MessageID: 3}); !errors.Is(err, configured) {
		t.Fatalf("configured delete error = %v, want %v", err, configured)
	}
	if calls := fake.DeleteCalls(); len(calls) != 1 || calls[0].ChatID != 7 || calls[0].MessageID != 3 {
		t.Fatalf("failed delete call = %#v", calls)
	}
}

func TestFakePinMessageRecordsExactRequestAndConfiguredError(t *testing.T) {
	fake := NewFake(FakeData{})
	if err := fake.PinMessage(context.Background(), PinMessageRequest{ChatID: 7, MessageID: 3, Unpin: false}); err != nil {
		t.Fatalf("PinMessage(pin) error = %v", err)
	}
	if err := fake.PinMessage(context.Background(), PinMessageRequest{ChatID: 8, MessageID: 4, Unpin: true}); err != nil {
		t.Fatalf("PinMessage(unpin) error = %v", err)
	}
	want := []PinMessageRequest{
		{ChatID: 7, MessageID: 3, Unpin: false},
		{ChatID: 8, MessageID: 4, Unpin: true},
	}
	if got := fake.PinCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PinCalls() = %#v, want %#v", got, want)
	}

	configured := errors.New("opaque pin transport detail")
	fake = NewFake(FakeData{PinError: configured})
	if err := fake.PinMessage(context.Background(), PinMessageRequest{ChatID: 7, MessageID: 3}); !errors.Is(err, configured) {
		t.Fatalf("configured pin error = %v, want %v", err, configured)
	}
	if calls := fake.PinCalls(); len(calls) != 1 || calls[0].ChatID != 7 || calls[0].MessageID != 3 || calls[0].Unpin {
		t.Fatalf("failed pin call = %#v", calls)
	}
}

func TestFakeReactToMessageRecordsExactRequestAndConfiguredError(t *testing.T) {
	fake := NewFake(FakeData{})
	if err := fake.ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 7, MessageID: 3, Emoji: "👍", Remove: false}); err != nil {
		t.Fatalf("ReactToMessage(add) error = %v", err)
	}
	if err := fake.ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 8, MessageID: 4, Emoji: "❤️", Remove: true}); err != nil {
		t.Fatalf("ReactToMessage(remove) error = %v", err)
	}
	want := []ReactToMessageRequest{
		{ChatID: 7, MessageID: 3, Emoji: "👍", Remove: false},
		{ChatID: 8, MessageID: 4, Emoji: "❤️", Remove: true},
	}
	if got := fake.ReactCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReactCalls() = %#v, want %#v", got, want)
	}

	configured := errors.New("opaque reaction transport detail")
	fake = NewFake(FakeData{ReactError: configured})
	if err := fake.ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 7, MessageID: 3, Emoji: "🔥"}); !errors.Is(err, configured) {
		t.Fatalf("configured react error = %v, want %v", err, configured)
	}
	if calls := fake.ReactCalls(); len(calls) != 1 || calls[0].ChatID != 7 || calls[0].MessageID != 3 || calls[0].Emoji != "🔥" || calls[0].Remove {
		t.Fatalf("failed react call = %#v", calls)
	}
}

func TestFakeOwnsFixtureAndLoadedValues(t *testing.T) {
	cause := errors.New("opaque cause")
	failure := &domain.AppError{Kind: domain.ErrorNetwork, Op: "send", Message: "safe", Cause: cause}
	chats := []domain.Chat{{ID: 7, Title: "original"}}
	messages := map[domain.ChatID][]domain.Message{
		7: {{ID: 1, ChatID: 7, Text: "original", Failure: failure}},
	}
	avatars := map[string]string{"small": "/original.png"}
	fake := NewFake(FakeData{Chats: chats, Messages: messages, Avatars: avatars})

	chats[0].Title = "mutated"
	messages[7][0].Text = "mutated"
	failure.Message = "mutated"
	avatars["small"] = "/mutated.png"

	loadedChats, err := fake.LoadChats(context.Background(), ChatCursor{})
	if err != nil {
		t.Fatalf("LoadChats() error = %T, want nil", err)
	}
	loadedMessages, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil {
		t.Fatalf("LoadMessages() error = %T, want nil", err)
	}
	file, err := fake.DownloadAvatar(context.Background(), domain.AvatarRef{UniqueID: "small"}, AvatarSmall)
	if err != nil {
		t.Fatalf("DownloadAvatar() error = %T, want nil", err)
	}
	if loadedChats.Chats[0].Title != "original" || loadedMessages.Messages[0].Text != "original" || loadedMessages.Messages[0].Failure.Message != "safe" || file.Path != "/original.png" {
		t.Fatal("NewFake() retained mutable caller-owned fixture storage")
	}
	if loadedMessages.Messages[0].Failure.Cause != cause {
		t.Fatal("NewFake() did not retain the opaque failure cause")
	}

	loadedChats.Chats[0].Title = "changed output"
	loadedMessages.Messages[0].Text = "changed output"
	loadedMessages.Messages[0].Failure.Message = "changed output"
	reloadedChats, _ := fake.LoadChats(context.Background(), ChatCursor{})
	reloadedMessages, _ := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if reloadedChats.Chats[0].Title != "original" || reloadedMessages.Messages[0].Text != "original" || reloadedMessages.Messages[0].Failure.Message != "safe" {
		t.Fatal("load result mutation changed fake-owned storage")
	}
}

func TestFakeInitializesNilMaps(t *testing.T) {
	fake := NewFake(FakeData{})
	if _, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 3, Text: "hello"}); err != nil {
		t.Fatalf("SendText() error = %T, want nil", err)
	}
	page, err := fake.LoadMessages(context.Background(), 3, MessageCursor{})
	if err != nil {
		t.Fatalf("LoadMessages() error = %T, want nil", err)
	}
	if len(page.Messages) != 1 || !page.Done {
		t.Fatal("nil message map was not initialized")
	}
	file, err := fake.DownloadAvatar(context.Background(), domain.AvatarRef{UniqueID: "missing"}, AvatarSmall)
	if err != nil || file.Path != "" {
		t.Fatal("nil avatar map did not behave as an empty map")
	}
}

func TestFakeChatPagination(t *testing.T) {
	fake := NewFake(FakeData{Chats: []domain.Chat{{ID: 1}, {ID: 2}, {ID: 3}}})
	tests := []struct {
		name    string
		limit   int
		wantIDs []domain.ChatID
		done    bool
	}{
		{name: "zero returns all", wantIDs: []domain.ChatID{1, 2, 3}, done: true},
		{name: "negative returns all", limit: -1, wantIDs: []domain.ChatID{1, 2, 3}, done: true},
		{name: "positive prefix", limit: 2, wantIDs: []domain.ChatID{1, 2}, done: false},
		{name: "oversized", limit: 9, wantIDs: []domain.ChatID{1, 2, 3}, done: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := fake.LoadChats(context.Background(), ChatCursor{Limit: test.limit})
			if err != nil {
				t.Fatalf("LoadChats() error = %T, want nil", err)
			}
			if page.Done != test.done || len(page.Chats) != len(test.wantIDs) {
				t.Fatal("LoadChats() page bounds differ from expectation")
			}
			for index, want := range test.wantIDs {
				if page.Chats[index].ID != want {
					t.Fatalf("chat ID at index %d = %d, want %d", index, page.Chats[index].ID, want)
				}
			}
		})
	}
}

func TestFakeMessagePagination(t *testing.T) {
	fixture := make([]domain.Message, 5)
	for index := range fixture {
		fixture[index] = domain.Message{ID: domain.MessageID(index + 1), ChatID: 7}
	}
	fake := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{7: fixture}})
	tests := []struct {
		name    string
		cursor  MessageCursor
		wantIDs []domain.MessageID
		done    bool
	}{
		{name: "zero returns all", wantIDs: []domain.MessageID{1, 2, 3, 4, 5}, done: true},
		{name: "latest window", cursor: MessageCursor{Limit: 2}, wantIDs: []domain.MessageID{4, 5}, done: false},
		{name: "before boundary", cursor: MessageCursor{FromMessageID: 4, Limit: 2}, wantIDs: []domain.MessageID{2, 3}, done: false},
		{name: "oldest boundary", cursor: MessageCursor{FromMessageID: 2, Limit: 3}, wantIDs: []domain.MessageID{1}, done: true},
		{name: "first boundary", cursor: MessageCursor{FromMessageID: 1, Limit: 3}, wantIDs: nil, done: true},
		{name: "unknown boundary", cursor: MessageCursor{FromMessageID: 99, Limit: 3}, wantIDs: nil, done: true},
		{name: "only local", cursor: MessageCursor{Limit: 2, OnlyLocal: true}, wantIDs: []domain.MessageID{4, 5}, done: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := fake.LoadMessages(context.Background(), 7, test.cursor)
			if err != nil {
				t.Fatalf("LoadMessages() error = %T, want nil", err)
			}
			if page.Done != test.done || len(page.Messages) != len(test.wantIDs) {
				t.Fatal("LoadMessages() page bounds differ from expectation")
			}
			for index, want := range test.wantIDs {
				if page.Messages[index].ID != want {
					t.Fatalf("message ID at index %d = %d, want %d", index, page.Messages[index].ID, want)
				}
			}
		})
	}
}

func TestFakeMessagePaginationSortsShuffledFixturesChronologically(t *testing.T) {
	base := time.Date(2026, time.July, 21, 9, 0, 0, 0, time.UTC)
	fixture := []domain.Message{
		{ID: 5, ChatID: 7, SentAt: base.Add(2 * time.Minute)},
		{ID: 2, ChatID: 7, SentAt: base},
		{ID: 4, ChatID: 7, SentAt: base.Add(time.Minute)},
		{ID: 1, ChatID: 7, SentAt: base},
		{ID: 3, ChatID: 7, SentAt: base.Add(time.Minute)},
	}
	fake := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{7: fixture}})

	all, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil {
		t.Fatalf("LoadMessages(all) error = %T, want nil", err)
	}
	assertMessageIDs(t, all.Messages, []domain.MessageID{1, 2, 3, 4, 5})

	latest, err := fake.LoadMessages(context.Background(), 7, MessageCursor{Limit: 2})
	if err != nil {
		t.Fatalf("LoadMessages(latest) error = %T, want nil", err)
	}
	assertMessageIDs(t, latest.Messages, []domain.MessageID{4, 5})
	if latest.Done {
		t.Fatal("latest shuffled page marked done while older messages remain")
	}

	before, err := fake.LoadMessages(context.Background(), 7, MessageCursor{FromMessageID: 4, Limit: 2})
	if err != nil {
		t.Fatalf("LoadMessages(before) error = %T, want nil", err)
	}
	assertMessageIDs(t, before.Messages, []domain.MessageID{2, 3})
	if before.Done {
		t.Fatal("shuffled page before boundary marked done while an older message remains")
	}
}

func TestFakeSendFollowsLatestFixtureMessageChronologically(t *testing.T) {
	base := time.Date(2026, time.July, 21, 9, 0, 0, 0, time.UTC)
	data := FakeData{Messages: map[domain.ChatID][]domain.Message{
		7: {
			{ID: 3, ChatID: 7, SentAt: base.Add(2 * time.Minute)},
			{ID: 1, ChatID: 7, SentAt: base},
			{ID: 2, ChatID: 7, SentAt: base.Add(time.Minute)},
		},
		8: {{ID: 8, ChatID: 8, SentAt: base.Add(10 * time.Minute)}},
	}}
	first := NewFake(data)
	second := NewFake(data)

	firstSent, err := first.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "newest"})
	if err != nil {
		t.Fatalf("first SendText() error = %T, want nil", err)
	}
	secondSent, err := second.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "newest"})
	if err != nil {
		t.Fatalf("second SendText() error = %T, want nil", err)
	}
	if !reflect.DeepEqual(firstSent, secondSent) {
		t.Fatal("identical fixtures produced different sent messages")
	}
	globalLatest := base.Add(10 * time.Minute)
	if !firstSent.SentAt.After(globalLatest) {
		t.Fatal("sent message timestamp did not follow the global latest fixture message")
	}

	all, err := first.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil {
		t.Fatalf("LoadMessages(all) error = %T, want nil", err)
	}
	assertMessageIDs(t, all.Messages, []domain.MessageID{1, 2, 3, firstSent.ID})
	for index := 1; index < len(all.Messages); index++ {
		if all.Messages[index].SentAt.Before(all.Messages[index-1].SentAt) {
			t.Fatal("SendText() left fake history out of chronological order")
		}
	}

	latest, err := first.LoadMessages(context.Background(), 7, MessageCursor{Limit: 2})
	if err != nil {
		t.Fatalf("LoadMessages(latest) error = %T, want nil", err)
	}
	assertMessageIDs(t, latest.Messages, []domain.MessageID{3, firstSent.ID})
	before, err := first.LoadMessages(context.Background(), 7, MessageCursor{FromMessageID: firstSent.ID, Limit: 2})
	if err != nil {
		t.Fatalf("LoadMessages(before sent) error = %T, want nil", err)
	}
	assertMessageIDs(t, before.Messages, []domain.MessageID{2, 3})
}

func TestFakeSendBaseUsesGlobalMaximumDeterministically(t *testing.T) {
	base := time.Date(2026, time.July, 21, 9, 0, 0, 0, time.UTC)
	data := FakeData{Messages: map[domain.ChatID][]domain.Message{
		1: {{ID: 1, ChatID: 1, SentAt: base}},
		2: {{ID: 2, ChatID: 2, SentAt: base.Add(500 * time.Millisecond)}},
	}}
	wantSentAt := base.Add(1500 * time.Millisecond)
	var first domain.Message
	for attempt := range 512 {
		sent, err := NewFake(data).SendText(context.Background(), SendTextRequest{ChatID: 1, Text: "same"})
		if err != nil {
			t.Fatalf("SendText() attempt %d error = %T, want nil", attempt, err)
		}
		if !sent.SentAt.Equal(wantSentAt) {
			t.Fatalf("SendText() attempt %d timestamp differs from global maximum plus one second", attempt)
		}
		if attempt == 0 {
			first = sent
			continue
		}
		if !reflect.DeepEqual(sent, first) {
			t.Fatalf("SendText() attempt %d differs for the same fixture", attempt)
		}
	}
}

func TestFakeDownloadAvatarSelectsRequestedIdentity(t *testing.T) {
	fake := NewFake(FakeData{Avatars: map[string]string{
		"small":    "/small.png",
		"original": "/original.png",
	}})
	ref := domain.AvatarRef{UniqueID: "small", OriginalUniqueID: "original"}

	small, err := fake.DownloadAvatar(context.Background(), ref, AvatarSmall)
	if err != nil || small.Path != "/small.png" {
		t.Fatal("AvatarSmall did not use the small unique ID")
	}
	original, err := fake.DownloadAvatar(context.Background(), ref, AvatarOriginal)
	if err != nil || original.Path != "/original.png" {
		t.Fatal("AvatarOriginal did not use the original unique ID")
	}
	fallback, err := fake.DownloadAvatar(context.Background(), domain.AvatarRef{UniqueID: "small"}, AvatarOriginal)
	if err != nil || fallback.Path != "/small.png" {
		t.Fatal("AvatarOriginal did not fall back to the small unique ID")
	}
}

func TestFakeStartEmitAndStopLifecycle(t *testing.T) {
	fake := NewFake(FakeData{})
	updates := make(chan Update, 2)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- fake.Start(startCtx, updates) }()

	receiveUpdate(t, updates)
	if err := fake.Start(context.Background(), make(chan Update, 1)); !errors.Is(err, errFakeAlreadyStarted) {
		t.Fatalf("concurrent Start() error = %T, want stable already-started error", err)
	}
	wantUpdate := ConnectionChanged{State: domain.ConnectionOnline}
	if err := fake.Emit(context.Background(), wantUpdate); err != nil {
		t.Fatalf("Emit() error = %T, want nil", err)
	}
	if got := receiveUpdate(t, updates); !reflect.DeepEqual(got, wantUpdate) {
		t.Fatal("Emit() delivered a different update")
	}

	cancelStart()
	if err := receiveError(t, startDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() exit error = %T, want context cancellation", err)
	}
	if err := fake.Emit(context.Background(), Ready{}); !errors.Is(err, errFakeNotStarted) {
		t.Fatalf("Emit() after stop error = %T, want stable not-started error", err)
	}
}

func TestFakeStartRejectsNilUpdateChannel(t *testing.T) {
	if err := NewFake(FakeData{}).Start(context.Background(), nil); !errors.Is(err, errFakeNilUpdates) {
		t.Fatalf("Start(nil) error = %T, want stable nil-channel error", err)
	}
}

func TestFakeDuplicateStartReturnsWithoutReadyReceiver(t *testing.T) {
	_, cleanup := startFakeWithoutReadyReceiver(t, NewFake(FakeData{}))
	cleanup()
}

func TestFakeOperationsContinueWhileReadyHasNoReceiver(t *testing.T) {
	fake := NewFake(FakeData{
		Chats:    []domain.Chat{{ID: 7}},
		Messages: map[domain.ChatID][]domain.Message{7: {{ID: 1, ChatID: 7}}},
		Avatars:  map[string]string{"small": "/small.png"},
	})
	_, cleanup := startFakeWithoutReadyReceiver(t, fake)
	defer cleanup()

	operations := []struct {
		name string
		call func() error
	}{
		{name: "load chats", call: func() error {
			_, err := fake.LoadChats(context.Background(), ChatCursor{})
			return err
		}},
		{name: "load messages", call: func() error {
			_, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
			return err
		}},
		{name: "send text", call: func() error {
			_, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "pending"})
			return err
		}},
		{name: "download avatar", call: func() error {
			_, err := fake.DownloadAvatar(context.Background(), domain.AvatarRef{UniqueID: "small"}, AvatarSmall)
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			result := make(chan error, 1)
			go func() { result <- operation.call() }()
			if err := receiveError(t, result); err != nil {
				t.Fatalf("operation error = %T, want nil", err)
			}
		})
	}
}

func TestFakeEmitWaitsForReadyWithoutOvertaking(t *testing.T) {
	fake := NewFake(FakeData{})
	updates, cleanup := startFakeWithoutReadyReceiver(t, fake)
	defer cleanup()
	want := ConnectionChanged{State: domain.ConnectionOnline}
	emitDone := make(chan error, 1)
	go func() { emitDone <- fake.Emit(context.Background(), want) }()

	if _, ok := receiveUpdate(t, updates).(Ready); !ok {
		t.Fatal("first fake update was not Ready")
	}
	if got := receiveUpdate(t, updates); !reflect.DeepEqual(got, want) {
		t.Fatal("Emit() update overtook Ready")
	}
	if err := receiveError(t, emitDone); err != nil {
		t.Fatalf("Emit() error = %T, want nil", err)
	}
}

func TestFakeReadyPrecedesImmediateEmit(t *testing.T) {
	const attempts = 128
	for range attempts {
		fake := NewFake(FakeData{})
		updates := make(chan Update, 1)
		startCtx, cancelStart := context.WithCancel(context.Background())
		startDone := make(chan error, 1)
		go func() { startDone <- fake.Start(startCtx, updates) }()

		if _, ok := receiveUpdate(t, updates).(Ready); !ok {
			cancelStart()
			receiveError(t, startDone)
			t.Fatal("first fake update was not Ready")
		}
		want := ConnectionChanged{State: domain.ConnectionOnline}
		if err := fake.Emit(context.Background(), want); err != nil {
			cancelStart()
			receiveError(t, startDone)
			t.Fatalf("immediate Emit() after Ready error = %T, want nil", err)
		}
		if got := receiveUpdate(t, updates); !reflect.DeepEqual(got, want) {
			cancelStart()
			receiveError(t, startDone)
			t.Fatal("immediate Emit() delivered a different update")
		}

		cancelStart()
		if err := receiveError(t, startDone); !errors.Is(err, context.Canceled) {
			t.Fatalf("Start() exit error = %T, want context cancellation", err)
		}
	}
}

func TestFakeEmitCanBeCanceledWhileRunning(t *testing.T) {
	fake := NewFake(FakeData{})
	updates := make(chan Update, 1)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- fake.Start(startCtx, updates) }()
	receiveUpdate(t, updates)

	updates <- Ready{}
	emitCtx, cancelEmit := context.WithCancel(context.Background())
	emitDone := make(chan error, 1)
	go func() { emitDone <- fake.Emit(emitCtx, Closed{}) }()
	cancelEmit()
	if err := receiveError(t, emitDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked Emit() error = %T, want context cancellation", err)
	}
	<-updates
	cancelStart()
	receiveError(t, startDone)
}

func TestFakeAlreadyCanceledOperationsDoNotMutateState(t *testing.T) {
	fake := NewFake(FakeData{Messages: map[domain.ChatID][]domain.Message{7: {{ID: 1, ChatID: 7}}}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updates := make(chan Update, 1)

	operations := []struct {
		name string
		call func() error
	}{
		{name: "start", call: func() error { return fake.Start(ctx, updates) }},
		{name: "load chats", call: func() error { _, err := fake.LoadChats(ctx, ChatCursor{}); return err }},
		{name: "load messages", call: func() error { _, err := fake.LoadMessages(ctx, 7, MessageCursor{}); return err }},
		{name: "send", call: func() error {
			_, err := fake.SendText(ctx, SendTextRequest{ChatID: 7, Text: "must not append"})
			return err
		}},
		{name: "download", call: func() error { _, err := fake.DownloadAvatar(ctx, domain.AvatarRef{}, AvatarSmall); return err }},
		{name: "emit", call: func() error { return fake.Emit(ctx, Ready{}) }},
		{name: "close", call: func() error { return fake.Close(ctx) }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error = %T, want context cancellation", err)
			}
		})
	}
	if len(updates) != 0 {
		t.Fatal("canceled Start() emitted an update")
	}
	page, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("canceled operation changed fake storage")
	}
	if err := fake.Emit(context.Background(), Ready{}); !errors.Is(err, errFakeNotStarted) {
		t.Fatal("canceled Start() left the fake registered")
	}
}

func TestFakeOperationsRecheckCancellationAfterLockContention(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Fake) error
	}{
		{name: "load chats", call: func(ctx context.Context, fake *Fake) error {
			_, err := fake.LoadChats(ctx, ChatCursor{})
			return err
		}},
		{name: "load messages", call: func(ctx context.Context, fake *Fake) error {
			_, err := fake.LoadMessages(ctx, 7, MessageCursor{})
			return err
		}},
		{name: "send text", call: func(ctx context.Context, fake *Fake) error {
			_, err := fake.SendText(ctx, SendTextRequest{ChatID: 7, Text: "must not append"})
			return err
		}},
		{name: "download avatar", call: func(ctx context.Context, fake *Fake) error {
			_, err := fake.DownloadAvatar(ctx, domain.AvatarRef{UniqueID: "small"}, AvatarSmall)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := NewFake(FakeData{
				Chats:    []domain.Chat{{ID: 7}},
				Messages: map[domain.ChatID][]domain.Message{7: {{ID: 1, ChatID: 7}}},
				Avatars:  map[string]string{"small": "/small.png"},
			})
			baseCtx, cancel := context.WithCancel(context.Background())
			ctx := newObservedErrContext(baseCtx)
			result := make(chan error, 1)

			fake.mu.Lock()
			locked := true
			defer func() {
				if locked {
					fake.mu.Unlock()
				}
			}()
			go func() { result <- test.call(ctx, fake) }()
			waitForSignal(t, ctx.errObserved, "operation context check")
			cancel()
			fake.mu.Unlock()
			locked = false

			if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error after lock contention = %T, want context cancellation", err)
			}
			fake.mu.Lock()
			messageCount := len(fake.data.Messages[7])
			nextID := fake.nextID
			fake.mu.Unlock()
			if messageCount != 1 || nextID != -1 {
				t.Fatal("canceled operation mutated fake message state")
			}
		})
	}
}

func TestFakeStartRechecksCancellationAfterLockContention(t *testing.T) {
	fake := NewFake(FakeData{})
	updates := make(chan Update, 1)
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := newControlledDoneContext(baseCtx)
	defer ctx.release()
	result := make(chan error, 1)

	fake.mu.Lock()
	locked := true
	defer func() {
		if locked {
			fake.mu.Unlock()
		}
	}()
	go func() { result <- fake.Start(ctx, updates) }()
	waitForSignal(t, ctx.errObserved, "Start context check")
	cancel()
	fake.mu.Unlock()
	locked = false

	guardCtx, cancelGuard := context.WithTimeout(context.Background(), time.Second)
	defer cancelGuard()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start() error after lock contention = %T, want context cancellation", err)
		}
		if len(updates) != 0 {
			t.Fatal("canceled Start() emitted Ready after lock contention")
		}
	case <-updates:
		ctx.release()
		receiveError(t, result)
		t.Fatal("canceled Start() emitted Ready after lock contention")
	case <-guardCtx.Done():
		ctx.release()
		receiveError(t, result)
		t.Fatal("timed out waiting for contended Start()")
	}
}

func TestFakeEmitRechecksCancellationAfterLockContention(t *testing.T) {
	fake := NewFake(FakeData{})
	updates := make(chan Update, 2)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- fake.Start(startCtx, updates) }()
	if _, ok := receiveUpdate(t, updates).(Ready); !ok {
		cancelStart()
		receiveError(t, startDone)
		t.Fatal("first fake update was not Ready")
	}
	probe := ConnectionChanged{State: domain.ConnectionWaiting}
	if err := fake.Emit(context.Background(), probe); err != nil {
		cancelStart()
		receiveError(t, startDone)
		t.Fatalf("setup Emit() error = %T, want nil", err)
	}
	receiveUpdate(t, updates)

	baseCtx, cancelEmit := context.WithCancel(context.Background())
	emitCtx := newControlledDoneContext(baseCtx)
	defer emitCtx.release()
	result := make(chan error, 1)
	fake.mu.Lock()
	locked := true
	defer func() {
		if locked {
			fake.mu.Unlock()
		}
	}()
	go func() { result <- fake.Emit(emitCtx, Closed{}) }()
	waitForSignal(t, emitCtx.errObserved, "Emit context check")
	cancelEmit()
	fake.mu.Unlock()
	locked = false

	if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
		cancelStart()
		receiveError(t, startDone)
		t.Fatalf("Emit() error after lock contention = %T, want context cancellation", err)
	}
	if len(updates) != 0 {
		cancelStart()
		receiveError(t, startDone)
		t.Fatal("canceled Emit() delivered an update after lock contention")
	}
	cancelStart()
	if err := receiveError(t, startDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() exit error = %T, want context cancellation", err)
	}
}

func TestFakeSendPhotoRecordsExactRequestAndDeterministicMessage(t *testing.T) {
	request := SendPhotoRequest{
		ChatID:           77,
		LocalPath:        "/tmp/photo.jpg",
		Caption:          "photo caption",
		ReplyToMessageID: 42,
	}
	first := NewFake(FakeData{})
	second := NewFake(FakeData{})
	firstSent, err := first.SendPhoto(context.Background(), request)
	if err != nil {
		t.Fatalf("first SendPhoto error = %v", err)
	}
	secondSent, err := second.SendPhoto(context.Background(), request)
	if err != nil {
		t.Fatalf("second SendPhoto error = %v", err)
	}
	if !reflect.DeepEqual(firstSent, secondSent) {
		t.Fatal("identical requests produced different sent messages")
	}
	if firstSent.ID >= 0 {
		t.Fatalf("message ID = %d, want negative", firstSent.ID)
	}
	if firstSent.Kind != domain.MessagePhoto {
		t.Fatalf("kind = %v, want MessagePhoto", firstSent.Kind)
	}
	if firstSent.Text != "photo caption" {
		t.Fatalf("text = %q, want photo caption", firstSent.Text)
	}
	if firstSent.ChatID != 77 {
		t.Fatalf("chatID = %d, want 77", firstSent.ChatID)
	}
	if firstSent.ReplyToMessageID != 42 {
		t.Fatalf("replyTo = %d, want 42", firstSent.ReplyToMessageID)
	}
	if !firstSent.HasReply {
		t.Fatal("HasReply should be true")
	}
	if !firstSent.Outgoing {
		t.Fatal("Outgoing should be true")
	}
	if firstSent.SendState != domain.SendPending {
		t.Fatalf("sendState = %v, want SendPending", firstSent.SendState)
	}
	if firstSent.Media.File.LocalPath != "/tmp/photo.jpg" {
		t.Fatalf("media localPath = %q, want /tmp/photo.jpg", firstSent.Media.File.LocalPath)
	}
	if !firstSent.Media.File.Downloaded {
		t.Fatal("media Downloaded should be true")
	}
	if got := first.PhotoSendCalls(); !reflect.DeepEqual(got, []SendPhotoRequest{request}) {
		t.Fatalf("PhotoSendCalls() = %#v, want %#v", got, []SendPhotoRequest{request})
	}
}

func TestFakeSendPhotoZeroReplyHasNoReplyFields(t *testing.T) {
	fake := NewFake(FakeData{})
	message, err := fake.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 7, LocalPath: "/p.jpg", Caption: "c", ReplyToMessageID: 0})
	if err != nil {
		t.Fatal(err)
	}
	if message.HasReply {
		t.Fatal("HasReply should be false when ReplyToMessageID is 0")
	}
	if message.ReplyToMessageID != 0 {
		t.Fatalf("ReplyToMessageID = %d, want 0", message.ReplyToMessageID)
	}
}

func TestFakeSendPhotoConfiguredErrorReturnsExactErrorAndRecordsCall(t *testing.T) {
	configured := errors.New("opaque photo transport detail")
	request := SendPhotoRequest{
		ChatID:           99,
		LocalPath:        "/distinct/photo.png",
		Caption:          "distinct caption",
		ReplyToMessageID: 55,
	}
	fake := NewFake(FakeData{PhotoSendError: configured})
	_, err := fake.SendPhoto(context.Background(), request)
	if !errors.Is(err, configured) {
		t.Fatalf("error = %v, want %v", err, configured)
	}
	if got := fake.PhotoSendCalls(); !reflect.DeepEqual(got, []SendPhotoRequest{request}) {
		t.Fatalf("PhotoSendCalls() = %#v, want %#v", got, []SendPhotoRequest{request})
	}
	msgs, err := fake.LoadMessages(context.Background(), 99, MessageCursor{})
	if err != nil || len(msgs.Messages) != 0 {
		t.Fatalf("chat 99 message count = %d, want 0", len(msgs.Messages))
	}
}

func TestFakeSendPhotoCanceledContextDoesNotRecord(t *testing.T) {
	fake := NewFake(FakeData{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fake.SendPhoto(ctx, SendPhotoRequest{ChatID: 7, LocalPath: "/p.jpg"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	calls := fake.PhotoSendCalls()
	if len(calls) != 0 {
		t.Fatalf("calls count = %d, want 0", len(calls))
	}
}

func TestFakeSendPhotoReturnedCallSliceIsOwned(t *testing.T) {
	fake := NewFake(FakeData{})
	_, _ = fake.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 7, LocalPath: "/p.jpg"})
	first := fake.PhotoSendCalls()
	first[0].ChatID = 999
	second := fake.PhotoSendCalls()
	if second[0].ChatID != 7 {
		t.Fatalf("returned slice mutation changed internal state")
	}
}

func TestFakeSendPhotoPreservesCaptionAndPathInReturnedMessage(t *testing.T) {
	fake := NewFake(FakeData{})
	message, err := fake.SendPhoto(context.Background(), SendPhotoRequest{
		ChatID:    7,
		LocalPath: "/exact/path.png",
		Caption:   "exact caption",
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Media.File.LocalPath != "/exact/path.png" {
		t.Fatalf("localPath = %q, want /exact/path.png", message.Media.File.LocalPath)
	}
	if message.Text != "exact caption" {
		t.Fatalf("text = %q, want exact caption", message.Text)
	}
}

func TestFakeSendPhotoRechecksCancellationAfterLockContention(t *testing.T) {
	fake := NewFake(FakeData{
		Messages: map[domain.ChatID][]domain.Message{7: {{ID: 1, ChatID: 7}}},
	})
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := newObservedErrContext(baseCtx)
	result := make(chan error, 1)

	fake.mu.Lock()
	locked := true
	defer func() {
		if locked {
			fake.mu.Unlock()
		}
	}()
	go func() {
		_, err := fake.SendPhoto(ctx, SendPhotoRequest{ChatID: 7, LocalPath: "/p.jpg"})
		result <- err
	}()
	waitForSignal(t, ctx.errObserved, "SendPhoto context check")
	cancel()
	fake.mu.Unlock()
	locked = false

	if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendPhoto error after lock contention = %T, want context cancellation", err)
	}
	if got := fake.PhotoSendCalls(); len(got) != 0 {
		t.Fatalf("PhotoSendCalls() count = %d, want 0", len(got))
	}
	fake.mu.Lock()
	messageCount := len(fake.data.Messages[7])
	nextID := fake.nextID
	fake.mu.Unlock()
	if messageCount != 1 || nextID != -1 {
		t.Fatal("canceled SendPhoto() mutated fake state")
	}
}

func TestFakeSendPhotoAppendsToFixtureMessages(t *testing.T) {
	originalMsg := domain.Message{ID: 1, ChatID: 7, Text: "existing"}
	callerMessages := map[domain.ChatID][]domain.Message{
		7: {originalMsg},
	}
	// clone expected original for later assertion
	expectedOriginal := domain.Message{ID: originalMsg.ID, ChatID: originalMsg.ChatID, Text: originalMsg.Text}

	fake := NewFake(FakeData{Messages: callerMessages})

	message, err := fake.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 7, LocalPath: "/p.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	// prove caller-owned fixture was not mutated
	if len(callerMessages[7]) != 1 {
		t.Fatalf("caller message count = %d, want 1", len(callerMessages[7]))
	}
	if !reflect.DeepEqual(callerMessages[7][0], expectedOriginal) {
		t.Fatalf("caller message mutated = %#v, want %#v", callerMessages[7][0], expectedOriginal)
	}

	// load from fake: should contain original + sent Photo
	page, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil || len(page.Messages) != 2 {
		t.Fatalf("LoadMessages count = %d, want 2", len(page.Messages))
	}
	if page.Messages[0].ID != 1 {
		t.Fatalf("first message ID = %d, want 1", page.Messages[0].ID)
	}
	if page.Messages[1].ID != message.ID {
		t.Fatal("sent message should be second in loaded messages")
	}
	if page.Messages[1].Kind != domain.MessagePhoto {
		t.Fatal("second message should be Photo kind")
	}
}

func TestFakeDownloadMediaRecordsExactRequestAndConfiguredResult(t *testing.T) {
	fake := NewFake(FakeData{})
	file, err := fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 101, UniqueID: "photo"})
	if err != nil {
		t.Fatalf("DownloadMedia() error = %T, want nil", err)
	}
	if file.Path != "" {
		t.Fatalf("DownloadMedia() returned unexpected file = %#v", file)
	}

	_, err = fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 102, UniqueID: "video"})
	if err != nil {
		t.Fatalf("second DownloadMedia() error = %T, want nil", err)
	}
	calls := fake.MediaDownloadCalls()
	if len(calls) != 2 {
		t.Fatalf("MediaDownloadCalls() count = %d, want 2", len(calls))
	}
	if calls[0].ID != 101 || calls[0].UniqueID != "photo" {
		t.Fatalf("first media call = %#v", calls[0])
	}
	if calls[1].ID != 102 || calls[1].UniqueID != "video" {
		t.Fatalf("second media call = %#v", calls[1])
	}
}

func TestFakeDownloadMediaErrorPropagation(t *testing.T) {
	configured := errors.New("opaque media transport detail")
	fake := NewFake(FakeData{MediaError: configured})
	_, err := fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 201})
	if !errors.Is(err, configured) {
		t.Fatalf("configured media download error = %v, want %v", err, configured)
	}
	calls := fake.MediaDownloadCalls()
	if len(calls) != 1 || calls[0].ID != 201 {
		t.Fatalf("failed media call = %#v", calls)
	}
}

func TestFakeDownloadMediaLocalPathConfig(t *testing.T) {
	fake := NewFake(FakeData{
		MediaFiles: map[int32]string{
			101: "/tmp/photo.jpg",
			102: "/tmp/video.mp4",
		},
	})
	file1, err := fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 101})
	if err != nil || file1.Path != "/tmp/photo.jpg" {
		t.Fatalf("DownloadMedia(ID=101) = (%#v, %v)", file1, err)
	}
	file2, err := fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 102})
	if err != nil || file2.Path != "/tmp/video.mp4" {
		t.Fatalf("DownloadMedia(ID=102) = (%#v, %v)", file2, err)
	}
	// Unknown ID should return empty path
	file3, err := fake.DownloadMedia(context.Background(), domain.MediaFileRef{ID: 999})
	if err != nil || file3.Path != "" {
		t.Fatalf("DownloadMedia(ID=999) = (%#v, %v)", file3, err)
	}
}

func TestFakeConcurrentSendsUseUniqueNegativeIDs(t *testing.T) {
	const count = 128
	fake := NewFake(FakeData{})
	start := make(chan struct{})
	ids := make(chan domain.MessageID, count)
	errs := make(chan error, count)
	var workers sync.WaitGroup
	workers.Add(count)
	for range count {
		go func() {
			defer workers.Done()
			<-start
			message, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "same text"})
			if err == nil {
				ids <- message.ID
			}
			errs <- err
		}()
	}
	close(start)
	workers.Wait()
	close(ids)
	close(errs)

	seen := make(map[domain.MessageID]struct{}, count)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SendText() error = %T, want nil", err)
		}
	}
	for id := range ids {
		if id >= 0 {
			t.Fatalf("sent message ID = %d, want negative", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate sent message ID = %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("unique ID count = %d, want %d", len(seen), count)
	}
	page, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil || len(page.Messages) != count {
		t.Fatal("concurrent sends were not all stored")
	}
}

func TestFakeSendIDsDoNotWrapIntoDuplicates(t *testing.T) {
	fake := NewFake(FakeData{})
	fake.nextID = domain.MessageID(math.MinInt64)

	message, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "last ID"})
	if err != nil || message.ID != domain.MessageID(math.MinInt64) {
		t.Fatal("SendText() did not allocate the last negative message ID")
	}
	if _, err := fake.SendText(context.Background(), SendTextRequest{ChatID: 7, Text: "must not wrap"}); err == nil {
		t.Fatal("SendText() allowed temporary IDs to wrap")
	}
	page, err := fake.LoadMessages(context.Background(), 7, MessageCursor{})
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("exhausted SendText() changed message storage")
	}
}

func TestFakeCloseIsConcurrentSafe(t *testing.T) {
	fake := NewFake(FakeData{})
	const count = 64
	var workers sync.WaitGroup
	workers.Add(count)
	for range count {
		go func() {
			defer workers.Done()
			if err := fake.Close(context.Background()); err != nil {
				t.Errorf("Close() error = %T, want nil", err)
			}
		}()
	}
	workers.Wait()
}

func receiveUpdate(t *testing.T, updates <-chan Update) Update {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case update := <-updates:
		return update
	case <-ctx.Done():
		t.Fatal("timed out waiting for fake update")
		return nil
	}
}

func receiveError(t *testing.T, result <-chan error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatal("timed out waiting for fake operation")
		return nil
	}
}

func startFakeWithoutReadyReceiver(t *testing.T, fake *Fake) (<-chan Update, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan Update)
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- fake.Start(ctx, updates) }()
	}

	guardCtx, cancelGuard := context.WithTimeout(context.Background(), time.Second)
	defer cancelGuard()
	select {
	case err := <-results:
		if !errors.Is(err, errFakeAlreadyStarted) {
			cancel()
			receiveError(t, results)
			t.Fatalf("duplicate Start() error = %T, want stable already-started error", err)
		}
	case <-guardCtx.Done():
		cancel()
		receiveError(t, results)
		receiveError(t, results)
		t.Fatal("duplicate Start() blocked while Ready had no receiver")
	}

	return updates, func() {
		cancel()
		if err := receiveError(t, results); !errors.Is(err, context.Canceled) {
			t.Fatalf("active Start() exit error = %T, want context cancellation", err)
		}
	}
}

func assertMessageIDs(t *testing.T, messages []domain.Message, want []domain.MessageID) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("message count = %d, want %d", len(messages), len(want))
	}
	for index := range want {
		if messages[index].ID != want[index] {
			t.Fatalf("message ID at index %d = %d, want %d", index, messages[index].ID, want[index])
		}
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", operation)
	}
}

type observedErrContext struct {
	context.Context
	errOnce     sync.Once
	errObserved chan struct{}
}

func newObservedErrContext(ctx context.Context) *observedErrContext {
	return &observedErrContext{
		Context:     ctx,
		errObserved: make(chan struct{}),
	}
}

func (c *observedErrContext) Err() error {
	err := c.Context.Err()
	if err == nil {
		c.errOnce.Do(func() { close(c.errObserved) })
	}
	return err
}

type controlledDoneContext struct {
	*observedErrContext
	done     chan struct{}
	doneOnce sync.Once
}

func newControlledDoneContext(ctx context.Context) *controlledDoneContext {
	return &controlledDoneContext{
		observedErrContext: newObservedErrContext(ctx),
		done:               make(chan struct{}),
	}
}

func (c *controlledDoneContext) Done() <-chan struct{} {
	return c.done
}

func (c *controlledDoneContext) release() {
	c.doneOnce.Do(func() { close(c.done) })
}

func TestFakeDownloadMediaRecordsExactRefAndOwnsFixtureMap(t *testing.T) {
	ctx := context.Background()

	// Caller-owned fixture map: prove Fake deep-copies it.
	callerOwned := map[int32]string{101: "/tmp/fixture.jpg"}
	fake := NewFake(FakeData{
		MediaFiles: callerOwned,
		MediaError: nil,
	})

	// Verify initial empty state.
	original := fake.MediaDownloadCalls()
	if len(original) != 0 {
		t.Fatalf("initial calls = %d, want 0", len(original))
	}

	// Remote ref: full ref with every field populated; recorded call DeepEqual to full ref.
	remoteRef := domain.MediaFileRef{ID: 101, UniqueID: "media-101", Size: 99999, ExpectedSize: 99999, CanDownload: true}
	local, err := fake.DownloadMedia(ctx, remoteRef)
	if err != nil {
		t.Fatalf("remote download error = %v, want nil", err)
	}
	if local.Path != "/tmp/fixture.jpg" {
		t.Fatalf("remote path = %q, want /tmp/fixture.jpg", local.Path)
	}
	calls := fake.MediaDownloadCalls()
	if len(calls) != 1 {
		t.Fatalf("calls count = %d, want 1", len(calls))
	}
	if !reflect.DeepEqual(calls[0], remoteRef) {
		t.Fatalf("recorded ref = %#v, want DeepEqual to %#v", calls[0], remoteRef)
	}

	// Local reuse: exact path returned; full ref recorded.
	localRef := domain.MediaFileRef{ID: 200, UniqueID: "local-200", Downloaded: true, LocalPath: "/tmp/local.jpg", CanDownload: false}
	local2, err := fake.DownloadMedia(ctx, localRef)
	if err != nil {
		t.Fatalf("local reuse error = %v, want nil", err)
	}
	if local2.Path != "/tmp/local.jpg" {
		t.Fatalf("local path = %q, want /tmp/local.jpg", local2.Path)
	}
	calls = fake.MediaDownloadCalls()
	if len(calls) != 2 {
		t.Fatalf("calls count = %d, want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[1], localRef) {
		t.Fatalf("recorded local ref = %#v, want DeepEqual to %#v", calls[1], localRef)
	}

	// Caller map mutation does not affect Fake: mutate and delete caller map.
	delete(callerOwned, 101)
	callerOwned[101] = "/tmp/mutated.jpg"
	calls = fake.MediaDownloadCalls()
	if calls[0].ID != 101 {
		t.Fatalf("fake should own its copy, got ID=%d", calls[0].ID)
	}
	// Fake still returns original fixture path.
	local3, err := fake.DownloadMedia(ctx, domain.MediaFileRef{ID: 101})
	if err != nil || local3.Path != "/tmp/fixture.jpg" {
		t.Fatalf("after caller mutation: path=%q err=%v, want /tmp/fixture.jpg nil", local3.Path, err)
	}

	// Combined path + error: Fake configured with both MediaFiles and MediaError;
	// the single call returns the path and exact error while recording the full ref.
	combinedErr := errors.New("configured-media-error-detail")
	fakeCombined := NewFake(FakeData{
		MediaFiles: map[int32]string{303: "/tmp/configured-error.jpg"},
		MediaError: combinedErr,
	})
	combinedRef := domain.MediaFileRef{ID: 303, UniqueID: "cfg-303", Size: 7777, ExpectedSize: 7777, CanDownload: true}
	localCombined, err := fakeCombined.DownloadMedia(ctx, combinedRef)
	if err != combinedErr || !errors.Is(err, combinedErr) {
		t.Fatalf("combined error = %v, want %v", err, combinedErr)
	}
	if localCombined.Path != "/tmp/configured-error.jpg" {
		t.Fatalf("combined path = %q, want /tmp/configured-error.jpg", localCombined.Path)
	}
	combinedCalls := fakeCombined.MediaDownloadCalls()
	if len(combinedCalls) != 1 {
		t.Fatalf("combined calls count = %d, want 1", len(combinedCalls))
	}
	if !reflect.DeepEqual(combinedCalls[0], combinedRef) {
		t.Fatalf("combined recorded ref = %#v, want %#v", combinedCalls[0], combinedRef)
	}

	// Returned MediaDownloadCalls() slice ownership: mutate returned slice element,
	// fetch again, prove internal call unchanged.
	originalCalls := fake.MediaDownloadCalls()
	originalID := originalCalls[0].ID
	originalCalls[0].ID = 99999
	retrievedCalls := fake.MediaDownloadCalls()
	if retrievedCalls[0].ID != originalID {
		t.Fatalf("slice mutation should not affect internal state: got ID=%d, want %d", retrievedCalls[0].ID, originalID)
	}

	// Canceled context returns context.Canceled and does not record.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel before call
	_, err = fake.DownloadMedia(cancelCtx, domain.MediaFileRef{ID: 999})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}
	calls = fake.MediaDownloadCalls()
	// 3 calls: remoteRef, localRef, and the local3 re-fetch of ID:101 above.
	if len(calls) != 3 {
		t.Fatalf("canceled call should not record, calls = %d, want 3", len(calls))
	}
}
