package frontend

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// buildDetailsLayer builds the details pane surface: a rounded pane root with
// an intrinsic title, a close control, and the active chat's avatar, title,
// username, and optional View image action. Interactions are absolute. An
// empty viewport intersection returns a zero surface.
func buildDetailsLayer(model ViewModel, styles renderStyles) surfaceResult {
	rect := model.Layout.Details.Intersect(image.Rect(0, 0, model.Width, model.Height))
	if rect.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	pane := buildPane(rect, "Info", model.Focus == FocusDetails, "pane:details", FocusDetails, styles)
	if pane.Layer == nil {
		return pane
	}

	interactions := append([]layerInteraction(nil), pane.Interactions...)

	// Close control in the top-right corner when width permits.
	if rect.Dx() >= 2 {
		closeLocal := image.Rect(rect.Dx()-2, 0, rect.Dx()-1, 1)
		closeContent := styles.Accent.Render("×")
		interactions = append(interactions, addInteractive(
			pane.Layer, rect.Min, closeLocal, "details:close", zControl, closeContent,
			ActionReceived{Action: ToggleDetails}, ActionReceived{}, ActionReceived{},
		))
	}

	inner := rect.Inset(1)
	if inner.Empty() {
		return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
	}

	if model.ActiveChat.ID == 0 {
		// Centered intrinsic "No conversation", clipped to inner width.
		text := "No conversation"
		width := min(inner.Dx(), displayWidth(text))
		clipped := ansi.Truncate(text, width, "")
		content := renderLine(styles.Muted, clipped, width)
		x := inner.Min.X + centeredX(inner.Dx(), width)
		y := inner.Min.Y + inner.Dy()/2
		pane.Layer.AddLayers(lipgloss.NewLayer(content).X(x - rect.Min.X).Y(y - rect.Min.Y).Z(zContent))
		return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
	}

	row := activeChatSurfaceRow(model)

	avatarWidth := min(12, inner.Dx())
	avatarHeight := min(6, inner.Dy())
	avatarX := inner.Min.X + (inner.Dx()-avatarWidth)/2
	avatarRect := image.Rect(avatarX, inner.Min.Y, avatarX+avatarWidth, inner.Min.Y+avatarHeight)
	y := avatarRect.Max.Y + 1

	// Avatar. On retry (only for an exact 12x6 avatar) the avatar root moves to
	// zControl, gets an ID, and carries a clipped Retry text child on its
	// bottom row; the interaction is absolute over the avatar rect.
	avatarZ := zContent
	if row.AvatarError != nil && avatarRect.Dx() == 12 && avatarRect.Dy() == 6 {
		avatarZ = zControl
	}
	avatarLocal := image.Rect(avatarRect.Min.X-rect.Min.X, avatarRect.Min.Y-rect.Min.Y, avatarRect.Max.X-rect.Min.X, avatarRect.Max.Y-rect.Min.Y)
	if avatarLayer := buildAvatarLayer(avatarLocal, row.Avatar, row.AvatarKey, model.ActiveChat.Title, styles, avatarZ); avatarLayer != nil {
		if row.AvatarError != nil && avatarRect.Dx() == 12 && avatarRect.Dy() == 6 {
			avatarLayer.ID("details:avatar-retry")
			retry := ansi.Truncate("Retry", avatarLocal.Dx(), "")
			avatarLayer.AddLayers(lipgloss.NewLayer(styles.Error.Render(retry)).
				X(0).Y(avatarLocal.Dy() - 1).Z(zControl + 2))
			interactions = append(interactions, layerInteraction{
				ID:    "details:avatar-retry",
				Rect:  avatarRect,
				Z:     zControl,
				Click: ActionReceived{Action: Retry, AvatarKey: row.AvatarKey},
			})
		}
		pane.Layer.AddLayers(avatarLayer)
	}

	// Centered title.
	if y < inner.Max.Y {
		titleText := model.ActiveChat.Title
		titleWidth := min(inner.Dx(), displayWidth(titleText))
		titleClipped := ansi.Truncate(titleText, titleWidth, "")
		titleContent := renderLine(styles.Emphasis, titleClipped, titleWidth)
		titleX := inner.Min.X + centeredX(inner.Dx(), titleWidth)
		pane.Layer.AddLayers(lipgloss.NewLayer(titleContent).X(titleX - rect.Min.X).Y(y - rect.Min.Y).Z(zContent))
		y++
	}

	// Optional @username.
	if model.ActiveChat.Username != "" && y < inner.Max.Y {
		userText := "@" + model.ActiveChat.Username
		userWidth := min(inner.Dx(), displayWidth(userText))
		userClipped := ansi.Truncate(userText, userWidth, "")
		userContent := renderLine(styles.Muted, userClipped, userWidth)
		userX := inner.Min.X + centeredX(inner.Dx(), userWidth)
		pane.Layer.AddLayers(lipgloss.NewLayer(userContent).X(userX - rect.Min.X).Y(y - rect.Min.Y).Z(zContent))
		y++
	}

	// Keyboard (j/k + Enter) follows model.DetailsSelected; full-row mouse
	// actions dispatch the same dynamic, permission-gated action order.
	// Selection matches the action-modal cursor: a "> " prefix on the
	// focused selected row, plain text everywhere else.
	selected := model.DetailsSelected
	focused := model.Focus == FocusDetails
	actionText := func(index int, text string) string {
		if focused && selected == index {
			return "> " + text
		}
		return text
	}
	for index, item := range DetailsActionItems(model.ActiveChat) {
		if y >= inner.Max.Y {
			break
		}
		text := actionText(index, item.Label)
		width := min(inner.Dx(), displayWidth(text))
		x := inner.Min.X + (inner.Dx()-width)/2
		actionLocal := image.Rect(x-rect.Min.X, y-rect.Min.Y, x-rect.Min.X+width, y-rect.Min.Y+1)
		actionContent := renderLine(styles.Accent, text, width)
		interactions = append(interactions, addInteractive(
			pane.Layer, rect.Min, actionLocal, detailsActionID(item.Action), zControl, actionContent,
			ActionReceived{Action: item.Action}, ActionReceived{}, ActionReceived{},
		))
		y++
	}

	return surfaceResult{Layer: pane.Layer, Rect: pane.Rect, Interactions: interactions, Cursor: pane.Cursor}
}

func detailsActionID(action Action) string {
	switch action {
	case OpenDetailsAvatar:
		return "details:view-image"
	case OpenMembers:
		return "details:members"
	case OpenInviteLinks:
		return "details:invite-links"
	case OpenGroupPermissions:
		return "details:group-permissions"
	case OpenChatSettings:
		return "details:chat-settings"
	default:
		return "details:action"
	}
}
