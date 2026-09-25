package frontend

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestGroupPermissionsInfoActionKeepsDynamicOrder(t *testing.T) {
	chat := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, Title: "Group", CanRestrictMembers: true}
	model := detailsModel(140, 30, chat, detailsRowFor(chat))
	model.DetailsSelected = 2
	permissions := false
	invite := false
	for _, hit := range buildDetailsLayer(model, newRenderStyles(false)).Interactions {
		if hit.ID == "details:group-permissions" && hit.Click.Action == OpenGroupPermissions {
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
	for key, want := range map[tea.Key]Action{
		{Code: tea.KeyDown}:   SelectNext,
		{Code: tea.KeyUp}:     SelectPrevious,
		{Code: tea.KeyEnter}:  Activate,
		{Code: tea.KeyEscape}: Close,
	} {
		if got, ok := mapKeyPress(FocusAdministration, tea.KeyPressMsg(key)); !ok || got.Action != want {
			t.Fatalf("administration key %#v = (%#v,%t), want %v", key, got, ok, want)
		}
	}
	model := ViewModel{
		Width: 100, Height: 30, Layout: ViewLayout{Mode: LayoutWide}, Focus: FocusAdministration,
		Administration: &AdministrationState{ChatID: 9, Mode: AdministrationDefaultPermissions,
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
		if hit.Click.Action == ToggleAdministrationItem && hit.Click.AdminIndex == 0 && hit.Click.ChatID == 9 {
			toggle = true
		}
	}
	if !toggle {
		t.Fatal("mouse toggle lacks field/chat identity")
	}
	model.Administration.Mode = AdministrationConfirmation
	model.Administration.PendingAction = telegram.MemberAdministrationBan
	model.Administration.UserID = 1
	layer = buildAdministrationLayer(model, newRenderStyles(true))
	canvas, _ = detailsSurfaceCanvas(model, layer)
	if !strings.Contains(plainText(canvas.Render()), "Ban this member") {
		t.Fatal("ban consequence missing")
	}
}

func TestMemberDetailShowsManageActionOnlyWhenEnabled(t *testing.T) {
	members := &MembersState{ChatID: 9, Detail: &MemberDetail{UserID: 1, Name: "Ada"}}
	detailOptions := func() []modalRowSpec {
		rows, _ := memberDetailRows(members, nil)
		var out []modalRowSpec
		for _, row := range rows {
			if rowSelectable(row) {
				out = append(out, row)
			}
		}
		return out
	}
	for _, option := range detailOptions() {
		if option.Action.Action == OpenMemberAdministration {
			t.Fatal("management option shown without capability")
		}
	}
	members.Detail.CanManageInChat = true
	found := false
	for _, option := range detailOptions() {
		if option.Action == (ActionReceived{Action: OpenMemberAdministration, ChatID: 9, UserID: 1}) {
			found = true
		}
	}
	if !found {
		t.Fatal("management option missing")
	}
}
