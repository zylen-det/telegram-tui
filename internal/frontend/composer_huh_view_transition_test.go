package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComposerLayerRendersInjectedHuhViewWithoutLegacyDraftOrCursor(t *testing.T) {
	model := ViewModel{
		Width: 100, Height: 24, Focus: FocusComposer,
		ActiveChat: domain.Chat{ID: 9, CanSend: true},
		Draft:      "LEGACY_DRAFT_MUST_NOT_RENDER",
	}
	rect := image.Rect(30, 18, 100, 24)
	styles := newRenderStyles(false)
	injected := "HUH_VISIBLE_MARKER\nSECOND\nTHIRD\nFOURTH\nFIFTH\nSIXTH\nSEVENTH\nEIGHTH\nNINTH\nTENTH"
	surface := buildComposerLayer(model, rect, styles, injected)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render(""))
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	rendered := lipgloss.NewCompositor(root).Render()
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "HUH_VISIBLE_MARKER") {
		t.Fatalf("injected Huh view is absent\n%s", plain)
	}
	if strings.Contains(plain, model.Draft) {
		t.Fatalf("legacy ViewModel draft was rendered after cut-over\n%s", plain)
	}
	if surface.Cursor.Visible {
		t.Fatalf("composer still declares a terminal cursor: %+v", surface.Cursor)
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != model.Height {
		t.Fatalf("injected Huh view escaped viewport: lines=%d, want %d", len(lines), model.Height)
	}
	for row, line := range lines {
		if got := ansi.StringWidth(line); got > model.Width {
			t.Fatalf("row %d width=%d exceeds %d", row, got, model.Width)
		}
	}
}

func TestClipComposerTextViewStopsAtExactHeight(t *testing.T) {
	got := clipComposerTextView("row1\nrow2\nrow3", 20, 2)
	if want := "row1\nrow2"; got != want {
		t.Fatalf("height-clipped view = %q, want %q", got, want)
	}
}

func TestComposerLayerWithoutInjectedViewDoesNotFallbackToDraft(t *testing.T) {
	model := ViewModel{
		Width: 100, Height: 24, Focus: FocusComposer,
		ActiveChat: domain.Chat{ID: 9, CanSend: true},
		Draft:      "LEGACY_FALLBACK_MUST_NOT_RENDER",
	}
	styles := newRenderStyles(false)
	surface := buildComposerLayer(model, image.Rect(30, 18, 100, 24), styles)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render(""))
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	plain := ansi.Strip(lipgloss.NewCompositor(root).Render())
	if strings.Contains(plain, model.Draft) {
		t.Fatalf("composer fell back to ViewModel draft without injected Huh View\n%s", plain)
	}
}
