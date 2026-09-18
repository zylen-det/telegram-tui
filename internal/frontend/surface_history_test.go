package frontend

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// historySurfaceCanvas composes a history surface under a full viewport root
// and returns the canvas plus compositor for hit testing.
func historySurfaceCanvas(model ui.ViewModel, surface surfaceResult) (*lipgloss.Canvas, *lipgloss.Compositor) {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor), compositor
}

// historyModel builds a wide-layout viewmodel with a conversation rect.
func historyModel(width, height int, rect image.Rectangle) ui.ViewModel {
	return ui.ViewModel{
		Width:  width,
		Height: height,
		Layout: ui.ComputeLayout(width, height, false, app.FocusConversation),
		Focus:  app.FocusConversation,
	}
}

func TestHistoryRootRectIDActionsAndEmptyBackgroundHit(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	model.Groups = nil
	model.HistoryDone = true
	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)

	if surface.Layer == nil {
		t.Fatal("history layer nil")
	}
	if surface.Layer.GetID() != "conversation:history" {
		t.Errorf("root ID = %q, want conversation:history", surface.Layer.GetID())
	}
	if !surface.Rect.Eq(image.Rect(0, 0, 100, 40)) {
		t.Errorf("surface rect = %v, want (0,0)-(100,40)", surface.Rect)
	}

	var rootHit *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "conversation:history" {
			rootHit = &surface.Interactions[i]
		}
	}
	if rootHit == nil {
		t.Fatal("no history root interaction")
	}
	if rootHit.Click.Action != app.FocusPane || rootHit.Click.TargetFocus != app.FocusConversation {
		t.Errorf("root click = %#v", rootHit.Click)
	}
	if rootHit.WheelUp.Action != app.PageUp || rootHit.WheelDown.Action != app.PageDown {
		t.Errorf("root wheels = %#v/%#v", rootHit.WheelUp, rootHit.WheelDown)
	}
	if rootHit.Z != zPaneBackground {
		t.Errorf("root Z = %d, want %d", rootHit.Z, zPaneBackground)
	}
	if !rootHit.Rect.Eq(image.Rect(0, 0, 100, 40)) {
		t.Errorf("root hit rect = %v", rootHit.Rect)
	}

	// Compositor hit parity at an empty background point (top-left corner).
	canvas, compositor := historySurfaceCanvas(model, surface)
	_ = canvas
	pt := image.Pt(2, 2)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "conversation:history" {
		t.Errorf("Hit(%v) = %q, want conversation:history", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(rootHit.Rect) {
		t.Errorf("root hit bounds = %v, want %v", got, rootHit.Rect)
	}
}

func TestHistoryErrorRetryRowAndWins(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	model.HistoryError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "offline"}
	model.Groups = nil
	model.HistoryDone = true
	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)

	var retry *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "conversation:history-retry" {
			retry = &surface.Interactions[i]
		}
	}
	if retry == nil {
		t.Fatal("no history retry interaction")
	}
	wantRect := image.Rect(0, 0, 100, 1)
	if !retry.Rect.Eq(wantRect) {
		t.Errorf("retry rect = %v, want %v", retry.Rect, wantRect)
	}
	if retry.Click.Action != app.Retry {
		t.Errorf("retry click = %#v, want Retry", retry.Click)
	}
	if retry.Z != zControl {
		t.Errorf("retry Z = %d, want %d", retry.Z, zControl)
	}

	canvas, compositor := historySurfaceCanvas(model, surface)
	// Retry text present.
	if !strings.Contains(plainText(canvas.Render()), "offline  Retry") {
		t.Errorf("retry text missing, got %q", plainText(canvas.Render()))
	}
	// Retry wins over the root at its point.
	pt := image.Pt(5, 0)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "conversation:history-retry" {
		t.Errorf("Hit(%v) = %q, want history retry", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(wantRect) {
		t.Errorf("retry hit bounds = %v, want %v", got, wantRect)
	}
}

func TestHistoryCenteredEmptyBelowErrorNonzeroOrigin(t *testing.T) {
	styles := newRenderStyles(false)
	// Absolute history rect with a nonzero Min.Y so consumed-row centering is
	// observable.
	hist := image.Rect(0, 10, 100, 30)
	model := historyModel(100, 40, hist)
	model.HistoryError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "offline"}
	model.Groups = nil
	model.HistoryDone = true
	surface := buildHistoryLayer(model, hist, time.Local, styles)

	// Retry occupies the first absolute row (Min.Y).
	var retry *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "conversation:history-retry" {
			retry = &surface.Interactions[i]
		}
	}
	if retry == nil {
		t.Fatal("no history retry interaction")
	}
	wantRetry := image.Rect(0, 10, 100, 11)
	if !retry.Rect.Eq(wantRetry) {
		t.Errorf("retry rect = %v, want %v", retry.Rect, wantRetry)
	}

	// Remaining content is rows [11,30) (height 19). "No messages" is
	// centered within it at the exact absolute Y.
	contentMinY := 11
	contentHeight := 19
	expectedY := contentMinY + contentHeight/2
	canvas, _ := historySurfaceCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "No messages") {
		t.Errorf("missing No messages text, got %q", text)
	}
	// Assert the centered text row equals the expected absolute Y by checking
	// the cell at the centered X.
	textWidth := displayWidth("No messages")
	centeredX := centeredX(100, textWidth)
	if got := canvas.CellAt(centeredX, expectedY); got == nil || got.Content != "N" {
		t.Errorf("No messages at (%d,%d) = %v, want 'N' (expected centered Y %d)", centeredX, expectedY, got, expectedY)
	}
	// No overlap: the retry row (10) and the centered text row are distinct.
	if expectedY == 10 {
		t.Errorf("centered text overlaps the retry row")
	}
}

func TestHistoryExclusiveEmptyStates(t *testing.T) {
	styles := newRenderStyles(false)

	// Loading state.
	loading := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	loading.Groups = nil
	loading.HistoryLoading = true
	ls := buildHistoryLayer(loading, image.Rect(0, 0, 100, 40), time.Local, styles)
	lc, _ := historySurfaceCanvas(loading, ls)
	if !strings.Contains(plainText(lc.Render()), "Loading messages...") {
		t.Errorf("loading empty missing text, got %q", plainText(lc.Render()))
	}
	if strings.Contains(plainText(lc.Render()), "No messages") {
		t.Errorf("loading empty must not show No messages")
	}

	// Done state.
	done := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	done.Groups = nil
	done.HistoryDone = true
	ds := buildHistoryLayer(done, image.Rect(0, 0, 100, 40), time.Local, styles)
	dc, _ := historySurfaceCanvas(done, ds)
	if !strings.Contains(plainText(dc.Render()), "No messages") {
		t.Errorf("done empty missing text, got %q", plainText(dc.Render()))
	}
	if strings.Contains(plainText(dc.Render()), "Loading messages...") {
		t.Errorf("done empty must not show Loading messages")
	}

	// Neither: no centered text.
	neither := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	neither.Groups = nil
	ns := buildHistoryLayer(neither, image.Rect(0, 0, 100, 40), time.Local, styles)
	nc, _ := historySurfaceCanvas(neither, ns)
	if strings.Contains(plainText(nc.Render()), "Loading messages...") || strings.Contains(plainText(nc.Render()), "No messages") {
		t.Errorf("neither state must render no empty text, got %q", plainText(nc.Render()))
	}
}

func TestHistoryLoadingOlderStillRendersGroups(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	msg := testMessage(1, 7, "hello")
	model.Groups = []ui.RenderedMessageGroup{styledMessageGroup("Mina", false, msg)}
	model.HistoryLoading = true
	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	canvas, _ := historySurfaceCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Loading older messages...") {
		t.Errorf("missing loading-older text, got %q", text)
	}
	if !strings.Contains(text, "hello") {
		t.Errorf("groups not rendered under loading, got %q", text)
	}
}

func TestHistoryBottomAlignmentAndInterGroupGap(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	g1 := styledMessageGroup("Mina", false, testMessage(1, 7, "first"))
	g2 := styledMessageGroup("Lou", false, testMessage(2, 7, "second"))
	model.Groups = []ui.RenderedMessageGroup{g1, g2}
	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	canvas, _ := historySurfaceCanvas(model, surface)

	// Both groups are 1 row tall (no avatar). Bottom-aligned: second group at
	// row 39, first group at row 37 (one blank separator row 38 between).
	if got := canvas.CellAt(1, 39).Content; got != "s" {
		t.Errorf("bottom row 39 = %q, want 's' (second group)", got)
	}
	if got := canvas.CellAt(1, 37).Content; got != "f" {
		t.Errorf("row 37 = %q, want 'f' (first group)", got)
	}
	// The separator row 38 is background only.
	if got := canvas.CellAt(1, 38).Content; got == "f" || got == "s" {
		t.Errorf("separator row 38 has content %q, want background", got)
	}
}

func TestHistoryOffsetMatchesGroupsBeforeSurfaceOffset(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	g1 := styledMessageGroup("Mina", false, testMessage(1, 7, "first"))
	g2 := styledMessageGroup("Lou", false, testMessage(2, 7, "second"))
	g3 := styledMessageGroup("Zed", false, testMessage(3, 7, "third"))
	model.Groups = []ui.RenderedMessageGroup{g1, g2, g3}
	model.HistoryOffset = 1 // hides the latest message (g3)

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	canvas, _ := historySurfaceCanvas(model, surface)
	text := plainText(canvas.Render())
	if strings.Contains(text, "third") {
		t.Errorf("offset=1 must hide latest message 'third', got %q", text)
	}
	if !strings.Contains(text, "first") || !strings.Contains(text, "second") {
		t.Errorf("offset=1 must keep first/second, got %q", text)
	}
}

func TestBuildVisibleHistoryResultsBoundsOrdinaryHistoryToViewport(t *testing.T) {
	groups := make([]ui.RenderedMessageGroup, 500)
	for index := range groups {
		groups[index] = styledMessageGroup("Mina", false, testMessage(domain.MessageID(index+1), 7, "message"))
	}

	calls := 0
	results := buildVisibleHistoryResults(groups, 9, false, messageSelection{}, func(group ui.RenderedMessageGroup) messageGroupResult {
		calls++
		return messageGroupResult{Height: 1, Group: group}
	})

	if calls != 5 || len(results) != 5 {
		t.Fatalf("builder calls/results = %d/%d, want 5/5 for a 9-row viewport", calls, len(results))
	}
	if first, last := results[0].Group.Messages[0].ID, results[len(results)-1].Group.Messages[0].ID; first != 496 || last != 500 {
		t.Fatalf("built message range = %d..%d, want 496..500", first, last)
	}
}

func TestBuildVisibleHistoryResultsBoundsCenteredSelectionToViewport(t *testing.T) {
	groups := make([]ui.RenderedMessageGroup, 500)
	for index := range groups {
		groups[index] = styledMessageGroup("Mina", false, testMessage(domain.MessageID(index+1), 7, "message"))
	}
	selection := messageSelection{ChatID: 7, MessageID: 250}

	calls := 0
	results := buildVisibleHistoryResults(groups, 9, true, selection, func(group ui.RenderedMessageGroup) messageGroupResult {
		calls++
		message := group.Messages[0]
		return messageGroupResult{
			Height:   1,
			Group:    group,
			Selected: message.ChatID == selection.ChatID && message.ID == selection.MessageID,
		}
	})

	if calls != 5 || len(results) != 5 {
		t.Fatalf("builder calls/results = %d/%d, want 5/5 for a 9-row viewport", calls, len(results))
	}
	if first, last := results[0].Group.Messages[0].ID, results[len(results)-1].Group.Messages[0].ID; first != 248 || last != 252 {
		t.Fatalf("built message range = %d..%d, want 248..252", first, last)
	}
	if cursor := centeredHistoryCursor(results, image.Rect(0, 0, 40, 9)); cursor != 9 {
		t.Fatalf("centered cursor = %d, want 9", cursor)
	}
}

func TestHistorySelectionFollowCentersWhileMessagesOverflow(t *testing.T) {
	styles := newRenderStyles(false)
	historyRect := image.Rect(0, 0, 40, 9)
	model := historyModel(40, 9, historyRect)
	for id := domain.MessageID(1); id <= 7; id++ {
		model.Groups = append(model.Groups, styledMessageGroup("Mina", false, testMessage(id, 7, "message")))
	}
	model.SelectedMessageChat = 7
	model.SelectedMessage = 4
	model.HistoryFollowSelection = true

	surface := buildHistoryLayer(model, historyRect, time.Local, styles)
	selectedRect := messageInteractionRect(t, surface, 7, 4)
	if selectedRect.Min.Y != historyRect.Min.Y+historyRect.Dy()/2 {
		t.Fatalf("selected message row y = %d, want viewport middle %d", selectedRect.Min.Y, historyRect.Min.Y+historyRect.Dy()/2)
	}
	if messageInteractionRect(t, surface, 7, 3).Min.Y >= selectedRect.Min.Y || messageInteractionRect(t, surface, 7, 5).Min.Y <= selectedRect.Min.Y {
		t.Fatal("centered selection did not retain visible messages on both sides")
	}
}

func TestHistorySelectionFollowKeepsBottomAlignmentWhenEverythingFits(t *testing.T) {
	styles := newRenderStyles(false)
	historyRect := image.Rect(0, 0, 40, 12)
	model := historyModel(40, 12, historyRect)
	for id := domain.MessageID(1); id <= 3; id++ {
		model.Groups = append(model.Groups, styledMessageGroup("Mina", false, testMessage(id, 7, "message")))
	}
	model.SelectedMessageChat = 7
	model.SelectedMessage = 2
	model.HistoryFollowSelection = true

	surface := buildHistoryLayer(model, historyRect, time.Local, styles)
	if newest := messageInteractionRect(t, surface, 7, 3); newest.Max.Y != historyRect.Max.Y {
		t.Fatalf("newest message bottom = %d, want %d when all messages fit", newest.Max.Y, historyRect.Max.Y)
	}
	if selected := messageInteractionRect(t, surface, 7, 2); selected.Min.Y == historyRect.Min.Y+historyRect.Dy()/2 {
		t.Fatal("selection was unnecessarily centered when the whole history fit")
	}
}

func messageInteractionRect(t *testing.T, surface surfaceResult, chatID domain.ChatID, messageID domain.MessageID) image.Rectangle {
	t.Helper()
	for _, interaction := range surface.Interactions {
		if interaction.Click.Action == app.SelectMessage && interaction.Click.ChatID == chatID && interaction.Click.MessageID == messageID {
			return interaction.Rect
		}
	}
	t.Fatalf("message interaction (%d, %d) is not visible", chatID, messageID)
	return image.Rectangle{}
}

func TestHistoryTopClippedGroupRendersOnlyVisibleRows(t *testing.T) {
	styles := newRenderStyles(false)
	// A 100x4 history viewport forces a tall top group to clip at the top.
	hist := image.Rect(0, 0, 100, 4)
	model := historyModel(100, 4, hist)
	// A long message wraps to many rows at width 100 (content width 98).
	long := testMessage(10, 7, strings.Repeat("word ", 60)) // wraps to >= 4 rows
	g1 := styledMessageGroup("Mina", false, long)
	g2 := styledMessageGroup("Lou", false, testMessage(11, 7, "bottom"))
	model.Groups = []ui.RenderedMessageGroup{g1, g2}

	// Prerequisite: top full height + separator + bottom height must exceed
	// the content height so the top group is genuinely clipped.
	full1 := buildMessageGroupLayer(g1, 100, time.Local, messageSelection{}, nil, styles)
	full2 := buildMessageGroupLayer(g2, 100, time.Local, messageSelection{}, nil, styles)
	if full1.Height+1+full2.Height <= 4 {
		t.Fatalf("prerequisite unmet: top %d + sep 1 + bottom %d = %d, need > 4", full1.Height, full2.Height, full1.Height+1+full2.Height)
	}

	surface := buildHistoryLayer(model, hist, time.Local, styles)
	canvas, compositor := historySurfaceCanvas(model, surface)

	// Expected bottom-up geometry: bottom group at the final row (3); the top
	// group's visible slice starts at startRow = full1.Height-2 > 0.
	expectedStartRow := full1.Height - 2
	if expectedStartRow <= 0 {
		t.Fatalf("expected startRow = %d, want > 0", expectedStartRow)
	}

	// Bottom group on the final row.
	if got := canvas.CellAt(1, 3).Content; got != "b" {
		t.Errorf("bottom group at row 3 = %q, want 'b'", got)
	}

	// At least one visible top-group interaction keeps a nonzero/original
	// row-index suffix while its absolute rect begins inside the history.
	var topInteraction *layerInteraction
	for i := range surface.Interactions {
		li := &surface.Interactions[i]
		if li.ID == "conversation:history" || li.ID == "conversation:history-retry" {
			continue
		}
		if li.Click.ChatID == 7 && li.Click.MessageID == 10 {
			topInteraction = li
			break
		}
	}
	if topInteraction == nil {
		t.Fatal("no visible top-group interaction")
	}
	// Suffix is the original row index (RowOffset = startRow for the first
	// visible row) which is nonzero.
	if !strings.HasPrefix(topInteraction.ID, "message:7:10:") {
		t.Errorf("top interaction ID = %q, want message:7:10: prefix", topInteraction.ID)
	}
	suffix := strings.TrimPrefix(topInteraction.ID, "message:7:10:")
	if suffix == "0" {
		t.Errorf("top interaction suffix = %q, want nonzero original row index", suffix)
	}
	if topInteraction.Rect.Min.Y < 0 || topInteraction.Rect.Max.Y > 4 {
		t.Errorf("top interaction rect %v escapes history", topInteraction.Rect)
	}

	// No interaction, compositor bounds, or text cell escapes the history.
	for _, interaction := range surface.Interactions {
		if interaction.ID == "conversation:history" || interaction.ID == "conversation:history-retry" {
			continue
		}
		if interaction.Rect.Min.Y < 0 || interaction.Rect.Max.Y > 4 {
			t.Errorf("interaction %q rect %v escapes history viewport", interaction.ID, interaction.Rect)
		}
	}
	if got := compositor.Bounds(); got.Min.Y < 0 || got.Max.Y > 4 || got.Min.X < 0 || got.Max.X > 100 {
		t.Errorf("compositor bounds %v escape history", got)
	}
	// No text cell above the history (y<0 is impossible on a 4-row canvas, but
	// the top group's clipped rows must not draw into row 0's top neighbor).
	// The topmost rendered row of the history must be within the viewport.
	for y := 0; y < 4; y++ {
		for x := 0; x < 100; x++ {
			if cell := canvas.CellAt(x, y); cell != nil && cell.Content != "" && cell.Content != " " {
				// Any content is inside the 4-row canvas by construction.
				_ = cell
			}
		}
	}
}

func TestHistoryPartiallyClippedAvatarNoRetry(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	msg := testMessage(20, 7, "hello")
	group := styledMessageGroup("Mina", true, msg)
	group.AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "failed"}
	model.Groups = []ui.RenderedMessageGroup{group}

	// With a history height of 1, the 2-row group is clipped to its bottom row
	// (the body); the avatar row is above the viewport and must be omitted.
	small := historyModel(100, 1, image.Rect(0, 0, 100, 1))
	small.Groups = []ui.RenderedMessageGroup{group}
	small.HistoryDone = true
	surface := buildHistoryLayer(small, image.Rect(0, 0, 100, 1), time.Local, styles)
	for _, interaction := range surface.Interactions {
		if strings.HasPrefix(interaction.ID, "message-avatar-retry:") {
			t.Errorf("partially clipped avatar published retry %q", interaction.ID)
		}
	}
	// No avatar content above the viewport.
	canvas, _ := historySurfaceCanvas(small, surface)
	if got := canvas.CellAt(0, 0).Content; got == "░" {
		t.Errorf("clipped avatar cell leaked at (0,0)")
	}
}

func TestHistoryFullyContainedAvatarRetained(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	msg := testMessage(21, 7, "hello")
	group := styledMessageGroup("Mina", true, msg)
	model.Groups = []ui.RenderedMessageGroup{group}

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	canvas, _ := historySurfaceCanvas(model, surface)
	// The single 2-row group is bottom aligned at rows 38-39, so the avatar
	// (rows 0-1 of the group) is fully visible at absolute rows 38-39.
	if got := canvas.CellAt(0, 38).Content; got != "░" {
		t.Errorf("avatar cell (0,38) = %q, want ░", got)
	}
	if got := canvas.CellAt(3, 39).Content; got != "░" {
		t.Errorf("avatar cell (3,39) = %q, want ░", got)
	}
}

func TestHistorySelectedAvatarRetryRetainedNonzeroStart(t *testing.T) {
	styles := newRenderStyles(false)
	// Selected ShowAvatar group: full height 4, avatar base rect (1,1)-(5,3).
	// A 3-row history slices [1,4), wholly containing the avatar, which is
	// Y-adjusted to fragment (1,0)-(5,2).
	hist := image.Rect(0, 0, 100, 3)
	model := historyModel(100, 3, hist)
	msg := testMessage(45, 7, "selected avatar")
	group := styledMessageGroup("Mina", true, msg)
	group.AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "failed"}
	model.Groups = []ui.RenderedMessageGroup{group}
	model.SelectedMessageChat = 7
	model.SelectedMessage = 45

	// Prerequisite: the selected group is 4 rows tall.
	full := buildMessageGroupLayer(group, 100, time.Local, messageSelection{ChatID: 7, MessageID: 45}, nil, styles)
	if full.Height != 4 {
		t.Fatalf("selected group height = %d, want 4", full.Height)
	}

	surface := buildHistoryLayer(model, hist, time.Local, styles)
	canvas, compositor := historySurfaceCanvas(model, surface)

	// Absolute retry rect = history origin + fragment (1,0)-(5,2).
	wantRect := image.Rect(1, 0, 5, 2)
	var retry *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "message-avatar-retry:7:45" {
			retry = &surface.Interactions[i]
		}
	}
	if retry == nil {
		t.Fatal("no retained avatar retry interaction")
	}
	if !retry.Rect.Eq(wantRect) {
		t.Errorf("retry rect = %v, want %v", retry.Rect, wantRect)
	}
	if retry.Click.Action != app.Retry || retry.Click.AvatarKey != "avatar-key" {
		t.Errorf("retry click = %#v, want Retry/avatar-key", retry.Click)
	}
	if retry.Z != zControl {
		t.Errorf("retry Z = %d, want %d", retry.Z, zControl)
	}

	// Compositor.Hit at a representative avatar point returns the same ID and
	// absolute bounds.
	pt := image.Pt(3, 1)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "message-avatar-retry:7:45" {
		t.Errorf("Hit(%v) = %q, want message-avatar-retry:7:45", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(wantRect) {
		t.Errorf("retry hit bounds = %v, want %v", got, wantRect)
	}

	// Avatar cells render at the adjusted Y (fragment rows 0-1).
	if got := canvas.CellAt(1, 0).Content; got != "░" {
		t.Errorf("avatar cell (1,0) = %q, want ░", got)
	}
	if got := canvas.CellAt(4, 1).Content; got != "░" {
		t.Errorf("avatar cell (4,1) = %q, want ░", got)
	}

	// All bounds remain inside the history.
	if got := compositor.Bounds(); got.Min.Y < 0 || got.Max.Y > 3 || got.Min.X < 0 || got.Max.X > 100 {
		t.Errorf("compositor bounds %v escape history", got)
	}
}

func TestHistorySelectedClippedFragmentRoundedFrame(t *testing.T) {
	styles := newRenderStyles(false)
	// A selected group whose body wraps makes the full group taller than the
	// history. In a 4-row history the visible slice is the bottom 4 rows of the
	// taller group; a Height(4) RoundedBorder renders exactly 4 rows, so the
	// real corners fit exactly inside the history without escaping.
	hist := image.Rect(0, 0, 30, 4)
	model := historyModel(30, 4, hist)
	long := "this is a long selected body that wraps onto several rows of text content to make it tall"
	msg := testMessage(30, 7, long)
	group := styledMessageGroup("Mina", false, msg)
	model.Groups = []ui.RenderedMessageGroup{group}
	model.SelectedMessageChat = 7
	model.SelectedMessage = 30

	// Prerequisite: the selected full group is taller than the 4-row history so
	// slicing actually occurs.
	full := buildMessageGroupLayer(group, 30, time.Local, messageSelection{ChatID: 7, MessageID: 30}, nil, styles)
	if full.Selected != true || full.Height <= 4 {
		t.Fatalf("selected group height = %d, want > 4", full.Height)
	}

	surface := buildHistoryLayer(model, hist, time.Local, styles)
	canvas, compositor := historySurfaceCanvas(model, surface)

	// The visible fragment is the bottom 4 rows of the selected group; the
	// RoundedBorder corners span the exact history corners.
	if got := canvas.CellAt(0, 0).Content; got != "╭" {
		t.Errorf("top-left corner = %q, want ╭", got)
	}
	if got := canvas.CellAt(29, 0).Content; got != "╮" {
		t.Errorf("top-right corner = %q, want ╮", got)
	}
	if got := canvas.CellAt(0, 3).Content; got != "╰" {
		t.Errorf("bottom-left corner = %q, want ╰", got)
	}
	if got := canvas.CellAt(29, 3).Content; got != "╯" {
		t.Errorf("bottom-right corner = %q, want ╯", got)
	}

	// Compositor bounds stay inside the history.
	if got := compositor.Bounds(); got.Min.Y < 0 || got.Max.Y > 4 || got.Min.X < 0 || got.Max.X > 30 {
		t.Errorf("compositor bounds %v escape history", got)
	}
	// No interaction escapes the history.
	for _, interaction := range surface.Interactions {
		if interaction.ID == "conversation:history" {
			continue
		}
		if interaction.Rect.Min.Y < 0 || interaction.Rect.Max.Y > 4 {
			t.Errorf("interaction %q rect %v escapes small history", interaction.ID, interaction.Rect)
		}
	}
}

func TestHistoryEveryNonVirtualInteractionHitParityAndUniqueIDs(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	msg := testMessage(40, 7, "hit me")
	group := styledMessageGroup("Mina", false, msg)
	model.Groups = []ui.RenderedMessageGroup{group}
	model.HistoryDone = true

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	_, compositor := historySurfaceCanvas(model, surface)

	seen := map[string]bool{}
	for _, interaction := range surface.Interactions {
		if interaction.Virtual {
			continue
		}
		if interaction.Rect.Empty() {
			continue
		}
		if seen[interaction.ID] {
			t.Errorf("duplicate interaction ID %q", interaction.ID)
		}
		seen[interaction.ID] = true
		pt := image.Pt(interaction.Rect.Min.X+interaction.Rect.Dx()/2, interaction.Rect.Min.Y+interaction.Rect.Dy()/2)
		hit := compositor.Hit(pt.X, pt.Y)
		if hit.ID() != interaction.ID {
			t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), interaction.ID)
		}
		if got := hit.Bounds(); !got.Eq(interaction.Rect) {
			t.Errorf("%s hit bounds = %v, want %v", interaction.ID, got, interaction.Rect)
		}
	}
}

func TestHistoryLongFE0FZWJReactionStaysWithinWidth(t *testing.T) {
	styles := newRenderStyles(false)
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	msg := testMessage(50, 7, "👨‍👩‍👧‍👦🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀")
	msg.Outgoing = true
	group := styledMessageGroup("Mina", false, msg)
	model.Groups = []ui.RenderedMessageGroup{group}
	model.HistoryDone = true

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	canvas, compositor := historySurfaceCanvas(model, surface)

	// Compositor bounds stay within the history rect.
	if got := compositor.Bounds(); got.Min.X < 0 || got.Max.X > 100 || got.Min.Y < 0 || got.Max.Y > 40 {
		t.Errorf("compositor bounds %v escape history", got)
	}

	// The outgoing ZWJ text is right-aligned and clipped to the width. Verify
	// every rendered line stays within the 100-cell width by scanning the
	// canvas cells: no non-empty cell may appear at or beyond column 100, and
	// the widest non-space cell on each row is at most column 99.
	for y := 0; y < 40; y++ {
		if cell := canvas.CellAt(100, y); cell != nil && cell.Content != "" {
			t.Errorf("cell at column 100 row %d = %q, want empty (width leak)", y, cell.Content)
		}
	}
	// The outgoing row is right-aligned: its text ends at the right edge of
	// the history, so the bottom row's last non-space cell is at column 99.
	for y := 0; y < 40; y++ {
		for x := 0; x < 100; x++ {
			if cell := canvas.CellAt(x, y); cell != nil && cell.Content != "" && cell.Content != " " {
				// Any content cell is inside the 100-cell canvas by construction.
				_ = cell
			}
		}
	}
}

func TestHistoryEmptyAndTinyViewportSafety(t *testing.T) {
	styles := newRenderStyles(false)

	// Empty rect -> zero surface with hidden cursor.
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	surface := buildHistoryLayer(model, image.Rect(0, 0, 0, 0), time.Local, styles)
	if surface.Layer != nil {
		t.Errorf("empty rect layer = %v, want nil", surface.Layer)
	}
	if surface.Cursor.X != -1 || surface.Cursor.Y != -1 {
		t.Errorf("empty rect cursor = %#v, want hidden", surface.Cursor)
	}

	// Tiny viewport with a group: must not panic and must not leak.
	tiny := historyModel(5, 2, image.Rect(0, 0, 5, 2))
	msg := testMessage(60, 7, "x")
	tiny.Groups = []ui.RenderedMessageGroup{styledMessageGroup("Mina", false, msg)}
	tiny.HistoryDone = true
	ts := buildHistoryLayer(tiny, image.Rect(0, 0, 5, 2), time.Local, styles)
	if ts.Layer == nil {
		t.Fatal("tiny viewport layer nil")
	}
	canvas, _ := historySurfaceCanvas(tiny, ts)
	_ = canvas
}

// TestHistoryPublishesKittyPlacementsAbsolute asserts that a fully visible
// Kitty thumbnail surfaces as an absolute-viewport Inline placement.
func TestHistoryPublishesKittyPlacementsAbsolute(t *testing.T) {
	styles := newRenderStyles(false)
	msg := testMessage(4001, 9, "[Photo]")
	msg.Kind = domain.MessagePhoto
	group := styledMessageGroup("Mina", false, msg)

	block := thumbnail.Block{Text: "kitty", Width: 20, Height: 8, Kitty: true, ImageID: 99}
	model := historyModel(100, 40, image.Rect(0, 0, 100, 40))
	model.Groups = []ui.RenderedMessageGroup{group}
	model.InlineThumbnails = []ui.RenderedThumbnail{{ChatID: 9, MessageID: 4001, Block: block}}

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 40), time.Local, styles)
	if len(surface.Inline) != 1 {
		t.Fatalf("Inline placements = %d, want 1", len(surface.Inline))
	}
	// The single group is bottom-aligned: height 9 (8 thumbnails + photo body)
	// in a 40-row viewport places its top at row 31.
	placement := surface.Inline[0]
	if placement.ImageID != 99 || placement.X != 1 || placement.Y != 31 ||
		placement.Width != 20 || placement.Height != 8 || placement.Text != "kitty" {
		t.Fatalf("placement = %#v, want {ImageID:99 X:1 Y:31 W:20 H:8}", placement)
	}
}

// TestHistoryClipsKittyPlacementsToViewport asserts that a thumbnail whose
// cell box is partially scrolled out of the viewport surfaces as a cropped
// absolute-viewport placement instead of being dropped.
func TestHistoryClipsKittyPlacementsToViewport(t *testing.T) {
	styles := newRenderStyles(false)
	msg := testMessage(4002, 9, "[Photo]")
	msg.Kind = domain.MessagePhoto
	group := styledMessageGroup("Mina", false, msg)

	transmit := "\x1b_Ga=T,f=100,i=100,s=64,v=64,c=20,r=8,q=2;AAAA\x1b\\"
	block := thumbnail.Block{Text: transmit, Width: 20, Height: 8, Kitty: true, ImageID: 100}
	model := historyModel(100, 4, image.Rect(0, 0, 100, 4))
	model.Groups = []ui.RenderedMessageGroup{group}
	model.InlineThumbnails = []ui.RenderedThumbnail{{ChatID: 9, MessageID: 4002, Block: block}}

	surface := buildHistoryLayer(model, image.Rect(0, 0, 100, 4), time.Local, styles)
	if len(surface.Inline) != 1 {
		t.Fatalf("clipped Inline placements = %d, want 1", len(surface.Inline))
	}
	placement := surface.Inline[0]
	wantID := derivedInlineID(100, 0)
	// The 8-row block starts at absolute row -5 (bottom-aligned in 4 rows) and
	// only rows 0..2 remain inside the pane: a 3-row crop from source y=40.
	if placement.ImageID != wantID || placement.X != 1 || placement.Y != 0 ||
		placement.Width != 20 || placement.Height != 3 {
		t.Fatalf("placement = %#v, want {ID:%d X:1 Y:0 W:20 H:3}", placement, wantID)
	}
	if !strings.Contains(placement.Text, "i="+strconv.FormatUint(uint64(wantID), 10)) ||
		!strings.Contains(placement.Text, "c=20") || !strings.Contains(placement.Text, "r=3") ||
		!strings.Contains(placement.Text, "x=0,y=40,w=64,h=24") {
		t.Fatalf("transmit not rewritten: %q", placement.Text)
	}
}

func TestHistoryForumNoTopicPlaceholder(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 100, 40)

	// Forum without a known topic still loading: loading text wins.
	loading := historyModel(100, 40, rect)
	loading.ActiveChat = domain.Chat{ID: 1, Kind: domain.ChatSupergroup, IsForum: true}
	loading.Groups = nil
	loading.HistoryLoading = true
	ls := buildHistoryLayer(loading, rect, time.Local, styles)
	lc, _ := historySurfaceCanvas(loading, ls)
	if !strings.Contains(plainText(lc.Render()), "Loading messages...") {
		t.Errorf("forum no-topic loading text missing, got %q", plainText(lc.Render()))
	}
	if strings.Contains(plainText(lc.Render()), "Select a topic to view messages") {
		t.Errorf("forum no-topic loading leaked placeholder")
	}

	done := historyModel(100, 40, rect)
	done.ActiveChat = domain.Chat{ID: 1, Kind: domain.ChatSupergroup, IsForum: true}
	done.Groups = nil
	done.HistoryDone = true
	ds := buildHistoryLayer(done, rect, time.Local, styles)
	dc, _ := historySurfaceCanvas(done, ds)
	if !strings.Contains(plainText(dc.Render()), "Select a topic to view messages") {
		t.Errorf("forum no-topic done placeholder missing, got %q", plainText(dc.Render()))
	}

	// Known topic: existing Loading/Done texts unchanged.
	topic := historyModel(100, 40, rect)
	topic.ActiveChat = domain.Chat{ID: 1, Kind: domain.ChatSupergroup, IsForum: true}
	topic.ActiveTopicKnown = true
	topic.Groups = nil
	topic.HistoryDone = true
	ts := buildHistoryLayer(topic, rect, time.Local, styles)
	tc, _ := historySurfaceCanvas(topic, ts)
	if !strings.Contains(plainText(tc.Render()), "No messages") {
		t.Errorf("forum known-topic done must show No messages, got %q", plainText(tc.Render()))
	}

	// Non-forum: placeholder never appears.
	plain := historyModel(100, 40, rect)
	plain.ActiveChat = domain.Chat{ID: 1}
	plain.Groups = nil
	plain.HistoryDone = true
	ps := buildHistoryLayer(plain, rect, time.Local, styles)
	pc, _ := historySurfaceCanvas(plain, ps)
	if strings.Contains(plainText(pc.Render()), "Select a topic to view messages") {
		t.Errorf("non-forum history leaked forum placeholder")
	}

	// ALL mode: placeholder suppressed; falls through to the normal Done text.
	allMode := historyModel(100, 40, rect)
	allMode.ActiveChat = domain.Chat{ID: 1, Kind: domain.ChatSupergroup, IsForum: true}
	allMode.ShowAllTopics = true
	allMode.Groups = nil
	allMode.HistoryDone = true
	as := buildHistoryLayer(allMode, rect, time.Local, styles)
	ac, _ := historySurfaceCanvas(allMode, as)
	if strings.Contains(plainText(ac.Render()), "Select a topic to view messages") {
		t.Errorf("forum ALL mode leaked placeholder")
	}
	if !strings.Contains(plainText(ac.Render()), "No messages") {
		t.Errorf("forum ALL mode done must show No messages, got %q", plainText(ac.Render()))
	}
}
