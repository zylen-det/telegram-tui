package frontend

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestPhotoPathInputHostUsesPersistentHuhInput(t *testing.T) {
	host := newPhotoPathInputHost()
	if host == nil || host.field == nil {
		t.Fatal("newPhotoPathInputHost did not construct a Huh Input")
	}
	field := host.field
	if cmd := host.Sync(domain.ChatID(9), "/tmp/a.png", true, 17); cmd == nil {
		t.Fatal("initial focus transition returned nil command")
	}
	if host.field != field {
		t.Fatal("Sync replaced the persistent Huh Input")
	}
	if got := host.Identity(); got != 9 {
		t.Fatalf("identity = %d, want 9", got)
	}
	if got := host.Value(); got != "/tmp/a.png" {
		t.Fatalf("value = %q", got)
	}
	if !host.focused || host.width != 17 {
		t.Fatalf("host focus/width = %t/%d", host.focused, host.width)
	}
}

func TestPhotoPathInputHostSynchronizesIdentityValueFocusAndWidth(t *testing.T) {
	host := newPhotoPathInputHost()
	_ = host.Sync(9, "first", true, 0)
	if host.width != 1 {
		t.Fatalf("clamped width = %d, want 1", host.width)
	}
	if cmd := host.Sync(9, "first", true, 7); cmd != nil {
		t.Fatal("same focused state returned a transition command")
	}
	if host.width != 7 {
		t.Fatalf("resized width = %d, want 7", host.width)
	}
	if cmd := host.Sync(9, "authoritative", true, 7); cmd != nil {
		t.Fatal("same-focus authoritative value sync returned a transition command")
	}
	if host.Identity() != 9 || host.Value() != "authoritative" {
		t.Fatalf("same-identity value sync = id:%d value:%q", host.Identity(), host.Value())
	}
	if cmd := host.Sync(10, "authoritative", false, 11); cmd != nil {
		t.Fatal("Huh Input.Blur returned an unexpected command")
	}
	if host.Identity() != 10 || host.Value() != "authoritative" || host.focused || host.width != 11 {
		t.Fatalf("resynchronized host = id:%d value:%q focus:%t width:%d", host.Identity(), host.Value(), host.focused, host.width)
	}
	_ = host.Sync(0, "", false, 3)
	if host.Identity() != 0 || host.Value() != "" {
		t.Fatal("cleared identity retained stale path")
	}
}

func TestPhotoPathInputHostUpdateReportsWholeValue(t *testing.T) {
	host := newPhotoPathInputHost()
	_ = host.Sync(9, "", true, 20)
	changed, value, _ := host.Update(tea.KeyPressMsg(tea.Key{Text: "a界"}))
	if !changed || value != "a界" {
		t.Fatalf("key update = changed:%t value:%q", changed, value)
	}
	changed, value, _ = host.Update(tea.PasteMsg{Content: "🙂x"})
	if !changed || value != "a界🙂x" {
		t.Fatalf("paste update = changed:%t value:%q", changed, value)
	}
	changed, value, _ = host.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if !changed || value != "a界🙂" {
		t.Fatalf("backspace update = changed:%t value:%q", changed, value)
	}
}
