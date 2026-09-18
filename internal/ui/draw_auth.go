package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/mattn/go-runewidth"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func drawStatus(buffer *gotui.Buffer, model ViewModel) {
	rectangle := model.Layout.Status.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	buffer.Fill(gotui.NewCell(' ', baseStyle), rectangle)
	drawClipped(buffer, image.Pt(rectangle.Min.X+1, rectangle.Min.Y), max(0, rectangle.Dx()-2), "telegram-tui", accentStyle)

	connection := connectionLabel(model.Connection)
	connectionWidth := runewidth.StringWidth(connection)
	connectionX := rectangle.Max.X - 1 - connectionWidth
	if connectionX > rectangle.Min.X {
		drawClipped(buffer, image.Pt(connectionX, rectangle.Min.Y), connectionWidth, connection, connectionStyle(model.Connection))
	}

	title := model.ActiveChat.Title
	if title == "" {
		title = "Chats"
	}
	titleWidth := runewidth.StringWidth(title)
	titleX := rectangle.Min.X + (rectangle.Dx()-titleWidth)/2
	leftLimit := rectangle.Min.X + len("telegram-tui") + 3
	rightLimit := connectionX - 2
	if titleX < leftLimit {
		titleX = leftLimit
	}
	if titleX < rightLimit {
		drawClipped(buffer, image.Pt(titleX, rectangle.Min.Y), rightLimit-titleX, title, gotui.NewStyle(textColor, backgroundColor))
	}
}

func promptHelp(kind auth.PromptKind) string {
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

func drawPrompt(buffer *gotui.Buffer, model ViewModel) {
	if model.Prompt == nil || buffer == nil {
		return
	}
	bounds := terminalBounds(buffer, model)
	width := min(56, bounds.Dx()-4)
	height := min(9, bounds.Dy())
	if width < 4 || height < 4 {
		return
	}
	rectangle := centeredRectangle(bounds, width, height)
	inner := drawRoundedBlock(buffer, rectangle, "Authorization", model.Focus == app.FocusAuth)
	if inner.Empty() {
		return
	}

	drawClipped(buffer, inner.Min, inner.Dx(), model.Prompt.Prompt.Label, emphasisStyle)
	helpY := min(inner.Max.Y-1, inner.Min.Y+1)
	if helpY >= inner.Min.Y {
		drawClipped(buffer, image.Pt(inner.Min.X, helpY), inner.Dx(), promptHelp(model.Prompt.Prompt.Kind), mutedStyle)
	}
	value := string(model.Prompt.Input)
	if model.Prompt.Prompt.Secret {
		value = strings.Repeat("•", len(model.Prompt.Input))
	}
	inputY := min(inner.Max.Y-1, inner.Min.Y+3)
	if inputY >= inner.Min.Y {
		inputRect := image.Rect(inner.Min.X, inputY, inner.Max.X, inputY+1).Intersect(inner)
		buffer.Fill(gotui.NewCell(' ', gotui.NewStyle(textColor, selectedColor)), inputRect)
		display := value
		if display == "" {
			display = "Type here…"
		}
		drawClipped(buffer, inputRect.Min, inputRect.Dx(), display, gotui.NewStyle(textColor, selectedColor))
	}
	footerY := min(inner.Max.Y-1, inputY+1)
	if footerY > inputY {
		drawClipped(buffer, image.Pt(inner.Min.X, footerY), inner.Dx(), "Enter: continue   Backspace: delete   Ctrl-C: quit", mutedStyle)
	}
}

func drawToast(buffer *gotui.Buffer, model ViewModel) {
	if model.Toast == nil || model.Layout.Conversation.Empty() {
		return
	}
	inner := insetRectangle(model.Layout.Conversation.Intersect(buffer.Rectangle), 1)
	if inner.Empty() {
		return
	}
	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	y := composerTop - 1
	if y < inner.Min.Y {
		return
	}
	style := warningStyle
	if model.Toast.Kind == domain.ErrorInternal || model.Toast.Kind == domain.ErrorStorage {
		style = errorStyle
	}
	message := fmt.Sprintf("[%s] %s", model.Toast.Kind, model.Toast.Message)
	buffer.Fill(gotui.NewCell(' ', style), image.Rect(inner.Min.X, y, inner.Max.X, y+1))
	drawClipped(buffer, image.Pt(inner.Min.X, y), inner.Dx(), message, style)
}

func drawTooSmall(buffer *gotui.Buffer, model ViewModel) {
	if buffer == nil || buffer.Empty() {
		return
	}
	buffer.Fill(gotui.NewCell(' ', baseStyle), buffer.Rectangle)
	bounds := terminalBounds(buffer, model)
	message := fmt.Sprintf("telegram-tui requires at least 60x18; current %dx%d", model.Width, model.Height)
	x := bounds.Min.X + max(0, (bounds.Dx()-runewidth.StringWidth(message))/2)
	y := bounds.Min.Y + bounds.Dy()/2
	drawClipped(buffer, image.Pt(x, y), bounds.Max.X-x, message, warningStyle)
}

func connectionLabel(connection domain.ConnectionState) string {
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

func connectionStyle(connection domain.ConnectionState) gotui.Style {
	switch connection {
	case domain.ConnectionOnline:
		return gotui.NewStyle(accentColor, backgroundColor)
	case domain.ConnectionOffline:
		return gotui.NewStyle(errorColor, backgroundColor)
	case domain.ConnectionReconnecting:
		return gotui.NewStyle(warningColor, backgroundColor)
	default:
		return gotui.NewStyle(mutedTextColor, backgroundColor)
	}
}

func terminalBounds(buffer *gotui.Buffer, model ViewModel) image.Rectangle {
	if buffer == nil {
		return image.Rectangle{}
	}
	requested := image.Rect(0, 0, model.Width, model.Height)
	if requested.Empty() {
		return buffer.Rectangle
	}
	return requested.Intersect(buffer.Rectangle)
}

func centeredRectangle(bounds image.Rectangle, width, height int) image.Rectangle {
	width = min(max(width, 0), bounds.Dx())
	height = min(max(height, 0), bounds.Dy())
	x := bounds.Min.X + (bounds.Dx()-width)/2
	y := bounds.Min.Y + (bounds.Dy()-height)/2
	return image.Rect(x, y, x+width, y+height)
}
