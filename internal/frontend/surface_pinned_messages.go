package frontend

import (
	"fmt"
	"image"
	"time"
)

func pinnedMessagesFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	width := min(64, bounds.Dx())
	height := min(8, bounds.Dy())
	return centeredSurfaceRectangle(bounds, width, height).Intersect(bounds)
}

func buildPinnedMessagesLayer(model ViewModel, location *time.Location, overlayStyles renderStyles, selectorView string) surfaceResult {
	if model.PinnedMessages == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	rows := displayedPinnedMessagesRows(model, location)
	bounds := image.Rect(0, 0, model.Width, model.Height)
	width := pinnedMessagesFrame(bounds).Dx()
	if selectorView != "" {
		return buildListModalWidth(bounds, "Pinned messages", rows, overlayStyles, width, selectorView)
	}
	return buildListModalWidth(bounds, "Pinned messages", rows, overlayStyles, width)
}

// displayedPinnedMessagesRows is the single row source for the pinned message
// modal: result rows plus the trailing loading/error/empty informational row,
// reduced to the window the frame actually shows.
func displayedPinnedMessagesRows(model ViewModel, location *time.Location) []modalRowSpec {
	if model.PinnedMessages == nil {
		return nil
	}
	rows := pinnedMessagesRows(model, location)
	return windowPinnedMessagesRows(rows, model.PinnedMessages.Selected, max(1, model.Height-5))
}

func pinnedMessagesRows(model ViewModel, location *time.Location) []modalRowSpec {
	pinned := model.PinnedMessages
	if pinned == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(pinned.Results)+1)
	for index, message := range pinned.Results {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("pinned:%d", message.ID),
			Label:    pinnedMessageResultLabel(message.SenderName, message.SentAt, message.DisplayText(), location),
			Selected: index == pinned.Selected,
			Action:   ActionReceived{Action: SelectMessage, ChatID: pinned.ChatID, MessageID: message.ID},
		})
	}
	switch {
	case pinned.Loading:
		rows = append(rows, modalRowSpec{Label: "Loading pinned messages..."})
	case pinned.Error != nil:
		rows = append(rows, modalRowSpec{Label: pinned.Error.Message})
	case len(pinned.Results) == 0:
		rows = append(rows, modalRowSpec{Label: "No pinned messages"})
	}
	return rows
}

func windowPinnedMessagesRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}

func pinnedMessageResultLabel(sender string, sentAt time.Time, text string, location *time.Location) string {
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
