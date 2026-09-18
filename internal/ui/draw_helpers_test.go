package ui

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
)

func TestDrawClippedUsesGraphemeCellWidths(t *testing.T) {
	tests := []struct {
		name  string
		width int
		text  string
		want  string
	}{
		{name: "CJK", width: 4, text: "A界B", want: "A界 B"},
		{name: "emoji", width: 4, text: "A👩‍💻B", want: "A👩 B"},
		{name: "combining", width: 2, text: "e\u0301x", want: "éx"},
		{name: "clips", width: 3, text: "abcd", want: "abc"},
		{name: "cluster wider than area", width: 1, text: "界", want: "…"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buffer := gotui.NewBuffer(image.Rect(3, 4, 12, 6))
			drawClipped(buffer, image.Pt(4, 4), test.width, test.text, gotui.NewStyle(gotui.ColorGreen))
			got := cellsText(buffer, image.Rect(4, 4, 4+test.width, 5))
			if got != test.want {
				t.Fatalf("drawClipped() cells = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDrawClippedRejectsNonPositiveWidthAndOutsidePoint(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(0, 0, 3, 1))
	for _, test := range []struct {
		point image.Point
		width int
	}{
		{point: image.Pt(0, 0), width: 0},
		{point: image.Pt(0, 0), width: -1},
		{point: image.Pt(-4, 0), width: 3},
		{point: image.Pt(4, 0), width: 3},
	} {
		drawClipped(buffer, test.point, test.width, "secret", gotui.StyleClear)
	}
	if got := cellsText(buffer, buffer.Rectangle); got != "   " {
		t.Fatalf("buffer = %q, want untouched", got)
	}
}

func TestDrawClippedDoesNotWriteEllipsisAfterWidthIsFull(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(0, 0, 3, 1))
	drawClipped(buffer, image.Pt(0, 0), 1, "A界", gotui.StyleClear)
	if got := cellsText(buffer, buffer.Rectangle); got != "A  " {
		t.Fatalf("buffer = %q, want no write beyond width", got)
	}
}

func TestWrapCellsPreservesGraphemesAndNewlines(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{name: "CJK", text: "A界B", width: 2, want: []string{"A", "界", "B"}},
		{name: "emoji", text: "go👩‍💻now", width: 4, want: []string{"go👩‍💻", "now"}},
		{name: "combining", text: "e\u0301x", width: 1, want: []string{"é", "x"}},
		{name: "long token", text: "abcdefgh", width: 3, want: []string{"abc", "def", "gh"}},
		{name: "newlines", text: "one\ntwo\n", width: 8, want: []string{"one", "two", ""}},
		{name: "empty", text: "", width: 4, want: []string{""}},
		{name: "invalid width", text: "text", width: 0, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := wrapCells(test.text, test.width); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("wrapCells(%q, %d) = %#v, want %#v", test.text, test.width, got, test.want)
			}
		})
	}
}

func TestDrawRoundedBlockClampsAndUsesFocusedBorder(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(2, 3, 12, 9))
	inner := drawRoundedBlock(buffer, image.Rect(-10, -10, 8, 7), "界 pane", true)
	if !inner.In(buffer.Rectangle) && !inner.Empty() {
		t.Fatalf("inner = %v, want inside %v", inner, buffer.Rectangle)
	}
	if got := buffer.GetCell(image.Pt(2, 3)); got.Rune != '╭' || got.Style.Fg != focusedBorderColor {
		t.Fatalf("focused corner = %#v, want rounded focused border", got)
	}

	for _, rectangle := range []image.Rectangle{{}, image.Rect(2, 3, 2, 8), image.Rect(20, 20, 30, 30)} {
		if got := drawRoundedBlock(buffer, rectangle, "ignored", false); !got.Empty() {
			t.Fatalf("drawRoundedBlock(%v) = %v, want empty", rectangle, got)
		}
	}
}

func TestDrawPixelAvatarUsesRGBAndStaysInBounds(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(5, 6, 8, 8))
	avatar := pixel.Avatar{Width: 2, Height: 1, Cells: []pixel.Cell{
		{Rune: 'X', Foreground: color.NRGBA{R: 1, G: 2, B: 3}, Background: color.NRGBA{R: 4, G: 5, B: 6}},
		{Rune: 'Y', Foreground: color.NRGBA{R: 7, G: 8, B: 9}, Background: color.NRGBA{R: 10, G: 11, B: 12}},
	}}
	drawPixelAvatar(buffer, image.Pt(5, 6), avatar)
	first := buffer.GetCell(image.Pt(5, 6))
	if first.Rune != '▀' || first.Style.Fg != gotui.NewRGBColor(1, 2, 3) || first.Style.Bg != gotui.NewRGBColor(4, 5, 6) {
		t.Fatalf("first avatar cell = %#v", first)
	}
	second := buffer.GetCell(image.Pt(6, 6))
	if second.Rune != '▀' || second.Style.Fg != gotui.NewRGBColor(7, 8, 9) {
		t.Fatalf("second avatar cell = %#v", second)
	}

	for _, invalid := range []pixel.Avatar{
		{Width: -1, Height: 1},
		{Width: 2, Height: 2, Cells: make([]pixel.Cell, 1)},
	} {
		drawPixelAvatar(buffer, image.Pt(5, 7), invalid)
	}
}

func TestDimRectanglePreservesCellAndAddsDim(t *testing.T) {
	buffer := gotui.NewBuffer(image.Rect(0, 0, 2, 1))
	wantStyle := gotui.NewStyle(gotui.ColorRed, gotui.ColorBlue, tcell.AttrBold)
	buffer.SetCell(gotui.NewCell('Z', wantStyle), image.Pt(0, 0))
	dimRectangle(buffer, image.Rect(-5, -5, 1, 1))
	got := buffer.GetCell(image.Pt(0, 0))
	if got.Rune != 'Z' || got.Style.Fg != wantStyle.Fg || got.Style.Bg != wantStyle.Bg {
		t.Fatalf("dimmed cell changed content/style = %#v", got)
	}
	if got.Style.Modifier != tcell.AttrBold|tcell.AttrDim {
		t.Fatalf("modifier = %v, want bold|dim", got.Style.Modifier)
	}
}

func cellsText(buffer *gotui.Buffer, rectangle image.Rectangle) string {
	result := make([]rune, 0, rectangle.Dx()*rectangle.Dy())
	for y := rectangle.Min.Y; y < rectangle.Max.Y; y++ {
		for x := rectangle.Min.X; x < rectangle.Max.X; x++ {
			r := buffer.GetCell(image.Pt(x, y)).Rune
			if r == 0 {
				r = ' '
			}
			result = append(result, r)
		}
	}
	return string(result)
}
