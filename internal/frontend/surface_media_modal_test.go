package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// mediaModalCanvas composes a full-size styled Base root with the modal layer
// and returns the single Compositor and a Canvas over the viewport. With a nil
// layer it still returns a working compositor/canvas for zero-surface checks.
func mediaModalCanvas(bounds image.Rectangle, surface surfaceResult) (*lipgloss.Compositor, *lipgloss.Canvas) {
	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(bounds.Dx()).Height(bounds.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	compositor := lipgloss.NewCompositor(root)
	return compositor, lipgloss.NewCanvas(bounds.Dx(), bounds.Dy()).Compose(compositor)
}

// mediaModal builds the media modal surface/overlay and a composed canvas.
func mediaModal(bounds image.Rectangle, model ui.ViewModel) (surfaceResult, overlayRequest, *lipgloss.Compositor, *lipgloss.Canvas) {
	surface, request := buildMediaModalLayer(model, newRenderStyles(false))
	compositor, canvas := mediaModalCanvas(bounds, surface)
	if compositor.Render() == "" {
		panic("media modal compositor rendered empty")
	}
	return surface, request, compositor, canvas
}

// readyMediaModel returns a modal in the ready state at a valid viewport.
func readyMediaModel() ui.ViewModel {
	return ui.ViewModel{
		Width:      80,
		Height:     24,
		Focus:      app.FocusModal,
		Modal:      &app.ModalState{Title: "Weekend 開發群", Path: "/tmp/photo.png"},
		ActiveChat: domain.Chat{ID: 77},
	}
}

func mediaInteraction(t *testing.T, surface surfaceResult, id string) layerInteraction {
	t.Helper()
	for _, interaction := range surface.Interactions {
		if interaction.ID == id {
			return interaction
		}
	}
	t.Fatalf("no interaction with ID %q", id)
	return layerInteraction{}
}

func TestMediaModalLayerNilAndEmptySafeZeroRequest(t *testing.T) {
	styles := newRenderStyles(false)
	for _, model := range []ui.ViewModel{
		{Width: 80, Height: 24}, // nil Modal
		{Width: 0, Height: 0, Modal: &app.ModalState{Path: "/tmp/p.png"}},
		{Width: 80, Height: 0, Modal: &app.ModalState{Path: "/tmp/p.png"}},
		{Width: 0, Height: 24, Modal: &app.ModalState{Path: "/tmp/p.png"}},
	} {
		surface, request := buildMediaModalLayer(model, styles)
		if surface.Layer != nil {
			t.Errorf("%#v: zero/empty should have no Layer", model)
		}
		if surface.Cursor.Visible {
			t.Errorf("cursor should be hidden")
		}
		if got := surface.Cursor; got.X != -1 || got.Y != -1 {
			t.Errorf("cursor = %v, want {-1,-1}", got)
		}
		if len(surface.Interactions) != 0 {
			t.Errorf("zero/empty should have no interactions")
		}
		if request.Path != "" || !request.Bounds.Empty() || request.Ready {
			t.Errorf("zero request = %#v", request)
		}
	}
}

func TestMediaModalLayerExactFrameContentGeometry(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	model := readyMediaModel()
	surface, request, _, canvas := mediaModal(bounds, model)

	wantFrame, wantContent := modalSurfaceRectangles(bounds)
	if !surface.Rect.Eq(wantFrame) {
		t.Errorf("surface rect = %v, want %v", surface.Rect, wantFrame)
	}
	if !request.Bounds.Eq(wantContent) {
		t.Errorf("request bounds = %v, want %v", request.Bounds, wantContent)
	}
	if surface.Layer == nil {
		t.Fatal("modal layer is nil")
	}
	if surface.Layer.GetX() != wantFrame.Min.X || surface.Layer.GetY() != wantFrame.Min.Y {
		t.Errorf("layer pos = (%d,%d), want (%d,%d)",
			surface.Layer.GetX(), surface.Layer.GetY(), wantFrame.Min.X, wantFrame.Min.Y)
	}
	if surface.Layer.Width() != wantFrame.Dx() || surface.Layer.Height() != wantFrame.Dy() {
		t.Errorf("layer size = %dx%d, want %dx%d",
			surface.Layer.Width(), surface.Layer.Height(), wantFrame.Dx(), wantFrame.Dy())
	}
	if got := surface.Layer.GetID(); got != "" {
		t.Errorf("frame layer ID = %q, want empty", got)
	}

	// Actual four rounded corners with the focused palette.
	for _, tc := range []struct {
		x, y int
		want string
	}{
		{wantFrame.Min.X, wantFrame.Min.Y, "╭"},
		{wantFrame.Max.X - 1, wantFrame.Min.Y, "╮"},
		{wantFrame.Min.X, wantFrame.Max.Y - 1, "╰"},
		{wantFrame.Max.X - 1, wantFrame.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(tc.x, tc.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) nil", tc.x, tc.y)
		}
		if got := cell.Content; got != tc.want {
			t.Errorf("corner (%d,%d) = %q, want %q", tc.x, tc.y, got, tc.want)
		}
		if got := colorOf(cell.Style.Fg); got != rgba(focusedBorderColor) {
			t.Errorf("corner (%d,%d) fg = %v, want %v", tc.x, tc.y, got, rgba(focusedBorderColor))
		}
	}

	// Title sits at local (2,0) → absolute frame.Min+(2,0).
	titleCell := canvas.CellAt(wantFrame.Min.X+2, wantFrame.Min.Y)
	if titleCell == nil || titleCell.Content == "" || titleCell.Content == "╮" {
		t.Errorf("title first cell = %q", titleCell.Content)
	}
}

func TestMediaModalLayerTitleCJKFE0FZWJClipsBeforeCloseAndCloseParity(t *testing.T) {
	// A real ZWJ family, CJK, and FE0F emoji: the title must clip strictly
	// before the top-right close cell so the corner/close always win.
	title := "很长的标题👨‍👩‍👧‍👦🔥❤️多字节"
	model := readyMediaModel()
	model.Modal.Title = title
	bounds := image.Rect(0, 0, 80, 24)
	surface, _, compositor, canvas := mediaModal(bounds, model)
	frame, _ := modalSurfaceRectangles(bounds)

	// Top-right corner stays the rounded ┐ regardless of the title.
	corner := canvas.CellAt(frame.Max.X-1, frame.Min.Y)
	if corner == nil || corner.Content != "╮" {
		t.Errorf("top-right corner = %+v, want ╮", corner)
	}

	// Close at frame.Max.X-2 under media:close, exact 1-cell.
	close := mediaInteraction(t, surface, "media:close")
	wantRect := image.Rect(frame.Max.X-2, frame.Min.Y, frame.Max.X-1, frame.Min.Y+1)
	if !close.Rect.Eq(wantRect) {
		t.Errorf("close rect = %v, want %v", close.Rect, wantRect)
	}
	if close.Z != zModalControl {
		t.Errorf("close Z = %d, want %d", close.Z, zModalControl)
	}
	if close.Click.Action != app.Close {
		t.Errorf("close action = %#v, want Close", close.Click)
	}
	if close.Virtual {
		t.Errorf("close visual interaction must not be Virtual")
	}
	// Compositor Hit parity at the close cell and inside it.
	pt := wantRect.Min
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "media:close" {
		t.Errorf("Hit(%v) = %q, want media:close", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(close.Rect) {
		t.Errorf("close hit bounds = %v, want %v", got, close.Rect)
	}

	// The clipped title never grows past the interval before the close cell.
	titleWidth := frame.Dx() - 4
	if titleWidth > 0 {
		if got := ansi.StringWidth(ansi.Truncate(title, titleWidth, "")); got > titleWidth {
			t.Errorf("clipped title width = %d, want <= %d", got, titleWidth)
		}
	}
	// A cell far left inside the top border is a title glyph, not the corner.
	first := canvas.CellAt(frame.Min.X+2, frame.Min.Y)
	if first == nil || first.Content == "" || first.Content == "╮" || first.Content == "│" {
		t.Errorf("title left cell = %+v", first)
	}
}

func TestMediaModalLayerOutsideInteractionsExactNoVisual(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	model := readyMediaModel()
	surface, _, compositor, _ := mediaModal(bounds, model)
	frame, _ := modalSurfaceRectangles(bounds)

	wantRects := []struct {
		id   string
		rect image.Rectangle
	}{
		{"media:outside:top", image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, frame.Min.Y)},
		{"media:outside:bottom", image.Rect(bounds.Min.X, frame.Max.Y, bounds.Max.X, bounds.Max.Y)},
		{"media:outside:left", image.Rect(bounds.Min.X, frame.Min.Y, frame.Min.X, frame.Max.Y)},
		{"media:outside:right", image.Rect(frame.Max.X, frame.Min.Y, bounds.Max.X, frame.Max.Y)},
	}
	ids := map[string]bool{}
	for _, want := range wantRects {
		want.rect = want.rect.Intersect(bounds)
		if want.rect.Empty() {
			continue
		}
		interaction := mediaInteraction(t, surface, want.id)
		if !interaction.Rect.Eq(want.rect) {
			t.Errorf("%s rect = %v, want %v", want.id, interaction.Rect, want.rect)
		}
		if !interaction.Virtual {
			t.Errorf("%s must be Virtual", want.id)
		}
		if interaction.Click.Action != app.Close {
			t.Errorf("%s action = %#v, want Close", want.id, interaction.Click)
		}
		if interaction.Z != zModalControl {
			t.Errorf("%s Z = %d, want %d", want.id, interaction.Z, zModalControl)
		}
		// Corner/edge coverage never overlaps the frame.
		if !interaction.Rect.Intersect(frame).Empty() {
			t.Errorf("%s overlaps the frame", want.id)
		}
		// Virtual interactions must not exist as visual layers in the compositor.
		pt := image.Pt(interaction.Rect.Min.X+interaction.Rect.Dx()/2, interaction.Rect.Min.Y+interaction.Rect.Dy()/2)
		if hit := compositor.Hit(pt.X, pt.Y); hit.ID() != "" {
			t.Errorf("%s should have no compositor layer hit, got %q", want.id, hit.ID())
		}
		ids[want.id] = true
	}
	// No interaction id contains the outside prefix as a Layer id.
	if surface.Layer != nil {
		// The four outside regions above documented; assert none of the four
		// leaves a real Layer id behind by checking the involved IDs are all
		// Virtual (already asserted) and IDs unique.
		if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
			t.Errorf("unique IDs: %v", err)
		}
	}
	_ = ids
}

func TestMediaModalLayerReadyRequestMetadata(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	model := readyMediaModel()
	surface, request, _, _ := mediaModal(bounds, model)
	_, content := modalSurfaceRectangles(bounds)

	if request.Path != model.Modal.Path {
		t.Errorf("request path = %q, want %q", request.Path, model.Modal.Path)
	}
	if !request.Bounds.Eq(content) {
		t.Errorf("request bounds = %v, want %v", request.Bounds, content)
	}
	if request.Columns != 80 {
		t.Errorf("request columns = %d, want 80", request.Columns)
	}
	if request.Rows != 24 {
		t.Errorf("request rows = %d, want 24", request.Rows)
	}
	if request.SelectedID != int64(model.ActiveChat.ID) {
		t.Errorf("request selectedID = %d, want %d", request.SelectedID, int64(model.ActiveChat.ID))
	}
	if !request.Ready {
		t.Errorf("ready request should be Ready")
	}
	if !surface.Rect.Empty() && surface.Rect.Dx() > 0 {
		// Ready content rectangle equals the text builder Content.
		if !request.Bounds.Eq(content) {
			t.Errorf("ready bounds != modalSurfaceRectangles content")
		}
	}
}

func TestMediaModalLayerReadyFalseIndependently(t *testing.T) {
	build := func(mutate func(*ui.ViewModel)) (ui.ViewModel, bool) {
		model := readyMediaModel()
		mutate(&model)
		_, request, _, _ := mediaModal(image.Rect(0, 0, model.Width, model.Height), model)
		return model, request.Ready && request.Path == model.Modal.Path && !request.Bounds.Empty() &&
			request.Columns == model.Width && request.Rows == model.Height &&
			request.SelectedID == int64(model.ActiveChat.ID)
	}

	cases := []struct {
		name   string
		mutate func(*ui.ViewModel)
	}{
		{"empty path", func(m *ui.ViewModel) { m.Modal.Path = "" }},
		{"loading", func(m *ui.ViewModel) { m.Modal.Loading = true }},
		{"error", func(m *ui.ViewModel) { m.Modal.Error = &domain.AppError{Kind: domain.ErrorMedia, Message: "boom"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, _ := build(tc.mutate)
			_, request := buildMediaModalLayer(model, newRenderStyles(false))
			if request.Ready {
				t.Errorf("request should not be Ready for %q", tc.name)
			}
			// Metadata retained for the valid modal (Bounds stays the content
			// rectangle even when the path is empty/loading/error).
			_, wantContent := modalSurfaceRectangles(image.Rect(0, 0, model.Width, model.Height))
			if !request.Bounds.Eq(wantContent) || request.Columns != model.Width || request.Rows != model.Height ||
				request.SelectedID != int64(model.ActiveChat.ID) {
				t.Errorf("%q: metadata not retained: %#v", tc.name, request)
			}
		})
	}

	// Width below minimum.
	t.Run("narrow", func(t *testing.T) {
		model := readyMediaModel()
		model.Width = minimumWidth - 1
		_, request := buildMediaModalLayer(model, newRenderStyles(false))
		if request.Ready {
			t.Errorf("narrow should not be Ready")
		}
		if request.Columns != model.Width || request.Bounds.Empty() {
			t.Errorf("narrow metadata lost: %#v", request)
		}
	})
	// Height below minimum.
	t.Run("short", func(t *testing.T) {
		model := readyMediaModel()
		model.Height = minimumHeight - 1
		_, request := buildMediaModalLayer(model, newRenderStyles(false))
		if request.Ready {
			t.Errorf("short should not be Ready")
		}
		if request.Rows != model.Height || request.Bounds.Empty() {
			t.Errorf("short metadata lost: %#v", request)
		}
	})
	// Prompt nonnil.
	t.Run("prompt", func(t *testing.T) {
		model := readyMediaModel()
		model.Prompt = &app.PromptState{}
		_, request := buildMediaModalLayer(model, newRenderStyles(false))
		if request.Ready {
			t.Errorf("prompt should not be Ready")
		}
	})
	// FocusAuth.
	t.Run("auth", func(t *testing.T) {
		model := readyMediaModel()
		model.Focus = app.FocusAuth
		_, request := buildMediaModalLayer(model, newRenderStyles(false))
		if request.Ready {
			t.Errorf("auth focus should not be Ready")
		}
	})
}

func TestMediaModalLayerLoadingShell(t *testing.T) {
	model := readyMediaModel()
	model.Modal.Loading = true
	bounds := image.Rect(0, 0, model.Width, model.Height)
	surface, request, _, canvas := mediaModal(bounds, model)
	_, content := modalSurfaceRectangles(bounds)

	if request.Ready {
		t.Errorf("loading request should not be Ready")
	}
	if !request.Bounds.Eq(content) {
		t.Errorf("loading request bounds = %v, want %v", request.Bounds, content)
	}

	text := plainText(canvas.Render())
	if !strings.Contains(text, "Loading image...") {
		t.Errorf("loading text missing, got %q", text)
	}
	// Single-line, bounded: full 24-row viewport never wraps or overflows.
	lines := strings.Split(text, "\n")
	if len(lines) != 24 {
		t.Errorf("loading rendered %d lines, want 24", len(lines))
	}
	// Centered within content vertically.
	wantY := content.Min.Y + content.Dy()/2
	lines2 := strings.Split(plainText(canvas.Render()), "\n")
	if !strings.Contains(lines2[wantY], "Loading image") {
		t.Errorf("loading text not on centered Content Y=%d", wantY)
	}
	// No retry interaction.
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "media:retry" {
			t.Errorf("loading state should not have retry")
		}
	}
}

func TestMediaModalLayerErrorShellAndRetry(t *testing.T) {
	model := readyMediaModel()
	model.Modal.Error = &domain.AppError{Kind: domain.ErrorMedia, Message: "Image unavailable"}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	surface, request, compositor, canvas := mediaModal(bounds, model)
	_, content := modalSurfaceRectangles(bounds)

	if request.Ready {
		t.Errorf("error request should not be Ready")
	}
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Image unavailable") {
		t.Errorf("error message missing, got %q", text)
	}

	// Error message grapheme-safe bounded at y = content.Min.Y+max(0,Dy/2-1).
	wantY := content.Min.Y + max(0, content.Dy()/2-1)
	lines := strings.Split(plainText(canvas.Render()), "\n")
	if len(lines) != 24 || ansi.StringWidth(lines[wantY]) > model.Width {
		t.Errorf("error line %d out of bounds", wantY)
	}

	// Exact Retry interaction.
	retry := mediaInteraction(t, surface, "media:retry")
	if retry.Click.Action != app.Retry {
		t.Errorf("retry action = %#v, want Retry", retry.Click)
	}
	if retry.Z != zModalControl {
		t.Errorf("retry Z = %d, want %d", retry.Z, zModalControl)
	}
	if retry.Virtual {
		t.Errorf("retry must be a visual interaction")
	}
	// Retry text is centered and clipped within Content.
	retryY := min(content.Max.Y-1, wantY+2)
	if !strings.Contains(strings.Split(plainText(canvas.Render()), "\n")[retryY], "Retry") {
		t.Errorf("retry not on expected line")
	}
	// Hit parity at retry.
	pt := image.Pt(retry.Rect.Min.X+retry.Rect.Dx()/2, retry.Rect.Min.Y+retry.Rect.Dy()/2)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "media:retry" {
		t.Errorf("Hit(%v) = %q, want media:retry", pt, hit.ID())
	}
}

func TestMediaModalLayerReadyStateNoShellText(t *testing.T) {
	model := readyMediaModel()
	bounds := image.Rect(0, 0, model.Width, model.Height)
	surface, request, compositor, canvas := mediaModal(bounds, model)

	if !request.Ready {
		t.Fatalf("ready model should be Ready")
	}
	text := plainText(canvas.Render())
	for _, forbidden := range []string{"Loading image...", "Retry", "Image unavailable"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("ready state should not contain %q, got %q", forbidden, text)
		}
	}
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "media:retry" {
			t.Errorf("ready state should not have retry interaction")
		}
	}
	// Retry has no compositor hit in ready state.
	if hit := compositor.Hit(0, 0); hit.ID() == "media:retry" {
		t.Errorf("ready state should have no retry hit")
	}
}

func TestMediaModalLayerInteractionsMatchHitsAndCompile(t *testing.T) {
	model := readyMediaModel()
	model.Modal.Error = &domain.AppError{Kind: domain.ErrorMedia, Message: "无法加载图片"}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	surface, _, compositor, _ := mediaModal(bounds, model)

	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	// Every visual interaction matches the Compositor Hit bounds; virtual ones
	// do not exist in the compositor.
	for _, interaction := range surface.Interactions {
		pt := image.Pt(interaction.Rect.Min.X+interaction.Rect.Dx()/2, interaction.Rect.Min.Y+interaction.Rect.Dy()/2)
		hit := compositor.Hit(pt.X, pt.Y)
		if interaction.Virtual {
			if hit.ID() != "" {
				t.Errorf("virtual %q should not have layer hit, got %q", interaction.ID, hit.ID())
			}
			continue
		}
		if hit.ID() != interaction.ID {
			t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), interaction.ID)
		}
		if got := hit.Bounds(); !got.Eq(interaction.Rect) {
			t.Errorf("Hit(%v) bounds = %v, want %v", pt, got, interaction.Rect)
		}
	}

	// All interactions (including virtual) compile into Hits with the virtual
	// ones carrying the Close action.
	hits := compileHits(surface.Interactions)
	if len(hits) == 0 {
		t.Fatal("compileHits produced no hits")
	}
	virtualCount := 0
	for _, interaction := range surface.Interactions {
		if interaction.Virtual {
			virtualCount++
			found := false
			for _, hit := range hits {
				if hit.Rect.Eq(interaction.Rect) {
					found = true
					if hit.Click.Action != app.Close {
						t.Errorf("virtual %q hit click = %#v, want Close", interaction.ID, hit.Click)
					}
				}
			}
			if !found {
				t.Errorf("virtual %q missing from compiled hits", interaction.ID)
			}
		}
	}
	if virtualCount != 4 {
		t.Errorf("virtual interactions = %d, want 4", virtualCount)
	}
}

func TestMediaModalLayerTinyPositiveViewportsSafeNeverReady(t *testing.T) {
	styles := newRenderStyles(false)
	for _, w := range []int{1, 2, 3, 4, 5, 8} {
		for _, h := range []int{1, 2, 3, 4, 5, 8} {
			model := ui.ViewModel{
				Width:  w,
				Height: h,
				Modal:  &app.ModalState{Path: "/tmp/p.png"},
			}
			surface, request := buildMediaModalLayer(model, styles)
			if request.Ready {
				t.Errorf("%dx%d: tiny viewport should never be Ready", w, h)
			}
			if surface.Cursor.Visible {
				t.Errorf("%dx%d: cursor should be hidden", w, h)
			}
			if surface.Layer != nil {
				if surface.Layer.GetX() < 0 || surface.Layer.GetY() < 0 ||
					surface.Layer.GetX()+surface.Layer.Width() > w ||
					surface.Layer.GetY()+surface.Layer.Height() > h {
					t.Errorf("%dx%d: layer escapes viewport", w, h)
				}
			}
		}
	}
}
