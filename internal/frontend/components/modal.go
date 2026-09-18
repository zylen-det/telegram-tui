package components

import "image"

// Item is one semantic action in a modal list.
type Item struct {
	Label  string
	Action int
}

// Modal describes renderer-neutral action-list content.
type Modal struct {
	Title    string
	Items    []Item
	Selected int
	Width    int
}

// Row is the clipped geometry of one modal item.
type Row struct {
	Rect     image.Rectangle
	Item     Item
	Index    int
	Selected bool
}

// ModalLayout is centered, clipped geometry ready for a renderer.
type ModalLayout struct {
	Frame image.Rectangle
	Close image.Point
	Rows  []Row
}

// Layout centers a compact list modal and clips all geometry to bounds.
func (modal Modal) Layout(bounds image.Rectangle) ModalLayout {
	if bounds.Empty() {
		return ModalLayout{}
	}
	preferredWidth := modal.Width
	if preferredWidth <= 0 {
		preferredWidth = 28
	}
	width := min(preferredWidth, bounds.Dx())
	height := min(max(4, len(modal.Items)+5), bounds.Dy())
	x := bounds.Min.X + (bounds.Dx()-width)/2
	y := bounds.Min.Y + (bounds.Dy()-height)/2
	frame := image.Rect(x, y, x+width, y+height).Intersect(bounds)
	layout := ModalLayout{Frame: frame, Close: image.Pt(max(frame.Min.X, frame.Max.X-2), frame.Min.Y)}
	for index, item := range modal.Items {
		row := image.Rect(frame.Min.X+2, frame.Min.Y+2+index, frame.Max.X-2, frame.Min.Y+3+index).Intersect(frame)
		if !row.Empty() {
			layout.Rows = append(layout.Rows, Row{Rect: row, Item: item, Index: index, Selected: index == modal.Selected})
		}
	}
	return layout
}
