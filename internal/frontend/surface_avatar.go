package frontend

import (
	"hash/fnv"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
)

// buildAvatarLayer builds an avatar surface as a single exact multiline root
// at rect.Min with Z z. Real avatars render nearest-neighbor upper-half-block
// cells; placeholders render the deterministic palette with centered initials.
// An empty rect returns nil.
func buildAvatarLayer(rect image.Rectangle, avatar pixel.Avatar, seed, name string, styles renderStyles, z int) *lipgloss.Layer {
	if rect.Empty() {
		return nil
	}

	if avatar.Width > 0 && avatar.Height > 0 && avatar.Width <= len(avatar.Cells)/avatar.Height {
		return buildRealAvatarLayer(rect, avatar, styles, z)
	}
	return buildPlaceholderAvatarLayer(rect, seed, name, styles, z)
}

// buildRealAvatarLayer renders the avatar's nearest-neighbor sampled cells as
// a single multiline string of upper-half blocks. Each target cell uses the
// sampled source cell's foreground/background and Faint(styles.Dim).
func buildRealAvatarLayer(rect image.Rectangle, avatar pixel.Avatar, styles renderStyles, z int) *lipgloss.Layer {
	rows := make([]string, 0, rect.Dy())
	for localY := 0; localY < rect.Dy(); localY++ {
		sourceY := localY * avatar.Height / rect.Dy()
		var row strings.Builder
		for localX := 0; localX < rect.Dx(); localX++ {
			sourceX := localX * avatar.Width / rect.Dx()
			cell := avatar.Cells[sourceY*avatar.Width+sourceX]
			row.WriteString(lipgloss.NewStyle().
				Foreground(rgba(rgb{cell.Foreground.R, cell.Foreground.G, cell.Foreground.B})).
				Background(rgba(rgb{cell.Background.R, cell.Background.G, cell.Background.B})).
				Faint(styles.Dim).
				Render("▀"))
		}
		rows = append(rows, row.String())
	}
	content := strings.Join(rows, "\n")
	return lipgloss.NewLayer(content).X(rect.Min.X).Y(rect.Min.Y).Z(z)
}

// buildPlaceholderAvatarLayer renders a deterministic palette placeholder with
// grapheme-safe centered initials using the established palette and FNV seed.
func buildPlaceholderAvatarLayer(rect image.Rectangle, seed, name string, styles renderStyles, z int) *lipgloss.Layer {
	palette := [][2]rgb{
		{{42, 122, 142}, {22, 63, 75}},
		{{159, 83, 103}, {76, 38, 49}},
		{{87, 137, 79}, {39, 66, 36}},
		{{154, 113, 54}, {72, 51, 23}},
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(seed + "\x00" + name))
	colors := palette[int(hash.Sum32())%len(palette)]

	rows := make([]string, 0, rect.Dy())
	for y := 0; y < rect.Dy(); y++ {
		var row strings.Builder
		for x := 0; x < rect.Dx(); x++ {
			row.WriteString(lipgloss.NewStyle().
				Foreground(rgba(colors[0])).
				Background(rgba(colors[1])).
				Faint(styles.Dim).
				Render("░"))
		}
		rows = append(rows, row.String())
	}
	content := strings.Join(rows, "\n")

	root := lipgloss.NewLayer(content).X(rect.Min.X).Y(rect.Min.Y).Z(z)

	initials := avatarSurfaceInitials(name)
	clipped := ansi.Truncate(initials, rect.Dx(), "")
	initialStyle := lipgloss.NewStyle().
		Foreground(rgba(textColor)).
		Background(rgba(colors[1])).
		Bold(true).
		Faint(styles.Dim)
	initialContent := initialStyle.Render(clipped)

	initialX := centeredX(rect.Dx(), displayWidth(clipped))
	initialY := rect.Dy() / 2
	root.AddLayers(lipgloss.NewLayer(initialContent).X(initialX).Y(initialY).Z(z + 1))
	return root
}
