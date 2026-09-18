package telegram

import (
	"context"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeAdministrationMutationsAndChannelGuard(t *testing.T) {
	chatID := domain.ChatID(5)
	userID := domain.UserID(8)
	fake := NewFake(FakeData{
		AdminSnapshots:      map[domain.ChatID]AdministrationSnapshot{chatID: {Kind: domain.ChatSupergroup}},
		MemberAdminStatuses: map[domain.ChatID]map[domain.UserID]MemberAdministrationStatus{chatID: {userID: {Role: domain.ChatMemberRoleMember}}},
	})
	rights := AdministratorRights{CanManageChat: true, CanPostStories: true}
	if err := fake.ApplyMemberAdministration(context.Background(), MemberAdministrationRequest{ChatID: chatID, UserID: userID, Action: MemberAdministrationPromote, Rights: rights}); err != nil {
		t.Fatal(err)
	}
	status, err := fake.LoadMemberAdministration(context.Background(), chatID, userID)
	if err != nil || status.Role != domain.ChatMemberRoleAdministrator || !status.Rights.CanPostStories {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	permissions := ChatPermissions{CanSendPhotos: true, CanCreateTopics: true}
	if err := fake.SetDefaultChatPermissions(context.Background(), chatID, permissions); err != nil {
		t.Fatal(err)
	}
	snapshot, err := fake.LoadAdministration(context.Background(), chatID)
	if err != nil || snapshot.DefaultPermissions != permissions {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	channel := domain.ChatID(6)
	fake = NewFake(FakeData{AdminSnapshots: map[domain.ChatID]AdministrationSnapshot{channel: {Kind: domain.ChatChannel}}})
	if err := fake.SetDefaultChatPermissions(context.Background(), channel, ChatPermissions{}); err == nil {
		t.Fatal("channel default permission write succeeded")
	}
}
