package frontend

import (
	"image"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestSemanticInputMapsNavigationAndEditors(t *testing.T) {
	tests := []struct {
		name  string
		focus app.Focus
		key   tea.Key
		want  app.ActionReceived
	}{
		{name: "down", focus: app.FocusChats, key: tea.Key{Code: tea.KeyDown}, want: app.ActionReceived{Action: app.SelectNext}},
		{name: "j", focus: app.FocusChats, key: tea.Key{Text: "j", Code: 'j'}, want: app.ActionReceived{Action: app.SelectNext}},
		{name: "up", focus: app.FocusChats, key: tea.Key{Code: tea.KeyUp}, want: app.ActionReceived{Action: app.SelectPrevious}},
		{name: "left mirrors h", focus: app.FocusChats, key: tea.Key{Code: tea.KeyLeft}, want: app.ActionReceived{Action: app.FocusPrevious}},
		{name: "right mirrors l", focus: app.FocusChats, key: tea.Key{Code: tea.KeyRight}, want: app.ActionReceived{Action: app.FocusNext}},
		{name: "tab", focus: app.FocusChats, key: tea.Key{Code: tea.KeyTab}, want: app.ActionReceived{Action: app.FocusNext}},
		{name: "shift tab", focus: app.FocusChats, key: tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, want: app.ActionReceived{Action: app.FocusPrevious}},
		{name: "details", focus: app.FocusChats, key: tea.Key{Code: tea.KeyF2}, want: app.ActionReceived{Action: app.ToggleDetails}},
		{name: "next unread", focus: app.FocusChats, key: tea.Key{Text: "u", Code: 'u'}, want: app.ActionReceived{Action: app.SelectNextUnread}},
		{name: "next mention", focus: app.FocusChats, key: tea.Key{Text: "m", Code: 'm'}, want: app.ActionReceived{Action: app.SelectNextMention}},
		{name: "page up", focus: app.FocusConversation, key: tea.Key{Code: tea.KeyPgUp}, want: app.ActionReceived{Action: app.PageUp}},
		{name: "page down", focus: app.FocusConversation, key: tea.Key{Code: tea.KeyPgDown}, want: app.ActionReceived{Action: app.PageDown}},
		{name: "ctrl u", focus: app.FocusConversation, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl}, want: app.ActionReceived{Action: app.PageUp}},
		{name: "ctrl d", focus: app.FocusConversation, key: tea.Key{Code: 'd', Text: "d", Mod: tea.ModCtrl}, want: app.ActionReceived{Action: app.PageDown}},
		{name: "composer submit", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyEnter}, want: app.ActionReceived{Action: app.ComposerSubmit}},
		{name: "composer escape with lock state", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModNumLock}, want: app.ActionReceived{Action: app.Close}},
		{name: "composer newline", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}, want: app.ActionReceived{Action: app.ComposerNewline}},
		{name: "auth backspace", focus: app.FocusAuth, key: tea.Key{Code: tea.KeyBackspace}, want: app.ActionReceived{Action: app.ComposerBackspace}},
		{name: "modal q", focus: app.FocusModal, key: tea.Key{Text: "q", Code: 'q'}, want: app.ActionReceived{Action: app.Close}},
		{name: "quit", focus: app.FocusComposer, key: tea.Key{Code: 'c', Mod: tea.ModCtrl}, want: app.ActionReceived{Action: app.Quit}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := mapKeyPress(test.focus, tea.KeyPressMsg(test.key))
			if !ok || got != test.want {
				t.Fatalf("mapKeyPress() = (%#v, %t), want (%#v, true)", got, ok, test.want)
			}
		})
	}
}

func TestEditableModifierTextDoesNotMutateEngine(t *testing.T) {

}

func TestMessageSelectionAndActionMenuKeyboardMappings(t *testing.T) {
	for _, test := range []struct {
		name string
		key  tea.Key
		want app.Action
	}{
		{name: "next message", key: tea.Key{Code: tea.KeyDown}, want: app.SelectNextMessage},
		{name: "previous message", key: tea.Key{Code: tea.KeyUp}, want: app.SelectPreviousMessage},
		{name: "open menu", key: tea.Key{Code: tea.KeyEnter}, want: app.OpenMessageActionMenu},
		{name: "copy", key: tea.Key{Code: 'c', Text: "c"}, want: app.CopyMessage},
		{name: "edit", key: tea.Key{Code: 'e', Text: "e"}, want: app.EditMessage},
		{name: "close with q", key: tea.Key{Code: 'q', Text: "q"}, want: app.Close},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(test.key))
			if !ok || got.Action != test.want {
				t.Fatalf("mapped action = (%v, %t), want (%v, true)", got.Action, ok, test.want)
			}
		})
	}
}

func TestUnreadAndMentionShortcutsAreChatsOnlyAndUnmodified(t *testing.T) {
	for _, test := range []struct {
		focus app.Focus
		key   tea.Key
	}{
		{focus: app.FocusConversation, key: tea.Key{Text: "u", Code: 'u'}},
		{focus: app.FocusConversation, key: tea.Key{Text: "m", Code: 'm'}},
		{focus: app.FocusChats, key: tea.Key{Text: "u", Code: 'u', Mod: tea.ModAlt}},
		{focus: app.FocusChats, key: tea.Key{Text: "m", Code: 'm', Mod: tea.ModCtrl}},
		{focus: app.FocusChats, key: tea.Key{Text: "U", Code: 'U', Mod: tea.ModShift}},
		{focus: app.FocusChats, key: tea.Key{Text: "M", Code: 'M', Mod: tea.ModShift}},
	} {
		got, ok := mapKeyPress(test.focus, tea.KeyPressMsg(test.key))
		if ok && (got.Action == app.SelectNextUnread || got.Action == app.SelectNextMention) {
			t.Fatalf("focus %v key %#v unexpectedly mapped to unread navigation %v", test.focus, test.key, got.Action)
		}
	}
}

func TestDeleteMessageHasNoConversationShortcut(t *testing.T) {
	for _, key := range []tea.Key{
		{Code: 'd', Text: "d"},
		{Code: 'D', Text: "D"},
		{Code: tea.KeyDelete},
	} {
		got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(key))
		if ok && (got.Action == app.DeleteMessage || got.Action == app.DeleteForEveryone) {
			t.Fatalf("conversation key %#v unexpectedly mapped to delete action %v", key, got.Action)
		}
	}
}

func TestActionModalKeyboardListMappings(t *testing.T) {
	for _, test := range []struct {
		key  tea.Key
		want app.Action
	}{
		{tea.Key{Code: tea.KeyDown}, app.SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, app.SelectNext},
		{tea.Key{Code: tea.KeyUp}, app.SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, app.SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, app.Activate},
		{tea.Key{Code: tea.KeyEscape}, app.Close},
		{tea.Key{Code: 'q', Text: "q"}, app.Close},
	} {
		got, ok := mapKeyPress(app.FocusModal, tea.KeyPressMsg(test.key))
		if !ok || got.Action != test.want {
			t.Fatalf("modal key %#v = (%v,%t), want %v", test.key, got.Action, ok, test.want)
		}
	}
}

func TestForwardPickerKeyboardMappings(t *testing.T) {
	for _, test := range []struct {
		key  tea.Key
		want app.Action
	}{
		{tea.Key{Code: tea.KeyDown}, app.SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, app.SelectNext},
		{tea.Key{Code: tea.KeyUp}, app.SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, app.SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, app.Activate},
		{tea.Key{Code: tea.KeyEscape}, app.Close},
	} {
		got, ok := mapKeyPress(app.FocusForwardPicker, tea.KeyPressMsg(test.key))
		if !ok || got.Action != test.want {
			t.Fatalf("picker key %#v = (%v,%t), want %v", test.key, got.Action, ok, test.want)
		}
	}
	for _, reserved := range []tea.Key{
		{Code: 'h', Text: "h"},
		{Code: tea.KeyLeft},
		{Code: 'l', Text: "l"},
		{Code: tea.KeyRight},
		{Code: 'q', Text: "q"},
		{Code: tea.KeyTab},
	} {
		if got, ok := mapKeyPress(app.FocusForwardPicker, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved picker key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestArrowKeysMirrorHJKLByFocus(t *testing.T) {
	tests := []struct {
		focus app.Focus
		arrow tea.Key
		vim   tea.Key
	}{
		{app.FocusChats, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{app.FocusChats, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{app.FocusChats, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{app.FocusChats, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{app.FocusConversation, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{app.FocusConversation, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{app.FocusConversation, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{app.FocusConversation, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{app.FocusModal, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{app.FocusModal, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{app.FocusForwardPicker, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{app.FocusForwardPicker, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{app.FocusForwardPicker, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{app.FocusForwardPicker, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{app.FocusReactionPicker, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{app.FocusReactionPicker, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{app.FocusReactionPicker, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{app.FocusReactionPicker, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
	}
	for _, test := range tests {
		arrow, arrowOK := mapKeyPress(test.focus, tea.KeyPressMsg(test.arrow))
		vim, vimOK := mapKeyPress(test.focus, tea.KeyPressMsg(test.vim))
		if arrowOK != vimOK || arrow != vim {
			t.Fatalf("focus %v arrow %#v = (%#v,%t), vim %#v = (%#v,%t)", test.focus, test.arrow, arrow, arrowOK, test.vim, vim, vimOK)
		}
	}
}

func TestReactionPickerKeyboardMappings(t *testing.T) {
	for _, test := range []struct {
		key  tea.Key
		want app.Action
	}{
		{tea.Key{Code: tea.KeyDown}, app.SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, app.SelectNext},
		{tea.Key{Code: tea.KeyUp}, app.SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, app.SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, app.Activate},
		{tea.Key{Code: tea.KeyEscape}, app.Close},
	} {
		got, ok := mapKeyPress(app.FocusReactionPicker, tea.KeyPressMsg(test.key))
		if !ok || got.Action != test.want {
			t.Fatalf("reaction picker key %#v = (%v,%t), want %v", test.key, got.Action, ok, test.want)
		}
	}
	for _, reserved := range []tea.Key{
		{Code: 'h', Text: "h"},
		{Code: tea.KeyLeft},
		{Code: 'l', Text: "l"},
		{Code: tea.KeyRight},
		{Code: 'q', Text: "q"},
		{Code: tea.KeyTab},
		{Code: 'j', Text: "j", Mod: tea.ModAlt},
	} {
		if got, ok := mapKeyPress(app.FocusReactionPicker, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved reaction picker key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestEditableModifiersTextDoesNotMutateEngineContinuation(t *testing.T) {
	for _, test := range []struct {
		name  string
		focus app.Focus
		key   tea.Key
	}{
		{name: "auth alt text", focus: app.FocusAuth, key: tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}},
		{name: "auth ctrl text", focus: app.FocusAuth, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl}},
		{name: "composer meta text", focus: app.FocusComposer, key: tea.Key{Code: 'q', Text: "q", Mod: tea.ModMeta}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := app.InitialState()
			state.Focus = test.focus
			if test.focus == app.FocusAuth {
				state.Prompt = &app.PromptState{}
			} else {
				state.Chats = []domain.Chat{{ID: 7, CanSend: true}}
				state.Drafts[7] = "before"
			}
			model := newAppModelForTest(t, app.NewEngine(state), newBoundedAppRuntimeForModelTest(t))
			before := model.engine.Snapshot()
			_, cmd := model.Update(tea.KeyPressMsg(test.key))
			if cmd != nil {
				t.Fatal("modified text key returned a command")
			}
			after := model.engine.Snapshot()
			if test.focus == app.FocusAuth {
				if len(after.Prompt.Input) != len(before.Prompt.Input) {
					t.Fatal("modified text key changed authorization input")
				}
			} else if after.Drafts[7] != before.Drafts[7] {
				t.Fatal("modified text key changed composer draft")
			}
		})
	}
}

func TestMouseHitMapsClickAndWheel(t *testing.T) {
	hits := ui.HitMap{{
		Rect:      image.Rect(2, 3, 10, 12),
		Click:     app.ActionReceived{Action: app.SelectChat, ChatID: 42},
		WheelUp:   app.ActionReceived{Action: app.PageUp},
		WheelDown: app.ActionReceived{Action: app.PageDown},
	}}

	click, ok := mapMouseClick(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft}, hits)
	if !ok || click.Action != app.SelectChat || click.ChatID != 42 {
		t.Fatalf("click mapping = (%#v, %t)", click, ok)
	}
	up, ok := mapMouseWheel(tea.MouseWheelMsg{X: 4, Y: 5, Button: tea.MouseWheelUp}, hits)
	if !ok || up.Action != app.PageUp {
		t.Fatalf("wheel-up mapping = (%#v, %t)", up, ok)
	}
	down, ok := mapMouseWheel(tea.MouseWheelMsg{X: 4, Y: 5, Button: tea.MouseWheelDown}, hits)
	if !ok || down.Action != app.PageDown {
		t.Fatalf("wheel-down mapping = (%#v, %t)", down, ok)
	}
	if _, ok := mapMouseClick(tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft}, hits); ok {
		t.Fatal("half-open hit boundary mapped a click")
	}
	if _, ok := mapMouseClick(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseRight}, hits); ok {
		t.Fatal("right click unexpectedly mapped")
	}
}

func TestModifierNegativeDirectMapping(t *testing.T) {
	tests := []struct {
		name  string
		focus app.Focus
		key   tea.Key
	}{
		{name: "ctrl j", focus: app.FocusChats, key: tea.Key{Code: 'j', Text: "j", Mod: tea.ModCtrl}},
		{name: "alt up", focus: app.FocusChats, key: tea.Key{Code: tea.KeyUp, Mod: tea.ModAlt}},
		{name: "meta tab", focus: app.FocusChats, key: tea.Key{Code: tea.KeyTab, Mod: tea.ModMeta}},
		{name: "shift h", focus: app.FocusChats, key: tea.Key{Code: 'h', Text: "h", Mod: tea.ModShift}},
		{name: "ctrl shift c", focus: app.FocusChats, key: tea.Key{Code: 'c', Text: "c", Mod: tea.ModCtrl | tea.ModShift}},
		{name: "ctrl alt u", focus: app.FocusConversation, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl | tea.ModAlt}},
		{name: "ctrl meta d", focus: app.FocusConversation, key: tea.Key{Code: 'd', Text: "d", Mod: tea.ModCtrl | tea.ModMeta}},
		{name: "alt page up", focus: app.FocusConversation, key: tea.Key{Code: tea.KeyPgUp, Mod: tea.ModAlt}},
		{name: "ctrl f2", focus: app.FocusChats, key: tea.Key{Code: tea.KeyF2, Mod: tea.ModCtrl}},
		{name: "shift escape", focus: app.FocusModal, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModShift}},
		{name: "alt modal q", focus: app.FocusModal, key: tea.Key{Code: 'q', Text: "q", Mod: tea.ModAlt}},
		{name: "ctrl composer enter", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}},
		{name: "alt composer backspace", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}},
		{name: "meta auth escape", focus: app.FocusAuth, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModMeta}},
		{name: "shift auth enter", focus: app.FocusAuth, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{name: "ctrl shift composer enter", focus: app.FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl | tea.ModShift}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := mapKeyPress(test.focus, tea.KeyPressMsg(test.key)); ok {
				t.Fatalf("mapKeyPress() = (%#v, true), want no semantic action", got)
			}
		})
	}
}

func TestModifierNegativeAppModelUpdateDoesNotMutate(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.engine.Snapshot()
	state.Focus = app.FocusChats
	model.engine = app.NewEngine(state)

	before := model.engine.Snapshot().SelectedChat
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}))
	if got := model.engine.Snapshot().SelectedChat; got != before {
		t.Fatalf("Alt-J changed selected chat from %d to %d", before, got)
	}

	state = model.engine.Snapshot()
	state.Focus = app.FocusComposer
	beforeDraft := state.Drafts[state.Chats[state.SelectedChat].ID]
	model.engine = app.NewEngine(state)
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
	state = model.engine.Snapshot()
	chatID := state.Chats[state.SelectedChat].ID
	if got := state.Drafts[chatID]; got != beforeDraft {
		t.Fatalf("Alt-Backspace changed draft from %q to %q", beforeDraft, got)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A", Mod: tea.ModShift}))
	if got := model.engine.Snapshot().Drafts[chatID]; got != beforeDraft+"A" {
		t.Fatalf("uppercase editable input = %q, want %q", got, beforeDraft+"A")
	}
}

func TestPhotoSendInput(t *testing.T) {
	// 1: Ctrl+O focus matrix.
	t.Run("ctrlOFocusMatrix", func(t *testing.T) {
		// Composer: Ctrl+O opens PhotoSend.
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusComposer
		state.Drafts[1] = "draft"
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap := model.engine.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != app.FocusPhotoSend {
			t.Fatalf("Composer Ctrl+O: PhotoSend=%v Focus=%v", snap.PhotoSend, snap.Focus)
		}

		// Conversation: Ctrl+O unhandled.
		state2 := app.InitialState()
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = app.FocusConversation
		snap2 := state2
		engine2 := app.NewEngine(snap2)
		model2 := newAppModelForTest(t, engine2, newBoundedAppRuntimeForModelTest(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: 'O', Mod: tea.ModCtrl}))
		snap2 = model2.engine.Snapshot()
		if snap2.PhotoSend != nil {
			t.Errorf("Conversation Ctrl+O opened PhotoSend: unexpected")
		}

		// Modal: Ctrl+O unhandled.
		state3 := app.InitialState()
		state3.Focus = app.FocusModal
		state3.Modal = &app.ModalState{Title: "test"}
		engine3 := app.NewEngine(state3)
		model3 := newAppModelForTest(t, engine3, newBoundedAppRuntimeForModelTest(t))
		model3, _ = updateAppModel(t, model3, tea.WindowSizeMsg{Width: 100, Height: 24})
		model3, _ = updateAppModel(t, model3, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap3 := model3.engine.Snapshot()
		if snap3.PhotoSend != nil {
			t.Errorf("Modal Ctrl+O opened PhotoSend: unexpected")
		}

		// PhotoSend: Ctrl+O unhandled (full-state no-op).
		state4 := app.InitialState()
		state4.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state4.SelectedChat = 0
		state4.Focus = app.FocusPhotoSend
		state4.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: app.FocusConversation}
		engine4 := app.NewEngine(state4)
		model4 := newAppModelForTest(t, engine4, newBoundedAppRuntimeForModelTest(t))
		model4, _ = updateAppModel(t, model4, tea.WindowSizeMsg{Width: 100, Height: 24})
		before := model4.engine.Snapshot()
		model4, _ = updateAppModel(t, model4, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap4 := model4.engine.Snapshot()
		if snap4.PhotoSend == nil {
			t.Fatal("PhotoSend Ctrl+O closed PhotoSend unexpectedly")
		}
		if !reflect.DeepEqual(before.PhotoSend, snap4.PhotoSend) {
			t.Errorf("PhotoSend Ctrl+O mutated state: before=%#v after=%#v", before.PhotoSend, snap4.PhotoSend)
		}
		if snap4.Focus != app.FocusPhotoSend {
			t.Errorf("PhotoSend Ctrl+O changed focus: %v, want FocusPhotoSend", snap4.Focus)
		}

		// Auth: Ctrl+O unhandled.
		state5 := app.InitialState()
		state5.Focus = app.FocusAuth
		state5.Prompt = &app.PromptState{Prompt: auth.Prompt{Label: "Auth"}}
		engine5 := app.NewEngine(state5)
		model5 := newAppModelForTest(t, engine5, newBoundedAppRuntimeForModelTest(t))
		model5, _ = updateAppModel(t, model5, tea.WindowSizeMsg{Width: 100, Height: 24})
		model5, _ = updateAppModel(t, model5, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap5 := model5.engine.Snapshot()
		if snap5.PhotoSend != nil {
			t.Errorf("Auth Ctrl+O opened PhotoSend: unexpected")
		}
	})

	// 2: Ctrl+C quits.
	t.Run("ctrlCQuits", func(t *testing.T) {
		state := app.InitialState()
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("x")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		if cmd == nil {
			t.Fatal("Ctrl+C did not return quit command")
		}
		if !model.engine.Snapshot().Quitting {
			t.Error("Ctrl+C did not set Quitting")
		}
	})

	// 3: Enter/Escape/Backspace in PhotoSend.
	t.Run("enterEscapeBackspace", func(t *testing.T) {
		// Enter on empty path stays open.
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		snap := model.engine.Snapshot()
		if snap.PhotoSend == nil {
			t.Fatal("empty Enter closed PhotoSend")
		}

		// Enter on non-empty path closes with delivery.
		state2 := app.InitialState()
		state2.Connection = domain.ConnectionOnline
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = app.FocusPhotoSend
		state2.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg")}
		engine2 := app.NewEngine(state2)
		model2 := newAppModelForTest(t, engine2, newBoundedAppRuntimeForModelTest(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		snap2 := model2.engine.Snapshot()
		if snap2.PhotoSend != nil {
			t.Fatal("non-empty Enter kept PhotoSend open")
		}

		// Escape closes without altering draft.
		state3 := app.InitialState()
		state3.Connection = domain.ConnectionOnline
		state3.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state3.SelectedChat = 0
		state3.Drafts[1] = "original draft"
		state3.Focus = app.FocusConversation
		state3.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: app.FocusConversation}
		engine3 := app.NewEngine(state3)
		model3 := newAppModelForTest(t, engine3, newBoundedAppRuntimeForModelTest(t))
		model3, _ = updateAppModel(t, model3, tea.WindowSizeMsg{Width: 100, Height: 24})
		model3, _ = updateAppModel(t, model3, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap3 := model3.engine.Snapshot()
		if snap3.PhotoSend != nil {
			t.Fatal("Escape kept PhotoSend open")
		}
		if snap3.Drafts[1] != "original draft" {
			t.Errorf("Escape changed draft: %q", snap3.Drafts[1])
		}
		if snap3.Focus != app.FocusConversation {
			t.Errorf("Escape focus = %v, want Conversation", snap3.Focus)
		}

		// Backspace removes a rune.
		state4 := app.InitialState()
		state4.Connection = domain.ConnectionOnline
		state4.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state4.SelectedChat = 0
		state4.Focus = app.FocusPhotoSend
		state4.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("abc")}
		engine4 := app.NewEngine(state4)
		model4 := newAppModelForTest(t, engine4, newBoundedAppRuntimeForModelTest(t))
		model4, _ = updateAppModel(t, model4, tea.WindowSizeMsg{Width: 100, Height: 24})
		model4, _ = updateAppModel(t, model4, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		snap4 := model4.engine.Snapshot()
		if snap4.PhotoSend == nil {
			t.Fatal("Backspace closed PhotoSend")
		}
		if string(snap4.PhotoSend.Input) != "ab" {
			t.Errorf("Backspace input = %q, want ab", string(snap4.PhotoSend.Input))
		}
	})

	// 4: Modified printable rejection.
	t.Run("modifiedRejected", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/x")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}))
		snap := model.engine.Snapshot()
		if snap.PhotoSend == nil {
			t.Fatal("Alt-J closed PhotoSend")
		}
		if string(snap.PhotoSend.Input) != "/tmp/x" {
			t.Errorf("Alt-J modified input: %q", string(snap.PhotoSend.Input))
		}
	})

	// 5: Plain a/o text behavior - falls through as Runes.
	t.Run("plainAOText", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
		snap := model.engine.Snapshot()
		if string(snap.PhotoSend.Input) != "ao" {
			t.Errorf("Plain a/o input = %q, want ao", string(snap.PhotoSend.Input))
		}
	})

	// 6: Key runes CJK/emoji.
	t.Run("cjkEmojiRunes", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "界"}))
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "👍"}))
		snap := model.engine.Snapshot()
		if string(snap.PhotoSend.Input) != "界👍" {
			t.Errorf("CJK/emoji input = %q, want 界👍", string(snap.PhotoSend.Input))
		}
	})

	// 7: Paste with CRLF/trailing newline.
	t.Run("pasteCRLF", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "/tmp/x.jpg\r\n"})
		snap := model.engine.Snapshot()
		// CR/LF should be stripped by the app reducer (ignored), so path only.
		if string(snap.PhotoSend.Input) != "/tmp/x.jpg" {
			t.Errorf("paste CRLF input = %q, want /tmp/x.jpg", string(snap.PhotoSend.Input))
		}

		// Trailing newline.
		state2 := app.InitialState()
		state2.Focus = app.FocusPhotoSend
		state2.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine2 := app.NewEngine(state2)
		model2 := newAppModelForTest(t, engine2, newBoundedAppRuntimeForModelTest(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.PasteMsg{Content: "/tmp/x.jpg\n"})
		snap2 := model2.engine.Snapshot()
		if string(snap2.PhotoSend.Input) != "/tmp/x.jpg" {
			t.Errorf("paste trailing newline input = %q, want /tmp/x.jpg", string(snap2.PhotoSend.Input))
		}
	})
}

func TestPhotoSendAppModelIntegration(t *testing.T) {
	// Mouse click at composer:photo opens PhotoSend.
	t.Run("mouseClickPhoto", func(t *testing.T) {
		state := app.InitialState()
		state.Width, state.Height = 100, 24
		state.Focus = app.FocusComposer
		state.Connection = domain.ConnectionOnline
		state.ChatsLoaded = true
		state.Chats = []domain.Chat{
			{ID: 2, Kind: domain.ChatSupergroup, Title: "Weekend dev", CanSend: true},
		}
		state.SelectedChat = 0
		state.Drafts[2] = "draft reply"
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

		// Compose view to populate hitRegions.
		_ = model.View()

		// Find the [Photo] hit before opening.
		var foundPhotoHit bool
		for _, hit := range model.hitRegions() {
			if hit.Click.Action == app.OpenPhotoSend {
				foundPhotoHit = true
				break
			}
		}
		if !foundPhotoHit {
			t.Fatal("no OpenPhotoSend hit found in view")
		}

		// Click on the photo button to open PhotoSend.
		photoHit := ui.HitMap{}
		for _, hit := range model.hitRegions() {
			if hit.Click.Action == app.OpenPhotoSend {
				photoHit = append(photoHit, hit)
			}
		}
		if len(photoHit) == 0 {
			t.Fatal("no OpenPhotoSend hit to click")
		}
		pt := photoHit[0].Rect.Min
		model, _ = updateAppModel(t, model, tea.MouseClickMsg{
			X:      pt.X,
			Y:      pt.Y,
			Button: tea.MouseLeft,
		})
		// After clicking composer:photo, PhotoSend should be open.
		snap := model.engine.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != app.FocusPhotoSend {
			t.Fatalf("mouse click did not open PhotoSend: PhotoSend=%v Focus=%v", snap.PhotoSend, snap.Focus)
		}

		// Republish the modal hit map after PhotoSend opens.
		_ = model.View()
		hits := model.hitRegions()
		if hits == nil {
			t.Fatal("republished hit map is nil after PhotoSend opened")
		}
		// The hit map should only contain actions allowed by the photo leaf:
		// zero-action input, Close controls, and (blank path) no PhotoSendSubmit.
		for _, hit := range hits {
			switch hit.Click.Action {
			case app.OpenPhotoSend, app.ComposerSubmit, app.FocusPane,
				app.SelectChat, app.PageUp, app.PageDown:
				t.Fatalf("photo hit map should not contain %v click", hit.Click.Action)
			}
			if hit.WheelUp.Action != app.NoAction || hit.WheelDown.Action != app.NoAction {
				t.Fatal("photo hit map should not contain nonzero wheel actions")
			}
		}
		// Find an actual Close hit and mouse-close the modal.
		var closeHitFound bool
		for _, hit := range hits {
			if hit.Click.Action == app.Close {
				closeHitFound = true
				model, _ = updateAppModel(t, model, tea.MouseClickMsg{
					X:      hit.Rect.Min.X,
					Y:      hit.Rect.Min.Y,
					Button: tea.MouseLeft,
				})
				break
			}
		}
		if !closeHitFound {
			t.Fatal("no Close hit found in photo hit map")
		}
		snap = model.engine.Snapshot()
		if snap.PhotoSend != nil {
			t.Fatalf("mouse close did not clear PhotoSend: %v", snap.PhotoSend)
		}
		if snap.Focus != app.FocusComposer {
			t.Fatalf("mouse close focus = %v, want Composer", snap.Focus)
		}

		// Verify Ctrl+O also works to open PhotoSend from composer focus.
		// First close via Escape.
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap = model.engine.Snapshot()
		if snap.PhotoSend != nil {
			t.Fatalf("Escape did not close PhotoSend")
		}
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap = model.engine.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != app.FocusPhotoSend {
			t.Fatalf("Ctrl+O did not open PhotoSend: PhotoSend=%v Focus=%v", snap.PhotoSend, snap.Focus)
		}
	})

	// Escape closes without altering draft/reply.
	t.Run("escapePreservesDraft", func(t *testing.T) {
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Drafts[1] = "saved draft"
		state.ReplyTarget = &app.ReplyTarget{ChatID: 1, MessageID: 42, Sender: "Alice", Preview: "Hello world"}
		state.Focus = app.FocusConversation
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: app.FocusConversation}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap := model.engine.Snapshot()
		if snap.PhotoSend != nil {
			t.Fatal("Escape did not close PhotoSend")
		}
		if snap.Drafts[1] != "saved draft" {
			t.Errorf("Escape changed draft: %q", snap.Drafts[1])
		}
		if snap.ReplyTarget == nil {
			t.Fatal("Escape cleared ReplyTarget")
		}
		if snap.ReplyTarget.ChatID != 1 || snap.ReplyTarget.MessageID != 42 || snap.ReplyTarget.Sender != "Alice" || snap.ReplyTarget.Preview != "Hello world" {
			t.Errorf("ReplyTarget fields changed: %#v", snap.ReplyTarget)
		}
	})

	// Enter on empty path stays open; Enter on non-empty path closes with delivery.
	t.Run("enterEmptyStaysNonEmptyCloses", func(t *testing.T) {
		// Empty path Enter stays open.
		state := app.InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = app.FocusPhotoSend
		state.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("")}
		engine := app.NewEngine(state)
		model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if model.engine.Snapshot().PhotoSend == nil {
			t.Fatal("empty Enter closed PhotoSend")
		}

		// Non-empty Enter closes with delivery command.
		state2 := app.InitialState()
		state2.Connection = domain.ConnectionOnline
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = app.FocusPhotoSend
		state2.PhotoSend = &app.PhotoSendState{ChatID: 1, Input: []rune("/tmp/photo.jpg")}
		engine2 := app.NewEngine(state2)
		model2 := newAppModelForTest(t, engine2, newBoundedAppRuntimeForModelTest(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, cmd := updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if cmd == nil {
			t.Fatal("non-empty Enter returned nil command")
		}
		if model2.engine.Snapshot().PhotoSend != nil {
			t.Fatal("non-empty Enter kept PhotoSend open")
		}
	})
}
