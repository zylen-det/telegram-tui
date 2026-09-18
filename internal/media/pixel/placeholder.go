package pixel

import (
	"crypto/sha256"
	"image/color"
	"unicode"
)

func Placeholder(label string, pixelWidth, pixelHeight int, background color.NRGBA) Avatar {
	if pixelWidth < 1 || pixelHeight < 2 || pixelHeight%2 != 0 {
		return Avatar{}
	}
	digest := sha256.Sum256([]byte(label))
	fill := color.NRGBA{
		R: 64 + digest[0]%128,
		G: 64 + digest[1]%128,
		B: 64 + digest[2]%128,
		A: 255,
	}
	contrast := color.NRGBA{A: 255}
	brightness := int(fill.R)*299 + int(fill.G)*587 + int(fill.B)*114
	if brightness < 128000 {
		contrast = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	if background.A == 0 {
		background.A = 255
	}

	avatar := Avatar{
		Width:  pixelWidth,
		Height: pixelHeight / 2,
		Cells:  make([]Cell, pixelWidth*(pixelHeight/2)),
	}
	for index := range avatar.Cells {
		avatar.Cells[index] = Cell{Rune: ' ', Foreground: background, Background: fill}
	}
	initial := firstLetter(label)
	if initial != 0 {
		center := (avatar.Height/2)*avatar.Width + avatar.Width/2
		avatar.Cells[center] = Cell{Rune: initial, Foreground: contrast, Background: fill}
	}
	return avatar
}

func firstLetter(label string) rune {
	for _, value := range label {
		if unicode.IsLetter(value) {
			return value
		}
	}
	return 0
}
