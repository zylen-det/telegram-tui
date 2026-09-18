package ui

import (
	"image"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/mattn/go-runewidth"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type conversationLine struct {
	text     string
	style    gotui.Style
	outgoing bool
}

type conversationGroup struct {
	lines      []conversationLine
	showAvatar bool
	avatarKey  string
	avatarName string
	avatar     RenderedMessageGroup
}

func drawConversation(buffer *gotui.Buffer, model ViewModel, hits *HitMap) {
	rectangle := model.Layout.Conversation.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	title := model.ActiveChat.Title
	if title == "" {
		title = "Conversation"
	}
	inner := drawRoundedBlock(buffer, rectangle, title, model.Focus == app.FocusConversation || model.Focus == app.FocusComposer)
	appendHit(hits, Hit{
		Rect:  rectangle,
		Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusConversation},
	})
	active := model.ActiveChat.ID != 0
	if active && !model.DetailsOpen && rectangle.Dx() >= 3 {
		infoRect := image.Rect(rectangle.Max.X-2, rectangle.Min.Y, rectangle.Max.X-1, rectangle.Min.Y+1).Intersect(rectangle)
		drawClipped(buffer, infoRect.Min, infoRect.Dx(), "ⓘ", accentStyle)
		appendHit(hits, Hit{Rect: infoRect, Click: app.ActionReceived{Action: app.ToggleDetails}})
	}
	if inner.Empty() {
		return
	}

	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	composer := image.Rect(inner.Min.X, composerTop, inner.Max.X, inner.Max.Y)
	history := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, composerTop)
	if model.Toast != nil && history.Dy() > 0 {
		history.Max.Y--
	}
	if !history.Empty() {
		if active {
			appendHit(hits, Hit{
				Rect:      history,
				Click:     app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusConversation},
				WheelUp:   app.ActionReceived{Action: app.PageUp},
				WheelDown: app.ActionReceived{Action: app.PageDown},
			})
			drawHistory(buffer, history, model, hits)
		} else {
			y := history.Min.Y + history.Dy()/2
			drawCenteredText(buffer, image.Rect(history.Min.X, y, history.Max.X, y+1), "No conversation", mutedStyle)
		}
	}
	drawComposer(buffer, composer, model, hits)
}

func drawHistory(buffer *gotui.Buffer, history image.Rectangle, model ViewModel, hits *HitMap) {
	if model.HistoryError != nil && history.Dy() > 0 {
		errorRect := image.Rect(history.Min.X, history.Min.Y, history.Max.X, history.Min.Y+1)
		drawClipped(buffer, errorRect.Min, errorRect.Dx(), model.HistoryError.Message+"  Retry", gotui.NewStyle(errorColor, panelColor, tcell.AttrBold))
		appendHit(hits, Hit{Rect: errorRect, Click: app.ActionReceived{Action: app.Retry}})
		history.Min.Y++
	}
	groups := groupsBeforeOffset(model.Groups, model.HistoryOffset)
	blocks := make([]conversationGroup, 0, len(groups))
	for _, group := range groups {
		blocks = append(blocks, buildConversationGroup(group, history.Dx()))
	}

	cursor := history.Max.Y
	for index := len(blocks) - 1; index >= 0; index-- {
		block := blocks[index]
		if index < len(blocks)-1 {
			cursor--
		}
		top := cursor - len(block.lines)
		drawConversationGroup(buffer, history, top, block, hits)
		cursor = top
		if cursor <= history.Min.Y {
			break
		}
	}
}

func buildConversationGroup(group RenderedMessageGroup, historyWidth int) conversationGroup {
	result := conversationGroup{
		showAvatar: group.ShowAvatar,
		avatarKey:  group.AvatarKey,
		avatarName: group.SenderName,
		avatar:     group,
	}
	if len(group.Messages) == 0 || historyWidth <= 0 {
		return result
	}

	contentWidth := max(1, historyWidth-2)
	if group.ShowAvatar {
		contentWidth = max(1, historyWidth-5)
		first := group.Messages[0]
		header := strings.TrimSpace(group.SenderName + "  " + first.SentAt.In(time.Local).Format("15:04"))
		result.lines = append(result.lines, conversationLine{text: header, style: emphasisStyle})
	}

	for _, message := range group.Messages {
		messageWidth := contentWidth
		if message.Outgoing {
			messageWidth = max(1, historyWidth-2)
		}
		displayText := message.DisplayText()
		switch message.SendState {
		case domain.SendPending:
			displayText += " …"
		case domain.SendFailed:
			displayText += " !"
		}
		lines := wrapCells(displayText, messageWidth)
		if len(lines) == 0 {
			lines = []string{""}
		}
		for _, line := range lines {
			style := panelStyle
			if message.Service {
				style = mutedStyle
			}
			result.lines = append(result.lines, conversationLine{text: line, style: style, outgoing: message.Outgoing})
		}
		if message.SendState == domain.SendFailed {
			result.lines[len(result.lines)-1].style = errorStyle
		}
	}
	if group.ShowAvatar && len(result.lines) < 2 {
		result.lines = append(result.lines, conversationLine{style: panelStyle})
	}
	return result
}

func drawConversationGroup(buffer *gotui.Buffer, history image.Rectangle, top int, group conversationGroup, hits *HitMap) {
	textMinX := history.Min.X + 1
	if group.showAvatar {
		textMinX = history.Min.X + 5
		avatarRect := image.Rect(history.Min.X, top, history.Min.X+4, top+2)
		if avatarRect.In(history) {
			drawAvatar(buffer, avatarRect, group.avatar.Avatar, group.avatarKey, group.avatarName)
			if group.avatar.AvatarError != nil {
				appendHit(hits, Hit{Rect: avatarRect, Click: app.ActionReceived{Action: app.Retry, AvatarKey: group.avatarKey}})
				drawClipped(buffer, image.Pt(avatarRect.Min.X, avatarRect.Max.Y-1), avatarRect.Dx(), "Retry", gotui.NewStyle(errorColor, panelColor, tcell.AttrBold))
			}
		}
	}

	for index, line := range group.lines {
		y := top + index
		if y < history.Min.Y || y >= history.Max.Y {
			continue
		}
		if line.outgoing {
			width := min(history.Dx()-2, runewidth.StringWidth(line.text))
			x := max(history.Min.X+1, history.Max.X-1-width)
			drawClipped(buffer, image.Pt(x, y), history.Max.X-1-x, line.text, line.style)
			continue
		}
		drawClipped(buffer, image.Pt(textMinX, y), max(0, history.Max.X-textMinX), line.text, line.style)
	}
}

func drawComposer(buffer *gotui.Buffer, composer image.Rectangle, model ViewModel, hits *HitMap) {
	if composer.Empty() {
		return
	}
	composerStyle := panelStyle
	if model.Focus == app.FocusComposer {
		composerStyle = gotui.NewStyle(textColor, selectedColor)
	}
	buffer.Fill(gotui.NewCell(' ', composerStyle), composer)
	if model.ActiveChat.ID == 0 {
		return
	}
	if !model.ActiveChat.CanSend {
		drawClipped(buffer, image.Pt(composer.Min.X+1, composer.Min.Y+1), max(0, composer.Dx()-2), "Read-only", mutedStyle)
		return
	}
	appendHit(hits, Hit{
		Rect:  composer,
		Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusComposer},
	})

	sendText := "[Send]"
	sendWidth := runewidth.StringWidth(sendText)
	sendX := max(composer.Min.X, composer.Max.X-sendWidth-1)
	sendY := composer.Max.Y - 1
	sendRect := image.Rect(sendX, sendY, min(composer.Max.X, sendX+sendWidth), sendY+1)
	draftWidth := max(1, sendX-composer.Min.X-1)
	draftLines := wrapCells(model.Draft, draftWidth)
	maxDraftLines := min(2, composer.Dy())
	if len(draftLines) > maxDraftLines {
		draftLines = draftLines[len(draftLines)-maxDraftLines:]
	}
	for index, line := range draftLines {
		y := composer.Min.Y + index
		if y >= composer.Max.Y {
			break
		}
		drawClipped(buffer, image.Pt(composer.Min.X+1, y), max(0, draftWidth-1), line, composerStyle)
	}
	if !sendRect.Empty() {
		drawClipped(buffer, sendRect.Min, sendRect.Dx(), sendText, accentStyle)
		appendHit(hits, Hit{Rect: sendRect, Click: app.ActionReceived{Action: app.ComposerSubmit}})
	}
}

func groupsBeforeOffset(groups []RenderedMessageGroup, offset int) []RenderedMessageGroup {
	if offset <= 0 {
		return groups
	}
	result := append([]RenderedMessageGroup(nil), groups...)
	remaining := offset
	for index := len(result) - 1; index >= 0 && remaining > 0; index-- {
		messageCount := len(result[index].Messages)
		if remaining >= messageCount {
			remaining -= messageCount
			result = result[:index]
			continue
		}
		result[index].Messages = append([]domain.Message(nil), result[index].Messages[:messageCount-remaining]...)
		remaining = 0
	}
	return result
}
