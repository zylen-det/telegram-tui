package frontend

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// buildCommandMenuLayer renders a non-modal completion list over the bottom of
// the conversation history, immediately above the composer.
func buildCommandMenuLayer(model ui.ViewModel, historyRect, composerRect image.Rectangle, styles renderStyles) surfaceResult {
	menu := model.CommandMenu
	if menu == nil || historyRect.Empty() || composerRect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	rowCount := len(menu.Candidates)
	if menu.Loading || menu.Error != nil {
		rowCount = 1
	}
	if rowCount == 0 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	visibleRows := min(app.CommandMenuVisibleRows, rowCount, max(0, historyRect.Dy()-2))
	if visibleRows < 1 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	height := visibleRows + 2
	frame := image.Rect(composerRect.Min.X, composerRect.Min.Y-height, composerRect.Max.X, composerRect.Min.Y).
		Intersect(historyRect)
	if frame.Dx() < 4 || frame.Dy() < 3 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	visibleRows = frame.Dy() - 2

	rootContent := styles.Panel.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.FocusedBorder.GetForeground()).
		BorderBackground(styles.FocusedBorder.GetBackground()).
		Width(frame.Dx()).Height(frame.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(frame.Min.X).Y(frame.Min.Y).Z(zCommandMenuFrame)

	title := ansi.Truncate("Commands", max(0, frame.Dx()-6), "")
	if title != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(title)).X(2).Y(0).Z(zCommandMenuText))
	}

	request := func(action app.Action) app.ActionReceived {
		return app.ActionReceived{Action: action}
	}
	interactions := []layerInteraction{{
		ID: "command-menu", Rect: frame, Z: zCommandMenuFrame, Virtual: true,
		WheelUp: request(app.CommandMenuPrevious), WheelDown: request(app.CommandMenuNext),
	}}
	innerWidth := max(0, frame.Dx()-2)
	if menu.Loading {
		addCommandMenuStatus(root, styles, "Loading commands…", innerWidth)
		return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
	}
	if menu.Error != nil {
		addCommandMenuStatus(root, styles, "Commands unavailable", innerWidth)
		return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
	}

	first := max(0, min(menu.First, max(0, len(menu.Candidates)-visibleRows)))
	if menu.Selected < first {
		first = menu.Selected
	}
	if menu.Selected >= first+visibleRows {
		first = menu.Selected - visibleRows + 1
	}
	last := min(len(menu.Candidates), first+visibleRows)
	for index := first; index < last; index++ {
		row := index - first + 1
		command := menu.Candidates[index]
		label := command.Invocation()
		if command.Description != "" {
			label += "  " + command.Description
		}
		label = ansi.Truncate(label, innerWidth, "")
		style := styles.Panel
		prefix := "  "
		if index == menu.Selected {
			style = styles.Selected
			prefix = "› "
		}
		text := renderLine(style, prefix+label, innerWidth)
		local := image.Rect(1, row, frame.Dx()-1, row+1)
		action := app.ActionReceived{Action: app.CommandMenuActivate, ChatID: menu.ChatID, CommandIndex: index}
		interactions = append(interactions, addInteractive(root, frame.Min, local,
			fmt.Sprintf("command-menu:%d", index), zCommandMenuRow, text, action,
			request(app.CommandMenuPrevious), request(app.CommandMenuNext)))
	}

	return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
}

func addCommandMenuStatus(root *lipgloss.Layer, styles renderStyles, text string, width int) {
	text = ansi.Truncate(text, width, "")
	root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(text)).X(1).Y(1).Z(zCommandMenuText))
}
