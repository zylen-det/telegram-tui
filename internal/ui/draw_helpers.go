package ui

import (
	"hash/fnv"
	"image"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/mattn/go-runewidth"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/rivo/uniseg"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"golang.org/x/text/unicode/norm"
)

func drawRoundedBlock(buffer *gotui.Buffer, rectangle image.Rectangle, title string, focused bool) image.Rectangle {
	if buffer == nil {
		return image.Rectangle{}
	}
	rectangle = rectangle.Intersect(buffer.Rectangle)
	if rectangle.Dx() < 2 || rectangle.Dy() < 2 {
		return image.Rectangle{}
	}

	block := gotui.NewBlock()
	block.BorderRounded = true
	block.BackgroundColor = panelColor
	block.BorderStyle = gotui.NewStyle(borderColor, backgroundColor)
	if focused {
		block.BorderStyle.Fg = focusedBorderColor
	}
	block.SetRect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y)
	block.Draw(buffer)
	if title != "" && rectangle.Dx() > 4 {
		drawClipped(buffer, image.Pt(rectangle.Min.X+2, rectangle.Min.Y), rectangle.Dx()-4, title, gotui.NewStyle(textColor, backgroundColor, tcell.AttrBold))
	}
	return block.Inner.Intersect(buffer.Rectangle)
}

func drawClipped(buffer *gotui.Buffer, point image.Point, width int, text string, style gotui.Style) {
	if buffer == nil || width <= 0 || !point.In(buffer.Rectangle) {
		return
	}
	width = min(width, buffer.Max.X-point.X)
	if width <= 0 {
		return
	}

	x := 0
	text = norm.NFC.String(text)
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		cluster := graphemes.Str()
		if strings.ContainsRune(cluster, '\n') {
			return
		}
		clusterWidth := runewidth.StringWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}
		if x >= width {
			return
		}
		if clusterWidth > width {
			buffer.SetCell(gotui.NewCell('…', style), point.Add(image.Pt(x, 0)))
			return
		}
		if x+clusterWidth > width {
			return
		}

		buffer.SetCell(gotui.NewCell(representativeRune(cluster), style), point.Add(image.Pt(x, 0)))
		for continuation := 1; continuation < clusterWidth; continuation++ {
			buffer.SetCell(gotui.NewCell(' ', style), point.Add(image.Pt(x+continuation, 0)))
		}
		x += clusterWidth
	}
}

func wrapCells(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	if text == "" {
		return []string{""}
	}

	lines := make([]string, 0, 1)
	var line strings.Builder
	lineWidth := 0
	flush := func() {
		lines = append(lines, line.String())
		line.Reset()
		lineWidth = 0
	}

	text = norm.NFC.String(text)
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		cluster := graphemes.Str()
		if cluster == "\n" || cluster == "\r\n" {
			flush()
			continue
		}
		clusterWidth := runewidth.StringWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}
		if clusterWidth > width {
			cluster = "…"
			clusterWidth = 1
		}
		if lineWidth > 0 && lineWidth+clusterWidth > width {
			flush()
		}
		line.WriteString(cluster)
		lineWidth += clusterWidth
	}
	flush()
	return lines
}

func drawPixelAvatar(buffer *gotui.Buffer, point image.Point, avatar pixel.Avatar) {
	if buffer == nil || avatar.Width <= 0 || avatar.Height <= 0 || avatar.Width > len(avatar.Cells)/avatar.Height {
		return
	}
	for y := 0; y < avatar.Height; y++ {
		for x := 0; x < avatar.Width; x++ {
			cell := avatar.Cells[y*avatar.Width+x]
			style := gotui.NewStyle(
				gotui.NewRGBColor(int32(cell.Foreground.R), int32(cell.Foreground.G), int32(cell.Foreground.B)),
				gotui.NewRGBColor(int32(cell.Background.R), int32(cell.Background.G), int32(cell.Background.B)),
			)
			buffer.SetCell(gotui.NewCell('▀', style), point.Add(image.Pt(x, y)))
		}
	}
}

func dimRectangle(buffer *gotui.Buffer, rectangle image.Rectangle) {
	if buffer == nil {
		return
	}
	rectangle = rectangle.Intersect(buffer.Rectangle)
	for y := rectangle.Min.Y; y < rectangle.Max.Y; y++ {
		for x := rectangle.Min.X; x < rectangle.Max.X; x++ {
			point := image.Pt(x, y)
			cell := buffer.GetCell(point)
			cell.Style.Modifier |= tcell.AttrDim
			buffer.SetCell(cell, point)
		}
	}
}

func drawAvatar(buffer *gotui.Buffer, rectangle image.Rectangle, avatar pixel.Avatar, seed, name string) {
	if buffer == nil {
		return
	}
	rectangle = rectangle.Intersect(buffer.Rectangle)
	if rectangle.Empty() {
		return
	}
	if validAvatar(avatar) {
		drawScaledAvatar(buffer, rectangle, avatar)
		return
	}
	drawAvatarPlaceholder(buffer, rectangle, seed, name)
}

func drawScaledAvatar(buffer *gotui.Buffer, rectangle image.Rectangle, avatar pixel.Avatar) {
	for y := rectangle.Min.Y; y < rectangle.Max.Y; y++ {
		sourceY := (y - rectangle.Min.Y) * avatar.Height / rectangle.Dy()
		for x := rectangle.Min.X; x < rectangle.Max.X; x++ {
			sourceX := (x - rectangle.Min.X) * avatar.Width / rectangle.Dx()
			cell := avatar.Cells[sourceY*avatar.Width+sourceX]
			style := gotui.NewStyle(
				gotui.NewRGBColor(int32(cell.Foreground.R), int32(cell.Foreground.G), int32(cell.Foreground.B)),
				gotui.NewRGBColor(int32(cell.Background.R), int32(cell.Background.G), int32(cell.Background.B)),
			)
			buffer.SetCell(gotui.NewCell('▀', style), image.Pt(x, y))
		}
	}
}

func validAvatar(avatar pixel.Avatar) bool {
	return avatar.Width > 0 && avatar.Height > 0 && avatar.Width <= len(avatar.Cells)/avatar.Height
}

func drawAvatarPlaceholder(buffer *gotui.Buffer, rectangle image.Rectangle, seed, name string) {
	palette := [][2]gotui.Color{
		{gotui.NewRGBColor(42, 122, 142), gotui.NewRGBColor(22, 63, 75)},
		{gotui.NewRGBColor(159, 83, 103), gotui.NewRGBColor(76, 38, 49)},
		{gotui.NewRGBColor(87, 137, 79), gotui.NewRGBColor(39, 66, 36)},
		{gotui.NewRGBColor(154, 113, 54), gotui.NewRGBColor(72, 51, 23)},
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(seed + "\x00" + name))
	colors := palette[int(hash.Sum32())%len(palette)]
	style := gotui.NewStyle(colors[0], colors[1])
	buffer.Fill(gotui.NewCell('░', style), rectangle)
	initials := avatarInitials(name)
	initialWidth := runewidth.StringWidth(initials)
	x := rectangle.Min.X + max(0, (rectangle.Dx()-initialWidth)/2)
	y := rectangle.Min.Y + rectangle.Dy()/2
	drawClipped(buffer, image.Pt(x, y), rectangle.Max.X-x, initials, gotui.NewStyle(textColor, colors[1], tcell.AttrBold))
}

func avatarInitials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	initials := make([]rune, 0, 2)
	for _, field := range fields {
		for _, r := range field {
			initials = append(initials, unicode.ToUpper(r))
			break
		}
		if len(initials) == 2 {
			break
		}
	}
	return string(initials)
}

func representativeRune(cluster string) rune {
	// gotui stores one rune per cell. NFC preserves precomposed clusters; ZWJ
	// sequences still use their representative rune until gotui supports strings.
	for _, r := range cluster {
		if runewidth.RuneWidth(r) > 0 {
			return r
		}
	}
	return ' '
}

func insetRectangle(rectangle image.Rectangle, amount int) image.Rectangle {
	if amount <= 0 {
		return rectangle
	}
	rectangle = rectangle.Inset(amount)
	if rectangle.Empty() {
		return image.Rectangle{}
	}
	return rectangle
}
