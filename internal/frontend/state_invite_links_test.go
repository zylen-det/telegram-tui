package frontend

import (
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func inviteState(canManage bool) State {
	return State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanManageInviteLinks: canManage}}}
}

func TestInviteLinksGatedByChatCapability(t *testing.T) {
	state := inviteState(false)
	commands := updateState(&state, ActionReceived{Action: OpenInviteLinks, ChatID: 9})
	if state.InviteLinks != nil || len(commands) != 0 || state.Focus != FocusDetails {
		t.Fatalf("ungated invite links opened: state=%#v commands=%#v", state.InviteLinks, commands)
	}
	state = inviteState(true)
	commands = updateState(&state, ActionReceived{Action: OpenInviteLinks, ChatID: 9})
	if state.InviteLinks == nil || state.Focus != FocusInviteLinks || len(commands) != 1 {
		t.Fatalf("gated invite links did not open: state=%#v commands=%#v", state.InviteLinks, commands)
	}
}

func TestInviteLinksPagesDeduplicateAndIgnoreStaleResults(t *testing.T) {
	state := inviteState(true)
	updateState(&state, ActionReceived{Action: OpenInviteLinks, ChatID: 9})
	request := state.InviteLinks.RequestID
	updateState(&state, InviteLinksLoaded{RequestID: request, ChatID: 9, Page: telegram.InviteLinkPage{
		Primary: &telegram.InviteLink{URL: "primary"}, Links: []telegram.InviteLink{{URL: "one"}, {URL: "primary"}}, NextCursor: telegram.InviteLinkCursor{OffsetURL: "one"},
	}})
	if len(state.InviteLinks.Links) != 1 || state.InviteLinks.Links[0].URL != "one" {
		t.Fatalf("page was not ordered/deduplicated: %#v", state.InviteLinks.Links)
	}
	state.InviteLinks.Loading = true
	state.InviteLinks.RequestID++
	before := state.InviteLinks.Links[0].URL
	updateState(&state, InviteLinksLoaded{RequestID: request, ChatID: 9, Page: telegram.InviteLinkPage{Links: []telegram.InviteLink{{URL: "stale"}}}})
	if len(state.InviteLinks.Links) != 1 || state.InviteLinks.Links[0].URL != before {
		t.Fatalf("stale page changed state: %#v", state.InviteLinks.Links)
	}
}

func TestInviteLinksNavigationCreateCopyRevokeAndReload(t *testing.T) {
	state := inviteState(true)
	updateState(&state, ActionReceived{Action: OpenInviteLinks, ChatID: 9})
	request := state.InviteLinks.RequestID
	updateState(&state, InviteLinksLoaded{RequestID: request, ChatID: 9, Page: telegram.InviteLinkPage{Primary: &telegram.InviteLink{URL: "primary"}, Links: []telegram.InviteLink{{URL: "one"}}, Done: true}})
	updateState(&state, ActionReceived{Action: OpenInviteLinkDetail, ChatID: 9, InviteURL: "one"})
	if state.InviteLinks.DetailURL != "one" {
		t.Fatal("link detail did not open")
	}
	updateState(&state, ActionReceived{Action: CopyInviteLink, ChatID: 9, InviteURL: "one"})
	copyRequest := state.InviteLinks.RequestID
	updateState(&state, InviteLinkCopied{RequestID: copyRequest, ChatID: 9, URL: "one"})
	if state.InviteLinks.Working || state.InviteLinks.Notice != "Invite link copied" {
		t.Fatalf("copy feedback missing: %#v", state.InviteLinks)
	}
	updateState(&state, ActionReceived{Action: RevokeInviteLink, ChatID: 9, InviteURL: "one"})
	if !state.InviteLinks.Confirming {
		t.Fatal("revoke did not require confirmation")
	}
	updateState(&state, ActionReceived{Action: CancelRevokeInviteLink, ChatID: 9})
	if state.InviteLinks.Confirming {
		t.Fatal("revoke cancellation failed")
	}
	updateState(&state, ActionReceived{Action: RevokeInviteLink, ChatID: 9, InviteURL: "one"})
	commands := updateState(&state, ActionReceived{Action: ConfirmRevokeInviteLink, ChatID: 9, InviteURL: "one"})
	if len(commands) != 1 || !state.InviteLinks.Working {
		t.Fatalf("revoke did not start: state=%#v commands=%#v", state.InviteLinks, commands)
	}
	revokeRequest := state.InviteLinks.RequestID
	commands = updateState(&state, InviteLinkRevoked{RequestID: revokeRequest, ChatID: 9, URL: "one"})
	if len(commands) != 1 || !state.InviteLinks.Loading || state.InviteLinks.DetailURL != "" || state.InviteLinks.Notice != "Invite link revoked" {
		t.Fatalf("revoke did not reload list: state=%#v commands=%#v", state.InviteLinks, commands)
	}
}

func TestInviteLinksFailuresAreSanitized(t *testing.T) {
	state := inviteState(true)
	updateState(&state, ActionReceived{Action: OpenInviteLinks, ChatID: 9})
	request := state.InviteLinks.RequestID
	updateState(&state, InviteLinksLoadFailed{RequestID: request, ChatID: 9})
	if state.InviteLinks.Error == nil || state.InviteLinks.Error.Message == "" || state.InviteLinks.Error.Message == "raw" {
		t.Fatalf("unsanitized load failure: %#v", state.InviteLinks.Error)
	}
	updateState(&state, ActionReceived{Action: Retry, ChatID: 9})
	request = state.InviteLinks.RequestID
	updateState(&state, InviteLinkCreateFailed{RequestID: request, ChatID: 9})
	if state.Toast == nil || state.Toast.Message == "" || state.InviteLinks.Notice != "Could not create invite link" {
		t.Fatalf("missing sanitized failure toast: %#v", state.Toast)
	}
}
