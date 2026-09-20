package frontend

import (
	"image"
)

type ViewLayout struct {
	Mode         Layout
	Status       image.Rectangle
	Chats        image.Rectangle
	Conversation image.Rectangle
	Details      image.Rectangle
}

func ComputeLayout(width, height int, details bool, focus Focus) ViewLayout {
	if width <= 0 || height <= 0 {
		return ViewLayout{Mode: LayoutTooSmall}
	}

	layout := ViewLayout{
		Mode:   layoutMode(width, height),
		Status: image.Rect(0, 0, width, 1),
	}
	body := image.Rect(0, 1, width, height)

	switch layout.Mode {
	case LayoutWide:
		layout.Chats = image.Rect(0, 1, 32, height)
		if details {
			layout.Conversation = image.Rect(32, 1, width-32, height)
			layout.Details = image.Rect(width-32, 1, width, height)
		} else {
			layout.Conversation = image.Rect(32, 1, width, height)
		}
	case LayoutNormal:
		if details {
			layout.Details = body
		} else {
			layout.Chats = image.Rect(0, 1, 30, height)
			layout.Conversation = image.Rect(30, 1, width, height)
		}
	case LayoutNarrow:
		if details {
			layout.Details = body
		} else if focus == FocusChats {
			layout.Chats = body
		} else {
			layout.Conversation = body
		}
	}

	return layout
}

func layoutMode(width, height int) Layout {
	if width < 60 || height < 18 {
		return LayoutTooSmall
	}
	if width >= 120 && height >= 24 {
		return LayoutWide
	}
	if width >= 80 && height >= 20 {
		return LayoutNormal
	}
	return LayoutNarrow
}

type Hit struct {
	Rect      image.Rectangle
	Click     ActionReceived
	WheelUp   ActionReceived
	WheelDown ActionReceived
}

type HitMap []Hit

func (hits HitMap) ActionAt(x, y int) (ActionReceived, bool) {
	return hits.actionAt(x, y, func(hit Hit) ActionReceived { return hit.Click })
}

func (hits HitMap) WheelAt(x, y int, up bool) (ActionReceived, bool) {
	if up {
		return hits.actionAt(x, y, func(hit Hit) ActionReceived { return hit.WheelUp })
	}
	return hits.actionAt(x, y, func(hit Hit) ActionReceived { return hit.WheelDown })
}

func (hits HitMap) actionAt(x, y int, action func(Hit) ActionReceived) (ActionReceived, bool) {
	point := image.Pt(x, y)
	for i := len(hits) - 1; i >= 0; i-- {
		if !point.In(hits[i].Rect) {
			continue
		}
		mapped := action(hits[i])
		if mapped.Action != NoAction {
			return mapped, true
		}
	}
	return ActionReceived{}, false
}
