package frontend

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// huhTheme returns a Huh Theme whose focused/blurred field styles use the
// existing telegram-tui palette from this package, not Huh's default visual
// identity.
func huhTheme() huh.Theme {
	return telegramHuhTheme{}
}

type telegramHuhTheme struct{}

// Huh calls Theme for every rendered option, not just once per View. The
// palette is constant; rebuilding ThemeBase for each member and key repeat
// allocates megabytes per frame on long lists.
var telegramHuhStyles = newTelegramHuhStyles()

func (telegramHuhTheme) Theme(_ bool) *huh.Styles { return telegramHuhStyles }

func newTelegramHuhStyles() *huh.Styles {
	s := huh.ThemeBase(false)

	panel := rgba(panelColor)
	text := rgba(textColor)
	muted := rgba(mutedTextColor)
	accent := rgba(accentColor)
	err := rgba(errorColor)

	// Ordinary field surfaces use the terminal's default background. Focused
	// buttons and blurred buttons/cards keep their intentional fills.
	// ── Focused ──
	s.Focused.Base = lipgloss.NewStyle()
	s.Focused.Title = lipgloss.NewStyle().Foreground(accent).Bold(true)
	s.Focused.Description = lipgloss.NewStyle().Foreground(muted)
	s.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(err)
	s.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(err)
	s.Focused.SelectSelector = lipgloss.NewStyle().Foreground(accent).SetString("> ")
	s.Focused.Option = lipgloss.NewStyle().Foreground(text)
	s.Focused.NextIndicator = lipgloss.NewStyle().Foreground(accent)
	s.Focused.PrevIndicator = lipgloss.NewStyle().Foreground(accent)
	s.Focused.Directory = lipgloss.NewStyle().Foreground(text)
	s.Focused.File = lipgloss.NewStyle().Foreground(text)
	s.Focused.MultiSelectSelector = lipgloss.NewStyle().Foreground(accent)
	s.Focused.SelectedOption = lipgloss.NewStyle().Foreground(accent)
	s.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(accent)
	s.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(text)
	s.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(muted)
	s.Focused.FocusedButton = lipgloss.NewStyle().Foreground(text).Background(accent)
	s.Focused.BlurredButton = lipgloss.NewStyle().Foreground(text).Background(panel)
	s.Focused.Card = lipgloss.NewStyle().Foreground(text).Background(panel)
	s.Focused.NoteTitle = lipgloss.NewStyle().Foreground(text).Bold(true)
	s.Focused.Next = lipgloss.NewStyle().Foreground(accent)
	s.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(text)
	s.Focused.TextInput.CursorText = lipgloss.NewStyle().Foreground(text)
	s.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	s.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(text)
	s.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(text)

	// ── Blurred ──
	s.Blurred.Base = lipgloss.NewStyle()
	s.Blurred.Title = lipgloss.NewStyle().Foreground(text)
	s.Blurred.Description = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.ErrorIndicator = lipgloss.NewStyle().Foreground(err)
	s.Blurred.ErrorMessage = lipgloss.NewStyle().Foreground(err)
	s.Blurred.SelectSelector = lipgloss.NewStyle().Foreground(muted).SetString("> ")
	s.Blurred.Option = lipgloss.NewStyle().Foreground(text)
	s.Blurred.NextIndicator = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.PrevIndicator = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.Directory = lipgloss.NewStyle().Foreground(text)
	s.Blurred.File = lipgloss.NewStyle().Foreground(text)
	s.Blurred.MultiSelectSelector = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.SelectedOption = lipgloss.NewStyle().Foreground(accent)
	s.Blurred.SelectedPrefix = lipgloss.NewStyle().Foreground(accent)
	s.Blurred.UnselectedOption = lipgloss.NewStyle().Foreground(text)
	s.Blurred.UnselectedPrefix = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.FocusedButton = lipgloss.NewStyle().Foreground(text).Background(accent)
	s.Blurred.BlurredButton = lipgloss.NewStyle().Foreground(text).Background(panel)
	s.Blurred.Card = lipgloss.NewStyle().Foreground(text).Background(panel)
	s.Blurred.NoteTitle = lipgloss.NewStyle().Foreground(text)
	s.Blurred.Next = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.TextInput.Cursor = lipgloss.NewStyle().Foreground(text)
	s.Blurred.TextInput.CursorText = lipgloss.NewStyle().Foreground(text)
	s.Blurred.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	s.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(text)
	s.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(text)

	return s
}

// huhKeyMap returns a fresh explicit keymap suitable for embedded Huh fields.
// Huh mutates key-binding enabled states through WithPosition/WithKeyMap, so
// every component needs its own copy.
func huhKeyMap() *huh.KeyMap {
	return huh.NewDefaultKeyMap()
}
