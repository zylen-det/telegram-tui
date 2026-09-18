package ui

import (
	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
)

var (
	backgroundColor    = gotui.NewRGBColor(15, 17, 20)
	panelColor         = gotui.NewRGBColor(21, 24, 28)
	selectedColor      = gotui.NewRGBColor(38, 48, 55)
	borderColor        = gotui.NewRGBColor(92, 103, 111)
	focusedBorderColor = gotui.NewRGBColor(73, 176, 196)
	textColor          = gotui.NewRGBColor(225, 230, 232)
	mutedTextColor     = gotui.NewRGBColor(145, 155, 160)
	accentColor        = gotui.NewRGBColor(111, 202, 162)
	warningColor       = gotui.NewRGBColor(231, 181, 85)
	errorColor         = gotui.NewRGBColor(230, 106, 111)
)

var (
	baseStyle     = gotui.NewStyle(textColor, backgroundColor)
	panelStyle    = gotui.NewStyle(textColor, panelColor)
	mutedStyle    = gotui.NewStyle(mutedTextColor, panelColor)
	emphasisStyle = gotui.NewStyle(textColor, panelColor, tcell.AttrBold)
	accentStyle   = gotui.NewStyle(accentColor, panelColor, tcell.AttrBold)
	warningStyle  = gotui.NewStyle(warningColor, panelColor)
	errorStyle    = gotui.NewStyle(errorColor, panelColor)
)
