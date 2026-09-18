package frontend

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func membersViewModel() ui.ViewModel {
	return ui.ViewModel{
		Width: 100, Height: 30,
		Layout: ui.Layout{Mode: app.LayoutWide},
		Focus:  app.FocusMembers,
		Members: &app.MembersState{
			RequestID: 10, ChatID: 9,
			Results: []domain.ChatMember{
				{User: domain.User{ID: 1, Name: "Ada", Username: "ada"}, Role: domain.ChatMemberRoleOwner},
				{User: domain.User{ID: 2, Name: "Bob"}, Role: domain.ChatMemberRoleMember, Tag: "helper"},
			},
			Selected: 0,
		},
	}
}

func TestMembersKeyMappings(t *testing.T) {
	if got, ok := mapKeyPress(app.FocusDetails, tea.KeyPressMsg(tea.Key{Code: 'm', Text: "m"})); !ok || got.Action != app.OpenMembers {
		t.Fatalf("details m = (%#v,%t), want OpenMembers", got, ok)
	}
	cases := map[tea.Key]app.Action{
		{Code: tea.KeyDown}:    app.SelectNext,
		{Code: 'j', Text: "j"}: app.SelectNext,
		{Code: tea.KeyUp}:      app.SelectPrevious,
		{Code: 'k', Text: "k"}: app.SelectPrevious,
		{Code: tea.KeyEnter}:   app.Activate,
		{Code: tea.KeyEscape}:  app.Close,
		{Code: 'q', Text: "q"}: app.Close,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(app.FocusMembers, tea.KeyPressMsg(key))
		if !ok || got.Action != want {
			t.Fatalf("members key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
}

func TestMembersSelectorOptionsCarryChatUserIdentity(t *testing.T) {
	model := membersViewModel()
	options := selectorOptionsFromRows(membersRows(model.Members))
	if len(options) != 2 || options[0].ID != "member:1" || options[1].ID != "member:2" {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Value != (app.ActionReceived{Action: app.OpenMemberDetail, ChatID: 9, UserID: 1}) {
		t.Fatalf("option payload = %#v", options[0].Value)
	}
	if got := selectorOptionsFromRows(membersRows(nil)); got != nil {
		t.Fatalf("nil members options = %#v", got)
	}
}

func TestMemberResultLabelSanitizesAndBounds(t *testing.T) {
	label := memberResultLabel(domain.ChatMember{User: domain.User{ID: 1, Name: " Ada\x00\x01 ", Username: "@ada"}, Role: domain.ChatMemberRoleAdministrator, Tag: "lead\x00"})
	if strings.Contains(label, "\x00") || !strings.Contains(label, "Ada") || !strings.Contains(label, "@ada") || !strings.Contains(label, "Administrator: lead") {
		t.Fatalf("label = %q", label)
	}
	empty := memberResultLabel(domain.ChatMember{})
	if !strings.Contains(empty, "Unknown") {
		t.Fatalf("empty label = %q", empty)
	}
}

func TestMembersLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := membersViewModel()
	layer := buildMembersLayer(model, styles, "")
	if layer.Layer == nil || !layer.IsModal || layer.Rect.Empty() {
		t.Fatalf("layer = nil=%t modal=%t rect=%v", layer.Layer == nil, layer.IsModal, layer.Rect)
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if wantWidth := membersFrame(bounds).Dx(); layer.Rect.Dx() != wantWidth {
		t.Fatalf("width = %d, want %d", layer.Rect.Dx(), wantWidth)
	}
	for _, interaction := range layer.Interactions {
		if strings.HasPrefix(interaction.ID, "conversation:") || strings.HasPrefix(interaction.ID, "chat:") {
			t.Fatalf("underlying hit leaked: %q", interaction.ID)
		}
	}
	loading := membersViewModel()
	loading.Members.Results = nil
	loading.Members.Loading = true
	if rows := membersRows(loading.Members); len(rows) != 1 || rows[0].Label != "Loading members..." {
		t.Fatalf("loading rows = %#v", rows)
	}
	emptyModel := membersViewModel()
	emptyModel.Members.Results = nil
	if rows := membersRows(emptyModel.Members); len(rows) != 1 || rows[0].Label != "No members" {
		t.Fatalf("empty rows = %#v", rows)
	}
	failed := membersViewModel()
	failed.Members.Results = nil
	failed.Members.Error = &domain.AppError{Message: "Could not load members"}
	if rows := membersRows(failed.Members); len(rows) != 1 || rows[0].Label != "Could not load members" {
		t.Fatalf("error rows = %#v", rows)
	}
	rows := membersRows(model.Members)
	if rows[0].Action != (app.ActionReceived{Action: app.OpenMemberDetail, ChatID: 9, UserID: 1}) {
		t.Fatalf("row action = %#v", rows[0].Action)
	}
}

func TestMemberDetailRowsShowInfoBackAndActions(t *testing.T) {
	model := membersViewModel()
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada", Role: domain.ChatMemberRoleOwner}
	rows, headers := memberDetailRows(model.Members, nil)
	// Name header + role header + Back + 5 actions.
	if len(rows) != 8 || headers != 2 || !strings.Contains(rows[0].Label, "Ada") || !strings.Contains(rows[0].Label, "@ada") {
		t.Fatalf("rows = %#v headers=%d", rows, headers)
	}
	if rows[1].Label != "Owner" {
		t.Fatalf("role row = %#v", rows[1])
	}
	if rows[2].ID != "member:back" || rows[2].Label != "‹ Back" {
		t.Fatalf("back row = %#v", rows[2])
	}
	if rows[3].Label != "Copy username" || rows[3].Action.Action != app.CopyMemberUsername {
		t.Fatalf("first action = %#v", rows[3])
	}
	if rows[3].Action != (app.ActionReceived{Action: app.CopyMemberUsername, ChatID: 9, UserID: 1}) {
		t.Fatalf("copy action = %#v", rows[3].Action)
	}
	// Anonymous member hides Copy and has no role header.
	model.Members.Detail = &app.MemberDetail{UserID: 2, Name: "Bob"}
	rows, headers = memberDetailRows(model.Members, nil)
	if len(rows) != 6 || headers != 1 || rows[1].Label != "‹ Back" || rows[2].Label != "Add to contacts" {
		t.Fatalf("anonymous rows = %#v headers=%d", rows, headers)
	}
	// Working appends a status row.
	model.Members.Detail.Working = true
	rows, _ = memberDetailRows(model.Members, nil)
	if rows[len(rows)-1].Label != "Working..." {
		t.Fatalf("working rows = %#v", rows)
	}
	if _, headers := memberDetailRows(nil, nil); headers != 0 {
		t.Fatal("nil detail rows should be nil")
	}
	if _, headers := memberDetailRows(&app.MembersState{}, nil); headers != 0 {
		t.Fatal("empty detail rows should be nil")
	}
}

func TestMemberDetailAvatarRowsCenteredAndCounted(t *testing.T) {
	cells := make([]pixel.Cell, 36)
	for i := range cells {
		cells[i] = pixel.Cell{Rune: ' ', Foreground: color.NRGBA{R: 1, G: 2, B: 3, A: 255}, Background: color.NRGBA{R: 4, G: 5, B: 6, A: 255}}
	}
	avatar := pixel.Avatar{Width: 6, Height: 6, Cells: cells}
	labels := memberAvatarLabels(avatar, 40, false)
	if len(labels) != memberDetailAvatarHeight {
		t.Fatalf("avatar rows = %d, want %d", len(labels), memberDetailAvatarHeight)
	}
	for _, label := range labels {
		plain := ansi.Strip(label)
		if got := ansi.StringWidth(plain); got != (40-memberDetailAvatarWidth)/2+memberDetailAvatarWidth {
			t.Fatalf("avatar row width = %d: %q", got, label)
		}
		if !strings.Contains(label, "▀") {
			t.Fatalf("avatar row missing half blocks: %q", label)
		}
	}
	if memberAvatarLabels(pixel.Avatar{}, 40, false) != nil {
		t.Fatal("empty avatar should yield no rows")
	}
	if got := memberDetailChromeRows(&app.MemberDetail{Role: domain.ChatMemberRoleAdministrator}, true); got != 2+memberDetailAvatarHeight {
		t.Fatalf("chrome rows = %d", got)
	}
	if got := memberDetailChromeRows(&app.MemberDetail{}, false); got != 1 {
		t.Fatalf("minimal chrome rows = %d", got)
	}
}

func TestMemberDetailLayerShowsAvatarAndKeepsSelection(t *testing.T) {
	styles := newRenderStyles(true)
	model := membersViewModel()
	cells := make([]pixel.Cell, 36)
	for i := range cells {
		cells[i] = pixel.Cell{Rune: ' '}
	}
	model.MemberDetailAvatar = pixel.Avatar{Width: 6, Height: 6, Cells: cells}
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada"}
	layer := buildMembersLayer(model, styles, "")
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("detail layer should be modal")
	}
	canvas, _ := detailsSurfaceCanvas(model, layer)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Ada") || !strings.Contains(text, "‹ Back") || !strings.Contains(text, "▀") {
		t.Fatalf("detail layer missing avatar/info/back: %q", text)
	}
}

func TestMessageMenuUserInfoRowAfterCopy(t *testing.T) {
	menu := &app.MessageActionMenu{
		ChatID: 9, MessageID: 22, UserID: 7,
		Capabilities: domain.MessageCapabilities{Copy: true},
	}
	options := selectorOptionsFromRows(messageActionRows(menu))
	if len(options) != 2 || options[0].ID != "action:copy" || options[1].ID != "action:user-info" {
		t.Fatalf("options = %#v", options)
	}
	if options[1].Value != (app.ActionReceived{Action: app.ViewUserInfo, ChatID: 9, MessageID: 22}) {
		t.Fatalf("user info option = %#v", options[1].Value)
	}
	styles := newRenderStyles(false)
	model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menu}
	surface := buildActionModalLayer(model, styles)
	if len(surface.Interactions) != 3 {
		t.Fatalf("interactions = %#v", surface.Interactions)
	}
	if surface.Interactions[2].ID != "action:user-info" {
		t.Fatalf("row order = %#v", surface.Interactions)
	}
	anonymous := &app.MessageActionMenu{ChatID: 9, MessageID: 22, Capabilities: domain.MessageCapabilities{Copy: true}}
	if got := selectorOptionsFromRows(messageActionRows(anonymous)); len(got) != 1 {
		t.Fatalf("anonymous options = %#v", got)
	}
}

func TestMemberDetailViewAvatarRowAndOption(t *testing.T) {
	model := membersViewModel()
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada", Avatar: domain.AvatarRef{UniqueID: "a1"}}
	rows, _ := memberDetailRows(model.Members, nil)
	// Header + Back + View avatar + Copy + 4 contact/block actions.
	if len(rows) != 8 || rows[2].Label != "View avatar" {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[2].Action != (app.ActionReceived{Action: app.ViewMemberAvatar, ChatID: 9, UserID: 1}) {
		t.Fatalf("view action = %#v", rows[2].Action)
	}
	options := selectorOptionsFromRows(rows)
	if len(options) != 7 || options[1].ID != "member-action:view-avatar" {
		t.Fatalf("options = %#v", options)
	}
}

func TestMemberAvatarModalStaysTopmostOverMembers(t *testing.T) {
	model := membersViewModel()
	model.Focus = app.FocusModal
	model.Modal = &app.ModalState{Title: "Ada", Path: "/tmp/avatar.png"}
	frame := composeApplication(model, time.UTC)
	for _, hit := range frame.Hits {
		switch hit.Click.Action {
		case app.OpenMemberDetail, app.CloseMemberDetail, app.ViewMemberAvatar,
			app.CopyMemberUsername, app.AddMemberContact, app.RemoveMemberContact,
			app.BlockMember, app.UnblockMember:
			t.Fatalf("member action leaked under avatar modal: %#v", hit.Click)
		}
	}
	if !strings.Contains(frame.Content, "Ada") {
		t.Fatalf("avatar modal title missing: %q", frame.Content)
	}
}

func TestMemberDetailSelectorOptionsMirrorRows(t *testing.T) {
	model := membersViewModel()
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada"}
	rows, _ := memberDetailRows(model.Members, nil)
	options := selectorOptionsFromRows(rows)
	if len(options) != 6 || options[0].ID != "member:back" || options[1].ID != "member-action:copy-username" {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Value != (app.ActionReceived{Action: app.CloseMemberDetail, ChatID: 9}) {
		t.Fatalf("back option = %#v", options[0].Value)
	}
}

func TestMembersLayerUsesDetailTitle(t *testing.T) {
	styles := newRenderStyles(true)
	model := membersViewModel()
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada"}
	if got := membersTitle(model.Members); got != "Ada" {
		t.Fatalf("title = %q", got)
	}
	layer := buildMembersLayer(model, styles, "")
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("detail layer should be modal")
	}
	foundBack := false
	for _, interaction := range layer.Interactions {
		if interaction.ID == "member:back" {
			foundBack = true
		}
	}
	if !foundBack {
		t.Fatalf("back interaction missing: %#v", layer.Interactions)
	}
}

func TestMemberDetailAvatarResolvesFromCache(t *testing.T) {
	cells := make([]pixel.Cell, 36)
	for i := range cells {
		cells[i] = pixel.Cell{Rune: 'x'}
	}
	state := app.InitialState()
	state.Focus = app.FocusMembers
	state.Members = &app.MembersState{
		ChatID: 9,
		Detail: &app.MemberDetail{UserID: 1, Name: "Ada", AvatarKey: "avatar-1:chat-list"},
	}
	state.Avatars["avatar-1:chat-list"] = app.AvatarState{Cells: pixel.Avatar{Width: 6, Height: 6, Cells: cells}}
	model := ui.Select(state, time.UTC)
	if len(model.MemberDetailAvatar.Cells) != 36 {
		t.Fatalf("avatar cells = %d", len(model.MemberDetailAvatar.Cells))
	}
	model.MemberDetailAvatar.Cells[0].Rune = 'y'
	if state.Avatars["avatar-1:chat-list"].Cells.Cells[0].Rune != 'x' {
		t.Fatal("viewmodel aliased avatar cells")
	}
	// Loading entries resolve to no avatar.
	loading := state
	entry := loading.Avatars["avatar-1:chat-list"]
	entry.Loading = true
	loading.Avatars["avatar-1:chat-list"] = entry
	if got := ui.Select(loading, time.UTC); len(got.MemberDetailAvatar.Cells) != 0 {
		t.Fatalf("loading avatar = %d cells", len(got.MemberDetailAvatar.Cells))
	}
}

func TestMembersViewModelClonesWithoutAliasing(t *testing.T) {
	state := app.InitialState()
	state.Focus = app.FocusMembers
	state.Members = &app.MembersState{
		ChatID:  9,
		Results: []domain.ChatMember{{User: domain.User{ID: 1, Name: "x"}}},
	}
	model := ui.Select(state, time.UTC)
	if model.Members == nil || len(model.Members.Results) != 1 {
		t.Fatalf("projection = %#v", model.Members)
	}
	model.Members.Results[0].User.Name = "mutated"
	if state.Members.Results[0].User.Name != "x" {
		t.Fatal("viewmodel aliased members state")
	}
}
