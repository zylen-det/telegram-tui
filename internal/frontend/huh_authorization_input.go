package frontend

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// authorizationInputHost owns transient Huh editing/cursor state for
// authorization prompt fields (API token, phone, code, password). It
// synchronizes an authoritative identity/value/focus/secret/width snapshot
// while the embedded *huh.Input handles cursor positioning and character
// editing.
type authorizationInputHost struct {
	field    *huh.Input
	value    string
	identity uint64
	focused  bool
	secret   bool
	width    int
}

// newAuthorizationInputHost creates a fresh host with an actual
// huh.NewInput() bound to &h.value, themed, and keyed for authorization
// semantics. Enter/Tab/Shift-Tab are left AppModel-owned.
func newAuthorizationInputHost() *authorizationInputHost {
	var value string
	theme := huhTheme()
	km := huhKeyMap()

	// Configure input-key semantics BEFORE WithKeyMap (keymap is copied as a
	// struct during WithKeyMap, so post-call modifications would be lost):
	//   - Next, Prev, Submit bindings are disabled so AppModel owns
	//     Enter/Tab/Shift-Tab navigation.
	km.Input.Next.SetKeys()
	km.Input.Prev.SetKeys()
	km.Input.Submit.SetKeys()

	// Build the input field: bind value, apply theme, apply keymap.
	i := huh.NewInput().Value(&value).WithTheme(theme).WithKeyMap(km).(*huh.Input)

	return &authorizationInputHost{
		field: i,
		value: value,
	}
}

// Sync synchronizes the host with an authoritative snapshot.
// Clamps width to at least 1. When identity or authoritative value differs,
// rebinds through the public Huh Value API so a new PromptID cannot retain
// old content or cursor. Every sync sets echo mode explicitly so both
// secret↔non-secret transitions are supported. Focus/blur transitions call
// Focus()/Blur() exactly once.
func (h *authorizationInputHost) Sync(promptID uint64, value string, focused bool, secret bool, width int) tea.Cmd {
	// Clamp width to at least 1.
	if width < 1 {
		width = 1
	}

	// Fast path: nothing changed.
	if h.identity == promptID && h.value == value && h.focused == focused && h.secret == secret && h.width == width {
		return nil
	}

	// Determine if identity or authoritative value changed.
	identityChanged := h.identity != promptID
	valueChanged := value != h.value

	// When identity or authoritative value differs, rebind through Value API.
	if identityChanged || valueChanged {
		h.identity = promptID
		h.value = value
		// Rebind through the public Huh Value API — resets internal state.
		h.field.Value(&h.value)
	}

	// Echo mode: only set when changed — avoids internal recompute.
	if h.secret != secret {
		if secret {
			h.field.EchoMode(huh.EchoModePassword)
		} else {
			h.field.EchoMode(huh.EchoModeNormal)
		}
	}

	// Focus transition handling.
	var cmd tea.Cmd
	if focused && !h.focused {
		// Focus transition: false → true
		cmd = h.field.Focus()
	} else if !focused && h.focused {
		// Blur transition: true → false.
		cmd = h.field.Blur()
	}
	// Same focus state: no transition command returned.

	geometryChanged := h.width != width
	h.focused = focused
	h.secret = secret
	h.width = width

	if geometryChanged {
		h.field.WithWidth(width)
	}

	return cmd
}

// Update processes a Bubble Tea message against the embedded Huh Input field.
// Returns changed=true only when the complete bound value differs after the
// update. Reassigns the concrete *huh.Input field for the next iteration.
func (h *authorizationInputHost) Update(msg tea.Msg) (changed bool, value string, cmd tea.Cmd) {
	before := h.value

	result, fieldCmd := h.field.Update(msg)
	// Type-assert to *huh.Input for the next iteration.
	if i, ok := result.(*huh.Input); ok {
		h.field = i
	}

	changed = h.value != before
	return changed, h.value, fieldCmd
}

// View renders the authorization input host.
func (h *authorizationInputHost) View() string {
	return h.field.View()
}

// Identity returns the current PromptID identity.
func (h *authorizationInputHost) Identity() uint64 {
	return h.identity
}

// Value returns the current authoritative text value.
func (h *authorizationInputHost) Value() string {
	return h.value
}
