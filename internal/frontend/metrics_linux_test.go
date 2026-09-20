package frontend

import (
	"image"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCellPixelsFromWinsize(t *testing.T) {
	tests := []struct {
		name    string
		winsize *unix.Winsize
		columns int
		rows    int
		want    image.Point
	}{
		{name: "kitty pixels", winsize: &unix.Winsize{Xpixel: 1600, Ypixel: 960}, columns: 200, rows: 48, want: image.Pt(8, 20)},
		{name: "zero pixel fallback", winsize: &unix.Winsize{}, columns: 100, rows: 30, want: image.Pt(1, 2)},
		{name: "partial zero fallback", winsize: &unix.Winsize{Xpixel: 800}, columns: 100, rows: 30, want: image.Pt(1, 2)},
		{name: "invalid dimensions fallback", winsize: &unix.Winsize{Xpixel: 800, Ypixel: 600}, columns: 0, rows: 30, want: image.Pt(1, 2)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cellPixelsFromWinsize(test.winsize, test.columns, test.rows); got != test.want {
				t.Fatalf("cellPixelsFromWinsize() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFitImageRect(t *testing.T) {
	bounds := image.Rect(10, 5, 30, 15)
	cell := image.Pt(8, 16)
	tests := []struct {
		name string
		size image.Point
		want image.Rectangle
	}{
		{name: "landscape", size: image.Pt(800, 400), want: image.Rect(10, 7, 30, 12)},
		{name: "portrait", size: image.Pt(400, 800), want: image.Rect(15, 5, 25, 15)},
		{name: "one cell", size: image.Pt(1, 1000), want: image.Rect(19, 5, 20, 15)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fitImageRect(bounds, test.size, cell)
			if got != test.want {
				t.Fatalf("fitImageRect() = %v, want %v", got, test.want)
			}
			if got.Empty() || !got.In(bounds) {
				t.Fatalf("fitImageRect() = %v, want nonempty inside %v", got, bounds)
			}
		})
	}
}

func TestFitImageRectClampsDegenerateInputs(t *testing.T) {
	bounds := image.Rect(3, 4, 4, 5)
	if got := fitImageRect(bounds, image.Point{}, image.Point{}); got != bounds {
		t.Fatalf("fitImageRect() = %v, want %v", got, bounds)
	}
}
