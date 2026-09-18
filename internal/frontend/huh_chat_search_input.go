package frontend

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// chatSearchInputHost owns the transient cursor mechanics for the global chat search.
type chatSearchInputHost struct {
	field   *huh.Input
	value   string
	focused bool
	width   int
}

func newChatSearchInputHost() *chatSearchInputHost {
	value := ""
	km := huhKeyMap()
	km.Input.Next.SetKeys()
	km.Input.Prev.SetKeys()
	km.Input.Submit.SetKeys()
	field := huh.NewInput().Value(&value).WithTheme(huhTheme()).WithKeyMap(km).(*huh.Input)
	return &chatSearchInputHost{field: field}
}

func (h *chatSearchInputHost) Sync(value string, focused bool, width int) tea.Cmd {
	width = max(1, width)
	if h.value == value && h.focused == focused && h.width == width {
		return nil
	}
	h.value = value
	h.field.Value(&h.value)
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

func (h *chatSearchInputHost) Update(msg tea.Msg) (bool, string, tea.Cmd) {
	before := h.value
	updated, cmd := h.field.Update(msg)
	if field, ok := updated.(*huh.Input); ok {
		h.field = field
	}
	return h.value != before, h.value, cmd
}

func (h *chatSearchInputHost) View() string {
	return h.field.View()
}
