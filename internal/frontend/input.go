package frontend

import (
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

type surfaceState struct {
	mu   sync.RWMutex
	hits ui.HitMap
}

func editableFocus(focus app.Focus) bool {
	return focus == app.FocusAuth || focus == app.FocusComposer || focus == app.FocusPhotoSend ||
		focus == app.FocusSearchInput || focus == app.FocusChatSearchInput || focus == app.FocusChatSettingsInput
}

func textInputAllowed(key tea.Key) bool {
	if key.Text == "" {
		return false
	}
	return key.Mod & ^tea.ModShift == 0
}

func mapKeyPress(focus app.Focus, msg tea.KeyPressMsg) (app.ActionReceived, bool) {
	key := msg.Key()
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod == tea.ModCtrl && (key.Code == 'c' || key.Code == 'C') {
		return actionReceived(app.Quit)
	}
	if key.Code == tea.KeyTab && key.Mod == tea.ModShift {
		return actionReceived(app.FocusPrevious)
	}
	if focus == app.FocusComposer && key.Code == tea.KeyEnter && key.Mod == tea.ModShift {
		return actionReceived(app.ComposerNewline)
	}

	// Composer media shortcuts are handled before generic editable rejection.
	if focus == app.FocusComposer && key.Mod == tea.ModCtrl && (key.Code == 'o' || key.Code == 'O') {
		return actionReceived(app.OpenPhotoSend)
	}
	if focus == app.FocusComposer && key.Mod == tea.ModCtrl && (key.Code == 's' || key.Code == 'S') {
		return actionReceived(app.OpenStickerPicker)
	}

	// Live global search navigates while the input stays focused: arrows move
	// the selection, Enter activates, Esc closes. Printable runes fall through
	// to the editable handler below so typing never leaves the input.
	if focus == app.FocusChatSearchInput && key.Mod == 0 {
		switch key.Code {
		case tea.KeyUp:
			return actionReceived(app.SelectPrevious)
		case tea.KeyDown:
			return actionReceived(app.SelectNext)
		case tea.KeyEnter:
			return actionReceived(app.Activate)
		case tea.KeyEscape:
			return actionReceived(app.Close)
		}
	}

	if editableFocus(focus) {
		if key.Mod != 0 {
			return app.ActionReceived{}, false
		}
		switch key.Code {
		case tea.KeyEnter:
			switch focus {
			case app.FocusComposer:
				return actionReceived(app.ComposerSubmit)
			case app.FocusPhotoSend:
				return actionReceived(app.PhotoSendSubmit)
			case app.FocusSearchInput:
				return actionReceived(app.SubmitMessageSearch)
			case app.FocusChatSearchInput:
				return actionReceived(app.Activate)
			case app.FocusChatSettingsInput:
				return actionReceived(app.SaveChatSetting)
			default:
				return actionReceived(app.ComposerSubmit)
			}
		case tea.KeyBackspace:
			return actionReceived(app.ComposerBackspace)
		case tea.KeyEscape:
			switch focus {
			case app.FocusPhotoSend:
				return actionReceived(app.Close)
			default:
				return actionReceived(app.Close)
			}
		default:
			return app.ActionReceived{}, false
		}
	}

	if focus == app.FocusStickerPicker && key.Mod == 0 {
		switch {
		case key.Code == tea.KeyEscape:
			return actionReceived(app.Close)
		case keyText(key) == "h" || key.Code == tea.KeyLeft:
			return actionReceived(app.StickerMoveLeft)
		case keyText(key) == "l" || key.Code == tea.KeyRight:
			return actionReceived(app.StickerMoveRight)
		case keyText(key) == "k" || key.Code == tea.KeyUp:
			return actionReceived(app.StickerMoveUp)
		case keyText(key) == "j" || key.Code == tea.KeyDown:
			return actionReceived(app.StickerMoveDown)
		case key.Code == tea.KeyEnter:
			return actionReceived(app.StickerActivate)
		default:
			return app.ActionReceived{}, false
		}
	}

	if isListNavigationFocus(focus) {
		// Forward and reaction pickers historically reject every modified key;
		// the other list surfaces let modified keys reach the global bindings.
		if key.Mod != 0 {
			if focus == app.FocusForwardPicker || focus == app.FocusReactionPicker {
				return app.ActionReceived{}, false
			}
		} else if received, ok := mapListNavigationKey(focus, key); ok {
			return received, true
		} else if focus != app.FocusModal {
			// Every list except the legacy generic modal owns unmodified keys that
			// it does not map. FocusModal intentionally retains global fallthrough.
			return app.ActionReceived{}, false
		}
	}
	if focus == app.FocusDetails && key.Mod == 0 && keyText(key) == "m" {
		return actionReceived(app.OpenMembers)
	}
	if focus == app.FocusDetails && key.Mod == 0 && keyText(key) == "l" {
		return actionReceived(app.OpenInviteLinks)
	}
	if focus == app.FocusChats && key.Mod == 0 {
		switch keyText(key) {
		case "/":
			return actionReceived(app.OpenChatSearch)
		case "u":
			return actionReceived(app.SelectNextUnread)
		case "m":
			return actionReceived(app.SelectNextMention)
		}
	}
	if focus == app.FocusConversation && key.Mod == 0 {
		switch keyText(key) {
		case "/":
			return actionReceived(app.OpenMessageSearch)
		case "p":
			return actionReceived(app.OpenPinnedMessages)
		case "t":
			return actionReceived(app.OpenTopics)
		case "j":
			return actionReceived(app.SelectNextMessage)
		case "k":
			return actionReceived(app.SelectPreviousMessage)
		case "c":
			return actionReceived(app.CopyMessage)
		case "r":
			return actionReceived(app.ReplyMessage)
		case "e":
			return actionReceived(app.EditMessage)
		case "q":
			return actionReceived(app.Close)
		}
		if key.Code == tea.KeyEnter {
			return actionReceived(app.OpenMessageActionMenu)
		}
		if key.Code == tea.KeyDown {
			return actionReceived(app.SelectNextMessage)
		}
		if key.Code == tea.KeyUp {
			return actionReceived(app.SelectPreviousMessage)
		}
	}
	switch {
	case key.Mod == 0 && (key.Code == tea.KeyDown || keyText(key) == "j"):
		return actionReceived(app.SelectNext)
	case key.Mod == 0 && (key.Code == tea.KeyUp || keyText(key) == "k"):
		return actionReceived(app.SelectPrevious)
	case key.Mod == 0 && (key.Code == tea.KeyTab || key.Code == tea.KeyRight || keyText(key) == "l"):
		return actionReceived(app.FocusNext)
	case key.Mod == 0 && (key.Code == tea.KeyLeft || keyText(key) == "h"):
		return actionReceived(app.FocusPrevious)
	case key.Mod == 0 && key.Code == tea.KeyEnter:
		return actionReceived(app.Activate)
	case key.Mod == 0 && (keyText(key) == "i" || key.Code == tea.KeyF2):
		return actionReceived(app.ToggleDetails)
	case key.Mod == 0 && key.Code == tea.KeyEscape:
		return actionReceived(app.Close)
	case (key.Mod == tea.ModCtrl && keyText(key) == "u") || (key.Mod == 0 && key.Code == tea.KeyPgUp):
		return actionReceived(app.PageUp)
	case (key.Mod == tea.ModCtrl && keyText(key) == "d") || (key.Mod == 0 && key.Code == tea.KeyPgDown):
		return actionReceived(app.PageDown)
	default:
		return app.ActionReceived{}, false
	}
}

// isListNavigationFocus identifies the list-like surfaces that share the same
// close, previous, next, and activate controls. Keeping this list in one place
// makes a new ordinary list focus a one-entry frontend change.
func isListNavigationFocus(focus app.Focus) bool {
	switch focus {
	case app.FocusModal,
		app.FocusForwardPicker,
		app.FocusReactionPicker,
		app.FocusSearchResults,
		app.FocusChatSearchResults,
		app.FocusChatActions,
		app.FocusMembers,
		app.FocusInviteLinks,
		app.FocusAdministration,
		app.FocusPinnedResults,
		app.FocusTopics:
		return true
	default:
		return false
	}
}

// mapListNavigationKey maps the common unmodified list controls. Search result
// lists add their own slash action while retaining the shared navigation.
func mapListNavigationKey(focus app.Focus, key tea.Key) (app.ActionReceived, bool) {
	switch {
	case key.Code == tea.KeyEscape:
		return actionReceived(app.Close)
	case keyText(key) == "q" && focus != app.FocusForwardPicker && focus != app.FocusReactionPicker:
		return actionReceived(app.Close)
	case keyText(key) == "/" && focus == app.FocusSearchResults:
		return actionReceived(app.OpenMessageSearch)
	case keyText(key) == "/" && focus == app.FocusChatSearchResults:
		return actionReceived(app.OpenChatSearch)
	case keyText(key) == "j" || key.Code == tea.KeyDown:
		return actionReceived(app.SelectNext)
	case keyText(key) == "k" || key.Code == tea.KeyUp:
		return actionReceived(app.SelectPrevious)
	case key.Code == tea.KeyEnter:
		return actionReceived(app.Activate)
	default:
		return app.ActionReceived{}, false
	}
}

func mapCommandMenuKey(active bool, msg tea.KeyPressMsg) (app.ActionReceived, bool) {
	if !active {
		return app.ActionReceived{}, false
	}
	key := msg.Key()
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod != 0 {
		return app.ActionReceived{}, false
	}
	switch key.Code {
	case tea.KeyEscape:
		return actionReceived(app.CommandMenuDismiss)
	case tea.KeyEnter:
		return actionReceived(app.CommandMenuActivate)
	case tea.KeyUp:
		return actionReceived(app.CommandMenuPrevious)
	case tea.KeyDown, tea.KeyTab:
		return actionReceived(app.CommandMenuNext)
	default:
		return app.ActionReceived{}, false
	}
}

func keyText(key tea.Key) string {
	if key.Text != "" {
		return key.Text
	}
	if key.Code >= 0 && key.Code <= 0x7f {
		return string(key.Code)
	}
	return ""
}

func mapMouseClick(msg tea.MouseClickMsg, hits ui.HitMap) (app.ActionReceived, bool) {
	if msg.Button != tea.MouseLeft {
		return app.ActionReceived{}, false
	}
	return hits.ActionAt(msg.X, msg.Y)
}

func mapMouseWheel(msg tea.MouseWheelMsg, hits ui.HitMap) (app.ActionReceived, bool) {
	switch msg.Button {
	case tea.MouseWheelUp:
		return hits.WheelAt(msg.X, msg.Y, true)
	case tea.MouseWheelDown:
		return hits.WheelAt(msg.X, msg.Y, false)
	default:
		return app.ActionReceived{}, false
	}
}

func actionReceived(action app.Action) (app.ActionReceived, bool) {
	return app.ActionReceived{Action: action}, true
}

func (m AppModel) setHitRegions(hits ui.HitMap) {
	if m.surface == nil {
		return
	}
	m.surface.mu.Lock()
	m.surface.hits = append(m.surface.hits[:0], hits...)
	m.surface.mu.Unlock()
}

func (m AppModel) hitRegions() ui.HitMap {
	if m.surface == nil {
		return nil
	}
	m.surface.mu.RLock()
	hits := append(ui.HitMap(nil), m.surface.hits...)
	m.surface.mu.RUnlock()
	return hits
}
