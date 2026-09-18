package frontend

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func chatSettingsFrame(bounds image.Rectangle) image.Rectangle {
	return centeredSurfaceRectangle(bounds, min(72, bounds.Dx()), min(16, bounds.Dy())).Intersect(bounds)
}

func buildChatSettingsLayer(model ui.ViewModel, styles renderStyles, inputView string) surfaceResult {
	settings := model.ChatSettings
	if settings == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	frame := chatSettingsFrame(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	content := styles.Panel.Border(lipgloss.RoundedBorder()).BorderForeground(styles.FocusedBorder.GetForeground()).
		BorderBackground(styles.FocusedBorder.GetBackground()).Width(frame.Dx()).Height(frame.Dy()).Render("")
	root := lipgloss.NewLayer(content).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)
	interactions := []layerInteraction{}
	title := chatSettingsTitle(settings)
	if settings.Notice != "" {
		title += " · " + sanitizeDisplayString(settings.Notice)
	}
	if title != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(ansi.Truncate(title, max(1, frame.Dx()-5), ""))).X(2).Y(0).Z(zModalContent))
	}
	closeLocal := image.Rect(frame.Dx()-2, 0, frame.Dx()-1, 1)
	interactions = append(interactions, addInteractive(root, frame.Min, closeLocal, "chat-settings:close", zModalControl, renderLine(styles.Accent, "×", 1), app.ActionReceived{Action: app.Close}, app.ActionReceived{}, app.ActionReceived{}))

	rows := app.ChatSettingsMenuItems(settings)
	if settings.Mode == app.ChatSettingsTitleEditor || settings.Mode == app.ChatSettingsDescriptionEditor {
		label := "Title"
		if settings.Mode == app.ChatSettingsDescriptionEditor {
			label = "Description"
		}
		addSettingsHeader(root, frame, 2, label, styles)
		inputRect := image.Rect(frame.Min.X+2, frame.Min.Y+3, frame.Max.X-2, frame.Min.Y+4)
		view := inputView
		if view == "" {
			value := settings.TitleInput
			if settings.Mode == app.ChatSettingsDescriptionEditor {
				value = settings.DescriptionInput
			}
			view = sanitizeDisplayString(string(value))
		}
		view = clipPhotoPathInputView(view, inputRect.Dx())
		root.AddLayers(lipgloss.NewLayer(renderLine(styles.Panel, view, inputRect.Dx())).X(inputRect.Min.X - frame.Min.X).Y(inputRect.Min.Y - frame.Min.Y).Z(zModalContent))
	}
	startY := 3
	if settings.Mode == app.ChatSettingsTitleEditor || settings.Mode == app.ChatSettingsDescriptionEditor {
		startY = 5
	}
	for index, item := range rows {
		y := frame.Min.Y + startY + index
		if y >= frame.Max.Y-1 {
			break
		}
		row := image.Rect(frame.Min.X+1, y, frame.Max.X-1, y+1)
		if item.Header {
			addSettingsHeader(root, frame, startY+index, item.Label, styles)
			continue
		}
		selected := index == settings.Selected
		style := styles.Panel
		if selected {
			style = styles.Selected
		}
		interactions = append(interactions, addInteractive(root, frame.Min, row.Sub(frame.Min), "chat-settings:row:"+itoa(index), zModalRow, renderEmptyBox(style, row.Dx(), 1), item.Action, app.ActionReceived{}, app.ActionReceived{}))
		root.AddLayers(lipgloss.NewLayer(style.Render(ansi.Truncate(item.Label, max(1, row.Dx()-2), ""))).X(row.Min.X - frame.Min.X + 1).Y(row.Min.Y - frame.Min.Y).Z(zModalContent))
	}
	if settings.Loading {
		addSettingsHeader(root, frame, frame.Dy()-2, "Loading chat settings...", styles)
	} else if settings.Working {
		addSettingsHeader(root, frame, frame.Dy()-2, "Saving...", styles)
	} else if settings.Error != nil {
		addSettingsHeader(root, frame, frame.Dy()-2, settings.Error.Message, styles)
	}
	return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
}

func addSettingsHeader(root *lipgloss.Layer, frame image.Rectangle, y int, label string, styles renderStyles) {
	if root == nil || y < 0 || y >= frame.Dy()-1 || label == "" {
		return
	}
	width := max(1, frame.Dx()-4)
	root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(ansi.Truncate(sanitizeDisplayString(label), width, ""))).X(2).Y(y).Z(zModalContent))
}

func chatSettingsTitle(settings *app.ChatSettingsState) string {
	switch settings.Mode {
	case app.ChatSettingsTitleEditor:
		return "Edit title"
	case app.ChatSettingsDescriptionEditor:
		return "Edit description"
	case app.ChatSettingsSlowModeMenu:
		return "Slow mode"
	default:
		return "Chat settings"
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[index:])
}
