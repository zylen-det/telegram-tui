package frontend

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// conversationSurface composes a conversation root returned by
// buildConversationLayer under one full-size viewport root/compositor/canvas.
func conversationSurface(model ui.ViewModel, styles renderStyles, composerView ...string) (*lipgloss.Canvas, *lipgloss.Compositor, surfaceResult) {
	result := buildConversationLayer(model, time.Local, styles, composerView...)
	root := lipgloss.NewLayer(renderEmptyBox(styles.Base, model.Width, model.Height)).X(0).Y(0).Z(zFrame)
	if result.Layer != nil {
		root.AddLayers(result.Layer)
	}
	compositor := lipgloss.NewCompositor(root)
	canvas := lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor)
	return canvas, compositor, result
}

// conversationModel builds a wide/normal layout viewmodel with an active chat.
func conversationModel(width, height int, active domain.Chat) ui.ViewModel {
	return ui.ViewModel{
		Width:      width,
		Height:     height,
		Layout:     ui.ComputeLayout(width, height, false, app.FocusConversation),
		Focus:      app.FocusConversation,
		ActiveChat: active,
	}
}

func TestConversationLayerEmptyOutOfViewportAndTiny(t *testing.T) {
	styles := newRenderStyles(false)

	// Empty layout conversation rect (zero bounds).
	model := conversationModel(100, 30, writable(1))
	model.Layout.Conversation = image.Rect(0, 0, 0, 0)
	result := buildConversationLayer(model, time.Local, styles)
	if result.Layer != nil {
		t.Fatalf("empty layout produced a layer")
	}
	if result.Cursor.Visible || result.Cursor.X != -1 || result.Cursor.Y != -1 {
		t.Fatalf("empty layout cursor = %+v, want hidden (-1,-1)", result.Cursor)
	}
	if len(result.Interactions) != 0 {
		t.Fatalf("empty layout produced %d interactions", len(result.Interactions))
	}

	// Tiny pane below buildPane minimum (1x2).
	model = conversationModel(100, 30, writable(1))
	model.Layout.Conversation = image.Rect(32, 1, 33, 3)
	tiny := buildConversationLayer(model, time.Local, styles)
	if tiny.Layer != nil {
		t.Fatalf("tiny pane produced a layer")
	}
	if tiny.Cursor.Visible {
		t.Fatalf("tiny pane cursor visible, want hidden")
	}

	// Out-of-viewport conversation rect.
	model = conversationModel(100, 30, writable(1))
	model.Layout.Conversation = image.Rect(120, 40, 180, 50)
	out := buildConversationLayer(model, time.Local, styles)
	if out.Layer != nil {
		t.Fatalf("out-of-viewport rect produced a layer")
	}
	if out.Cursor.Visible {
		t.Fatalf("out-of-viewport cursor visible, want hidden")
	}
}

func TestConversationLayerExactRectRootIDTitleAndFocus(t *testing.T) {
	styles := newRenderStyles(false)
	for _, tc := range []struct {
		name    string
		focus   app.Focus
		wantFoc bool
	}{
		{"conversation-focus", app.FocusConversation, true},
		{"composer-focus", app.FocusComposer, true},
		{"details-focus", app.FocusDetails, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := conversationModel(120, 30, writable(1))
			model.Focus = tc.focus
			model.Layout = ui.ComputeLayout(model.Width, model.Height, false, tc.focus)
			surface := buildConversationLayer(model, time.Local, styles)

			wantRect := model.Layout.Conversation
			if !surface.Rect.Eq(wantRect) {
				t.Fatalf("rect = %v, want %v", surface.Rect, wantRect)
			}
			if surface.Layer == nil || surface.Layer.GetID() != "pane:conversation" {
				t.Fatalf("root ID = %q, want pane:conversation", surface.Layer.GetID())
			}

			var paneHit *layerInteraction
			for i := range surface.Interactions {
				if surface.Interactions[i].ID == "pane:conversation" {
					paneHit = &surface.Interactions[i]
				}
			}
			if paneHit == nil {
				t.Fatal("no pane interaction")
			}
			if paneHit.Click.Action != app.FocusPane || paneHit.Click.TargetFocus != app.FocusConversation {
				t.Errorf("pane click = %#v", paneHit.Click)
			}
			if !paneHit.Rect.Eq(wantRect) {
				t.Errorf("pane hit rect = %v, want %v", paneHit.Rect, wantRect)
			}

			// Focused border color on the top border.
			canvas, _, _ := conversationSurface(model, styles)
			got := cellColor(canvas, wantRect.Min.X+1, wantRect.Min.Y)
			want := borderColor
			if tc.wantFoc {
				want = focusedBorderColor
			}
			if got != rgba(want) {
				t.Errorf("top border color = %v, want %v", got, rgba(want))
			}
		})
	}
}

func TestConversationLayerTitleFallback(t *testing.T) {
	styles := newRenderStyles(false)

	chat := writable(1)
	chat.Title = ""
	model := conversationModel(120, 30, chat)
	surface := buildConversationLayer(model, time.Local, styles)
	if !strings.Contains(plainTitleRow(surface), "Conversation") {
		t.Errorf("fallback title missing")
	}

	chat.Title = "Dev Team"
	model = conversationModel(120, 30, chat)
	surface = buildConversationLayer(model, time.Local, styles)
	if !strings.Contains(plainTitleRow(surface), "Dev Team") {
		t.Errorf("title missing")
	}
}

// plainTitleRow renders the pane root (composed under a full-size frame root)
// and strips ANSI to read text placed intrinsically on its top border row.
func plainTitleRow(surface surfaceResult) string {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(surface.Rect.Max.X).Height(surface.Rect.Max.Y).Render("")).X(0).Y(0).Z(zFrame)
	if surface.Layer != nil {
		root.AddLayers(surface.Layer)
	}
	rows := strings.Split(plainText(lipgloss.NewCompositor(root).Render()), "\n")
	if surface.Rect.Min.Y >= 0 && surface.Rect.Min.Y < len(rows) {
		return rows[surface.Rect.Min.Y]
	}
	return ""
}

func TestConversationLayerInfoGeometryAndWins(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	canvas, compositor, result := conversationSurface(model, styles)

	info := interactionByID(result, "conversation:info")
	if info == nil {
		t.Fatal("no info interaction")
	}
	rect := model.Layout.Conversation
	wantAbs := image.Rect(rect.Dx()-2, 0, rect.Dx()-1, 1).Add(rect.Min)
	if !info.Rect.Eq(wantAbs) {
		t.Fatalf("info rect = %v, want %v", info.Rect, wantAbs)
	}
	if info.Click.Action != app.ToggleDetails {
		t.Errorf("info click = %+v, want ToggleDetails", info.Click)
	}
	if info.Z != zControl {
		t.Errorf("info Z = %d, want zControl", info.Z)
	}

	hit := compositor.Hit(wantAbs.Min.X, wantAbs.Min.Y)
	if hit.ID() != "conversation:info" {
		t.Errorf("Hit(%v) = %q, want conversation:info", wantAbs.Min, hit.ID())
	}
	if !hit.Bounds().Eq(info.Rect) {
		t.Errorf("info hit bounds = %v, want %v", hit.Bounds(), info.Rect)
	}

	// Info wins over pane on the top border; adjacent cell hits pane.
	for x := wantAbs.Min.X; x < wantAbs.Max.X; x++ {
		if h := compositor.Hit(x, wantAbs.Min.Y); h.ID() != "conversation:info" {
			t.Errorf("info cell (%d,%d) hit %q", x, wantAbs.Min.Y, h.ID())
		}
	}
	adj := image.Pt(wantAbs.Min.X-1, wantAbs.Min.Y)
	if h := compositor.Hit(adj.X, adj.Y); h.ID() != "pane:conversation" {
		t.Errorf("adjacent top-border hit = %q, want pane:conversation", h.ID())
	}

	if got := canvas.CellAt(wantAbs.Min.X, wantAbs.Min.Y); got == nil || got.Content != "ⓘ" {
		t.Errorf("info cell content = %q, want ⓘ", cellContent(canvas, wantAbs.Min.X, wantAbs.Min.Y))
	}
}

func TestConversationLayerInfoOmitted(t *testing.T) {
	styles := newRenderStyles(false)

	// No active chat.
	model := conversationModel(120, 30, domain.Chat{})
	model.ActiveChat = domain.Chat{ID: 0}
	surface := buildConversationLayer(model, time.Local, styles)
	if interactionByID(surface, "conversation:info") != nil {
		t.Errorf("info present with no active chat")
	}

	// Details open.
	model = conversationModel(120, 30, writable(1))
	model.DetailsOpen = true
	model.Layout = ui.ComputeLayout(model.Width, model.Height, true, app.FocusConversation)
	surface = buildConversationLayer(model, time.Local, styles)
	if interactionByID(surface, "conversation:info") != nil {
		t.Errorf("info present with details open")
	}

	// Insufficient width (rect.Dx() < 3).
	model = conversationModel(120, 30, writable(1))
	model.Layout.Conversation = image.Rect(32, 1, 34, 30) // 2 wide
	surface = buildConversationLayer(model, time.Local, styles)
	if interactionByID(surface, "conversation:info") != nil {
		t.Errorf("info present with width 2")
	}
}

func TestConversationLayerInnerHistoryComposerSplit(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	model.HistoryDone = true
	_, _, result := conversationSurface(model, styles)

	rect := model.Layout.Conversation
	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	wantComposer := image.Rect(inner.Min.X, composerTop, inner.Max.X, inner.Max.Y)
	wantHistory := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, composerTop)

	histLayer := result.Layer.GetLayer("conversation:history")
	compLayer := result.Layer.GetLayer("composer")
	if histLayer == nil {
		t.Error("history root not nested under pane")
	}
	if compLayer == nil {
		t.Error("composer root not nested under pane")
	}

	// Pane-local X/Y equal absolute child Min minus pane Min.
	if histLayer != nil {
		if histLayer.GetX() != wantHistory.Min.X-rect.Min.X || histLayer.GetY() != wantHistory.Min.Y-rect.Min.Y {
			t.Errorf("history local pos = (%d,%d), want (%d,%d)",
				histLayer.GetX(), histLayer.GetY(), wantHistory.Min.X-rect.Min.X, wantHistory.Min.Y-rect.Min.Y)
		}
	}
	if compLayer != nil {
		if compLayer.GetX() != wantComposer.Min.X-rect.Min.X || compLayer.GetY() != wantComposer.Min.Y-rect.Min.Y {
			t.Errorf("composer local pos = (%d,%d), want (%d,%d)",
				compLayer.GetX(), compLayer.GetY(), wantComposer.Min.X-rect.Min.X, wantComposer.Min.Y-rect.Min.Y)
		}
	}

	// Absolute interaction rects, no double translation.
	histInteraction := interactionByID(result, "conversation:history")
	compInteraction := interactionByID(result, "composer")
	if histInteraction != nil && !histInteraction.Rect.Eq(wantHistory) {
		t.Errorf("history interaction rect = %v, want %v", histInteraction.Rect, wantHistory)
	}
	if compInteraction == nil {
		t.Error("no composer interaction for writable active chat")
	} else if !compInteraction.Rect.Eq(wantComposer) {
		t.Errorf("composer interaction rect = %v, want %v", compInteraction.Rect, wantComposer)
	}
}

func TestConversationLayerActiveHistoryComposerActionsAndNesting(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	model.HistoryDone = true
	_, _, result := conversationSurface(model, styles)

	history := interactionByID(result, "conversation:history")
	if history == nil {
		t.Fatal("no history interaction")
	}
	if history.Click.Action != app.FocusPane || history.Click.TargetFocus != app.FocusConversation {
		t.Errorf("history click = %#v", history.Click)
	}
	if history.WheelUp.Action != app.PageUp || history.WheelDown.Action != app.PageDown {
		t.Errorf("history wheels = %#v/%#v", history.WheelUp, history.WheelDown)
	}

	composer := interactionByID(result, "composer")
	if composer == nil {
		t.Fatal("no composer interaction")
	}
	if composer.Click.Action != app.FocusPane || composer.Click.TargetFocus != app.FocusComposer {
		t.Errorf("composer click = %#v", composer.Click)
	}
	send := interactionByID(result, "composer:send")
	if send == nil {
		t.Fatal("no send interaction for writable chat")
	}
	if send.Click.Action != app.ComposerSubmit {
		t.Errorf("send click = %+v, want ComposerSubmit", send.Click)
	}

	// Child layers are truly nested under the pane root (GetLayer walks the
	// hierarchy), not siblings or rendered strings.
	if result.Layer.GetLayer("conversation:history") == nil {
		t.Error("history not nested under pane root")
	}
	if result.Layer.GetLayer("composer") == nil {
		t.Error("composer not nested under pane root")
	}
}

func TestConversationLayerInteractionsAbsoluteHitParityUnique(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	model.HistoryDone = true
	model.Focus = app.FocusComposer
	model.ReplyTarget = &app.ReplyTarget{ChatID: 1, Sender: "s", Preview: "p"}
	model.Draft = "hello draft"
	_, compositor, result := conversationSurface(model, styles)

	if err := assertUniqueNonEmptyIDs(result.Interactions); err != nil {
		t.Error(err)
	}

	rect := model.Layout.Conversation
	for _, ls := range result.Interactions {
		if ls.Rect.Empty() || ls.Virtual {
			continue
		}
		if ls.Rect.Min.X < rect.Min.X || ls.Rect.Min.Y < rect.Min.Y {
			t.Errorf("interaction %q rect %v is not absolute", ls.ID, ls.Rect)
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
			t.Errorf("interaction %q: hit bounds %v != rect %v", ls.ID, hit.Bounds(), ls.Rect)
		}
	}
}

func TestConversationLayerInteractionsWinOverPaneInAreas(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	model.HistoryDone = true
	_, compositor, result := conversationSurface(model, styles)

	rect := model.Layout.Conversation
	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	composerTop := max(inner.Min.Y, inner.Max.Y-3)

	hist := interactionByID(result, "conversation:history")
	if h := compositor.Hit(hist.Rect.Min.X, hist.Rect.Min.Y); h.ID() != "conversation:history" {
		t.Errorf("history area hit = %q, want conversation:history", h.ID())
	}
	comp := interactionByID(result, "composer")
	if h := compositor.Hit(comp.Rect.Min.X, comp.Rect.Min.Y); h.ID() != "composer" {
		t.Errorf("composer area hit = %q, want composer", h.ID())
	}
	// Top border belongs to pane except the info cell.
	for x := rect.Min.X + 1; x < rect.Max.X-1; x++ {
		h := compositor.Hit(x, rect.Min.Y)
		if h.ID() == "conversation:info" {
			continue
		}
		if h.ID() != "pane:conversation" {
			t.Errorf("top border cell (%d,%d) hit %q, want pane:conversation", x, rect.Min.Y, h.ID())
		}
	}
	// Composer top boundary line belongs to composer.
	if h := compositor.Hit(rect.Min.X+1, composerTop); h.ID() != "composer" {
		t.Errorf("composer top boundary hit = %q, want composer", h.ID())
	}
}

func TestConversationLayerNoActiveCenteredPlaceholder(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, domain.Chat{})
	model.ActiveChat = domain.Chat{ID: 0}
	canvas, compositor, result := conversationSurface(model, styles)

	for _, id := range []string{"conversation:history", "composer", "composer:send", "conversation:info"} {
		if interactionByID(result, id) != nil {
			t.Errorf("%s interaction present without active chat", id)
		}
	}
	if result.Cursor.Visible {
		t.Error("hidden cursor expected without active chat")
	}

	rect := model.Layout.Conversation
	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	historyRect := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, composerTop)

	textWidth := displayWidth("No conversation")
	xAbs := historyRect.Min.X + centeredX(historyRect.Dx(), textWidth)
	yAbs := historyRect.Min.Y + historyRect.Dy()/2
	if got := canvas.CellAt(xAbs, yAbs); got == nil || got.Content != "N" {
		t.Errorf("centered placeholder cell = %q, want 'N'", cellContent(canvas, xAbs, yAbs))
	}

	// Inactive composer background still renders.
	composerRect := image.Rect(inner.Min.X, composerTop, inner.Max.X, inner.Max.Y)
	if canvas.CellAt(composerRect.Min.X+1, composerRect.Min.Y+1) == nil {
		t.Fatal("no inactive composer background cell")
	}

	// Placeholder region has no history interaction; pane wins.
	if h := compositor.Hit(xAbs, yAbs); h.ID() != "pane:conversation" {
		t.Errorf("placeholder hit = %q, want pane:conversation", h.ID())
	}
}

func TestConversationLayerReadOnlyActive(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, readOnly(1))
	model.HistoryDone = true
	canvas, _, result := conversationSurface(model, styles)

	if result.Layer.GetLayer("conversation:history") == nil {
		t.Error("read-only history missing")
	}
	inner := image.Rect(model.Layout.Conversation.Min.X+1, model.Layout.Conversation.Min.Y+1, model.Layout.Conversation.Max.X-1, model.Layout.Conversation.Max.Y-1)
	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	composerRect := image.Rect(inner.Min.X, composerTop, inner.Max.X, inner.Max.Y)
	if canvas.CellAt(composerRect.Min.X+1, composerRect.Min.Y+1) == nil {
		t.Error("read-only composer background cell missing")
	}
	for _, id := range []string{"composer", "composer:send"} {
		if interactionByID(result, id) != nil {
			t.Errorf("%s interaction present in read-only chat", id)
		}
	}
	if result.Cursor.Visible {
		t.Error("cursor present in read-only chat")
	}

	row := composerRect.Min.Y + 1
	if !strings.Contains(strings.Join(renderRows(canvas, row), ""), "Read-only") {
		t.Error("read-only label missing on composer row")
	}
}

func renderRows(c *lipgloss.Canvas, y int) []string {
	var out []string
	for x := 0; x < c.Width(); x++ {
		if cell := c.CellAt(x, y); cell != nil {
			out = append(out, cell.Content)
		}
	}
	return out
}

func TestConversationLayerHuhComposerVisibleWithTerminalCursorHidden(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(120, 30, writable(1))
	model.Focus = app.FocusComposer
	canvas, _, result := conversationSurface(model, styles, "HUH_CONVERSATION")
	if !strings.Contains(plainText(canvas.Render()), "HUH_CONVERSATION") {
		t.Fatal("Huh composer content missing from conversation layer")
	}
	if result.Cursor.Visible || result.Cursor.X != -1 || result.Cursor.Y != -1 {
		t.Fatalf("conversation propagated composer terminal cursor: %+v", result.Cursor)
	}
	send := interactionByID(result, "composer:send")
	if send == nil {
		t.Fatal("no send interaction")
	}
}

func TestConversationLayerDetailsOpenKeepsPaneHistoryComposer(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(140, 30, writable(1))
	model.DetailsOpen = true
	model.HistoryDone = true
	model.Layout = ui.ComputeLayout(model.Width, model.Height, true, app.FocusConversation)
	_, _, result := conversationSurface(model, styles)

	if interactionByID(result, "conversation:info") != nil {
		t.Error("info interaction present with details open")
	}
	if result.Layer == nil || result.Layer.GetID() != "pane:conversation" {
		t.Errorf("pane root missing with details open: %q", result.Layer.GetID())
	}
	if result.Layer.GetLayer("conversation:history") == nil {
		t.Error("history missing with details open")
	}
	if result.Layer.GetLayer("composer") == nil {
		t.Error("composer missing with details open")
	}
}

func TestConversationLayerTopClippedMessageReachable(t *testing.T) {
	styles := newRenderStyles(false)
	model := conversationModel(60, 18, writable(1))
	model.HistoryDone = true
	long := testMessage(10, 7, strings.Repeat("word ", 300))
	topGroup := styledMessageGroup("Mina", false, long)
	bottomGroup := styledMessageGroup("Lou", false, testMessage(11, 7, "bottom"))
	model.Groups = []ui.RenderedMessageGroup{topGroup, bottomGroup}

	rect := model.Layout.Conversation
	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	composerTop := max(inner.Min.Y, inner.Max.Y-3)
	historyRect := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, composerTop)
	fullTop := buildMessageGroupLayer(topGroup, historyRect.Dx(), time.Local, messageSelection{}, nil, styles)
	fullBottom := buildMessageGroupLayer(bottomGroup, historyRect.Dx(), time.Local, messageSelection{}, nil, styles)
	expectedStartRow := fullTop.Height + 1 + fullBottom.Height - historyRect.Dy()
	if expectedStartRow <= 0 {
		t.Fatalf("clipping prerequisite unmet: top=%d separator=1 bottom=%d history=%d", fullTop.Height, fullBottom.Height, historyRect.Dy())
	}
	wantID := fmt.Sprintf("message:7:10:%d", expectedStartRow)

	_, compositor, result := conversationSurface(model, styles)

	var topInteraction *layerInteraction
	for i := range result.Interactions {
		if result.Interactions[i].ID == wantID {
			topInteraction = &result.Interactions[i]
			break
		}
	}
	if topInteraction == nil {
		t.Fatalf("no first visible top-clipped interaction %q", wantID)
	}
	if topInteraction.Rect.Min.Y != historyRect.Min.Y {
		t.Errorf("first visible clipped row Y=%d, want history top %d", topInteraction.Rect.Min.Y, historyRect.Min.Y)
	}
	hit := compositor.Hit(topInteraction.Rect.Min.X, topInteraction.Rect.Min.Y)
	if hit.ID() != topInteraction.ID {
		t.Errorf("top message hit = %q, want %q", hit.ID(), topInteraction.ID)
	}
	if !hit.Bounds().Eq(topInteraction.Rect) {
		t.Errorf("top message bounds = %v != rect %v", hit.Bounds(), topInteraction.Rect)
	}
}

func TestConversationLayerBoundsStayInPaneAndDoNotOverflow(t *testing.T) {
	styles := newRenderStyles(false)
	for _, tc := range []struct{ width, height int }{
		{120, 30}, // wide
		{80, 20},  // normal
		{60, 20},  // narrow minimum
	} {
		model := conversationModel(tc.width, tc.height, writable(1))
		model.HistoryDone = true
		canvas, compositor, result := conversationSurface(model, styles)

		if compositor == nil {
			t.Fatalf("%dx%d: nil compositor", tc.width, tc.height)
		}
		viewport := image.Rect(0, 0, tc.width, tc.height)
		if got := compositor.Bounds(); !got.Eq(viewport) {
			t.Errorf("%dx%d compositor bounds = %v, want %v", tc.width, tc.height, got, viewport)
		}
		for y, line := range strings.Split(compositor.Render(), "\n") {
			if got := displayWidth(line); got > tc.width {
				t.Errorf("%dx%d frame row %d width = %d, want <= %d", tc.width, tc.height, y, got, tc.width)
			}
		}
		rect := model.Layout.Conversation
		for _, ls := range result.Interactions {
			if ls.Rect.Empty() {
				continue
			}
			if ls.Rect.Min.X < rect.Min.X || ls.Rect.Min.Y < rect.Min.Y ||
				ls.Rect.Max.X > rect.Max.X || ls.Rect.Max.Y > rect.Max.Y {
				t.Errorf("%dx%d interaction %q rect %v escapes pane %v", tc.width, tc.height, ls.ID, ls.Rect, rect)
			}
		}
		// The pane occupies its exact rect; the corner cells of the pane exist.
		if !rect.Empty() && canvas != nil {
			if canvas.CellAt(rect.Min.X, rect.Min.Y) == nil {
				t.Errorf("%dx%d pane min cell nil", tc.width, tc.height)
			}
			if canvas.CellAt(rect.Max.X-1, rect.Max.Y-1) == nil {
				t.Errorf("%dx%d pane max cell nil", tc.width, tc.height)
			}
		}
	}
}

func cellColor(c *lipgloss.Canvas, x, y int) color.RGBA {
	if cell := c.CellAt(x, y); cell != nil && cell.Style.Fg != nil {
		return colorOf(cell.Style.Fg)
	}
	return color.RGBA{}
}

func TestConversationLayerActiveTopicTitle(t *testing.T) {
	styles := newRenderStyles(false)

	chat := writable(1)
	chat.Title = "Dev Team"
	model := conversationModel(120, 30, chat)
	surface := buildConversationLayer(model, time.Local, styles)
	if strings.Contains(plainTitleRow(surface), "›") {
		t.Errorf("title without active topic leaked separator: %q", plainTitleRow(surface))
	}

	model.ActiveTopic = domain.ForumTopic{ID: 2, Name: "Announcements"}
	model.ActiveTopicKnown = true
	surface = buildConversationLayer(model, time.Local, styles)
	titleRow := plainTitleRow(surface)
	if !strings.Contains(titleRow, "Dev Team") || !strings.Contains(titleRow, "›") || !strings.Contains(titleRow, "Announcements") {
		t.Errorf("active topic title missing: %q", titleRow)
	}
}
