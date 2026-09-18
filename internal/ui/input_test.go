package ui

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func keyboardEvent(id string) gotui.Event {
	return gotui.Event{
		Type: gotui.KeyboardEvent,
		ID:   id,
	}
}

func TestMapKeyGlobalStringIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   string
		want app.Action
	}{
		{id: "j", want: app.SelectNext},
		{id: "<Down>", want: app.SelectNext},
		{id: "k", want: app.SelectPrevious},
		{id: "<Up>", want: app.SelectPrevious},
		{id: "l", want: app.FocusNext},
		{id: "<Tab>", want: app.FocusNext},
		{id: "h", want: app.FocusPrevious},
		{id: "<Enter>", want: app.Activate},
		{id: "i", want: app.ToggleDetails},
		{id: "<F2>", want: app.ToggleDetails},
		{id: "<Escape>", want: app.Close},
		{id: "<C-u>", want: app.PageUp},
		{id: "<PageUp>", want: app.PageUp},
		{id: "<C-d>", want: app.PageDown},
		{id: "<PageDown>", want: app.PageDown},
		{id: "<C-c>", want: app.Quit},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()

			got, ok := MapKey(app.FocusChats, keyboardEvent(test.id))
			if !ok || got != (app.ActionReceived{Action: test.want}) {
				t.Fatalf("MapKey(%q) = (%#v, %t), want action %v", test.id, got, ok, test.want)
			}
		})
	}
}

func TestMapKeyRejectsUnknownAndInvalidEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event gotui.Event
	}{
		{name: "unknown ID", event: keyboardEvent("<F12>")},
		{name: "unknown ID with wrong payload", event: gotui.Event{Type: gotui.KeyboardEvent, ID: "<F12>", Payload: gotui.Mouse{X: 1, Y: 2}}},
		{name: "unknown ID with typed nil key payload", event: gotui.Event{Type: gotui.KeyboardEvent, ID: "<F12>", Payload: (*tcell.EventKey)(nil)}},
		{name: "wrong event type without payload", event: gotui.Event{Type: gotui.ResizeEvent, ID: "j"}},
		{name: "wrong event type with key payload", event: gotui.Event{Type: gotui.ResizeEvent, ID: "j", Payload: tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := MapKey(app.FocusChats, test.event); ok {
				t.Fatalf("MapKey() = (%#v, true), want no mapping", got)
			}
		})
	}
}

func TestMapKeyStringIDsDoNotRequireRawKeyPayload(t *testing.T) {
	t.Parallel()

	payloads := []struct {
		name  string
		value any
	}{
		{name: "nil"},
		{name: "wrong nonnil", value: gotui.Mouse{X: 1, Y: 2}},
		{name: "typed nil key", value: (*tcell.EventKey)(nil)},
	}
	tests := []struct {
		name  string
		focus app.Focus
		id    string
		want  app.Action
	}{
		{name: "global", focus: app.FocusChats, id: "j", want: app.SelectNext},
		{name: "composer", focus: app.FocusComposer, id: "<Enter>", want: app.ComposerSubmit},
		{name: "auth", focus: app.FocusAuth, id: "<Enter>", want: app.ComposerSubmit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, payload := range payloads {
				t.Run(payload.name, func(t *testing.T) {
					event := gotui.Event{Type: gotui.KeyboardEvent, ID: test.id, Payload: payload.value}
					got, ok := MapKey(test.focus, event)
					if !ok || got != (app.ActionReceived{Action: test.want}) {
						t.Fatalf("MapKey(%v, %#v) = (%#v, %t), want action %v", test.focus, event, got, ok, test.want)
					}
				})
			}
		})
	}
}

func TestMapKeyEditableFocusMapsTextAndEditingCommands(t *testing.T) {
	t.Parallel()

	for _, focus := range []app.Focus{app.FocusAuth, app.FocusComposer} {
		t.Run(focusName(focus), func(t *testing.T) {
			t.Parallel()

			for _, test := range []struct {
				id   string
				want app.ActionReceived
			}{
				{id: "a", want: app.ActionReceived{Rune: 'a'}},
				{id: "界", want: app.ActionReceived{Rune: '界'}},
				{id: "j", want: app.ActionReceived{Rune: 'j'}},
				{id: "k", want: app.ActionReceived{Rune: 'k'}},
				{id: "q", want: app.ActionReceived{Rune: 'q'}},
				{id: "<Enter>", want: app.ActionReceived{Action: app.ComposerSubmit}},
				{id: "<Backspace>", want: app.ActionReceived{Action: app.ComposerBackspace}},
				{id: "<Escape>", want: app.ActionReceived{Action: app.Close}},
			} {
				got, ok := MapKey(focus, keyboardEvent(test.id))
				if !ok || got != test.want {
					t.Errorf("MapKey(%v, %q) = (%#v, %t), want (%#v, true)", focus, test.id, got, ok, test.want)
				}
			}

			for _, id := range []string{"ab", "界面", "", "<F2>"} {
				if got, ok := MapKey(focus, keyboardEvent(id)); ok {
					t.Errorf("MapKey(%v, %q) = (%#v, true), want no mapping", focus, id, got)
				}
			}
		})
	}
}

func focusName(focus app.Focus) string {
	switch focus {
	case app.FocusAuth:
		return "auth"
	case app.FocusComposer:
		return "composer"
	default:
		return "other"
	}
}

func TestMapKeyModalQClosesAndNavigationRemainsSemantic(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		id   string
		want app.Action
	}{
		{id: "q", want: app.Close},
		{id: "<Escape>", want: app.Close},
		{id: "j", want: app.SelectNext},
	} {
		got, ok := MapKey(app.FocusModal, keyboardEvent(test.id))
		if !ok || got.Action != test.want {
			t.Errorf("MapKey(FocusModal, %q) = (%#v, %t), want action %v", test.id, got, ok, test.want)
		}
	}

	if got, ok := MapKey(app.FocusChats, keyboardEvent("q")); ok {
		t.Fatalf("MapKey(FocusChats, q) = (%#v, true), want no mapping", got)
	}
}

func TestMapKeyRawBacktabFocusesPrevious(t *testing.T) {
	t.Parallel()

	event := gotui.Event{
		Type:    gotui.KeyboardEvent,
		ID:      "<Backtab>",
		Payload: tcell.NewEventKey(tcell.KeyBacktab, "", tcell.ModNone),
	}

	got, ok := MapKey(app.FocusChats, event)
	if !ok || got != (app.ActionReceived{Action: app.FocusPrevious}) {
		t.Fatalf("MapKey(Backtab) = (%#v, %t), want FocusPrevious", got, ok)
	}
}

func TestMapKeyRawShiftEnterInComposerAddsNewline(t *testing.T) {
	t.Parallel()

	event := gotui.Event{
		Type:    gotui.KeyboardEvent,
		ID:      "<Enter>",
		Payload: tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModShift|tcell.ModAlt),
	}

	got, ok := MapKey(app.FocusComposer, event)
	if !ok || got != (app.ActionReceived{Action: app.ComposerNewline}) {
		t.Fatalf("MapKey(Shift+Enter) = (%#v, %t), want ComposerNewline", got, ok)
	}
}

func TestMapKeyEnterWithoutShiftDoesNotAddNewline(t *testing.T) {
	t.Parallel()

	event := gotui.Event{
		Type:    gotui.KeyboardEvent,
		ID:      "<Enter>",
		Payload: tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone),
	}

	got, ok := MapKey(app.FocusComposer, event)
	if !ok || got != (app.ActionReceived{Action: app.ComposerSubmit}) {
		t.Fatalf("MapKey(Enter) = (%#v, %t), want ComposerSubmit", got, ok)
	}
}

func TestMapMouseRejectsWrongTypeAndPayload(t *testing.T) {
	t.Parallel()

	hits := HitMap{{Rect: image.Rect(0, 0, 10, 10), Click: app.ActionReceived{Action: app.Activate}}}
	tests := []gotui.Event{
		{Type: gotui.KeyboardEvent, ID: "<MouseLeft>", Payload: gotui.Mouse{X: 2, Y: 3}},
		{Type: gotui.MouseEvent, ID: "<MouseLeft>", Payload: tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)},
		{Type: gotui.MouseEvent, ID: "<MouseLeft>", Payload: &gotui.Mouse{X: 2, Y: 3}},
		{Type: gotui.MouseEvent, ID: "<MouseLeft>"},
	}

	for _, event := range tests {
		if got, ok := MapMouse(event, hits); ok {
			t.Errorf("MapMouse(%#v) = (%#v, true), want no mapping", event, got)
		}
	}
}

func TestMapMouseRejectsNonClickButtons(t *testing.T) {
	t.Parallel()

	hits := HitMap{{Rect: image.Rect(0, 0, 10, 10), Click: app.ActionReceived{Action: app.Activate}}}
	for _, id := range []string{
		"<MouseRelease>",
		"<MouseMiddle>",
		"<MouseRight>",
		"Unknown_Mouse_Button",
	} {
		t.Run(id, func(t *testing.T) {
			event := gotui.Event{Type: gotui.MouseEvent, ID: id, Payload: gotui.Mouse{X: 2, Y: 3}}
			if got, ok := MapMouse(event, hits); ok {
				t.Fatalf("MapMouse(%q) = (%#v, true), want no mapping", id, got)
			}
		})
	}
}

func TestMapMouseMapsClickAndWheels(t *testing.T) {
	t.Parallel()

	click := app.ActionReceived{Action: app.SelectChat, ChatID: domain.ChatID(88), AvatarKey: "click"}
	wheelUp := app.ActionReceived{Action: app.PageUp, TargetFocus: app.FocusConversation, At: time.Unix(80, 1)}
	wheelDown := app.ActionReceived{Action: app.PageDown, MessageID: domain.MessageID(99), Rune: 'd'}
	hits := HitMap{{
		Rect:      image.Rect(4, 5, 10, 12),
		Click:     click,
		WheelUp:   wheelUp,
		WheelDown: wheelDown,
	}}

	for _, test := range []struct {
		name string
		id   string
		want app.ActionReceived
	}{
		{name: "click", id: "<MouseLeft>", want: click},
		{name: "wheel up", id: "<MouseWheelUp>", want: wheelUp},
		{name: "wheel down", id: "<MouseWheelDown>", want: wheelDown},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := gotui.Event{Type: gotui.MouseEvent, ID: test.id, Payload: gotui.Mouse{X: 6, Y: 8}}
			got, ok := MapMouse(event, hits)
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("MapMouse(%q) = (%#v, %t), want (%#v, true)", test.id, got, ok, test.want)
			}
		})
	}

	event := gotui.Event{Type: gotui.MouseEvent, ID: "<MouseLeft>", Payload: gotui.Mouse{X: 10, Y: 8}}
	if got, ok := MapMouse(event, hits); ok {
		t.Fatalf("MapMouse(outside) = (%#v, true), want no mapping", got)
	}
}
