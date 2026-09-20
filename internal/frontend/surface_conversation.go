package frontend

import (
	"image"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// buildConversationLayer assembles the conversation pane surface from the
// accepted sub-panes: a rounded pane root, an optional info control, the
// history surface, and the composer surface.
//
// The pane rect is first intersected with the model viewport. The returned
// conversation pane root carries its child history/composer Layer roots nested
// beneath it in pane-local coordinates, while every returned interaction is in
// absolute viewport coordinates and every layer's global Z remains unchanged.
//
// A pane that is empty or below buildPane's minimum returns a zero surface
// with a hidden cursor.
func buildConversationLayer(
	model ViewModel,
	location *time.Location,
	styles renderStyles,
	composerView ...string,
) surfaceResult {
	rect := model.Layout.Conversation.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	title := model.ActiveChat.Title
	if title == "" {
		title = "Conversation"
	}
	if model.ActiveTopicKnown {
		topicName := model.ActiveTopic.Name
		if topicName == "" {
			topicName = "Topic"
		}
		title += " › " + topicName
	}

	pane := buildPane(
		rect,
		title,
		model.Focus == FocusConversation || model.Focus == FocusComposer,
		"pane:conversation",
		FocusConversation,
		styles,
	)
	if pane.Layer == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	interactions := []layerInteraction(pane.Interactions)

	// Optional info control on the top border.
	if model.ActiveChat.ID != 0 && !model.DetailsOpen && rect.Dx() >= 3 {
		infoLocal := image.Rect(rect.Dx()-2, 0, rect.Dx()-1, 1)
		info := addInteractive(
			pane.Layer, rect.Min, infoLocal, "conversation:info", zControl,
			styles.Accent.Render("ⓘ"),
			ActionReceived{Action: ToggleDetails}, ActionReceived{}, ActionReceived{},
		)
		interactions = append(interactions, info)
	}

	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	if inner.Empty() {
		return surfaceResult{
			Layer:        pane.Layer,
			Rect:         rect,
			Interactions: interactions,
			Cursor:       renderCursor{X: -1, Y: -1},
		}
	}

	composerRect := composerSurfaceRect(model)
	historyRect := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, composerRect.Min.Y)

	if composerRect.Empty() {
		return surfaceResult{
			Layer:        pane.Layer,
			Rect:         rect,
			Interactions: interactions,
			Cursor:       renderCursor{X: -1, Y: -1},
		}
	}

	if model.ActiveChat.ID == 0 {
		// No active conversation: no interactive history; only an intrinsic
		// centered placeholder plus the inactive composer background.
		if !historyRect.Empty() {
			text := "No conversation"
			textWidth := min(historyRect.Dx(), displayWidth(text))
			xAbs := historyRect.Min.X + centeredX(historyRect.Dx(), textWidth)
			yAbs := historyRect.Min.Y + historyRect.Dy()/2
			clipped := ansi.Truncate(text, textWidth, "")
			content := styles.Muted.Render(clipped)
			pane.Layer.AddLayers(lipgloss.NewLayer(content).
				X(xAbs - rect.Min.X).Y(yAbs - rect.Min.Y).Z(zContent))
		}
		composer := buildComposerLayer(model, composerRect, styles, composerView...)
		if composer.Layer != nil {
			anchorChild(pane.Layer, composer.Layer, rect.Min)
		}
		return surfaceResult{
			Layer:        pane.Layer,
			Rect:         rect,
			Interactions: interactions,
			Cursor:       renderCursor{X: -1, Y: -1},
		}
	}

	// Active conversation: history + composer sub-panes nested under the pane.
	history := buildHistoryLayer(model, historyRect, location, styles)
	if history.Layer != nil {
		anchorChild(pane.Layer, history.Layer, rect.Min)
	}
	interactions = append(interactions, history.Interactions...)

	composer := buildComposerLayer(model, composerRect, styles, composerView...)
	if composer.Layer != nil {
		anchorChild(pane.Layer, composer.Layer, rect.Min)
	}
	interactions = append(interactions, composer.Interactions...)

	commandMenu := buildCommandMenuLayer(model, historyRect, composerRect, styles)
	inline := history.Inline
	if commandMenu.Layer != nil {
		anchorChild(pane.Layer, commandMenu.Layer, rect.Min)
		interactions = append(interactions, commandMenu.Interactions...)
		inline = clipInlinePlacementsWithOverlay(inline, historyRect, commandMenu.Rect)
	}

	return surfaceResult{
		Layer:        pane.Layer,
		Rect:         rect,
		Interactions: interactions,
		Cursor:       composer.Cursor,
		Inline:       inline,
	}
}

// anchorChild repositions a fresh child root from absolute viewport coordinates
// to pane-local coordinates relative to the pivot's Min, then nests it beneath
// the pane. The child's Rect/Interactions/Cursor remain absolute.
func anchorChild(pane *lipgloss.Layer, child *lipgloss.Layer, pivot image.Point) {
	child.X(child.GetX() - pivot.X)
	child.Y(child.GetY() - pivot.Y)
	pane.AddLayers(child)
}
