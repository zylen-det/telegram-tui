package frontend

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
)

// modalRowSpec is one semantic action list row for the shared list modal.
// Header marks a non-actionable structural row (section header, input echo)
// that windowing should keep attached to the rows below it.
type modalRowSpec struct {
	ID       string
	Label    string
	Selected bool
	Action   ActionReceived
	Header   bool
	Key      string // direct activation key for action menus; empty on other lists
}

// buildListModal builds the shared action-list modal surface. Geometry is
// owned entirely by components.Modal.Layout; close, rows, and content all
// follow that layout exactly. Actionable rows carry their ID and full-row
// interaction; informational rows are purely visual.
func buildListModal(bounds image.Rectangle, title string, rows []modalRowSpec, styles renderStyles, selectorView ...string) surfaceResult {
	return buildListModalWidth(bounds, title, rows, styles, 0, selectorView...)
}

// buildListModalWidth renders the shared list modal with an optional preferred
// frame width. A nonpositive width preserves the compact shared default.
func buildListModalWidth(bounds image.Rectangle, title string, rows []modalRowSpec, styles renderStyles, width int, selectorView ...string) surfaceResult {
	if bounds.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	items := make([]components.Item, 0, len(rows))
	selected := -1
	for index, row := range rows {
		items = append(items, components.Item{Label: row.Label})
		if selected < 0 && row.Selected {
			selected = index
		}
	}

	layout := (components.Modal{Title: title, Items: items, Selected: selected, Width: width}).Layout(bounds)
	if layout.Frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	frame := layout.Frame

	// Frame root: absolute at Frame.Min, no ID.
	var rootContent string
	if frame.Dx() >= 3 && frame.Dy() >= 3 {
		rootContent = styles.Panel.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.FocusedBorder.GetForeground()).
			BorderBackground(styles.FocusedBorder.GetBackground()).
			Width(frame.Dx()).
			Height(frame.Dy()).
			Render("")
	} else {
		rootContent = renderEmptyBox(styles.Panel, frame.Dx(), frame.Dy())
	}
	root := lipgloss.NewLayer(rootContent).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)

	var interactions []layerInteraction
	injected := len(selectorView) > 0
	var selectorRect image.Rectangle

	// Title: intrinsic child local (2,0), clipped strictly before the close
	// cell so it never overwrites the top-right border/close corner.
	titleWidth := (layout.Close.X - frame.Min.X) - 2
	if titleWidth > 0 && title != "" {
		clipped := ansi.Truncate(title, titleWidth, "")
		if clipped != "" {
			root.AddLayers(lipgloss.NewLayer(styles.Title.Render(clipped)).X(2).Y(0).Z(zModalContent))
		}
	}

	// Close control: exact 1-cell at Close intersect Frame, click Close.
	closeLocal := image.Rect(layout.Close.X, layout.Close.Y, layout.Close.X+1, layout.Close.Y+1).
		Intersect(frame).Sub(frame.Min)
	if !closeLocal.Empty() {
		closeContent := renderLine(styles.Accent, "×", 1)
		interactions = append(interactions, addInteractive(
			root, frame.Min, closeLocal, "modal:close", zModalControl, closeContent,
			ActionReceived{Action: Close}, ActionReceived{}, ActionReceived{},
		))
	}

	// Visible rows, mapped to their spec by row.Index.
	for _, row := range layout.Rows {
		if row.Rect.Empty() {
			continue
		}
		var spec modalRowSpec
		if row.Index >= 0 && row.Index < len(rows) {
			spec = rows[row.Index]
		}

		rowStyle := styles.Panel
		if spec.Selected && !injected {
			rowStyle = styles.Selected
		}
		rowLocal := row.Rect.Sub(frame.Min)
		if rowLocal.Dx() <= 0 || rowLocal.Dy() <= 0 {
			continue
		}

		interactive := spec.ID != "" && spec.Action.Action != NoAction

		if interactive {
			// Union of visible actionable row rects; equals the host's modal-row
			// geometry and hosts the injected Huh Selector View when present.
			selectorRect = selectorRect.Union(rowLocal)
			rowContent := renderEmptyBox(rowStyle, rowLocal.Dx(), rowLocal.Dy())
			interactions = append(interactions, addInteractive(
				root, frame.Min, rowLocal, spec.ID, zModalRow, rowContent,
				spec.Action, ActionReceived{}, ActionReceived{},
			))
			if !injected {
				rowLayer := root.GetLayer(spec.ID)
				addModalRowLabel(rowLayer, row, spec, rowStyle)
			}
		} else {
			rowContent := renderEmptyBox(rowStyle, rowLocal.Dx(), rowLocal.Dy())
			rowLayer := lipgloss.NewLayer(rowContent).X(rowLocal.Min.X).Y(rowLocal.Min.Y).Z(zModalRow)
			addModalRowLabel(rowLayer, row, spec, rowStyle)
			root.AddLayers(rowLayer)
		}
	}

	// Injected Huh Selector View: replaces the actionable row labels/selection
	// paint. An empty view adds no content and never falls back to legacy
	// labels; non-status rows keep their manual rendering.
	if injected && !selectorRect.Empty() {
		viewWidth := selectorRect.Dx()
		if len(rows) > 0 && rows[0].Key != "" && viewWidth > 2 {
			viewWidth -= 2 // reserve a gap and the right-aligned shortcut cell
		}
		view := clipSelectorHuhView(selectorView[0], viewWidth, selectorRect.Dy())
		if view != "" {
			root.AddLayers(lipgloss.NewLayer(view).X(selectorRect.Min.X).Y(selectorRect.Min.Y).Z(zModalContent))
		}
	}

	// Huh owns the selection/label paint; the shortcut is shared chrome on
	// top of either paint path, never another selector label.
	for _, row := range layout.Rows {
		if row.Index < 0 || row.Index >= len(rows) || rows[row.Index].Key == "" || row.Rect.Dx() < 3 {
			continue
		}
		keyStyle := styles.Muted
		if rows[row.Index].Selected && !injected {
			keyStyle = keyStyle.Background(styles.Selected.GetBackground())
		}
		local := row.Rect.Sub(frame.Min)
		root.AddLayers(lipgloss.NewLayer(keyStyle.Render(rows[row.Index].Key)).X(local.Max.X - 1).Y(local.Min.Y).Z(zModalContent + 1))
	}

	return surfaceResult{
		Layer:        root,
		Rect:         frame,
		Interactions: interactions,
		Cursor:       renderCursor{X: -1, Y: -1},
		IsModal:      true,
	}
}

// Both the renderer and the Huh selector host use this width so the reference
// action fits on one line in normal-sized terminals.
const messageActionModalWidth = 48

// buildActionModalLayer adapts the message action menu into the shared list
// modal. Selection follows the generated capability-gated row order rather
// than capability ordinals. View image, Reply, reference navigation, and Copy survive loading/error
// states; every other actionable row is gated on a settled menu. When a
// selector Huh View is injected it replaces the actionable row labels and
// selected-row paint; omitted injection retains the legacy rows.
func buildActionModalLayer(model ViewModel, styles renderStyles, selectorView ...string) surfaceResult {
	if model.MessageMenu == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	return buildListModalWidth(image.Rect(0, 0, model.Width, model.Height), "Message actions", messageActionRows(model.MessageMenu), styles, messageActionModalWidth, selectorView...)
}

// messageActionRows is the single displayed row source for the message action
// menu: every actionable row in generated order plus the trailing
// loading/error informational row. Nothing else may compile this list; the
// renderer and the list modal controller both call it so row order, gating,
// labels, semantic payloads, and the selected row stay identical.
func messageActionRows(menu *MessageActionMenu) []modalRowSpec {
	if menu == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, 12)

	if (menu.MediaFile.Downloaded && menu.MediaFile.LocalPath != "") || (menu.MediaFile.ID != 0 && menu.MediaFile.CanDownload) {
		id := "action:view-image"
		label := "View image"
		switch menu.MediaKind {
		case domain.MessageVideo:
			id = "action:open-video"
			label = "Open video"
		case domain.MessageAudio:
			id = "action:open-audio"
			label = "Open audio"
		case domain.MessageDocument:
			id = "action:open-file"
			label = "Open file"
		case domain.MessageAnimation:
			id = "action:open-animation"
			label = "Open animation"
		case domain.MessageVoiceNote:
			id = "action:open-voice-note"
			label = "Open voice note"
		case domain.MessageVideoNote:
			id = "action:open-video-note"
			label = "Open video note"
		}
		rows = append(rows, modalRowSpec{
			ID:     id,
			Label:  label,
			Action: ActionReceived{Action: ViewMessageMedia, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}

	if menu.Capabilities.Reply {
		rows = append(rows, modalRowSpec{
			ID:     "action:reply",
			Label:  "Reply",
			Action: ActionReceived{Action: ReplyMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if menu.ReferencedMessageID > 0 {
		rows = append(rows, modalRowSpec{
			ID:     "action:go-to-reference",
			Label:  "Go to referenced message",
			Action: ActionReceived{Action: GoToReferencedMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.Capabilities.Forward {
		rows = append(rows, modalRowSpec{
			ID:     "action:forward",
			Label:  "Forward",
			Action: ActionReceived{Action: ForwardMessageSource, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.Capabilities.Edit {
		rows = append(rows, modalRowSpec{
			ID:     "action:edit",
			Label:  "Edit",
			Action: ActionReceived{Action: EditMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if menu.Capabilities.Copy {
		rows = append(rows, modalRowSpec{
			ID:     "action:copy",
			Label:  "Copy",
			Action: ActionReceived{Action: CopyMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if menu.UserID != 0 {
		rows = append(rows, modalRowSpec{
			ID:     "action:user-info",
			Label:  "User info",
			Action: ActionReceived{Action: ViewUserInfo, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.CanReact {
		rows = append(rows, modalRowSpec{
			ID:     "action:react",
			Label:  "React",
			Action: ActionReceived{Action: ReactMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.Capabilities.Pin {
		pinLabel := "Pin"
		if menu.Pinned {
			pinLabel = "Unpin"
		}
		rows = append(rows, modalRowSpec{
			ID:     "action:pin",
			Label:  pinLabel,
			Action: ActionReceived{Action: PinMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.Capabilities.DeleteForSelf {
		rows = append(rows, modalRowSpec{
			ID:     "action:delete",
			Label:  "Delete",
			Action: ActionReceived{Action: DeleteMessage, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if !menu.Loading && menu.Error == nil && menu.Capabilities.DeleteForAll {
		rows = append(rows, modalRowSpec{
			ID:     "action:delete-all",
			Label:  "Delete for everyone",
			Action: ActionReceived{Action: DeleteForEveryone, ChatID: menu.ChatID, MessageID: menu.MessageID},
		})
	}
	if menu.Loading {
		rows = append(rows, modalRowSpec{Label: "Loading actions..."})
	} else if menu.Error != nil {
		rows = append(rows, modalRowSpec{Label: "Actions unavailable"})
	}
	if menu.JumpRequestID != 0 {
		rows = append(rows, modalRowSpec{Label: "Opening referenced message..."})
	}

	for index := range rows {
		rows[index].Selected = index == menu.Selected
		rows[index].Key = actionModalShortcut(rows[index].Action.Action)
	}
	return rows
}

// buildReactionPickerLayer adapts the reaction picker into the shared list
// modal. When a selector Huh View is injected it replaces the palette row
// labels and selected-row paint; omitted injection retains the legacy rows.
func buildReactionPickerLayer(model ViewModel, styles renderStyles, selectorView ...string) surfaceResult {
	if model.ReactionPicker == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	return buildListModal(image.Rect(0, 0, model.Width, model.Height), "React", reactionRows(model.ReactionPicker), styles, selectorView...)
}

// reactionRows is the single displayed row source for the reaction picker: one
// row per ReactionPalette entry in exact order. The Rune encodes the
// palette index as a supplementary-plane code point, preserving the
// mouse/reducer contract.
func reactionRows(picker *ReactionPicker) []modalRowSpec {
	if picker == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(ReactionPalette))
	for index, emoji := range ReactionPalette {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("reaction:%d", index),
			Label:    emoji,
			Selected: index == picker.Selected,
			Action: ActionReceived{
				Action:    Activate,
				ChatID:    picker.ChatID,
				MessageID: picker.MessageID,
				Rune:      rune(index + 0x10000),
			},
		})
	}
	return rows
}

// buildForwardPickerLayer adapts the forward picker into the shared list
// modal. When a selector Huh View is injected it replaces the chat row labels
// and selected-row paint; omitted injection retains the legacy rows.
func buildForwardPickerLayer(model ViewModel, styles renderStyles, selectorView ...string) surfaceResult {
	if model.ForwardPicker == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	return buildListModal(image.Rect(0, 0, model.Width, model.Height), "Forward to", forwardRows(model.ForwardPicker, model.Chats), styles, selectorView...)
}

// forwardRows is the single displayed row source for the forward picker: one
// row per ChatRow entry in exact current order. The destination is the
// chat's own ID, so the semantic payload survives chat reordering.
func forwardRows(picker *ForwardPicker, chats []ChatRow) []modalRowSpec {
	if picker == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(chats))
	for index, row := range chats {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("forward:%d:%d", index, row.Chat.ID),
			Label:    row.Chat.Title,
			Selected: index == picker.SelectedChat,
			Action:   ActionReceived{Action: Activate, ChatID: row.Chat.ID},
		})
	}
	return rows
}

// addModalRowLabel attaches an intrinsic label child at row local (1,0) when
// there is room, clipped to the row width minus the leading cell.
func addModalRowLabel(rowLayer *lipgloss.Layer, row components.Row, spec modalRowSpec, rowStyle lipgloss.Style) {
	if rowLayer == nil {
		return
	}
	labelWidth := row.Rect.Dx() - 1
	if spec.Key != "" {
		labelWidth -= 2 // gap plus right-aligned key
	}
	if labelWidth <= 0 || spec.Label == "" {
		return
	}
	clipped := ansi.Truncate(spec.Label, labelWidth, "")
	if clipped == "" {
		return
	}
	rowLayer.AddLayers(lipgloss.NewLayer(rowStyle.Render(clipped)).X(1).Y(0).Z(zModalContent))
}
