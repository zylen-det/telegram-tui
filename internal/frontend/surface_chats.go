package frontend

import (
	"fmt"
	"image"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// buildChatsLayer builds the chats pane surface: a rounded pane root with an
// intrinsic title, an optional error/loading/empty state line, and a windowed
// list of chat rows nested pane-locally. Interactions are absolute. An empty
// viewport intersection returns a zero surface.
func buildChatsLayer(model ui.ViewModel, location *time.Location, styles renderStyles) surfaceResult {
	rect := model.Layout.Chats.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	pane := buildPane(rect, "Chats", model.Focus == app.FocusChats, "pane:chats", app.FocusChats, styles)
	if pane.Layer == nil {
		return pane
	}

	inner := rect.Inset(1)
	if inner.Empty() {
		return pane
	}

	interactions := append([]layerInteraction(nil), pane.Interactions...)

	rowsTop := inner.Min.Y
	switch {
	case model.ChatsError != nil:
		pane.Layer.AddLayers(lipgloss.NewLayer(renderLine(styles.Error, model.ChatsError.Message, inner.Dx())).
			X(inner.Min.X - rect.Min.X).Y(rowsTop - rect.Min.Y).Z(zContent))
		if len(model.Chats) == 0 {
			return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
		}
		rowsTop++
	case model.ChatsLoading:
		pane.Layer.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "Loading chats...", inner.Dx())).
			X(inner.Min.X - rect.Min.X).Y(rowsTop - rect.Min.Y).Z(zContent))
		if len(model.Chats) == 0 {
			return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
		}
		rowsTop++
	case model.ChatsLoaded && len(model.Chats) == 0:
		pane.Layer.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "No chats", inner.Dx())).
			X(inner.Min.X - rect.Min.X).Y(rowsTop - rect.Min.Y).Z(zContent))
		return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
	}

	capacity := max(0, (inner.Max.Y-rowsTop)/chatRowHeight)
	if capacity == 0 {
		return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
	}

	selected := -1
	for index, row := range model.Chats {
		if row.Selected {
			selected = index
			break
		}
	}
	start := 0
	if selected >= capacity {
		start = selected - capacity + 1
	}

	for index := start; index < min(len(model.Chats), start+capacity); index++ {
		row := model.Chats[index]
		y := rowsTop + (index-start)*chatRowHeight
		rowRect := image.Rect(inner.Min.X, y, inner.Max.X, min(inner.Max.Y, y+chatRowHeight))
		rowSurface := buildChatRowLayer(row, rowRect, location, styles)
		if rowSurface.Layer == nil {
			continue
		}
		// Reset the row root to pane-local coordinates; interactions stay
		// absolute so standalone and nested geometry share one source.
		rowSurface.Layer.X(rowRect.Min.X - rect.Min.X).Y(rowRect.Min.Y - rect.Min.Y)
		pane.Layer.AddLayers(rowSurface.Layer)
		interactions = append(interactions, rowSurface.Interactions...)
	}

	return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
}

// buildChatRowLayer builds one chat row surface at the absolute rect.Min. The
// root is a fixed full-row background with the chat ID; children (avatar and
// text lines) are positioned locally inside the row. Interactions are absolute.
// An empty rect returns a zero surface with a hidden cursor.
func buildChatRowLayer(row ui.ChatRow, rect image.Rectangle, location *time.Location, styles renderStyles) surfaceResult {
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	rootStyle := styles.Panel
	if row.Selected {
		rootStyle = styles.Selected
	}
	rootContent := rootStyle.Width(rect.Dx()).Height(rect.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).
		ID(fmt.Sprintf("chat:%d", row.Chat.ID)).
		X(rect.Min.X).Y(rect.Min.Y).Z(zRowBackground)

	interactions := []layerInteraction{{
		ID:        fmt.Sprintf("chat:%d", row.Chat.ID),
		Rect:      rect,
		Z:         zRowBackground,
		Click:     app.ActionReceived{Action: app.SelectChat, ChatID: row.Chat.ID},
		WheelUp:   app.ActionReceived{Action: app.SelectPrevious},
		WheelDown: app.ActionReceived{Action: app.SelectNext},
	}}

	avatarLocal := image.Rect(0, 0, min(rect.Dx(), 6), min(rect.Dy(), 3))
	textX := min(rect.Dx(), avatarLocal.Max.X+1)

	// Avatar. On retry the avatar root moves to zControl, gets an ID, and
	// carries a clipped Retry text child on its bottom row.
	avatarZ := zContent
	if row.AvatarError != nil {
		avatarZ = zControl
	}
	if avatarLayer := buildAvatarLayer(avatarLocal, row.Avatar, row.AvatarKey, row.Chat.Title, styles, avatarZ); avatarLayer != nil {
		if row.AvatarError != nil {
			avatarLayer.ID(fmt.Sprintf("chat-avatar-retry:%d", row.Chat.ID))
			retry := ansi.Truncate("Retry", avatarLocal.Dx(), "")
			avatarLayer.AddLayers(lipgloss.NewLayer(styles.Error.Render(retry)).
				X(0).Y(avatarLocal.Dy() - 1).Z(zControl + 2))
		}
		root.AddLayers(avatarLayer)
	}

	// Title, preview, and metadata lines. A supported cloud draft replaces
	// the last-message preview without exposing reply/message identifiers.
	preview := row.Chat.LastMessage
	timestamp := row.Chat.LastMessageAt
	hasDraft := row.Draft.Text != "" || row.Draft.ReplyToMessageID > 0
	if hasDraft {
		draftText := strings.Join(strings.Fields(row.Draft.Text), " ")
		if draftText == "" {
			draftText = "Replying to message"
		}
		preview = "Draft: " + draftText
		if row.Draft.Date > 0 {
			timestamp = row.Draft.Date
		}
	}
	titleStyle := styles.Emphasis
	previewStyle := styles.Panel
	if hasDraft {
		previewStyle = styles.Emphasis
	}
	metadataStyle := styles.Muted
	if row.Selected {
		titleStyle = titleStyle.Background(rgba(selectedColor))
		previewStyle = previewStyle.Background(rgba(selectedColor))
		metadataStyle = metadataStyle.Background(rgba(selectedColor))
	}
	width := rect.Dx() - textX
	if width > 0 {
		if rect.Dy() >= 1 {
			root.AddLayers(lipgloss.NewLayer(renderLine(titleStyle, row.Chat.Title, width)).X(textX).Y(0).Z(zContent))
		}
		if rect.Dy() >= 2 {
			root.AddLayers(lipgloss.NewLayer(renderLine(previewStyle, preview, width)).X(textX).Y(1).Z(zContent))
		}
		if rect.Dy() >= 3 {
			root.AddLayers(lipgloss.NewLayer(renderLine(metadataStyle, chatMetadataSurface(timestamp, row.Chat.UnreadCount, row.Chat.UnreadMentionCount, row.Chat.Muted, location), width)).X(textX).Y(2).Z(zContent))
		}
	}

	if row.AvatarError != nil {
		interactions = append(interactions, layerInteraction{
			ID:    fmt.Sprintf("chat-avatar-retry:%d", row.Chat.ID),
			Rect:  avatarLocal.Add(rect.Min),
			Z:     zControl,
			Click: app.ActionReceived{Action: app.Retry, AvatarKey: row.AvatarKey},
		})
	}

	return surfaceResult{
		Layer:        root,
		Rect:         rect,
		Interactions: interactions,
		Cursor:       renderCursor{X: -1, Y: -1},
	}
}
