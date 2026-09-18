package pixel

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"

	"golang.org/x/image/draw"
)

const (
	maxPixelCount      = 16 * 1024 * 1024
	nrgbaBytesPerPixel = 4
)

type Cell struct {
	Rune       rune
	Foreground color.NRGBA
	Background color.NRGBA
}

type Avatar struct {
	Width  int
	Height int
	Cells  []Cell
}

func Render(source image.Image, pixelWidth, pixelHeight int, background color.NRGBA) (Avatar, error) {
	if pixelWidth < 1 {
		return Avatar{}, fmt.Errorf("pixel width must be at least 1: %d", pixelWidth)
	}
	if pixelHeight < 2 || pixelHeight%2 != 0 {
		return Avatar{}, fmt.Errorf("pixel height must be even and at least 2: %d", pixelHeight)
	}
	if err := validateRenderSize(pixelWidth, pixelHeight); err != nil {
		return Avatar{}, err
	}
	if isNilImage(source) {
		return Avatar{}, fmt.Errorf("source image must not be nil")
	}

	bounds := source.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return Avatar{}, fmt.Errorf("source image bounds must not be empty: %v", bounds)
	}

	side := min(bounds.Dx(), bounds.Dy())
	cropMin := image.Point{
		X: bounds.Min.X + (bounds.Dx()-side)/2,
		Y: bounds.Min.Y + (bounds.Dy()-side)/2,
	}
	crop := image.Rectangle{Min: cropMin, Max: cropMin.Add(image.Pt(side, side))}
	scaled := image.NewNRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, crop, draw.Src, nil)

	avatar := Avatar{
		Width:  pixelWidth,
		Height: pixelHeight / 2,
		Cells:  make([]Cell, 0, pixelWidth*(pixelHeight/2)),
	}
	for y := 0; y < pixelHeight; y += 2 {
		for x := 0; x < pixelWidth; x++ {
			avatar.Cells = append(avatar.Cells, Cell{
				Rune:       '▀',
				Foreground: composite(scaled.NRGBAAt(x, y), background),
				Background: composite(scaled.NRGBAAt(x, y+1), background),
			})
		}
	}

	return avatar, nil
}

func validateRenderSize(pixelWidth, pixelHeight int) error {
	if pixelWidth > math.MaxInt/pixelHeight {
		return fmt.Errorf("pixel dimensions overflow: %dx%d", pixelWidth, pixelHeight)
	}
	pixelCount := pixelWidth * pixelHeight
	if pixelCount > math.MaxInt/nrgbaBytesPerPixel {
		return fmt.Errorf("pixel buffer size overflows: %d pixels", pixelCount)
	}
	if pixelCount > maxPixelCount {
		return fmt.Errorf("pixel dimensions exceed limit: %dx%d", pixelWidth, pixelHeight)
	}
	return nil
}

func isNilImage(source image.Image) bool {
	if source == nil {
		return true
	}

	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func composite(source, background color.NRGBA) color.NRGBA {
	alpha := uint32(source.A)
	inverseAlpha := 255 - alpha
	blend := func(foreground, behind uint8) uint8 {
		return uint8((uint32(foreground)*alpha + uint32(behind)*inverseAlpha + 127) / 255)
	}

	return color.NRGBA{
		R: blend(source.R, background.R),
		G: blend(source.G, background.G),
		B: blend(source.B, background.B),
		A: 255,
	}
}
