package frontend

import (
	"hash/fnv"
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
)

// avatarCanvas composes a compositor rooted at (0,0) containing the avatar
// layer onto a canvas, so nested initials children are drawn.
func avatarCanvas(size image.Point, layer *lipgloss.Layer) *lipgloss.Canvas {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(size.X).Height(size.Y).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(size.X, size.Y).Compose(compositor)
}

func TestAvatarRealNearestNeighborParity(t *testing.T) {
	// Port of TestAvatarParityScalesSixByThreeToTwelveBySix without editing it:
	// a 6x3 avatar scaled to a 12x6 target rect, every cell is ▀ with exact
	// nearest-neighbor foreground/background.
	avatar := patternedAvatar(t, 6, 3)
	styles := newRenderStyles(false)
	rect := image.Rect(1, 1, 13, 7)
	layer := buildAvatarLayer(rect, avatar, "avatar-key", "Mina Chen", styles, zContent)
	if layer == nil {
		t.Fatal("avatar layer is nil")
	}
	canvas := avatarCanvas(image.Pt(14, 8), layer)

	for y := 0; y < 6; y++ {
		for x := 0; x < 12; x++ {
			source := (y*3/6)*6 + x*6/12
			cell := canvas.CellAt(rect.Min.X+x, rect.Min.Y+y)
			if cell == nil {
				t.Fatalf("CellAt(%d,%d) is nil", rect.Min.X+x, rect.Min.Y+y)
			}
			if cell.Content != "▀" {
				t.Fatalf("avatar cell (%d,%d) content = %q, want upper half block", x, y, cell.Content)
			}
			if got := colorOf(cell.Style.Fg); got != rgba(rgb{uint8(source + 1), 0, 0}) {
				t.Fatalf("avatar cell (%d,%d) foreground = %v, want source index %d", x, y, got, source)
			}
			if got := colorOf(cell.Style.Bg); got != rgba(rgb{0, 0, uint8(source + 61)}) {
				t.Fatalf("avatar cell (%d,%d) background = %v, want source index %d", x, y, got, source)
			}
		}
	}
}

func TestAvatarPlaceholderParity(t *testing.T) {
	const seed = "avatar-key"
	const name = "mina chen"
	styles := newRenderStyles(false)
	rect := image.Rect(1, 1, 7, 4)
	layer := buildAvatarLayer(rect, pixel.Avatar{}, seed, name, styles, zContent)
	if layer == nil {
		t.Fatal("avatar layer is nil")
	}
	canvas := avatarCanvas(image.Pt(8, 5), layer)

	palette := [][2]rgb{
		{{42, 122, 142}, {22, 63, 75}},
		{{159, 83, 103}, {76, 38, 49}},
		{{87, 137, 79}, {39, 66, 36}},
		{{154, 113, 54}, {72, 51, 23}},
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(seed + "\x00" + name))
	colors := palette[int(hash.Sum32())%len(palette)]

	initialX := rect.Min.X + (rect.Dx()-2)/2
	initialY := rect.Min.Y + rect.Dy()/2
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				t.Fatalf("CellAt(%d,%d) is nil", x, y)
			}
			if got := colorOf(cell.Style.Bg); got != rgba(colors[1]) {
				t.Fatalf("placeholder cell (%d,%d) background = %v, want %v", x, y, got, rgba(colors[1]))
			}
			if y == initialY && (x == initialX || x == initialX+1) {
				want := []string{"M", "C"}[x-initialX]
				if cell.Content != want {
					t.Fatalf("initial cell (%d,%d) = %q, want %q", x, y, cell.Content, want)
				}
				if got := colorOf(cell.Style.Fg); got != rgba(textColor) {
					t.Fatalf("initial cell (%d,%d) foreground = %v, want textColor", x, y, got)
				}
				if cell.Style.Attrs&uv.AttrBold == 0 {
					t.Fatalf("initial cell (%d,%d) is not bold", x, y)
				}
				continue
			}
			if cell.Content != "░" {
				t.Fatalf("placeholder cell (%d,%d) = %q, want shaded cell", x, y, cell.Content)
			}
			if got := colorOf(cell.Style.Fg); got != rgba(colors[0]) {
				t.Fatalf("placeholder cell (%d,%d) foreground = %v, want %v", x, y, got, rgba(colors[0]))
			}
		}
	}
}

func TestAvatarDimSetsFaint(t *testing.T) {
	// Real cells.
	avatar := patternedAvatar(t, 2, 1)
	dim := newRenderStyles(true)
	realLayer := buildAvatarLayer(image.Rect(0, 0, 4, 2), avatar, "k", "n", dim, zContent)
	realCanvas := avatarCanvas(image.Pt(4, 2), realLayer)
	if cell := realCanvas.CellAt(0, 0); cell.Style.Attrs&uv.AttrFaint == 0 {
		t.Error("real avatar cell should be Faint when dim")
	}

	// Placeholder cells and initials.
	placeLayer := buildAvatarLayer(image.Rect(0, 0, 6, 3), pixel.Avatar{}, "k", "n", dim, zContent)
	placeCanvas := avatarCanvas(image.Pt(6, 3), placeLayer)
	if cell := placeCanvas.CellAt(0, 0); cell.Style.Attrs&uv.AttrFaint == 0 {
		t.Error("placeholder cell should be Faint when dim")
	}
	initialY := 3 / 2
	initialX := (6 - 2) / 2
	if cell := placeCanvas.CellAt(initialX, initialY); cell.Style.Attrs&uv.AttrFaint == 0 {
		t.Error("placeholder initial should be Faint when dim")
	}

	// Non-dim styles do not set Faint.
	plain := newRenderStyles(false)
	plainLayer := buildAvatarLayer(image.Rect(0, 0, 6, 3), pixel.Avatar{}, "k", "n", plain, zContent)
	plainCanvas := avatarCanvas(image.Pt(6, 3), plainLayer)
	if cell := plainCanvas.CellAt(0, 0); cell.Style.Attrs&uv.AttrFaint != 0 {
		t.Error("plain placeholder cell should not be Faint")
	}
}

func TestAvatarEmptyRectNil(t *testing.T) {
	styles := newRenderStyles(false)
	for _, rect := range []image.Rectangle{
		{Min: image.Pt(0, 0), Max: image.Pt(0, 0)},
		{Min: image.Pt(5, 5), Max: image.Pt(5, 5)},
		{Min: image.Pt(2, 2), Max: image.Pt(2, 2)},
	} {
		if layer := buildAvatarLayer(rect, pixel.Avatar{}, "k", "n", styles, zContent); layer != nil {
			t.Errorf("%v: empty rect should return nil, got %v", rect, layer)
		}
	}
}

func TestAvatarFamilyInitialsNotByteSliced(t *testing.T) {
	// A name whose first two fields are wide graphemes must not be byte-sliced:
	// the initials child must keep whole grapheme clusters.
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 8, 3)
	layer := buildAvatarLayer(rect, pixel.Avatar{}, "k", "👨‍👩‍👧‍👦 界", styles, zContent)
	canvas := avatarCanvas(image.Pt(8, 3), layer)
	initialY := 3 / 2
	// The initials "👨‍👩‍👧‍👦界" have width 2+2=4, centered in 8 -> x=2. The two
	// wide graphemes occupy cells 2-3 and 4-5; their leading cells carry the
	// bold style.
	for _, x := range []int{2, 4} {
		cell := canvas.CellAt(x, initialY)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) is nil", x, initialY)
		}
		if cell.Style.Attrs&uv.AttrBold == 0 {
			t.Fatalf("initial cell (%d,%d) is not bold", x, initialY)
		}
	}
	// The wide graphemes must be intact, not split into invalid bytes. The
	// initials child renders "👨界" (first rune of each field) whole.
	text := plainText(canvas.Render())
	if !strings.Contains(text, "👨") || !strings.Contains(text, "界") {
		t.Errorf("family initials were byte-sliced, got %q", text)
	}
}
