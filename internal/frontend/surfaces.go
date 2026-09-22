package frontend

import (
	"fmt"
	"image"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

const chatRowHeight = 4

// connectionText maps a connection state to its status-bar label.
func connectionText(connection domain.ConnectionState) string {
	switch connection {
	case domain.ConnectionOnline:
		return "online"
	case domain.ConnectionOffline:
		return "offline"
	case domain.ConnectionReconnecting:
		return "reconnecting"
	default:
		return "waiting"
	}
}

// chatMetadataSurface renders the trailing metadata of one chat row: a local
// time, unread and mention brackets, and a mute flag, joined by single spaces.
func chatMetadataSurface(timestamp int64, unread, mentions int, muted bool, location *time.Location) string {
	parts := make([]string, 0, 4)
	if timestamp != 0 {
		if location == nil {
			location = time.Local
		}
		parts = append(parts, time.Unix(timestamp, 0).In(location).Format("15:04"))
	}
	if unread > 0 {
		parts = append(parts, fmt.Sprintf("[%d]", unread))
	}
	if mentions > 0 {
		parts = append(parts, fmt.Sprintf("[@%d]", mentions))
	}
	if muted {
		parts = append(parts, "mute")
	}
	return strings.Join(parts, " ")
}

// detailsChatSurfaceRow returns the ChatRow owned by the Info pane. It may be
// different from the selected conversation when Info was opened from the
// focused chat's action menu.
func detailsChatSurfaceRow(model ViewModel) ChatRow {
	chatID := model.DetailsChat.ID
	if chatID == 0 {
		chatID = model.ActiveChat.ID
	}
	for _, row := range model.Chats {
		if row.Chat.ID == chatID {
			return row
		}
	}
	return ChatRow{}
}

// avatarSurfaceInitials returns up to two uppercase initials from name.
func avatarSurfaceInitials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	initials := make([]rune, 0, 2)
	for _, field := range fields {
		for _, value := range field {
			initials = append(initials, unicode.ToUpper(value))
			break
		}
		if len(initials) == 2 {
			break
		}
	}
	return string(initials)
}

// clipSurfaceLine normalizes whitespace and truncates value to width,
// appending an ellipsis when truncation is required. It measures cells with
// the accepted Charm ANSI API so it never splits a grapheme cluster.
func clipSurfaceLine(value string, width int) string {
	value = strings.Join(strings.Fields(value), " ")
	if width <= 0 || value == "" {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

// groupsBeforeSurfaceOffset returns a copy of groups with the trailing message
// groups trimmed until exactly offset messages have been removed. A nonpositive
// offset returns the original slice unchanged.
func groupsBeforeSurfaceOffset(groups []RenderedMessageGroup, offset int) []RenderedMessageGroup {
	if offset <= 0 {
		return groups
	}
	result := append([]RenderedMessageGroup(nil), groups...)
	remaining := offset
	for index := len(result) - 1; index >= 0 && remaining > 0; index-- {
		count := len(result[index].Messages)
		if remaining >= count {
			remaining -= count
			result = result[:index]
			continue
		}
		result[index].Messages = append([]domain.Message(nil), result[index].Messages[:count-remaining]...)
		remaining = 0
	}
	return result
}

// modalSurfaceRectangles returns the rounded frame rectangle (80x80 percent of
// bounds, centered) and the content rectangle inset inside it. Geometry is
// renderer-neutral and shared by the media modal text shell and the Kitty
// overlay placement.
func modalSurfaceRectangles(bounds image.Rectangle) (image.Rectangle, image.Rectangle) {
	if bounds.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	width := max(4, bounds.Dx()*80/100)
	height := max(4, bounds.Dy()*80/100)
	frame := centeredSurfaceRectangle(bounds, width, height)
	inner := insetSurfaceRectangle(frame, 1)
	if inner.Empty() {
		return frame, image.Rectangle{}
	}
	content := insetSurfaceRectangle(inner, 1)
	if content.Empty() {
		content = inner
	}
	return frame, content
}

// insetSurfaceRectangle returns the rectangle inset by amount on all sides,
// or empty when the result is degenerate.
func insetSurfaceRectangle(rectangle image.Rectangle, amount int) image.Rectangle {
	if rectangle.Dx() <= amount*2 || rectangle.Dy() <= amount*2 {
		return image.Rectangle{}
	}
	return image.Rect(rectangle.Min.X+amount, rectangle.Min.Y+amount, rectangle.Max.X-amount, rectangle.Max.Y-amount)
}

// centeredSurfaceRectangle returns a width x height rectangle centered within
// bounds and clipped to it.
func centeredSurfaceRectangle(bounds image.Rectangle, width, height int) image.Rectangle {
	x := bounds.Min.X + (bounds.Dx()-width)/2
	y := bounds.Min.Y + (bounds.Dy()-height)/2
	return image.Rect(x, y, x+width, y+height).Intersect(bounds)
}
