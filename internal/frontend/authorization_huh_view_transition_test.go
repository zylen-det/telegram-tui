package frontend

import (
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/auth"
)

func renderAuthorizationSurface(t *testing.T, data authorizationData, injected ...string) (string, surfaceResult) {
	t.Helper()
	bounds := image.Rect(0, 0, 100, 24)
	surface := buildAuthorizationLayer(bounds, data, newRenderStyles(false), injected...)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(bounds.Dx()).Height(bounds.Dy()).Render(""))
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	return lipgloss.NewCompositor(root).Render(), surface
}

func TestAuthorizationLayerRendersInjectedHuhViewWithoutLegacyInputOrCursor(t *testing.T) {
	data := authorizationData{
		Label:  "Password",
		Input:  "LEGACY_SECRET_MUST_NOT_RENDER",
		Secret: true,
	}
	injected := "HUH_AUTH_VISIBLE\nSECOND_ROW_MUST_BE_CLIPPED\nTHIRD_ROW_MUST_BE_CLIPPED"
	rendered, surface := renderAuthorizationSurface(t, data, injected)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "HUH_AUTH_VISIBLE") {
		t.Fatalf("injected Huh authorization view is absent\n%s", plain)
	}
	if strings.Contains(plain, data.Input) || strings.Contains(plain, "SECOND_ROW_MUST_BE_CLIPPED") || strings.Contains(plain, "THIRD_ROW_MUST_BE_CLIPPED") {
		t.Fatalf("legacy input or overflow row rendered after cut-over\n%s", plain)
	}
	if surface.Cursor.Visible {
		t.Fatalf("authorization surface still declares terminal cursor: %+v", surface.Cursor)
	}
	for row, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 100 {
			t.Fatalf("row %d width=%d escaped viewport", row, got)
		}
	}
}

func TestAuthorizationLayerInjectedEmptyViewDoesNotFallbackToPromptInput(t *testing.T) {
	data := authorizationData{Label: "Code", Input: "LEGACY_FALLBACK_MUST_NOT_RENDER"}
	rendered, surface := renderAuthorizationSurface(t, data, "")
	if plain := ansi.Strip(rendered); strings.Contains(plain, data.Input) {
		t.Fatalf("production injection fell back to Prompt.Input\n%s", plain)
	}
	if surface.Cursor.Visible {
		t.Fatal("empty injected Huh view restored terminal cursor")
	}
}

func TestAuthorizationLayerOmittedInjectionPreservesStandaloneCompatibility(t *testing.T) {
	data := authorizationData{Label: "API ID", Input: "12345"}
	rendered, surface := renderAuthorizationSurface(t, data)
	if plain := ansi.Strip(rendered); !strings.Contains(plain, data.Input) {
		t.Fatalf("standalone compatibility lost manual input\n%s", plain)
	}
	if !surface.Cursor.Visible {
		t.Fatal("standalone compatibility lost terminal cursor")
	}
}

func TestAuthorizationHuhViewFlowsFromAppModelThroughProductionComposition(t *testing.T) {
	checks := []struct {
		path     string
		required []string
	}{
		{"view.go", []string{"editorViews{", "Authorization: m.authorizationInput.View()"}},
		{"render_frame.go", []string{"type editorViews struct", "Authorization string", "selectedViews := selectEditorViews(views)", "ctx.hasViews = len(views) > 0", "stack.compose(ctx, modalBase{"}},
		// The concrete overlay branch now lives in the overlay registry; the
		// injected authorization Huh View still reaches the builder there.
		{"modal_stack.go", []string{"func (ctx modalContext) injectedView(view string) []string", "if !ctx.hasViews", "buildAuthorizationLayer(ctx.bounds, data, ctx.styles, ctx.injectedView(ctx.views.Authorization)...)"}},
		{"surface_authorization.go", []string{"authorizationView ...string", "clipAuthorizationInputView", "inputRect.Min.X - frameX", "inputRect.Min.Y - frameY"}},
	}
	for _, check := range checks {
		source, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		for _, required := range check.required {
			if !strings.Contains(string(source), required) {
				t.Errorf("%s missing production seam %q", check.path, required)
			}
		}
	}
}

func TestAppModelAuthorizationViewUsesPasswordHuhPrivacy(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = FocusAuth
	state.Prompt = &PromptState{
		Prompt: auth.Prompt{ID: 91, Kind: auth.PromptPassword, Label: "Password", Secret: true},
		Input:  []rune("distinct🙂secret"),
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncAuthorizationInputHost()
	view := model.View()
	plain := ansi.Strip(view.Content)
	if strings.Contains(plain, "distinct") || strings.Contains(plain, "🙂") {
		t.Fatalf("AppModel authorization View leaked password plaintext\n%s", plain)
	}
	if !strings.Contains(plain, "Password") {
		t.Fatalf("authorization prompt frame missing after Huh cut-over\n%s", plain)
	}
	inputRect := authorizationInputRect(image.Rect(0, 0, state.Width, state.Height))
	inputY := inputRect.Min.Y
	lines := strings.Split(plain, "\n")
	if inputY < 0 || inputY >= len(lines) || strings.TrimSpace(ansi.Cut(lines[inputY], inputRect.Min.X, inputRect.Max.X)) == "" {
		t.Fatalf("AppModel authorization View omitted the non-empty Huh password mask\n%s", plain)
	}

	selected := Select(model.Snapshot(), time.Local)
	frame := composeApplication(selected, time.Local, editorViews{Authorization: model.authorizationInput.View()})
	if frame.Cursor.Visible {
		t.Fatalf("production authorization frame exposes terminal cursor: %+v", frame.Cursor)
	}
}
