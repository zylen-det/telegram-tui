package pixel

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func TestRenderExactHalfBlockColors(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	fillRow(source, 0, color.NRGBA{R: 255, A: 255})
	fillRow(source, 1, color.NRGBA{B: 255, A: 255})

	avatar, err := Render(source, 2, 2, color.NRGBA{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := Avatar{
		Width:  2,
		Height: 1,
		Cells: []Cell{
			{Rune: '▀', Foreground: color.NRGBA{R: 255, A: 255}, Background: color.NRGBA{B: 255, A: 255}},
			{Rune: '▀', Foreground: color.NRGBA{R: 255, A: 255}, Background: color.NRGBA{B: 255, A: 255}},
		},
	}
	if !reflect.DeepEqual(avatar, want) {
		t.Fatalf("Render() = %#v, want %#v", avatar, want)
	}
}

func TestRenderRejectsInvalidDimensions(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	tests := []struct {
		name        string
		pixelWidth  int
		pixelHeight int
	}{
		{name: "zero width", pixelWidth: 0, pixelHeight: 2},
		{name: "negative width", pixelWidth: -1, pixelHeight: 2},
		{name: "zero height", pixelWidth: 1, pixelHeight: 0},
		{name: "height below two", pixelWidth: 1, pixelHeight: 1},
		{name: "odd height", pixelWidth: 1, pixelHeight: 3},
		{name: "negative height", pixelWidth: 1, pixelHeight: -2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Render(source, tt.pixelWidth, tt.pixelHeight, color.NRGBA{}); err == nil {
				t.Fatal("Render() error = nil, want an error")
			}
		})
	}
}

func TestRenderRejectsOversizedDimensionsWithoutPanicking(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Render() panicked for oversized dimensions: %v", recovered)
		}
	}()

	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if _, err := Render(source, math.MaxInt, 2, color.NRGBA{}); err == nil {
		t.Fatal("Render() error = nil, want an error for oversized dimensions")
	}
}

func TestRenderRejectsNilAndEmptySources(t *testing.T) {
	var typedNil *image.NRGBA
	tests := []struct {
		name   string
		source image.Image
	}{
		{name: "nil", source: nil},
		{name: "typed nil", source: typedNil},
		{name: "empty width", source: image.NewNRGBA(image.Rect(0, 0, 0, 2))},
		{name: "empty height", source: image.NewNRGBA(image.Rect(0, 0, 2, 0))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Render(tt.source, 2, 2, color.NRGBA{}); err == nil {
				t.Fatal("Render() error = nil, want an error")
			}
		})
	}
}

func TestRenderDimensionsCellCountAndRowMajorOrder(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	colors := [][]color.NRGBA{
		{{R: 1, A: 255}, {R: 2, A: 255}, {R: 3, A: 255}, {R: 4, A: 255}},
		{{G: 1, A: 255}, {G: 2, A: 255}, {G: 3, A: 255}, {G: 4, A: 255}},
		{{B: 1, A: 255}, {B: 2, A: 255}, {B: 3, A: 255}, {B: 4, A: 255}},
		{{R: 1, G: 1, A: 255}, {R: 2, G: 2, A: 255}, {R: 3, G: 3, A: 255}, {R: 4, G: 4, A: 255}},
	}
	for y := range colors {
		for x, c := range colors[y] {
			source.SetNRGBA(x, y, c)
		}
	}

	avatar, err := Render(source, 4, 4, color.NRGBA{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if avatar.Width != 4 || avatar.Height != 2 {
		t.Fatalf("Render() dimensions = %dx%d, want 4x2", avatar.Width, avatar.Height)
	}
	if len(avatar.Cells) != avatar.Width*avatar.Height {
		t.Fatalf("len(Cells) = %d, want %d", len(avatar.Cells), avatar.Width*avatar.Height)
	}

	want := make([]Cell, 0, 8)
	for y := 0; y < 4; y += 2 {
		for x := 0; x < 4; x++ {
			want = append(want, Cell{Rune: '▀', Foreground: colors[y][x], Background: colors[y+1][x]})
		}
	}
	if !reflect.DeepEqual(avatar.Cells, want) {
		t.Fatalf("Render() cells = %#v, want row-major %#v", avatar.Cells, want)
	}
}

func TestRenderCenterCropsWideSource(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 6, 2))
	fillRect(source, image.Rect(0, 0, 2, 2), color.NRGBA{R: 255, A: 255})
	fillRect(source, image.Rect(2, 0, 4, 2), color.NRGBA{G: 255, A: 255})
	fillRect(source, image.Rect(4, 0, 6, 2), color.NRGBA{B: 255, A: 255})

	avatar, err := Render(source, 2, 2, color.NRGBA{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for i, cell := range avatar.Cells {
		green := color.NRGBA{G: 255, A: 255}
		if cell.Foreground != green || cell.Background != green {
			t.Errorf("cell %d = %#v, want center green band", i, cell)
		}
	}
}

func TestRenderCenterCropsTallSource(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 6))
	fillRect(source, image.Rect(0, 0, 2, 2), color.NRGBA{R: 255, A: 255})
	fillRect(source, image.Rect(0, 2, 2, 4), color.NRGBA{G: 255, A: 255})
	fillRect(source, image.Rect(0, 4, 2, 6), color.NRGBA{B: 255, A: 255})

	avatar, err := Render(source, 2, 2, color.NRGBA{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for i, cell := range avatar.Cells {
		green := color.NRGBA{G: 255, A: 255}
		if cell.Foreground != green || cell.Background != green {
			t.Errorf("cell %d = %#v, want center green band", i, cell)
		}
	}
}

func TestRenderHonorsNonZeroSourceBounds(t *testing.T) {
	source := image.NewNRGBA(image.Rect(10, 20, 12, 22))
	fillRow(source, 20, color.NRGBA{R: 255, A: 255})
	fillRow(source, 21, color.NRGBA{B: 255, A: 255})

	avatar, err := Render(source, 2, 2, color.NRGBA{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for i, cell := range avatar.Cells {
		if cell.Foreground != (color.NRGBA{R: 255, A: 255}) || cell.Background != (color.NRGBA{B: 255, A: 255}) {
			t.Errorf("cell %d = %#v, want red over blue", i, cell)
		}
	}
}

func TestRenderCompositesAlphaOverBackground(t *testing.T) {
	background := color.NRGBA{R: 10, G: 20, B: 30, A: 17}
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	fillRow(source, 0, color.NRGBA{R: 250, G: 1, B: 200, A: 0})
	fillRow(source, 1, color.NRGBA{R: 200, G: 100, B: 50, A: 128})

	avatar, err := Render(source, 2, 2, background)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	wantTransparent := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	wantHalf := color.NRGBA{R: 105, G: 60, B: 40, A: 255}
	for i, cell := range avatar.Cells {
		if cell.Foreground != wantTransparent || cell.Background != wantHalf {
			t.Errorf("cell %d = %#v, want foreground %#v background %#v", i, cell, wantTransparent, wantHalf)
		}
	}
}

func TestRenderDoesNotModifySource(t *testing.T) {
	source := image.NewNRGBA(image.Rect(3, 5, 7, 7))
	for i := range source.Pix {
		source.Pix[i] = byte(i*37 + 11)
	}
	wantPix := append([]byte(nil), source.Pix...)

	if _, err := Render(source, 2, 2, color.NRGBA{R: 9, G: 8, B: 7, A: 255}); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !reflect.DeepEqual(source.Pix, wantPix) {
		t.Fatal("Render() modified source pixels")
	}
}

func fillRow(img *image.NRGBA, y int, c color.NRGBA) {
	fillRect(img, image.Rect(img.Bounds().Min.X, y, img.Bounds().Max.X, y+1), c)
}

func fillRect(img *image.NRGBA, rect image.Rectangle, c color.NRGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}
