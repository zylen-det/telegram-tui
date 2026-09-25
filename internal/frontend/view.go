package frontend

import (
	"image"
	"image/color"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
)

func (m Model) View() tea.View {
	content, cursorX, cursorY := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeNone
	view.WindowTitle = "telegram-tui Authorization"
	view.ForegroundColor = color.RGBA{R: textColor.r, G: textColor.g, B: textColor.b, A: 255}
	if cursorX >= 0 && cursorY >= 0 {
		view.Cursor = tea.NewCursor(cursorX, cursorY)
	}
	return view
}

func (m Model) render() (string, int, int) {
	if m.width == 0 || m.height == 0 {
		return "", -1, -1
	}

	bounds := image.Rect(0, 0, m.width, m.height)
	if m.width < minimumWidth || m.height < minimumHeight {
		frame := composeTooSmall(bounds, "telegram-tui requires at least 60x18")
		return frame.Content, -1, -1
	}

	frame := composeAuthorization(bounds, authorizationData{
		Label:     "Telegram API ID",
		Guidance:  "Enter the numeric api_id from my.telegram.org/apps",
		Input:     string(m.input),
		Submitted: m.submitted,
	})
	if !frame.Cursor.Visible {
		return frame.Content, -1, -1
	}
	return frame.Content, frame.Cursor.X, frame.Cursor.Y
}

func (m AppModel) View() tea.View {
	// Snapshot the owned state once, select the view model, and compose the
	// frame.
	state := m.Snapshot()
	model := Select(state, m.location)
	var titleView, descriptionView string
	if state.ChatSettings != nil {
		if state.ChatSettings.Mode == ChatSettingsTitleEditor && m.chatTitleInput.Identity() == state.ChatSettings.TitleEditorID {
			titleView = m.chatTitleInput.View()
		}
		if state.ChatSettings.Mode == ChatSettingsDescriptionEditor && m.chatDescriptionInput.Identity() == state.ChatSettings.DescriptionEditorID {
			descriptionView = m.chatDescriptionInput.View()
		}
	}
	frame := composeApplication(model, m.location, editorViews{
		Composer:      m.composerText.View(),
		Authorization: m.authorizationInput.View(),

		PhotoPath: m.photoPathInput.View(),

		MessageSearch: m.messageSearchInput.View(),

		ChatSearch: m.chatSearchInput.View(),

		ChatSettingsTitle:       titleView,
		ChatSettingsDescription: descriptionView,
	})
	m.setHitRegions(frame.Hits)
	title := m.publishOverlayDesired(frame.Overlay, frame.Inline)
	view := tea.NewView(frame.Content)
	view.AltScreen = true
	view.ReportFocus = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = title
	view.ForegroundColor = color.RGBA{R: textColor.r, G: textColor.g, B: textColor.b, A: 255}
	if frame.Cursor.Visible && frame.Cursor.X >= 0 && frame.Cursor.Y >= 0 {
		view.Cursor = tea.NewCursor(frame.Cursor.X, frame.Cursor.Y)
	}
	return view
}

func (m AppModel) publishOverlayDesired(request overlayRequest, inline []inlinePlacement) string {
	if m.overlay == nil {
		return overlayWindowTitle
	}
	m.overlay.SetInlineImages(inline)
	if !request.Ready {
		return m.overlay.ClearDesired()
	}
	return m.overlay.SetDesired(request.Path, request.Bounds, request.Columns, request.Rows, request.SelectedID)
}

func authorizationPromptGuidance(kind auth.PromptKind) string {
	switch kind {
	case auth.PromptAPIID:
		return "Enter the numeric api_id from my.telegram.org/apps"
	case auth.PromptAPIHash:
		return "Enter the api_hash from my.telegram.org/apps (saved in local config)"
	case auth.PromptPhone:
		return "Enter your Telegram phone number, including country code"
	case auth.PromptCode:
		return "Enter the verification code sent by Telegram"
	case auth.PromptPassword:
		return "Enter your Telegram two-step verification password"
	default:
		return "Type a value and press Enter"
	}
}
