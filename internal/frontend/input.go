package frontend

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

type surfaceState struct {
	mu   sync.RWMutex
	hits HitMap
}

func editableFocus(focus Focus) bool {
	return focus == FocusAuth || focus == FocusComposer || focus == FocusPhotoSend ||
		focus == FocusSearchInput || focus == FocusChatSearchInput || focus == FocusChatSettingsInput
}

func textInputAllowed(key tea.Key) bool {
	if key.Text == "" {
		return false
	}
	return key.Mod & ^tea.ModShift == 0
}

func mapKeyPress(focus Focus, msg tea.KeyPressMsg) (ActionReceived, bool) {
	key := msg.Key()
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod == tea.ModCtrl && (key.Code == 'c' || key.Code == 'C') {
		return actionReceived(Quit)
	}
	if key.Code == tea.KeyTab && key.Mod == tea.ModShift {
		return actionReceived(FocusPrevious)
	}
	if focus == FocusComposer && key.Code == tea.KeyEnter && key.Mod == tea.ModShift {
		return actionReceived(ComposerNewline)
	}

	// Composer media shortcuts are handled before generic editable rejection.
	if focus == FocusComposer && key.Mod == tea.ModCtrl && (key.Code == 'o' || key.Code == 'O') {
		return actionReceived(OpenPhotoSend)
	}
	if focus == FocusComposer && key.Mod == tea.ModCtrl && (key.Code == 's' || key.Code == 'S') {
		return actionReceived(OpenStickerPicker)
	}

	// Live global search navigates while the input stays focused: arrows move
	// the selection, Enter activates, Esc closes. Printable runes fall through
	// to the editable handler below so typing never leaves the input.
	if focus == FocusChatSearchInput && key.Mod == 0 {
		switch key.Code {
		case tea.KeyUp:
			return actionReceived(SelectPrevious)
		case tea.KeyDown:
			return actionReceived(SelectNext)
		case tea.KeyEnter:
			return actionReceived(Activate)
		case tea.KeyEscape:
			return actionReceived(Close)
		}
	}

	if editableFocus(focus) {
		if key.Mod != 0 {
			return ActionReceived{}, false
		}
		switch key.Code {
		case tea.KeyEnter:
			switch focus {
			case FocusComposer:
				return actionReceived(ComposerSubmit)
			case FocusPhotoSend:
				return actionReceived(PhotoSendSubmit)
			case FocusSearchInput:
				return actionReceived(SubmitMessageSearch)
			case FocusChatSearchInput:
				return actionReceived(Activate)
			case FocusChatSettingsInput:
				return actionReceived(SaveChatSetting)
			default:
				return actionReceived(ComposerSubmit)
			}
		case tea.KeyBackspace:
			return actionReceived(ComposerBackspace)
		case tea.KeyEscape:
			switch focus {
			case FocusPhotoSend:
				return actionReceived(Close)
			default:
				return actionReceived(Close)
			}
		default:
			return ActionReceived{}, false
		}
	}

	if focus == FocusStickerPicker && key.Mod == 0 {
		switch {
		case key.Code == tea.KeyEscape:
			return actionReceived(Close)
		case keyText(key) == "h" || key.Code == tea.KeyLeft:
			return actionReceived(StickerMoveLeft)
		case keyText(key) == "l" || key.Code == tea.KeyRight:
			return actionReceived(StickerMoveRight)
		case keyText(key) == "k" || key.Code == tea.KeyUp:
			return actionReceived(StickerMoveUp)
		case keyText(key) == "j" || key.Code == tea.KeyDown:
			return actionReceived(StickerMoveDown)
		case key.Code == tea.KeyEnter:
			return actionReceived(StickerActivate)
		default:
			return ActionReceived{}, false
		}
	}

	if isListNavigationFocus(focus) {
		// Forward and reaction pickers historically reject every modified key;
		// the other list surfaces let modified keys reach the global bindings.
		if key.Mod != 0 {
			if focus == FocusForwardPicker || focus == FocusReactionPicker {
				return ActionReceived{}, false
			}
		} else if received, ok := mapListNavigationKey(focus, key); ok {
			return received, true
		} else if focus != FocusModal {
			// Every list except the legacy generic modal owns unmodified keys that
			// it does not map. FocusModal intentionally retains global fallthrough.
			return ActionReceived{}, false
		}
	}
	if focus == FocusDetails && key.Mod == 0 && keyText(key) == "m" {
		return actionReceived(OpenMembers)
	}
	if focus == FocusDetails && key.Mod == 0 && keyText(key) == "l" {
		return actionReceived(OpenInviteLinks)
	}
	if focus == FocusChats && key.Mod == 0 {
		switch keyText(key) {
		case "/":
			return actionReceived(OpenChatSearch)
		case "u":
			return actionReceived(SelectNextUnread)
		case "m":
			return actionReceived(SelectNextMention)
		case "l":
			return actionReceived(OpenChat)
		}
		switch key.Code {
		case tea.KeyRight:
			return actionReceived(OpenChat)
		case tea.KeyEnter:
			return actionReceived(OpenChatActionMenu)
		}
	}
	if focus == FocusConversation && key.Mod == 0 {
		switch keyText(key) {
		case "/":
			return actionReceived(OpenMessageSearch)
		case "p":
			return actionReceived(OpenPinnedMessages)
		case "t":
			return actionReceived(OpenTopics)
		case "j":
			return actionReceived(SelectNextMessage)
		case "k":
			return actionReceived(SelectPreviousMessage)
		case "c":
			return actionReceived(CopyMessage)
		case "r":
			return actionReceived(ReplyMessage)
		case "e":
			return actionReceived(EditMessage)
		case "q":
			return actionReceived(Close)
		}
		if key.Code == tea.KeyEnter {
			return actionReceived(OpenMessageActionMenu)
		}
		if key.Code == tea.KeyDown {
			return actionReceived(SelectNextMessage)
		}
		if key.Code == tea.KeyUp {
			return actionReceived(SelectPreviousMessage)
		}
	}
	switch {
	case key.Mod == 0 && (key.Code == tea.KeyDown || keyText(key) == "j"):
		return actionReceived(SelectNext)
	case key.Mod == 0 && (key.Code == tea.KeyUp || keyText(key) == "k"):
		return actionReceived(SelectPrevious)
	case key.Mod == 0 && (key.Code == tea.KeyTab || key.Code == tea.KeyRight || keyText(key) == "l"):
		return actionReceived(FocusNext)
	case key.Mod == 0 && (key.Code == tea.KeyLeft || keyText(key) == "h"):
		return actionReceived(FocusPrevious)
	case key.Mod == 0 && key.Code == tea.KeyEnter:
		return actionReceived(Activate)
	case key.Mod == 0 && (keyText(key) == "i" || key.Code == tea.KeyF2):
		return actionReceived(ToggleDetails)
	case key.Mod == 0 && key.Code == tea.KeyEscape:
		return actionReceived(Close)
	case (key.Mod == tea.ModCtrl && keyText(key) == "u") || (key.Mod == 0 && key.Code == tea.KeyPgUp):
		return actionReceived(PageUp)
	case (key.Mod == tea.ModCtrl && keyText(key) == "d") || (key.Mod == 0 && key.Code == tea.KeyPgDown):
		return actionReceived(PageDown)
	default:
		return ActionReceived{}, false
	}
}

// isListNavigationFocus identifies the list-like surfaces that share the same
// close, previous, next, and activate controls. Keeping this list in one place
// makes a new ordinary list focus a one-entry frontend change.
func isListNavigationFocus(focus Focus) bool {
	switch focus {
	case FocusModal,
		FocusForwardPicker,
		FocusReactionPicker,
		FocusSearchResults,
		FocusChatSearchResults,
		FocusChatActions,
		FocusMembers,
		FocusInviteLinks,
		FocusAdministration,
		FocusPinnedResults,
		FocusTopics:
		return true
	default:
		return false
	}
}

// mapListNavigationKey maps the common unmodified list controls. Search result
// lists add their own slash action while retaining the shared navigation.
func mapListNavigationKey(focus Focus, key tea.Key) (ActionReceived, bool) {
	switch {
	case key.Code == tea.KeyEscape:
		return actionReceived(Close)
	case keyText(key) == "q" && focus != FocusForwardPicker && focus != FocusReactionPicker:
		return actionReceived(Close)
	case keyText(key) == "/" && focus == FocusSearchResults:
		return actionReceived(OpenMessageSearch)
	case keyText(key) == "/" && focus == FocusChatSearchResults:
		return actionReceived(OpenChatSearch)
	case keyText(key) == "j" || key.Code == tea.KeyDown:
		return actionReceived(SelectNext)
	case keyText(key) == "k" || key.Code == tea.KeyUp:
		return actionReceived(SelectPrevious)
	case key.Code == tea.KeyEnter:
		return actionReceived(Activate)
	default:
		return ActionReceived{}, false
	}
}

func mapCommandMenuKey(active bool, msg tea.KeyPressMsg) (ActionReceived, bool) {
	if !active {
		return ActionReceived{}, false
	}
	key := msg.Key()
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod != 0 {
		return ActionReceived{}, false
	}
	switch key.Code {
	case tea.KeyEscape:
		return actionReceived(CommandMenuDismiss)
	case tea.KeyEnter:
		return actionReceived(CommandMenuActivate)
	case tea.KeyUp:
		return actionReceived(CommandMenuPrevious)
	case tea.KeyDown, tea.KeyTab:
		return actionReceived(CommandMenuNext)
	default:
		return ActionReceived{}, false
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

func mapMouseClick(msg tea.MouseClickMsg, hits HitMap) (ActionReceived, bool) {
	if msg.Button != tea.MouseLeft {
		return ActionReceived{}, false
	}
	return hits.ActionAt(msg.X, msg.Y)
}

func mapMouseWheel(msg tea.MouseWheelMsg, hits HitMap) (ActionReceived, bool) {
	switch msg.Button {
	case tea.MouseWheelUp:
		return hits.WheelAt(msg.X, msg.Y, true)
	case tea.MouseWheelDown:
		return hits.WheelAt(msg.X, msg.Y, false)
	default:
		return ActionReceived{}, false
	}
}

func actionReceived(action Action) (ActionReceived, bool) {
	return ActionReceived{Action: action}, true
}

func (m AppModel) setHitRegions(hits HitMap) {
	if m.surface == nil {
		return
	}
	m.surface.mu.Lock()
	m.surface.hits = append(m.surface.hits[:0], hits...)
	m.surface.mu.Unlock()
}

func (m AppModel) hitRegions() HitMap {
	if m.surface == nil {
		return nil
	}
	m.surface.mu.RLock()
	hits := append(HitMap(nil), m.surface.hits...)
	m.surface.mu.RUnlock()
	return hits
}
