package frontend

import (
	"image"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestAppModelHuhComposerSynchronizesReservedTransitions(t *testing.T) {
	t.Run("Ctrl-O blurs the composer host", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusComposer
		state.Drafts[9] = "keep"
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		_ = model.syncComposerTextHost()

		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o", Mod: tea.ModCtrl}))
		if got := engine.Snapshot().Focus; got != app.FocusPhotoSend {
			t.Fatalf("focus = %v, want FocusPhotoSend", got)
		}
		assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "keep", false, 1, 1)
	})

	t.Run("Escape leaves edit identity for the authoritative draft", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusComposer
		state.Drafts[9] = "draft"
		state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "old"}}
		state.EditTarget = &app.EditTarget{ChatID: 9, MessageID: 22, Original: "old", Buffer: "edit"}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		_ = model.syncComposerTextHost()

		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		if engine.Snapshot().EditTarget != nil {
			t.Fatal("Escape did not clear authoritative EditTarget")
		}
		assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 1, 1)
	})
}

func TestAppModelHuhComposerSynchronizesMouseFocusTransition(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = app.FocusConversation
	state.Layout = app.LayoutWide
	state.Width = 100
	state.Height = 30
	state.Drafts[9] = "draft"
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	_ = model.syncComposerTextHost()
	model.setHitRegions(ui.HitMap{{
		Rect:  image.Rect(2, 3, 8, 4),
		Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusComposer},
	}})

	model, _ = updateAppModel(t, model, tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft})
	if got := engine.Snapshot().Focus; got != app.FocusComposer {
		t.Fatalf("focus = %v, want FocusComposer", got)
	}
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 42, 2)
}

func TestAppModelHuhComposerNeverDiscardsSyncCommands(t *testing.T) {
	source, err := os.ReadFile("app_model.go")
	if err != nil {
		t.Fatalf("read app_model.go: %v", err)
	}
	if strings.Contains(string(source), "_ = m.syncComposerTextHost()") {
		t.Fatal("AppModel.Update discards a composer host synchronization command")
	}
}
