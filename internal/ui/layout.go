package ui

import (
	"image"

	"github.com/zylen-det/telegram-tui/internal/app"
)

type Layout struct {
	Mode         app.Layout
	Status       image.Rectangle
	Chats        image.Rectangle
	Conversation image.Rectangle
	Details      image.Rectangle
}

func ComputeLayout(width, height int, details bool, focus app.Focus) Layout {
	if width <= 0 || height <= 0 {
		return Layout{Mode: app.LayoutTooSmall}
	}

	layout := Layout{
		Mode:   layoutMode(width, height),
		Status: image.Rect(0, 0, width, 1),
	}
	body := image.Rect(0, 1, width, height)

	switch layout.Mode {
	case app.LayoutWide:
		layout.Chats = image.Rect(0, 1, 32, height)
		if details {
			layout.Conversation = image.Rect(32, 1, width-32, height)
			layout.Details = image.Rect(width-32, 1, width, height)
		} else {
			layout.Conversation = image.Rect(32, 1, width, height)
		}
	case app.LayoutNormal:
		if details {
			layout.Details = body
		} else {
			layout.Chats = image.Rect(0, 1, 30, height)
			layout.Conversation = image.Rect(30, 1, width, height)
		}
	case app.LayoutNarrow:
		if details {
			layout.Details = body
		} else if focus == app.FocusChats {
			layout.Chats = body
		} else {
			layout.Conversation = body
		}
	}

	return layout
}

func layoutMode(width, height int) app.Layout {
	if width < 60 || height < 18 {
		return app.LayoutTooSmall
	}
	if width >= 120 && height >= 24 {
		return app.LayoutWide
	}
	if width >= 80 && height >= 20 {
		return app.LayoutNormal
	}
	return app.LayoutNarrow
}

type Hit struct {
	Rect      image.Rectangle
	Click     app.ActionReceived
	WheelUp   app.ActionReceived
	WheelDown app.ActionReceived
}

type HitMap []Hit

func (hits HitMap) ActionAt(x, y int) (app.ActionReceived, bool) {
	return hits.actionAt(x, y, func(hit Hit) app.ActionReceived { return hit.Click })
}

func (hits HitMap) WheelAt(x, y int, up bool) (app.ActionReceived, bool) {
	if up {
		return hits.actionAt(x, y, func(hit Hit) app.ActionReceived { return hit.WheelUp })
	}
	return hits.actionAt(x, y, func(hit Hit) app.ActionReceived { return hit.WheelDown })
}

func (hits HitMap) actionAt(x, y int, action func(Hit) app.ActionReceived) (app.ActionReceived, bool) {
	point := image.Pt(x, y)
	for i := len(hits) - 1; i >= 0; i-- {
		if !point.In(hits[i].Rect) {
			continue
		}
		mapped := action(hits[i])
		if mapped.Action != app.NoAction {
			return mapped, true
		}
	}
	return app.ActionReceived{}, false
}
