package frontend

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestChatActionInputAndSelectorOptions(t *testing.T) {
	for _, key := range []tea.Key{{Code: tea.KeyDown}, {Text: "j", Code: 'j'}} {
		got, ok := mapKeyPress(FocusChatActions, tea.KeyPressMsg(key))
		if !ok || got.Action != SelectNext {
			t.Fatalf("down key %#v = %#v, %t", key, got, ok)
		}
	}
	if got, ok := mapKeyPress(FocusChatActions, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); !ok || got.Action != Activate {
		t.Fatalf("enter = %#v, %t", got, ok)
	}
	menu := &ChatActionMenuState{ChatID: 9}
	chat := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, IsMember: true}
	options := selectorOptionsFromRows(chatActionRows(chat, menu))
	if len(options) < 2 || options[0].Label != "Open chat" || options[0].Value != (ActionReceived{Action: OpenChatFromMenu, ChatID: 9}) {
		t.Fatalf("options = %#v", options)
	}
}

func TestChatActionLayerOwnsModalInteractions(t *testing.T) {
	model := ViewModel{
		Width: 80, Height: 24,
		Layout:      ComputeLayout(80, 24, false, FocusChatActions),
		Focus:       FocusChatActions,
		ActiveChat:  domain.Chat{ID: 9, Title: "Team", Kind: domain.ChatSupergroup, IsMember: true},
		ChatActions: &ChatActionMenuState{ChatID: 9},
	}
	layer := buildChatActionLayer(model, newRenderStyles(false), "")
	if layer.Layer == nil || !layer.IsModal || len(layer.Interactions) == 0 {
		t.Fatalf("layer = %#v", layer)
	}
	found := false
	for _, interaction := range layer.Interactions {
		if interaction.Click.Action == OpenChatFromMenu && interaction.Click.ChatID == 9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing open action in %#v", layer.Interactions)
	}
}
