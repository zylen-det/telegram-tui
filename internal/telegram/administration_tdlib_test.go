//go:build tdlib

package telegram

import (
	"testing"

	td "github.com/zelenin/go-tdlib/client"
)

func TestAdministrationPermissionMappingPreservesFields(t *testing.T) {
	permissions := fromTDPermissions(&td.ChatPermissions{CanSendBasicMessages: true, CanEditTag: true, CanCreateTopics: true})
	if !permissions.CanSendBasicMessages || !permissions.CanEditTag || !permissions.CanCreateTopics {
		t.Fatalf("permissions=%#v", permissions)
	}
	rights := fromTDRights(&td.ChatAdministratorRights{CanManageChat: true, CanPostStories: true, CanManageDirectMessages: true, IsAnonymous: true})
	if !rights.CanManageChat || !rights.CanPostStories || !rights.CanManageDirectMessages || !rights.IsAnonymous {
		t.Fatalf("rights=%#v", rights)
	}
	back := toTDPermissions(permissions)
	if !back.CanSendBasicMessages || !back.CanEditTag || !back.CanCreateTopics {
		t.Fatalf("round trip=%#v", back)
	}
}
