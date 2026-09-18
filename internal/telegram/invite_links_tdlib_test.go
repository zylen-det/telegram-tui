//go:build tdlib

package telegram

import (
	"testing"

	td "github.com/zelenin/go-tdlib/client"
)

func TestInviteLinkPresentation(t *testing.T) {
	value := inviteLinkPresentation(&td.ChatInviteLink{InviteLink: "opaque", Date: 12, ExpirationDate: 34, MemberLimit: 5, MemberCount: 2, PendingJoinRequestCount: 1, CreatesJoinRequest: true})
	if value.URL != "opaque" || value.Date != 12 || value.ExpirationDate != 34 || value.MemberLimit != 5 || !value.CreatesJoinRequest {
		t.Fatalf("unexpected presentation: %#v", value)
	}
}
