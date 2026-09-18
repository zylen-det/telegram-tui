//go:build tdlib

package telegram

import (
	"context"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type chatSettingsOpsStub struct {
	tdOperations
	chat        *td.Chat
	status      td.ChatMemberStatus
	basicInfoID int64
	title       *td.SetChatTitleRequest
	slow        *td.SetChatSlowModeDelayRequest
}

func (o *chatSettingsOpsStub) GetChat(context.Context, *td.GetChatRequest) (*td.Chat, error) {
	return o.chat, nil
}
func (o *chatSettingsOpsStub) GetMe(context.Context) (*td.User, error) {
	return &td.User{Id: 5}, nil
}
func (o *chatSettingsOpsStub) GetChatMember(context.Context, *td.GetChatMemberRequest) (*td.ChatMember, error) {
	return &td.ChatMember{Status: o.status}, nil
}
func (o *chatSettingsOpsStub) GetBasicGroupFullInfo(_ context.Context, request *td.GetBasicGroupFullInfoRequest) (*td.BasicGroupFullInfo, error) {
	o.basicInfoID = request.BasicGroupId
	return &td.BasicGroupFullInfo{Description: "about"}, nil
}
func (o *chatSettingsOpsStub) GetSupergroupFullInfo(context.Context, *td.GetSupergroupFullInfoRequest) (*td.SupergroupFullInfo, error) {
	return &td.SupergroupFullInfo{Description: "channel", SlowModeDelay: 30}, nil
}
func (o *chatSettingsOpsStub) SetChatTitle(_ context.Context, request *td.SetChatTitleRequest) (*td.Ok, error) {
	o.title = request
	return &td.Ok{}, nil
}
func (o *chatSettingsOpsStub) SetChatDescription(context.Context, *td.SetChatDescriptionRequest) (*td.Ok, error) {
	return &td.Ok{}, nil
}
func (o *chatSettingsOpsStub) SetChatSlowModeDelay(_ context.Context, request *td.SetChatSlowModeDelayRequest) (*td.Ok, error) {
	o.slow = request
	return &td.Ok{}, nil
}

func TestChatSettingsUsesPeerIDAndLiveMemberRight(t *testing.T) {
	operations := &chatSettingsOpsStub{chat: &td.Chat{Id: 70, Type: &td.ChatTypeBasicGroup{BasicGroupId: 7}, Title: "Group", Permissions: &td.ChatPermissions{CanChangeInfo: true}}, status: &td.ChatMemberStatusMember{}}
	adapter := &Adapter{operations: operations}
	settings, err := adapter.LoadChatSettings(context.Background(), 70)
	if err != nil || operations.basicInfoID != 7 || settings.Description != "about" || !settings.CanChangeInfo {
		t.Fatalf("basic group settings=%#v peer ID=%d error=%v", settings, operations.basicInfoID, err)
	}
	if err := adapter.SetChatTitle(context.Background(), 70, "Renamed"); err != nil || operations.title == nil || operations.title.ChatId != 70 || operations.title.Title != "Renamed" {
		t.Fatalf("set title request=%#v error=%v", operations.title, err)
	}
	operations.chat = &td.Chat{Id: 80, Type: &td.ChatTypeSupergroup{SupergroupId: 8, IsChannel: true}, Permissions: &td.ChatPermissions{CanChangeInfo: true}}
	settings, err = adapter.LoadChatSettings(context.Background(), 80)
	if err != nil || settings.CanChangeInfo {
		t.Fatalf("channel member inherited group default right: %#v error=%v", settings, err)
	}
	operations.title = nil
	if err := adapter.SetChatTitle(context.Background(), 80, "Forbidden"); err == nil || operations.title != nil {
		t.Fatalf("channel member changed title: request=%#v error=%v", operations.title, err)
	}
}

func TestChatSettingsSlowModeOnlyForAuthorizedSupergroup(t *testing.T) {
	operations := &chatSettingsOpsStub{chat: &td.Chat{Id: 80, Type: &td.ChatTypeSupergroup{SupergroupId: 8}}, status: &td.ChatMemberStatusAdministrator{Rights: &td.ChatAdministratorRights{CanRestrictMembers: true}}}
	adapter := &Adapter{operations: operations}
	if err := adapter.SetChatSlowModeDelay(context.Background(), domain.ChatID(80), 300); err != nil || operations.slow == nil || operations.slow.ChatId != 80 || operations.slow.SlowModeDelay != 300 {
		t.Fatalf("slow mode request=%#v error=%v", operations.slow, err)
	}
	operations.slow = nil
	if err := adapter.SetChatSlowModeDelay(context.Background(), 80, 1); err == nil || operations.slow != nil {
		t.Fatalf("invalid delay reached TDLib: request=%#v error=%v", operations.slow, err)
	}
	operations.chat.Type = &td.ChatTypeSupergroup{SupergroupId: 8, IsChannel: true}
	if err := adapter.SetChatSlowModeDelay(context.Background(), 80, 30); err == nil || operations.slow != nil {
		t.Fatalf("channel slow mode reached TDLib: request=%#v error=%v", operations.slow, err)
	}
}
