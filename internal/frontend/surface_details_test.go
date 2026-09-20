package frontend

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// detailsSurfaceCanvas composes a details surface under a viewport root and
// returns the canvas plus the compositor for hit testing.
func detailsSurfaceCanvas(model ViewModel, surface surfaceResult) (*lipgloss.Canvas, *lipgloss.Compositor) {
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(surface.Layer)
	compositor := lipgloss.NewCompositor(root)
	return lipgloss.NewCanvas(model.Width, model.Height).Compose(compositor), compositor
}

// detailsModel builds a wide-layout viewmodel with details open.
func detailsModel(width, height int, active domain.Chat, row ChatRow) ViewModel {
	return ViewModel{
		Width:      width,
		Height:     height,
		Layout:     ComputeLayout(width, height, true, FocusDetails),
		Focus:      FocusDetails,
		ActiveChat: active,
		Chats:      []ChatRow{row},
	}
}

func detailsRowFor(chat domain.Chat) ChatRow {
	return ChatRow{Chat: chat, AvatarKey: "key"}
}

func TestDetailsPaneCloseIDsActionsAndHitParity(t *testing.T) {
	styles := newRenderStyles(false)
	active := domain.Chat{ID: 5, Title: "Mina Chen", Username: "mina"}
	model := detailsModel(140, 30, active, detailsRowFor(active))
	surface := buildDetailsLayer(model, styles)
	if surface.Layer == nil {
		t.Fatal("details layer is nil")
	}
	if surface.Layer.GetID() != "pane:details" {
		t.Errorf("pane root ID = %q, want pane:details", surface.Layer.GetID())
	}
	canvas, compositor := detailsSurfaceCanvas(model, surface)

	// Pane interaction.
	var paneHit *layerInteraction
	var closeHit *layerInteraction
	for i := range surface.Interactions {
		switch surface.Interactions[i].ID {
		case "pane:details":
			paneHit = &surface.Interactions[i]
		case "details:close":
			closeHit = &surface.Interactions[i]
		}
	}
	if paneHit == nil {
		t.Fatal("no pane interaction")
	}
	if paneHit.Click.Action != FocusPane || paneHit.Click.TargetFocus != FocusDetails {
		t.Errorf("pane click = %#v", paneHit.Click)
	}
	if closeHit == nil {
		t.Fatal("no close interaction")
	}
	if closeHit.Click.Action != ToggleDetails {
		t.Errorf("close click = %#v, want ToggleDetails", closeHit.Click)
	}
	if closeHit.Z != zControl {
		t.Errorf("close Z = %d, want %d", closeHit.Z, zControl)
	}
	// Close rect is the top-right corner of the pane.
	wantClose := image.Rect(model.Layout.Details.Max.X-2, model.Layout.Details.Min.Y, model.Layout.Details.Max.X-1, model.Layout.Details.Min.Y+1)
	if !closeHit.Rect.Eq(wantClose) {
		t.Errorf("close rect = %v, want %v", closeHit.Rect, wantClose)
	}

	// Compositor.Hit parity for the close control.
	pt := image.Pt(wantClose.Min.X, wantClose.Min.Y)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "details:close" {
		t.Errorf("Hit(%v) = %q, want details:close", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(wantClose) {
		t.Errorf("close hit bounds = %v, want %v", got, wantClose)
	}
	// The × glyph renders.
	if got := canvas.CellAt(wantClose.Min.X, wantClose.Min.Y).Content; got != "×" {
		t.Errorf("close glyph = %q, want ×", got)
	}
}

func TestDetailsNoActiveState(t *testing.T) {
	styles := newRenderStyles(false)
	model := detailsModel(140, 30, domain.Chat{}, ChatRow{})
	surface := buildDetailsLayer(model, styles)
	canvas, _ := detailsSurfaceCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "No conversation") {
		t.Errorf("no-active state missing text, got %q", text)
	}
}

func TestDetailsCenteredAvatarTitleUsernameViewImage(t *testing.T) {
	styles := newRenderStyles(false)
	active := domain.Chat{ID: 5, Title: "Mina Chen", Username: "mina"}
	model := detailsModel(140, 30, active, detailsRowFor(active))
	surface := buildDetailsLayer(model, styles)
	canvas, compositor := detailsSurfaceCanvas(model, surface)

	inner := model.Layout.Details.Inset(1)
	avatarWidth, avatarHeight := 12, 6
	avatarX := inner.Min.X + (inner.Dx()-avatarWidth)/2
	avatarRect := image.Rect(avatarX, inner.Min.Y, avatarX+avatarWidth, inner.Min.Y+avatarHeight)

	// Avatar placeholder cells render.
	if got := canvas.CellAt(avatarRect.Min.X+1, avatarRect.Min.Y+1).Content; got != "░" {
		t.Errorf("avatar cell = %q, want ░", got)
	}

	// Title centered below avatar.
	titleY := avatarRect.Max.Y + 1
	titleText := "Mina Chen"
	titleWidth := displayWidth(titleText)
	titleX := inner.Min.X + centeredX(inner.Dx(), titleWidth)
	if got := canvas.CellAt(titleX, titleY).Content; got != "M" {
		t.Errorf("title cell = %q, want M", got)
	}

	// Username centered below title.
	userY := titleY + 1
	userText := "@mina"
	userWidth := displayWidth(userText)
	userX := inner.Min.X + centeredX(inner.Dx(), userWidth)
	if got := canvas.CellAt(userX, userY).Content; got != "@" {
		t.Errorf("username cell = %q, want @", got)
	}

	// View image action.
	var viewHit *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "details:view-image" {
			viewHit = &surface.Interactions[i]
		}
	}
	if viewHit == nil {
		t.Fatal("no view-image interaction")
	}
	if viewHit.Click.Action != OpenDetailsAvatar {
		t.Errorf("view-image click = %#v, want OpenDetailsAvatar", viewHit.Click)
	}
	if viewHit.Z != zControl {
		t.Errorf("view-image Z = %d, want %d", viewHit.Z, zControl)
	}
	// Hit parity at the action's center.
	pt := image.Pt(viewHit.Rect.Min.X+viewHit.Rect.Dx()/2, viewHit.Rect.Min.Y)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "details:view-image" {
		t.Errorf("Hit(%v) = %q, want details:view-image", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(viewHit.Rect) {
		t.Errorf("view-image hit bounds = %v, want %v", got, viewHit.Rect)
	}
}

func TestDetailsActionSelectionUsesCursorPrefix(t *testing.T) {
	styles := newRenderStyles(false)
	active := domain.Chat{ID: 5, Kind: domain.ChatSupergroup, Title: "Group"}

	// Focused selection marks exactly one row with "> ", like the action modal.
	model := detailsModel(140, 30, active, detailsRowFor(active))
	model.DetailsSelected = 1
	surface := buildDetailsLayer(model, styles)
	canvas, _ := detailsSurfaceCanvas(model, surface)
	text := plainText(canvas.Render())
	if !strings.Contains(text, "> Members") {
		t.Fatalf("selected row missing cursor prefix: %q", text)
	}
	if strings.Contains(text, "> View image") {
		t.Fatalf("unselected row has cursor prefix: %q", text)
	}

	// Unfocused pane shows no cursor prefix.
	unfocused := detailsModel(140, 30, active, detailsRowFor(active))
	unfocused.Focus = FocusConversation
	unfocused.DetailsSelected = 1
	uSurface := buildDetailsLayer(unfocused, styles)
	uCanvas, _ := detailsSurfaceCanvas(unfocused, uSurface)
	if got := plainText(uCanvas.Render()); strings.Contains(got, ">") {
		t.Fatalf("unfocused pane shows cursor prefix: %q", got)
	}
}

func TestDetailsAvatarRetryOnlyExact12x6AndWins(t *testing.T) {
	styles := newRenderStyles(false)
	active := domain.Chat{ID: 5, Title: "Mina Chen"}
	row := detailsRowFor(active)
	row.AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "failed"}
	model := detailsModel(140, 30, active, row)
	surface := buildDetailsLayer(model, styles)

	var retry *layerInteraction
	for i := range surface.Interactions {
		if surface.Interactions[i].ID == "details:avatar-retry" {
			retry = &surface.Interactions[i]
		}
	}
	if retry == nil {
		t.Fatal("no details avatar retry interaction")
	}
	inner := model.Layout.Details.Inset(1)
	avatarRect := image.Rect(inner.Min.X+(inner.Dx()-12)/2, inner.Min.Y, inner.Min.X+(inner.Dx()-12)/2+12, inner.Min.Y+6)
	if !retry.Rect.Eq(avatarRect) {
		t.Errorf("retry rect = %v, want %v", retry.Rect, avatarRect)
	}
	if retry.Click.Action != Retry || retry.Click.AvatarKey != "key" {
		t.Errorf("retry click = %#v, want Retry/key", retry.Click)
	}
	if retry.Z != zControl {
		t.Errorf("retry Z = %d, want %d", retry.Z, zControl)
	}

	canvas, compositor := detailsSurfaceCanvas(model, surface)
	pt := image.Pt(avatarRect.Min.X+2, avatarRect.Min.Y+1)
	hit := compositor.Hit(pt.X, pt.Y)
	if hit.ID() != "details:avatar-retry" {
		t.Errorf("Hit(%v) = %q, want details:avatar-retry", pt, hit.ID())
	}
	if got := hit.Bounds(); !got.Eq(avatarRect) {
		t.Errorf("retry hit bounds = %v, want %v", got, avatarRect)
	}
	// Retry is intrinsic: cells after the text retain avatar content.
	if got := canvas.CellAt(avatarRect.Max.X-1, avatarRect.Max.Y-1).Content; got != "░" {
		t.Errorf("avatar cell after Retry = %q, want preserved ░", got)
	}

	// Retry text present.
	if !strings.Contains(plainText(canvas.Render()), "Retry") {
		t.Errorf("details missing Retry text")
	}
}

func TestDetailsAvatarRetryNotForSmallAvatar(t *testing.T) {
	styles := newRenderStyles(false)
	// A details pane with inner width < 12 yields an avatar smaller than
	// 12x6; no retry is published even with an avatar error.
	active := domain.Chat{ID: 5, Title: "Mina Chen"}
	row := detailsRowFor(active)
	row.AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "failed"}
	model := detailsModel(60, 18, active, row)
	// Force a narrow details pane so the avatar is not exactly 12x6.
	model.Layout.Details = image.Rect(0, 1, 12, 18)
	surface := buildDetailsLayer(model, styles)
	for _, interaction := range surface.Interactions {
		if interaction.ID == "details:avatar-retry" {
			t.Fatalf("narrow details should not publish avatar retry")
		}
	}
}

func TestDetailsNarrowNeverPublishesOutOfBoundsInteraction(t *testing.T) {
	styles := newRenderStyles(false)
	// Very narrow details pane: inner width may be small, avatar clipped.
	active := domain.Chat{ID: 5, Title: "Mina Chen"}
	model := detailsModel(60, 18, active, detailsRowFor(active))
	surface := buildDetailsLayer(model, styles)
	inner := model.Layout.Details.Inset(1)
	for _, interaction := range surface.Interactions {
		if interaction.ID == "details:close" || interaction.ID == "pane:details" {
			continue
		}
		if !interaction.Rect.In(inner) {
			t.Errorf("interaction %q rect %v outside inner %v", interaction.ID, interaction.Rect, inner)
		}
	}
}

func TestDetailsFocusedUnfocusedBorder(t *testing.T) {
	styles := newRenderStyles(false)
	active := domain.Chat{ID: 5, Title: "Mina Chen"}

	focused := detailsModel(140, 30, active, detailsRowFor(active))
	focused.Focus = FocusDetails
	fs := buildDetailsLayer(focused, styles)
	fCanvas, _ := detailsSurfaceCanvas(focused, fs)
	if got := colorOf(fCanvas.CellAt(focused.Layout.Details.Min.X, focused.Layout.Details.Min.Y).Style.Fg); got != rgba(focusedBorderColor) {
		t.Errorf("focused border = %v, want focusedBorderColor", got)
	}

	unfocused := detailsModel(140, 30, active, detailsRowFor(active))
	unfocused.Focus = FocusConversation
	us := buildDetailsLayer(unfocused, styles)
	uCanvas, _ := detailsSurfaceCanvas(unfocused, us)
	if got := colorOf(uCanvas.CellAt(unfocused.Layout.Details.Min.X, unfocused.Layout.Details.Min.Y).Style.Fg); got != rgba(borderColor) {
		t.Errorf("unfocused border = %v, want borderColor", got)
	}

	// The border is the rounded pane border from buildPane, not a manual
	// border glyph: corners render the rounded glyphs.
	if got := uCanvas.CellAt(unfocused.Layout.Details.Min.X, unfocused.Layout.Details.Min.Y).Content; got != "╭" {
		t.Errorf("details corner = %q, want rounded ╭", got)
	}
}
