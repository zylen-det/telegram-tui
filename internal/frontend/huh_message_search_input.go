package frontend

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

// messageSearchInputHost owns transient cursor mechanics for the search query.
type messageSearchInputHost struct {
	field    *huh.Input
	value    string
	identity domain.ChatID
	focused  bool
	width    int
}

func newMessageSearchInputHost() *messageSearchInputHost {
	value := ""
	km := huhKeyMap()
	km.Input.Next.SetKeys()
	km.Input.Prev.SetKeys()
	km.Input.Submit.SetKeys()
	field := huh.NewInput().Value(&value).WithTheme(huhTheme()).WithKeyMap(km).(*huh.Input)
	return &messageSearchInputHost{field: field}
}

func (h *messageSearchInputHost) Sync(chatID domain.ChatID, value string, focused bool, width int) tea.Cmd {
	width = max(1, width)
	if h.identity == chatID && h.value == value && h.focused == focused && h.width == width {
		return nil
	}
	if h.identity != chatID || h.value != value {
		h.identity = chatID
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

func (h *messageSearchInputHost) Update(msg tea.Msg) (bool, string, tea.Cmd) {
	before := h.value
	updated, cmd := h.field.Update(msg)
	if field, ok := updated.(*huh.Input); ok {
		h.field = field
	}
	return h.value != before, h.value, cmd
}

func (h *messageSearchInputHost) View() string            { return h.field.View() }
func (h *messageSearchInputHost) Identity() domain.ChatID { return h.identity }
