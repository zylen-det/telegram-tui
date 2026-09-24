package frontend

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAppModelCommandMenuKeepsTypingInHuhComposerAndCompletes(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Layout = LayoutNormal
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusComposer
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatPrivate, Title: "Bot", CanSend: true}}
	state.BotCommandCatalogs[9] = BotCommandCatalogState{Loaded: true, Commands: []domain.BotCommand{{Name: "help"}, {Name: "hello"}}}
	model := newAppModelForTest(t, state, newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "/"}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "h"}))
	snapshot := model.Snapshot()
	if snapshot.Focus != FocusComposer || snapshot.Drafts[9] != "/h" || snapshot.CommandMenu == nil {
		t.Fatalf("typed menu state = focus:%v draft:%q menu:%#v", snapshot.Focus, snapshot.Drafts[9], snapshot.CommandMenu)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	snapshot = model.Snapshot()
	if snapshot.CommandMenu != nil || snapshot.Drafts[9] != "/help " || model.composerText.Value() != "/help " {
		t.Fatalf("completed menu state = draft:%q host:%q menu:%#v", snapshot.Drafts[9], model.composerText.Value(), snapshot.CommandMenu)
	}
}

func TestAppModelSlashDeliversBotCommandLoad(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = FocusComposer
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}

	effects := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	found := false
	for _, effect := range effects {
		if load, ok := effect.(LoadBotCommands); ok {
			if load.ChatID != 9 {
				t.Fatalf("LoadBotCommands chat ID = %d, want 9", load.ChatID)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("slash input effects = %#v, want LoadBotCommands", effects)
	}

	model := newAppModelForTest(t, state, newTestSession(t))
	if _, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "/"})); cmd == nil {
		t.Fatal("slash input returned no command")
	}
}
func TestCommandMenuKeyMappingOnlyInterceptsCompletionKeys(t *testing.T) {
	cases := []struct {
		key  tea.Key
		want Action
		ok   bool
	}{
		{tea.Key{Code: tea.KeyUp}, CommandMenuPrevious, true},
		{tea.Key{Code: tea.KeyDown}, CommandMenuNext, true},
		{tea.Key{Code: tea.KeyEnter}, CommandMenuActivate, true},
		{tea.Key{Code: tea.KeyEscape}, CommandMenuDismiss, true},
		{tea.Key{Text: "x"}, NoAction, false},
		{tea.Key{Code: 'c', Mod: tea.ModCtrl}, NoAction, false},
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
	model.Focus = FocusComposer
	model.CommandMenu = &CommandMenuState{
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
	var startHit *Hit
	for index := range frame.Hits {
		hit := &frame.Hits[index]
		if hit.Click.Action == CommandMenuActivate && hit.Click.CommandIndex == 1 {
			startHit = hit
			break
		}
	}
	if startHit == nil {
		t.Fatal("command row click hit missing")
	}
	got, ok := frame.Hits.ActionAt(startHit.Rect.Min.X, startHit.Rect.Min.Y)
	if !ok || got.Action != CommandMenuActivate || got.ChatID != 2 || got.CommandIndex != 1 {
		t.Fatalf("topmost row action = %#v, %t", got, ok)
	}
}

func TestCommandMenuGeometryIsBoundedAtSupportedMinimum(t *testing.T) {
	model := frameBaseModel(60, 18)
	model.Focus = FocusComposer
	commands := make([]domain.BotCommand, 12)
	for index := range commands {
		commands[index] = domain.BotCommand{Name: "command"}
	}
	model.CommandMenu = &CommandMenuState{ChatID: 2, Candidates: commands, Selected: 9, First: 2}
	composer := composerSurfaceRect(model)
	history := image.Rect(model.Layout.Conversation.Min.X+1, model.Layout.Conversation.Min.Y+1, model.Layout.Conversation.Max.X-1, composer.Min.Y)
	surface := buildCommandMenuLayer(model, history, composer, newRenderStyles(false))
	viewport := image.Rect(0, 0, model.Width, model.Height)
	if surface.Layer == nil || !surface.Rect.In(viewport) || surface.Rect.Dy() > CommandMenuVisibleRows+2 {
		t.Fatalf("bounded command menu = %v viewport=%v", surface.Rect, viewport)
	}
}
