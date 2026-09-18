package frontend

import (
	"fmt"
	"image"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// topicsFrame is the centered modal geometry for the forum-topic list,
// mirroring the Members modal frame.
func topicsFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	width := min(64, bounds.Dx())
	height := min(12, bounds.Dy())
	return centeredSurfaceRectangle(bounds, width, height).Intersect(bounds)
}

// buildTopicsLayer renders the forum-topic list modal from model.Topics.
// One actionable row per result; loading/error/empty informational rows are
// purely visual. When a selector Huh View is injected it replaces the
// actionable row labels and selected-row paint; omitted injection retains the
// legacy rows.
func buildTopicsLayer(model ui.ViewModel, styles renderStyles, selectorView ...string) surfaceResult {
	if model.Topics == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	width := topicsFrame(bounds).Dx()
	rows := displayedTopicsRows(model)
	if len(selectorView) > 0 {
		return buildListModalWidth(bounds, "Topics", rows, styles, width, selectorView[0])
	}
	return buildListModalWidth(bounds, "Topics", rows, styles, width)
}

// displayedTopicsRows is the single row source for the forum-topic modal:
// the ALL pseudo-row plus one row per result and the trailing
// loading/error/empty informational row, reduced to the window the frame
// shows.
func displayedTopicsRows(model ui.ViewModel) []modalRowSpec {
	if model.Topics == nil {
		return nil
	}
	rows := topicsRows(model.Topics)
	return windowTopicsRows(rows, model.Topics.Selected, max(1, model.Height-5))
}

func topicsRows(topics *app.TopicListState) []modalRowSpec {
	if topics == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(topics.Results)+1)
	// Row 0 is the ALL pseudo-row, matching the reducer's display indexing
	// where Selected==0 means ALL is selected.
	rows = append(rows, modalRowSpec{
		ID:       "topic:all",
		Label:    "All messages",
		Selected: topics.Selected == 0,
		Action: app.ActionReceived{
			Action:  app.SelectTopic,
			ChatID:  topics.ChatID,
			TopicID: 0,
		},
	})
	for index, topic := range topics.Results {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("topic:%d", topic.ID),
			Label:    topicResultLabel(topic),
			Selected: index+1 == topics.Selected,
			Action: app.ActionReceived{
				Action:  app.SelectTopic,
				ChatID:  topics.ChatID,
				TopicID: topic.ID,
			},
		})
	}
	switch {
	case topics.Loading:
		rows = append(rows, modalRowSpec{Label: "Loading topics..."})
	case topics.Error != nil:
		rows = append(rows, modalRowSpec{Label: topics.Error.Message})
	case len(topics.Results) == 0:
		rows = append(rows, modalRowSpec{Label: "No topics"})
	}
	return rows
}

func windowTopicsRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}

// topicResultLabel renders one topic row: the name plus unread, pinned,
// closed, and draft markers.
func topicResultLabel(topic domain.ForumTopic) string {
	label := sanitizeDisplayString(topic.Name)
	if label == "" {
		label = "Topic"
	}
	if topic.UnreadCount > 0 {
		label += fmt.Sprintf(" (unread %d)", topic.UnreadCount)
	}
	if topic.IsPinned {
		label += " \U0001F4CC"
	}
	if topic.IsClosed {
		label += " \U0001F512"
	}
	if topic.Draft.Text != "" || topic.Draft.ReplyToMessageID != 0 {
		label += " …"
	}
	return label
}
