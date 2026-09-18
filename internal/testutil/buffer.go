package testutil

import (
	"image"
	"strings"

	gotui "github.com/metaspartan/gotui/v5"
)

// BufferText serializes visible runes while preserving the buffer's origin.
func BufferText(buffer *gotui.Buffer) string {
	if buffer == nil || buffer.Empty() {
		return ""
	}

	var output strings.Builder
	for y := buffer.Min.Y; y < buffer.Max.Y; y++ {
		line := make([]rune, 0, buffer.Dx())
		for x := buffer.Min.X; x < buffer.Max.X; x++ {
			r := buffer.GetCell(image.Pt(x, y)).Rune
			if r == 0 {
				r = ' '
			}
			line = append(line, r)
		}
		output.WriteString(strings.TrimRight(string(line), " "))
		output.WriteByte('\n')
	}
	return output.String()
}
