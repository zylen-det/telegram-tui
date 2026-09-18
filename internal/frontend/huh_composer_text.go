package frontend

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

// composerTextIdentity uniquely identifies the composer target.
type composerTextIdentity struct {
	ChatID        domain.ChatID
	EditMessageID domain.MessageID
	TopicID       domain.TopicID
}

// composerTextHost owns transient Huh editing/cursor state and synchronizes
// an authoritative (ChatID, EditMessageID, Value, Focus, Width, Height)
// snapshot.
type composerTextHost struct {
	field    *huh.Text
	value    string
	identity composerTextIdentity
	focused  bool
	width    int
	height   int
}

// newComposerTextHost creates a fresh host with an actual huh.NewText() bound
// to &h.value, themed, and keyed for composer semantics.
func newComposerTextHost() *composerTextHost {
	var value string
	theme := huhTheme()
	km := huhKeyMap()

	// Configure composer-key semantics BEFORE WithKeyMap (keymap is copied as a
	// struct during WithKeyMap, so post-call modifications would be lost):
	//   - NewLine is exactly shift+enter only
	//   - Next, Prev, Submit bindings are disabled
	//   - Editor binding is disabled
	km.Text.NewLine.SetKeys("shift+enter")
	km.Text.Next.SetKeys()
	km.Text.Prev.SetKeys()
	km.Text.Submit.SetKeys()
	km.Text.Editor.SetKeys()

	// Build the text field: bind value, apply theme, apply keymap.
	t := huh.NewText().Value(&value).WithTheme(theme).WithKeyMap(km).(*huh.Text)
	t.ExternalEditor(false)

	return &composerTextHost{
		field: t,
		value: value,
	}
}

// Sync synchronizes the host with an authoritative snapshot.
// Clamps width/height to at least 1. When identity or value differs, rebinding
// through the public Huh Value API resets the internal textarea to
// authoritative content. Focus transitions call Focus()/Blur() once;
// identity change never retains previous chat/edit text.
func (h *composerTextHost) Sync(identity composerTextIdentity, value string, focused bool, width, height int) tea.Cmd {
	// Clamp dimensions.
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	// Fast path: nothing changed — avoid expensive WithWidth/Height and
	// textarea reflow that would otherwise run on every keystroke (long-press
	// hot path at 30+ Hz).
	if h.identity == identity && h.value == value && h.focused == focused && h.width == width && h.height == height {
		return nil
	}

	// Determine if identity changed.
	identityChanged := h.identity != identity

	// When identity or authoritative value differs, rebind through Value API.
	if identityChanged || value != h.value {
		h.identity = identity
		h.value = value
		// Rebind through the public Huh Value API — resets internal textarea.
		h.field.Value(&h.value)
	}

	// Focus transition handling.
	var cmd tea.Cmd
	if focused && !h.focused {
		// Focus transition: true ← false
		cmd = h.field.Focus()
	} else if !focused && h.focused {
		// Blur transition: false ← true
		cmd = h.field.Blur()
	}
	// Same focus state: no transition command returned.

	// Only apply geometry when it actually changed — WithWidth/Height
	// triggers textarea viewport recomputation and is the dominant cost
	// on the long-press hot path.
	geometryChanged := h.width != width || h.height != height
	h.focused = focused
	h.width = width
	h.height = height

	if geometryChanged {
		h.field.WithWidth(width).WithHeight(height)
	}

	return cmd
}

// Update processes a Bubble Tea message against the embedded Huh Text field.
// Returns changed=true only when the complete bound value differs after the
// update.  Returns the current value and any Huh command.
func (h *composerTextHost) Update(msg tea.Msg) (changed bool, value string, cmd tea.Cmd) {
	before := h.value

	result, fieldCmd := h.field.Update(msg)
	// Type-assert to *huh.Text for the next iteration.
	if t, ok := result.(*huh.Text); ok {
		h.field = t
	}

	changed = h.value != before
	return changed, h.value, fieldCmd
}

// View renders the composer host.
func (h *composerTextHost) View() string {
	return h.field.View()
}

// Identity returns the current composer identity.
func (h *composerTextHost) Identity() composerTextIdentity {
	return h.identity
}

// Value returns the current authoritative text value.
func (h *composerTextHost) Value() string {
	return h.value
}
