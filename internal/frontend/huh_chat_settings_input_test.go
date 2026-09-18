package frontend

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestChatSettingsInputHostSwitchesEditorsWithoutKeepingOldText(t *testing.T) {
	host := newChatSettingsInputHost()
	host.Sync(11, "First", true, 24)
	if host.Identity() != 11 || host.Value() != "First" {
		t.Fatalf("initial editor = %d %q", host.Identity(), host.Value())
	}
	changed, value, _ := host.Update(tea.KeyPressMsg(tea.Key{Text: "界"}))
	if !changed || value != "First界" {
		t.Fatalf("Unicode edit = %q, changed=%t", value, changed)
	}
	host.Sync(12, "Second", true, 24)
	if host.Identity() != 12 || host.Value() != "Second" {
		t.Fatalf("new editor kept old text: %d %q", host.Identity(), host.Value())
	}
}
