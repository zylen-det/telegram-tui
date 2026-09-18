package frontend

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func buildStickerPickerLayer(model ui.ViewModel, styles renderStyles) surfaceResult {
	picker := model.StickerPicker
	if picker == nil || model.Width <= 0 || model.Height <= 0 {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	modalW, modalH, columns, rows := app.StickerGridGeometry(model.Width, model.Height)
	bounds := image.Rect(0, 0, model.Width, model.Height)
	frame := image.Rect((model.Width-modalW)/2, (model.Height-modalH)/2, (model.Width-modalW)/2+modalW, (model.Height-modalH)/2+modalH).Intersect(bounds)
	if frame.Empty() {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	rootContent := styles.Panel.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.FocusedBorder.GetForeground()).
		BorderBackground(styles.FocusedBorder.GetBackground()).
		Width(frame.Dx()).Height(frame.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(frame.Min.X).Y(frame.Min.Y).Z(zModalFrame)

	request := func(action app.Action) app.ActionReceived {
		return app.ActionReceived{Action: action, RequestID: picker.RequestID}
	}
	interactions := []layerInteraction{{
		ID: "sticker-picker:modal", Rect: bounds, Z: zModalFrame - 1, Virtual: true,
		WheelUp: request(app.StickerMoveUp), WheelDown: request(app.StickerMoveDown),
	}}
	if title := ansi.Truncate("Stickers", max(0, frame.Dx()-6), ""); title != "" {
		root.AddLayers(lipgloss.NewLayer(styles.Title.Render(title)).X(2).Y(0).Z(zModalContent))
	}
	closeLocal := image.Rect(frame.Dx()-3, 0, frame.Dx()-2, 1).Intersect(image.Rect(0, 0, frame.Dx(), frame.Dy()))
	if !closeLocal.Empty() {
		interactions = append(interactions, addInteractive(root, frame.Min, closeLocal, "sticker-picker:close", zModalControl,
			renderLine(styles.Accent, "×", closeLocal.Dx()), request(app.Close), app.ActionReceived{}, app.ActionReceived{}))
	}

	if picker.Loading {
		addStickerPickerStatus(root, frame, styles, "Loading stickers…")
		return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
	}
	if picker.Error != nil {
		addStickerPickerStatus(root, frame, styles, "Could not load stickers")
		return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
	}
	if len(picker.Catalog) == 0 {
		addStickerPickerStatus(root, frame, styles, "No recent or favorite stickers")
		return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, IsModal: true}
	}

	blocks := make(map[int32]thumbnail.Block, len(model.StickerThumbnails))
	for _, rendered := range model.StickerThumbnails {
		blocks[rendered.StickerFileID] = rendered.Block
	}
	first := max(0, picker.FirstRow) * columns
	last := min(len(picker.Catalog), first+rows*columns)
	var inline []inlinePlacement
	for index := first; index < last; index++ {
		visible := index - first
		row, column := visible/columns, visible%columns
		local := image.Rect(1+column*app.StickerTileWidth, 2+row*app.StickerTileHeight,
			1+(column+1)*app.StickerTileWidth, 2+(row+1)*app.StickerTileHeight).
			Intersect(image.Rect(1, 1, frame.Dx()-1, frame.Dy()-1))
		if local.Empty() {
			continue
		}
		sticker := picker.Catalog[index]
		style := styles.Panel
		if index == picker.Selected {
			style = styles.Selected
		}
		tileText := style.Width(local.Dx()).Height(local.Dy()).Render("")
		id := fmt.Sprintf("sticker:%d", sticker.File.ID)
		click := app.ActionReceived{Action: app.StickerActivate, RequestID: picker.RequestID, StickerFileID: sticker.File.ID}
		interactions = append(interactions, addInteractive(root, frame.Min, local, id, zModalRow, tileText,
			click, request(app.StickerMoveUp), request(app.StickerMoveDown)))

		block := blocks[sticker.File.ID]
		if block.Width > 0 && block.Height > 0 {
			x := local.Min.X + max(0, (local.Dx()-block.Width)/2)
			y := local.Min.Y + max(0, (local.Dy()-block.Height)/2)
			if block.Kitty {
				inline = append(inline, inlinePlacement{ImageID: block.ImageID, X: frame.Min.X + x, Y: frame.Min.Y + y, Width: block.Width, Height: block.Height, Text: block.Text})
			} else {
				root.AddLayers(lipgloss.NewLayer(block.Text).X(x).Y(y).Z(zModalContent))
			}
			continue
		}
		fallback := sticker.Emoji
		if fallback == "" {
			fallback = "[Sticker]"
		}
		fallback = ansi.Truncate(fallback, local.Dx(), "")
		root.AddLayers(lipgloss.NewLayer(style.Render(fallback)).
			X(local.Min.X + max(0, (local.Dx()-displayWidth(fallback))/2)).Y(local.Min.Y + local.Dy()/2).Z(zModalContent))
	}
	return surfaceResult{Layer: root, Rect: frame, Interactions: interactions, Cursor: renderCursor{X: -1, Y: -1}, Inline: inline, IsModal: true}
}

func addStickerPickerStatus(root *lipgloss.Layer, frame image.Rectangle, styles renderStyles, text string) {
	text = ansi.Truncate(text, max(0, frame.Dx()-4), "")
	root.AddLayers(lipgloss.NewLayer(styles.Muted.Render(text)).
		X(max(1, (frame.Dx()-displayWidth(text))/2)).Y(max(2, frame.Dy()/2)).Z(zModalContent))
}
