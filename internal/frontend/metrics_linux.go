//go:build linux

package frontend

import (
	"image"
	"os"

	"golang.org/x/sys/unix"
)

var ioctlGetWinsize = unix.IoctlGetWinsize

func LinuxCellPixelsForFD(fd uintptr) func(columns, rows int) image.Point {
	return func(columns, rows int) image.Point {
		winsize, err := ioctlGetWinsize(int(fd), unix.TIOCGWINSZ)
		if err != nil {
			return image.Pt(1, 2)
		}
		return cellPixelsFromWinsize(winsize, columns, rows)
	}
}

func LinuxCellPixels(columns, rows int) image.Point {
	return LinuxCellPixelsForFD(os.Stdout.Fd())(columns, rows)
}

func cellPixelsFromWinsize(winsize *unix.Winsize, columns, rows int) image.Point {
	if winsize == nil || columns <= 0 || rows <= 0 || winsize.Xpixel == 0 || winsize.Ypixel == 0 {
		return image.Pt(1, 2)
	}
	width := int(winsize.Xpixel) / columns
	height := int(winsize.Ypixel) / rows
	if width < 1 || height < 1 {
		return image.Pt(1, 2)
	}
	return image.Pt(width, height)
}

func fitImageRect(bounds image.Rectangle, imageSize, cellPixels image.Point) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	if imageSize.X < 1 {
		imageSize.X = 1
	}
	if imageSize.Y < 1 {
		imageSize.Y = 1
	}
	if cellPixels.X < 1 {
		cellPixels.X = 1
	}
	if cellPixels.Y < 1 {
		cellPixels.Y = 2
	}

	width, height := bounds.Dx(), bounds.Dy()
	if imageSize.X*height*cellPixels.Y <= imageSize.Y*width*cellPixels.X {
		width = max(1, imageSize.X*height*cellPixels.Y/(imageSize.Y*cellPixels.X))
	} else {
		height = max(1, imageSize.Y*width*cellPixels.X/(imageSize.X*cellPixels.Y))
	}
	width = min(width, bounds.Dx())
	height = min(height, bounds.Dy())
	minimum := image.Pt(bounds.Min.X+(bounds.Dx()-width)/2, bounds.Min.Y+(bounds.Dy()-height)/2)
	return image.Rectangle{Min: minimum, Max: minimum.Add(image.Pt(width, height))}
}
