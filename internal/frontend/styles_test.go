package frontend

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// This file is the focused authority for the global visual contract of
// renderStyles, i.e. newRenderStyles in styles.go:
//
//   - every semantic style carries only its exact palette foreground;
//   - ordinary surfaces leave the background unset so the terminal canvas and
//     the terminal's own background show through;
//   - the only intentional renderStyles fill is the selected-row background;
//   - dim=true additionally marks every style Faint while preserving the
//     palette, so an undimmed overlay modal can sit above the dimmed
//     application.
//
// Feature tests assert their own geometry, content, border/selection
// distinction, clipping, cursor, interaction, and hit parity instead of
// restating these palette and default-background details.

// styleRole is one semantic renderStyles role together with the palette entry
// it must render as its foreground.
type styleRole struct {
	name  string
	style lipgloss.Style
	rgb   rgb
}

// renderStylesRoles enumerates every semantic role so each contract below
// covers the whole style set instead of a hand-picked subset.
func renderStylesRoles(styles renderStyles) []styleRole {
	return []styleRole{
		{name: "Base", style: styles.Base, rgb: textColor},
		{name: "Status", style: styles.Status, rgb: textColor},
		{name: "Panel", style: styles.Panel, rgb: textColor},
		{name: "Muted", style: styles.Muted, rgb: mutedTextColor},
		{name: "Emphasis", style: styles.Emphasis, rgb: textColor},
		{name: "Accent", style: styles.Accent, rgb: accentColor},
		{name: "Border", style: styles.Border, rgb: borderColor},
		{name: "FocusedBorder", style: styles.FocusedBorder, rgb: focusedBorderColor},
		{name: "Title", style: styles.Title, rgb: textColor},
		{name: "Input", style: styles.Input, rgb: textColor},
		{name: "Warning", style: styles.Warning, rgb: warningColor},
		{name: "Error", style: styles.Error, rgb: errorColor},
		{name: "Selected", style: styles.Selected, rgb: textColor},
	}
}

// roleStyle returns the style of one semantic role by name.
func roleStyle(styles renderStyles, name string) lipgloss.Style {
	for _, role := range renderStylesRoles(styles) {
		if role.name == name {
			return role.style
		}
	}
	panic("unknown renderStyles role " + name)
}

// emittedBackgrounds returns every truecolor background triple the rendered
// string sets. Lipgloss groups several SGR parameters into one escape sequence
// ("\x1b[38;2;r;g;b;48;2;r;g;bm"), so the parameters are walked instead of the
// whole sequence being matched.
func emittedBackgrounds(rendered string) []string {
	var backgrounds []string
	for _, sequence := range ansiSequence.FindAllString(rendered, -1) {
		parameters := strings.Split(strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), "m"), ";")
		for index := 0; index+4 < len(parameters); index++ {
			if parameters[index] == "48" && parameters[index+1] == "2" {
				backgrounds = append(backgrounds, strings.Join(parameters[index+2:index+5], ";"))
			}
		}
	}
	return backgrounds
}

// backgroundTriple renders one palette entry the way Lipgloss writes it as an
// SGR background parameter.
func backgroundTriple(value rgb) string {
	return fmt.Sprintf("%d;%d;%d", value.r, value.g, value.b)
}

func TestRenderStylesCarryExactPaletteForegrounds(t *testing.T) {
	for _, dim := range []bool{false, true} {
		for _, role := range renderStylesRoles(newRenderStyles(dim)) {
			if got := colorOf(role.style.GetForeground()); got != rgba(role.rgb) {
				t.Errorf("dim=%v %s foreground = %v, want %v", dim, role.name, got, rgba(role.rgb))
			}
		}
	}

	styles := newRenderStyles(false)
	// Emphasis, Accent, and Title are the only bold text roles. Selection is
	// carried by the intentional fill, never by font weight.
	for _, role := range renderStylesRoles(styles) {
		wantBold := role.name == "Emphasis" || role.name == "Accent" || role.name == "Title"
		if role.style.GetBold() != wantBold {
			t.Errorf("%s bold = %v, want %v", role.name, role.style.GetBold(), wantBold)
		}
	}

	// The unfocused and focused border roles must stay distinguishable, and
	// Error must keep serving as the offline color.
	if colorOf(styles.Border.GetForeground()) == colorOf(styles.FocusedBorder.GetForeground()) {
		t.Error("Border and FocusedBorder must use different palette colors")
	}
	if colorOf(styles.Error.GetForeground()) != colorOf(rgba(offlineColor)) {
		t.Errorf("Error foreground = %v, want the offline color %v", colorOf(styles.Error.GetForeground()), rgba(offlineColor))
	}
}

func TestRenderStylesOrdinarySurfacesUseTerminalDefaultBackground(t *testing.T) {
	for _, dim := range []bool{false, true} {
		for _, role := range renderStylesRoles(newRenderStyles(dim)) {
			if role.name == "Selected" {
				continue
			}
			if got := role.style.GetBackground(); got != (lipgloss.NoColor{}) {
				t.Errorf("dim=%v %s background = %#v, want no color (terminal default)", dim, role.name, got)
			}
			// An unset background must emit no SGR background parameter at all so
			// the terminal's own background shows through.
			if got := emittedBackgrounds(role.style.Render("x")); len(got) != 0 {
				t.Errorf("dim=%v %s emitted background %v, want none (terminal default)", dim, role.name, got)
			}
		}
	}
}

func TestRenderStylesSelectedIsTheOnlyIntentionalFill(t *testing.T) {
	fill := backgroundTriple(selectedColor)
	for _, dim := range []bool{false, true} {
		styles := newRenderStyles(dim)
		if got := colorOf(styles.Selected.GetBackground()); got != rgba(selectedColor) {
			t.Errorf("dim=%v Selected background = %v, want %v", dim, got, rgba(selectedColor))
		}
		if got := emittedBackgrounds(styles.Selected.Render("x")); len(got) != 1 || got[0] != fill {
			t.Errorf("dim=%v Selected emitted backgrounds %v, want exactly the intentional fill %q", dim, got, fill)
		}
		for _, role := range renderStylesRoles(styles) {
			if role.name == "Selected" {
				continue
			}
			if got := emittedBackgrounds(role.style.Render("x")); len(got) != 0 {
				t.Errorf("dim=%v %s emitted background %v, want no fill outside Selected", dim, role.name, got)
			}
		}
	}
}

func TestRenderStylesDimFaintsEveryRoleAndPreservesPalette(t *testing.T) {
	dim := newRenderStyles(true)
	plain := newRenderStyles(false)

	if !dim.Dim {
		t.Error("newRenderStyles(true).Dim = false, want true")
	}
	if plain.Dim {
		t.Error("newRenderStyles(false).Dim = true, want false")
	}

	for _, role := range renderStylesRoles(dim) {
		if !role.style.GetFaint() {
			t.Errorf("dim %s should be Faint so an undimmed overlay modal stands out", role.name)
		}
	}
	for _, role := range renderStylesRoles(plain) {
		if role.style.GetFaint() {
			t.Errorf("plain %s should not be Faint", role.name)
		}
	}

	// Dimming adds Faint only: the foreground palette and the intentional
	// selected fill survive unchanged.
	for _, role := range renderStylesRoles(dim) {
		undimmed := roleStyle(plain, role.name)
		if got, want := colorOf(role.style.GetForeground()), colorOf(undimmed.GetForeground()); got != want {
			t.Errorf("dim %s foreground = %v, want the undimmed palette %v", role.name, got, want)
		}
		if got, want := colorOf(role.style.GetBackground()), colorOf(undimmed.GetBackground()); got != want {
			t.Errorf("dim %s background = %v, want the undimmed background %v", role.name, got, want)
		}
	}
}
