package frontend

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestMembersKeyboardNavigationUsesHuhAcrossWindowAndPage(t *testing.T) {
	state := membersBaseState(domain.ChatSupergroup)
	state.Width, state.Height = 80, 24
	state.Focus = FocusMembers
	state.Members = &MembersState{RequestID: 10, ChatID: 9, NextOffset: 40}
	for i := 1; i <= 40; i++ {
		state.Members.Results = append(state.Members.Results, domain.ChatMember{User: domain.User{ID: domain.UserID(i), Name: fmt.Sprintf("Member %d", i)}})
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	for i := 1; i <= 39; i++ {
		key := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
		if i%2 == 0 {
			key = tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"})
		}
		model, _ = updateAppModel(t, model, key)
		if got := model.Snapshot().Members.Selected; got != i {
			t.Fatalf("down %d: selected=%d", i, got)
		}
		if got := model.listModals.host.Value().UserID; got != domain.UserID(i+1) {
			t.Fatalf("down %d: Huh user=%d", i, got)
		}
		_ = model.View()
	}
	if !model.Snapshot().Members.Loading {
		t.Fatal("last member should request next page")
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if model.Snapshot().Members.Selected != 39 || model.listModals.host.Value().UserID != 40 {
		t.Fatal("navigation during pagination must not wrap or advance")
	}
	// After the page arrives, navigate upwards across the viewport boundary.
	model, _ = updateAppModel(t, model, MembersLoaded{RequestID: model.Snapshot().Members.RequestID, ChatID: 9, Page: telegram.MemberPage{Members: []domain.ChatMember{{User: domain.User{ID: 41, Name: "Member 41"}}}, NextOffset: 41, Done: true}})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if model.Snapshot().Members.Selected != 40 || model.listModals.host.Value().UserID != 41 {
		t.Fatal("newly loaded last member is not selectable")
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if model.Snapshot().Members.Selected != 40 || model.listModals.host.Value().UserID != 41 {
		t.Fatal("Huh wrapped past the end of the list")
	}
	for i := 39; i >= 10; i-- {
		key := tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
		if i%2 == 0 {
			key = tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"})
		}
		model, _ = updateAppModel(t, model, key)
		if got := model.Snapshot().Members.Selected; got != i {
			t.Fatalf("up %d: selected=%d", i, got)
		}
		if got := model.listModals.host.Value().UserID; got != domain.UserID(i+1) {
			t.Fatalf("up %d: Huh user=%d", i, got)
		}
		if !strings.Contains(ansi.Strip(model.listModals.View()), fmt.Sprintf("Member %d", i+1)) {
			t.Fatalf("up %d: selected member not visible in Huh", i)
		}
		_ = model.View()
	}
	// A mouse click after Huh scrolling must still target the displayed row.
	var clicked Hit
	for _, hit := range model.hitRegions() {
		if hit.Click.Action == OpenMemberDetail && hit.Click.UserID != 11 {
			clicked = hit
			break
		}
	}
	if clicked.Rect.Empty() {
		t.Fatal("no clickable member row after scrolling")
	}
	model, _ = updateAppModel(t, model, tea.MouseClickMsg{X: clicked.Rect.Min.X, Y: clicked.Rect.Min.Y, Button: tea.MouseLeft})
	if detail := model.Snapshot().Members.Detail; detail == nil || detail.UserID != clicked.Click.UserID {
		t.Fatalf("click opened %#v, want user %d", detail, clicked.Click.UserID)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if detail := model.Snapshot().Members.Detail; detail == nil || detail.UserID != 11 {
		t.Fatalf("Enter must activate Huh-selected member 11, detail=%#v", detail)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if members := model.Snapshot().Members; members.Detail != nil || members.Selected != 10 || model.listModals.host.Value().UserID != 11 {
		t.Fatalf("returning from detail lost the selected member: %#v", members)
	}
}
