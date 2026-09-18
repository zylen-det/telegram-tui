package ui

import (
	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
)

func MapKey(focus app.Focus, event gotui.Event) (app.ActionReceived, bool) {
	if event.Type != gotui.KeyboardEvent {
		return app.ActionReceived{}, false
	}
	// Process quit is global, including while an editor owns focus.
	if event.ID == "<C-c>" {
		return keyAction(app.Quit)
	}
	if key, ok := event.Payload.(*tcell.EventKey); ok && key != nil {
		if key.Key() == tcell.KeyBacktab {
			return keyAction(app.FocusPrevious)
		}
		if focus == app.FocusComposer && key.Key() == tcell.KeyEnter && key.Modifiers()&tcell.ModShift != 0 {
			return keyAction(app.ComposerNewline)
		}
	}

	if focus == app.FocusAuth || focus == app.FocusComposer {
		switch event.ID {
		case "<Enter>":
			return keyAction(app.ComposerSubmit)
		case "<Backspace>":
			return keyAction(app.ComposerBackspace)
		case "<Escape>":
			return keyAction(app.Close)
		}

		runes := []rune(event.ID)
		if len(runes) == 1 {
			return app.ActionReceived{Rune: runes[0]}, true
		}
		return app.ActionReceived{}, false
	}

	if focus == app.FocusModal && event.ID == "q" {
		return keyAction(app.Close)
	}

	switch event.ID {
	case "j", "<Down>":
		return keyAction(app.SelectNext)
	case "k", "<Up>":
		return keyAction(app.SelectPrevious)
	case "l", "<Tab>":
		return keyAction(app.FocusNext)
	case "h":
		return keyAction(app.FocusPrevious)
	case "<Enter>":
		return keyAction(app.Activate)
	case "i", "<F2>":
		return keyAction(app.ToggleDetails)
	case "<Escape>":
		return keyAction(app.Close)
	case "<C-u>", "<PageUp>":
		return keyAction(app.PageUp)
	case "<C-d>", "<PageDown>":
		return keyAction(app.PageDown)
	default:
		return app.ActionReceived{}, false
	}
}

func MapMouse(event gotui.Event, hits HitMap) (app.ActionReceived, bool) {
	if event.Type != gotui.MouseEvent {
		return app.ActionReceived{}, false
	}
	mouse, ok := event.Payload.(gotui.Mouse)
	if !ok {
		return app.ActionReceived{}, false
	}

	switch event.ID {
	case "<MouseLeft>":
		return hits.ActionAt(mouse.X, mouse.Y)
	case "<MouseWheelUp>":
		return hits.WheelAt(mouse.X, mouse.Y, true)
	case "<MouseWheelDown>":
		return hits.WheelAt(mouse.X, mouse.Y, false)
	default:
		return app.ActionReceived{}, false
	}
}

func keyAction(action app.Action) (app.ActionReceived, bool) {
	return app.ActionReceived{Action: action}, true
}
