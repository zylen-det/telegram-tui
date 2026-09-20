package frontend

import (
	"image"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

// modalStackOverlayCase binds one registered overlay ID to the ViewModel
// mutation that activates exactly that overlay, plus the toast policy that
// overlay must keep. The slice order is the frame precedence order.
type modalStackOverlayCase struct {
	id              string
	suppressesToast bool
	activate        func(*ViewModel)
}

func modalStackOverlayCases() []modalStackOverlayCase {
	return []modalStackOverlayCase{
		{id: "media", suppressesToast: true, activate: func(m *ViewModel) {
			m.Modal = &ModalState{Title: "Photo", Path: "/tmp/photo.jpg"}
		}},
		{id: "message-menu", suppressesToast: true, activate: func(m *ViewModel) {
			m.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true}}
		}},
		// The reaction picker is the historical dim-but-visible toast case: it
		// dims the base and rebuilds the toast faint instead of hiding it.
		{id: "reaction-picker", suppressesToast: false, activate: func(m *ViewModel) {
			m.ReactionPicker = &ReactionPicker{ChatID: 2, MessageID: 7, Selected: 1}
		}},
		{id: "forward-picker", suppressesToast: true, activate: func(m *ViewModel) {
			m.ForwardPicker = &ForwardPicker{SourceChatID: 2, SourceMessageID: 7}
		}},
		{id: "sticker-picker", suppressesToast: true, activate: func(m *ViewModel) {
			m.StickerPicker = &StickerPickerState{RequestID: 8, ChatID: 2, Columns: 5, VisibleRows: 2}
		}},
		{id: "photo-send", suppressesToast: true, activate: func(m *ViewModel) {
			m.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("/tmp/photo.jpg")}
		}},
		{id: "message-search", suppressesToast: false, activate: func(m *ViewModel) {
			m.MessageSearch = &MessageSearchState{ChatID: 2, Input: []rune("search"), Query: "search"}
		}},
		{id: "chat-search", suppressesToast: true, activate: func(m *ViewModel) {
			m.ChatSearch = &ChatSearchState{Input: []rune("week"), Query: "week"}
		}},
		{id: "chat-actions", suppressesToast: true, activate: func(m *ViewModel) {
			m.ChatActions = &ChatActionMenuState{ChatID: 2}
		}},
		{id: "pinned-messages", suppressesToast: true, activate: func(m *ViewModel) {
			m.PinnedMessages = &PinnedMessagesState{ChatID: 2, Loading: true}
		}},
		{id: "topics", suppressesToast: false, activate: func(m *ViewModel) {
			m.Topics = &TopicListState{ChatID: 2, Results: []domain.ForumTopic{{ID: 1, ChatID: 2, Name: "General"}}}
		}},
		{id: "members", suppressesToast: true, activate: func(m *ViewModel) {
			m.Members = &MembersState{ChatID: 2, Loading: true}
		}},
		{id: "invite-links", suppressesToast: true, activate: func(m *ViewModel) {
			m.InviteLinks = &InviteLinksState{ChatID: 2, Loading: true}
		}},
		{id: "administration", suppressesToast: true, activate: func(m *ViewModel) {
			m.Administration = &AdministrationState{ChatID: 2, Loading: true}
		}},
		{id: "chat-settings", suppressesToast: true, activate: func(m *ViewModel) {
			m.ChatSettings = &ChatSettingsState{ChatID: 2, Mode: ChatSettingsTitleEditor, TitleInput: []rune("Title")}
		}},
		{id: "authorization", suppressesToast: false, activate: func(m *ViewModel) {
			m.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}, Input: []rune("123")}
		}},
	}
}

// modalStackTestContext builds the per-frame overlay context a composition
// needs: a real root layer plus the undimmed overlay styles.
func modalStackTestContext(model ViewModel) modalContext {
	if model.Width <= 0 || model.Height <= 0 {
		model.Width, model.Height = 100, 24
	}
	root := lipgloss.NewLayer(lipgloss.NewStyle().Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	return modalContext{
		model:    model,
		location: time.UTC,
		bounds:   image.Rect(0, 0, model.Width, model.Height),
		root:     root,
		styles:   newRenderStyles(false),
	}
}

// activeIDs lists the IDs of the specs the stack picked for one frame.
func activeIDs(specs []modalSpec) []string {
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.id)
	}
	return ids
}

// Registry precedence is the exact historical order: media first/bottom-most,
// authorization last/topmost, and Members deliberately before the invite-link,
// administration, and chat-settings overlays it must sit under.
func TestDefaultModalStackPrecedenceOrder(t *testing.T) {
	cases := modalStackOverlayCases()
	stack := defaultModalStack()

	want := make([]string, 0, len(cases))
	for _, tc := range cases {
		want = append(want, tc.id)
	}
	got := stack.ids()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("registry order = %v, want %v", got, want)
	}

	// Every overlay active at once (Members hidden by the media modal) renders
	// in exactly registry order, so a later spec always paints above the
	// earlier ones.
	model := frameBaseModel(120, 30)
	for _, tc := range cases {
		tc.activate(&model)
	}
	ctx := modalStackTestContext(model)
	wantRendered := []string{
		"media", "message-menu", "reaction-picker", "forward-picker", "sticker-picker",
		"photo-send", "message-search", "chat-search", "chat-actions", "pinned-messages",
		"topics", "invite-links", "administration", "chat-settings", "authorization",
	}
	result := stack.compose(ctx, modalBase{})
	if strings.Join(result.rendered, ",") != strings.Join(wantRendered, ",") {
		t.Errorf("render order = %v, want %v (members must stay hidden under media)", result.rendered, wantRendered)
	}
	if last := result.rendered[len(result.rendered)-1]; last != "authorization" {
		t.Errorf("topmost overlay = %q, want authorization", last)
	}
}

// AnyActive is the single base-dim predicate, so it must be true for every
// overlay that used to appear in the composeApplication condition - and for
// each of them exactly that one overlay activates.
func TestModalStackAnyActiveCoversEveryOverlay(t *testing.T) {
	stack := defaultModalStack()
	clean := modalStackTestContext(frameBaseModel(100, 24))
	if stack.anyActive(clean) {
		t.Fatal("clean base model reports an active overlay")
	}
	if got := stack.activeSpecs(clean); len(got) != 0 {
		t.Fatalf("clean base model active specs = %v, want none", stack.ids())
	}

	for _, tc := range modalStackOverlayCases() {
		t.Run(tc.id, func(t *testing.T) {
			model := frameBaseModel(100, 24)
			tc.activate(&model)
			ctx := modalStackTestContext(model)

			if !stack.anyActive(ctx) {
				t.Fatalf("%s is active but does not dim the base", tc.id)
			}
			active := stack.activeSpecs(ctx)
			if len(active) != 1 || active[0].id != tc.id {
				t.Fatalf("active specs = %v, want exactly %s", activeIDs(active), tc.id)
			}

			// The dim predicate must reach the composed frame: the status brand
			// cell is Faint while the overlay is open.
			frame := composeApplication(model, time.Local)
			cell := lipgloss.NewCanvas(model.Width, model.Height).Compose(frame.Compositor).CellAt(1, 0)
			if cell == nil {
				t.Fatal("status brand cell is nil")
			}
			if cell.Content != "t" {
				t.Fatalf("status brand cell = %q, want the brand glyph", cell.Content)
			}
			if cell.Style.Attrs&uv.AttrFaint == 0 {
				t.Errorf("status brand cell is not Faint under %s", tc.id)
			}
		})
	}

	// A clean base frame keeps the brand bright, proving the dim predicate is
	// the registry answer and not a constant.
	base := composeApplication(frameBaseModel(100, 24), time.Local)
	cell := lipgloss.NewCanvas(100, 24).Compose(base.Compositor).CellAt(1, 0)
	if cell == nil || cell.Style.Attrs&uv.AttrFaint != 0 {
		t.Errorf("clean base status brand cell unexpectedly dimmed: %+v", cell)
	}
}

// The toast policy is the exact historical suppression set. Every other
// overlay - reaction picker, message search, topics, authorization - keeps a
// visible toast above the dimmed base.
func TestModalStackToastSuppressionPolicy(t *testing.T) {
	stack := defaultModalStack()
	specs := map[string]modalSpec{}
	for _, spec := range stack.specs {
		specs[spec.id] = spec
	}

	for _, tc := range modalStackOverlayCases() {
		spec, ok := specs[tc.id]
		if !ok {
			t.Fatalf("registry has no overlay %q", tc.id)
		}
		if spec.suppressesToast != tc.suppressesToast {
			t.Errorf("%s spec suppressesToast = %v, want %v", tc.id, spec.suppressesToast, tc.suppressesToast)
		}

		model := frameBaseModel(100, 24)
		model.Toast = &domain.AppError{Message: "Message copied"}
		tc.activate(&model)
		ctx := modalStackTestContext(model)
		if got := stack.suppressesToast(ctx); got != tc.suppressesToast {
			t.Errorf("%s stack suppressesToast = %v, want %v", tc.id, got, tc.suppressesToast)
		}

		// The answer must drive the frame, not just the registry.
		frame := composeApplication(model, time.Local)
		content := ansi.Strip(frame.Content)
		if visible := strings.Contains(content, "Message copied"); visible == tc.suppressesToast {
			t.Errorf("%s toast visible = %v, want %v", tc.id, visible, !tc.suppressesToast)
		}
	}

	clean := frameBaseModel(100, 24)
	clean.Toast = &domain.AppError{Message: "Message copied"}
	ctx := modalStackTestContext(clean)
	if stack.suppressesToast(ctx) {
		t.Error("clean base model suppresses the toast")
	}
	if content := ansi.Strip(composeApplication(clean, time.Local).Content); !strings.Contains(content, "Message copied") {
		t.Error("clean base frame lost its toast")
	}
}

// The latest active overlay owns interactions and the cursor: the base plane
// is replaced, never merged, and each overlay keeps its own cursor semantics.
func TestModalStackComposeReplacesInteractionsAndCursor(t *testing.T) {
	baseInteraction := layerInteraction{
		ID:    "chat:2",
		Rect:  image.Rect(0, 0, 30, 20),
		Z:     zRowBackground,
		Click: ActionReceived{Action: SelectChat, ChatID: 2},
	}
	sentinel := renderCursor{X: 42, Y: 21, Visible: true}

	clean := modalStackTestContext(frameBaseModel(100, 24))
	result := defaultModalStack().compose(clean, modalBase{interactions: []layerInteraction{baseInteraction}, cursor: sentinel})
	if len(result.rendered) != 0 {
		t.Fatalf("clean base rendered %v, want nothing", result.rendered)
	}
	if len(result.interactions) != 1 || result.interactions[0].ID != baseInteraction.ID {
		t.Fatalf("clean base interactions = %v, want the untouched base plane", result.interactions)
	}
	if result.cursor != sentinel {
		t.Fatalf("clean base cursor = %+v, want the untouched base cursor", result.cursor)
	}

	hiddenCursor := map[string]bool{
		"media": true, "message-menu": true, "reaction-picker": true, "forward-picker": true,
		"sticker-picker": true, "chat-actions": true, "topics": true,
	}
	for _, tc := range modalStackOverlayCases() {
		t.Run(tc.id, func(t *testing.T) {
			model := frameBaseModel(100, 24)
			tc.activate(&model)
			ctx := modalStackTestContext(model)
			result := defaultModalStack().compose(ctx, modalBase{interactions: []layerInteraction{baseInteraction}, cursor: sentinel})
			if len(result.rendered) != 1 || result.rendered[0] != tc.id {
				t.Fatalf("rendered = %v, want [%s]", result.rendered, tc.id)
			}
			if len(result.interactions) == 0 && tc.id != "authorization" {
				t.Fatalf("%s dropped the base interactions without owning its own", tc.id)
			}
			for _, interaction := range result.interactions {
				if interaction.ID == baseInteraction.ID {
					t.Fatalf("%s kept a base interaction: %+v", tc.id, interaction)
				}
			}
			if hiddenCursor[tc.id] {
				if result.cursor.Visible || result.cursor.X >= 0 || result.cursor.Y >= 0 {
					t.Errorf("%s cursor = %+v, want the hidden cursor", tc.id, result.cursor)
				}
			} else if result.cursor == sentinel {
				t.Errorf("%s kept the base cursor %+v", tc.id, result.cursor)
			}

			// The composed frame agrees with the registry answer.
			frame := composeApplication(model, time.Local)
			if frame.Cursor != result.cursor {
				t.Errorf("%s frame cursor = %+v, registry cursor = %+v", tc.id, frame.Cursor, result.cursor)
			}
			if tc.id == "media" {
				// The media overlay owns the whole plane: every remaining hit is
				// its own dismiss, never a base chat selection.
				for _, hit := range frame.Hits {
					if hit.Click.Action != Close {
						t.Errorf("media frame kept a non-overlay hit: %+v", hit)
					}
				}
			}
		})
	}
}

// Members sits at its historical narrowest predicate: it owns the frame while
// open alone, and disappears under the media transport overlay.
func TestModalStackMembersHiddenUnderMedia(t *testing.T) {
	stack := defaultModalStack()
	members := &MembersState{
		ChatID: 2,
		Results: []domain.ChatMember{
			{User: domain.User{ID: 1, Name: "Ada"}, Role: domain.ChatMemberRoleOwner},
			{User: domain.User{ID: 2, Name: "Bob"}, Role: domain.ChatMemberRoleMember},
		},
	}

	solo := frameBaseModel(100, 24)
	solo.Members = members
	if got := activeIDs(stack.activeSpecs(modalStackTestContext(solo))); len(got) != 1 || got[0] != "members" {
		t.Fatalf("members-only active specs = %v, want [members]", got)
	}
	soloCtx := modalStackTestContext(solo)
	soloResult := stack.compose(soloCtx, modalBase{})
	if strings.Join(soloResult.rendered, ",") != "members" {
		t.Fatalf("members-only render order = %v, want [members]", soloResult.rendered)
	}
	if soloPaint := lipgloss.NewCompositor(soloCtx.root).Render(); !strings.Contains(ansi.Strip(soloPaint), "Ada") {
		t.Error("members overlay alone did not paint its list")
	}
	if count := countMemberHits(composeApplication(solo, time.Local).Hits); count == 0 {
		t.Fatal("members-only frame has no member actions to hide")
	}

	underMedia := solo
	underMedia.Modal = &ModalState{Title: "Photo", Path: "/tmp/photo.jpg"}
	ctx := modalStackTestContext(underMedia)
	if got := activeIDs(stack.activeSpecs(ctx)); len(got) != 1 || got[0] != "media" {
		t.Fatalf("members + media active specs = %v, want [media]", got)
	}
	result := stack.compose(ctx, modalBase{})
	if strings.Join(result.rendered, ",") != "media" {
		t.Fatalf("members + media render order = %v, want [media]", result.rendered)
	}
	paint := ansi.Strip(lipgloss.NewCompositor(ctx.root).Render())
	if !strings.Contains(paint, "Photo") {
		t.Error("media transport overlay was not painted by the stack")
	}
	if strings.Contains(paint, "Ada") {
		t.Error("members list painted while hidden under the media overlay")
	}

	frame := composeApplication(underMedia, time.Local)
	if count := countMemberHits(frame.Hits); count != 0 {
		t.Errorf("member actions survived under the media overlay: %d", count)
	}
	if len(frame.Hits) == 0 {
		t.Error("media overlay lost its own interactions")
	}
	if !strings.Contains(ansi.Strip(frame.Content), "Photo") {
		t.Error("media modal missing from the frame")
	}
}

func countMemberHits(hits []Hit) int {
	count := 0
	for _, hit := range hits {
		if hit.Click.Action == OpenMemberDetail || hit.Click.Action == SelectMember {
			count++
		}
	}
	return count
}

// Authorization stays topmost and receives pointer events itself, so nothing
// under it - including the base plane and every lower overlay - keeps an
// interaction.
func TestModalStackAuthorizationTopmostAndNotInteractive(t *testing.T) {
	stack := defaultModalStack()
	model := frameBaseModel(100, 24)
	model.Modal = &ModalState{Title: "Photo", Path: "/tmp/photo.jpg"}
	model.Members = &MembersState{ChatID: 2, Loading: true}
	model.ChatSettings = &ChatSettingsState{ChatID: 2, Mode: ChatSettingsTitleEditor, TitleInput: []rune("Title")}
	model.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}, Input: []rune("123")}
	ctx := modalStackTestContext(model)

	active := activeIDs(stack.activeSpecs(ctx))
	if len(active) == 0 || active[len(active)-1] != "authorization" {
		t.Fatalf("active specs = %v, want authorization last", active)
	}
	result := stack.compose(ctx, modalBase{
		interactions: []layerInteraction{{ID: "chat:2", Rect: image.Rect(0, 0, 30, 20), Z: zRowBackground, Click: ActionReceived{Action: SelectChat, ChatID: 2}}},
		cursor:       renderCursor{X: 42, Y: 21, Visible: true},
	})
	if result.rendered[len(result.rendered)-1] != "authorization" {
		t.Fatalf("render order = %v, want authorization painted last", result.rendered)
	}
	if len(result.interactions) != 0 {
		t.Fatalf("authorization left interactions behind: %+v", result.interactions)
	}
	if result.cursor == (renderCursor{X: 42, Y: 21, Visible: true}) {
		t.Errorf("authorization kept the base cursor %+v", result.cursor)
	}
	paint := ansi.Strip(lipgloss.NewCompositor(ctx.root).Render())
	if !strings.Contains(paint, "Photo") {
		t.Error("authorization stopped the lower overlays from being added")
	}

	// Without the prompt the same overlays keep their own interactions.
	plain := model
	plain.Prompt = nil
	withOverlay := stack.compose(modalStackTestContext(plain), modalBase{
		interactions: []layerInteraction{{ID: "chat:2", Rect: image.Rect(0, 0, 30, 20), Z: zRowBackground, Click: ActionReceived{Action: SelectChat, ChatID: 2}}},
	})
	if len(withOverlay.interactions) == 0 {
		t.Fatal("chat-settings overlay owned no interactions without the prompt")
	}
	if _, leaked := findInteraction(withOverlay.interactions, "chat:2"); leaked {
		t.Error("base interaction survived an active overlay")
	}

	frame := composeApplication(model, time.Local)
	if len(frame.Hits) != 0 {
		t.Fatalf("authorized frame is interactive: %+v", frame.Hits)
	}
	if !strings.Contains(ansi.Strip(frame.Content), "Telegram API ID") {
		t.Error("authorization panel missing from the frame")
	}
}

func findPlacement(placements []inlinePlacement, imageID uint32) (inlinePlacement, bool) {
	for _, placement := range placements {
		if placement.ImageID == imageID {
			return placement, true
		}
	}
	return inlinePlacement{}, false
}

func findInteraction(interactions []layerInteraction, id string) (layerInteraction, bool) {
	for _, interaction := range interactions {
		if interaction.ID == id {
			return interaction, true
		}
	}
	return layerInteraction{}, false
}

// The media overlay request travels out of the composition unchanged, and its
// transport frame keeps owning the inline plane clip region.
func TestModalStackCapturesMediaTransportOverlay(t *testing.T) {
	stack := defaultModalStack()
	ready := frameBaseModel(100, 24)
	ready.Modal = &ModalState{Title: "Photo", Path: "/tmp/photo.jpg"}
	ctx := modalStackTestContext(ready)
	media, request := buildMediaModalLayer(ctx.model, ctx.styles)
	result := stack.compose(ctx, modalBase{})
	if result.overlay != request {
		t.Fatalf("media overlay = %+v, want %+v", result.overlay, request)
	}
	if result.transportFrame != media.Rect {
		t.Fatalf("media transport frame = %v, want %v", result.transportFrame, media.Rect)
	}
	if !result.overlay.Ready {
		t.Fatal("ready media overlay reported as not ready")
	}
	if result.overlay.Path != "/tmp/photo.jpg" {
		t.Fatalf("media overlay path = %q", result.overlay.Path)
	}

	// A prompt above the media modal does not take the transport slot: the
	// capture still comes from the media overlay, whose own build reports it is
	// not ready to transport while the prompt is open.
	guarded := ready
	guarded.Prompt = &PromptState{Prompt: auth.Prompt{Label: "Telegram API ID"}, Input: []rune("123")}
	guardedCtx := modalStackTestContext(guarded)
	guardedMedia, guardedRequest := buildMediaModalLayer(guardedCtx.model, guardedCtx.styles)
	guardedResult := stack.compose(guardedCtx, modalBase{})
	if guardedResult.overlay != guardedRequest {
		t.Fatalf("guarded media overlay = %+v, want %+v", guardedResult.overlay, guardedRequest)
	}
	if guardedResult.overlay.Ready {
		t.Error("media overlay reported ready while authorization is open")
	}
	if guardedResult.transportFrame != guardedMedia.Rect {
		t.Fatalf("media transport frame lost under authorization: %v, want %v", guardedResult.transportFrame, guardedMedia.Rect)
	}
	if guardedResult.rendered[len(guardedResult.rendered)-1] != "authorization" {
		t.Fatalf("render order = %v, want authorization last", guardedResult.rendered)
	}
}

// The sticker picker replaces the conversation inline plane and clips its own
// placements to the viewport.
func TestModalStackStickerOwnsInlinePlaneAndClipsToViewport(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 30
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusStickerPicker
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{{ID: 77, ChatID: 9, Kind: domain.MessageSticker, Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 700}}}}
	state.Thumbnails[9] = map[domain.MessageID]thumbnail.Block{77: {Text: "\x1b_Ga=T,f=900,i=700,c=10,r=4,q=2,s=64,v=32;conversation-kitty\x1b\\", Width: 10, Height: 4, Kitty: true, ImageID: 700}}
	state.StickerPicker = &StickerPickerState{RequestID: 8, ChatID: 9, Catalog: []domain.StickerRef{{File: domain.MediaFileRef{ID: 101}}}, Columns: 5, VisibleRows: 2}
	state.StickerThumbnails[101] = thumbnail.Block{Text: "\x1b_Ga=T,f=900,i=801,c=10,r=4,q=2,s=64,v=32;picker-kitty\x1b\\", Width: 10, Height: 4, Kitty: true, ImageID: 801}
	model := Select(state, nil)

	// Conversation inline plane while the picker is closed.
	closedState := state
	closedState.StickerPicker = nil
	closedState.Focus = FocusConversation
	conversationInline := composeApplication(Select(closedState, nil), nil).Inline
	if len(conversationInline) != 1 || conversationInline[0].ImageID == 801 {
		t.Fatalf("conversation inline plane = %+v, want the message sticker placement", conversationInline)
	}

	ctx := modalStackTestContext(model)
	leaf := buildStickerPickerLayer(model, ctx.styles)
	if len(leaf.Inline) != 1 || leaf.Inline[0].ImageID != 801 {
		t.Fatalf("picker placements = %+v", leaf.Inline)
	}
	result := defaultModalStack().compose(ctx, modalBase{inline: conversationInline})
	if !result.ownsInline {
		t.Fatal("sticker picker did not claim inline ownership")
	}
	if len(result.inline) != 1 || result.inline[0].ImageID != 801 {
		t.Fatalf("composed inline plane = %+v, want only the picker placement", result.inline)
	}
	// Owning the plane means the picker's placements are clipped to the
	// viewport only: its own modal frame must not subtract them away.
	want := clipInlinePlacements(leaf.Inline, ctx.bounds)
	if len(result.inline) != len(want) || result.inline[0].X != want[0].X || result.inline[0].Width != want[0].Width {
		t.Fatalf("picker placements = %+v, want viewport-clipped %+v", result.inline, want)
	}
	if result.clipFrame != leaf.Rect {
		t.Fatalf("clip frame = %v, want the picker frame %v", result.clipFrame, leaf.Rect)
	}
}

// fakeOverlay is a test-only modal spec. It proves the registry is the only
// place that knows about an overlay: a spec with an ID, an active predicate,
// toast metadata, and a render callback participates in composition exactly
// like a production overlay.
type fakeOverlay struct {
	t               *testing.T
	id              string
	rect            image.Rectangle
	modal           bool
	suppressesToast bool
	ownsInline      bool
	transport       bool
	inline          []inlinePlacement
	renderCalls     int
}

func (f *fakeOverlay) spec() modalSpec {
	return modalSpec{
		id:              f.id,
		active:          func(modalContext) bool { return true },
		suppressesToast: f.suppressesToast,
		render: func(ctx modalContext) modalResult {
			f.renderCalls++
			// The stack, never the callback, attaches the layer exactly once.
			if ctx.root.GetLayer(f.id+":panel") != nil {
				f.t.Errorf("overlay %s attached its own layer inside the callback", f.id)
			}
			content := lipgloss.NewStyle().Width(f.rect.Dx()).Height(f.rect.Dy()).Render("")
			layer := lipgloss.NewLayer(content).ID(f.id + ":panel").X(f.rect.Min.X).Y(f.rect.Min.Y).Z(zModalFrame)
			surface := surfaceResult{
				Layer: layer,
				Rect:  f.rect,
				Interactions: []layerInteraction{{
					ID: f.id + ":dismiss", Rect: f.rect, Z: zModalFrame,
					Click: ActionReceived{Action: Close},
				}},
				Cursor:  renderCursor{X: -1, Y: -1},
				Inline:  f.inline,
				IsModal: f.modal,
			}
			result := hiddenCursorModalResult(surface)
			if f.ownsInline {
				result.inline = inlinePolicyOwned
			}
			if f.transport {
				result.overlay = overlayRequest{Path: "/tmp/fake.jpg", Bounds: f.rect, Columns: f.rect.Dx(), Rows: f.rect.Dy(), Ready: true}
				result.transport = true
			}
			return result
		},
	}
}

// kittyInlinePlacement builds one conversation-style placement the inline
// clipper can parse (real Kitty control with source dimensions).
func kittyInlinePlacement(id uint32, x, y, width, height int) inlinePlacement {
	return inlinePlacement{
		ImageID: id, X: x, Y: y, Width: width, Height: height,
		Text: "\x1b_Ga=T,f=100,i=" + strconv.FormatUint(uint64(id), 10) +
			",c=" + strconv.Itoa(width) + ",r=" + strconv.Itoa(height) +
			",q=2,s=" + strconv.Itoa(width*8) + ",v=" + strconv.Itoa(height*16) + ";AAAA\x1b\\",
	}
}

// Every open modal frame subtracts from the inline plane: the clip region is
// the union of the active IsModal rects, and the transport overlay's own frame
// is only the standalone fallback.
func TestModalStackClipsInlineToUnionOfEveryModalFrame(t *testing.T) {
	model := frameBaseModel(100, 24)
	ctx := modalStackTestContext(model)
	below := kittyInlinePlacement(900, 12, 12, 4, 2)
	coveredBySecond := kittyInlinePlacement(901, 60, 12, 4, 2)
	free := kittyInlinePlacement(902, 90, 2, 2, 2)
	base := []inlinePlacement{below, coveredBySecond, free}

	first := &fakeOverlay{t: t, id: "fake-one", rect: image.Rect(10, 10, 30, 20), modal: true}
	second := &fakeOverlay{t: t, id: "fake-two", rect: image.Rect(50, 8, 70, 22), modal: true}
	nonModal := &fakeOverlay{t: t, id: "fake-plain", rect: image.Rect(80, 10, 95, 18)}
	stack := newModalStack(first.spec(), second.spec(), nonModal.spec())

	result := stack.compose(ctx, modalBase{inline: base})
	wantClip := image.Rect(10, 8, 70, 22)
	if result.clipFrame != wantClip {
		t.Fatalf("clip frame = %v, want the union of both modal rects %v", result.clipFrame, wantClip)
	}
	want := clipInlinePlacementsWithOverlay(base, ctx.bounds, wantClip)
	if len(result.inline) != len(want) || !reflect.DeepEqual(result.inline, want) {
		t.Fatalf("clipped inline = %+v, want the union-clipped %+v", result.inline, want)
	}
	if len(result.inline) != 1 || result.inline[0].ImageID != 902 {
		t.Fatalf("clipped inline = %+v, want only the uncovered placement 902: both covered ones cropped away", result.inline)
	}
	if first.renderCalls != 1 || second.renderCalls != 1 || nonModal.renderCalls != 1 {
		t.Fatalf("render calls = %d/%d/%d, want one each", first.renderCalls, second.renderCalls, nonModal.renderCalls)
	}
	if ctx.root.GetLayer("fake-plain:panel") == nil {
		t.Error("a non-modal active overlay was not added to the root")
	}

	// Without any IsModal surface the transport overlay's own frame keeps the
	// historical standalone clip fallback.
	transportCtx := modalStackTestContext(model)
	transport := &fakeOverlay{t: t, id: "fake-transport", rect: image.Rect(20, 5, 40, 15), transport: true}
	transportBase := append(append([]inlinePlacement(nil), base...), kittyInlinePlacement(905, 22, 8, 4, 2))
	transportResult := newModalStack(transport.spec()).compose(transportCtx, modalBase{inline: transportBase})
	if !transportResult.clipFrame.Empty() {
		t.Fatalf("clip frame = %v, want empty without a modal surface", transportResult.clipFrame)
	}
	if transportResult.transportFrame != transport.rect {
		t.Fatalf("transport frame = %v, want %v", transportResult.transportFrame, transport.rect)
	}
	wantTransport := clipInlinePlacementsWithOverlay(transportBase, transportCtx.bounds, transport.rect)
	if !reflect.DeepEqual(transportResult.inline, wantTransport) {
		t.Fatalf("transport-clipped inline = %+v, want %+v", transportResult.inline, wantTransport)
	}
	if len(clipInlinePlacementsWithOverlay([]inlinePlacement{kittyInlinePlacement(905, 22, 8, 4, 2)}, transportCtx.bounds, transport.rect)) != 0 {
		t.Fatal("reference clip does not crop a placement fully inside the transport frame")
	}
	for _, placement := range transportResult.inline {
		if placement.ImageID == 905 {
			t.Errorf("placement under the transport frame survived clipping: %+v", placement)
		}
	}

	// An inline-plane owner replaces the conversation plane and is clipped to
	// the viewport only.
	ownerCtx := modalStackTestContext(model)
	owner := &fakeOverlay{
		t: t, id: "fake-owner", rect: image.Rect(5, 5, 25, 15), modal: true,
		ownsInline: true, inline: []inlinePlacement{kittyInlinePlacement(950, 90, 20, 20, 8)},
	}
	ownerResult := newModalStack(owner.spec()).compose(ownerCtx, modalBase{inline: base})
	if !ownerResult.ownsInline {
		t.Fatal("inline ownership was not reported")
	}
	wantOwned := clipInlinePlacements(owner.inline, ownerCtx.bounds)
	if len(wantOwned) != 1 || wantOwned[0].X+wantOwned[0].Width > 100 || wantOwned[0].Y+wantOwned[0].Height > 24 {
		t.Fatalf("reference viewport clip = %+v, want the overflowing placement cropped inside 100x24", wantOwned)
	}
	if !reflect.DeepEqual(ownerResult.inline, wantOwned) {
		t.Fatalf("owned inline = %+v, want the viewport-clipped overlay placement %+v", ownerResult.inline, wantOwned)
	}
	clipped := ownerResult.inline[0]
	if clipped.Width >= 20 || clipped.Height >= 8 {
		t.Fatalf("owned placement %v was not cropped to the viewport", clipped)
	}
	if clipped.X+clipped.Width > 100 || clipped.Y+clipped.Height > 24 || clipped.X < 90 {
		t.Fatalf("cropped owned placement outside the viewport it should sit in: %+v", clipped)
	}
}

// A new overlay joins the frame by registering a spec: it participates in the
// dim/toast answers, is added to the root exactly once, is ordered by the
// registry, and composeApplication is never touched.
func TestModalStackFakeSpecParticipatesWithoutTouchingComposeApplication(t *testing.T) {
	model := frameBaseModel(100, 24)
	model.Toast = &domain.AppError{Message: "Message copied"}
	model.PhotoSend = &PhotoSendState{ChatID: 2, Input: []rune("/tmp/photo.jpg")}
	covered := kittyInlinePlacement(903, 12, 12, 4, 2)
	free := kittyInlinePlacement(904, 92, 2, 2, 2)
	fake := &fakeOverlay{
		t: t, id: "fake-extension", rect: image.Rect(10, 10, 30, 20),
		modal: true, suppressesToast: true,
	}

	// The registered overlays alone leave the placement the fake will crop in
	// place, so the assertions below prove the fake spec really participates.
	plainCtx := modalStackTestContext(model)
	plainResult := defaultModalStack().compose(plainCtx, modalBase{inline: []inlinePlacement{covered, free}})
	if fake.renderCalls != 0 {
		t.Fatalf("unused fake rendered %d times", fake.renderCalls)
	}
	// The registered overlays replace the base plane and own the toast answer.
	withFake := newModalStack(append([]modalSpec{fake.spec()}, defaultModalStack().specs...)...)
	ctx := modalStackTestContext(model)
	if !withFake.anyActive(ctx) {
		t.Fatal("the fake overlay does not dim the base")
	}
	if !withFake.suppressesToast(ctx) {
		t.Fatal("the fake overlay did not suppress the toast through its metadata")
	}
	result := withFake.compose(ctx, modalBase{
		interactions: []layerInteraction{{ID: "chat:2", Rect: image.Rect(0, 0, 30, 20), Z: zRowBackground, Click: ActionReceived{Action: SelectChat, ChatID: 2}}},
		cursor:       renderCursor{X: 42, Y: 21, Visible: true},
		inline:       []inlinePlacement{covered, free},
	})

	if fake.renderCalls != 1 {
		t.Errorf("fake render calls = %d, want exactly one per composition", fake.renderCalls)
	}
	if ctx.root.GetLayer("fake-extension:panel") == nil {
		t.Error("the stack did not attach the fake overlay layer")
	}
	if result.rendered[0] != "fake-extension" {
		t.Errorf("render order = %v, want the fake first (bottom-most)", result.rendered)
	}
	if last := result.rendered[len(result.rendered)-1]; last != "photo-send" {
		t.Errorf("render order = %v, want the registered overlay above the fake", result.rendered)
	}
	if _, leaked := findInteraction(result.interactions, "chat:2"); leaked {
		t.Error("base interaction survived the overlay plane")
	}
	if _, leaked := findInteraction(result.interactions, "fake-extension:dismiss"); leaked {
		t.Error("the lower fake overlay kept interactions the later overlay should own")
	}
	if len(result.interactions) == 0 {
		t.Error("the later registered overlay owns no interactions")
	}
	if _, kept := findPlacement(result.inline, 903); kept {
		t.Error("placement covered only by the fake overlay survived the clip union")
	}
	if _, kept := findPlacement(result.inline, 904); !kept {
		t.Error("the fake overlay cropped a placement outside its frame")
	}
	if len(plainResult.inline) != 2 {
		t.Fatalf("registered-overlays-only composition lost a placement the fake overlay crops: %+v", plainResult.inline)
	}

	// composeApplication must not name a concrete overlay at all: registering
	// the fake above is the only change a new overlay needs.
	body := composeApplicationSource(t)
	for _, forbidden := range []string{
		"model.Modal", "model.MessageMenu", "model.ReactionPicker", "model.ForwardPicker",
		"model.StickerPicker", "model.PhotoSend", "model.MessageSearch", "model.ChatSearch",
		"model.ChatActions", "model.PinnedMessages", "model.Topics", "model.Members",
		"model.InviteLinks", "model.Administration", "model.ChatSettings", "model.Prompt",
		"model.Focus ==", "buildMediaModalLayer", "buildActionModalLayer", "buildMembersLayer",
		"buildStickerPickerLayer", "buildAuthorizationLayer", "clipInlinePlacements",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("composeApplication still names %q", forbidden)
		}
	}
	for _, required := range []string{
		"stack := defaultModalStack()", "stack.anyActive(ctx)", "stack.suppressesToast(ctx)",
		"stack.compose(ctx, modalBase{", "compileHits(composed.interactions)", "composed.cursor",
		"composed.overlay", "composed.inline",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("composeApplication no longer %q", required)
		}
	}

	// The single attach lives in the stack, in exactly one place.
	stackSource, err := os.ReadFile("modal_stack.go")
	if err != nil {
		t.Fatal(err)
	}
	composeBody := functionBody(t, string(stackSource), "func (s modalStack) compose")
	if got := strings.Count(composeBody, "ctx.root.AddLayers("); got != 1 {
		t.Errorf("compose attaches layers in %d places, want exactly one", got)
	}
}

// composeApplicationSource returns the body of the frame composition function.
func composeApplicationSource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("render_frame.go")
	if err != nil {
		t.Fatal(err)
	}
	return functionBody(t, string(source), "func composeApplication(")
}

func functionBody(t *testing.T, source, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("%q not found in the source", signature)
	}
	rest := source[start:]
	end := strings.Index(rest[len(signature):], "\nfunc ")
	if end < 0 {
		t.Fatalf("%q is not followed by another function", signature)
	}
	return rest[:len(signature)+end]
}
