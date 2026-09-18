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

func TestGroupPermissionsInfoActionKeepsDynamicOrder(t *testing.T) {
	chat := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, Title: "Group", CanRestrictMembers: true}
	model := detailsModel(140, 30, chat, detailsRowFor(chat))
	model.DetailsSelected = 2
	permissions := false
	invite := false
	for _, hit := range buildDetailsLayer(model, newRenderStyles(false)).Interactions {
		if hit.ID == "details:group-permissions" && hit.Click.Action == app.OpenGroupPermissions {
			permissions = true
		}
		if hit.ID == "details:invite-links" {
			invite = true
		}
	}
	if !permissions || invite {
		t.Fatalf("permission=%t invite=%t", permissions, invite)
	}
	chat.CanManageInviteLinks = true
	model.ActiveChat = chat
	model.DetailsSelected = 3
	permissions = false
	invite = false
	for _, hit := range buildDetailsLayer(model, newRenderStyles(false)).Interactions {
		permissions = permissions || hit.ID == "details:group-permissions"
		invite = invite || hit.ID == "details:invite-links"
	}
	if !permissions || !invite {
		t.Fatalf("permission=%t invite=%t with invite capability", permissions, invite)
	}
}

func TestAdministrationModalHasKeyboardMouseParityAndManualRows(t *testing.T) {
	for key, want := range map[tea.Key]app.Action{
		{Code: tea.KeyDown}:   app.SelectNext,
		{Code: tea.KeyUp}:     app.SelectPrevious,
		{Code: tea.KeyEnter}:  app.Activate,
		{Code: tea.KeyEscape}: app.Close,
	} {
		if got, ok := mapKeyPress(app.FocusAdministration, tea.KeyPressMsg(key)); !ok || got.Action != want {
			t.Fatalf("administration key %#v = (%#v,%t), want %v", key, got, ok, want)
		}
	}
	model := ui.ViewModel{
		Width: 100, Height: 30, Layout: ui.Layout{Mode: app.LayoutWide}, Focus: app.FocusAdministration,
		Administration: &app.AdministrationState{ChatID: 9, Mode: app.AdministrationDefaultPermissions,
			Snapshot:          &telegram.AdministrationSnapshot{Kind: domain.ChatSupergroup},
			EditedPermissions: telegram.ChatPermissions{CanSendBasicMessages: true}},
	}
	layer := buildAdministrationLayer(model, newRenderStyles(true))
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("administration layer should be modal")
	}
	canvas, _ := detailsSurfaceCanvas(model, layer)
	if !strings.Contains(plainText(canvas.Render()), "[x] Send basic messages") {
		t.Fatal("permission state not visible in manual rows")
	}
	toggle := false
	for _, hit := range layer.Interactions {
		if hit.Click.Action == app.ToggleAdministrationItem && hit.Click.AdminIndex == 0 && hit.Click.ChatID == 9 {
			toggle = true
		}
	}
	if !toggle {
		t.Fatal("mouse toggle lacks field/chat identity")
	}
	model.Administration.Mode = app.AdministrationConfirmation
	model.Administration.PendingAction = telegram.MemberAdministrationBan
	model.Administration.UserID = 1
	layer = buildAdministrationLayer(model, newRenderStyles(true))
	canvas, _ = detailsSurfaceCanvas(model, layer)
	if !strings.Contains(plainText(canvas.Render()), "Ban this member") {
		t.Fatal("ban consequence missing")
	}
}

func TestMemberDetailShowsManageActionOnlyWhenEnabled(t *testing.T) {
	members := &app.MembersState{ChatID: 9, Detail: &app.MemberDetail{UserID: 1, Name: "Ada"}}
	detailOptions := func() []selectorOption {
		rows, _ := memberDetailRows(members, nil)
		return selectorOptionsFromRows(rows)
	}
	for _, option := range detailOptions() {
		if option.Value.Action == app.OpenMemberAdministration {
			t.Fatal("management option shown without capability")
		}
	}
	members.Detail.CanManageInChat = true
	found := false
	for _, option := range detailOptions() {
		if option.Value.Action == app.OpenMemberAdministration && option.Value.UserID == 1 && option.Value.ChatID == 9 {
			found = true
		}
	}
	if !found {
		t.Fatal("management option missing")
	}
}
