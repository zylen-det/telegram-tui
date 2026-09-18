package frontend

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestFullSizeRootDimensions(t *testing.T) {
	for _, tc := range []struct {
		width, height int
	}{
		{60, 18},
		{100, 24},
		{140, 30},
	} {
		model := ui.ViewModel{Width: tc.width, Height: tc.height}
		frame := composeApplication(model, time.Local)
		if frame.Compositor == nil {
			t.Fatalf("%dx%d: compositor is nil", tc.width, tc.height)
		}
		bounds := frame.Compositor.Bounds()
		if got := bounds.Dx(); got != tc.width {
			t.Errorf("%dx%d: compositor width = %d, want %d", tc.width, tc.height, got, tc.width)
		}
		if got := bounds.Dy(); got != tc.height {
			t.Errorf("%dx%d: compositor height = %d, want %d", tc.width, tc.height, got, tc.height)
		}
		lines := strings.Split(frame.Content, "\n")
		if got := len(lines); got != tc.height {
			t.Errorf("%dx%d: rendered %d lines, want %d", tc.width, tc.height, got, tc.height)
		}
		for y, line := range lines {
			if got := ansi.StringWidth(line); got > tc.width {
				t.Errorf("%dx%d: line %d width = %d, want <= %d", tc.width, tc.height, y, got, tc.width)
			}
		}
	}
}

func TestNestedXYAbsoluteHitBounds(t *testing.T) {
	// Root at (0,0) sized 60x18. A nested child at local (10,5) sized 8x3 with
	// a grandchild at local (2,1) sized 4x2.
	rootContent := lipgloss.NewStyle().Width(60).Height(18).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	childContent := lipgloss.NewStyle().Width(8).Height(3).Render("")
	child := lipgloss.NewLayer(childContent).ID("child").X(10).Y(5).Z(zPane)

	grandContent := lipgloss.NewStyle().Width(4).Height(2).Render("")
	grand := lipgloss.NewLayer(grandContent).ID("grand").X(2).Y(1).Z(zContent)

	child.AddLayers(grand)
	root.AddLayers(child)

	compositor := lipgloss.NewCompositor(root)

	// Child absolute bounds: (10,5)-(18,8).
	if hit := compositor.Hit(11, 5); hit.ID() != "child" {
		t.Errorf("Hit(11,5) = %q, want child", hit.ID())
	}
	if hit := compositor.Hit(10, 5); hit.ID() != "child" {
		t.Errorf("Hit(10,5) = %q, want child", hit.ID())
	}
	// Grandchild absolute bounds: parent(10,5)+local(2,1) = (12,6)-(16,8).
	if hit := compositor.Hit(13, 7); hit.ID() != "grand" {
		t.Errorf("Hit(13,7) = %q, want grand", hit.ID())
	}
	if got, want := compositor.Hit(13, 7).Bounds(), image.Rect(12, 6, 16, 8); !got.Eq(want) {
		t.Errorf("grand bounds = %v, want %v", got, want)
	}
	// Outside child: no hit.
	if hit := compositor.Hit(20, 6); hit.ID() != "" {
		t.Errorf("Hit(20,6) = %q, want empty", hit.ID())
	}
}

func TestGlobalZBehaviorChildNotAddedToParent(t *testing.T) {
	// Pinned v2.0.6: a child's Z is not added to its parent's Z. An empty-content
	// parent at Z=100 with a child at Z=1 overlapping a sibling at Z=50 must be
	// won by the sibling, proving the parent Z was not added to the child Z.
	// (The contract forbids relying on equal-Z ordering because v2.0.6 uses a
	// non-stable sort, so no equal-Z winner is asserted.)
	rootContent := lipgloss.NewStyle().Width(20).Height(10).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	parent := lipgloss.NewLayer("").ID("parent").X(0).Y(0).Z(100)
	child := lipgloss.NewLayer(lipgloss.NewStyle().Width(10).Height(10).Render("")).ID("child").X(0).Y(0).Z(1)
	parent.AddLayers(child)
	sibling := lipgloss.NewLayer(lipgloss.NewStyle().Width(10).Height(10).Render("")).ID("sibling").X(0).Y(0).Z(50)
	root.AddLayers(parent, sibling)

	compositor := lipgloss.NewCompositor(root)
	// The sibling at global Z=50 must win over the child at global Z=1 even
	// though the child's parent is at Z=100. If parent Z were added to child Z
	// the child would be at 101 and win.
	if hit := compositor.Hit(5, 5); hit.ID() != "sibling" {
		t.Errorf("overlap hit = %q, want sibling (parent Z not added to child Z)", hit.ID())
	}

	// A child with a higher global tier wins over a parent with a lower tier,
	// regardless of nesting.
	parent2 := lipgloss.NewLayer(lipgloss.NewStyle().Width(10).Height(10).Render("")).ID("parent").X(0).Y(0).Z(zPane)
	highChild := lipgloss.NewLayer(lipgloss.NewStyle().Width(5).Height(5).Render("")).ID("high").X(0).Y(0).Z(zModalFrame)
	parent2.AddLayers(highChild)
	root2 := lipgloss.NewLayer(lipgloss.NewStyle().Width(20).Height(10).Render("")).X(0).Y(0).Z(zFrame)
	root2.AddLayers(parent2)
	compositor2 := lipgloss.NewCompositor(root2)
	if hit := compositor2.Hit(2, 2); hit.ID() != "high" {
		t.Errorf("child global Z should win over parent: hit = %q, want high", hit.ID())
	}
}

func TestRenderLineDoesNotWrapOverlong(t *testing.T) {
	style := lipgloss.NewStyle()
	cases := []struct {
		name  string
		text  string
		width int
	}{
		{"ascii", "abcdefghij", 5},
		{"heart", "❤️", 1},
		{"family", "👨‍👩‍👧‍👦", 1},
	}
	for _, tc := range cases {
		rendered := renderLine(style, tc.text, tc.width)
		plain := ansi.Strip(rendered)
		lines := strings.Split(plain, "\n")
		if len(lines) != 1 {
			t.Errorf("%s: renderLine produced %d lines, want 1 (no wrap)", tc.name, len(lines))
		}
		if got := ansi.StringWidth(plain); got != tc.width {
			t.Errorf("%s: renderLine width = %d, want %d", tc.name, got, tc.width)
		}
	}
}

func TestGraphemeWidthsAreTwo(t *testing.T) {
	for _, emoji := range []string{"❤️", "👨‍👩‍👧‍👦", "👍", "🔥"} {
		if got := ansi.StringWidth(emoji); got != 2 {
			t.Errorf("StringWidth(%q) = %d, want 2", emoji, got)
		}
		if got := displayWidth(emoji); got != 2 {
			t.Errorf("displayWidth(%q) = %d, want 2", emoji, got)
		}
	}
}

func TestOversizedGraphemeBecomesEllipsisWrapText(t *testing.T) {
	// A single grapheme wider than the line becomes "…" so the line always fits.
	lines := wrapText("❤️", 1)
	if len(lines) != 1 {
		t.Fatalf("wrapText(❤️,1) = %d lines, want 1", len(lines))
	}
	if got := lines[0]; got != "…" {
		t.Errorf("wrapText(❤️,1) = %q, want …", got)
	}
	if got := ansi.StringWidth(lines[0]); got != 1 {
		t.Errorf("oversized-grapheme line width = %d, want 1", got)
	}
}

func TestInteractionsCompileInVisualZOrder(t *testing.T) {
	// Two interactions at the same point with different global Z tiers. The
	// higher-Z action must win in the compiled HitMap.
	base := layerInteraction{
		ID:    "base",
		Rect:  image.Rect(0, 0, 10, 10),
		Z:     zPane,
		Click: app.ActionReceived{Action: app.SelectChat, ChatID: 1},
	}
	top := layerInteraction{
		ID:    "top",
		Rect:  image.Rect(0, 0, 10, 10),
		Z:     zModalContent,
		Click: app.ActionReceived{Action: app.Close},
	}
	hits := compileHits([]layerInteraction{base, top})
	action, ok := hits.ActionAt(5, 5)
	if !ok {
		t.Fatal("no action found at (5,5)")
	}
	if action.Action != app.Close {
		t.Errorf("topmost action = %v, want Close", action.Action)
	}
}

func TestUniqueIDsAndHitParity(t *testing.T) {
	// Build a small compositor with two non-virtual interactions and verify
	// each interaction's ID and absolute bounds match Compositor.Hit at a
	// representative point.
	rootContent := lipgloss.NewStyle().Width(40).Height(20).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	origin := image.Point{0, 0}
	interactionA := addInteractive(root, origin, image.Rect(2, 3, 12, 6), "chat:5", zRowBackground, "", app.ActionReceived{Action: app.SelectChat, ChatID: 5}, app.ActionReceived{}, app.ActionReceived{})
	interactionB := addInteractive(root, origin, image.Rect(20, 4, 30, 7), "composer", zControl, "", app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusComposer}, app.ActionReceived{}, app.ActionReceived{})

	if err := assertUniqueNonEmptyIDs([]layerInteraction{interactionA, interactionB}); err != nil {
		t.Fatalf("unexpected duplicate IDs: %v", err)
	}

	compositor := lipgloss.NewCompositor(root)
	check := func(interaction layerInteraction, point image.Point) {
		hit := compositor.Hit(point.X, point.Y)
		if hit.ID() != interaction.ID {
			t.Errorf("Hit(%v) = %q, want %q", point, hit.ID(), interaction.ID)
		}
		if got := hit.Bounds(); !got.Eq(interaction.Rect) {
			t.Errorf("%s bounds = %v, want %v", interaction.ID, got, interaction.Rect)
		}
	}
	check(interactionA, image.Pt(5, 4))
	check(interactionB, image.Pt(25, 5))
}

func TestNestedAddInteractiveAbsoluteBounds(t *testing.T) {
	// A nested interactive child: parent at (10,5), child local (2,1) sized
	// 4x2. The interaction's absolute rect must be (12,6)-(16,8).
	rootContent := lipgloss.NewStyle().Width(40).Height(20).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	parent := lipgloss.NewLayer(lipgloss.NewStyle().Width(20).Height(10).Render("")).X(10).Y(5).Z(zPane)
	root.AddLayers(parent)

	interaction := addInteractive(parent, image.Pt(10, 5), image.Rect(2, 1, 6, 3), "nested", zContent, "", app.ActionReceived{Action: app.SelectMessage}, app.ActionReceived{}, app.ActionReceived{})
	want := image.Rect(12, 6, 16, 8)
	if !interaction.Rect.Eq(want) {
		t.Errorf("nested interaction rect = %v, want %v", interaction.Rect, want)
	}
	compositor := lipgloss.NewCompositor(root)
	if hit := compositor.Hit(13, 7); hit.ID() != "nested" {
		t.Errorf("Hit(13,7) = %q, want nested", hit.ID())
	}
	if got := compositor.Hit(13, 7).Bounds(); !got.Eq(want) {
		t.Errorf("nested hit bounds = %v, want %v", got, want)
	}
}

func TestTrailingCells(t *testing.T) {
	const family = "👨‍👩‍👧‍👦"
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{name: "width zero", s: "abc", width: 0, want: ""},
		{name: "negative width", s: "abc", width: -3, want: ""},
		{name: "already fits", s: "abc", width: 5, want: "abc"},
		{name: "exact fit", s: "abc", width: 3, want: "abc"},
		{name: "truncates", s: "abcdef", width: 3, want: "def"},
		{name: "trailing family zwj", s: "discarded" + family, width: 2, want: family},
		{name: "heart too narrow", s: "a❤️", width: 1, want: ""},
		{name: "heart fits", s: "a❤️", width: 2, want: "❤️"},
		{name: "family too narrow", s: "a" + family, width: 1, want: ""},
		{name: "cjk too narrow", s: "a界", width: 1, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := trailingCells(tc.s, tc.width)
			if got != tc.want {
				t.Errorf("trailingCells(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
			}
			if w := ansi.StringWidth(got); w > max(tc.width, 0) {
				t.Errorf("trailingCells(%q, %d) result width = %d, want <= %d", tc.s, tc.width, w, max(tc.width, 0))
			}
		})
	}
}

func TestWrapTextFlushesBeforeOversizedCluster(t *testing.T) {
	// The flush of a pending line must happen before an oversized cluster is
	// handled, so "a界" at width 1 yields two one-cell lines, not a two-cell
	// "a…".
	lines := wrapText("a界", 1)
	if got, want := strings.Join(lines, "|"), "a|…"; got != want {
		t.Errorf("wrapText(a界,1) = %q, want %q", got, want)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 1 {
			t.Errorf("wrapText(a界,1) line %d width = %d, want <= 1", i, got)
		}
	}
}

func TestAddInteractiveExactContentAccepted(t *testing.T) {
	rootContent := lipgloss.NewStyle().Width(40).Height(20).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	// Non-empty content whose Lipgloss dimensions exactly match the local
	// rectangle is accepted and its styles preserved.
	content := lipgloss.NewStyle().Width(8).Height(3).Foreground(color.RGBA{1, 2, 3, 255}).Render("hi")
	interaction := addInteractive(root, image.Point{0, 0}, image.Rect(2, 3, 10, 6), "exact", zContent, content, app.ActionReceived{Action: app.SelectMessage}, app.ActionReceived{}, app.ActionReceived{})

	if got, want := interaction.Rect, image.Rect(2, 3, 10, 6); !got.Eq(want) {
		t.Errorf("interaction rect = %v, want %v", got, want)
	}
	compositor := lipgloss.NewCompositor(root)
	if hit := compositor.Hit(5, 4); hit.ID() != "exact" {
		t.Errorf("Hit(5,4) = %q, want exact", hit.ID())
	}
	// The content's style must be preserved verbatim.
	if !strings.Contains(content, "\x1b[38;2;1;2;3m") {
		t.Errorf("content style was not preserved: %q", content)
	}
}

func TestAddInteractiveMismatchedContentPanics(t *testing.T) {
	rootContent := lipgloss.NewStyle().Width(40).Height(20).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	// Content sized 8x3 but a local rectangle of 10x3 must panic as a
	// programmer error rather than silently restyle/pad the ANSI content.
	content := lipgloss.NewStyle().Width(8).Height(3).Render("hi")
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("addInteractive with mismatched content did not panic")
		}
		message, ok := r.(string)
		if !ok {
			t.Fatalf("panic value = %#v, want string", r)
		}
		if !strings.Contains(message, "addInteractive") {
			t.Errorf("panic message %q should mention addInteractive", message)
		}
	}()
	addInteractive(root, image.Point{0, 0}, image.Rect(2, 3, 12, 6), "mismatch", zContent, content, app.ActionReceived{}, app.ActionReceived{}, app.ActionReceived{})
}

func TestAddInteractiveEmptyRectangleSafe(t *testing.T) {
	rootContent := lipgloss.NewStyle().Width(40).Height(20).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	// An empty local rectangle returns a safe empty interaction without adding
	// a layer to the parent.
	interaction := addInteractive(root, image.Point{0, 0}, image.Rect(5, 5, 5, 5), "empty", zContent, "", app.ActionReceived{Action: app.Close}, app.ActionReceived{}, app.ActionReceived{})
	if interaction.ID != "empty" {
		t.Errorf("empty-rect interaction ID = %q, want empty", interaction.ID)
	}
	if !interaction.Rect.Empty() {
		t.Errorf("empty-rect interaction rect = %v, want empty", interaction.Rect)
	}
	if got := root.GetLayer("empty"); got != nil {
		t.Errorf("empty local rectangle added a layer, got %v", got)
	}
}

func TestDuplicateIDDetection(t *testing.T) {
	// A fixture with duplicate non-empty IDs must be detected by the helper.
	duplicates := []layerInteraction{
		{ID: "a", Rect: image.Rect(0, 0, 2, 2)},
		{ID: "b", Rect: image.Rect(0, 0, 2, 2)},
		{ID: "a", Rect: image.Rect(0, 0, 2, 2)},
	}
	if err := assertUniqueNonEmptyIDs(duplicates); err == nil {
		t.Fatal("assertUniqueNonEmptyIDs did not detect duplicate IDs")
	}
	// Empty IDs are ignored, so a fixture with only empty IDs is valid.
	if err := assertUniqueNonEmptyIDs([]layerInteraction{
		{ID: "", Rect: image.Rect(0, 0, 2, 2)},
		{ID: "", Rect: image.Rect(0, 0, 2, 2)},
	}); err != nil {
		t.Fatalf("assertUniqueNonEmptyIDs rejected empty-only IDs: %v", err)
	}
}

// assertUniqueNonEmptyIDs checks that all non-empty interaction IDs are unique
// and returns an error on any duplicate. Empty IDs (non-visual text layers)
// are ignored.
func assertUniqueNonEmptyIDs(interactions []layerInteraction) error {
	seen := map[string]bool{}
	for _, interaction := range interactions {
		if interaction.ID == "" {
			continue
		}
		if seen[interaction.ID] {
			return fmt.Errorf("duplicate interaction ID %q", interaction.ID)
		}
		seen[interaction.ID] = true
	}
	return nil
}

func colorOf(c color.Color) color.RGBA {
	if c == nil {
		return color.RGBA{}
	}
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}
