package frontend

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAuthorizationInputHostOwnsPersistentHuhInput(t *testing.T) {
	host := newAuthorizationInputHost()
	if host == nil || host.field == nil {
		t.Fatal("fresh authorization input host or Huh field is nil")
	}
	if host.Identity() != 0 || host.Value() != "" {
		t.Fatalf("fresh host identity/value = %d/%q, want zero/empty", host.Identity(), host.Value())
	}
}

func TestAuthorizationInputHostSyncIdentityValueSecretFocusAndWidth(t *testing.T) {
	host := newAuthorizationInputHost()
	focusCmd := host.Sync(11, "s界🙂cret", true, true, 0)
	if focusCmd == nil {
		t.Fatal("initial focus transition returned nil command")
	}
	if host.Identity() != 11 || host.Value() != "s界🙂cret" || !host.focused || !host.secret || host.width != 1 {
		t.Fatalf("synced host = id:%d value:%q focused:%t secret:%t width:%d", host.Identity(), host.Value(), host.focused, host.secret, host.width)
	}
	plain := ansiSequence.ReplaceAllString(host.View(), "")
	if strings.Contains(plain, "s界🙂cret") || strings.Contains(plain, "🙂") {
		t.Fatalf("password Huh Input leaked secret: %q", plain)
	}
	if strings.TrimSpace(plain) == "" {
		t.Fatal("password Huh Input rendered no mask")
	}

	if cmd := host.Sync(11, "plain", true, false, 24); cmd != nil {
		t.Fatal("same-focus sync returned transition command")
	}
	plain = ansiSequence.ReplaceAllString(host.View(), "")
	if !strings.Contains(plain, "plain") {
		t.Fatalf("secret-to-normal transition did not restore normal echo: %q", plain)
	}

	if cmd := host.Sync(12, "plain", true, false, 24); cmd != nil {
		t.Fatal("same-focus identity transition returned command")
	}
	if host.Identity() != 12 || host.Value() != "plain" {
		t.Fatalf("same-value identity transition = %d/%q, want 12/plain", host.Identity(), host.Value())
	}

	blurCmd := host.Sync(13, "next", false, true, 18)
	if blurCmd != nil {
		t.Fatal("Huh Input Blur returned a fabricated command")
	}
	if host.Identity() != 13 || host.Value() != "next" || host.focused || !host.secret || host.width != 18 {
		t.Fatalf("identity transition retained stale state: id:%d value:%q focused:%t secret:%t width:%d", host.Identity(), host.Value(), host.focused, host.secret, host.width)
	}
	changed, value, _ := host.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	if changed || value != "next" {
		t.Fatalf("blurred host accepted text: changed=%t value=%q", changed, value)
	}
}

func TestAuthorizationInputHostUpdateReturnsWholeUnicodeValue(t *testing.T) {
	host := newAuthorizationInputHost()
	_ = host.Sync(21, "", false, false, 32)
	_ = host.Sync(21, "", true, false, 32)

	changed, value, _ := host.Update(tea.PasteMsg{Content: "a界🙂"})
	if !changed || value != "a界🙂" || host.Value() != value {
		t.Fatalf("paste changed/value/host = %t/%q/%q", changed, value, host.Value())
	}
	changed, value, _ = host.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if !changed || value != "a界" {
		t.Fatalf("backspace changed/value = %t/%q, want true/a界", changed, value)
	}
}

func TestAuthorizationInputHostDisablesHuhSubmitNavigation(t *testing.T) {
	host := newAuthorizationInputHost()
	_ = host.Sync(31, "keep", true, false, 32)
	for _, key := range []tea.Key{
		{Code: tea.KeyEnter},
		{Code: tea.KeyTab},
		{Code: tea.KeyTab, Mod: tea.ModShift},
	} {
		changed, value, cmd := host.Update(tea.KeyPressMsg(key))
		if changed || value != "keep" {
			t.Fatalf("reserved key %#v changed value: changed=%t value=%q", key, changed, value)
		}
		if cmd != nil {
			msg := cmd()
			t.Fatalf("reserved key %#v emitted Huh navigation/submit command: %T %#v", key, msg, msg)
		}
	}
}
