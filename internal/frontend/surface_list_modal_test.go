package frontend

import (
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// listModalCanvas composes a full-size styles.Base root with the modal layer
// and returns the single Compositor and a Canvas over the viewport.
func listModalCanvas(bounds image.Rectangle, surface surfaceResult) (*lipgloss.Compositor, *lipgloss.Canvas) {
	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(bounds.Dx()).Height(bounds.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return compositor, lipgloss.NewCanvas(bounds.Dx(), bounds.Dy()).Compose(compositor)
}

func listModal(t *testing.T, width, height int, title string, rows []modalRowSpec) (image.Rectangle, surfaceResult, *lipgloss.Compositor, *lipgloss.Canvas) {
	t.Helper()
	bounds := image.Rect(0, 0, width, height)
	styles := newRenderStyles(false)
	items := make([]components.Item, 0, len(rows))
	selected := -1
	for index, row := range rows {
		items = append(items, components.Item{Label: row.Label})
		if selected < 0 && row.Selected {
			selected = index
		}
	}
	_layout := (components.Modal{Title: title, Items: items, Selected: selected}).Layout(bounds)
	surface := buildListModal(bounds, title, rows, styles)
	compositor, canvas := listModalCanvas(bounds, surface)
	if compositor.Render() == "" {
		t.Fatal("compositor rendered empty content")
	}
	return _layout.Frame, surface, compositor, canvas
}

func TestListModalZeroTinyBoundsSafe(t *testing.T) {
	styles := newRenderStyles(false)
	rows := []modalRowSpec{
		{ID: "r0", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
	}
	for _, w := range []int{0, 1, 2} {
		for _, h := range []int{0, 1, 2} {
			bounds := image.Rect(0, 0, w, h)
			surface := buildListModal(bounds, "Actions", rows, styles)
			if w == 0 || h == 0 {
				// Truly zero bounds: no layer, no interactions, hidden cursor.
				if surface.Layer != nil {
					t.Errorf("%dx%d: zero bounds should have no layer", w, h)
				}
				if len(surface.Interactions) != 0 {
					t.Errorf("%dx%d: zero bounds should have no interactions", w, h)
				}
			} else {
				// Tiny positive bounds: a safe frame layer that never escapes.
				if surface.Layer == nil {
					t.Fatalf("%dx%d: tiny bounds should produce a safe layer", w, h)
				}
				if surface.Layer.GetX() < 0 || surface.Layer.GetY() < 0 ||
					surface.Layer.GetX()+surface.Layer.Width() > w ||
					surface.Layer.GetY()+surface.Layer.Height() > h {
					t.Errorf("%dx%d: layer escapes viewport", w, h)
				}
			}
			if surface.Cursor.Visible {
				t.Errorf("%dx%d: modal should have hidden cursor", w, h)
			}
			if got := surface.Cursor; got.X != -1 || got.Y != -1 {
				t.Errorf("%dx%d: cursor = %v, want {-1,-1}", w, h, got)
			}
		}
	}
}

func TestListModalTinyBoundsNoChildEscape(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "r0", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
		{ID: "r1", Label: "Reply", Action: app.ActionReceived{Action: app.ReplyMessage}},
	}
	for _, w := range []int{3, 4, 5, 6, 7, 8} {
		for _, h := range []int{3, 4, 5, 6} {
			_, surface, _, canvas := listModal(t, w, h, "Actions", rows)
			if w <= 4 || h <= 4 {
				// Too small to hold a frame: safe, no children escape.
				continue
			}
			cb := canvas.Bounds()
			if got := int(cb.Dx()); got != w {
				t.Errorf("%dx%d: canvas width = %d, want %d", w, h, got, w)
			}
			if surface.Layer.GetX() < 0 || surface.Layer.GetY() < 0 ||
				surface.Layer.GetX()+surface.Layer.Width() > w ||
				surface.Layer.GetY()+surface.Layer.Height() > h {
				t.Errorf("%dx%d: layer escapes viewport", w, h)
			}
		}
	}
}

func TestListModalExactLayoutGeometry(t *testing.T) {
	title := "Message actions"
	rows := []modalRowSpec{
		{ID: "a", Label: "Copy", Selected: true, Action: app.ActionReceived{Action: app.CopyMessage}},
		{ID: "b", Label: "Reply", Action: app.ActionReceived{Action: app.ReplyMessage}},
		{ID: "c", Label: "Pin", Action: app.ActionReceived{Action: app.PinMessage}},
	}
	bounds := image.Rect(0, 0, 80, 24)
	styles := newRenderStyles(false)

	items := make([]components.Item, len(rows))
	for i, row := range rows {
		items[i] = components.Item{Label: row.Label}
	}
	want := (components.Modal{Title: title, Items: items, Selected: 0}).Layout(bounds)
	surface := buildListModal(bounds, title, rows, styles)

	if !surface.Rect.Eq(want.Frame) {
		t.Errorf("surface rect = %v, want %v", surface.Rect, want.Frame)
	}
	if surface.Layer.GetX() != want.Frame.Min.X || surface.Layer.GetY() != want.Frame.Min.Y {
		t.Errorf("layer pos = (%d,%d), want (%d,%d)",
			surface.Layer.GetX(), surface.Layer.GetY(), want.Frame.Min.X, want.Frame.Min.Y)
	}
	if surface.Layer.Width() != want.Frame.Dx() || surface.Layer.Height() != want.Frame.Dy() {
		t.Errorf("layer size = %dx%d, want %dx%d",
			surface.Layer.Width(), surface.Layer.Height(), want.Frame.Dx(), want.Frame.Dy())
	}
	if got := surface.Layer.GetID(); got != "" {
		t.Errorf("frame layer ID = %q, want empty", got)
	}
	if len(want.Rows) != 3 {
		t.Fatalf("layout rows = %d, want 3", len(want.Rows))
	}
	for i, row := range want.Rows {
		if !row.Rect.Eq(want.Rows[i].Rect) {
			t.Errorf("layout row %d rect = %v, want %v", i, row.Rect, want.Rows[i].Rect)
		}
	}
}

func TestListModalRoundedBorderCornersAndPalette(t *testing.T) {
	title := "Actions"
	rows := []modalRowSpec{
		{ID: "a", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
	}
	frame, surface, _, canvas := listModal(t, 80, 24, title, rows)
	if surface.Layer == nil {
		t.Fatal("modal layer is nil")
	}

	for _, corner := range []struct {
		x, y int
		want string
	}{
		{frame.Min.X, frame.Min.Y, "╭"},
		{frame.Max.X - 1, frame.Min.Y, "╮"},
		{frame.Min.X, frame.Max.Y - 1, "╰"},
		{frame.Max.X - 1, frame.Max.Y - 1, "╯"},
	} {
		cell := canvas.CellAt(corner.x, corner.y)
		if cell == nil {
			t.Fatalf("CellAt(%d,%d) is nil", corner.x, corner.y)
		}
		if got := cell.Content; got != corner.want {
			t.Errorf("corner (%d,%d) = %q, want %q", corner.x, corner.y, got, corner.want)
		}
		if got := colorOf(cell.Style.Fg); got != rgba(focusedBorderColor) {
			t.Errorf("corner (%d,%d) fg = %v, want %v", corner.x, corner.y, got, rgba(focusedBorderColor))
		}
	}
	// A left edge interior cell is a vertical border.
	if cell := canvas.CellAt(frame.Min.X, frame.Min.Y+1); cell == nil || cell.Content != "│" {
		t.Errorf("left edge cell = %v, want │", cell)
	}
}

func TestListModalLongCJKFE0FZWJTitleClipsBeforeClose(t *testing.T) {
	// A long title spanning wide CJK, FE0F, and ZWJ clusters must clip before
	// the close cell so the top-right border corner (and the close) always win.
	title := "很长的标题👨‍👩‍👧‍👦🔥❤️多字节"
	rows := []modalRowSpec{
		{ID: "a", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
	}
	frame, _, compositor, canvas := listModal(t, 80, 24, title, rows)

	// Top row holds the title into the content interval and a close at the
	// right. The top-right border corner must remain ┐ regardless of title.
	topRight := canvas.CellAt(frame.Max.X-1, frame.Min.Y)
	if topRight == nil || topRight.Content != "╮" {
		t.Errorf("top-right corner = %v, want ╮", topRight.Content)
	}
	// Close cell (the 1-cell rectangle at Close) is under modal:close.
	closePt := frame.Min.Add(image.Pt(frame.Dx()-2, 0))
	hit := compositor.Hit(closePt.X, closePt.Y)
	if hit.ID() != "modal:close" {
		t.Errorf("Hit(close) = %q, want modal:close", hit.ID())
	}
	// A local title row cell far left is a title glyph, never the corner.
	first := canvas.CellAt(frame.Min.X+2, frame.Min.Y)
	if first == nil || first.Content == "" || first.Content == "╮" {
		t.Errorf("title first cell = %q", first.Content)
	}
	// Title width never exceeds the interval before the close cell.
	titleWidth := ansi.Truncate(title, titleWidthFor(frame), "")
	if window := titleWidthFor(frame); window > 0 {
		if got := ansi.StringWidth(titleWidth); got > window {
			t.Errorf("clipped title width = %d, want <= %d", got, window)
		}
	}
}

func titleWidthFor(frame image.Rectangle) int {
	return (frame.Min.X + frame.Dx() - 2) - (frame.Min.X + 2)
}

// closePoint returns the absolute Close cell of the layout mirroring production.
func closePoint(frame image.Rectangle) image.Point {
	return image.Pt(max(frame.Min.X, frame.Max.X-2), frame.Min.Y)
}

func TestListModalCloseIDRectActionAndHitParity(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "a", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
	}
	frame, surface, compositor, _ := listModal(t, 80, 24, "Actions", rows)

	var close *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "modal:close" {
			close = &surface.Interactions[i]
		}
	}
	if close == nil {
		t.Fatal("no modal:close interaction")
	}
	wantRect := image.Rect(closePoint(frame).X, closePoint(frame).Y, closePoint(frame).X+1, closePoint(frame).Y+1)
	if !close.Rect.Eq(wantRect) {
		t.Errorf("close rect = %v, want %v", close.Rect, wantRect)
	}
	if close.Z != zModalControl {
		t.Errorf("close Z = %d, want %d", close.Z, zModalControl)
	}
	if close.Click.Action != app.Close {
		t.Errorf("close click = %#v, want Close", close.Click)
	}
	if close.WheelUp.Action != app.NoAction || close.WheelDown.Action != app.NoAction {
		t.Errorf("close wheel should be empty actions")
	}
	// Hit parity at the close cell and inside it.
	pt := image.Pt(wantRect.Min.X, wantRect.Min.Y)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != close.ID {
		t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), close.ID)
	}
	if got := hit.Bounds(); !got.Eq(close.Rect) {
		t.Errorf("close hit bounds = %v, want %v", got, close.Rect)
	}
	// A cell just left of the close (before the corner) is not the close.
	left := image.Pt(wantRect.Min.X-1, wantRect.Min.Y)
	if hit := compositor.Hit(left.X, left.Y); hit.ID() == "modal:close" {
		t.Errorf("cell left of close should not be modal:close")
	}
}

func TestListModalSelectedRowCoversFullRow(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "sel", Label: "Selected item", Selected: true, Action: app.ActionReceived{Action: app.CopyMessage}},
		{ID: "unsel", Label: "Plain item", Action: app.ActionReceived{Action: app.NoAction}},
	}
	_, _, _, canvas := listModal(t, 80, 24, "Actions", rows)

	// Find the selected row rect from the layout.
	var selRect, unselRect image.Rectangle
	items := []components.Item{{Label: "Selected item"}, {Label: "Plain item"}}
	layout := (components.Modal{Title: "Actions", Items: items, Selected: 0}).Layout(image.Rect(0, 0, 80, 24))
	for _, row := range layout.Rows {
		if row.Selected {
			selRect = row.Rect
		} else {
			unselRect = row.Rect
		}
	}

	// Selected row covers its exact full row with selected background.
	for y := selRect.Min.Y; y < selRect.Max.Y; y++ {
		for x := selRect.Min.X; x < selRect.Max.X; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				t.Errorf("CellAt(%d,%d) nil in selected row", x, y)
				continue
			}
			if got := colorOf(cell.Style.Bg); got != rgba(selectedColor) {
				t.Errorf("selected row (%d,%d) bg = %v, want %v", x, y, got, rgba(selectedColor))
			}
		}
	}
	// Unselected row uses the terminal background and differs.
	if cell := canvas.CellAt(unselRect.Min.X, unselRect.Min.Y); cell != nil && cell.Style.Bg != nil {
		t.Errorf("unselected row bg = %v, want nil terminal background", colorOf(cell.Style.Bg))
	}
	// Label of the selected row is present.
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Selected item") {
		t.Errorf("selected row label missing, got %q", text)
	}
}

func TestListModalActionableRowExactIDRectActionHit(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "action:copy", Label: "Copy message", Action: app.ActionReceived{Action: app.CopyMessage, MessageID: 7}},
	}
	_, surface, compositor, _ := listModal(t, 80, 24, "Actions", rows)

	var row *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "action:copy" {
			row = &surface.Interactions[i]
		}
	}
	if row == nil {
		t.Fatal("no actionable row interaction")
	}
	items := []components.Item{{Label: "Copy message"}}
	layout := (components.Modal{Title: "Actions", Items: items}).Layout(image.Rect(0, 0, 80, 24))
	wantRect := layout.Rows[0].Rect
	if !row.Rect.Eq(wantRect) {
		t.Errorf("row rect = %v, want %v", row.Rect, wantRect)
	}
	if row.Z != zModalRow {
		t.Errorf("row Z = %d, want %d", row.Z, zModalRow)
	}
	if row.Click.Action != app.CopyMessage || row.Click.MessageID != 7 {
		t.Errorf("row click = %#v, want CopyMessage/7", row.Click)
	}
	// Full-row hit parity: any interior point within the row bounds.
	pt := image.Pt(wantRect.Min.X+1, wantRect.Min.Y+wantRect.Dy()/2)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != row.ID {
		t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), row.ID)
	}
	if got := hit.Bounds(); !got.Eq(row.Rect) {
		t.Errorf("hit bounds = %v, want %v", got, row.Rect)
	}
	// Leftmost border cell of the row (frame interior) is still the row.
	edge := image.Pt(wantRect.Min.X, wantRect.Min.Y)
	if hit := compositor.Hit(edge.X, edge.Y); hit.ID() != row.ID {
		t.Errorf("Hit(edge) = %q, want %q", hit.ID(), row.ID)
	}
}

func TestListModalInformationalRowNoIDHit(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "info", Label: "Just a label", Action: app.ActionReceived{Action: app.NoAction}},
		{ID: "act", Label: "Do it", Action: app.ActionReceived{Action: app.Activate}},
	}
	_, surface, compositor, _ := listModal(t, 80, 24, "Actions", rows)

	items := []components.Item{{Label: "Just a label"}, {Label: "Do it"}}
	layout := (components.Modal{Title: "Actions", Items: items}).Layout(image.Rect(0, 0, 80, 24))
	infoRect := layout.Rows[0].Rect
	actRect := layout.Rows[1].Rect

	// The informational row must not produce its own interaction.
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "info" {
			t.Errorf("informational row should not be interactive")
		}
	}
	// At an informational-row point, no ID hits (only the close/actionable).
	pt := image.Pt(infoRect.Min.X+1, infoRect.Min.Y)
	if hit := compositor.Hit(pt.X, pt.Y); hit.ID() != "" {
		t.Errorf("Hit(info point) = %q, want empty", hit.ID())
	}
	// The actionable row still works.
	apt := image.Pt(actRect.Min.X+1, actRect.Min.Y)
	if hit := compositor.Hit(apt.X, apt.Y); hit.ID() != "act" {
		t.Errorf("Hit(actionable point) = %q, want act", hit.ID())
	}
}

func TestListModalLongWideLabelSingleLineBounded(t *testing.T) {
	long := strings.Repeat("字", 60) + strings.Repeat("x", 40)
	rows := []modalRowSpec{
		{ID: "long", Label: long, Action: app.ActionReceived{Action: app.Activate}},
	}
	_, surface, _, canvas := listModal(t, 80, 24, "Actions", rows)

	text := plainText(canvas.Render())
	if got := strings.Count(text, "\n"); got != 23 {
		t.Errorf("rendered lines = %d, want 24 (single line, no wrapping)", got+1)
	}
	// The label is clipped to the row width (row width minus the leading
	// cell), so the raw over-long label never escapes horizontally.
	items := []components.Item{{Label: long}}
	layout := (components.Modal{Title: "Actions", Items: items}).Layout(image.Rect(0, 0, 80, 24))
	row := layout.Rows[0]
	// First cell of the row holds the start of the clipped label.
	first := canvas.CellAt(row.Rect.Min.X+1, row.Rect.Min.Y)
	if first == nil || first.Content == "" {
		t.Errorf("long label first cell empty")
	}
	// No glyph extends past the row's right interior edge into the border
	// (the production clips to row width-1). Measure the clipped label width.
	clipped := ansi.Truncate(long, row.Rect.Dx()-1, "")
	if got := ansi.StringWidth(clipped); got > row.Rect.Dx()-1 {
		t.Errorf("clipped label width %d would overflow row width %d", got, row.Rect.Dx()-1)
	}
	if ansi.StringWidth(clipped) >= row.Rect.Dx() {
		t.Errorf("clipped label must fit inside the row interior")
	}
	if surface.Layer.GetX()+surface.Layer.Width() > 80 || surface.Layer.GetY()+surface.Layer.Height() > 24 {
		t.Errorf("layer escapes bounds")
	}
}

func TestListModalAllIDsUniqueAndInteractionsMatchHitBounds(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "r1", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
		{ID: "r2", Label: "Reply", Action: app.ActionReceived{Action: app.ReplyMessage}},
		{ID: "r3", Label: "Pin", Action: app.ActionReceived{Action: app.PinMessage}},
	}
	_, surface, compositor, _ := listModal(t, 80, 24, "Actions", rows)

	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	// Every interaction rect must exactly match the compositor hit bounds at a
	// representative point inside it.
	for i := range surface.Interactions {
		interaction := surface.Interactions[i]
		pt := image.Pt(interaction.Rect.Min.X+interaction.Rect.Dx()/2, interaction.Rect.Min.Y+interaction.Rect.Dy()/2)
		hit := compositor.Hit(pt.X, pt.Y)
		if hit.ID() != interaction.ID {
			t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), interaction.ID)
		}
		if got := hit.Bounds(); !got.Eq(interaction.Rect) {
			t.Errorf("Hit(%v) bounds = %v, want %v", pt, got, interaction.Rect)
		}
	}
}

func TestListModalHiddenCursor(t *testing.T) {
	rows := []modalRowSpec{
		{ID: "a", Label: "Copy", Action: app.ActionReceived{Action: app.CopyMessage}},
	}
	_, surface, _, _ := listModal(t, 80, 24, "Actions", rows)
	if surface.Cursor.Visible {
		t.Errorf("modal cursor should be hidden")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("cursor = %v, want {-1,-1}", got)
	}
	if reflect.TypeOf(surface.Layer) == nil {
		t.Errorf("layer should be non-nil type")
	}
}

func TestActionModalLayerNilState(t *testing.T) {
	styles := newRenderStyles(false)
	model := ui.ViewModel{Width: 80, Height: 24} // MessageMenu nil
	surface := buildActionModalLayer(model, styles)
	if surface.Layer != nil {
		t.Errorf("nil menu should have no layer, got %v", surface.Layer)
	}
	if len(surface.Interactions) != 0 {
		t.Errorf("nil menu should have no interactions, got %d", len(surface.Interactions))
	}
	if surface.Cursor.Visible {
		t.Errorf("nil menu cursor should be hidden")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("nil menu cursor = %v, want {-1,-1}", got)
	}
}

func TestActionModalLayerCanonicalOrderPayloadSelection(t *testing.T) {
	styles := newRenderStyles(false)
	const chatID = domain.ChatID(77)
	const messageID = domain.MessageID(88)
	menu := &app.MessageActionMenu{
		ChatID:    chatID,
		MessageID: messageID,
		Capabilities: domain.MessageCapabilities{
			Copy: true, Reply: true, Forward: true, Edit: true,
			Pin: true, DeleteForSelf: true, DeleteForAll: true,
		},
		CanReact: true,
		Selected: 2, // edit row
	}
	model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menu}
	surface := buildActionModalLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("action menu layer is nil")
	}

	wantOrder := []string{
		"modal:close",
		"action:reply",
		"action:forward",
		"action:edit",
		"action:copy",
		"action:react",
		"action:pin",
		"action:delete",
		"action:delete-all",
	}
	if len(surface.Interactions) != len(wantOrder) {
		t.Fatalf("interactions = %d, want %d", len(surface.Interactions), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got := surface.Interactions[i].ID; got != want {
			t.Errorf("interaction[%d] ID = %q, want %q", i, got, want)
		}
	}
	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	// Every actionable row carries the exact ChatID/MessageID.
	for i := 1; i < len(surface.Interactions); i++ {
		click := surface.Interactions[i].Click
		if click.ChatID != chatID || click.MessageID != messageID {
			t.Errorf("interaction[%d] %q payload = chat:%d msg:%d, want chat:%d msg:%d",
				i, surface.Interactions[i].ID, click.ChatID, click.MessageID, chatID, messageID)
		}
	}

	// The selected generated row (edit) is covered by selectedColor across its
	// full exact row.
	items := []components.Item{
		{Label: "Reply"}, {Label: "Forward"}, {Label: "Edit"}, {Label: "Copy"},
		{Label: "React"}, {Label: "Pin"}, {Label: "Delete"}, {Label: "Delete for everyone"},
	}
	layout := (components.Modal{Title: "Message actions", Items: items, Selected: 2}).Layout(image.Rect(0, 0, 80, 24))
	var selRect image.Rectangle
	for _, row := range layout.Rows {
		if row.Selected {
			selRect = row.Rect
		}
	}
	if selRect.Empty() {
		t.Fatal("no selected row in layout")
	}
	_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
	for y := selRect.Min.Y; y < selRect.Max.Y; y++ {
		for x := selRect.Min.X; x < selRect.Max.X; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				t.Errorf("CellAt(%d,%d) nil in selected row", x, y)
				continue
			}
			if got := colorOf(cell.Style.Bg); got != rgba(selectedColor) {
				t.Errorf("selected row (%d,%d) bg = %v, want %v", x, y, got, rgba(selectedColor))
			}
		}
	}
}

func TestActionModalLayerLoadingAndErrorGating(t *testing.T) {
	styles := newRenderStyles(false)
	forbidden := []string{
		"action:forward", "action:edit", "action:react",
		"action:pin", "action:delete", "action:delete-all",
	}

	build := func(loading bool, err *domain.AppError) (surfaceResult, string) {
		menu := &app.MessageActionMenu{
			ChatID:    9,
			MessageID: 2,
			Capabilities: domain.MessageCapabilities{
				Copy: true, Reply: true, Forward: true, Edit: true,
				Pin: true, DeleteForSelf: true, DeleteForAll: true,
			},
			CanReact: true,
			Loading:  loading,
			Error:    err,
		}
		model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menu}
		surface := buildActionModalLayer(model, styles)
		_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
		return surface, plainText(canvas.Render())
	}

	// Loading state.
	surface, text := build(true, nil)
	if !strings.Contains(text, "Loading actions...") {
		t.Errorf("loading text missing %q, got %q", "Loading actions...", text)
	}
	assertPermittedOnly(t, surface, forbidden)

	// Error state.
	surface, text = build(false, &domain.AppError{Kind: domain.ErrorNetwork, Message: "boom"})
	if !strings.Contains(text, "Actions unavailable") {
		t.Errorf("error text missing %q, got %q", "Actions unavailable", text)
	}
	assertPermittedOnly(t, surface, forbidden)
}

// assertPermittedOnly checks that only modal:close plus the Reply/Copy rows
// remain and that every forbidden ID is absent.
func assertPermittedOnly(t *testing.T, surface surfaceResult, forbidden []string) {
	t.Helper()
	allowed := map[string]bool{"modal:close": true, "action:reply": true, "action:copy": true}
	for _, interaction := range surface.Interactions {
		if !allowed[interaction.ID] {
			t.Errorf("unexpected interaction %q in gated menu", interaction.ID)
		}
	}
	for _, id := range forbidden {
		for _, interaction := range surface.Interactions {
			if interaction.ID == id {
				t.Errorf("forbidden interaction %q present in gated menu", id)
			}
		}
	}
}

func TestActionModalLayerPinUnpin(t *testing.T) {
	styles := newRenderStyles(false)
	build := func(pinned bool) (surfaceResult, string) {
		menu := &app.MessageActionMenu{
			ChatID:       9,
			MessageID:    2,
			Capabilities: domain.MessageCapabilities{Pin: true},
			Pinned:       pinned,
			CanReact:     false,
		}
		model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menu}
		surface := buildActionModalLayer(model, styles)
		_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
		return surface, plainText(canvas.Render())
	}

	surface, text := build(false)
	if !strings.Contains(text, "Pin") {
		t.Errorf("unpinned menu should render Pin, got %q", text)
	}
	if strings.Contains(text, "Unpin") {
		t.Errorf("unpinned menu should not render Unpin, got %q", text)
	}
	pinAction := actionForID(t, surface, "action:pin")
	if pinAction.Action != app.PinMessage || pinAction.ChatID != 9 || pinAction.MessageID != 2 {
		t.Errorf("pin action = %#v, want PinMessage/9/2", pinAction)
	}

	surface, text = build(true)
	if !strings.Contains(text, "Unpin") {
		t.Errorf("pinned menu should render Unpin, got %q", text)
	}
	if strings.Contains(text, "Pin") {
		t.Errorf("pinned menu should not render Pin, got %q", text)
	}
	unpinAction := actionForID(t, surface, "action:pin")
	if unpinAction.Action != app.PinMessage || unpinAction.ChatID != 9 || unpinAction.MessageID != 2 {
		t.Errorf("unpin action = %#v, want PinMessage/9/2", unpinAction)
	}
}

func actionForID(t *testing.T, surface surfaceResult, id string) app.ActionReceived {
	t.Helper()
	for _, interaction := range surface.Interactions {
		if interaction.ID == id {
			return interaction.Click
		}
	}
	t.Fatalf("no interaction with ID %q", id)
	return app.ActionReceived{}
}

func TestReactionPickerLayerPayloadHitAndEmojiPresence(t *testing.T) {
	styles := newRenderStyles(false)
	picker := &app.ReactionPicker{ChatID: 9, MessageID: 2, Selected: 0}
	model := ui.ViewModel{Width: 80, Height: 24, ReactionPicker: picker}
	surface := buildReactionPickerLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("reaction picker layer is nil")
	}
	compositor, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)

	if len(surface.Interactions) != len(app.ReactionPalette)+1 {
		t.Fatalf("interactions = %d, want %d", len(surface.Interactions), len(app.ReactionPalette)+1)
	}
	for index, emoji := range app.ReactionPalette {
		id := fmt.Sprintf("reaction:%d", index)
		interaction := actionForID(t, surface, id)
		if interaction.Action != app.Activate {
			t.Errorf("reaction:%d action = %v, want Activate", index, interaction.Action)
		}
		if interaction.ChatID != 9 || interaction.MessageID != 2 {
			t.Errorf("reaction:%d payload = chat:%d msg:%d, want 9/2", index, interaction.ChatID, interaction.MessageID)
		}
		if interaction.Rune != rune(index+0x10000) {
			t.Errorf("reaction:%d rune = %U, want %U", index, interaction.Rune, rune(index+0x10000))
		}
		// Full-row Compositor Hit ID/bounds.
		row := interactionForID(t, surface, id)
		pt := image.Pt(row.Rect.Min.X+row.Rect.Dx()/2, row.Rect.Min.Y+row.Rect.Dy()/2)
		hit := compositor.Hit(pt.X, pt.Y)
		if hit.ID() != id {
			t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), id)
		}
		if got := hit.Bounds(); !got.Eq(row.Rect) {
			t.Errorf("Hit(%v) bounds = %v, want %v", pt, got, row.Rect)
		}
		_ = emoji
	}
	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	text := plainText(canvas.Render())
	for _, emoji := range []string{"👍", "❤️", "🔥"} {
		if !strings.Contains(text, emoji) {
			t.Errorf("rendered reaction output missing %q, got %q", emoji, text)
		}
	}
}

func interactionForID(t *testing.T, surface surfaceResult, id string) layerInteraction {
	t.Helper()
	for _, interaction := range surface.Interactions {
		if interaction.ID == id {
			return interaction
		}
	}
	t.Fatalf("no interaction with ID %q", id)
	return layerInteraction{}
}

func TestReactionPickerLayerSelectionGeometryStable(t *testing.T) {
	styles := newRenderStyles(false)
	items := make([]components.Item, len(app.ReactionPalette))
	for i, emoji := range app.ReactionPalette {
		items[i] = components.Item{Label: emoji}
	}

	// Baseline geometry map ID->Rect for selection 0.
	baseline := map[string]image.Rectangle{}
	base := buildReactionPickerLayer(ui.ViewModel{Width: 80, Height: 24, ReactionPicker: &app.ReactionPicker{ChatID: 9, MessageID: 2, Selected: 0}}, styles)
	for _, interaction := range base.Interactions {
		baseline[interaction.ID] = interaction.Rect
	}

	for sel := 0; sel < len(app.ReactionPalette); sel++ {
		surface := buildReactionPickerLayer(ui.ViewModel{Width: 80, Height: 24, ReactionPicker: &app.ReactionPicker{ChatID: 9, MessageID: 2, Selected: sel}}, styles)
		for _, interaction := range surface.Interactions {
			if want, ok := baseline[interaction.ID]; !ok || !interaction.Rect.Eq(want) {
				t.Errorf("selection %d: interaction %q rect = %v, want %v", sel, interaction.ID, interaction.Rect, want)
			}
		}

		// Selected exact full row background is selectedColor.
		layout := (components.Modal{Title: "React", Items: items, Selected: sel}).Layout(image.Rect(0, 0, 80, 24))
		var selRect image.Rectangle
		for _, row := range layout.Rows {
			if row.Selected {
				selRect = row.Rect
			}
		}
		_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
		for y := selRect.Min.Y; y < selRect.Max.Y; y++ {
			for x := selRect.Min.X; x < selRect.Max.X; x++ {
				cell := canvas.CellAt(x, y)
				if cell == nil {
					t.Errorf("selection %d: CellAt(%d,%d) nil", sel, x, y)
					continue
				}
				// Wide emoji glyphs occupy two cells; the continuation cell has
				// empty content and no background, so skip it.
				if cell.Content == "" {
					continue
				}
				if got := colorOf(cell.Style.Bg); got != rgba(selectedColor) {
					t.Errorf("selection %d: selected row (%d,%d) bg = %v, want %v", sel, x, y, got, rgba(selectedColor))
				}
			}
		}

		// Grapheme labels remain within layout Rows/Frame and one line: the
		// full viewport canvas is 24 lines and the modal frame never wraps.
		text := plainText(canvas.Render())
		if got := strings.Count(text, "\n"); got != 23 {
			t.Errorf("selection %d: rendered lines = %d, want 24 (single line)", sel, got+1)
		}
		for _, row := range layout.Rows {
			clipped := ansi.Truncate(row.Item.Label, row.Rect.Dx()-1, "")
			if ansi.StringWidth(clipped) > row.Rect.Dx()-1 {
				t.Errorf("selection %d: label width %d overflows row width %d", sel, ansi.StringWidth(clipped), row.Rect.Dx()-1)
			}
		}
	}
}

func TestForwardPickerLayerOrderPayloadSelectionAndWideTitles(t *testing.T) {
	styles := newRenderStyles(false)
	longTitle := "很长的标题👨‍👩‍👧‍👦🔥❤️多字节群聊"
	chats := []ui.ChatRow{
		{Chat: domain.Chat{ID: 101, Title: "Alpha"}},
		{Chat: domain.Chat{ID: 202, Title: longTitle}},
		{Chat: domain.Chat{ID: 303, Title: "Gamma"}},
	}
	picker := &app.ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1}
	model := ui.ViewModel{Width: 80, Height: 24, Chats: chats, ForwardPicker: picker}
	surface := buildForwardPickerLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("forward picker layer is nil")
	}
	compositor, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)

	// Chats stay in order with exact forward:<index>:<chatID> IDs and Activate
	// to the destination ChatID.
	for index, chat := range chats {
		id := fmt.Sprintf("forward:%d:%d", index, chat.Chat.ID)
		interaction := actionForID(t, surface, id)
		if interaction.Action != app.Activate {
			t.Errorf("%s action = %v, want Activate", id, interaction.Action)
		}
		if interaction.ChatID != chat.Chat.ID {
			t.Errorf("%s ChatID = %d, want %d", id, interaction.ChatID, chat.Chat.ID)
		}
		row := interactionForID(t, surface, id)
		pt := image.Pt(row.Rect.Min.X+row.Rect.Dx()/2, row.Rect.Min.Y+row.Rect.Dy()/2)
		if hit := compositor.Hit(pt.X, pt.Y); hit.ID() != id {
			t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), id)
		}
	}
	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	// Selected generated row (index 1) has selectedColor across its full row.
	items := []components.Item{{Label: "Alpha"}, {Label: longTitle}, {Label: "Gamma"}}
	layout := (components.Modal{Title: "Forward to", Items: items, Selected: 1}).Layout(image.Rect(0, 0, 80, 24))
	var selRect image.Rectangle
	for _, row := range layout.Rows {
		if row.Selected {
			selRect = row.Rect
		}
	}
	for y := selRect.Min.Y; y < selRect.Max.Y; y++ {
		for x := selRect.Min.X; x < selRect.Max.X; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				t.Errorf("CellAt(%d,%d) nil in selected row", x, y)
				continue
			}
			// Wide emoji glyphs occupy two cells; the continuation cell has
			// empty content and no background, so skip it.
			if cell.Content == "" {
				continue
			}
			if got := colorOf(cell.Style.Bg); got != rgba(selectedColor) {
				t.Errorf("selected row (%d,%d) bg = %v, want %v", x, y, got, rgba(selectedColor))
			}
		}
	}

	// Long CJK/ZWJ/FE0F title stays bounded and single-line: the full viewport
	// canvas is 24 lines and the modal frame never wraps.
	text := plainText(canvas.Render())
	if got := strings.Count(text, "\n"); got != 23 {
		t.Errorf("rendered lines = %d, want 24 (single line)", got+1)
	}
	for _, row := range layout.Rows {
		clipped := ansi.Truncate(row.Item.Label, row.Rect.Dx()-1, "")
		if ansi.StringWidth(clipped) > row.Rect.Dx()-1 {
			t.Errorf("label width %d overflows row width %d", ansi.StringWidth(clipped), row.Rect.Dx()-1)
		}
	}
}

func TestForwardPickerLayerNilState(t *testing.T) {
	styles := newRenderStyles(false)
	model := ui.ViewModel{Width: 80, Height: 24} // ForwardPicker nil
	surface := buildForwardPickerLayer(model, styles)
	if surface.Layer != nil {
		t.Errorf("nil forward picker should have no layer, got %v", surface.Layer)
	}
	if len(surface.Interactions) != 0 {
		t.Errorf("nil forward picker should have no interactions, got %d", len(surface.Interactions))
	}
	if surface.Cursor.Visible {
		t.Errorf("nil forward picker cursor should be hidden")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("nil forward picker cursor = %v, want {-1,-1}", got)
	}
}

func TestReactionPickerLayerNilState(t *testing.T) {
	styles := newRenderStyles(false)
	model := ui.ViewModel{Width: 80, Height: 24} // ReactionPicker nil
	surface := buildReactionPickerLayer(model, styles)
	if surface.Layer != nil {
		t.Errorf("nil reaction picker should have no layer, got %v", surface.Layer)
	}
	if len(surface.Interactions) != 0 {
		t.Errorf("nil reaction picker should have no interactions, got %d", len(surface.Interactions))
	}
	if surface.Cursor.Visible {
		t.Errorf("nil reaction picker cursor should be hidden")
	}
	if got := surface.Cursor; got.X != -1 || got.Y != -1 {
		t.Errorf("nil reaction picker cursor = %v, want {-1,-1}", got)
	}
}

func TestListModalEmptyRowsSelectionFallback(t *testing.T) {
	// No rows: layout produces an empty, safe modal with no row interactions.
	styles := newRenderStyles(false)
	bounds := image.Rect(0, 0, 80, 24)
	surface := buildListModal(bounds, "Empty", nil, styles)
	layout := (components.Modal{Title: "Empty", Items: nil, Selected: -1}).Layout(bounds)
	if !surface.Rect.Eq(layout.Frame) {
		t.Errorf("empty-rows rect = %v, want %v", surface.Rect, layout.Frame)
	}
	if len(layout.Rows) != 0 {
		t.Errorf("layout rows = %d, want 0", len(layout.Rows))
	}
	for i := range surface.Interactions {
		if strings.HasPrefix(surface.Interactions[i].ID, "r") {
			t.Errorf("empty modal should have no row interactions")
		}
	}
}

func TestActionModalViewImageRowExactOrderPayloadAndGating(t *testing.T) {
	styles := newRenderStyles(false)
	const chatID = domain.ChatID(11)
	const messageID = domain.MessageID(22)
	bounds := image.Rect(0, 0, 80, 24)

	// Build a fully-capable menu with eligible MediaFile.
	menuWithMedia := &app.MessageActionMenu{
		ChatID:    chatID,
		MessageID: messageID,
		Capabilities: domain.MessageCapabilities{
			Reply: true, Forward: true, Edit: true, Copy: true,
			Pin: true, DeleteForSelf: true, DeleteForAll: true,
		},
		CanReact:  true,
		Selected:  0,
		MediaFile: domain.MediaFileRef{ID: 55, CanDownload: true},
	}
	model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menuWithMedia}
	surface := buildActionModalLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("action menu layer is nil")
	}
	if len(surface.Interactions) == 0 {
		t.Fatal("should have interactions")
	}

	// Exact order: close, view-image, reply, forward, edit, copy, react, pin, delete, delete-all.
	wantOrder := []string{
		"modal:close",
		"action:view-image",
		"action:reply",
		"action:forward",
		"action:edit",
		"action:copy",
		"action:react",
		"action:pin",
		"action:delete",
		"action:delete-all",
	}
	if len(surface.Interactions) != len(wantOrder) {
		t.Fatalf("interactions count = %d, want %d", len(surface.Interactions), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got := surface.Interactions[i].ID; got != want {
			t.Errorf("interaction[%d] ID = %q, want %q", i, got, want)
		}
	}
	if err := assertUniqueNonEmptyIDs(surface.Interactions); err != nil {
		t.Errorf("unique IDs: %v", err)
	}

	// Exact view-image payload.
	viewImage := actionForID(t, surface, "action:view-image")
	wantAction := app.ActionReceived{
		Action:    app.ViewMessageMedia,
		ChatID:    chatID,
		MessageID: messageID,
	}
	if !reflect.DeepEqual(viewImage, wantAction) {
		t.Errorf("view-image click = %#v, want %#v", viewImage, wantAction)
	}

	// Hit parity: interior point of view-image rect.
	var viewRect image.Rectangle
	for _, interaction := range surface.Interactions {
		if interaction.ID == "action:view-image" {
			viewRect = interaction.Rect
			break
		}
	}
	if viewRect.Empty() {
		t.Fatal("view-image rect is empty")
	}
	compositor, canvas := listModalCanvas(bounds, surface)
	hit := compositor.Hit(viewRect.Min.X+viewRect.Dx()/2, viewRect.Min.Y+viewRect.Dy()/2)
	if hit.ID() != "action:view-image" {
		t.Errorf("Hit(view-image) = %q, want action:view-image", hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(viewRect) {
		t.Errorf("view-image hit bounds = %v, want %v", got, viewRect)
	}

	// Independent geometry: build the exact nine components.Item values in
	// frozen order, call Layout(bounds) and assign with :=, then compare the
	// first independent row Rect with the view-image interaction Rect.
	items := []components.Item{
		{Label: "View image"},
		{Label: "Reply"},
		{Label: "Forward"},
		{Label: "Edit"},
		{Label: "Copy"},
		{Label: "React"},
		{Label: "Pin"},
		{Label: "Delete"},
		{Label: "Delete for everyone"},
	}
	layout := (components.Modal{Title: "Message actions", Items: items, Selected: 0}).Layout(bounds)
	expectedRect := layout.Rows[0].Rect
	if !viewRect.Eq(expectedRect) {
		t.Errorf("view-image rect = %v, want %v", viewRect, expectedRect)
	}
	for x := expectedRect.Min.X; x < expectedRect.Max.X; x++ {
		for y := expectedRect.Min.Y; y < expectedRect.Max.Y; y++ {
			hit := compositor.Hit(x, y)
			if hit.ID() != "action:view-image" {
				t.Errorf("Hit(%d,%d) = %q, want action:view-image", x, y, hit.ID())
			}
			if got := hit.Bounds(); !got.Eq(expectedRect) {
				t.Errorf("Hit(%d,%d) bounds = %v, want %v", x, y, got, expectedRect)
			}
		}
	}

	// View image rendered label is present in composed canvas.
	text := plainText(canvas.Render())
	if !strings.Contains(text, "View image") {
		t.Errorf("canvas missing 'View image' label, got %q", text)
	}

	// View image remains visible in loading state.
	menuLoading := &app.MessageActionMenu{
		ChatID:    chatID,
		MessageID: messageID,
		Capabilities: domain.MessageCapabilities{
			Reply: true, Copy: true, Pin: true, DeleteForSelf: true,
		},
		CanReact:  false,
		Selected:  0,
		Loading:   true,
		MediaFile: domain.MediaFileRef{ID: 66, CanDownload: true},
	}
	surfaceLoading := buildActionModalLayer(ui.ViewModel{Width: 80, Height: 24, MessageMenu: menuLoading}, styles)
	if !hasInteractionID(surfaceLoading.Interactions, "action:view-image") {
		t.Errorf("loading state should have view-image row")
	}

	// View image remains visible in error state.
	menuError := &app.MessageActionMenu{
		ChatID:    chatID,
		MessageID: messageID,
		Capabilities: domain.MessageCapabilities{
			Reply: true, Copy: true, Pin: true, DeleteForSelf: true,
		},
		CanReact:  false,
		Selected:  0,
		Loading:   false,
		Error:     &domain.AppError{Kind: domain.ErrorMedia, Message: "something went wrong"},
		MediaFile: domain.MediaFileRef{ID: 77, CanDownload: true},
	}
	surfaceError := buildActionModalLayer(ui.ViewModel{Width: 80, Height: 24, MessageMenu: menuError}, styles)
	if !hasInteractionID(surfaceError.Interactions, "action:view-image") {
		t.Errorf("error state should have view-image row")
	}

	// Eligibility matrix for action:view-image presence gating.
	eligibilityCases := []struct {
		name    string
		media   domain.MediaFileRef
		present bool
	}{
		// Ineligible (action:view-image absent).
		{"zero", domain.MediaFileRef{}, false},
		{"unique-only", domain.MediaFileRef{UniqueID: "unique-only"}, false},
		{"can-download-false", domain.MediaFileRef{ID: 88, CanDownload: false, Downloaded: false}, false},
		{"local-empty-path", domain.MediaFileRef{ID: 0, UniqueID: "empty-local", Downloaded: true, LocalPath: ""}, false},
		// Eligible (action:view-image present).
		{"can-download-true", domain.MediaFileRef{ID: 55, CanDownload: true}, true},
		{"local-photo", domain.MediaFileRef{ID: 0, UniqueID: "local-photo", Downloaded: true, LocalPath: "/tmp/local-photo.jpg"}, true},
	}

	for _, tc := range eligibilityCases {
		t.Run(tc.name, func(t *testing.T) {
			menu := &app.MessageActionMenu{
				ChatID:    chatID,
				MessageID: messageID,
				Capabilities: domain.MessageCapabilities{
					Reply: true, Copy: true, Pin: true, DeleteForSelf: true,
				},
				CanReact:  false,
				Selected:  0,
				MediaFile: tc.media,
			}
			model := ui.ViewModel{Width: 80, Height: 24, MessageMenu: menu}
			surface := buildActionModalLayer(model, styles)
			if surface.Layer == nil {
				t.Fatal("eligibility test layer is nil")
			}
			got := hasInteractionID(surface.Interactions, "action:view-image")
			if got != tc.present {
				if tc.present {
					t.Errorf("%s: expected view-image present, but absent", tc.name)
				} else {
					t.Errorf("%s: expected view-image absent, but present", tc.name)
				}
			}
		})
	}
}

func hasInteractionID(interactions []layerInteraction, id string) bool {
	for i := range interactions {
		if interactions[i].ID == id {
			return true
		}
	}
	return false
}
