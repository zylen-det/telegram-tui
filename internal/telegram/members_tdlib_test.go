//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func memberUser(id int64, first, username string) *td.User {
	var usernames *td.Usernames
	if username != "" {
		usernames = &td.Usernames{ActiveUsernames: []string{username}}
	}
	return &td.User{Id: id, FirstName: first, Usernames: usernames}
}

func TestAdapterLoadMembersBasicGroupSlicesLocally(t *testing.T) {
	transport := &dataTransport{
		chatByID: map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeBasicGroup{BasicGroupId: 80}}},
		basicFull: map[int64]*td.BasicGroupFullInfo{80: {Members: []*td.ChatMember{
			{MemberId: &td.MessageSenderUser{UserId: 1}, Status: &td.ChatMemberStatusCreator{IsMember: true}},
			{MemberId: &td.MessageSenderUser{UserId: 2}, Status: &td.ChatMemberStatusMember{}},
			{MemberId: &td.MessageSenderUser{UserId: 3}, Status: &td.ChatMemberStatusMember{}},
		}}},
		userByID: map[int64]*td.User{
			1: memberUser(1, "Owner", "owner"),
			2: memberUser(2, "Ada", ""),
			3: memberUser(3, "Bob", ""),
		},
	}
	page, err := adapterForDataTests(transport).LoadMembers(context.Background(), 9, MemberCursor{Offset: 1, Limit: 1})
	if err != nil || page.TotalCount != 3 || page.NextOffset != 2 || page.Done || len(page.Members) != 1 || page.Members[0].User.ID != 2 {
		t.Fatalf("page = %#v, err = %v", page, err)
	}
}

func TestAdapterLoadMembersSupergroupRequestsRecentPage(t *testing.T) {
	transport := &dataTransport{
		chatByID:  map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeSupergroup{SupergroupId: 81}}},
		superFull: map[int64]*td.SupergroupFullInfo{81: {CanGetMembers: true}},
		members: &td.ChatMembers{TotalCount: 10, Members: []*td.ChatMember{
			{MemberId: &td.MessageSenderUser{UserId: 1}, Status: &td.ChatMemberStatusAdministrator{}, Tag: "lead"},
			{MemberId: &td.MessageSenderUser{UserId: 2}, Status: &td.ChatMemberStatusMember{}},
		}},
		userByID: map[int64]*td.User{1: memberUser(1, "Ada", "ada"), 2: memberUser(2, "Bob", "")},
	}
	page, err := adapterForDataTests(transport).LoadMembers(context.Background(), 9, MemberCursor{Offset: 4, Limit: 2})
	if err != nil || len(page.Members) != 2 || page.TotalCount != 10 || page.NextOffset != 6 || page.Done {
		t.Fatalf("page = %#v, err = %v", page, err)
	}
	if page.Members[0].Role != domain.ChatMemberRoleAdministrator || page.Members[0].Tag != "lead" || page.Members[0].User.Username != "ada" {
		t.Fatalf("first member = %#v", page.Members[0])
	}
	request := transport.getSupergroupMembersRequest
	if request == nil || request.SupergroupId != 81 || request.Offset != 4 || request.Limit != 2 {
		t.Fatalf("request = %#v", request)
	}
	if _, ok := request.Filter.(*td.SupergroupMembersFilterRecent); !ok {
		t.Fatalf("filter = %#v, want recent", request.Filter)
	}
}

func TestAdapterLoadMembersNormalizesRolesAndSkipsInvisible(t *testing.T) {
	transport := &dataTransport{
		chatByID:  map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeSupergroup{SupergroupId: 81}}},
		superFull: map[int64]*td.SupergroupFullInfo{81: {CanGetMembers: true}},
		members: &td.ChatMembers{TotalCount: 9, Members: []*td.ChatMember{
			{MemberId: &td.MessageSenderUser{UserId: 1}, Status: &td.ChatMemberStatusCreator{}},
			{MemberId: &td.MessageSenderUser{UserId: 2}, Status: &td.ChatMemberStatusRestricted{IsMember: true}},
			nil,
			{MemberId: &td.MessageSenderUser{UserId: 3}, Status: &td.ChatMemberStatusLeft{}},
			{MemberId: &td.MessageSenderUser{UserId: 4}, Status: &td.ChatMemberStatusBanned{}},
			{MemberId: &td.MessageSenderUser{UserId: 5}, Status: &td.ChatMemberStatusRestricted{IsMember: false}},
			{MemberId: &td.MessageSenderChat{ChatId: 99}, Status: &td.ChatMemberStatusLeft{}},
			{MemberId: &td.MessageSenderUser{UserId: 0}, Status: &td.ChatMemberStatusMember{}},
			{MemberId: &td.MessageSenderUser{UserId: 6}, Status: &td.ChatMemberStatusMember{}},
		}},
		userByID: map[int64]*td.User{
			1: memberUser(1, "Owner", ""),
			2: memberUser(2, "Restricted", ""),
			6: nil,
		},
	}
	page, err := adapterForDataTests(transport).LoadMembers(context.Background(), 9, MemberCursor{})
	if err != nil || len(page.Members) != 2 {
		t.Fatalf("page = %#v, err = %v", page, err)
	}
	if page.Members[0].Role != domain.ChatMemberRoleOwner || page.Members[1].Role != domain.ChatMemberRoleRestricted {
		t.Fatalf("roles = %v %v", page.Members[0].Role, page.Members[1].Role)
	}
}

func TestAdapterContactBlockSendsExactRequests(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	if err := adapter.AddContact(context.Background(), AddContactRequest{UserID: 7, FirstName: "Ada", LastName: "Lovelace"}); err != nil {
		t.Fatalf("AddContact() error = %v", err)
	}
	request := transport.addContactRequest
	if request == nil || request.UserId != 7 {
		t.Fatalf("add contact request = %#v", request)
	}
	if request.Contact == nil || request.Contact.FirstName != "Ada" || request.Contact.LastName != "Lovelace" || request.SharePhoneNumber {
		t.Fatalf("contact payload = %#v", request.Contact)
	}
	if err := adapter.RemoveContact(context.Background(), RemoveContactRequest{UserID: 7}); err != nil {
		t.Fatalf("RemoveContact() error = %v", err)
	}
	if transport.removeContactsRequest == nil || len(transport.removeContactsRequest.UserIds) != 1 || transport.removeContactsRequest.UserIds[0] != 7 {
		t.Fatalf("remove request = %#v", transport.removeContactsRequest)
	}
	if err := adapter.SetUserBlocked(context.Background(), SetUserBlockedRequest{UserID: 7, Blocked: true}); err != nil {
		t.Fatalf("SetUserBlocked() error = %v", err)
	}
	sender, ok := transport.setBlockRequest.SenderId.(*td.MessageSenderUser)
	if !ok || sender.UserId != 7 {
		t.Fatalf("block sender = %#v", transport.setBlockRequest.SenderId)
	}
	if _, ok := transport.setBlockRequest.BlockList.(*td.BlockListMain); !ok {
		t.Fatalf("block list = %#v, want main", transport.setBlockRequest.BlockList)
	}
	if err := adapter.SetUserBlocked(context.Background(), SetUserBlockedRequest{UserID: 7}); err != nil {
		t.Fatalf("unblock error = %v", err)
	}
	if transport.setBlockRequest.BlockList != nil {
		t.Fatalf("unblock list = %#v, want nil", transport.setBlockRequest.BlockList)
	}
	if err := adapter.AddContact(context.Background(), AddContactRequest{}); err == nil {
		t.Fatal("zero user add should fail")
	}
	failing := &dataTransport{err: errors.New("private transport detail")}
	if err := adapterForDataTests(failing).AddContact(context.Background(), AddContactRequest{UserID: 7, FirstName: "A"}); !errors.As(err, &domain.AppError{}) || strings.Contains(err.Error(), "private transport") {
		t.Fatalf("sanitized error = %#v", err)
	}
}

func TestAdapterLoadUserResolvesNormalizesAndFailsSafely(t *testing.T) {
	transport := &dataTransport{userByID: map[int64]*td.User{7: memberUser(7, "Ada", "ada")}}
	user, err := adapterForDataTests(transport).LoadUser(context.Background(), 7)
	if err != nil || user.ID != 7 || user.Name != "Ada" || user.Username != "ada" {
		t.Fatalf("LoadUser() = (%#v, %v)", user, err)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	if _, err := adapterForDataTests(&dataTransport{}).LoadUser(context.Background(), 0); err == nil {
		t.Fatal("zero user should fail")
	}
	failing := &dataTransport{err: errors.New("private transport detail")}
	if _, err := adapterForDataTests(failing).LoadUser(context.Background(), 7); !errors.As(err, &domain.AppError{}) || strings.Contains(err.Error(), "private transport") {
		t.Fatalf("sanitized error = %#v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapterForDataTests(&dataTransport{}).LoadUser(ctx, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestAdapterLoadMembersRejectsHiddenListBoundsAndFailures(t *testing.T) {
	hidden := &dataTransport{
		chatByID:  map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeSupergroup{SupergroupId: 81}}},
		superFull: map[int64]*td.SupergroupFullInfo{81: {}},
	}
	_, err := adapterForDataTests(hidden).LoadMembers(context.Background(), 9, MemberCursor{})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Op != "load members" || appError.Cause != nil {
		t.Fatalf("hidden error = %#v", err)
	}

	bounds := &dataTransport{
		chatByID:  map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeSupergroup{SupergroupId: 81}}},
		superFull: map[int64]*td.SupergroupFullInfo{81: {CanGetMembers: true}},
		members:   &td.ChatMembers{},
		userByID:  map[int64]*td.User{},
	}
	if _, err := adapterForDataTests(bounds).LoadMembers(context.Background(), 9, MemberCursor{Offset: -5}); err != nil {
		t.Fatalf("negative offset err = %v", err)
	}
	if request := bounds.getSupergroupMembersRequest; request == nil || request.Offset != 0 || request.Limit != 50 {
		t.Fatalf("default request = %#v", request)
	}
	if _, err := adapterForDataTests(bounds).LoadMembers(context.Background(), 9, MemberCursor{Limit: 500}); err != nil {
		t.Fatalf("capped limit err = %v", err)
	}
	if request := bounds.getSupergroupMembersRequest; request.Limit != 200 {
		t.Fatalf("capped limit = %d, want 200", request.Limit)
	}

	private, err := adapterForDataTests(&dataTransport{chatByID: map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypePrivate{UserId: 7}}}}).LoadMembers(context.Background(), 9, MemberCursor{})
	if err != nil || !private.Done || len(private.Members) != 0 {
		t.Fatalf("private page = %#v, err = %v", private, err)
	}

	failing := &dataTransport{chatByID: map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypeBasicGroup{BasicGroupId: 80}}}, err: errors.New("private transport detail")}
	_, err = adapterForDataTests(failing).LoadMembers(context.Background(), 9, MemberCursor{})
	if !errors.As(err, &appError) || appError.Op != "load members" || strings.Contains(err.Error(), "private transport") {
		t.Fatalf("sanitized error = %#v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapterForDataTests(&dataTransport{}).LoadMembers(ctx, 9, MemberCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled err = %v", err)
	}
}
