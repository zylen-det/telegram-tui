package frontend

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestChatActionInputAndSelectorOptions(t *testing.T) {
	for _, key := range []tea.Key{{Code: tea.KeyDown}, {Text: "j", Code: 'j'}} {
		got, ok := mapKeyPress(app.FocusChatActions, tea.KeyPressMsg(key))
		if !ok || got.Action != app.SelectNext {
			t.Fatalf("down key %#v = %#v, %t", key, got, ok)
		}
	}
	if got, ok := mapKeyPress(app.FocusChatActions, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); !ok || got.Action != app.Activate {
		t.Fatalf("enter = %#v, %t", got, ok)
	}
	menu := &app.ChatActionMenuState{ChatID: 9}
	chat := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, IsMember: true}
	options := selectorOptionsFromRows(chatActionRows(chat, menu))
	if len(options) < 2 || options[0].Label != "Open chat" || options[0].Value != (app.ActionReceived{Action: app.OpenChatFromMenu, ChatID: 9}) {
		t.Fatalf("options = %#v", options)
	}
}

func TestChatActionLayerOwnsModalInteractions(t *testing.T) {
	model := ui.ViewModel{
		Width: 80, Height: 24,
		Layout:      ui.ComputeLayout(80, 24, false, app.FocusChatActions),
		Focus:       app.FocusChatActions,
		ActiveChat:  domain.Chat{ID: 9, Title: "Team", Kind: domain.ChatSupergroup, IsMember: true},
		ChatActions: &app.ChatActionMenuState{ChatID: 9},
	}
	layer := buildChatActionLayer(model, newRenderStyles(false), "")
	if layer.Layer == nil || !layer.IsModal || len(layer.Interactions) == 0 {
		t.Fatalf("layer = %#v", layer)
	}
	found := false
	for _, interaction := range layer.Interactions {
		if interaction.Click.Action == app.OpenChatFromMenu && interaction.Click.ChatID == 9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing open action in %#v", layer.Interactions)
	}
}
