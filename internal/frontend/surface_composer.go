package frontend

import (
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// buildComposerLayer builds the composer surface for the given absolute
// viewport rect, clipped to the model viewport. The returned layer roots and
// interaction rectangles are absolute; all children are positioned in local
// coordinates relative to the fixed-size root.
func buildComposerLayer(
	model ViewModel,
	rect image.Rectangle,
	styles renderStyles,
	composerView ...string,
) surfaceResult {
	// Huh View injection: obtain the first variadic string; absent input
	// yields an empty string that the Huh host already rendered.
	composerStr := composerTextView(composerView)

	// Place the Huh string at the accepted text rect origin, localised to
	// the composer root. Huh was sized to textRect dimensions by L1d2.
	rect = rect.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	width := rect.Dx()
	height := rect.Dy()

	contentStyle := styles.Panel

	rootContent := contentStyle.Width(width).Height(height).Render("")
	root := lipgloss.NewLayer(rootContent).X(rect.Min.X).Y(rect.Min.Y).Z(zPaneBackground)

	interactive := model.ActiveChat.ID != 0 && model.ActiveChat.CanSend

	var interactions []layerInteraction
	if interactive {
		root.ID("composer")
		interactions = append(interactions, layerInteraction{
			ID:    "composer",
			Rect:  rect,
			Z:     zPaneBackground,
			Click: ActionReceived{Action: FocusPane, TargetFocus: FocusComposer},
		})
	}

	// No active chat: bare fixed root with no controls or cursor.
	if model.ActiveChat.ID == 0 {
		return surfaceResult{
			Layer:        root,
			Rect:         rect,
			Interactions: interactions,
			Cursor:       renderCursor{X: -1, Y: -1},
		}
	}

	// Read-only chat or closed topic: intrinsic grapheme-clipped label bounded
	// to the tiny rect.
	if !model.ActiveChat.CanSend || (model.ActiveTopicKnown && model.ActiveTopic.IsClosed) {
		if width > 1 && height > 0 {
			readOnly := ansi.Truncate("Read-only", width-1, "")
			if readOnly != "" {
				y := min(1, height-1)
				root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(readOnly)).
					X(1).Y(y).Z(zContent))
			}
		}
		return surfaceResult{
			Layer:        root,
			Rect:         rect,
			Interactions: interactions,
			Cursor:       renderCursor{X: -1, Y: -1},
		}
	}

	contentTop := 0

	// Reply banner (intrinsic, clipped strictly before the cancel interval).
	if model.ReplyTarget != nil && model.ReplyTarget.ChatID == model.ActiveChat.ID && contentTop < height {
		cancelX := max(0, width-displayWidth(cancelText)-1)
		if cancelX > 1 {
			banner := "Reply to " + model.ReplyTarget.Sender + ": " + model.ReplyTarget.Preview
			clippedBanner := ansi.Truncate(banner, cancelX-1, "")
			if clippedBanner != "" {
				root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(clippedBanner)).
					X(1).Y(contentTop).Z(zContent))
			}
		}
		cancelLocal := image.Rect(cancelX, contentTop, min(width, cancelX+displayWidth(cancelText)), contentTop+1).
			Intersect(image.Rect(0, contentTop, width, contentTop+1))
		if !cancelLocal.Empty() {
			cancelContent := renderLine(styles.Accent, cancelText, cancelLocal.Dx())
			interactions = append(interactions, addInteractive(
				root, rect.Min, cancelLocal, "composer:cancel-reply", zControl, cancelContent,
				ActionReceived{Action: CancelReply}, ActionReceived{}, ActionReceived{},
			))
		}
		contentTop++
	}

	// Edit banner (intrinsic, clipped strictly before its cancel), plus the
	// optional "Edit failed" error row directly beneath it.
	if model.EditTarget != nil && model.EditTarget.ChatID == model.ActiveChat.ID && contentTop < height {
		cancelX := max(0, width-displayWidth(cancelText)-1)
		if cancelX > 1 {
			clippedBanner := ansi.Truncate("Editing message", cancelX-1, "")
			if clippedBanner != "" {
				root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(clippedBanner)).
					X(1).Y(contentTop).Z(zContent))
			}
		}
		cancelLocal := image.Rect(cancelX, contentTop, min(width, cancelX+displayWidth(cancelText)), contentTop+1).
			Intersect(image.Rect(0, contentTop, width, contentTop+1))
		if !cancelLocal.Empty() {
			cancelContent := renderLine(styles.Accent, cancelText, cancelLocal.Dx())
			interactions = append(interactions, addInteractive(
				root, rect.Min, cancelLocal, "composer:cancel-edit", zControl, cancelContent,
				ActionReceived{Action: CancelEdit}, ActionReceived{}, ActionReceived{},
			))
		}
		contentTop++
		if model.EditTarget.Error != nil && contentTop < height-1 {
			if width > 1 {
				clipped := ansi.Truncate("Edit failed", width-1, "")
				if clipped != "" {
					root.AddLayers(lipgloss.NewLayer(styles.Error.Render(clipped)).
						X(1).Y(contentTop).Z(zContent))
				}
			}
			contentTop++
		}
	}

	// Send control always exists for an active writable chat when the rect
	// intersection is non-empty.
	controls := composerControls(width, interactive)
	sendY := height - 1
	sendLocal := image.Rect(controls.sendX, sendY, min(width, controls.sendX+displayWidth(sendText)), sendY+1).
		Intersect(image.Rect(0, 0, width, height))
	if !sendLocal.Empty() {
		sendContent := renderLine(styles.Accent, sendText, sendLocal.Dx())
		interactions = append(interactions, addInteractive(
			root, rect.Min, sendLocal, "composer:send", zControl, sendContent,
			ActionReceived{Action: ComposerSubmit}, ActionReceived{}, ActionReceived{},
		))
	}

	// Media controls sit immediately left of Send and use full-row hit boxes.
	if controls.hasPhoto {
		photoLocal := image.Rect(controls.photoX, sendY, controls.photoX+displayWidth(photoText), sendY+1)
		photoContent := renderLine(styles.Accent, photoText, photoLocal.Dx())
		interactions = append(interactions, addInteractive(
			root, rect.Min, photoLocal, "composer:photo", zControl, photoContent,
			ActionReceived{Action: OpenPhotoSend}, ActionReceived{}, ActionReceived{},
		))
	}
	if controls.hasSticker {
		stickerLocal := image.Rect(controls.stickerX, sendY, controls.stickerX+displayWidth(stickerText), sendY+1)
		stickerContent := renderLine(styles.Accent, stickerText, stickerLocal.Dx())
		interactions = append(interactions, addInteractive(
			root, rect.Min, stickerLocal, "composer:sticker", zControl, stickerContent,
			ActionReceived{Action: OpenStickerPicker}, ActionReceived{}, ActionReceived{},
		))
	}

	textRect := composerTextRect(model, rect)
	if composerStr != "" && !textRect.Empty() {
		localX := textRect.Min.X - rect.Min.X
		localY := textRect.Min.Y - rect.Min.Y
		clipped := clipComposerTextView(composerStr, textRect.Dx(), textRect.Dy())
		root.AddLayers(lipgloss.NewLayer(clipped).
			X(localX).Y(localY).Z(zContent))
	}

	// Composer cursor is hidden; the Huh View owns all visible editing
	// cursor styling for the writable composer.
	cursor := renderCursor{X: -1, Y: -1}

	return surfaceResult{
		Layer:        root,
		Rect:         rect,
		Interactions: interactions,
		Cursor:       cursor,
	}
}

// composerTextView extracts the first variadic Huh View string; absent
// input yields an empty string so existing direct builder calls render
// a composer with no draft content.
func composerTextView(composerView []string) string {
	if len(composerView) == 0 {
		return ""
	}
	return composerView[0]
}

// Shared fixed banner text labels for the composer controls.
// clipComposerTextView restricts ANSI text to the fixed composer text rect
// dimensions: at most `height` rows, each truncated to `width` cells.
// Returns empty string when view is empty or dimensions are invalid.
func clipComposerTextView(view string, width, height int) string {
	if view == "" || width <= 0 || height <= 0 {
		return ""
	}
	rows := strings.Split(view, "\n")
	if len(rows) > height {
		rows = rows[:height]
	}
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "")
	}
	return strings.Join(rows, "\n")
}

const (
	cancelText  = "[Cancel]"
	sendText    = "[Send]"
	photoText   = "[Photo]"
	stickerText = "[Sticker]"
)
