package frontend

import (
	"image"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func renderedListModal(t *testing.T, surface surfaceResult) string {
	t.Helper()
	_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
	return ansi.Strip(canvas.Render())
}

func TestSelectorHuhViewInjectionReplacesActionRowsPreservesChromeStatusAndHits(t *testing.T) {
	menu := &MessageActionMenu{
		ChatID: 9, MessageID: 2, Loading: true,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
	}
	model := ViewModel{Width: 80, Height: 24, MessageMenu: menu}
	styles := newRenderStyles(false)
	legacy := buildActionModalLayer(model, styles)
	injected := buildActionModalLayer(model, styles, "HUH_ACTION_ONE\nHUH_ACTION_TWO\nOVERFLOW_FORBIDDEN")
	_, injectedCanvas := listModalCanvas(image.Rect(0, 0, 80, 24), injected)
	rendered := ansi.Strip(injectedCanvas.Render())

	for _, required := range []string{"Message actions", "HUH_ACTION_ONE", "HUH_ACTION_TWO", "Loading actions..."} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("injected action surface missing %q\n%s", required, rendered)
		}
	}
	for _, forbidden := range []string{"Reply", "Copy", "OVERFLOW_FORBIDDEN"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("injected action surface retained/clipped incorrectly %q\n%s", forbidden, rendered)
		}
	}
	if !legacy.Rect.Eq(injected.Rect) || !reflect.DeepEqual(legacy.Interactions, injected.Interactions) {
		t.Fatalf("injection changed geometry/hits: legacy=%v/%#v injected=%v/%#v", legacy.Rect, legacy.Interactions, injected.Rect, injected.Interactions)
	}
	for _, interaction := range injected.Interactions {
		if interaction.ID != "action:reply" {
			continue
		}
		startCell := injectedCanvas.CellAt(interaction.Rect.Min.X, interaction.Rect.Min.Y)
		if startCell == nil || startCell.Content != "H" {
			t.Fatalf("injected Huh view starts at wrong cell: CellAt(%v)=%#v", interaction.Rect.Min, startCell)
		}
		cell := injectedCanvas.CellAt(interaction.Rect.Max.X-1, interaction.Rect.Min.Y)
		if cell != nil && colorOf(cell.Style.Bg) == rgba(selectedColor) {
			t.Fatal("injected Huh selector retained manual selected-row background")
		}
	}
}

func TestSelectorHuhViewInjectionReplacesReactionAndForwardRows(t *testing.T) {
	styles := newRenderStyles(false)
	reaction := ViewModel{
		Width: 80, Height: 24,
		ReactionPicker: &ReactionPicker{ChatID: 9, MessageID: 2, Selected: 1},
	}
	reactionRendered := renderedListModal(t, buildReactionPickerLayer(reaction, styles, "HUH_REACTION_ONLY"))
	if !strings.Contains(reactionRendered, "HUH_REACTION_ONLY") {
		t.Fatalf("reaction Huh view missing\n%s", reactionRendered)
	}
	for _, emoji := range ReactionPalette {
		if strings.Contains(reactionRendered, emoji) {
			t.Fatalf("reaction legacy row %q survived injection\n%s", emoji, reactionRendered)
		}
	}

	forward := ViewModel{
		Width: 80, Height: 24,
		ForwardPicker: &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0},
		Chats: []ChatRow{
			{Chat: domain.Chat{ID: 11, Title: "LEGACY_ALPHA_FORBIDDEN"}},
			{Chat: domain.Chat{ID: 22, Title: "LEGACY_BETA_FORBIDDEN"}},
		},
	}
	forwardRendered := renderedListModal(t, buildForwardPickerLayer(forward, styles, "HUH_FORWARD_ONE\nHUH_FORWARD_TWO"))
	for _, required := range []string{"Forward to", "HUH_FORWARD_ONE", "HUH_FORWARD_TWO"} {
		if !strings.Contains(forwardRendered, required) {
			t.Fatalf("forward Huh surface missing %q\n%s", required, forwardRendered)
		}
	}
	for _, forbidden := range []string{"LEGACY_ALPHA_FORBIDDEN", "LEGACY_BETA_FORBIDDEN"} {
		if strings.Contains(forwardRendered, forbidden) {
			t.Fatalf("forward legacy row %q survived injection\n%s", forbidden, forwardRendered)
		}
	}
}

func TestSelectorEmptyInjectedHuhViewNeverFallsBackButOmittedRetainsCompatibility(t *testing.T) {
	menu := &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
	}
	model := ViewModel{Width: 80, Height: 24, MessageMenu: menu}
	styles := newRenderStyles(false)

	emptyInjected := renderedListModal(t, buildActionModalLayer(model, styles, ""))
	if strings.Contains(emptyInjected, "Reply") || strings.Contains(emptyInjected, "Copy") {
		t.Fatalf("empty injected Huh view fell back to manual rows\n%s", emptyInjected)
	}
	omitted := renderedListModal(t, buildActionModalLayer(model, styles))
	if !strings.Contains(omitted, "Reply") || !strings.Contains(omitted, "Copy") {
		t.Fatalf("omitted injection lost standalone compatibility\n%s", omitted)
	}
}

func TestSelectorCompositionBundlePresenceDistinguishesEmptyFromOmitted(t *testing.T) {
	model := frameBaseModel(80, 24)
	model.MessageMenu = &MessageActionMenu{
		ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
	}
	omitted := ansi.Strip(composeApplication(model, time.Local).Content)
	if !strings.Contains(omitted, "Reply") || !strings.Contains(omitted, "Copy") {
		t.Fatalf("omitted production bundle lost direct compatibility\n%s", omitted)
	}
	presentEmpty := ansi.Strip(composeApplication(model, time.Local, editorViews{Selector: ""}).Content)
	if strings.Contains(presentEmpty, "Reply") || strings.Contains(presentEmpty, "Copy") {
		t.Fatalf("present empty selector bundle fell back to manual rows\n%s", presentEmpty)
	}
}

func TestAppModelSelectorHuhViewFlowsThroughProductionComposition(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusModal
	state.MessageMenu = &MessageActionMenu{
		RequestID: 7, ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncListModalController()
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "Reply") || !strings.Contains(plain, "Copy") {
		t.Fatalf("AppModel production Selector Huh View is absent\n%s", plain)
	}
}

func TestAppModelSelectorViewDoesNotSynchronizeHost(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusModal
	state.MessageMenu = &MessageActionMenu{
		RequestID: 7, ChatID: 9, MessageID: 2,
		Capabilities: domain.MessageCapabilities{Copy: true},
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncListModalController()
	wantIdentity := model.listModals.host.Identity()

	next := state
	next.MessageMenu = &MessageActionMenu{
		RequestID: 8, ChatID: 9, MessageID: 3,
		Capabilities: domain.MessageCapabilities{Reply: true},
	}
	model.state = &next
	_ = model.View()
	if got := model.listModals.host.Identity(); got != wantIdentity {
		t.Fatalf("View synchronized/mutated selector host: got=%#v want=%#v", got, wantIdentity)
	}
}

func TestSelectorCompositionRoutesInjectionToReactionAndForwardBuilders(t *testing.T) {
	reaction := frameBaseModel(80, 24)
	reaction.Focus = FocusReactionPicker
	reaction.ReactionPicker = &ReactionPicker{ChatID: 9, MessageID: 2, Selected: 0}
	reactionPlain := ansi.Strip(composeApplication(reaction, time.Local, editorViews{Selector: "ROUTED_REACTION"}).Content)
	if !strings.Contains(reactionPlain, "ROUTED_REACTION") {
		t.Fatalf("composeApplication did not route injection to Reaction builder\n%s", reactionPlain)
	}

	forward := frameBaseModel(80, 24)
	forward.Focus = FocusForwardPicker
	forward.ForwardPicker = &ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0}
	forward.Chats = []ChatRow{{Chat: domain.Chat{ID: 11, Title: "LEGACY_FORWARD_FORBIDDEN"}}}
	forwardPlain := ansi.Strip(composeApplication(forward, time.Local, editorViews{Selector: "ROUTED_FORWARD"}).Content)
	if !strings.Contains(forwardPlain, "ROUTED_FORWARD") || strings.Contains(forwardPlain, "LEGACY_FORWARD_FORBIDDEN") {
		t.Fatalf("composeApplication did not route injection to Forward builder\n%s", forwardPlain)
	}
}

func TestClipSelectorHuhViewUsesExactANSIWideRuneRectangle(t *testing.T) {
	view := "\x1b[31m1234567890界AB\x1b[0m\nsecond界row\nthird-forbidden"
	got := clipSelectorHuhView(view, 10, 2)
	rows := strings.Split(got, "\n")
	if len(rows) != 2 {
		t.Fatalf("clipped rows = %d, want 2: %q", len(rows), got)
	}
	for index, row := range rows {
		if width := ansi.StringWidth(row); width > 10 {
			t.Fatalf("row %d width = %d, want <=10: %q", index, width, row)
		}
	}
	if strings.Contains(got, "third-forbidden") {
		t.Fatalf("height clip retained third row: %q", got)
	}
	if got := clipSelectorHuhView("non-empty", 0, 2); got != "" {
		t.Fatalf("zero-width clip = %q, want empty", got)
	}
}

func TestSelectorHuhViewTypedProductionCompositionSeam(t *testing.T) {
	checks := []struct {
		path     string
		required []string
	}{
		{"view.go", []string{"editorViews{", "Selector: m.listModals.View()"}},
		{"render_frame.go", []string{"Selector string", "selectedViews := selectEditorViews(views)", "ctx.hasViews = len(views) > 0", "stack.compose(ctx, modalBase{"}},
		// The selector-owning overlay branches now live in the registry; each
		// still forwards the injected Huh Selector View to its builder.
		{"modal_stack.go", []string{"if !ctx.hasViews", "buildActionModalLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...)", "buildReactionPickerLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...)", "buildForwardPickerLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...)"}},
		{"surface_list_modal.go", []string{"selectorView ...string", "clipSelectorHuhView", "len(selectorView) > 0"}},
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
	viewSource, err := os.ReadFile("view.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(viewSource), "func (m AppModel) View() tea.View")
	end := strings.Index(string(viewSource), "func (m AppModel) publishOverlayDesired")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate AppModel.View source")
	}
	if strings.Contains(string(viewSource)[start:end], "syncListModalController()") {
		t.Fatal("AppModel.View mutates/synchronizes selector host instead of remaining one-snapshot pure composition")
	}
}
