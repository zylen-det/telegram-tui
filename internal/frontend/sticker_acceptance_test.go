package frontend

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

// embeddedKittyImageID extracts the i= image id from the first kitty
// graphics APC control of a cached transmit string.
func embeddedKittyImageID(transmit string) (uint32, bool) {
	control, ok := firstKittyControl(transmit)
	if !ok {
		return 0, false
	}
	for _, token := range strings.Split(control, ",") {
		key, value, cut := strings.Cut(token, "=")
		if !cut || key != "i" {
			continue
		}
		id, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(id), true
	}
	return 0, false
}

func TestStickerComposerShortcutControlAndGridKeyMapping(t *testing.T) {
	if action, ok := mapKeyPress(FocusComposer, tea.KeyPressMsg(tea.Key{Code: 's', Text: "s", Mod: tea.ModCtrl})); !ok || action.Action != OpenStickerPicker {
		t.Fatalf("Ctrl+S = %#v, %t", action, ok)
	}
	if _, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(tea.Key{Code: 's', Text: "s", Mod: tea.ModCtrl})); ok {
		t.Fatal("Ctrl+S escaped composer focus")
	}
	for _, test := range []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: tea.KeyLeft}, StickerMoveLeft},
		{tea.Key{Text: "l"}, StickerMoveRight},
		{tea.Key{Text: "k"}, StickerMoveUp},
		{tea.Key{Code: tea.KeyDown}, StickerMoveDown},
		{tea.Key{Code: tea.KeyEnter}, StickerActivate},
		{tea.Key{Code: tea.KeyEscape}, Close},
	} {
		got, ok := mapKeyPress(FocusStickerPicker, tea.KeyPressMsg(test.key))
		if !ok || got.Action != test.want {
			t.Errorf("key %#v = %#v, %t", test.key, got, ok)
		}
	}

	model := ViewModel{Width: 80, Height: 24, ActiveChat: domain.Chat{ID: 9, CanSend: true}}
	rect := imageRect(0, 0, 80, 3)
	surface := buildComposerLayer(model, rect, newRenderStyles(false))
	var stickerHit, photoHit bool
	for _, hit := range surface.Interactions {
		if hit.ID == "composer:sticker" {
			stickerHit = hit.Click.Action == OpenStickerPicker && hit.Rect.Dy() == 1
		}
		if hit.ID == "composer:photo" {
			photoHit = hit.Click.Action == OpenPhotoSend
		}
	}
	if !stickerHit || !photoHit {
		t.Fatalf("composer interactions = %#v", surface.Interactions)
	}
}

func imageRect(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }

func TestStickerGridHalfBlockFallbackHitsWheelAndKittyPlacement(t *testing.T) {
	picker := &StickerPickerState{RequestID: 7, ChatID: 9, Catalog: []domain.StickerRef{
		{File: domain.MediaFileRef{ID: 101}, Emoji: "🙂"}, {File: domain.MediaFileRef{ID: 102}},
	}, Selected: 1, Columns: 4, VisibleRows: 2}
	model := ViewModel{Width: 80, Height: 24, StickerPicker: picker, StickerThumbnails: []RenderedStickerThumbnail{
		{StickerFileID: 101, Block: thumbnail.Block{Text: "HALF_BLOCK", Width: 10, Height: 2}},
	}}
	surface := buildStickerPickerLayer(model, newRenderStyles(false))
	plain := ansi.Strip(lipgloss.NewCompositor(surface.Layer).Render())
	if !strings.Contains(plain, "HALF_BLOCK") || !strings.Contains(plain, "[Sticker]") {
		t.Fatalf("grid content missing:\n%s", plain)
	}
	var tileHit layerInteraction
	for _, hit := range surface.Interactions {
		if hit.ID == "sticker:102" {
			tileHit = hit
		}
	}
	if tileHit.Click.Action != StickerActivate || tileHit.Click.RequestID != 7 || tileHit.Click.StickerFileID != 102 || tileHit.WheelUp.Action != StickerMoveUp || tileHit.WheelDown.Action != StickerMoveDown {
		t.Fatalf("tile hit = %#v", tileHit)
	}

	model.StickerThumbnails = []RenderedStickerThumbnail{{StickerFileID: 101, Block: thumbnail.Block{Text: "kitty", Width: 10, Height: 4, Kitty: true, ImageID: 501}}}
	kitty := buildStickerPickerLayer(model, newRenderStyles(false))
	if len(kitty.Inline) != 1 || kitty.Inline[0].ImageID != 501 || kitty.Inline[0].X <= kitty.Rect.Min.X || kitty.Inline[0].Y <= kitty.Rect.Min.Y {
		t.Fatalf("Kitty placements = %#v", kitty.Inline)
	}
}

func TestStickerMessagesSharingCachedKittyBlockGetPlacementSpecificTransmits(t *testing.T) {
	styles := newRenderStyles(false)
	cachedTransmit := "\x1b_Ga=T,f=100,i=501,c=20,r=5,q=2,s=64,v=32;AAAA\x1b\\"
	cached := thumbnail.Block{Text: cachedTransmit, Width: 20, Height: 5, Kitty: true, ImageID: 501}
	photoTransmit := "\x1b_Ga=T,f=200,i=77,c=20,r=8,q=2;BBBB\x1b\\"
	photo := thumbnail.Block{Text: photoTransmit, Width: 20, Height: 8, Kitty: true, ImageID: 77}

	stickerA := testMessage(-1, 9, "[Sticker]")
	stickerA.Kind = domain.MessageSticker
	stickerA.Outgoing = true
	stickerB := testMessage(-2, 9, "[Sticker]")
	stickerB.Kind = domain.MessageSticker
	stickerB.Outgoing = true
	photoMsg := testMessage(77, 9, "[Photo]")
	photoMsg.Kind = domain.MessagePhoto
	photoMsg.Outgoing = true
	group := styledMessageGroup("Mina", false, stickerA, photoMsg, stickerB)

	// Both outgoing stickers hold the SAME picker-cached block, mirroring
	// the optimistic send path that copies the catalog block into every
	// MessageSticker. The photo keeps its own cached block.
	thumbs := map[domain.MessageID]thumbnail.Block{-1: cached, -2: cached, 77: photo}

	build := func() messageGroupResult {
		return buildMessageGroupLayer(group, 40, time.Local, messageSelection{}, thumbs, styles)
	}
	first := build()
	if len(first.Inline) != 3 {
		t.Fatalf("Inline placements = %d, want 3: %#v", len(first.Inline), first.Inline)
	}

	wantA := stickerMessageInlineID(9, -1)
	wantB := stickerMessageInlineID(9, -2)
	if wantA == 0 || wantB == 0 || wantA == wantB || wantA == 501 || wantB == 501 {
		t.Fatalf("placement ids are not placement-specific: A=%d B=%d cached=501", wantA, wantB)
	}

	byY := make(map[int]inlinePlacement, len(first.Inline))
	for _, placement := range first.Inline {
		byY[placement.Y] = placement
	}
	placeA, okA := byY[0]
	placeB, okB := byY[15]
	photoPlace, okP := byY[6]
	if !okA || !okB || !okP {
		t.Fatalf("placement rows = %#v", byY)
	}
	// Outgoing thumbnails right-align against the width 40 group.
	for name, p := range map[string]inlinePlacement{"stickerA": placeA, "stickerB": placeB, "photo": photoPlace} {
		if p.X != 19 || p.Width != 20 {
			t.Fatalf("%s geometry X=%d Width=%d, want X=19 Width=20", name, p.X, p.Width)
		}
	}
	if placeA.Height != 5 || photoPlace.Height != 8 {
		t.Fatalf("heights = %d/%d, want 5/8", placeA.Height, photoPlace.Height)
	}
	if placeA.ImageID != wantA || placeB.ImageID != wantB {
		t.Fatalf("sticker placement ids = %d/%d, want %d/%d", placeA.ImageID, placeB.ImageID, wantA, wantB)
	}
	if embedded, ok := embeddedKittyImageID(placeA.Text); !ok || embedded != wantA {
		t.Fatalf("stickerA transmit i=%d/%t, want %d", embedded, ok, wantA)
	}
	if embedded, ok := embeddedKittyImageID(placeB.Text); !ok || embedded != wantB {
		t.Fatalf("stickerB transmit i=%d/%t, want %d", embedded, ok, wantB)
	}
	if !strings.Contains(placeA.Text, "AAAA") || !strings.Contains(placeB.Text, "AAAA") {
		t.Fatal("sticker transmits lost the cached payload")
	}
	if placeA.Text == cachedTransmit || placeB.Text == cachedTransmit {
		t.Fatal("sticker transmits were not rewritten from the cached block")
	}
	// The Photo placement keeps its cached block identity and transmit.
	if photoPlace.ImageID != 77 || photoPlace.Text != photoTransmit {
		t.Fatalf("photo placement = %#v, want cached block i=77 unchanged", photoPlace)
	}
	if embedded, ok := embeddedKittyImageID(photoPlace.Text); !ok || embedded != 77 {
		t.Fatalf("photo transmit i=%d/%t, want 77", embedded, ok)
	}
	// The shared cached block is never mutated.
	if cached.Text != cachedTransmit || cached.ImageID != 501 {
		t.Fatal("shared cached block mutated")
	}

	// Deterministic: rebuilding the same composition reproduces ids and
	// rewritten transmits.
	second := build()
	if len(second.Inline) != 3 {
		t.Fatalf("second build placements = %d, want 3", len(second.Inline))
	}
	for _, p := range second.Inline {
		switch p.Y {
		case 0:
			if p.ImageID != wantA || p.Text != placeA.Text {
				t.Fatalf("second stickerA = %#v", p)
			}
		case 6:
			if p.ImageID != 77 || p.Text != photoTransmit {
				t.Fatalf("second photo = %#v", p)
			}
		case 15:
			if p.ImageID != wantB || p.Text != placeB.Text {
				t.Fatalf("second stickerB = %#v", p)
			}
		}
	}
}

func TestStickerPickerOwnsInlinePlaneAndSuppressesUnderlyingHits(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 30
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusStickerPicker
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{ID: 77, ChatID: 9, Kind: domain.MessageSticker, Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 700}}}}
	state.Thumbnails[9] = map[domain.MessageID]thumbnail.Block{77: {Text: "\x1b_Ga=T,f=900,i=700,c=10,r=4,q=2,s=64,v=32;conversation-kitty\x1b\\", Width: 10, Height: 4, Kitty: true, ImageID: 700}}
	state.StickerPicker = &StickerPickerState{RequestID: 8, ChatID: 9, Catalog: []domain.StickerRef{{File: domain.MediaFileRef{ID: 101}}}, Columns: 5, VisibleRows: 2}
	state.StickerThumbnails[101] = thumbnail.Block{Text: "picker-kitty", Width: 10, Height: 4, Kitty: true, ImageID: 801}
	model := Select(state, nil)
	frame := composeApplication(model, nil)
	if len(frame.Inline) != 1 || frame.Inline[0].ImageID != 801 {
		t.Fatalf("open picker inline = %#v", frame.Inline)
	}
	if _, ok := frame.Hits.ActionAt(0, 0); ok {
		t.Fatal("modal leaked an underlying hit")
	}

	state.StickerPicker = nil
	state.Focus = FocusConversation
	closed := composeApplication(Select(state, nil), nil)
	if len(closed.Inline) != 1 || closed.Inline[0].ImageID != stickerMessageInlineID(9, 77) {
		t.Fatalf("closed picker inline = %#v", closed.Inline)
	}
	if embedded, ok := embeddedKittyImageID(closed.Inline[0].Text); !ok || embedded != closed.Inline[0].ImageID {
		t.Fatalf("closed picker transmit i=%d/%t, want %d", embedded, ok, closed.Inline[0].ImageID)
	}
}
