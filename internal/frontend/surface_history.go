package frontend

import (
	"image"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

// buildHistoryLayer builds the conversation history surface: a pane background
// root with one absolute interaction, an optional error/loading/empty state,
// and bottom-aligned message groups. The viewport rect is first intersected
// with the model bounds. All children use coordinates local to the history
// root; interactions stay absolute.
//
// Rect is the absolute history viewport rectangle. An empty intersection
// returns a zero surface with a hidden cursor.
func buildHistoryLayer(
	model ViewModel,
	rect image.Rectangle, // absolute history viewport
	location *time.Location,
	styles renderStyles,
) surfaceResult {
	viewport := image.Rect(0, 0, model.Width, model.Height)
	rect = rect.Intersect(viewport)
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	rootContent := renderEmptyBox(styles.Panel, rect.Dx(), rect.Dy())
	root := lipgloss.NewLayer(rootContent).X(rect.Min.X).Y(rect.Min.Y).Z(zPaneBackground)
	root.ID("conversation:history")

	interactions := []layerInteraction{{
		ID:        "conversation:history",
		Rect:      rect,
		Z:         zPaneBackground,
		Click:     ActionReceived{Action: FocusPane, TargetFocus: FocusConversation},
		WheelUp:   ActionReceived{Action: PageUp},
		WheelDown: ActionReceived{Action: PageDown},
	}}

	// Content rect is a mutable copy; each consumed state row shrinks it.
	content := rect
	width := content.Dx()

	// Error state consumes the first content row as a full-width retry child.
	if model.HistoryError != nil {
		errLocal := image.Rect(0, 0, width, 1)
		interactions = append(interactions, addInteractive(root, rect.Min, errLocal, "conversation:history-retry", zControl,
			renderLine(styles.Error, model.HistoryError.Message+"  Retry", width),
			ActionReceived{Action: Retry}, ActionReceived{}, ActionReceived{}))
		content.Min.Y++
		if content.Empty() {
			return surfaceResult{Layer: root, Rect: rect, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
		}
	}

	// Empty group states: centered intrinsic message; mutually exclusive in
	// that order. No extra hits and return.
	if len(model.Groups) == 0 {
		var text string
		switch {
		case model.HistoryLoading:
			text = "Loading messages..."
		case model.HistoryDone && model.ActiveChat.IsForum && !model.ActiveTopicKnown && !model.ShowAllTopics:
			text = "Select a topic to view messages"
		case model.HistoryDone:
			text = "No messages"
		default:
			// Neither loading nor done: render nothing between the states.
			return surfaceResult{Layer: root, Rect: rect, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
		}
		displayTextWidth := min(width, displayWidth(text))
		contentStr := renderLine(styles.Muted, text, displayTextWidth)
		x := centeredX(width, displayTextWidth)
		y := content.Min.Y - rect.Min.Y + content.Dy()/2
		root.AddLayers(lipgloss.NewLayer(contentStr).X(x).Y(y).Z(zContent))
		return surfaceResult{Layer: root, Rect: rect, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
	}

	// Non-empty groups plus still loading: consume the first remaining row.
	if model.HistoryLoading {
		if content.Dy() <= 0 {
			return surfaceResult{Layer: root, Rect: rect, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
		}
		root.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "Loading older messages...", width)).
			X(0).Y(content.Min.Y - rect.Min.Y).Z(zContent))
		content.Min.Y++
		if content.Empty() {
			return surfaceResult{Layer: root, Rect: rect, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}}
		}
	}

	groups := model.Groups
	if !model.HistoryFollowSelection {
		groups = groupsBeforeSurfaceOffset(model.Groups, model.HistoryOffset)
	}
	inlineThumbnailsByID := make(map[domain.MessageID]thumbnail.Block, len(model.InlineThumbnails))
	for _, thumb := range model.InlineThumbnails {
		inlineThumbnailsByID[thumb.MessageID] = thumb.Block
	}

	selection := messageSelection{ChatID: model.SelectedMessageChat, MessageID: model.SelectedMessage}
	buildGroup := func(group RenderedMessageGroup) messageGroupResult {
		return buildMessageGroupLayer(group, width, location, selection, inlineThumbnailsByID, styles)
	}
	results := buildVisibleHistoryResults(groups, content.Dy(), model.HistoryFollowSelection, selection, buildGroup)

	// Bottom-up placement over the content rect. One blank separator row is
	// background only and never gets a layer or interaction. Keyboard message
	// navigation centers the selected message while there is overflow on both
	// sides; near either end it clamps to the content edge without blank rows.
	cursor := content.Max.Y
	if model.HistoryFollowSelection {
		cursor = centeredHistoryCursor(results, content)
	}
	var inline []inlinePlacement
	for index := len(results) - 1; index >= 0; index-- {
		if index < len(results)-1 {
			cursor--
		}
		result := results[index]
		top := cursor - result.Height
		fullRect := image.Rect(content.Min.X, top, content.Max.X, cursor)
		visible := fullRect.Intersect(content)
		if !visible.Empty() {
			startRow := visible.Min.Y - fullRect.Min.Y
			endRow := visible.Max.Y - fullRect.Min.Y
			fragment := result
			if startRow != 0 || endRow != result.Height {
				fragment = sliceMessageGroupLayer(result, startRow, endRow, styles, inlineThumbnailsByID)
			}
			if fragment.Layer != nil {
				fragmentPosX := visible.Min.X - rect.Min.X
				fragmentPosY := visible.Min.Y - rect.Min.Y
				fragment.Layer.X(fragmentPosX).Y(fragmentPosY)
				root.AddLayers(fragment.Layer)
				// Translate each local interaction by visible.Min to absolute.
				for _, li := range fragment.LocalInteractions {
					interactions = append(interactions, layerInteraction{
						ID:    li.ID,
						Rect:  li.Rect.Add(visible.Min),
						Z:     li.Z,
						Click: li.Click,
					})
				}
			}
			// Kitty inline thumbnails: translate each full-group-local
			// placement (already gated to the fully visible slice) to absolute
			// viewport cells. Both full groups and sliced fragments anchor
			// against the full group's top-left.
			for _, placement := range fragment.Inline {
				inline = append(inline, inlinePlacement{
					ImageID: placement.ImageID,
					X:       fullRect.Min.X + placement.X,
					Y:       fullRect.Min.Y + placement.Y,
					Width:   placement.Width,
					Height:  placement.Height,
					Text:    placement.Text,
				})
			}
		}
		cursor = top
		if cursor <= content.Min.Y {
			break
		}
	}

	return surfaceResult{
		Layer:        root,
		Rect:         rect,
		Interactions: interactions,
		Cursor:       renderCursor{X: -1, Y: -1},
		Inline:       clipInlinePlacements(inline, content),
	}
}

// buildVisibleHistoryResults builds only enough groups to cover the viewport.
// Ordinary history is filled backward from its newest remaining group. While
// following keyboard selection, the selected message (possibly inside a tall
// sender group) is surrounded by content for centering and edge clamping.
func buildVisibleHistoryResults(
	groups []RenderedMessageGroup,
	viewportHeight int,
	followSelection bool,
	selection messageSelection,
	build func(RenderedMessageGroup) messageGroupResult,
) []messageGroupResult {
	if len(groups) == 0 || viewportHeight <= 0 {
		return nil
	}
	selected := -1
	if followSelection {
		selected = selectedHistoryGroupIndex(groups, selection)
	}
	if selected < 0 {
		return buildHistoryTailResults(groups, viewportHeight, build)
	}

	center := build(groups[selected])
	selectedStart, selectedEnd := center.SelectedStart, center.SelectedEnd
	remaining := max(0, viewportHeight-(selectedEnd-selectedStart))
	aboveNeed := remaining / 2
	belowNeed := remaining - aboveNeed

	var after []messageGroupResult
	belowExtent := center.Height - selectedEnd
	nextAfter := selected + 1
	for nextAfter < len(groups) && belowExtent < belowNeed {
		result := build(groups[nextAfter])
		after = append(after, result)
		belowExtent += result.Height + 1
		nextAfter++
	}

	// If the selection is near the newest edge, use the unavailable lower
	// half of the viewport for additional older content.
	aboveTarget := aboveNeed
	if nextAfter == len(groups) && belowExtent < belowNeed {
		aboveTarget += belowNeed - belowExtent
	}

	var beforeReverse []messageGroupResult
	aboveExtent := selectedStart
	nextBefore := selected - 1
	for nextBefore >= 0 && aboveExtent < aboveTarget {
		result := build(groups[nextBefore])
		beforeReverse = append(beforeReverse, result)
		aboveExtent += result.Height + 1
		nextBefore--
	}

	// Symmetrically, a selection near the oldest edge gives its unavailable
	// upper half to newer content.
	if nextBefore < 0 && aboveExtent < aboveNeed {
		belowTarget := belowNeed + aboveNeed - aboveExtent
		for nextAfter < len(groups) && belowExtent < belowTarget {
			result := build(groups[nextAfter])
			after = append(after, result)
			belowExtent += result.Height + 1
			nextAfter++
		}
	}

	results := make([]messageGroupResult, 0, len(beforeReverse)+1+len(after))
	for index := len(beforeReverse) - 1; index >= 0; index-- {
		results = append(results, beforeReverse[index])
	}
	results = append(results, center)
	results = append(results, after...)
	return results
}

func buildHistoryTailResults(
	groups []RenderedMessageGroup,
	viewportHeight int,
	build func(RenderedMessageGroup) messageGroupResult,
) []messageGroupResult {
	var reverse []messageGroupResult
	extent := 0
	for index := len(groups) - 1; index >= 0 && extent < viewportHeight; index-- {
		result := build(groups[index])
		if len(reverse) > 0 {
			extent++
		}
		extent += result.Height
		reverse = append(reverse, result)
	}
	results := make([]messageGroupResult, len(reverse))
	for index := range reverse {
		results[len(reverse)-1-index] = reverse[index]
	}
	return results
}

func selectedHistoryGroupIndex(groups []RenderedMessageGroup, selection messageSelection) int {
	for groupIndex, group := range groups {
		for _, message := range group.Messages {
			if message.ChatID == selection.ChatID && message.ID == selection.MessageID {
				return groupIndex
			}
		}
	}
	return -1
}

func centeredHistoryCursor(results []messageGroupResult, content image.Rectangle) int {
	if len(results) == 0 || content.Empty() {
		return content.Max.Y
	}

	totalHeight := 0
	selectedTop := 0
	selectedHeight := 0
	for index, result := range results {
		if index > 0 {
			totalHeight++
		}
		if result.Selected {
			selectedTop = totalHeight + result.SelectedStart
			selectedHeight = result.SelectedEnd - result.SelectedStart
		}
		totalHeight += result.Height
	}
	if selectedHeight == 0 || totalHeight <= content.Dy() {
		return content.Max.Y
	}

	desiredTop := content.Min.Y + (content.Dy()-selectedHeight)/2
	origin := desiredTop - selectedTop
	origin = max(content.Max.Y-totalHeight, min(content.Min.Y, origin))
	return origin + totalHeight
}
