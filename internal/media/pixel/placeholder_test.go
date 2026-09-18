package pixel

import (
	"crypto/sha256"
	"image/color"
	"reflect"
	"testing"
)

func TestPlaceholderIsDeterministicAndLabelSpecific(t *testing.T) {
	background := color.NRGBA{R: 4, G: 5, B: 6, A: 255}
	first := Placeholder("Mina", 6, 6, background)
	again := Placeholder("Mina", 6, 6, background)
	other := Placeholder("Iris", 6, 6, background)

	if !reflect.DeepEqual(first, again) {
		t.Fatal("same label produced different placeholders")
	}
	if reflect.DeepEqual(first, other) {
		t.Fatal("different labels produced the same placeholder")
	}
	if first.Width != 6 || first.Height != 3 || len(first.Cells) != 18 {
		t.Fatalf("dimensions = %dx%d with %d cells, want 6x3 with 18", first.Width, first.Height, len(first.Cells))
	}
}

func TestPlaceholderUsesFirstUnicodeLetterAtCenter(t *testing.T) {
	avatar := Placeholder("  週末 Weekend", 4, 4, color.NRGBA{})
	if avatar.Width != 4 || avatar.Height != 2 || len(avatar.Cells) != 8 {
		t.Fatalf("dimensions = %dx%d with %d cells, want 4x2 with 8", avatar.Width, avatar.Height, len(avatar.Cells))
	}
	center := (avatar.Height/2)*avatar.Width + avatar.Width/2
	if got := avatar.Cells[center].Rune; got != '週' {
		t.Fatalf("center initial = %q, want %q", got, '週')
	}
	for index, cell := range avatar.Cells {
		if index != center && cell.Rune != ' ' {
			t.Errorf("cell %d rune = %q, want space", index, cell.Rune)
		}
	}
}

func TestPlaceholderUsesDigestColorAndReadableContrast(t *testing.T) {
	label := "Mina"
	digest := sha256.Sum256([]byte(label))
	wantFill := color.NRGBA{R: 64 + digest[0]%128, G: 64 + digest[1]%128, B: 64 + digest[2]%128, A: 255}
	avatar := Placeholder(label, 6, 6, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	center := (avatar.Height/2)*avatar.Width + avatar.Width/2
	for index, cell := range avatar.Cells {
		if cell.Background != wantFill {
			t.Fatalf("cell %d background = %#v, want digest fill %#v", index, cell.Background, wantFill)
		}
	}
	foreground := avatar.Cells[center].Foreground
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{A: 255}
	if foreground != white && foreground != black {
		t.Fatalf("center foreground = %#v, want black or white", foreground)
	}
}
