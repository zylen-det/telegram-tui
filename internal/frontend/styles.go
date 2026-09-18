package frontend

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

const (
	minimumWidth  = 60
	minimumHeight = 18
)

type rgb struct {
	r uint8
	g uint8
	b uint8
}

var (
	panelColor         = rgb{21, 24, 28}
	selectedColor      = rgb{38, 48, 55}
	borderColor        = rgb{92, 103, 111}
	focusedBorderColor = rgb{73, 176, 196}
	textColor          = rgb{225, 230, 232}
	mutedTextColor     = rgb{145, 155, 160}
	accentColor        = rgb{111, 202, 162}
	warningColor       = rgb{231, 181, 85}
	errorColor         = rgb{230, 106, 111}
	offlineColor       = errorColor
)

// renderStyles is the semantic Lipgloss style set used by the compositing
// renderer. Styles carry only exact RGB palette foreground colors; ordinary
// surfaces leave the terminal background unset. The Dim flag selects whether
// newRenderStyles makes the base surface styles Faint so an overlay modal can
// sit above an undimmed application.
type renderStyles struct {
	Base, Status, Panel, Muted, Emphasis, Accent lipgloss.Style
	Border, FocusedBorder, Title, Input          lipgloss.Style
	Warning, Error, Selected                     lipgloss.Style
	Dim                                          bool
}

// newRenderStyles builds the semantic Lipgloss style set from the current RGB
// palette. No style sets a background except the intentional Selected fill,
// so ordinary surfaces always use the terminal's default background. When dim
// is true every base-pane/text/avatar style is additionally set to Faint so a
// modal can be placed above an undimmed overlay without erasing the underlying
// cells.
func newRenderStyles(dim bool) renderStyles {
	base := lipgloss.NewStyle().Foreground(rgba(textColor))
	panel := lipgloss.NewStyle().Foreground(rgba(textColor))
	muted := lipgloss.NewStyle().Foreground(rgba(mutedTextColor))
	emphasis := lipgloss.NewStyle().Foreground(rgba(textColor)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(rgba(accentColor)).Bold(true)
	border := lipgloss.NewStyle().Foreground(rgba(borderColor))
	focusedBorder := lipgloss.NewStyle().Foreground(rgba(focusedBorderColor))
	title := lipgloss.NewStyle().Foreground(rgba(textColor)).Bold(true)
	input := lipgloss.NewStyle().Foreground(rgba(textColor))
	warning := lipgloss.NewStyle().Foreground(rgba(warningColor))
	errorStyle := lipgloss.NewStyle().Foreground(rgba(errorColor))
	selected := lipgloss.NewStyle().
		Foreground(rgba(textColor)).
		Background(rgba(selectedColor))

	styles := renderStyles{
		Base:          base,
		Status:        base,
		Panel:         panel,
		Muted:         muted,
		Emphasis:      emphasis,
		Accent:        accent,
		Border:        border,
		FocusedBorder: focusedBorder,
		Title:         title,
		Input:         input,
		Warning:       warning,
		Error:         errorStyle,
		Selected:      selected,
		Dim:           dim,
	}
	if dim {
		styles.Base = styles.Base.Faint(true)
		styles.Status = styles.Status.Faint(true)
		styles.Panel = styles.Panel.Faint(true)
		styles.Muted = styles.Muted.Faint(true)
		styles.Emphasis = styles.Emphasis.Faint(true)
		styles.Accent = styles.Accent.Faint(true)
		styles.Border = styles.Border.Faint(true)
		styles.FocusedBorder = styles.FocusedBorder.Faint(true)
		styles.Title = styles.Title.Faint(true)
		styles.Input = styles.Input.Faint(true)
		styles.Warning = styles.Warning.Faint(true)
		styles.Error = styles.Error.Faint(true)
		styles.Selected = styles.Selected.Faint(true)
	}
	return styles
}

func rgba(value rgb) color.RGBA {
	return color.RGBA{R: value.r, G: value.g, B: value.b, A: 255}
}
