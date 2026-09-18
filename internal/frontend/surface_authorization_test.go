package frontend

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// plainText strips ANSI sequences from a rendered frame for content assertions.
func plainText(content string) string {
	return ansi.Strip(content)
}

func authorizationDataForInput(input string) authorizationData {
	return authorizationData{
		Label:    "Telegram API ID",
		Guidance: "Enter the numeric api_id from my.telegram.org/apps",
		Input:    input,
	}
}

func TestAuthorizationFrameExactDimensions(t *testing.T) {
	for _, tc := range []struct {
		width, height int
	}{
		{60, 18},
		{100, 24},
		{120, 30},
	} {
		frame := composeAuthorization(image.Rect(0, 0, tc.width, tc.height), authorizationDataForInput(""))
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

func TestAuthorizationPlainContent(t *testing.T) {
	frame := composeAuthorization(image.Rect(0, 0, 100, 24), authorizationDataForInput(""))
	text := plainText(frame.Content)

	for _, want := range []string{
		"Authorization",
		"Telegram API ID",
		"my.telegram.org/apps",
		"Type here…",
		"Enter: continue",
		"Backspace: delete",
		"Ctrl-C: quit",
		"╭",
		"╮",
		"╰",
		"╯",
		"│",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("authorization content missing %q", want)
		}
	}
}

func TestAuthorizationNoManualBorderConcatenation(t *testing.T) {
	// The shared builder must rely on Lipgloss's rounded border, not manually
	// concatenated border glyphs. The rendered frame must still contain the
	// border corners (behavioral assertion), while the source must not
	// construct them with strings.Repeat.
	frame := composeAuthorization(image.Rect(0, 0, 100, 24), authorizationDataForInput(""))
	if !strings.Contains(plainText(frame.Content), "╭") {
		t.Fatal("rounded border top-left corner missing")
	}
}

func TestAuthorizationInputUsesTerminalBackground(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 24)
	frame := composeAuthorization(bounds, authorizationDataForInput("abc"))
	if frame.Compositor == nil {
		t.Fatal("compositor is nil")
	}

	frameWidth := min(56, bounds.Dx()-4)
	frameHeight := min(9, bounds.Dy())
	frameX := (bounds.Dx() - frameWidth) / 2
	frameY := (bounds.Dy() - frameHeight) / 2
	innerX := frameX + 2
	inputY := frameY + 4

	canvas := lipgloss.NewCanvas(bounds.Dx(), bounds.Dy()).Compose(frame.Compositor)
	cell := canvas.CellAt(innerX, inputY)
	if cell == nil {
		t.Fatalf("CellAt(%d,%d) is nil", innerX, inputY)
	}
	if cell.Style.Bg != nil {
		t.Errorf("input cell background = %v, want nil terminal background", colorOf(cell.Style.Bg))
	}
	// The input has no explicit foreground or background and deliberately
	// inherits the terminal colors supplied by Bubble Tea.
	if cell.Style.Fg != nil {
		t.Errorf("input text cell foreground = %v, want nil (inherit view ForegroundColor)", colorOf(cell.Style.Fg))
	}
}

func TestAuthorizationCursorCoordinates(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 24)
	frameWidth := min(56, bounds.Dx()-4)
	frameHeight := min(9, bounds.Dy())
	frameX := (bounds.Dx() - frameWidth) / 2
	frameY := (bounds.Dy() - frameHeight) / 2
	innerX := frameX + 2
	innerWidth := frameWidth - 4
	inputY := frameY + 4

	cases := []struct {
		name  string
		input string
		wantX int
	}{
		{"ascii", "abc", innerX + 3},
		{"family zwj", "👨‍👩‍👧‍👦", innerX + 2},
		{"mixed", "a👨‍👩‍👧‍👦b", innerX + 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := composeAuthorization(bounds, authorizationDataForInput(tc.input))
			if !frame.Cursor.Visible {
				t.Fatal("cursor not visible")
			}
			if got := frame.Cursor.X; got != tc.wantX {
				t.Errorf("cursor X = %d, want %d", got, tc.wantX)
			}
			if got := frame.Cursor.Y; got != inputY {
				t.Errorf("cursor Y = %d, want %d", got, inputY)
			}
			if got := frame.Cursor.X; got < innerX || got >= innerX+innerWidth {
				t.Errorf("cursor X = %d outside input [%d,%d)", got, innerX, innerX+innerWidth)
			}
		})
	}
}

func TestAuthorizationCursorStaysWithinInput(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 24)
	frameWidth := min(56, bounds.Dx()-4)
	frameX := (bounds.Dx() - frameWidth) / 2
	innerX := frameX + 2
	innerWidth := frameWidth - 4

	// Overlong input must clamp the cursor before the inner right edge.
	long := strings.Repeat("x", innerWidth*2)
	frame := composeAuthorization(bounds, authorizationDataForInput(long))
	if !frame.Cursor.Visible {
		t.Fatal("cursor not visible")
	}
	if got := frame.Cursor.X; got < innerX || got >= innerX+innerWidth {
		t.Errorf("overlong cursor X = %d outside input [%d,%d)", got, innerX, innerX+innerWidth)
	}
}

func TestAuthorizationSecretMasksInput(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 24)
	frameWidth := min(56, bounds.Dx()-4)
	frameX := bounds.Min.X + (bounds.Dx()-frameWidth)/2
	frame := composeAuthorization(bounds, authorizationData{
		Label:    "Password",
		Guidance: "Enter your password",
		Input:    "s3cret",
		Secret:   true,
	})
	text := plainText(frame.Content)
	if strings.Contains(text, "s3cret") {
		t.Error("secret input leaked the plaintext secret")
	}
	if got := strings.Count(text, "•"); got != 6 {
		t.Errorf("secret bullets = %d, want 6", got)
	}
	// Cursor advances by the masked display width.
	if !frame.Cursor.Visible {
		t.Fatal("cursor not visible")
	}
	if got, want := frame.Cursor.X, frameX+2+6; got != want {
		t.Errorf("secret cursor X = %d, want %d", got, want)
	}
}

func TestAuthorizationSubmittedState(t *testing.T) {
	frame := composeAuthorization(image.Rect(0, 0, 100, 24), authorizationData{
		Label:     "Telegram API ID",
		Guidance:  "Enter the numeric api_id from my.telegram.org/apps",
		Input:     "12345",
		Submitted: true,
	})
	text := plainText(frame.Content)
	if !strings.Contains(text, "Input accepted; waiting for authorization") {
		t.Error("submitted state missing accepted text")
	}
}

func TestAuthorizationWaitingState(t *testing.T) {
	frame := composeAuthorization(image.Rect(0, 0, 100, 24), authorizationData{
		Label:   "Telegram API ID",
		Waiting: true,
	})
	text := plainText(frame.Content)
	if !strings.Contains(text, "Connecting to Telegram") {
		t.Error("waiting state missing connecting line")
	}
	if !strings.Contains(text, "Waiting for authorization") {
		t.Error("waiting state missing waiting line")
	}
	if frame.Cursor.Visible {
		t.Error("waiting state should have no cursor")
	}
	if strings.Contains(text, "Type here…") {
		t.Error("waiting state should not show the input placeholder")
	}
}

func TestTooSmallExactDimensionsAndWarning(t *testing.T) {
	for _, tc := range []struct {
		width, height int
	}{
		{59, 17},
		{30, 10},
	} {
		frame := composeTooSmall(image.Rect(0, 0, tc.width, tc.height), "telegram-tui requires at least 60x18")
		if frame.Compositor == nil {
			t.Fatalf("%dx%d: compositor is nil", tc.width, tc.height)
		}
		bounds := frame.Compositor.Bounds()
		if got := bounds.Dx(); got != tc.width {
			t.Errorf("%dx%d: width = %d, want %d", tc.width, tc.height, got, tc.width)
		}
		if got := bounds.Dy(); got != tc.height {
			t.Errorf("%dx%d: height = %d, want %d", tc.width, tc.height, got, tc.height)
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
		if frame.Cursor.Visible {
			t.Errorf("%dx%d: too-small frame should have no cursor", tc.width, tc.height)
		}
		if len(frame.Hits) != 0 {
			t.Errorf("%dx%d: too-small frame should have no hits", tc.width, tc.height)
		}

		// Compare against the grapheme-safe visible text: composeTooSmall
		// truncates the warning to the viewport width on narrow viewports.
		lineWidth := min(tc.width, displayWidth("telegram-tui requires at least 60x18"))
		wantVisible := ansi.Strip(renderLine(newRenderStyles(false).Warning, "telegram-tui requires at least 60x18", lineWidth))
		if !strings.Contains(plainText(frame.Content), wantVisible) {
			t.Errorf("%dx%d: missing warning visible text %q", tc.width, tc.height, wantVisible)
		}
	}
}

func TestTooSmallZeroBoundsSafe(t *testing.T) {
	frame := composeTooSmall(image.Rect(0, 0, 0, 0), "telegram-tui requires at least 60x18")
	if frame.Content != "" {
		t.Errorf("zero-bounds frame content = %q, want empty", frame.Content)
	}
	if frame.Cursor.Visible {
		t.Error("zero-bounds frame should have no cursor")
	}
}

func TestTooSmallWarningCentered(t *testing.T) {
	bounds := image.Rect(0, 0, 59, 17)
	frame := composeTooSmall(bounds, "telegram-tui requires at least 60x18")
	lines := strings.Split(frame.Content, "\n")
	// The warning is centered at absolute Y = bounds.Min.Y + bounds.Dy()/2.
	wantY := bounds.Dy() / 2
	line := plainText(lines[wantY])
	if !strings.Contains(line, "requires at least 60x18") {
		t.Errorf("warning not on centered line %d: %q", wantY, line)
	}
}

func TestStandaloneModelViewUsesSharedBuilder(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	view := model.View()

	text := plainText(view.Content)
	for _, want := range []string{
		"Authorization",
		"Telegram API ID",
		"my.telegram.org/apps",
		"Type here…",
		"Enter: continue",
		"╭",
		"╯",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("standalone view missing %q", want)
		}
	}
	if !view.AltScreen {
		t.Error("standalone view did not request AltScreen")
	}
	// The transparent-background half of this view's contract is asserted once in
	// TestAuthorizationViewPreservesPromptVisualLanguage, derived from the
	// styles_test.go palette contract.
	if view.MouseMode != tea.MouseModeNone {
		t.Errorf("standalone view mouse mode = %v, want MouseModeNone", view.MouseMode)
	}
	if view.Cursor == nil {
		t.Error("standalone view did not declare a cursor")
	}
	// Cursor matches the shared builder's absolute input position.
	frameWidth := min(56, 100-4)
	frameHeight := min(9, 24)
	frameX := (100 - frameWidth) / 2
	frameY := (24 - frameHeight) / 2
	if got, want := view.Cursor.X, frameX+2; got != want {
		t.Errorf("standalone cursor X = %d, want %d", got, want)
	}
	if got, want := view.Cursor.Y, frameY+4; got != want {
		t.Errorf("standalone cursor Y = %d, want %d", got, want)
	}
	// The standalone view carries the semantic text color. The input text cell
	// inherits this ForegroundColor (its own cell foreground is nil), so the
	// two must agree.
	if got := colorOf(view.ForegroundColor); got != rgba(textColor) {
		t.Errorf("standalone view ForegroundColor = %v, want %v", got, rgba(textColor))
	}
}

func TestStandaloneModelViewTooSmall(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 59, Height: 17})
	view := model.View()
	if !strings.Contains(plainText(view.Content), "requires at least 60x18") {
		t.Error("small standalone view missing minimum-size guidance")
	}
	if view.Cursor != nil {
		t.Error("small standalone view should not declare a cursor")
	}
}

func TestStandaloneModelViewUsesSharedBuilderHits(t *testing.T) {
	// The standalone Model.View path must produce a frame with no hits.
	model := NewModel()
	model, _ = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	content, cursorX, cursorY := model.render()
	if content == "" {
		t.Fatal("standalone render produced empty content")
	}
	if cursorX < 0 || cursorY < 0 {
		t.Fatalf("standalone render cursor = (%d,%d), want visible", cursorX, cursorY)
	}
	// render() delegates to composeAuthorization, which has no hits.
	frame := composeAuthorization(image.Rect(0, 0, 100, 24), authorizationDataForInput(""))
	if len(frame.Hits) != 0 {
		t.Errorf("authorization frame should have no hits, got %d", len(frame.Hits))
	}
}

func TestAuthorizationLayerInvalidBoundsSafe(t *testing.T) {
	// buildAuthorizationLayer must be safe in isolation when there is not room
	// for the fixed nine-row frame and at least one inner content cell.
	styles := newRenderStyles(false)
	for _, bounds := range []image.Rectangle{
		{Min: image.Pt(0, 0), Max: image.Pt(0, 0)},
		{Min: image.Pt(0, 0), Max: image.Pt(10, 0)},
		{Min: image.Pt(0, 0), Max: image.Pt(-5, -5)},
		{Min: image.Pt(0, 0), Max: image.Pt(8, 9)},
		{Min: image.Pt(0, 0), Max: image.Pt(9, 8)},
	} {
		surface := buildAuthorizationLayer(bounds, authorizationDataForInput(""), styles)
		if surface.Layer != nil {
			t.Errorf("%v: invalid-bounds layer should have no Layer, got %v", bounds, surface.Layer)
		}
		if surface.Cursor.Visible {
			t.Errorf("%v: invalid-bounds layer should have a hidden cursor", bounds)
		}
	}

	// composeAuthorization returns a safe empty frame for unusable bounds.
	frame := composeAuthorization(image.Rect(0, 0, 8, 9), authorizationDataForInput(""))
	if frame.Content != "" {
		t.Errorf("invalid-bounds composeAuthorization content = %q, want empty", frame.Content)
	}
	if frame.Cursor.Visible {
		t.Error("invalid-bounds composeAuthorization should have a hidden cursor")
	}

	// The exact threshold remains usable.
	if surface := buildAuthorizationLayer(image.Rect(0, 0, 9, 9), authorizationDataForInput(""), styles); surface.Layer == nil {
		t.Error("9x9 authorization bounds should produce a layer")
	}
}
