package frontend

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// ─── C0: Huh foundation/adoption-proof tests ───

func TestHuhTextControlledUnicodePasteAndBackspace(t *testing.T) {
	var val string
	text := huh.NewText().Value(&val).WithTheme(huhTheme()).WithKeyMap(huhKeyMap()).(*huh.Text)

	// Focus
	text.Focus()

	// Initial value should be empty
	if got := val; got != "" {
		t.Fatalf("initial value = %q, want empty", got)
	}

	// Paste Unicode content via PasteMsg
	result, _ := text.Update(tea.PasteMsg{Content: "ab界🙂"})
	text = result.(*huh.Text)
	if got := val; got != "ab界🙂" {
		t.Fatalf("after paste = %q, want %q", got, "ab界🙂")
	}

	// Backspace should remove the last grapheme (grapheme-aware)
	result, _ = text.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	text = result.(*huh.Text)
	if got := val; got != "ab界" {
		t.Fatalf("after backspace = %q, want %q", got, "ab界")
	}

	// 界 is a single Unicode code point encoded as three UTF-8 bytes — one backspace removes it
	result, _ = text.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	text = result.(*huh.Text)
	if got := val; got != "ab" {
		t.Fatalf("after grapheme backspace = %q, want %q", got, "ab")
	}

	// Verify bound string synchronizes
	if got := val; got != "ab" {
		t.Fatalf("bound sync = %q, want %q", got, "ab")
	}

	// View must not be empty and must reflect the current bound value
	view := text.View()
	if strings.TrimSpace(view) == "" {
		t.Fatal("Text View returned empty string")
	}
	if !strings.Contains(view, "ab") {
		t.Fatalf("Text View should contain current value %q: %q", val, view)
	}
}

func TestHuhInputPasswordMasksSecret(t *testing.T) {
	var secret string
	input := huh.NewInput().Value(&secret).WithTheme(huhTheme()).WithKeyMap(huhKeyMap()).(*huh.Input)

	// Use EchoModePassword (not deprecated .Password(true))
	input.EchoMode(huh.EchoModePassword)

	// Focus so the field accepts key events
	input.Focus()

	// Enter the secret: mix of KeyPressMsg and PasteMsg
	result, _ := input.Update(tea.KeyPressMsg(tea.Key{Text: "s3cretP@ss"}))
	input = result.(*huh.Input)
	result, _ = input.Update(tea.PasteMsg{Content: "!🔑"})
	input = result.(*huh.Input)

	// Bound value must hold the entered secret
	if got := secret; got != "s3cretP@ss!🔑" {
		t.Fatalf("bound secret = %q, want %q", got, "s3cretP@ss!🔑")
	}

	// ANSI-strip and assert rendering
	view := input.View()
	plain := ansiSequence.ReplaceAllString(view, "")
	if strings.Contains(plain, "s3cretP@ss") {
		t.Fatal("password ANSI-stripped view leaks plaintext secret")
	}
	if strings.Contains(plain, "🔑") {
		t.Fatal("password ANSI-stripped view leaks distinctive secret Unicode")
	}
	// Non-empty masked rendering must be present (not empty-string pass)
	if strings.TrimSpace(plain) == "" {
		t.Fatal("password view has no masked rendering at all")
	}
}

func TestHuhSelectExplicitKeyMapValueAndGeometry(t *testing.T) {
	var val string
	// Select with no keymap is inert for navigation; explicit keymap enables it.
	selNoKM := huh.NewSelect[string]().
		Options(
			huh.NewOption("a", "Alpha"),
			huh.NewOption("b", "Bravo"),
			huh.NewOption("c", "Charlie"),
		).
		Value(&val).
		WithTheme(huhTheme()).(*huh.Select[string])

	// Without keymap, Select navigation via Down is inert
	_ = selNoKM.Focus()
	_, _ = selNoKM.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if got := val; got != "Alpha" {
		t.Fatalf("select without keymap Down should be inert: value=%q", got)
	}

	// With explicit keymap, Down should advance the bound value
	sel := huh.NewSelect[string]().
		Options(
			huh.NewOption("a", "Alpha"),
			huh.NewOption("b", "Bravo"),
			huh.NewOption("c", "Charlie"),
		).
		Value(&val).
		WithTheme(huhTheme()).
		WithKeyMap(huhKeyMap()).(*huh.Select[string])
	// Height() is on the concrete type, not on huh.Field.
	sel.Height(2)

	sel.Focus()

	// Down navigates to next option (bound value holds Option.Value)
	result, _ := sel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	sel = result.(*huh.Select[string])
	if got := val; got != "Bravo" {
		t.Fatalf("first down: value=%q, want %q", got, "Bravo")
	}

	// Another down to last option
	result, _ = sel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	sel = result.(*huh.Select[string])
	if got := val; got != "Charlie" {
		t.Fatalf("second down: value=%q, want %q", got, "Charlie")
	}

	// View must not be empty
	view := sel.View()
	if strings.TrimSpace(view) == "" {
		t.Fatal("Select View returned empty string")
	}

	// Height(2) sets outer geometry — prove it via lipgloss v2 measurement.
	if got := lipgloss.Height(view); got != 2 {
		t.Fatalf("Select outer geometry height = %d, want 2", got)
	}
}

// huhStyleContract is one entry of the Huh theme contract: the exact palette
// foreground and background a field style must carry, and whether it is bold.
// lipgloss.NoColor{} means "leave it unset" so the terminal's own colors show
// through.
type huhStyleContract struct {
	name       string
	style      lipgloss.Style
	foreground color.Color
	background color.Color
	bold       bool
}

// huhThemeContract flattens both field states into the complete palette
// contract derived from the telegram-tui palette in styles.go. It is the
// authority for the Huh half of the visual language: ordinary field surfaces
// keep the terminal's default background, while focused buttons, blurred
// buttons, and cards keep their intentional fills.
func huhThemeContract(theme *huh.Styles) []huhStyleContract {
	unset := lipgloss.NoColor{}
	text := rgba(textColor)
	muted := rgba(mutedTextColor)
	accent := rgba(accentColor)
	failure := rgba(errorColor)
	panel := rgba(panelColor)

	focused := []huhStyleContract{
		{name: "focused base", style: theme.Focused.Base, foreground: unset, background: unset},
		{name: "focused title", style: theme.Focused.Title, foreground: accent, background: unset, bold: true},
		{name: "focused description", style: theme.Focused.Description, foreground: muted, background: unset},
		{name: "focused error indicator", style: theme.Focused.ErrorIndicator, foreground: failure, background: unset},
		{name: "focused error message", style: theme.Focused.ErrorMessage, foreground: failure, background: unset},
		{name: "focused select selector", style: theme.Focused.SelectSelector, foreground: accent, background: unset},
		{name: "focused option", style: theme.Focused.Option, foreground: text, background: unset},
		{name: "focused next indicator", style: theme.Focused.NextIndicator, foreground: accent, background: unset},
		{name: "focused prev indicator", style: theme.Focused.PrevIndicator, foreground: accent, background: unset},
		{name: "focused directory", style: theme.Focused.Directory, foreground: text, background: unset},
		{name: "focused file", style: theme.Focused.File, foreground: text, background: unset},
		{name: "focused multiselect selector", style: theme.Focused.MultiSelectSelector, foreground: accent, background: unset},
		{name: "focused selected option", style: theme.Focused.SelectedOption, foreground: accent, background: unset},
		{name: "focused selected prefix", style: theme.Focused.SelectedPrefix, foreground: accent, background: unset},
		{name: "focused unselected option", style: theme.Focused.UnselectedOption, foreground: text, background: unset},
		{name: "focused unselected prefix", style: theme.Focused.UnselectedPrefix, foreground: muted, background: unset},
		{name: "focused textinput cursor", style: theme.Focused.TextInput.Cursor, foreground: text, background: unset},
		{name: "focused textinput cursor text", style: theme.Focused.TextInput.CursorText, foreground: text, background: unset},
		{name: "focused textinput placeholder", style: theme.Focused.TextInput.Placeholder, foreground: muted, background: unset},
		{name: "focused textinput prompt", style: theme.Focused.TextInput.Prompt, foreground: text, background: unset},
		{name: "focused textinput text", style: theme.Focused.TextInput.Text, foreground: text, background: unset},
		{name: "focused focused button", style: theme.Focused.FocusedButton, foreground: text, background: accent},
		{name: "focused blurred button", style: theme.Focused.BlurredButton, foreground: text, background: panel},
		{name: "focused card", style: theme.Focused.Card, foreground: text, background: panel},
		{name: "focused note title", style: theme.Focused.NoteTitle, foreground: text, background: unset, bold: true},
		{name: "focused next", style: theme.Focused.Next, foreground: accent, background: unset},
	}
	blurred := []huhStyleContract{
		{name: "blurred base", style: theme.Blurred.Base, foreground: unset, background: unset},
		{name: "blurred title", style: theme.Blurred.Title, foreground: text, background: unset},
		{name: "blurred description", style: theme.Blurred.Description, foreground: muted, background: unset},
		{name: "blurred error indicator", style: theme.Blurred.ErrorIndicator, foreground: failure, background: unset},
		{name: "blurred error message", style: theme.Blurred.ErrorMessage, foreground: failure, background: unset},
		{name: "blurred select selector", style: theme.Blurred.SelectSelector, foreground: muted, background: unset},
		{name: "blurred option", style: theme.Blurred.Option, foreground: text, background: unset},
		{name: "blurred next indicator", style: theme.Blurred.NextIndicator, foreground: muted, background: unset},
		{name: "blurred prev indicator", style: theme.Blurred.PrevIndicator, foreground: muted, background: unset},
		{name: "blurred directory", style: theme.Blurred.Directory, foreground: text, background: unset},
		{name: "blurred file", style: theme.Blurred.File, foreground: text, background: unset},
		{name: "blurred multiselect selector", style: theme.Blurred.MultiSelectSelector, foreground: muted, background: unset},
		{name: "blurred selected option", style: theme.Blurred.SelectedOption, foreground: accent, background: unset},
		{name: "blurred selected prefix", style: theme.Blurred.SelectedPrefix, foreground: accent, background: unset},
		{name: "blurred unselected option", style: theme.Blurred.UnselectedOption, foreground: text, background: unset},
		{name: "blurred unselected prefix", style: theme.Blurred.UnselectedPrefix, foreground: muted, background: unset},
		{name: "blurred textinput cursor", style: theme.Blurred.TextInput.Cursor, foreground: text, background: unset},
		{name: "blurred textinput cursor text", style: theme.Blurred.TextInput.CursorText, foreground: text, background: unset},
		{name: "blurred textinput placeholder", style: theme.Blurred.TextInput.Placeholder, foreground: muted, background: unset},
		{name: "blurred textinput prompt", style: theme.Blurred.TextInput.Prompt, foreground: text, background: unset},
		{name: "blurred textinput text", style: theme.Blurred.TextInput.Text, foreground: text, background: unset},
		{name: "blurred focused button", style: theme.Blurred.FocusedButton, foreground: text, background: accent},
		{name: "blurred blurred button", style: theme.Blurred.BlurredButton, foreground: text, background: panel},
		{name: "blurred card", style: theme.Blurred.Card, foreground: text, background: panel},
		{name: "blurred note title", style: theme.Blurred.NoteTitle, foreground: text, background: unset},
		{name: "blurred next", style: theme.Blurred.Next, foreground: muted, background: unset},
	}
	return append(focused, blurred...)
}

func TestHuhThemeUsesTelegramPalette(t *testing.T) {
	// Every field style uses the telegram palette: exact foreground, and a
	// background only for the intentional focused-button, blurred-button, and
	// card fills. The palette is also independent of the terminal's own light or
	// dark background, so Huh's dark-background flag must not change a style.
	for _, darkBackground := range []bool{false, true} {
		for _, contract := range huhThemeContract(huhTheme().Theme(darkBackground)) {
			if got := contract.style.GetForeground(); got != contract.foreground {
				t.Errorf("darkBg=%v %s foreground = %#v, want %#v", darkBackground, contract.name, got, contract.foreground)
			}
			if got := contract.style.GetBackground(); got != contract.background {
				t.Errorf("darkBg=%v %s background = %#v, want %#v", darkBackground, contract.name, got, contract.background)
			}
			if got := contract.style.GetBold(); got != contract.bold {
				t.Errorf("darkBg=%v %s bold = %v, want %v", darkBackground, contract.name, got, contract.bold)
			}
		}
	}

	theme := huhTheme().Theme(false)

	// Selection is communicated by accent colour, never by font weight: the
	// selected option must differ from the unselected one while both stay on the
	// terminal's default background.
	if colorOf(theme.Focused.SelectedOption.GetForeground()) == colorOf(theme.Focused.UnselectedOption.GetForeground()) {
		t.Error("focused selected and unselected options must be distinguishable")
	}
	if theme.Focused.SelectedOption.GetBold() || theme.Focused.UnselectedOption.GetBold() {
		t.Error("select option weight must not carry selection")
	}

	// The select cursor indicator is the literal "> " in both states.
	const indicator = "> "
	for _, style := range []lipgloss.Style{theme.Focused.SelectSelector, theme.Blurred.SelectSelector} {
		if got := style.String(); !strings.Contains(got, indicator) {
			t.Errorf("select selector indicator = %q, want it to carry %q", got, indicator)
		}
	}
}
