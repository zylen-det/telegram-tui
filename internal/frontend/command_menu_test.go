package frontend

import (
	"context"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestAppModelCommandMenuKeepsTypingInHuhComposerAndCompletes(t *testing.T) {
	state := app.InitialState()
	state.Width, state.Height = 100, 24
	state.Layout = app.LayoutNormal
	state.Connection = domain.ConnectionOnline
	state.Focus = app.FocusComposer
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatPrivate, Title: "Bot", CanSend: true}}
	state.BotCommandCatalogs[9] = app.BotCommandCatalogState{Loaded: true, Commands: []domain.BotCommand{{Name: "help"}, {Name: "hello"}}}
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "/"}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "h"}))
	snapshot := engine.Snapshot()
	if snapshot.Focus != app.FocusComposer || snapshot.Drafts[9] != "/h" || snapshot.CommandMenu == nil {
		t.Fatalf("typed menu state = focus:%v draft:%q menu:%#v", snapshot.Focus, snapshot.Drafts[9], snapshot.CommandMenu)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	snapshot = engine.Snapshot()
	if snapshot.CommandMenu != nil || snapshot.Drafts[9] != "/help " || model.composerText.Value() != "/help " {
		t.Fatalf("completed menu state = draft:%q host:%q menu:%#v", snapshot.Drafts[9], model.composerText.Value(), snapshot.CommandMenu)
	}
}

func TestAppModelSlashDeliversBotCommandLoad(t *testing.T) {
	state := app.InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = app.FocusComposer
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &AppRuntime{ctx: ctx, cancel: cancel, commands: make(chan app.Command, 2)}
	model := newAppModelForTest(t, app.NewEngine(state), runtime)
	_, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "/"}))
	if cmd == nil {
		t.Fatal("slash input returned no command delivery")
	}
	message := cmd()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("slash command result = %T, want tea.BatchMsg", message)
	}
	for _, child := range batch {
		if child != nil {
			_ = child()
		}
	}
	select {
	case command := <-runtime.commands:
		load, ok := command.(app.LoadBotCommands)
		if !ok || load.ChatID != 9 {
			t.Fatalf("delivered command = %#v", command)
		}
	default:
		t.Fatal("slash input did not enqueue LoadBotCommands")
	}
}

func TestCommandMenuKeyMappingOnlyInterceptsCompletionKeys(t *testing.T) {
	cases := []struct {
		key  tea.Key
		want app.Action
		ok   bool
	}{
		{tea.Key{Code: tea.KeyUp}, app.CommandMenuPrevious, true},
		{tea.Key{Code: tea.KeyDown}, app.CommandMenuNext, true},
		{tea.Key{Code: tea.KeyEnter}, app.CommandMenuActivate, true},
		{tea.Key{Code: tea.KeyEscape}, app.CommandMenuDismiss, true},
		{tea.Key{Text: "x"}, app.NoAction, false},
		{tea.Key{Code: 'c', Mod: tea.ModCtrl}, app.NoAction, false},
	}
	for _, test := range cases {
		got, ok := mapCommandMenuKey(true, tea.KeyPressMsg(test.key))
		if ok != test.ok || got.Action != test.want {
			t.Errorf("key %#v = (%v,%t), want (%v,%t)", test.key, got.Action, ok, test.want, test.ok)
		}
	}
	if _, ok := mapCommandMenuKey(false, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); ok {
		t.Fatal("inactive command menu intercepted Enter")
	}
}

func TestCommandMenuRendersAboveComposerWithTopmostMouseRows(t *testing.T) {
	model := frameBaseModel(100, 24)
	model.Focus = app.FocusComposer
	model.CommandMenu = &app.CommandMenuState{
		ChatID: 2, Selected: 1,
		Candidates: []domain.BotCommand{{Name: "help", Description: "Show help"}, {Name: "start", Description: "Start bot"}},
	}
	composer := composerSurfaceRect(model)
	history := image.Rect(model.Layout.Conversation.Min.X+1, model.Layout.Conversation.Min.Y+1, model.Layout.Conversation.Max.X-1, composer.Min.Y)
	surface := buildCommandMenuLayer(model, history, composer, newRenderStyles(false))
	if surface.Layer == nil || surface.Rect.Max.Y != composer.Min.Y || surface.Rect.Min.Y >= composer.Min.Y {
		t.Fatalf("command menu geometry = %v composer=%v", surface.Rect, composer)
	}

	frame := composeApplication(model, nil)
	plain := ansi.Strip(frame.Content)
	for _, text := range []string{"Commands", "/help", "Show help", "/start", "Start bot"} {
		if !strings.Contains(plain, text) {
			t.Errorf("frame missing %q", text)
		}
	}
	var startHit *ui.Hit
	for index := range frame.Hits {
		hit := &frame.Hits[index]
		if hit.Click.Action == app.CommandMenuActivate && hit.Click.CommandIndex == 1 {
			startHit = hit
			break
		}
	}
	if startHit == nil {
		t.Fatal("command row click hit missing")
	}
	got, ok := frame.Hits.ActionAt(startHit.Rect.Min.X, startHit.Rect.Min.Y)
	if !ok || got.Action != app.CommandMenuActivate || got.ChatID != 2 || got.CommandIndex != 1 {
		t.Fatalf("topmost row action = %#v, %t", got, ok)
	}
}

func TestCommandMenuGeometryIsBoundedAtSupportedMinimum(t *testing.T) {
	model := frameBaseModel(60, 18)
	model.Focus = app.FocusComposer
	commands := make([]domain.BotCommand, 12)
	for index := range commands {
		commands[index] = domain.BotCommand{Name: "command"}
	}
	model.CommandMenu = &app.CommandMenuState{ChatID: 2, Candidates: commands, Selected: 9, First: 2}
	composer := composerSurfaceRect(model)
	history := image.Rect(model.Layout.Conversation.Min.X+1, model.Layout.Conversation.Min.Y+1, model.Layout.Conversation.Max.X-1, composer.Min.Y)
	surface := buildCommandMenuLayer(model, history, composer, newRenderStyles(false))
	viewport := image.Rect(0, 0, model.Width, model.Height)
	if surface.Layer == nil || !surface.Rect.In(viewport) || surface.Rect.Dy() > app.CommandMenuVisibleRows+2 {
		t.Fatalf("bounded command menu = %v viewport=%v", surface.Rect, viewport)
	}
}
