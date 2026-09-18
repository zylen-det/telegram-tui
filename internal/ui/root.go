package ui

import (
	"image"
	"sync"

	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
)

type Root struct {
	gotui.Block
	mu    sync.RWMutex
	model ViewModel
	hits  HitMap
}

func NewRoot() *Root {
	return &Root{Block: *gotui.NewBlock()}
}

func (r *Root) Update(model ViewModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.model = model
	r.SetRect(0, 0, model.Width, model.Height)
}

func (r *Root) Hits() HitMap {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append(HitMap(nil), r.hits...)
}

func (r *Root) Model() ViewModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.model
}

func (r *Root) Draw(buffer *gotui.Buffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if buffer == nil || buffer.Empty() {
		r.hits = r.hits[:0]
		return
	}

	buffer.Fill(gotui.NewCell(' ', baseStyle), buffer.Rectangle)
	r.hits = r.hits[:0]
	if r.model.Layout.Mode == app.LayoutTooSmall {
		drawTooSmall(buffer, r.model)
		return
	}

	drawStatus(buffer, r.model)
	drawChats(buffer, r.model, &r.hits)
	drawConversation(buffer, r.model, &r.hits)
	drawDetails(buffer, r.model, &r.hits)
	drawToast(buffer, r.model)
	drawPrompt(buffer, r.model)
	drawModal(buffer, r.model, &r.hits)
	r.hits = boundedHits(r.hits, buffer.Rectangle)
}

func (r *Root) HandleEvent(gotui.Event) bool { return false }

func boundedHits(hits HitMap, bounds image.Rectangle) HitMap {
	bounded := hits[:0]
	for _, hit := range hits {
		hit.Rect = hit.Rect.Intersect(bounds)
		if !hit.Rect.Empty() {
			bounded = append(bounded, hit)
		}
	}
	return bounded
}
