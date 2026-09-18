package frontend

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// chatSettingsInputHost owns the cursor for the current title or description editor.
// The app owns the value and assigns a fresh editor ID each time editing starts.
type chatSettingsInputHost struct {
	field    *huh.Input
	value    string
	identity uint64
	focused  bool
	width    int
}

func newChatSettingsInputHost() *chatSettingsInputHost {
	value := ""
	keymap := huhKeyMap()
	keymap.Input.Next.SetKeys()
	keymap.Input.Prev.SetKeys()
	keymap.Input.Submit.SetKeys()
	field := huh.NewInput().Value(&value).WithTheme(huhTheme()).WithKeyMap(keymap).(*huh.Input)
	return &chatSettingsInputHost{field: field}
}

func (h *chatSettingsInputHost) Sync(identity uint64, value string, focused bool, width int) tea.Cmd {
	width = max(1, width)
	if h.identity == identity && h.value == value && h.focused == focused && h.width == width {
		return nil
	}
	if h.identity != identity || h.value != value {
		h.identity = identity
		h.value = value
		h.field.Value(&h.value)
	}
	var cmd tea.Cmd
	if focused && !h.focused {
		cmd = h.field.Focus()
	} else if !focused && h.focused {
		cmd = h.field.Blur()
	}
	h.focused = focused
	if h.width != width {
		h.width = width
		h.field.WithWidth(width)
	}
	return cmd
}

func (h *chatSettingsInputHost) Update(msg tea.Msg) (bool, string, tea.Cmd) {
	before := h.value
	updated, cmd := h.field.Update(msg)
	if field, ok := updated.(*huh.Input); ok {
		h.field = field
	}
	return h.value != before, h.value, cmd
}

func (h *chatSettingsInputHost) View() string     { return h.field.View() }
func (h *chatSettingsInputHost) Identity() uint64 { return h.identity }
func (h *chatSettingsInputHost) Value() string    { return h.value }
