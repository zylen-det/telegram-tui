package frontend

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestInviteLinksKeyboardAndInfoCapability(t *testing.T) {
	if got, ok := mapKeyPress(app.FocusDetails, tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"})); !ok || got.Action != app.OpenInviteLinks {
		t.Fatalf("details l = (%#v,%t)", got, ok)
	}
	for key, want := range map[tea.Key]app.Action{
		{Code: tea.KeyDown}:   app.SelectNext,
		{Code: tea.KeyUp}:     app.SelectPrevious,
		{Code: tea.KeyEnter}:  app.Activate,
		{Code: tea.KeyEscape}: app.Close,
	} {
		if got, ok := mapKeyPress(app.FocusInviteLinks, tea.KeyPressMsg(key)); !ok || got.Action != want {
			t.Fatalf("links key %#v = (%#v,%t), want %v", key, got, ok, want)
		}
	}
	chat := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, Title: "Group"}
	model := detailsModel(140, 30, chat, detailsRowFor(chat))
	for _, hit := range buildDetailsLayer(model, newRenderStyles(false)).Interactions {
		if hit.ID == "details:invite-links" {
			t.Fatal("invite action shown without permission")
		}
	}
	model.ActiveChat.CanManageInviteLinks = true
	found := false
	for _, hit := range buildDetailsLayer(model, newRenderStyles(false)).Interactions {
		if hit.ID == "details:invite-links" {
			found = hit.Click.Action == app.OpenInviteLinks
		}
	}
	if !found {
		t.Fatal("permitted invite action missing")
	}
}

func TestInviteLinksModalShowsSelectedActionsAndConfirmation(t *testing.T) {
	model := ui.ViewModel{
		Width: 100, Height: 30, Layout: ui.Layout{Mode: app.LayoutWide}, Focus: app.FocusInviteLinks,
		InviteLinks: &app.InviteLinksState{ChatID: 9, Primary: &telegram.InviteLink{URL: "https://t.me/+primary"}, Links: []telegram.InviteLink{{URL: "https://t.me/+one", Name: "Friends\x1b"}}, Selected: 2},
	}
	layer := buildInviteLinksLayer(model, newRenderStyles(true))
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("invite links layer should be modal")
	}
	canvas, _ := detailsSurfaceCanvas(model, layer)
	content := plainText(canvas.Render())
	if !strings.Contains(content, "Primary link") || !strings.Contains(content, "Friends") || strings.Contains(content, "\x1b") {
		t.Fatalf("list content = %q", content)
	}
	open := false
	for _, hit := range layer.Interactions {
		if hit.Click.Action == app.OpenInviteLinkDetail && hit.Click.InviteURL == "https://t.me/+one" {
			open = true
		}
	}
	if !open {
		t.Fatal("link action lacks identity")
	}
	model.InviteLinks.DetailURL = "https://t.me/+primary"
	model.InviteLinks.Confirming = true
	layer = buildInviteLinksLayer(model, newRenderStyles(true))
	canvas, _ = detailsSurfaceCanvas(model, layer)
	if !strings.Contains(plainText(canvas.Render()), "A new primary link will be issued") {
		t.Fatal("primary revoke consequence missing")
	}
	confirm := false
	for _, hit := range layer.Interactions {
		if hit.Click.Action == app.ConfirmRevokeInviteLink && hit.Click.InviteURL == "https://t.me/+primary" {
			confirm = true
		}
	}
	if !confirm {
		t.Fatal("confirmation action missing")
	}
}
