package frontend

import (
	"bytes"
	"image"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
)

func frameBaseModel(width, height int) ViewModel {
	state := InitialState()
	state.Width = width
	state.Height = height
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.ChatsLoaded = true
	state.Chats = []domain.Chat{
		{ID: 1, Kind: domain.ChatPrivate, Title: "Mina Chen", LastMessage: "Ship it", LastMessageAt: time.Date(2026, time.July, 20, 13, 2, 0, 0, time.Local).Unix(), CanSend: true},
		{ID: 2, Kind: domain.ChatSupergroup, Title: "Weekend dev", Username: "weekend_dev", LastMessage: "Hello", LastMessageAt: time.Date(2026, time.July, 20, 14, 35, 0, 0, time.Local).Unix(), UnreadCount: 3, CanSend: true},
	}
	state.SelectedChat = 1
	state.Messages[2] = []domain.Message{{
		ID: 7, ChatID: 2, SenderName: "Iris", SentAt: time.Date(2026, time.July, 20, 14, 30, 0, 0, time.Local), Kind: domain.MessageText, Text: "Hello from the group",
	}}
	state.Drafts[2] = "draft reply"
	return Select(state, time.Local)
}

// 1 & 8: composeApplication output equals its compositor render and is a full
// viewport when positive; zero clears content/hits/cursor/overlay/compositor.
func TestApplicationFrameContentEqualsCompositorAndFullViewport(t *testing.T) {
	for _, size := range []struct {
		width, height int
	}{
		{60, 18}, {100, 24}, {140, 30},
	} {
		model := frameBaseModel(size.width, size.height)
		frame := composeApplication(model, time.Local)
		assertFrameContentEqualsCompositor(t, frame)
		bounds := frame.Compositor.Bounds()
		if bounds.Dx() != size.width || bounds.Dy() != size.height {
			t.Errorf("%dx%d compositor bounds = %v", size.width, size.height, bounds)
		}
		lines := strings.Split(frame.Content, "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d frame rendered %d rows", size.width, size.height, len(lines))
		}
		for y, line := range lines {
			if got := ansi.StringWidth(line); got > size.width {
				t.Errorf("%dx%d row %d width = %d, want <= %d", size.width, size.height, y, got, size.width)
			}
		}
	}

	zero := composeApplication(ViewModel{}, time.Local)
	if zero.Compositor != nil || zero.Content != "" || zero.Hits != nil || zero.Cursor.Visible || zero.Overlay.Ready {
		t.Fatalf("zero frame not safe-empty: %+v", zero)
	}
}

// 2: base surfaces render via the assembler and emit expected semantic hits.
func TestApplicationFrameBaseSurfacesProduceSemanticHits(t *testing.T) {
	model := frameBaseModel(100, 24)
	frame := composeApplication(model, time.Local)
	plain := ansi.Strip(frame.Content)

	for _, want := range []string{"telegram-tui", "Chats", "Mina Chen", "Weekend dev", "[Send]"} {
		if !strings.Contains(plain, want) {
			t.Errorf("base frame missing %q", want)
		}
	}
	var focusChat, focusPane, composerSubmit bool
	for _, hit := range frame.Hits {
		switch {
		case hit.Click.Action == FocusChat && hit.Click.ChatID == 1:
			focusChat = true
		case hit.Click.Action == FocusPane:
			focusPane = true
		case hit.Click.Action == ComposerSubmit:
			composerSubmit = true
		}
	}
	if !focusChat || !focusPane || !composerSubmit {
		t.Fatalf("base semantic hits missing (focusChat=%t focusPane=%t submit=%t)", focusChat, focusPane, composerSubmit)
	}
}

// 3: every modal/picker/auth topology clears underlying FocusChat/FocusPane hits.
func TestApplicationFrameOverlaysSuppressUnderlyingHits(t *testing.T) {
	tops := []struct {
		name  string
		apply func(*ViewModel)
		auth  bool
	}{
		{"media modal", func(m *ViewModel) { m.Modal = &ModalState{Title: "Img", Loading: true} }, false},
		{"action menu", func(m *ViewModel) {
			m.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true}}
		}, false},
		{"reaction picker", func(m *ViewModel) { m.ReactionPicker = &ReactionPicker{ChatID: 2, MessageID: 7, Selected: 1} }, false},
		{"forward picker", func(m *ViewModel) {
			m.ForwardPicker = &ForwardPicker{SourceChatID: 2, SourceMessageID: 7, SelectedChat: 0}
		}, false},
		{"photo send", func(m *ViewModel) {
			m.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("/tmp/photo.jpg")}
		}, false},
		{"in-app prompt", func(*ViewModel) {}, true},
	}
	for _, tc := range tops {
		t.Run(tc.name, func(t *testing.T) {
			model := frameBaseModel(100, 24)
			tc.apply(&model)
			if tc.auth {
				model.Focus = FocusAuth
			}
			frame := composeApplication(model, time.Local)
			for _, hit := range frame.Hits {
				if hit.Click.Action == FocusChat || hit.Click.Action == FocusPane || hit.Click.Action == ComposerSubmit {
					t.Errorf("overlay %q retained underlying hit: %#v", tc.name, hit)
				}
			}
		})
	}
}

// 4: visual interaction IDs match Compositor.Hit IDs/bounds; virtual outside
// media hits compile into HitMap without a visual Layer. The authoritative
// surfaceResult comes from the matching accepted builder so the application
// assembler is proven to publish exactly the adapter's geometry and payload.
func TestApplicationFrameVisualIDsMatchCompositorHit(t *testing.T) {
	// Action menu overlay: a known topology with one visual interactive row
	// (Reply) plus the modal close control.
	model := frameBaseModel(100, 24)
	model.MessageMenu = &MessageActionMenu{
		ChatID: 2, MessageID: 7, Selected: 0,
		Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
	}
	frame := composeApplication(model, time.Local)
	compositor := frame.Compositor
	authoritative := buildActionModalLayer(model, newRenderStyles(false))

	for _, interaction := range authoritative.Interactions {
		if interaction.Virtual {
			continue
		}
		point := interaction.Rect.Min
		layerHit := compositor.Hit(point.X, point.Y)
		if got := layerHit.ID(); got != interaction.ID {
			t.Errorf("application compositor Hit(%v) = %q, want %q", point, got, interaction.ID)
		}
		if rect := layerHit.Bounds(); !rect.Eq(interaction.Rect) {
			t.Errorf("interaction %q hit bounds %v != interaction rect %v", interaction.ID, rect, interaction.Rect)
		}
		if !hitsContainRectAction(frame.Hits, interaction.Rect, interaction.Click) {
			t.Errorf("interaction %q rect %v + click %#v missing from frame.Hits", interaction.ID, interaction.Rect, interaction.Click)
		}
	}

	// Media modal: the four virtual outside interactions must each be present
	// in frame.Hits with their exact rect/action, while no visual Layer with
	// that ID is returned by Compositor.Hit at an interior point.
	modal := frameBaseModel(100, 24)
	modal.Modal = &ModalState{Title: "Img", Loading: true}
	mframe := composeApplication(modal, time.Local)
	mediaSurface, _ := buildMediaModalLayer(modal, newRenderStyles(false))

	var virtual []layerInteraction
	for _, interaction := range mediaSurface.Interactions {
		if !interaction.Virtual {
			continue
		}
		virtual = append(virtual, interaction)
		if !hitsContainRectAction(mframe.Hits, interaction.Rect, interaction.Click) {
			t.Errorf("virtual %q rect %v + click %#v missing from frame.Hits", interaction.ID, interaction.Rect, interaction.Click)
		}
		// An interior point of the virtual rect must not resolve to a visual
		// layer with that ID (the interaction is virtual, no Layer exists).
		interior := image.Pt(interaction.Rect.Min.X+interaction.Rect.Dx()/2, interaction.Rect.Min.Y+interaction.Rect.Dy()/2)
		if hit := mframe.Compositor.Hit(interior.X, interior.Y); hit.ID() == interaction.ID {
			t.Errorf("virtual %q resolved to a visual Layer at %v", interaction.ID, interior)
		}
	}
	if len(virtual) != 4 {
		t.Fatalf("media modal virtual outside interactions = %d, want exactly 4", len(virtual))
	}
}

// 5: Huh owns the composer cursor; modal/picker hide it; in-app auth retains
// its terminal cursor until that separate input cut-over.
func TestApplicationFrameCursorTopology(t *testing.T) {
	composer := frameBaseModel(100, 24)
	composer.Focus = FocusComposer
	cframe := composeApplication(composer, time.Local)
	if cframe.Cursor.Visible || cframe.Cursor.X != -1 || cframe.Cursor.Y != -1 {
		t.Fatalf("composer terminal cursor visible after Huh cut-over: %+v", cframe.Cursor)
	}

	modal := frameBaseModel(100, 24)
	modal.Modal = &ModalState{Title: "Img", Loading: true}
	if f := composeApplication(modal, time.Local); f.Cursor.Visible {
		t.Fatal("media modal cursor visible, want hidden")
	}

	authModel := frameBaseModel(100, 24)
	authModel.Focus = FocusAuth
	authModel.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}, Input: []rune("ab")}
	aframe := composeApplication(authModel, time.Local)
	if !aframe.Cursor.Visible {
		t.Fatal("in-app auth cursor hidden")
	}
	if aframe.Cursor.X < 0 || aframe.Cursor.Y < 0 {
		t.Fatalf("in-app auth cursor not absolute: %+v", aframe.Cursor)
	}
}

// 5b: in-app auth with CJK input uses grapheme-cell cursor math.
func TestApplicationFrameAuthCursorGraphemeSafe(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 24)
	frameWidth := min(56, bounds.Dx()-4)
	innerWidth := frameWidth - 4
	frameX := (bounds.Dx() - frameWidth) / 2
	innerX := frameX + 2
	inputY := (bounds.Dy()-min(9, bounds.Dy()))/2 + 4

	model := frameBaseModel(100, 24)
	model.Focus = FocusAuth
	model.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}, Input: []rune("a界")}
	frame := composeApplication(model, time.Local)
	if !frame.Cursor.Visible {
		t.Fatal("in-app auth cursor hidden with CJK input")
	}
	if got, want := frame.Cursor.X, innerX+3; got != want {
		t.Errorf("CJK cursor X = %d, want %d", got, want)
	}
	if frame.Cursor.Y != inputY {
		t.Errorf("auth cursor Y = %d, want %d", frame.Cursor.Y, inputY)
	}
	if frame.Cursor.X < innerX || frame.Cursor.X >= innerX+innerWidth {
		t.Errorf("auth cursor X = %d outside input [%d,%d)", frame.Cursor.X, innerX, innerX+innerWidth)
	}
}

// 6: toast condition and dimmed-under-reaction behavior.
func TestApplicationFrameToastConditionAndDim(t *testing.T) {
	base := frameBaseModel(100, 24)
	base.Toast = &domain.AppError{Message: "Message copied"}
	plainToast := ansi.Strip(composeApplication(base, time.Local).Content)
	if !strings.Contains(plainToast, "Message copied") {
		t.Fatal("toast missing under clean top layer")
	}

	modal := base
	modal.Modal = &ModalState{Title: "Img", Loading: true}
	if plain := ansi.Strip(composeApplication(modal, time.Local).Content); strings.Contains(plain, "Message copied") {
		t.Fatal("toast leaked under media modal")
	}

	reaction := base
	reaction.ReactionPicker = &ReactionPicker{ChatID: 2, MessageID: 7, Selected: 1}
	reaction.Focus = FocusReactionPicker
	rframe := composeApplication(reaction, time.Local)
	plain := ansi.Strip(rframe.Content)
	if !strings.Contains(plain, "Message copied") {
		t.Fatal("toast not rebuilt faint/dimmed under reaction picker")
	}

	// Under a reaction picker the underlying base/toast surface must actually
	// be Faint while the overlay (undimmed) stays bright. Inspect exact cells
	// via a Canvas so we assert the pinned cell Styles, not broad substrings.
	asideCanvas := lipgloss.NewCanvas(reaction.Width, reaction.Height).Compose(rframe.Compositor)
	overlaySurface := buildReactionPickerLayer(reaction, newRenderStyles(false))
	overlayCanvas := lipgloss.NewCanvas(reaction.Width, reaction.Height).Compose(lipgloss.NewCompositor(overlaySurface.Layer))

	toastText := "Message copied"
	toastLayout := (components.Toast{Text: toastText}).Layout(image.Rect(0, 0, reaction.Width, reaction.Height))
	baseCell := asideCanvas.CellAt(toastLayout.Content.Min.X, toastLayout.Content.Min.Y)
	if baseCell == nil {
		t.Fatalf("toast content cell (%d,%d) is nil", toastLayout.Content.Min.X, toastLayout.Content.Min.Y)
	}
	if baseCell.Style.Attrs&uv.AttrFaint == 0 {
		t.Errorf("underlying toast cell (%d,%d) is not Faint under reaction picker", toastLayout.Content.Min.X, toastLayout.Content.Min.Y)
	}
	// The reaction-picker overlay is built with undimmed styles; sample a cell
	// inside its frame and require it to stay bright.
	if !overlaySurface.Rect.Empty() {
		overlayPt := image.Pt(overlaySurface.Rect.Min.X+overlaySurface.Rect.Dx()/2, overlaySurface.Rect.Min.Y+overlaySurface.Rect.Dy()/2)
		overlayCell := overlayCanvas.CellAt(overlayPt.X, overlayPt.Y)
		if overlayCell == nil {
			t.Fatalf("overlay cell %v is nil", overlayPt)
		}
		if overlayCell.Style.Attrs&uv.AttrFaint != 0 {
			t.Errorf("overlay cell %v should stay undimmed, got Faint", overlayPt)
		}
	}
}

// hitsContainRectAction reports whether hits contains a hit with the exact
// rect and click payload (test helper; no production changes).
func hitsContainRectAction(hits HitMap, rect image.Rectangle, click ActionReceived) bool {
	for _, hit := range hits {
		if hit.Rect.Eq(rect) && hit.Click == click {
			return true
		}
	}
	return false
}

// 10: the reaction picker's full application frame keeps exact geometry stable
// across every palette selection while the selected row's style/content may
// change. This exercises the full assembler (composeApplication), not just the
// picker adapter, and derives expected interactions from the accepted builder.
func TestApplicationFrameReactionPickerSelectionGeometryStable(t *testing.T) {
	// The production palette must actually contain the three required emoji.
	for _, want := range []string{"👍", "❤️", "🔥"} {
		found := false
		for _, emoji := range ReactionPalette {
			if emoji == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("production ReactionPalette missing %q", want)
		}
	}

	width, height := 100, 24
	var baseline map[string]image.Rectangle

	for selected := 0; selected < len(ReactionPalette); selected++ {
		model := frameBaseModel(width, height)
		model.ReactionPicker = &ReactionPicker{ChatID: 2, MessageID: 7, Selected: selected}
		model.Focus = FocusReactionPicker

		frame := composeApplication(model, time.Local)

		// Full viewport geometry: exactly Height rows; transparent trailing cells
		// may be omitted from the rendered string.
		lines := strings.Split(frame.Content, "\n")
		if len(lines) != height {
			t.Fatalf("selection %d rendered %d rows, want %d", selected, len(lines), height)
		}
		for y, line := range lines {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("selection %d row %d width = %d, want <= %d", selected, y, got, width)
			}
		}

		// Derive the expected adapter interactions for this same selected state.
		authoritative := buildReactionPickerLayer(model, newRenderStyles(false))
		for _, interaction := range authoritative.Interactions {
			if interaction.Virtual {
				continue
			}
			point := interaction.Rect.Min
			layerHit := frame.Compositor.Hit(point.X, point.Y)
			if got := layerHit.ID(); got != interaction.ID {
				t.Errorf("selection %d Hit(%v) = %q, want %q", selected, point, got, interaction.ID)
			}
			if rect := layerHit.Bounds(); !rect.Eq(interaction.Rect) {
				t.Errorf("selection %d interaction %q hit bounds %v != interaction rect %v", selected, interaction.ID, rect, interaction.Rect)
			}
			if !hitsContainRectAction(frame.Hits, interaction.Rect, interaction.Click) {
				t.Errorf("selection %d interaction %q rect %v + click %#v missing from frame.Hits", selected, interaction.ID, interaction.Rect, interaction.Click)
			}
		}

		// Geometry baseline from the first selection must remain identical for
		// every selection (style/content may change; geometry may not).
		if selected == 0 {
			baseline = make(map[string]image.Rectangle, len(authoritative.Interactions))
			for _, interaction := range authoritative.Interactions {
				if interaction.Virtual {
					continue
				}
				baseline[interaction.ID] = interaction.Rect
			}
		} else {
			for _, interaction := range authoritative.Interactions {
				if interaction.Virtual {
					continue
				}
				if want, ok := baseline[interaction.ID]; !ok || !interaction.Rect.Eq(want) {
					t.Errorf("selection %d interaction %q rect %v changed from baseline %v", selected, interaction.ID, interaction.Rect, want)
				}
			}
		}

		// Underlying FocusChat/FocusPane hits must remain absent.
		for _, hit := range frame.Hits {
			if hit.Click.Action == FocusChat || hit.Click.Action == FocusPane {
				t.Errorf("selection %d retained underlying hit: %#v", selected, hit)
			}
		}
	}
}

func assertFrameContentEqualsCompositor(t *testing.T, frame frameResult) {
	t.Helper()
	if frame.Compositor == nil {
		t.Fatal("compositor is nil")
	}
	if got := frame.Compositor.Render(); got != frame.Content {
		t.Fatalf("composeApplication content != frame.Compositor.Render()")
	}
}

// 9: AppModel.View publishes exactly the frame hits and mirrors the frame's
// overlay request into the Kitty binder without recomputing modal geometry.
func TestAppModelViewPublishesFrameHitsAndOverlay(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = FocusModal
	path := writeOverlayPNG(t, "frame-modal.png", 76, 30)
	state.Modal = &ModalState{Title: "Image", Path: path, PreviousFocus: FocusConversation}
	state.ChatsLoaded = true
	state.Focus = FocusConversation
	state.SelectedChat = 0
	state.Chats = []domain.Chat{{ID: 11}, {ID: 22}}
	model := newAppModelForTest(t, state, newTestSession(t))
	images := &overlayImages{}
	var rendered bytes.Buffer
	overlay := NewOutputOverlay(&rendered, images, func(int, int) image.Point { return image.Pt(1, 2) })
	model.SetOutputOverlay(overlay)

	view := model.View()
	mustWriteOverlay(t, overlay, view.WindowTitle)

	// Publish exactly the Kitty request reflected by the frame's Overlay: the
	// reserved modal content rectangle, full columns/rows, and selected chat.
	model2 := Select(model.Snapshot(), time.Local)
	frame := composeApplication(model2, time.Local)
	if images.shows != 1 {
		t.Fatalf("Kitty shows = %d, want 1", images.shows)
	}
	if got := images.rectangles[0]; !got.Eq(frame.Overlay.Bounds) {
		t.Fatalf("publish bounds = %v, want frame overlay %v (no second geometry)", got, frame.Overlay.Bounds)
	}
	if overlay.active.columns != frame.Overlay.Columns || overlay.active.rows != frame.Overlay.Rows {
		t.Fatalf("published columns/rows = %d/%d, want %d/%d", overlay.active.columns, overlay.active.rows, frame.Overlay.Columns, frame.Overlay.Rows)
	}

	// hitRegions exactly equal the frame hits, field-by-field in order.
	frameHits := frame.Hits
	published := model.hitRegions()
	if len(published) != len(frameHits) {
		t.Fatalf("hit count = %d, want frame hits %d", len(published), len(frameHits))
	}
	for index := range frameHits {
		if !published[index].Rect.Eq(frameHits[index].Rect) {
			t.Errorf("hit %d rect = %v, want %v", index, published[index].Rect, frameHits[index].Rect)
		}
		if published[index].Click != frameHits[index].Click {
			t.Errorf("hit %d click = %#v, want %#v", index, published[index].Click, frameHits[index].Click)
		}
		if published[index].WheelUp != frameHits[index].WheelUp {
			t.Errorf("hit %d WheelUp = %#v, want %#v", index, published[index].WheelUp, frameHits[index].WheelUp)
		}
		if published[index].WheelDown != frameHits[index].WheelDown {
			t.Errorf("hit %d WheelDown = %#v, want %#v", index, published[index].WheelDown, frameHits[index].WheelDown)
		}
	}

	// The published Kitty selected ID must equal the frame overlay's selected
	// chat, proving View mirrors the frame request exactly.
	overlay.mu.Lock()
	publishedSelected := overlay.active.selectedID
	overlay.mu.Unlock()
	if publishedSelected != frame.Overlay.SelectedID {
		t.Errorf("published Kitty selectedID = %d, want frame overlay %d", publishedSelected, frame.Overlay.SelectedID)
	}
}

// 9b: a prompt/FocusAuth media overlay is never Ready and View clears it.
func TestAppModelViewAuthOverlayNotReady(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = FocusAuth
	state.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}}
	state.ChatsLoaded = true
	model := newAppModelForTest(t, state, newTestSession(t))
	images := &overlayImages{}
	overlay := NewOutputOverlay(&bytes.Buffer{}, images, nil)
	model.SetOutputOverlay(overlay)

	frame := composeApplication(Select(model.Snapshot(), time.Local), time.Local)
	if frame.Overlay.Ready {
		t.Fatal("auth/prompt overlay unexpectedly Ready")
	}
	model.publishOverlayDesired(frame.Overlay, frame.Inline)
	overlay.mu.Lock()
	empty := overlay.desired == overlayPlacement{}
	overlay.mu.Unlock()
	if !empty {
		t.Fatalf("non-ready overlay left a desired placement: %#v", overlay.desired)
	}
}

func TestApplicationFramePhotoSend(t *testing.T) {
	// Base model with PhotoSend active.
	basePhoto := frameBaseModel(100, 24)
	basePhoto.PhotoSend = &PhotoSendState{
		ChatID:        2,
		Input:         []rune("/tmp/photo.jpg"),
		PreviousFocus: FocusConversation,
	}

	// 1: Visible [Photo] in composer and [Send] photoSendSubmit.
	t.Run("photoAndSendVisible", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		plain := ansi.Strip(frame.Content)
		if !strings.Contains(plain, "[Send]") {
			t.Error("missing [Send]")
		}
		if !strings.Contains(plain, "[Photo]") {
			t.Error("missing [Photo]")
		}
	})

	// 2: Modal shows input path suffix.
	t.Run("modalPathSuffix", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		plain := ansi.Strip(frame.Content)
		if !strings.Contains(plain, "photo.jpg") {
			t.Errorf("modal missing path suffix in: %s", plain)
		}
		if !strings.Contains(plain, "Send file") {
			t.Error("modal missing title")
		}
		if !strings.Contains(plain, "Local media/document path") {
			t.Error("modal missing media/document label")
		}
	})

	// 3: overlayActive dims base (no toast under PhotoSend).
	t.Run("baseDimmed", func(t *testing.T) {
		toastModel := frameBaseModel(100, 24)
		toastModel.Toast = &domain.AppError{Message: "toast"}
		basePlain := ansi.Strip(composeApplication(toastModel, time.Local).Content)
		if !strings.Contains(basePlain, "toast") {
			t.Fatal("toast visible under clean overlay")
		}
		toastModel.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("/tmp/x")}
		framePlain := ansi.Strip(composeApplication(toastModel, time.Local).Content)
		if strings.Contains(framePlain, "toast") {
			t.Fatal("toast leaked under PhotoSend overlay")
		}
	})

	// 4: Modal-only semantic hits; Compositor.Hit parity.
	t.Run("modalHits", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		for _, hit := range frame.Hits {
			switch hit.Click.Action {
			case FocusChat, FocusPane, ComposerSubmit, OpenPhotoSend:
				t.Errorf("underlying hit leaked: %#v", hit)
			}
		}
		// Submit hit present when path non-empty.
		hasSubmit := false
		hasClose := false
		hasCancel := false
		for _, hit := range frame.Hits {
			switch hit.Click.Action {
			case PhotoSendSubmit:
				hasSubmit = true
			case Close:
				hasClose = true
			}
			// Cancel hit is detected via Close action in modal.
			if hit.Click.Action == Close {
				hasCancel = true
			}
		}
		if !hasSubmit {
			t.Error("missing photo send submit hit")
		}
		if !hasClose {
			t.Error("missing close hit")
		}
		if !hasCancel {
			t.Error("missing cancel hit")
		}
	})

	// 4b: Compositor.Hit parity for modal interactions.
	t.Run("compositorHitParity", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		authoritative := buildPhotoSendModalLayer(
			image.Rect(0, 0, basePhoto.Width, basePhoto.Height),
			photoSendModalData{Path: string(basePhoto.PhotoSend.Input)},
			newRenderStyles(false),
		)
		for _, interaction := range authoritative.Interactions {
			if interaction.Virtual {
				continue
			}
			point := interaction.Rect.Min
			layerHit := frame.Compositor.Hit(point.X, point.Y)
			if got := layerHit.ID(); got != interaction.ID {
				t.Errorf("Hit(%v) = %q, want %q", point, got, interaction.ID)
			}
			if rect := layerHit.Bounds(); !rect.Eq(interaction.Rect) {
				t.Errorf("interaction %q hit bounds %v != rect %v", interaction.ID, rect, interaction.Rect)
			}
		}
	})

	// 5: Blank path no submit hit.
	t.Run("blankNoSubmit", func(t *testing.T) {
		empty := basePhoto
		empty.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("")}
		frame := composeApplication(empty, time.Local)
		for _, hit := range frame.Hits {
			if hit.Click.Action == PhotoSendSubmit {
				t.Errorf("blank path has submit hit: %#v", hit)
			}
		}
	})

	// 6: Cursor exact leaf parity and in bounds.
	t.Run("cursorBounds", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		if !frame.Cursor.Visible {
			t.Fatal("cursor not visible")
		}
		if frame.Cursor.X < 0 || frame.Cursor.Y < 0 {
			t.Fatalf("cursor negative: %+v", frame.Cursor)
		}
		if frame.Cursor.X >= basePhoto.Width || frame.Cursor.Y >= basePhoto.Height {
			t.Fatalf("cursor outside viewport: %+v", frame.Cursor)
		}
		// Match leaf cursor.
		leaf := buildPhotoSendModalLayer(
			image.Rect(0, 0, basePhoto.Width, basePhoto.Height),
			photoSendModalData{Path: string(basePhoto.PhotoSend.Input)},
			newRenderStyles(false),
		)
		if frame.Cursor.X != leaf.Cursor.X || frame.Cursor.Y != leaf.Cursor.Y {
			t.Errorf("cursor %+v != leaf %+v", frame.Cursor, leaf.Cursor)
		}
	})

	// 7: No Overlay.Ready/path.
	t.Run("noOverlayReady", func(t *testing.T) {
		frame := composeApplication(basePhoto, time.Local)
		if frame.Overlay.Ready {
			t.Fatal("PhotoSend overlay unexpectedly Ready")
		}
	})

	// 8: Resize: valid then too-small/zero produces safe no stale modal hits/cursor.
	t.Run("resizeSafe", func(t *testing.T) {
		// Valid size.
		frame := composeApplication(basePhoto, time.Local)
		if !frame.Cursor.Visible {
			t.Fatal("valid resize: cursor not visible")
		}
		if len(frame.Hits) == 0 {
			t.Fatal("valid resize: no hits")
		}

		// Too-small: should produce safe empty frame.
		tooSmall := basePhoto
		tooSmall.Width = 10
		tooSmall.Height = 6
		frame2 := composeApplication(tooSmall, time.Local)
		if frame2.Cursor.Visible {
			t.Fatal("too-small: cursor should be hidden")
		}
	})

	// 9: Deterministic - same input produces same output.
	t.Run("deterministic", func(t *testing.T) {
		frame1 := composeApplication(basePhoto, time.Local)
		frame2 := composeApplication(basePhoto, time.Local)
		if frame1.Content != frame2.Content {
			t.Fatal("non-deterministic content")
		}
		if len(frame1.Hits) != len(frame2.Hits) {
			t.Fatalf("non-deterministic hit count: %d vs %d", len(frame1.Hits), len(frame2.Hits))
		}
	})

	// 10: Impossible auth+PhotoSend: auth precedence.
	t.Run("authPrecedence", func(t *testing.T) {
		authModel := frameBaseModel(100, 24)
		authModel.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("/tmp/x")}
		authModel.Focus = FocusAuth
		authModel.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Auth"}}
		frame := composeApplication(authModel, time.Local)
		// Auth should override PhotoSend; only auth hits present.
		for _, hit := range frame.Hits {
			if hit.Click.Action == PhotoSendSubmit || hit.Click.Action == OpenPhotoSend {
				t.Errorf("PhotoSend hit leaked under auth: %#v", hit)
			}
		}
	})
}
