package frontend

import (
	"image"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// chatSurfaceCanvas composes a chats surface under a viewport root and returns
// the canvas plus the compositor for hit testing.
func chatSurfaceCanvas(model ViewModel, surface surfaceResult) (*lipgloss.Canvas, *lipgloss.Compositor) {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor), compositor
}

// chatRowSurface builds a standalone chat row surface at its absolute rect and
// composes it under a viewport root, returning the canvas, compositor, and the
// raw surface result.
func chatRowSurface(row ChatRow, rect image.Rectangle, location *time.Location, styles renderStyles) (*lipgloss.Canvas, *lipgloss.Compositor, surfaceResult) {
	surface := buildChatRowLayer(row, rect, location, styles)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(rect.Max.X + 2).Height(rect.Max.Y + 2).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(rect.Max.X+2, rect.Max.Y+2).Compose(compositor), compositor, surface
}

func chatRowSurfaceRender(row ChatRow, rect image.Rectangle, location *time.Location, styles renderStyles) string {
	surface := buildChatRowLayer(row, rect, location, styles)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(rect.Max.X + 2).Height(rect.Max.Y + 2).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	return lipgloss.NewCompositor(root).Render()
}

func chatTestModel(width, height int, chats []ChatRow, loading, loaded bool, chatErr *domain.AppError) ViewModel {
	return ViewModel{
		Width:        width,
		Height:       height,
		Layout:       ComputeLayout(width, height, false, FocusChats),
		Focus:        FocusChats,
		Chats:        chats,
		ChatsLoading: loading,
		ChatsLoaded:  loaded,
		ChatsError:   chatErr,
	}
}

func chatRowFor(title string, id int, selected bool) ChatRow {
	return ChatRow{
		Chat: domain.Chat{
			ID:            domain.ChatID(id),
			Title:         title,
			LastMessage:   "preview",
			LastMessageAt: time.Date(2026, time.July, 20, 13, 2, 0, 0, time.Local).Unix(),
			UnreadCount:   3,
			Muted:         true,
		},
		Selected:  selected,
		AvatarKey: "key",
	}
}

func cellContent(c *lipgloss.Canvas, x, y int) string {
	if cell := c.CellAt(x, y); cell != nil {
		return cell.Content
	}
	return ""
}

func TestChatRowSelectedUnselectedDimensionsAndContent(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(2, 3, 20, 7) // 18x4 row

	// Unselected.
	unselected := chatRowFor("Mina Chen", 1, false)
	uCanvas, _, uSurface := chatRowSurface(unselected, rect, time.Local, styles)
	if uSurface.Layer == nil {
		t.Fatal("unselected row layer is nil")
	}
	if got := uSurface.Layer.Width(); got != rect.Dx() {
		t.Errorf("unselected width = %d, want %d", got, rect.Dx())
	}
	if got := uSurface.Layer.Height(); got != rect.Dy() {
		t.Errorf("unselected height = %d, want %d", got, rect.Dy())
	}
	if got := uSurface.Layer.GetX(); got != rect.Min.X {
		t.Errorf("unselected X = %d, want %d", got, rect.Min.X)
	}
	if got := uSurface.Layer.GetY(); got != rect.Min.Y {
		t.Errorf("unselected Y = %d, want %d", got, rect.Min.Y)
	}
	if got := uSurface.Layer.GetZ(); got != zRowBackground {
		t.Errorf("unselected Z = %d, want %d", got, zRowBackground)
	}
	if got := uSurface.Layer.GetID(); got != "chat:1" {
		t.Errorf("unselected ID = %q, want chat:1", got)
	}
	if got := uCanvas.CellAt(rect.Min.X+8, rect.Min.Y).Style.Bg; got != nil {
		t.Errorf("unselected background = %v, want nil terminal background", colorOf(got))
	}
	text := plainText(uCanvas.Render())
	for _, want := range []string{"Mina Chen", "preview", "13:02", "[3]"} {
		if !strings.Contains(text, want) {
			t.Errorf("unselected row missing %q, got %q", want, text)
		}
	}

	// Focused but not selected: the title carries the focus accent while the
	// row remains on the terminal background.
	focused := chatRowFor("Focused", 3, false)
	focused.Focused = true
	fCanvas, _, _ := chatRowSurface(focused, rect, time.Local, styles)
	if got := colorOf(fCanvas.CellAt(rect.Min.X+7, rect.Min.Y).Style.Fg); got != rgba(accentColor) {
		t.Errorf("focused title foreground = %v, want accentColor", got)
	}
	if got := fCanvas.CellAt(rect.Min.X+8, rect.Min.Y).Style.Bg; got != nil {
		t.Errorf("focused unselected background = %v, want nil", colorOf(got))
	}

	// Selected.
	selected := chatRowFor("Weekend dev", 2, true)
	sCanvas, _, sSurface := chatRowSurface(selected, rect, time.Local, styles)
	if got := sSurface.Layer.GetID(); got != "chat:2" {
		t.Errorf("selected ID = %q, want chat:2", got)
	}
	if got := colorOf(sCanvas.CellAt(rect.Min.X+8, rect.Min.Y).Style.Bg); got != rgba(selectedColor) {
		t.Errorf("selected background = %v, want selectedColor", got)
	}
	titleCell := sCanvas.CellAt(rect.Min.X+7, rect.Min.Y)
	if titleCell.Style.Attrs&uv.AttrBold == 0 {
		t.Errorf("selected title cell is not bold")
	}
}

func TestChatRowDraftReplacesPreviewWithoutExposingReplyIdentity(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 36, 4)
	row := chatRowFor("Mina Chen", 7, false)
	row.Draft = domain.Draft{Text: "  cloud\n draft  ", ReplyToMessageID: 987654321, Date: time.Date(2026, time.July, 20, 14, 35, 0, 0, time.Local).Unix()}
	plain := plainText(chatRowSurfaceRender(row, rect, time.Local, styles))
	if !strings.Contains(plain, "Draft: cloud draft") || !strings.Contains(plain, "14:35") {
		t.Fatalf("draft row = %q", plain)
	}
	if strings.Contains(plain, "preview") || strings.Contains(plain, "987654321") {
		t.Fatalf("draft row exposed replaced preview or reply identity: %q", plain)
	}

	row.Draft = domain.Draft{ReplyToMessageID: 44}
	plain = plainText(chatRowSurfaceRender(row, rect, time.Local, styles))
	if !strings.Contains(plain, "Draft: Replying to message") || strings.Contains(plain, "44") {
		t.Fatalf("reply-only draft row = %q", plain)
	}
}

func TestChatRowFullHitActionAndWheel(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(3, 2, 21, 6)
	row := chatRowFor("Mina Chen", 7, false)
	_, compositor, surface := chatRowSurface(row, rect, time.Local, styles)
	if len(surface.Interactions) != 1 {
		t.Fatalf("interactions = %d, want 1", len(surface.Interactions))
	}
	interaction := surface.Interactions[0]
	if interaction.ID != "chat:7" {
		t.Errorf("interaction ID = %q, want chat:7", interaction.ID)
	}
	if !interaction.Rect.Eq(rect) {
		t.Errorf("interaction rect = %v, want %v", interaction.Rect, rect)
	}
	if interaction.Click.Action != FocusChat || interaction.Click.ChatID != 7 {
		t.Errorf("click = %#v, want FocusChat/7", interaction.Click)
	}
	if interaction.WheelUp.Action != SelectPrevious {
		t.Errorf("wheel up = %#v, want SelectPrevious", interaction.WheelUp)
	}
	if interaction.WheelDown.Action != SelectNext {
		t.Errorf("wheel down = %#v, want SelectNext", interaction.WheelDown)
	}

	pt := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != interaction.ID {
		t.Errorf("Hit(%v) = %q, want %q", pt, hit.ID(), interaction.ID)
	}
	if got := hit.Bounds(); !got.Eq(interaction.Rect) {
		t.Errorf("hit bounds = %v, want %v", got, interaction.Rect)
	}
}

func TestChatRowAvatarRetryWinsOverRow(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(2, 3, 20, 7) // 18x4
	row := chatRowFor("Mina Chen", 9, false)
	row.AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "failed"}

	canvas, compositor, surface := chatRowSurface(row, rect, time.Local, styles)
	if len(surface.Interactions) != 2 {
		t.Fatalf("interactions = %d, want 2", len(surface.Interactions))
	}
	var retry *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "chat-avatar-retry:9" {
			retry = &surface.Interactions[i]
		}
	}
	if retry == nil {
		t.Fatal("no avatar retry interaction")
	}
	avatarRect := image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+6, rect.Min.Y+3)
	if !retry.Rect.Eq(avatarRect) {
		t.Errorf("retry rect = %v, want %v", retry.Rect, avatarRect)
	}
	if retry.Z != zControl {
		t.Errorf("retry Z = %d, want %d", retry.Z, zControl)
	}
	if retry.Click.Action != Retry || retry.Click.AvatarKey != "key" {
		t.Errorf("retry click = %#v, want Retry/key", retry.Click)
	}

	// At a point inside the avatar bounds, the retry wins over the row.
	pt := image.Pt(avatarRect.Min.X+2, avatarRect.Min.Y+1)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "chat-avatar-retry:9" {
		t.Errorf("Hit(%v) = %q, want chat-avatar-retry:9", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(avatarRect) {
		t.Errorf("retry hit bounds = %v, want %v", got, avatarRect)
	}
	// Retry is intrinsic: the remaining bottom-row avatar cell is preserved.
	if got := canvas.CellAt(avatarRect.Max.X-1, avatarRect.Max.Y-1).Content; got != "░" {
		t.Errorf("avatar cell after Retry = %q, want preserved ░", got)
	}

	text := plainText(chatRowSurfaceRender(row, rect, time.Local, styles))
	if !strings.Contains(text, "Retry") {
		t.Errorf("row missing Retry text, got %q", text)
	}
}

func TestChatRowAvatarContentSurvivesNesting(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 18, 4)
	row := chatRowFor("Mina Chen", 3, false)
	surface := buildChatRowLayer(row, rect, time.Local, styles)
	surface.Layer.X(0).Y(0)
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(18).Height(4).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	canvas := lipgloss.NewCanvas(18, 4).Compose(lipgloss.NewCompositor(root))

	if cell := canvas.CellAt(1, 1); cell == nil || cell.Content != "░" {
		t.Errorf("nested avatar cell (1,1) = %q, want ░", cellContent(canvas, 1, 1))
	}
	if cell := canvas.CellAt(7, 0); cell == nil {
		t.Errorf("nested title cell (7,0) is nil")
	}
}

func TestChatsPaneFocusHitRoundedTitleAndStates(t *testing.T) {
	styles := newRenderStyles(false)

	model := chatTestModel(100, 24, []ChatRow{
		chatRowFor("Mina Chen", 1, false),
		chatRowFor("Weekend dev", 2, true),
	}, false, true, nil)
	surface := buildChatsLayer(model, time.Local, styles)
	if surface.Layer == nil {
		t.Fatal("chats layer is nil")
	}
	if surface.Layer.GetID() != "pane:chats" {
		t.Errorf("pane root ID = %q, want pane:chats", surface.Layer.GetID())
	}
	if !surface.Rect.Eq(model.Layout.Chats) {
		t.Errorf("surface rect = %v, want %v", surface.Rect, model.Layout.Chats)
	}
	canvas, compositor := chatSurfaceCanvas(model, surface)

	var paneHit *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "pane:chats" {
			paneHit = &surface.Interactions[i]
		}
	}
	if paneHit == nil {
		t.Fatal("no pane interaction")
	}
	if paneHit.Click.Action != FocusPane || paneHit.Click.TargetFocus != FocusChats {
		t.Errorf("pane click = %#v", paneHit.Click)
	}
	text := plainText(canvas.Render())
	if !strings.Contains(text, "Chats") {
		t.Errorf("pane missing title, got %q", text)
	}
	if got := canvas.CellAt(model.Layout.Chats.Min.X, model.Layout.Chats.Min.Y).Content; got != "╭" {
		t.Errorf("top-left corner = %q, want ╭", got)
	}
	if got := colorOf(canvas.CellAt(model.Layout.Chats.Min.X, model.Layout.Chats.Min.Y).Style.Fg); got != rgba(focusedBorderColor) {
		t.Errorf("focused border = %v, want focusedBorderColor", got)
	}

	// Compositor.Hit parity for a chat row inside the pane.
	pt := image.Pt(model.Layout.Chats.Min.X+10, model.Layout.Chats.Min.Y+1+4+2)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "chat:2" {
		t.Errorf("Hit(%v) = %q, want chat:2", pt, hit.ID())
	}
}

func TestChatsPaneLoadingErrorEmpty(t *testing.T) {
	styles := newRenderStyles(false)

	loading := chatTestModel(100, 24, nil, true, false, nil)
	ls := buildChatsLayer(loading, time.Local, styles)
	lCanvas, _ := chatSurfaceCanvas(loading, ls)
	if !strings.Contains(plainText(lCanvas.Render()), "Loading chats...") {
		t.Errorf("loading state missing text")
	}

	errModel := chatTestModel(100, 24, nil, false, false, &domain.AppError{Kind: domain.ErrorInternal, Message: "boom"})
	es := buildChatsLayer(errModel, time.Local, styles)
	eCanvas, _ := chatSurfaceCanvas(errModel, es)
	if !strings.Contains(plainText(eCanvas.Render()), "boom") {
		t.Errorf("error state missing message")
	}

	empty := chatTestModel(100, 24, nil, false, true, nil)
	es2 := buildChatsLayer(empty, time.Local, styles)
	e2Canvas, _ := chatSurfaceCanvas(empty, es2)
	if !strings.Contains(plainText(e2Canvas.Render()), "No chats") {
		t.Errorf("empty state missing text")
	}
}

func TestChatsPaneCapacityAndStartKeepsSelectedVisible(t *testing.T) {
	styles := newRenderStyles(false)
	// 100x24 normal layout: Chats = (0,1,30,24), inner height 22, row height 4
	// -> capacity 5. Build 8 chats with the selected one at index 6.
	chats := make([]ChatRow, 8)
	for i := range chats {
		chats[i] = chatRowFor("Chat "+string(rune('A'+i)), i+1, false)
	}
	chats[6].Selected = true
	model := chatTestModel(100, 24, chats, false, true, nil)
	surface := buildChatsLayer(model, time.Local, styles)
	canvas, _ := chatSurfaceCanvas(model, surface)
	text := plainText(canvas.Render())

	for _, want := range []string{"Chat C", "Chat D", "Chat E", "Chat F", "Chat G"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in rendered rows", want)
		}
	}
	for _, not := range []string{"Chat A", "Chat B", "Chat H"} {
		if strings.Contains(text, not) {
			t.Errorf("row %q should be outside the window", not)
		}
	}
	inner := model.Layout.Chats.Inset(1)
	rowsTop := inner.Min.Y
	start := 2
	selectedY := rowsTop + (6-start)*chatRowHeight
	cell := canvas.CellAt(model.Layout.Chats.Min.X+10, selectedY)
	if got := colorOf(cell.Style.Bg); got != rgba(selectedColor) {
		t.Errorf("selected row background = %v, want selectedColor", got)
	}
}

func TestChatRowLongEmojiTitleCannotEscapeRowWidth(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 12, 4)
	row := chatRowFor("👨‍👩‍👧‍👦 界界界界界界界界界界界界", 5, false)
	row.Chat.LastMessage = "🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀"
	content := chatRowSurfaceRender(row, rect, time.Local, styles)
	for _, line := range strings.Split(content, "\n") {
		if got := displayWidth(line); got > 12 {
			t.Errorf("row line width = %d, want <= 12: %q", got, line)
		}
	}
}

func TestChatRowNilLocationHelperBehavior(t *testing.T) {
	styles := newRenderStyles(false)
	rect := image.Rect(0, 0, 24, 4)
	row := chatRowFor("Mina Chen", 1, false)
	row.Chat.LastMessageAt = time.Date(2026, time.July, 20, 13, 2, 0, 0, time.UTC).Unix()
	row.Chat.UnreadMentionCount = 2
	content := chatRowSurfaceRender(row, rect, nil, styles)
	plain := plainText(content)
	if !strings.Contains(plain, "[3]") {
		t.Errorf("metadata missing unread marker")
	}
	if !strings.Contains(plain, "[@2]") {
		t.Errorf("metadata missing unread mention marker")
	}
}
