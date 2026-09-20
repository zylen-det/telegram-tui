package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// composerCanvas composes a single fixed-size viewport root at (0,0) sized to
// the model dimensions and attaches the composer surface root, returning the
// resulting canvas, compositor, and the surface result for inspection.
func composerCanvas(model ViewModel, rect image.Rectangle, styles renderStyles, composerView ...string) (*lipgloss.Canvas, *lipgloss.Compositor, surfaceResult) {
	result := buildComposerLayer(model, rect, styles, composerView...)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	if result.Layer != nil {
		root.AddLayers(result.Layer)
	}
	compositor := lipgloss.NewCompositor(root)
	canvas := lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor)
	return canvas, compositor, result
}

func composerModel(width, height int, active domain.Chat) ViewModel {
	return ViewModel{
		Width:      width,
		Height:     height,
		Focus:      FocusConversation,
		ActiveChat: active,
	}
}

func writable(chatID int64) domain.Chat {
	return domain.Chat{ID: domain.ChatID(chatID), Title: "Chat", CanSend: true}
}

func readOnly(chatID int64) domain.Chat {
	return domain.Chat{ID: domain.ChatID(chatID), Title: "Chat", CanSend: false}
}

func hasInteraction(result surfaceResult, id string) bool {
	for _, interaction := range result.Interactions {
		if interaction.ID == id {
			return true
		}
	}
	return false
}

// interaction returns the interaction with the given ID, or nil.
func interactionByID(result surfaceResult, id string) *layerInteraction {
	for i := range result.Interactions {
		if result.Interactions[i].ID == id {
			return &result.Interactions[i]
		}
	}
	return nil
}

// withinRect reports whether every layer (root included) descends entirely
// within the given absolute rect and every interaction/rendered line is inside
// the root bounds.
func TestComposerLayerEmptyOrOutOfViewport(t *testing.T) {
	styles := newRenderStyles(false)
	model := composerModel(100, 30, writable(1))

	// Entirely out of viewport.
	empty := buildComposerLayer(model, image.Rect(120, 10, 180, 20), styles)
	if empty.Layer != nil {
		t.Fatalf("out-of-viewport layer non-nil")
	}
	if empty.Cursor.Visible {
		t.Fatalf("out-of-viewport cursor visible")
	}
	if empty.Cursor.X != -1 || empty.Cursor.Y != -1 {
		t.Fatalf("out-of-viewport cursor = %+v, want hidden (-1,-1)", empty.Cursor)
	}

	// Zero/empty rect.
	empty = buildComposerLayer(model, image.Rect(0, 0, 0, 0), styles)
	if empty.Layer != nil {
		t.Fatalf("zero-size layer non-nil")
	}
	if empty.Cursor.Visible {
		t.Fatalf("zero-size cursor visible")
	}

	// Partially out of viewport is clipped to the viewport.
	clipped := buildComposerLayer(model, image.Rect(-20, -10, 100, 5), styles)
	want := image.Rect(0, 0, 100, 5)
	if !clipped.Rect.Eq(want) {
		t.Fatalf("clipped rect = %v, want %v", clipped.Rect, want)
	}
	if clipped.Layer == nil {
		t.Fatalf("clipped layer is nil")
	}
}

// The composer root must occupy the viewport exactly and keep the same
// geometry whether or not it is focused. The palette and terminal-default
// background contract for its styles lives in styles_test.go.
func TestComposerLayerRootExactBoundsAcrossFocus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		focused bool
	}{
		{"unfocused-panel", false},
		{"focused-input", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h := 60, 6
			model := composerModel(w, h, writable(1))
			if tc.focused {
				model.Focus = FocusComposer
			}
			styles := newRenderStyles(false)
			_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

			bounds := compositor.Bounds()
			if !bounds.Eq(image.Rect(0, 0, w, h)) {
				t.Fatalf("compositor bounds = %v, want %v", bounds, image.Rect(0, 0, w, h))
			}
			if got := result.Rect; !got.Eq(image.Rect(0, 0, w, h)) {
				t.Fatalf("result rect = %v, want %v", got, image.Rect(0, 0, w, h))
			}
			if got := result.Layer.GetX(); got != 0 {
				t.Errorf("root X = %d, want 0", got)
			}
			if got := result.Layer.GetY(); got != 0 {
				t.Errorf("root Y = %d, want 0", got)
			}
			if got := result.Layer.Width(); got != w {
				t.Errorf("root width = %d, want %d", got, w)
			}
			if got := result.Layer.Height(); got != h {
				t.Errorf("root height = %d, want %d", got, h)
			}
			if got := result.Layer.GetID(); got != "composer" {
				t.Errorf("root ID = %q, want composer", got)
			}
			// Focus must not move the pane: the root interaction stays exact.
			interaction := interactionByID(result, "composer")
			if interaction == nil {
				t.Fatal("no composer interaction")
			}
			if !interaction.Rect.Eq(image.Rect(0, 0, w, h)) {
				t.Errorf("composer interaction rect = %v, want %v", interaction.Rect, image.Rect(0, 0, w, h))
			}
		})
	}
}

func TestComposerLayerCustomRectAbsOrigin(t *testing.T) {
	styles := newRenderStyles(false)
	// A rect not at the viewport origin still roots absolutely at rect.Min.
	model := composerModel(120, 40, writable(1))
	rect := image.Rect(10, 5, 90, 15)
	canvas, compositor, result := composerCanvas(model, rect, styles)
	if !result.Rect.Eq(rect) {
		t.Fatalf("result rect = %v, want %v", result.Rect, rect)
	}
	if got := result.Layer.GetX(); got != rect.Min.X {
		t.Errorf("root X = %d, want %d", got, rect.Min.X)
	}
	if got := result.Layer.GetY(); got != rect.Min.Y {
		t.Errorf("root Y = %d, want %d", got, rect.Min.Y)
	}
	// The root interaction covers the absolute rect.
	interaction := interactionByID(result, "composer")
	if interaction == nil {
		t.Fatal("no composer interaction")
	}
	if !interaction.Rect.Eq(rect) {
		t.Fatalf("composer interaction rect = %v, want %v", interaction.Rect, rect)
	}
	// Hit parity at an internal cell.
	if hit := compositor.Hit(rect.Min.X+20, rect.Min.Y+2); hit.ID() != "composer" {
		t.Fatalf("hit ID = %q at %v, want composer (bounds %v)",
			hit.ID(), image.Pt(rect.Min.X+20, rect.Min.Y+2), hit.Bounds())
	}
	canvas.CellAt(rect.Min.X, rect.Min.Y)
}

func TestComposerLayerNoActiveChat(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 60, 6
	model := composerModel(w, h, domain.Chat{})
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	if result.Layer == nil {
		t.Fatal("root layer nil")
	}
	if got := result.Layer.GetID(); got != "" {
		t.Errorf("root ID = %q, want empty for no active chat", got)
	}
	if len(result.Interactions) != 0 {
		t.Errorf("unexpected interactions for no active chat: %+v", result.Interactions)
	}
	if result.Cursor.Visible {
		t.Errorf("cursor visible for no active chat")
	}
	_ = compositor
}

func TestComposerLayerReadOnly(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 40, 4
	model := composerModel(w, h, readOnly(7))
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	if result.Layer == nil {
		t.Fatal("root layer nil")
	}
	// No interactions and no cursor.
	if len(result.Interactions) != 0 {
		t.Errorf("read-only interactions = %+v, want none", result.Interactions)
	}
	if result.Cursor.Visible {
		t.Errorf("read-only cursor visible")
	}
	// The "Read-only" label appears on row 1.
	row := rowPlain(compositor.Render(), 1)
	if !strings.Contains(row, "Read-only") {
		t.Errorf("read-only label missing on row 1 (got %q)", row)
	}

	// Tiny rect bounds the label and never escapes.
	for _, r := range []image.Rectangle{
		image.Rect(0, 0, 1, 1),
		image.Rect(0, 0, 2, 1),
		image.Rect(0, 0, 3, 2),
		image.Rect(0, 0, 4, 1),
	} {
		rect := r
		model := composerModel(6, 4, readOnly(7))
		_, _, res := composerCanvas(model, rect, styles)
		if res.Layer == nil {
			t.Fatalf("rect %v: layer nil", rect)
		}
		// The composer root layer itself must never escape the requested rect.
		if res.Layer.Width() > rect.Dx() || res.Layer.Height() > rect.Dy() {
			t.Fatalf("rect %v: read-only layer %dx%d exceeds rect %dx%d",
				rect, res.Layer.Width(), res.Layer.Height(), rect.Dx(), rect.Dy())
		}
		if res.Layer.GetX() != rect.Min.X || res.Layer.GetY() != rect.Min.Y {
			t.Fatalf("rect %v: layer origin (%d,%d) != (%d,%d)",
				rect, res.Layer.GetX(), res.Layer.GetY(), rect.Min.X, rect.Min.Y)
		}
	}
}

// plainTextLayer extracts the stripped content of a layer.
func plainTextLayer(layer *lipgloss.Layer, row int) (string, bool) {
	content := layer.GetContent()
	lines := strings.Split(plainText(content), "\n")
	if row < 0 || row >= len(lines) {
		return "", false
	}
	return lines[row], true
}

func TestComposerLayerWritableInteraction(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 60, 6
	model := composerModel(w, h, writable(1))
	canvas, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	if got := result.Layer.GetID(); got != "composer" {
		t.Fatalf("root ID = %q, want composer", got)
	}
	interaction := interactionByID(result, "composer")
	if interaction == nil {
		t.Fatal("no composer interaction")
	}
	wantRect := image.Rect(0, 0, w, h)
	if !interaction.Rect.Eq(wantRect) {
		t.Fatalf("composer interaction rect = %v, want %v", interaction.Rect, wantRect)
	}
	if interaction.Z != zPaneBackground {
		t.Errorf("composer interaction Z = %d, want %d", interaction.Z, zPaneBackground)
	}
	if interaction.Click.Action != FocusPane || interaction.Click.TargetFocus != FocusComposer {
		t.Errorf("composer interaction click = %+v, want FocusPane/FocusComposer", interaction.Click)
	}
	// Hit parity across several cells.
	for _, pt := range []image.Point{{1, 1}, {2, 3}, {w - 1, h - 1}, {5, 0}} {
		if hit := compositor.Hit(pt.X, pt.Y); hit.ID() != "composer" {
			t.Errorf("hit(%d,%d) = %q, want composer", pt.X, pt.Y, hit.ID())
		}
	}
	_ = canvas
}

func TestComposerLayerSendControl(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 30, 5
	model := composerModel(w, h, writable(1))
	canvas, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	send := interactionByID(result, "composer:send")
	if send == nil {
		t.Fatal("no send interaction")
	}
	sendTextW := displayWidth("[Send]")
	want := image.Rect(w-sendTextW-1, h-1, w-1, h)
	if !send.Rect.Eq(want) {
		t.Fatalf("send rect = %v, want %v", send.Rect, want)
	}
	if send.Z != zControl {
		t.Errorf("send Z = %d, want zControl", send.Z)
	}
	if send.Click.Action != ComposerSubmit {
		t.Errorf("send click = %+v, want ComposerSubmit", send.Click)
	}
	// Hit parity.
	if hit := compositor.Hit(want.Min.X, min(want.Min.Y, h-1)); hit.ID() != "composer:send" {
		t.Errorf("send hit = %q, want composer:send", hit.ID())
	}
	_ = canvas

	// Tiny width: root layer bounded, never escapes rect, interactions to the
	// intersection always within the rect.
	for _, rect := range []image.Rectangle{
		image.Rect(0, 0, 1, 1),
		image.Rect(0, 0, 2, 1),
		image.Rect(0, 0, 1, 2),
		image.Rect(0, 0, 9, 1),
	} {
		model := composerModel(12, 4, writable(1))
		_, _, res := composerCanvas(model, rect, styles)
		if res.Layer == nil {
			continue
		}
		if res.Layer.Width() > rect.Dx() || res.Layer.Height() > rect.Dy() {
			t.Fatalf("rect %v: layer %dx%d exceeds rect", rect, res.Layer.Width(), res.Layer.Height())
		}
		for _, ls := range res.Interactions {
			if ls.Rect.Empty() {
				continue
			}
			if ls.Rect.Max.X > rect.Max.X || ls.Rect.Max.Y > rect.Max.Y || ls.Rect.Min.X < rect.Min.X || ls.Rect.Min.Y < rect.Min.Y {
				t.Fatalf("rect %v: interaction %q rect %v escapes", rect, ls.ID, ls.Rect)
			}
		}
	}
}

func TestComposerLayerNoInteractionWithoutWritable(t *testing.T) {
	styles := newRenderStyles(false)
	// EditTarget exists but chat is read-only: no controls/interactions.
	model := composerModel(40, 5, readOnly(7))
	model.Focus = FocusComposer
	model.EditTarget = &EditTarget{ChatID: 7, Error: &domain.AppError{Message: "boom"}}
	_, _, result := composerCanvas(model, image.Rect(0, 0, 40, 5), styles)
	if len(result.Interactions) != 0 {
		t.Errorf("read-only with edit target produced interactions: %+v", result.Interactions)
	}
	if result.Cursor.Visible {
		t.Errorf("read-only cursor visible")
	}
}

func TestComposerLayerReplyBannerClipsBeforeCancel(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 30, 5
	model := composerModel(w, h, writable(1))
	model.ReplyTarget = &ReplyTarget{ChatID: 1, Sender: "Mina", Preview: "hello world"}
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	cancel := interactionByID(result, "composer:cancel-reply")
	if cancel == nil {
		t.Fatal("no cancel-reply interaction")
	}
	cancelW := displayWidth("[Cancel]")
	want := image.Rect(w-cancelW-1, 0, w-1, 1)
	if !cancel.Rect.Eq(want) {
		t.Fatalf("cancel-reply rect = %v, want %v", cancel.Rect, want)
	}
	if cancel.Click.Action != CancelReply {
		t.Errorf("cancel click = %+v, want CancelReply", cancel.Click)
	}
	if cancel.Z != zControl {
		t.Errorf("cancel Z = %d, want zControl", cancel.Z)
	}
	// Hit parity over the entire cancel interval.
	for x := want.Min.X; x < want.Max.X; x++ {
		if hit := compositor.Hit(x, 0); hit.ID() != "composer:cancel-reply" {
			t.Errorf("cancel hit(%d,0) = %q, want composer:cancel-reply", x, hit.ID())
		}
	}
	// The banner is clipped strictly before the cancel interval: no banner
	// cell on row 0 may report the cancel control.
	for x := 1; x < want.Min.X; x++ {
		if hit := compositor.Hit(x, 0); hit.ID() == "composer:cancel-reply" {
			t.Fatalf("cancel-reply hit at banner cell (%d,0); banner overlaps cancel", x)
		}
	}
}

// rowPlain returns the trimmed plain-text content of a rendered frame row.
func rowPlain(frame string, row int) string {
	rows := strings.Split(plainText(frame), "\n")
	if row < 0 || row >= len(rows) {
		return ""
	}
	return strings.TrimRight(rows[row], " ")
}

// TestComposerReplyCJKFe0fZwj uses a long multi-cluster preview with FE0F/ZWJ
// sequences. The banner clipping must never let a wide grapheme bleed into
// the cancel interval, so the visible banner cells stay strictly left of
// cancel.Min.X.
func TestComposerLayerReplyCJKFe0fZwj(t *testing.T) {
	styles := newRenderStyles(false)
	for _, width := range []int{20, 24, 30, 40} {
		w := width
		h := 5
		model := composerModel(w, h, writable(1))
		preview := "界❤️👨\u200d👩\u200d👧\u200d👦👍🔥 中文长文本overflow"
		model.ReplyTarget = &ReplyTarget{ChatID: 1, Sender: "名", Preview: preview}
		_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)
		cancel := interactionByID(result, "composer:cancel-reply")
		if cancel == nil {
			t.Fatalf("width %d: no cancel-reply interaction", w)
		}

		// Cancel occupies the exact right interval on row 0.
		cancelW := displayWidth(cancelText)
		want := image.Rect(w-cancelW-1, 0, w-1, 1)
		if !cancel.Rect.Eq(want) {
			t.Fatalf("width %d: cancel rect = %v, want %v", w, cancel.Rect, want)
		}
		// Every cell of the cancel interval must hit the cancel control, not a
		// bleed-through wide grapheme from the banner.
		for x := want.Min.X; x < want.Max.X; x++ {
			hit := compositor.Hit(x, 0)
			if hit.ID() != "composer:cancel-reply" {
				t.Fatalf("width %d: cancel cell (%d,0) hit %q, want composer:cancel-reply", w, x, hit.ID())
			}
		}
	}
}

func TestComposerLayerEditBannerAndError(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 30, 6
	model := composerModel(w, h, writable(1))
	model.EditTarget = &EditTarget{ChatID: 1, Buffer: "abc", Error: &domain.AppError{Message: "failed"}}
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	cancel := interactionByID(result, "composer:cancel-edit")
	if cancel == nil {
		t.Fatal("no cancel-edit interaction")
	}
	cancelW := displayWidth(cancelText)
	want := image.Rect(w-cancelW-1, 0, w-1, 1)
	if !cancel.Rect.Eq(want) {
		t.Fatalf("cancel-edit rect = %v, want %v", cancel.Rect, want)
	}
	if cancel.Click.Action != CancelEdit {
		t.Errorf("cancel-edit click = %+v, want CancelEdit", cancel.Click)
	}
	if hit := compositor.Hit(want.Min.X, 0); hit.ID() != "composer:cancel-edit" {
		t.Errorf("cancel-edit hit = %q, want composer:cancel-edit", hit.ID())
	}

	// "Edit failed" appears on row 1 (below the edit banner on row 0).
	rows := rowPlain(compositor.Render(), 1)
	if !strings.Contains(rows, "Edit failed") {
		t.Errorf("row 1 missing 'Edit failed'")
	}
	_ = result
}
func TestComposerLayerEditErrorOmittedNoPreSendRow(t *testing.T) {
	styles := newRenderStyles(false)
	// height 2: edit banner row 0, send row 1 => no room for Edit failed.
	w, h := 20, 2
	model := composerModel(w, h, writable(1))
	model.EditTarget = &EditTarget{ChatID: 1, Buffer: "x", Error: &domain.AppError{Message: "failed"}}
	_, compositor, _ := composerCanvas(model, image.Rect(0, 0, w, h), styles)
	// No error line exists; the error text must not render on the send row.
	rows := strings.Split(plainText(compositor.Render()), "\n")
	if len(rows) == 2 {
		if strings.Contains(rows[1], "Edit failed") {
			t.Errorf("edit error rendered on send row: %q", rows[1])
		}
	}
	// "Edit failed" must not appear anywhere.
	if strings.Contains(plainText(compositor.Render()), "Edit failed") {
		t.Errorf("edit error rendered without a pre-send row")
	}
}

func TestComposerLayerReplyAndEditCoexistInOrder(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 40, 6
	model := composerModel(w, h, writable(1))
	model.ReplyTarget = &ReplyTarget{ChatID: 1, Sender: "S", Preview: "p"}
	model.EditTarget = &EditTarget{ChatID: 1, Buffer: "abc", Error: nil}
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

	replyCancel := interactionByID(result, "composer:cancel-reply")
	editCancel := interactionByID(result, "composer:cancel-edit")
	if replyCancel == nil || editCancel == nil {
		t.Fatalf("reply/edit cancels missing: reply=%v edit=%v", replyCancel, editCancel)
	}
	cancelW := displayWidth(cancelText)
	if want := image.Rect(w-cancelW-1, 0, w-1, 1); !replyCancel.Rect.Eq(want) {
		t.Errorf("reply cancel rect = %v, want %v", replyCancel.Rect, want)
	}
	if want := image.Rect(w-cancelW-1, 1, w-1, 2); !editCancel.Rect.Eq(want) {
		t.Errorf("edit cancel rect = %v, want %v (should be row 1)", editCancel.Rect, want)
	}
	// Distinct controls never overlap.
	seen := map[string]bool{}
	for _, ls := range result.Interactions {
		if seen[ls.ID] {
			t.Errorf("duplicate interaction ID %q", ls.ID)
		}
		seen[ls.ID] = true
	}
	// Edit banner row 1, cancel row 1 hit parity.
	if hit := compositor.Hit(w-cancelW-1, 1); hit.ID() != "composer:cancel-edit" {
		t.Errorf("hit row 1 = %q, want composer:cancel-edit", hit.ID())
	}
	if hit := compositor.Hit(w-cancelW-1, 0); hit.ID() != "composer:cancel-reply" {
		t.Errorf("hit row 0 = %q, want composer:cancel-reply", hit.ID())
	}
}

func TestComposerLayerHuhViewReplacesLegacyDraftAndRetainsControls(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 20, 5
	model := composerModel(w, h, writable(1))
	model.Draft = "LEGACY_DRAFT"
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles, "HUH1\nHUH2")

	send := interactionByID(result, "composer:send")
	if send == nil {
		t.Fatal("no send interaction")
	}
	photo := interactionByID(result, "composer:photo")
	if photo == nil {
		t.Fatal("no photo interaction")
	}
	plain := plainText(compositor.Render())
	if !strings.Contains(plain, "HUH1") || !strings.Contains(plain, "HUH2") {
		t.Fatalf("injected Huh rows missing: %q", plain)
	}
	if strings.Contains(plain, model.Draft) {
		t.Fatalf("legacy draft rendered after cut-over: %q", plain)
	}
}

func TestComposerLayerHuhEmptyAndExplicitNewlineKeepTerminalCursorHidden(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 20, 5

	model := composerModel(w, h, writable(1))
	model.Focus = FocusComposer
	_, _, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)
	if result.Cursor.Visible || result.Cursor.X != -1 || result.Cursor.Y != -1 {
		t.Errorf("empty Huh view exposed terminal cursor: %+v", result.Cursor)
	}

	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles, "ab\ncd")
	plain := plainText(compositor.Render())
	if !strings.Contains(plain, "ab") || !strings.Contains(plain, "cd") {
		t.Fatalf("explicit Huh newline missing: %q", plain)
	}
	if result.Cursor.Visible || result.Cursor.X != -1 || result.Cursor.Y != -1 {
		t.Errorf("newline Huh view exposed terminal cursor: %+v", result.Cursor)
	}
}

func TestComposerLayerTerminalCursorHiddenFocusedAndUnfocused(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 30, 5

	unfocused := composerModel(w, h, writable(1))
	_, _, r1 := composerCanvas(unfocused, image.Rect(0, 0, w, h), styles, "hello")
	if r1.Cursor.Visible {
		t.Errorf("unfocused cursor visible, want hidden")
	}

	focused := composerModel(w, h, writable(1))
	focused.Focus = FocusComposer
	_, _, r2 := composerCanvas(focused, image.Rect(0, 0, w, h), styles, "hello")
	if r2.Cursor.Visible || r2.Cursor.X != -1 || r2.Cursor.Y != -1 {
		t.Errorf("focused composer exposed terminal cursor: %+v", r2.Cursor)
	}
}

func TestComposerLayerHuhViewRendersCJKZwjWithTerminalCursorHidden(t *testing.T) {
	styles := newRenderStyles(false)
	// Width 40 keeps the visible [Sticker]/[Photo]/Send controls while
	// leaving 15 draft columns, enough for the full CJK/ZWJ content.
	w, h := 40, 5
	model := composerModel(w, h, writable(1))
	model.Focus = FocusComposer
	text := "❤️界👨\u200d👩\u200d👧\u200d👦"
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles, text)
	if !strings.Contains(plainText(compositor.Render()), text) {
		t.Fatal("Huh CJK/ZWJ content missing")
	}
	if result.Cursor.Visible || result.Cursor.X != -1 || result.Cursor.Y != -1 {
		t.Fatalf("Huh composer exposed terminal cursor: %+v", result.Cursor)
	}
}

func TestComposerLayerInteractionIDsUnique(t *testing.T) {
	styles := newRenderStyles(false)
	model := composerModel(40, 8, writable(1))
	model.Focus = FocusComposer
	model.ReplyTarget = &ReplyTarget{ChatID: 1, Sender: "s", Preview: "p"}
	model.EditTarget = &EditTarget{ChatID: 1, Buffer: "b", Error: nil}
	model.Draft = "some draft text"
	_, compositor, result := composerCanvas(model, image.Rect(0, 0, 40, 8), styles)
	if err := assertUniqueNonEmptyIDs(result.Interactions); err != nil {
		t.Error(err)
	}
	// Representative hit parity: every non-empty interaction ID matches
	// Compositor.Hit at its own Min.
	for _, ls := range result.Interactions {
		if ls.Rect.Empty() {
			continue
		}
		pt := ls.Rect.Min
		hit := compositor.Hit(pt.X, pt.Y)
		if hit.Empty() {
			t.Errorf("interaction %q: no hit at %v", ls.ID, pt)
			continue
		}
		if hit.ID() != ls.ID {
			t.Errorf("interaction %q: hit ID = %q at %v, want matching", ls.ID, hit.ID(), pt)
		}
		if !hit.Bounds().Eq(ls.Rect) {
			t.Errorf("interaction %q: hit bounds %v != interaction rect %v", ls.ID, hit.Bounds(), ls.Rect)
		}
	}
}

func TestComposerLayerBoundsTinyDeterministic(t *testing.T) {
	styles := newRenderStyles(false)
	clusters := []string{"❤️", "👨\u200d👩\u200d👧\u200d👦", "火", "x"}
	draft := strings.Join(clusters, "")
	for width := 1; width <= 12; width++ {
		for height := 1; height <= 4; height++ {
			model := composerModel(width, height, writable(1))
			model.ReplyTarget = &ReplyTarget{ChatID: 1, Sender: "s", Preview: draft}
			model.EditTarget = &EditTarget{ChatID: 1, Buffer: "abc", Error: &domain.AppError{Message: "e"}}
			model.Draft = draft
			model.Focus = FocusComposer
			rect := image.Rect(0, 0, width, height)
			_, compositor, result := composerCanvas(model, rect, styles)
			if compositor == nil {
				t.Fatalf("%dx%d: nil compositor", width, height)
			}
			// The fixed root layer itself must not exceed the rect or escape it.
			if result.Layer == nil {
				t.Fatalf("%dx%d: nil layer", width, height)
			}
			if result.Layer.Width() > width || result.Layer.Height() > height {
				t.Fatalf("%dx%d: layer %dx%d exceeds rect", width, height, result.Layer.Width(), result.Layer.Height())
			}
			// Every rendered/layer bounds stays within the rect when intersected:
			// assert interaction rectangles never escape.
			for _, ls := range result.Interactions {
				if ls.Rect.Min.X < 0 || ls.Rect.Min.Y < 0 || ls.Rect.Max.X > width || ls.Rect.Max.Y > height {
					t.Fatalf("%dx%d: interaction %q rect %v escapes", width, height, ls.ID, ls.Rect)
				}
			}
			if result.Cursor.Visible {
				if result.Cursor.X < 0 || result.Cursor.Y < 0 || result.Cursor.X >= width || result.Cursor.Y >= height {
					t.Fatalf("%dx%d: cursor %+v outside rect", width, height, result.Cursor)
				}
			}
		}
	}
}

func TestComposerLayerPhoto(t *testing.T) {
	styles := newRenderStyles(false)

	// 1: Exact normal rect and action/Z/Hit.
	t.Run("normalRect", func(t *testing.T) {
		w, h := 30, 5
		model := composerModel(w, h, writable(1))

		_, compositor, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

		send := interactionByID(result, "composer:send")
		if send == nil {
			t.Fatal("no send interaction")
		}
		photo := interactionByID(result, "composer:photo")
		if photo == nil {
			t.Fatal("no photo interaction")
		}
		photoW := displayWidth(photoText)
		// Photo immediately left of send with one-cell gap.
		wantPhotoX := send.Rect.Min.X - photoW - 1
		wantPhoto := image.Rect(wantPhotoX, h-1, wantPhotoX+photoW, h)
		if !photo.Rect.Eq(wantPhoto) {
			t.Errorf("photo rect = %v, want %v", photo.Rect, wantPhoto)
		}
		// One-cell gap: photo.Max.X + 1 == send.Min.X.
		if photo.Rect.Max.X+1 != send.Rect.Min.X {
			t.Errorf("photo-send gap = %d, want 1 cell gap", send.Rect.Min.X-photo.Rect.Max.X)
		}
		if photo.Z != zControl {
			t.Errorf("photo Z = %d, want zControl", photo.Z)
		}
		if photo.Click.Action != OpenPhotoSend {
			t.Errorf("photo click = %+v, want OpenPhotoSend", photo.Click)
		}
		// Hit parity.
		if hit := compositor.Hit(photo.Rect.Min.X, h-1); hit.ID() != "composer:photo" {
			t.Errorf("photo hit = %q, want composer:photo", hit.ID())
		}
		// Send unchanged.
		if send.Click.Action != ComposerSubmit {
			t.Errorf("send action changed: %v", send.Click.Action)
		}
	})

	// 2: Absent in read-only and no-chat.
	t.Run("noPhotoReadOnlyNoChat", func(t *testing.T) {
		// Read-only.
		model := composerModel(60, 5, readOnly(7))
		model.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x")}
		_, _, result := composerCanvas(model, image.Rect(0, 0, 60, 5), styles)
		if interactionByID(result, "composer:photo") != nil {
			t.Error("read-only has photo")
		}
		if interactionByID(result, "composer:send") != nil {
			t.Error("read-only has send")
		}

		// No active chat.
		model2 := composerModel(60, 5, domain.Chat{})
		model2.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x")}
		_, _, result2 := composerCanvas(model2, image.Rect(0, 0, 60, 5), styles)
		if interactionByID(result2, "composer:photo") != nil {
			t.Error("no chat has photo")
		}
	})

	// 3: Tiny widths bounded/no overlap/deterministic.
	t.Run("tinyNoOverlap", func(t *testing.T) {
		for _, rect := range []image.Rectangle{
			image.Rect(0, 0, 1, 1),
			image.Rect(0, 0, 2, 1),
			image.Rect(0, 0, 9, 1),
			image.Rect(0, 0, 14, 1),
		} {
			model := composerModel(12, 4, writable(1))
			model.PhotoSend = &PhotoSendState{ChatID: 1, Input: []rune("/tmp/x")}
			_, _, res := composerCanvas(model, rect, styles)
			if res.Layer == nil {
				continue
			}
			// Layer never escapes.
			if res.Layer.Width() > rect.Dx() || res.Layer.Height() > rect.Dy() {
				t.Errorf("rect %v: layer exceeds", rect)
			}
			// No overlap between photo and send.
			photo := interactionByID(res, "composer:photo")
			send := interactionByID(res, "composer:send")
			if photo != nil && send != nil {
				if !photo.Rect.Empty() && !send.Rect.Empty() {
					if r := photo.Rect.Overlaps(send.Rect); r {
						t.Errorf("rect %v: photo %v overlaps send %v", rect, photo.Rect, send.Rect)
					}
				}
			}
		}
	})

	// 4: Draft/cursor stops before photo.
	t.Run("draftStopsBeforePhoto", func(t *testing.T) {
		w, h := 40, 5
		model := composerModel(w, h, writable(1))

		model.Focus = FocusComposer
		model.Draft = "hello world"
		_, _, result := composerCanvas(model, image.Rect(0, 0, w, h), styles)

		photo := interactionByID(result, "composer:photo")
		if photo == nil {
			t.Fatal("no photo interaction")
		}
		if result.Cursor.Visible {
			// Cursor must be before photo.
			if result.Cursor.X >= photo.Rect.Min.X {
				t.Errorf("cursor X = %d >= photo min X = %d", result.Cursor.X, photo.Rect.Min.X)
			}
		}
	})

	// 5: Deterministic output.
	t.Run("deterministic", func(t *testing.T) {
		model := composerModel(40, 5, writable(1))

		_, _, r1 := composerCanvas(model, image.Rect(0, 0, 40, 5), styles)
		_, _, r2 := composerCanvas(model, image.Rect(0, 0, 40, 5), styles)
		if len(r1.Interactions) != len(r2.Interactions) {
			t.Fatalf("non-deterministic interaction count: %d vs %d", len(r1.Interactions), len(r2.Interactions))
		}
	})

	// 6: Nonzero-origin rect — verifies photo geometry uses local coords so
	// draftWidth stays bounded by the rect, not inflated by the absolute
	// viewport offset. With rect.Min.X=15 the old code set draftWidth
	// to ~40 (absolute) instead of ~23 (local), expanding rows beyond
	// rect.Dx().
	t.Run("nonzeroOrigin", func(t *testing.T) {
		// Rect at (15,7) with width=45, height=12 — origin-offset to catch
		// local-vs-absolute bugs. Model must be wide/tall so the rect is not
		// clipped by the viewport.
		rect := image.Rect(15, 7, 60, 19)
		model := composerModel(100, 40, writable(1))
		model.Focus = FocusComposer
		model.Draft = "Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor"
		_, compositor, result := composerCanvas(model, rect, styles)

		// result.Rect must be the exact nonzero rect.
		if !result.Rect.Eq(rect) {
			t.Fatalf("result.Rect = %v, want %v", result.Rect, rect)
		}

		// Root layer must not exceed rect dimensions.
		if result.Layer.Width() > rect.Dx() || result.Layer.Height() > rect.Dy() {
			t.Fatalf("root layer %dx%d exceeds rect %dx%d",
				result.Layer.Width(), result.Layer.Height(), rect.Dx(), rect.Dy())
		}

		// Compositor bounds must not exceed the model viewport.
		bounds := compositor.Bounds()
		if bounds.Max.X > model.Width || bounds.Max.Y > model.Height {
			t.Fatalf("compositor bounds %v exceed viewport %dx%d", bounds, model.Width, model.Height)
		}

		// Photo and Send must be absolute with exact one-cell gap.
		photo := interactionByID(result, "composer:photo")
		send := interactionByID(result, "composer:send")
		if photo == nil {
			t.Fatal("no photo interaction")
		}
		if send == nil {
			t.Fatal("no send interaction")
		}
		if photo.Rect.Min.X >= send.Rect.Min.X {
			t.Fatalf("photo absolute Min.X %d must be < send Min.X %d",
				photo.Rect.Min.X, send.Rect.Min.X)
		}
		if photo.Rect.Max.X+1 != send.Rect.Min.X {
			t.Errorf("photo-send gap = %d, want 1", send.Rect.Min.X-photo.Rect.Max.X)
		}

		// Cursor must stop before absolute Photo Min.X.
		if result.Cursor.Visible {
			if result.Cursor.X >= photo.Rect.Min.X {
				t.Errorf("cursor X = %d >= photo absolute Min.X = %d",
					result.Cursor.X, photo.Rect.Min.X)
			}
		}

		// Determinism.
		_, _, r2 := composerCanvas(model, rect, styles)
		if len(result.Interactions) != len(r2.Interactions) {
			t.Fatalf("non-deterministic: %d vs %d interactions", len(result.Interactions), len(r2.Interactions))
		}
	})
}

func TestComposerLayerClosedTopicIsReadOnly(t *testing.T) {
	styles := newRenderStyles(false)
	w, h := 20, 5
	chat := writable(1)
	model := composerModel(w, h, chat)
	model.ActiveTopic = domain.ForumTopic{ID: 2, Name: "Announcements", IsClosed: true}
	model.ActiveTopicKnown = true
	model.Draft = "draft"
	rect := image.Rect(0, 0, w, h)

	canvas, _, result := composerCanvas(model, rect, styles)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Read-only") {
		t.Fatalf("closed topic composer missing Read-only: %q", text)
	}
	for _, id := range []string{"composer:send", "composer:photo", "composer:sticker"} {
		if hasInteraction(result, id) {
			t.Fatalf("closed topic composer has control %q", id)
		}
	}
	if result.Cursor.Visible {
		t.Fatalf("closed topic composer cursor visible")
	}

	// A writable chat with an open (non-closed) topic stays writable.
	model2 := composerModel(w, h, chat)
	model2.ActiveTopic = domain.ForumTopic{ID: 2, Name: "Announcements"}
	model2.ActiveTopicKnown = true
	_, _, result2 := composerCanvas(model2, rect, styles)
	if !hasInteraction(result2, "composer:send") {
		t.Fatalf("open topic composer lost send control")
	}
}
