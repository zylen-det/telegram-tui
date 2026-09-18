package frontend

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var ansiSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestAuthorizationViewPreservesPromptVisualLanguage(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	view := model.View()
	text := ansiSequence.ReplaceAllString(view.Content, "")

	for _, want := range []string{
		"Authorization",
		"Telegram API ID",
		"my.telegram.org/apps",
		"Type here…",
		"Enter: continue",
		"Backspace: delete",
		"Ctrl-C: quit",
		"╭",
		"╯",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("authorization view missing %q:\n%s", want, text)
		}
	}
	// The authorization view paints no background at all: the whole prompt keeps
	// the terminal's own background. See styles_test.go for the palette contract
	// that makes this true.
	if got := emittedBackgrounds(view.Content); len(got) != 0 {
		t.Fatalf("authorization view emitted opaque backgrounds %v, want none", got)
	}
	if !view.AltScreen {
		t.Fatal("authorization view did not request Bubble Tea alternate-screen mode")
	}
	if view.BackgroundColor != nil {
		t.Fatalf("authorization BackgroundColor = %#v, want nil", view.BackgroundColor)
	}
	if view.MouseMode != tea.MouseModeNone {
		t.Fatalf("mouse mode = %v, want MouseModeNone", view.MouseMode)
	}
	if view.Cursor == nil {
		t.Fatal("authorization view did not declare a Bubble Tea cursor")
	}
}

func TestResizeUsesTheFullWindow(t *testing.T) {
	model := NewModel()
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 20}, {Width: 120, Height: 30}} {
		model, _ = updateModel(t, model, size)
		plain := ansiSequence.ReplaceAllString(model.View().Content, "")
		lines := strings.Split(plain, "\n")
		if got := len(lines); got != size.Height {
			t.Fatalf("%dx%d view has %d lines", size.Width, size.Height, got)
		}
		for y, line := range lines {
			if got := ansi.StringWidth(line); got > size.Width {
				t.Fatalf("%dx%d view line %d has width %d, want <= %d", size.Width, size.Height, y, got, size.Width)
			}
		}
	}
}

func TestResizeShowsMinimumViewportRequirement(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 59, Height: 17})
	plain := ansiSequence.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(plain, "requires at least 60x18") {
		t.Fatalf("small viewport did not show minimum size guidance:\n%s", plain)
	}
}

func TestZWJGraphemeUsesTwoCellsAndPreservesCanvasWidth(t *testing.T) {
	const family = "👨‍👩‍👧‍👦"
	if got := ansi.StringWidth(family); got != 2 {
		t.Fatalf("family grapheme width = %d, want 2", got)
	}

	model := NewModel()
	model.input = []rune(family)
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 18})
	view := model.View()
	plain := ansiSequence.ReplaceAllString(view.Content, "")
	for y, line := range strings.Split(plain, "\n") {
		if got := ansi.StringWidth(line); got > model.width {
			t.Fatalf("ZWJ view line %d has width %d, want <= %d", y, got, model.width)
		}
	}

	if view.Cursor == nil {
		t.Fatal("ZWJ view did not declare a cursor")
	}
	inputX := (model.width-min(56, model.width-4))/2 + 2
	inputY := (model.height-min(9, model.height))/2 + 4
	if got, want := view.Cursor.X, inputX+2; got != want {
		t.Fatalf("ZWJ cursor X = %d, want visual end %d", got, want)
	}
	if got, want := view.Cursor.Y, inputY; got != want {
		t.Fatalf("ZWJ cursor Y = %d, want %d", got, want)
	}
}
