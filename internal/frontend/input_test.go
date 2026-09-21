package frontend

import (
	"image"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestSemanticInputMapsNavigationAndEditors(t *testing.T) {
	tests := []struct {
		name  string
		focus Focus
		key   tea.Key
		want  ActionReceived
	}{
		{name: "down", focus: FocusChats, key: tea.Key{Code: tea.KeyDown}, want: ActionReceived{Action: SelectNext}},
		{name: "j", focus: FocusChats, key: tea.Key{Text: "j", Code: 'j'}, want: ActionReceived{Action: SelectNext}},
		{name: "up", focus: FocusChats, key: tea.Key{Code: tea.KeyUp}, want: ActionReceived{Action: SelectPrevious}},
		{name: "left mirrors h", focus: FocusChats, key: tea.Key{Code: tea.KeyLeft}, want: ActionReceived{Action: FocusPrevious}},
		{name: "right opens chat", focus: FocusChats, key: tea.Key{Code: tea.KeyRight}, want: ActionReceived{Action: OpenChat}},
		{name: "enter opens chat actions", focus: FocusChats, key: tea.Key{Code: tea.KeyEnter}, want: ActionReceived{Action: OpenChatActionMenu}},
		{name: "tab", focus: FocusChats, key: tea.Key{Code: tea.KeyTab}, want: ActionReceived{Action: FocusNext}},
		{name: "shift tab", focus: FocusChats, key: tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, want: ActionReceived{Action: FocusPrevious}},
		{name: "details", focus: FocusChats, key: tea.Key{Code: tea.KeyF2}, want: ActionReceived{Action: ToggleDetails}},
		{name: "next unread", focus: FocusChats, key: tea.Key{Text: "u", Code: 'u'}, want: ActionReceived{Action: SelectNextUnread}},
		{name: "next mention", focus: FocusChats, key: tea.Key{Text: "m", Code: 'm'}, want: ActionReceived{Action: SelectNextMention}},
		{name: "page up", focus: FocusConversation, key: tea.Key{Code: tea.KeyPgUp}, want: ActionReceived{Action: PageUp}},
		{name: "page down", focus: FocusConversation, key: tea.Key{Code: tea.KeyPgDown}, want: ActionReceived{Action: PageDown}},
		{name: "ctrl u", focus: FocusConversation, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl}, want: ActionReceived{Action: PageUp}},
		{name: "ctrl d", focus: FocusConversation, key: tea.Key{Code: 'd', Text: "d", Mod: tea.ModCtrl}, want: ActionReceived{Action: PageDown}},
		{name: "composer submit", focus: FocusComposer, key: tea.Key{Code: tea.KeyEnter}, want: ActionReceived{Action: ComposerSubmit}},
		{name: "composer escape with lock state", focus: FocusComposer, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModNumLock}, want: ActionReceived{Action: Close}},
		{name: "composer newline", focus: FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}, want: ActionReceived{Action: ComposerNewline}},
		{name: "auth backspace", focus: FocusAuth, key: tea.Key{Code: tea.KeyBackspace}, want: ActionReceived{Action: ComposerBackspace}},
		{name: "modal q", focus: FocusModal, key: tea.Key{Text: "q", Code: 'q'}, want: ActionReceived{Action: Close}},
		{name: "quit", focus: FocusComposer, key: tea.Key{Code: 'c', Mod: tea.ModCtrl}, want: ActionReceived{Action: Quit}},
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
		want Action
	}{
		{name: "next message", key: tea.Key{Code: tea.KeyDown}, want: SelectNextMessage},
		{name: "previous message", key: tea.Key{Code: tea.KeyUp}, want: SelectPreviousMessage},
		{name: "open menu", key: tea.Key{Code: tea.KeyEnter}, want: OpenMessageActionMenu},
		{name: "copy", key: tea.Key{Code: 'c', Text: "c"}, want: CopyMessage},
		{name: "edit", key: tea.Key{Code: 'e', Text: "e"}, want: EditMessage},
		{name: "close with q", key: tea.Key{Code: 'q', Text: "q"}, want: Close},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(test.key))
			if !ok || got.Action != test.want {
				t.Fatalf("mapped action = (%v, %t), want (%v, true)", got.Action, ok, test.want)
			}
		})
	}
}

func TestUnreadAndMentionShortcutsAreChatsOnlyAndUnmodified(t *testing.T) {
	for _, test := range []struct {
		focus Focus
		key   tea.Key
	}{
		{focus: FocusConversation, key: tea.Key{Text: "u", Code: 'u'}},
		{focus: FocusConversation, key: tea.Key{Text: "m", Code: 'm'}},
		{focus: FocusChats, key: tea.Key{Text: "u", Code: 'u', Mod: tea.ModAlt}},
		{focus: FocusChats, key: tea.Key{Text: "m", Code: 'm', Mod: tea.ModCtrl}},
		{focus: FocusChats, key: tea.Key{Text: "U", Code: 'U', Mod: tea.ModShift}},
		{focus: FocusChats, key: tea.Key{Text: "M", Code: 'M', Mod: tea.ModShift}},
	} {
		got, ok := mapKeyPress(test.focus, tea.KeyPressMsg(test.key))
		if ok && (got.Action == SelectNextUnread || got.Action == SelectNextMention) {
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
		got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(key))
		if ok && (got.Action == DeleteMessage || got.Action == DeleteForEveryone) {
			t.Fatalf("conversation key %#v unexpectedly mapped to delete action %v", key, got.Action)
		}
	}
}

func TestActionModalKeyboardListMappings(t *testing.T) {
	for _, test := range []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: tea.KeyDown}, SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, SelectNext},
		{tea.Key{Code: tea.KeyUp}, SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, Activate},
		{tea.Key{Code: tea.KeyEscape}, Close},
		{tea.Key{Code: 'q', Text: "q"}, Close},
	} {
		got, ok := mapKeyPress(FocusModal, tea.KeyPressMsg(test.key))
		if !ok || got.Action != test.want {
			t.Fatalf("modal key %#v = (%v,%t), want %v", test.key, got.Action, ok, test.want)
		}
	}
}

func TestForwardPickerKeyboardMappings(t *testing.T) {
	for _, test := range []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: tea.KeyDown}, SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, SelectNext},
		{tea.Key{Code: tea.KeyUp}, SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, Activate},
		{tea.Key{Code: tea.KeyEscape}, Close},
	} {
		got, ok := mapKeyPress(FocusForwardPicker, tea.KeyPressMsg(test.key))
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
		if got, ok := mapKeyPress(FocusForwardPicker, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved picker key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestArrowKeysMirrorHJKLByFocus(t *testing.T) {
	tests := []struct {
		focus Focus
		arrow tea.Key
		vim   tea.Key
	}{
		{FocusChats, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{FocusChats, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{FocusChats, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{FocusChats, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{FocusConversation, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{FocusConversation, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{FocusConversation, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{FocusConversation, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{FocusModal, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{FocusModal, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{FocusForwardPicker, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{FocusForwardPicker, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{FocusForwardPicker, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{FocusForwardPicker, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
		{FocusReactionPicker, tea.Key{Code: tea.KeyDown}, tea.Key{Code: 'j', Text: "j"}},
		{FocusReactionPicker, tea.Key{Code: tea.KeyUp}, tea.Key{Code: 'k', Text: "k"}},
		{FocusReactionPicker, tea.Key{Code: tea.KeyLeft}, tea.Key{Code: 'h', Text: "h"}},
		{FocusReactionPicker, tea.Key{Code: tea.KeyRight}, tea.Key{Code: 'l', Text: "l"}},
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
		want Action
	}{
		{tea.Key{Code: tea.KeyDown}, SelectNext},
		{tea.Key{Code: 'j', Text: "j"}, SelectNext},
		{tea.Key{Code: tea.KeyUp}, SelectPrevious},
		{tea.Key{Code: 'k', Text: "k"}, SelectPrevious},
		{tea.Key{Code: tea.KeyEnter}, Activate},
		{tea.Key{Code: tea.KeyEscape}, Close},
	} {
		got, ok := mapKeyPress(FocusReactionPicker, tea.KeyPressMsg(test.key))
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
		if got, ok := mapKeyPress(FocusReactionPicker, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved reaction picker key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestEditableModifiersTextDoesNotMutateEngineContinuation(t *testing.T) {
	for _, test := range []struct {
		name  string
		focus Focus
		key   tea.Key
	}{
		{name: "auth alt text", focus: FocusAuth, key: tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}},
		{name: "auth ctrl text", focus: FocusAuth, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl}},
		{name: "composer meta text", focus: FocusComposer, key: tea.Key{Code: 'q', Text: "q", Mod: tea.ModMeta}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := InitialState()
			state.Focus = test.focus
			if test.focus == FocusAuth {
				state.Prompt = &PromptState{}
			} else {
				state.Chats = []domain.Chat{{ID: 7, CanSend: true}}
				state.Drafts[7] = "before"
			}
			model := newAppModelForTest(t, state, newTestSession(t))
			before := model.Snapshot()
			_, cmd := model.Update(tea.KeyPressMsg(test.key))
			if cmd != nil {
				t.Fatal("modified text key returned a command")
			}
			after := model.Snapshot()
			if test.focus == FocusAuth {
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
	hits := HitMap{{
		Rect:      image.Rect(2, 3, 10, 12),
		Click:     ActionReceived{Action: SelectChat, ChatID: 42},
		WheelUp:   ActionReceived{Action: PageUp},
		WheelDown: ActionReceived{Action: PageDown},
	}}

	click, ok := mapMouseClick(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft}, hits)
	if !ok || click.Action != SelectChat || click.ChatID != 42 {
		t.Fatalf("click mapping = (%#v, %t)", click, ok)
	}
	up, ok := mapMouseWheel(tea.MouseWheelMsg{X: 4, Y: 5, Button: tea.MouseWheelUp}, hits)
	if !ok || up.Action != PageUp {
		t.Fatalf("wheel-up mapping = (%#v, %t)", up, ok)
	}
	down, ok := mapMouseWheel(tea.MouseWheelMsg{X: 4, Y: 5, Button: tea.MouseWheelDown}, hits)
	if !ok || down.Action != PageDown {
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
		focus Focus
		key   tea.Key
	}{
		{name: "ctrl j", focus: FocusChats, key: tea.Key{Code: 'j', Text: "j", Mod: tea.ModCtrl}},
		{name: "alt up", focus: FocusChats, key: tea.Key{Code: tea.KeyUp, Mod: tea.ModAlt}},
		{name: "meta tab", focus: FocusChats, key: tea.Key{Code: tea.KeyTab, Mod: tea.ModMeta}},
		{name: "shift h", focus: FocusChats, key: tea.Key{Code: 'h', Text: "h", Mod: tea.ModShift}},
		{name: "ctrl shift c", focus: FocusChats, key: tea.Key{Code: 'c', Text: "c", Mod: tea.ModCtrl | tea.ModShift}},
		{name: "ctrl alt u", focus: FocusConversation, key: tea.Key{Code: 'u', Text: "u", Mod: tea.ModCtrl | tea.ModAlt}},
		{name: "ctrl meta d", focus: FocusConversation, key: tea.Key{Code: 'd', Text: "d", Mod: tea.ModCtrl | tea.ModMeta}},
		{name: "alt page up", focus: FocusConversation, key: tea.Key{Code: tea.KeyPgUp, Mod: tea.ModAlt}},
		{name: "ctrl f2", focus: FocusChats, key: tea.Key{Code: tea.KeyF2, Mod: tea.ModCtrl}},
		{name: "shift escape", focus: FocusModal, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModShift}},
		{name: "alt modal q", focus: FocusModal, key: tea.Key{Code: 'q', Text: "q", Mod: tea.ModAlt}},
		{name: "ctrl composer enter", focus: FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}},
		{name: "alt composer backspace", focus: FocusComposer, key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}},
		{name: "meta auth escape", focus: FocusAuth, key: tea.Key{Code: tea.KeyEscape, Mod: tea.ModMeta}},
		{name: "shift auth enter", focus: FocusAuth, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{name: "ctrl shift composer enter", focus: FocusComposer, key: tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl | tea.ModShift}},
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
	state := model.Snapshot()
	state.Focus = FocusChats
	model.state = &state

	before := model.Snapshot().SelectedChat
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}))
	if got := model.Snapshot().SelectedChat; got != before {
		t.Fatalf("Alt-J changed selected chat from %d to %d", before, got)
	}

	state = model.Snapshot()
	state.Focus = FocusComposer
	beforeDraft := state.Drafts[state.Chats[state.SelectedChat].ID]
	model.state = &state
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
	state = model.Snapshot()
	chatID := state.Chats[state.SelectedChat].ID
	if got := state.Drafts[chatID]; got != beforeDraft {
		t.Fatalf("Alt-Backspace changed draft from %q to %q", beforeDraft, got)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A", Mod: tea.ModShift}))
	if got := model.Snapshot().Drafts[chatID]; got != beforeDraft+"A" {
		t.Fatalf("uppercase editable input = %q, want %q", got, beforeDraft+"A")
	}
}

func TestPhotoSendInput(t *testing.T) {
	// 1: Ctrl+O focus matrix.
	t.Run("ctrlOFocusMatrix", func(t *testing.T) {
		// Composer: Ctrl+O opens PhotoSend.
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusComposer
		state.Drafts[1] = "draft"
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap := model.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != FocusPhotoSend {
			t.Fatalf("Composer Ctrl+O: PhotoSend=%v Focus=%v", snap.PhotoSend, snap.Focus)
		}

		// Conversation: Ctrl+O unhandled.
		state2 := InitialState()
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = FocusConversation
		snap2 := state2
		model2 := newAppModelForTest(t, snap2, newTestSession(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: 'O', Mod: tea.ModCtrl}))
		snap2 = model2.Snapshot()
		if snap2.PhotoSend != nil {
			t.Errorf("Conversation Ctrl+O opened PhotoSend: unexpected")
		}

		// Modal: Ctrl+O unhandled.
		state3 := InitialState()
		state3.Focus = FocusModal
		state3.Modal = &ModalState{Title: "test"}
		model3 := newAppModelForTest(t, state3, newTestSession(t))
		model3, _ = updateAppModel(t, model3, tea.WindowSizeMsg{Width: 100, Height: 24})
		model3, _ = updateAppModel(t, model3, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap3 := model3.Snapshot()
		if snap3.PhotoSend != nil {
			t.Errorf("Modal Ctrl+O opened PhotoSend: unexpected")
		}

		// PhotoSend: Ctrl+O unhandled (full-state no-op).
		state4 := InitialState()
		state4.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state4.SelectedChat = 0
		state4.Focus = FocusPhotoSend
		state4.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: FocusConversation}
		model4 := newAppModelForTest(t, state4, newTestSession(t))
		model4, _ = updateAppModel(t, model4, tea.WindowSizeMsg{Width: 100, Height: 24})
		before := model4.Snapshot()
		model4, _ = updateAppModel(t, model4, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap4 := model4.Snapshot()
		if snap4.PhotoSend == nil {
			t.Fatal("PhotoSend Ctrl+O closed PhotoSend unexpectedly")
		}
		if !reflect.DeepEqual(before.PhotoSend, snap4.PhotoSend) {
			t.Errorf("PhotoSend Ctrl+O mutated state: before=%#v after=%#v", before.PhotoSend, snap4.PhotoSend)
		}
		if snap4.Focus != FocusPhotoSend {
			t.Errorf("PhotoSend Ctrl+O changed focus: %v, want FocusPhotoSend", snap4.Focus)
		}

		// Auth: Ctrl+O unhandled.
		state5 := InitialState()
		state5.Focus = FocusAuth
		state5.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Auth"}}
		model5 := newAppModelForTest(t, state5, newTestSession(t))
		model5, _ = updateAppModel(t, model5, tea.WindowSizeMsg{Width: 100, Height: 24})
		model5, _ = updateAppModel(t, model5, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap5 := model5.Snapshot()
		if snap5.PhotoSend != nil {
			t.Errorf("Auth Ctrl+O opened PhotoSend: unexpected")
		}
	})

	// 2: Ctrl+C quits.
	t.Run("ctrlCQuits", func(t *testing.T) {
		state := InitialState()
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("x")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		if cmd == nil {
			t.Fatal("Ctrl+C did not return quit command")
		}
		if !model.Snapshot().Quitting {
			t.Error("Ctrl+C did not set Quitting")
		}
	})

	// 3: Enter/Escape/Backspace in PhotoSend.
	t.Run("enterEscapeBackspace", func(t *testing.T) {
		// Enter on empty path stays open.
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		snap := model.Snapshot()
		if snap.PhotoSend == nil {
			t.Fatal("empty Enter closed PhotoSend")
		}

		// Enter on non-empty path closes with delivery.
		state2 := InitialState()
		state2.Connection = domain.ConnectionOnline
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = FocusPhotoSend
		state2.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg")}
		model2 := newAppModelForTest(t, state2, newTestSession(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		snap2 := model2.Snapshot()
		if snap2.PhotoSend != nil {
			t.Fatal("non-empty Enter kept PhotoSend open")
		}

		// Escape closes without altering draft.
		state3 := InitialState()
		state3.Connection = domain.ConnectionOnline
		state3.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state3.SelectedChat = 0
		state3.Drafts[1] = "original draft"
		state3.Focus = FocusConversation
		state3.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: FocusConversation}
		model3 := newAppModelForTest(t, state3, newTestSession(t))
		model3, _ = updateAppModel(t, model3, tea.WindowSizeMsg{Width: 100, Height: 24})
		model3, _ = updateAppModel(t, model3, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap3 := model3.Snapshot()
		if snap3.PhotoSend != nil {
			t.Fatal("Escape kept PhotoSend open")
		}
		if snap3.Drafts[1] != "original draft" {
			t.Errorf("Escape changed draft: %q", snap3.Drafts[1])
		}
		if snap3.Focus != FocusConversation {
			t.Errorf("Escape focus = %v, want Conversation", snap3.Focus)
		}

		// Backspace removes a rune.
		state4 := InitialState()
		state4.Connection = domain.ConnectionOnline
		state4.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state4.SelectedChat = 0
		state4.Focus = FocusPhotoSend
		state4.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("abc")}
		model4 := newAppModelForTest(t, state4, newTestSession(t))
		model4, _ = updateAppModel(t, model4, tea.WindowSizeMsg{Width: 100, Height: 24})
		model4, _ = updateAppModel(t, model4, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		snap4 := model4.Snapshot()
		if snap4.PhotoSend == nil {
			t.Fatal("Backspace closed PhotoSend")
		}
		if string(snap4.PhotoSend.Input) != "ab" {
			t.Errorf("Backspace input = %q, want ab", string(snap4.PhotoSend.Input))
		}
	})

	// 4: Modified printable rejection.
	t.Run("modifiedRejected", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j", Mod: tea.ModAlt}))
		snap := model.Snapshot()
		if snap.PhotoSend == nil {
			t.Fatal("Alt-J closed PhotoSend")
		}
		if string(snap.PhotoSend.Input) != "/tmp/x" {
			t.Errorf("Alt-J modified input: %q", string(snap.PhotoSend.Input))
		}
	})

	// 5: Plain a/o text behavior - falls through as Runes.
	t.Run("plainAOText", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
		snap := model.Snapshot()
		if string(snap.PhotoSend.Input) != "ao" {
			t.Errorf("Plain a/o input = %q, want ao", string(snap.PhotoSend.Input))
		}
	})

	// 6: Key runes CJK/emoji.
	t.Run("cjkEmojiRunes", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "界"}))
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "👍"}))
		snap := model.Snapshot()
		if string(snap.PhotoSend.Input) != "界👍" {
			t.Errorf("CJK/emoji input = %q, want 界👍", string(snap.PhotoSend.Input))
		}
	})

	// 7: Paste with CRLF/trailing newline.
	t.Run("pasteCRLF", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "/tmp/x.jpg\r\n"})
		snap := model.Snapshot()
		// CR/LF should be stripped by the app reducer (ignored), so path only.
		if string(snap.PhotoSend.Input) != "/tmp/x.jpg" {
			t.Errorf("paste CRLF input = %q, want /tmp/x.jpg", string(snap.PhotoSend.Input))
		}

		// Trailing newline.
		state2 := InitialState()
		state2.Focus = FocusPhotoSend
		state2.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model2 := newAppModelForTest(t, state2, newTestSession(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, _ = updateAppModel(t, model2, tea.PasteMsg{Content: "/tmp/x.jpg\n"})
		snap2 := model2.Snapshot()
		if string(snap2.PhotoSend.Input) != "/tmp/x.jpg" {
			t.Errorf("paste trailing newline input = %q, want /tmp/x.jpg", string(snap2.PhotoSend.Input))
		}
	})
}

func TestPhotoSendAppModelIntegration(t *testing.T) {
	// Mouse click at composer:photo opens PhotoSend.
	t.Run("mouseClickPhoto", func(t *testing.T) {
		state := InitialState()
		state.Width, state.Height = 100, 24
		state.Focus = FocusComposer
		state.Connection = domain.ConnectionOnline
		state.ChatsLoaded = true
		state.Chats = []domain.Chat{
			{ID: 2, Kind: domain.ChatSupergroup, Title: "Weekend dev", CanSend: true},
		}
		state.SelectedChat = 0
		state.Drafts[2] = "draft reply"
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

		// Compose view to populate hitRegions.
		_ = model.View()

		// Find the [Photo] hit before opening.
		var foundPhotoHit bool
		for _, hit := range model.hitRegions() {
			if hit.Click.Action == OpenPhotoSend {
				foundPhotoHit = true
				break
			}
		}
		if !foundPhotoHit {
			t.Fatal("no OpenPhotoSend hit found in view")
		}

		// Click on the photo button to open PhotoSend.
		photoHit := HitMap{}
		for _, hit := range model.hitRegions() {
			if hit.Click.Action == OpenPhotoSend {
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
		snap := model.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != FocusPhotoSend {
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
			case OpenPhotoSend, ComposerSubmit, FocusPane,
				SelectChat, PageUp, PageDown:
				t.Fatalf("photo hit map should not contain %v click", hit.Click.Action)
			}
			if hit.WheelUp.Action != NoAction || hit.WheelDown.Action != NoAction {
				t.Fatal("photo hit map should not contain nonzero wheel actions")
			}
		}
		// Find an actual Close hit and mouse-close the modal.
		var closeHitFound bool
		for _, hit := range hits {
			if hit.Click.Action == Close {
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
		snap = model.Snapshot()
		if snap.PhotoSend != nil {
			t.Fatalf("mouse close did not clear PhotoSend: %v", snap.PhotoSend)
		}
		if snap.Focus != FocusComposer {
			t.Fatalf("mouse close focus = %v, want Composer", snap.Focus)
		}

		// Verify Ctrl+O also works to open PhotoSend from composer focus.
		// First close via Escape.
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap = model.Snapshot()
		if snap.PhotoSend != nil {
			t.Fatalf("Escape did not close PhotoSend")
		}
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
		snap = model.Snapshot()
		if snap.PhotoSend == nil || snap.Focus != FocusPhotoSend {
			t.Fatalf("Ctrl+O did not open PhotoSend: PhotoSend=%v Focus=%v", snap.PhotoSend, snap.Focus)
		}
	})

	// Escape closes without altering draft/reply.
	t.Run("escapePreservesDraft", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Drafts[1] = "saved draft"
		state.ReplyTarget = &ReplyTarget{ChatID: 1, MessageID: 42, Sender: "Alice", Preview: "Hello world"}
		state.Focus = FocusConversation
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x.jpg"), PreviousFocus: FocusConversation}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		snap := model.Snapshot()
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
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusPhotoSend
		state.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("")}
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if model.Snapshot().PhotoSend == nil {
			t.Fatal("empty Enter closed PhotoSend")
		}

		// Non-empty Enter closes with delivery command.
		state2 := InitialState()
		state2.Connection = domain.ConnectionOnline
		state2.Chats = []domain.Chat{{ID: 1, CanSend: true}}
		state2.SelectedChat = 0
		state2.Focus = FocusPhotoSend
		state2.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/photo.jpg")}
		model2 := newAppModelForTest(t, state2, newTestSession(t))
		model2, _ = updateAppModel(t, model2, tea.WindowSizeMsg{Width: 100, Height: 24})
		model2, cmd := updateAppModel(t, model2, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if cmd == nil {
			t.Fatal("non-empty Enter returned nil command")
		}
		if model2.Snapshot().PhotoSend != nil {
			t.Fatal("non-empty Enter kept PhotoSend open")
		}
	})
}
