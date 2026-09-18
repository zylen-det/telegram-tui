package components

import (
	"image"

	"github.com/charmbracelet/x/ansi"
)

// Toast describes renderer-neutral floating notification content.
type Toast struct {
	Text string
}

// ToastLayout places a small rounded panel and its text content.
type ToastLayout struct {
	Frame   image.Rectangle
	Content image.Rectangle
}

// Layout anchors the toast one cell from the bottom-right and clips it safely.
func (toast Toast) Layout(bounds image.Rectangle) ToastLayout {
	if bounds.Empty() {
		return ToastLayout{}
	}
	width := min(bounds.Dx(), max(3, ansi.StringWidth(toast.Text)+4))
	height := min(bounds.Dy(), 3)
	frame := image.Rect(bounds.Max.X-width-1, bounds.Max.Y-height-1, bounds.Max.X-1, bounds.Max.Y-1).Intersect(bounds)
	if frame.Empty() {
		frame = bounds
	}
	content := image.Rect(frame.Min.X+1, frame.Min.Y+1, frame.Max.X-1, frame.Max.Y-1).Intersect(frame)
	if content.Empty() {
		content = frame
	}
	return ToastLayout{Frame: frame, Content: content}
}
