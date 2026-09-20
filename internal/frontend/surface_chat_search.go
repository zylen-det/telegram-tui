package frontend

import (
	"fmt"
	"image"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func chatSearchFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	width := min(72, bounds.Dx())
	height := min(10, bounds.Dy())
	return centeredSurfaceRectangle(bounds, width, height).Intersect(bounds)
}

func chatSearchInputRect(bounds image.Rectangle) image.Rectangle {
	frame := chatSearchFrame(bounds)
	if frame.Dx() < 6 || frame.Dy() < 5 {
		return image.Rectangle{}
	}
	return image.Rect(frame.Min.X+2, frame.Min.Y+3, frame.Max.X-2, frame.Min.Y+4)
}

func buildChatSearchLayer(model ViewModel, location *time.Location, styles renderStyles, searchInputView, selectorView string) surfaceResult {
	if model.ChatSearch == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	// Empty query: input-only modal so the user sees where to type.
	if !model.ChatSearch.Submitted {
		return buildChatSearchInputLayer(image.Rect(0, 0, model.Width, model.Height), model.ChatSearch, styles, searchInputView)
	}
	// Live preview: input stays focused while typing, results render below in
	// the same modal regardless of Input/Results focus.
	return buildChatSearchUnifiedLayer(image.Rect(0, 0, model.Width, model.Height), model, location, styles, searchInputView, selectorView)
}

func buildChatSearchInputLayer(bounds image.Rectangle, search *ChatSearchState, styles renderStyles, inputView string) surfaceResult {
	frame := chatSearchFrame(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	content := styles.Panel.Border(lipgloss.RoundedBorder()).BorderForeground(styles.FocusedBorder.GetForeground()).
		BorderBackground(styles.FocusedBorder.GetBackground()).Width(frame.Dx()).Height(frame.Dy()).Render("")
	root := lipgloss.NewLayer(content).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)
	var interactions []layerInteraction
	title := ansi.Truncate("Search", max(0, frame.Dx()-4), "")
	if title != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(title)).X(2).Y(0).Z(zModalContent))
	}
	closeAbs := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1).Intersect(frame)
	if !closeAbs.Empty() {
		interactions = append(interactions, addInteractive(root, frame.Min, closeAbs.Sub(frame.Min), "chat-search:close", zModalControl,
			renderLine(styles.Accent, "×", 1), ActionReceived{Action: Close}, ActionReceived{}, ActionReceived{}))
	}
	label := ansi.Truncate("Search", max(0, frame.Dx()-4), "")
	if label != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(label)).X(2).Y(2).Z(zModalContent))
	}
	inputAbs := chatSearchInputRect(bounds)
	if !inputAbs.Empty() {
		view := inputView
		if view == "" {
			view = string(search.Input)
		}
		view = clipPhotoPathInputView(view, inputAbs.Dx())
		interactions = append(interactions, addInteractive(root, frame.Min, inputAbs.Sub(frame.Min), "chat-search:input", zModalControl,
			renderLine(styles.Panel, view, inputAbs.Dx()), ActionReceived{}, ActionReceived{}, ActionReceived{}))
	}
	buttonAbs := image.Rect(frame.Max.X-10, frame.Max.Y-3, frame.Max.X-2, frame.Max.Y-2).Intersect(frame)
	enabled := strings.TrimSpace(string(search.Input)) != ""
	buttonStyle := styles.Muted
	buttonAction := ActionReceived{}
	if enabled {
		buttonStyle = styles.Accent
		buttonAction = ActionReceived{Action: SubmitChatSearch}
	}
	if !buttonAbs.Empty() {
		interactions = append(interactions, addInteractive(root, frame.Min, buttonAbs.Sub(frame.Min), "chat-search:submit", zModalControl,
			renderLine(buttonStyle, "[Search]", buttonAbs.Dx()), buttonAction, ActionReceived{}, ActionReceived{}))
	}
	return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
}

// buildChatSearchUnifiedLayer renders one modal with the live input row on
// top and the three flattened sections below: local chats, global messages,
// public chats. Selection is a single flattened index shared with the
// selector host and mouse actions.
// Note: the unified layer deliberately ignores selectorView. The Huh
// selector overlay paints a header-less option list with its own internal
// scrolling, which desyncs from the sectioned rows (headers consume visual
// rows the selector does not know about) and lands option labels under the
// wrong headers. Manual row paint below is the single source of truth;
// keyboard navigation flows through the reducer and mouse through hit maps,
// neither of which needs the overlay.
func buildChatSearchUnifiedLayer(bounds image.Rectangle, model ViewModel, location *time.Location, styles renderStyles, searchInputView, _ string) surfaceResult {
	search := model.ChatSearch
	rows := buildUnifiedChatSearchRows(model, location)
	capacity := max(1, bounds.Dy()-12)
	rows = windowUnifiedChatSearchRows(rows, search.Selected, capacity)
	// The live input stays inside the modal: an echo row pinned above the
	// windowed sections. Production injects the real Huh input view (with
	// cursor); standalone callers fall back to the plain query text.
	echo := "> " + sanitizeDisplayString(string(search.Input))
	if searchInputView != "" {
		echo = searchInputView
	}
	rows = append([]modalRowSpec{{Label: echo, Header: true}}, rows...)
	width := chatSearchFrame(bounds).Dx()
	return buildListModalWidth(bounds, "Search", rows, styles, width)
}

// chatSearchSectionHeader renders one divider/header line separating the
// live result sections inside the modal.
func chatSearchSectionHeader(title string) modalRowSpec {
	return modalRowSpec{Label: "─ " + title + " ─", Header: true}
}

// buildUnifiedChatSearchRows assembles the three-section results layout in
// display order: local chats, global messages, public chats. Each non-empty
// section gets a header/divider row; selection counts actionable rows only.
func buildUnifiedChatSearchRows(model ViewModel, location *time.Location) []modalRowSpec {
	search := model.ChatSearch
	if search == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, 9)
	selectionCount := 0

	hasLocal := false
	for _, chat := range search.LocalChats {
		if chat.ID != 0 {
			hasLocal = true
			break
		}
	}
	if hasLocal {
		rows = append(rows, chatSearchSectionHeader("Chats"))
	}
	for _, chat := range search.LocalChats {
		if chat.ID == 0 {
			continue
		}
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("chat-search:local:%d", chat.ID),
			Label:    chatSearchResultLabel(chat),
			Selected: search.Selected == selectionCount,
			Action:   ActionReceived{Action: SelectChat, ChatID: chat.ID},
		})
		selectionCount++
	}

	hasMessages := false
	for _, msg := range search.GlobalMessages {
		if msg.ChatID != 0 && msg.ID != 0 {
			hasMessages = true
			break
		}
	}
	if hasMessages {
		rows = append(rows, chatSearchSectionHeader("Messages"))
	}
	for _, msg := range search.GlobalMessages {
		if msg.ChatID == 0 || msg.ID == 0 {
			continue
		}
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("chat-search:msg:%d:%d", msg.ChatID, msg.ID),
			Label:    globalMessageLabel(msg, location),
			Selected: search.Selected == selectionCount,
			Action:   ActionReceived{Action: SelectMessage, ChatID: msg.ChatID, MessageID: msg.ID},
		})
		selectionCount++
	}

	hasPublic := false
	for _, chat := range search.PublicChats {
		if chat.ID != 0 {
			hasPublic = true
			break
		}
	}
	if hasPublic {
		rows = append(rows, chatSearchSectionHeader("Public chats"))
	}
	for _, chat := range search.PublicChats {
		if chat.ID == 0 {
			continue
		}
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("chat-search:pub:%d", chat.ID),
			Label:    chatSearchResultLabel(chat),
			Selected: search.Selected == selectionCount,
			Action:   ActionReceived{Action: SelectChat, ChatID: chat.ID},
		})
		selectionCount++
	}

	if selectionCount == 0 {
		switch {
		case search.PublicLoading || search.MessagesLoading:
			rows = append(rows, modalRowSpec{Label: "Searching..."})
		case search.PublicError != nil:
			rows = append(rows, modalRowSpec{Label: search.PublicError.Message})
		case search.MessagesError != nil:
			rows = append(rows, modalRowSpec{Label: search.MessagesError.Message})
		default:
			rows = append(rows, modalRowSpec{Label: "No results"})
		}
	}
	return rows
}

// windowUnifiedChatSearchRows windows sectioned rows to capacity. The window
// always contains the selected actionable row, always starts at that row's
// section header when it fits, and never ends on a lone header. selected
// counts actionable rows only.
func windowUnifiedChatSearchRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	selFull := -1
	count := 0
	for index, row := range rows {
		if row.ID != "" && row.Action.Action != NoAction {
			if count == selected {
				selFull = index
				break
			}
			count++
		}
	}
	if selFull < 0 {
		// Stale selection: pin to the last actionable row, if any.
		for index := len(rows) - 1; index >= 0; index-- {
			if rows[index].ID != "" && rows[index].Action.Action != NoAction {
				selFull = index
				break
			}
		}
		if selFull < 0 {
			return rows[:min(len(rows), capacity)]
		}
	}
	// Prefer starting at the selected row's section header when the span fits.
	header := selFull
	for header > 0 && !rows[header].Header {
		header--
	}
	start := selFull - capacity + 1
	if header <= start && selFull-header+1 <= capacity {
		start = header
	}
	start = max(0, min(start, len(rows)-capacity))
	end := min(len(rows), start+capacity)
	if selFull >= end {
		end = selFull + 1
		start = max(0, end-capacity)
	}
	for end-start > 1 && rows[end-1].Header && end-1 != selFull {
		end--
	}
	return rows[start:end]
}

func windowChatSearchRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}

func chatSearchResultLabel(chat domain.Chat) string {
	title := sanitizeDisplayString(chat.Title)
	if title == "" {
		title = "Unknown"
	}
	username := strings.TrimPrefix(sanitizeDisplayString(chat.Username), "@")
	if username == "" {
		return title
	}
	return fmt.Sprintf("%s · @%s", title, username)
}

// globalMessageLabel formats a global search message result with sender, time, and preview.
func globalMessageLabel(message domain.Message, location *time.Location) string {
	sender := sanitizeDisplayString(message.SenderName)
	timeStr := formatRelativeTime(message.SentAt, location)
	text := sanitizeDisplayString(message.DisplayText())
	if sender == "" {
		return fmt.Sprintf("%s · %s", timeStr, text)
	}
	return fmt.Sprintf("%s · %s · %s", sender, timeStr, text)
}

// formatRelativeTime formats a time as a relative string.
func formatRelativeTime(t time.Time, location *time.Location) string {
	if t.IsZero() {
		return ""
	}
	if location != nil {
		t = t.In(location)
	}
	now := time.Now()
	diff := now.Sub(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	return t.Format("Jan 2")
}
