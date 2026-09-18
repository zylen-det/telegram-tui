package frontend

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

// TestComposerTextHostControlledUnicodeEditing verifies that a blurred host
// ignores input, and a focused host correctly updates via keypress, paste,
// and backspace with exact complete Unicode values.
func TestComposerTextHostControlledUnicodeEditing(t *testing.T) {
	h := newComposerTextHost()

	// Start blurred and empty.
	snap := composerTextIdentity{ChatID: 9, EditMessageID: 0}
	t.Run("blurred_ignores_input", func(t *testing.T) {
		// Sync blurred, no focus.
		h.Sync(snap, "", false, 30, 2)

		// KeyPress should be absorbed without changing value.
		changed, val, cmd := h.Update(tea.KeyPressMsg(tea.Key{Code: 0, Text: "a"}))
		if changed {
			t.Error("expected changed=false while blurred")
		}
		if val != "" {
			t.Errorf("expected empty value while blurred, got %q", val)
		}
		if cmd != nil {
			t.Errorf("expected nil cmd while blurred, got %v", cmd)
		}

		// Paste should also be ignored.
		changed, val, cmd = h.Update(tea.PasteMsg{Content: "你好"})
		if changed {
			t.Error("expected changed=false on paste while blurred")
		}
		if val != "" {
			t.Errorf("expected empty value on paste while blurred, got %q", val)
		}
		if cmd != nil {
			t.Error("expected nil cmd on paste while blurred")
		}
	})

	// Sync focused identity.
	t.Run("focused_sync", func(t *testing.T) {
		cmd := h.Sync(snap, "", true, 30, 2)
		// Focus transition cmd should be non-nil.
		if cmd == nil {
			t.Error("expected non-nil focus transition cmd")
		}
		if h.focused != true {
			t.Error("expected focused=true after Sync with focused=true")
		}
	})

	// Now input should work.
	t.Run("focused_keypress", func(t *testing.T) {
		changed, val, cmd := h.Update(tea.KeyPressMsg(tea.Key{Code: 0, Text: "a"}))
		if !changed {
			t.Error("expected changed=true on focused keypress")
		}
		if val != "a" {
			t.Errorf("expected value %q, got %q", "a", val)
		}
		if cmd != nil {
			t.Logf("got Huh cmd: %v", cmd)
		}
	})

	t.Run("focused_paste", func(t *testing.T) {
		changed, val, _ := h.Update(tea.PasteMsg{Content: "界"})
		if !changed {
			t.Error("expected changed=true on focused paste")
		}
		if val != "a界" {
			t.Errorf("expected value %q, got %q", "a界", val)
		}
	})

	t.Run("focused_backspace", func(t *testing.T) {
		changed, val, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		if !changed {
			t.Error("expected changed=true on focused backspace")
		}
		if val != "a" {
			t.Errorf("expected value %q, got %q", "a", val)
		}
	})

	t.Run("focused_emoji", func(t *testing.T) {
		changed, val, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: '🙂', Text: "🙂"}))
		if !changed {
			t.Error("expected changed=true on focused emoji")
		}
		if val != "a🙂" {
			t.Errorf("expected value %q, got %q", "a🙂", val)
		}
	})
}

// TestComposerTextHostAuthoritativeAndIdentityReset verifies that the host
// respects authoritative value resets and resets completely on identity
// change.
func TestComposerTextHostAuthoritativeAndIdentityReset(t *testing.T) {
	h := newComposerTextHost()

	snap1 := composerTextIdentity{ChatID: 9, EditMessageID: 0}
	snap2 := composerTextIdentity{ChatID: 10, EditMessageID: 1}

	t.Run("authoritative_reset", func(t *testing.T) {
		// Sync focused.
		h.Sync(snap1, "draft text", true, 30, 2)

		// Local edit.
		changed, val, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: 0, Text: "x"}))
		if !changed {
			t.Error("expected changed=true")
		}
		if val != "draft textx" {
			t.Errorf("expected 'draft textx', got %q", val)
		}

		// Same-identity authoritative reset replaces with exact value.
		h.Sync(snap1, "authoritative", true, 30, 2)
		if h.value != "authoritative" {
			t.Errorf("expected 'authoritative', got %q", h.value)
		}
	})

	t.Run("identity_reset", func(t *testing.T) {
		// Switching to chat 10 resets exact value and Identity.
		h.Sync(snap2, "", true, 30, 2)
		if h.Identity() != snap2 {
			t.Errorf("expected identity %v, got %v", snap2, h.Identity())
		}
		if h.value != "" {
			t.Errorf("expected empty value on identity change, got %q", h.value)
		}

		// Old text must be absent from the View.
		plain := ansi.Strip(h.View())
		if strings.Contains(plain, "authoritative") || strings.Contains(plain, "draft") {
			t.Errorf("ANSI-stripped View still contains old text: %q", plain)
		}
	})
}

// TestComposerTextHostFocusTransitions verifies that focus accepts input,
// blur ignores input, and repeated same-focus returns nil transition cmd.
func TestComposerTextHostFocusTransitions(t *testing.T) {
	h := newComposerTextHost()
	snap := composerTextIdentity{ChatID: 5, EditMessageID: 0}

	t.Run("focus_accepts", func(t *testing.T) {
		cmd1 := h.Sync(snap, "hello", true, 30, 2)
		if cmd1 == nil {
			t.Error("expected non-nil focus transition cmd")
		}
		if !h.focused {
			t.Error("expected focused=true")
		}

		// Focused input should change value.
		changed, _, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		if !changed {
			t.Error("expected changed=true while focused")
		}
		if h.value != "hell" {
			t.Errorf("expected 'hell', got %q", h.value)
		}
	})

	t.Run("blur_ignores", func(t *testing.T) {
		// Blur transition: Huh Blur() returns nil (no delayed cmd).
		cmd2 := h.Sync(snap, h.value, false, 30, 2)
		if cmd2 != nil {
			t.Logf("blur returned cmd (Huh Blur() returns nil): %v", cmd2)
		}
		if h.focused {
			t.Error("expected focused=false after blur Sync")
		}

		// Blurred input should NOT change value.
		changed, _, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: 0, Text: "x"}))
		if changed {
			t.Error("expected changed=false while blurred")
		}

		changed, _, _ = h.Update(tea.PasteMsg{Content: "test"})
		if changed {
			t.Error("expected changed=false on paste while blurred")
		}
	})

	t.Run("repeated_same_focus_nil_cmd", func(t *testing.T) {
		// Same blur state: should return nil transition cmd.
		cmd := h.Sync(snap, h.value, false, 30, 2)
		if cmd != nil {
			t.Errorf("expected nil cmd on repeated same-focus state, got %v", cmd)
		}
		if h.value != "hell" {
			t.Errorf("expected value preserved, got %q", h.value)
		}

		// Re-sync to true so the next sync is truly same-state.
		_ = h.Sync(snap, h.value, true, 30, 2)
		// Same focused state: nil cmd, value preserved.
		cmd = h.Sync(snap, h.value, true, 30, 2)
		if cmd != nil {
			t.Errorf("expected nil cmd on repeated focused state, got %v", cmd)
		}
		if h.value != "hell" {
			t.Errorf("expected value preserved, got %q", h.value)
		}
	})
}

// TestComposerTextHostEnterAndShiftEnter verifies composer Enter/Shift-Enter
// semantics: plain Enter does not mutate value, Shift-Enter inserts newline,
// and Ctrl-E does not launch an external editor.
func TestComposerTextHostEnterAndShiftEnter(t *testing.T) {
	h := newComposerTextHost()
	snap := composerTextIdentity{ChatID: 1, EditMessageID: 0}

	// Start focused and enter some text.
	h.Sync(snap, "line1", true, 30, 2)
	t.Run("shift_enter_inserts_newline", func(t *testing.T) {
		changed, val, _ := h.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}))
		if !changed {
			t.Error("expected changed=true on shift+enter")
		}
		if val != "line1\n" {
			t.Errorf("expected 'line1\\n', got %q", val)
		}
	})

	t.Run("plain_enter_unchanged", func(t *testing.T) {
		before := h.value
		changed, val, cmd := h.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

		if changed {
			t.Error("expected changed=false on plain enter")
		}
		if val != before {
			t.Errorf("expected value unchanged: %q == %q", before, val)
		}
		if cmd != nil {
			// Invoke the cmd and verify it does NOT produce Huh field-navigation.
			// Since nextFieldMsg/prevFieldMsg are private, we compare against
			// the public NextField()/PrevField() commands.
			msg := cmd()
			if msg == huh.NextField() {
				t.Error("plain Enter produced NextField")
			}
			if msg == huh.PrevField() {
				t.Error("plain Enter produced PrevField")
			}
		}
	})

	t.Run("ctrl_e_no_editor", func(t *testing.T) {
		changed, val, cmd := h.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Mod: tea.ModCtrl}))
		if changed {
			t.Error("expected changed=false on ctrl+e")
		}
		if val != "line1\n" {
			t.Errorf("expected value unchanged, got %q", val)
		}
		if cmd != nil {
			t.Fatal("ctrl+e produced a command with external editor disabled")
		}
	})
}

// TestComposerTextHostRenderedGeometry verifies that View() respects the
// Sync width/height dimensions.
func TestComposerTextHostRenderedGeometry(t *testing.T) {
	h := newComposerTextHost()
	longContent := "你好世界——这是一个很长的unicode字符串" // long unicode string

	// Sync with width 20, height 2.
	h.Sync(composerTextIdentity{ChatID: 1, EditMessageID: 0}, longContent, true, 20, 2)

	view := h.View()

	w := lipgloss.Width(view)
	if w > 20 {
		t.Errorf("expected width <= 20, got %d (view: %q)", w, view)
	}

	hh := lipgloss.Height(view)
	if hh != 2 {
		t.Errorf("expected height == 2, got %d (view: %q)", hh, view)
	}
}

// TestComposerTextHostTopicIdentitySwitch verifies that switching the active
// topic (same chat, different TopicID) is an identity change and resets host
// content to the topic-routed authoritative value.
func TestComposerTextHostTopicIdentitySwitch(t *testing.T) {
	h := newComposerTextHost()

	before := composerTextIdentity{ChatID: 9, TopicID: 0}
	h.Sync(before, "chat draft", true, 30, 2)
	if h.value != "chat draft" {
		t.Fatalf("value = %q, want %q", h.value, "chat draft")
	}

	// Edit target on the chat draft (identity adds EditMessageID).
	edit := composerTextIdentity{ChatID: 9, EditMessageID: 22}
	h.Sync(edit, "edit buffer", true, 30, 2)
	if h.Identity() != edit {
		t.Fatalf("identity = %#v, want %#v", h.Identity(), edit)
	}
	if h.value != "edit buffer" {
		t.Fatalf("value = %q, want %q", h.value, "edit buffer")
	}

	// Topic switch: same chat, new TopicID resets content entirely.
	topic := composerTextIdentity{ChatID: 9, TopicID: 5}
	h.Sync(topic, "topic draft", true, 30, 2)
	if h.Identity() != topic {
		t.Fatalf("identity = %#v, want %#v", h.Identity(), topic)
	}
	if h.value != "topic draft" {
		t.Fatalf("value = %q, want %q (topic identity did not reset)", h.value, "topic draft")
	}
}
