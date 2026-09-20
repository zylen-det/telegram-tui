package frontend

import (
	"fmt"
	"image"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func messageSearchFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	width := min(64, bounds.Dx())
	height := min(8, bounds.Dy())
	return centeredSurfaceRectangle(bounds, width, height).Intersect(bounds)
}

func messageSearchInputRect(bounds image.Rectangle) image.Rectangle {
	frame := messageSearchFrame(bounds)
	if frame.Dx() < 6 || frame.Dy() < 5 {
		return image.Rectangle{}
	}
	return image.Rect(frame.Min.X+2, frame.Min.Y+3, frame.Max.X-2, frame.Min.Y+4)
}

func buildMessageSearchLayer(model ViewModel, location *time.Location, styles renderStyles, searchInputView, selectorView string) surfaceResult {
	if model.MessageSearch == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	if !model.MessageSearch.Submitted || model.Focus == FocusSearchInput {
		return buildMessageSearchInputLayer(image.Rect(0, 0, model.Width, model.Height), model.MessageSearch, styles, searchInputView)
	}
	rows := displayedMessageSearchRows(model, location)
	bounds := image.Rect(0, 0, model.Width, model.Height)
	width := messageSearchFrame(bounds).Dx()
	if selectorView != "" {
		return buildListModalWidth(bounds, "Search messages", rows, styles, width, selectorView)
	}
	return buildListModalWidth(bounds, "Search messages", rows, styles, width)
}

// displayedMessageSearchRows is the single row source for a submitted message
// search modal: the result/informational rows built by messageSearchRows
// reduced to the window that actually reaches the frame. The renderer and the
// list modal controller both call it, so selector options, geometry, and paint
// always describe the same displayed rows.
func displayedMessageSearchRows(model ViewModel, location *time.Location) []modalRowSpec {
	if model.MessageSearch == nil {
		return nil
	}
	rows := messageSearchRows(model, location)
	return windowMessageSearchRows(rows, model.MessageSearch.Selected, max(1, model.Height-5))
}

func buildMessageSearchInputLayer(bounds image.Rectangle, search *MessageSearchState, styles renderStyles, inputView string) surfaceResult {
	frame := messageSearchFrame(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	content := styles.Panel.Border(lipgloss.RoundedBorder()).BorderForeground(styles.FocusedBorder.GetForeground()).
		BorderBackground(styles.FocusedBorder.GetBackground()).Width(frame.Dx()).Height(frame.Dy()).Render("")
	root := lipgloss.NewLayer(content).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)
	var interactions []layerInteraction
	title := ansi.Truncate("Search messages", max(0, frame.Dx()-4), "")
	if title != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(title)).X(2).Y(0).Z(zModalContent))
	}
	closeAbs := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1).Intersect(frame)
	if !closeAbs.Empty() {
		interactions = append(interactions, addInteractive(root, frame.Min, closeAbs.Sub(frame.Min), "search:close", zModalControl,
			renderLine(styles.Accent, "×", 1), ActionReceived{Action: Close}, ActionReceived{}, ActionReceived{}))
	}
	label := ansi.Truncate("Query", max(0, frame.Dx()-4), "")
	if label != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(label)).X(2).Y(2).Z(zModalContent))
	}
	inputAbs := messageSearchInputRect(bounds)
	if !inputAbs.Empty() {
		view := inputView
		if view == "" {
			view = string(search.Input)
		}
		view = clipPhotoPathInputView(view, inputAbs.Dx())
		interactions = append(interactions, addInteractive(root, frame.Min, inputAbs.Sub(frame.Min), "search:input", zModalControl,
			renderLine(styles.Panel, view, inputAbs.Dx()), ActionReceived{}, ActionReceived{}, ActionReceived{}))
	}
	buttonAbs := image.Rect(frame.Max.X-10, frame.Max.Y-3, frame.Max.X-2, frame.Max.Y-2).Intersect(frame)
	enabled := strings.TrimSpace(string(search.Input)) != ""
	buttonStyle := styles.Muted
	buttonAction := ActionReceived{}
	if enabled {
		buttonStyle = styles.Accent
		buttonAction = ActionReceived{Action: SubmitMessageSearch}
	}
	if !buttonAbs.Empty() {
		interactions = append(interactions, addInteractive(root, frame.Min, buttonAbs.Sub(frame.Min), "search:submit", zModalControl,
			renderLine(buttonStyle, "[Search]", buttonAbs.Dx()), buttonAction, ActionReceived{}, ActionReceived{}))
	}
	return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
}

func messageSearchRows(model ViewModel, location *time.Location) []modalRowSpec {
	search := model.MessageSearch
	if search == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(search.Results)+1)
	for index, message := range search.Results {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("search:%d", message.ID),
			Label:    messageSearchResultLabel(message.SenderName, message.SentAt, message.DisplayText(), location),
			Selected: index == search.Selected,
			Action:   ActionReceived{Action: SelectMessage, ChatID: search.ChatID, MessageID: message.ID},
		})
	}
	switch {
	case search.Loading && search.JumpMessageID != 0:
		rows = append(rows, modalRowSpec{Label: "Opening result..."})
	case search.Loading:
		rows = append(rows, modalRowSpec{Label: "Searching..."})
	case search.Error != nil:
		rows = append(rows, modalRowSpec{Label: search.Error.Message})
	case len(search.Results) == 0:
		rows = append(rows, modalRowSpec{Label: "No messages found"})
	}
	return rows
}

func windowMessageSearchRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}

func messageSearchResultLabel(sender string, sentAt time.Time, text string, location *time.Location) string {
	if location == nil {
		location = time.Local
	}
	sender = sanitizeDisplayString(sender)
	if sender == "" {
		sender = "Unknown"
	}
	text = sanitizeDisplayString(text)
	if text == "" {
		text = "[Unsupported]"
	}
	return fmt.Sprintf("%s · %s · %s", sender, sentAt.In(location).Format("Jan 02 15:04"), text)
}
