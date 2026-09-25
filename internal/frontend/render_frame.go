package frontend

import (
	"fmt"
	"image"
	"time"

	"charm.land/lipgloss/v2"
)

// renderCursor is the absolute viewport cell coordinate of the text cursor.
type renderCursor struct {
	X       int
	Y       int
	Visible bool
}

// layerInteraction is one interactive region of a surface. Rect is in absolute
// viewport cells. Z is the global Lipgloss Z tier. Virtual is true only for
// non-visual modal outside-click rectangles.
type layerInteraction struct {
	ID        string
	Rect      image.Rectangle
	Z         int
	Click     ActionReceived
	WheelUp   ActionReceived
	WheelDown ActionReceived
	Virtual   bool
	Primary   ActionReceived
}

// surfaceResult is the product of one surface builder.
// IsModal marks an opaque modal frame that should hide inline thumbnails
// via clipInlinePlacementsWithOverlay; see composeApplication.
type surfaceResult struct {
	Layer        *lipgloss.Layer
	Rect         image.Rectangle
	Interactions []layerInteraction
	Cursor       renderCursor
	Inline       []inlinePlacement
	IsModal      bool
}

// inlinePlacement is one Kitty graphics thumbnail to emit outside the text
// compositor. X and Y are absolute viewport cells of the image's top-left;
// Width and Height describe the cells the image overlays. Text is the cached
// go-termimg transmit sequence (with a stable image id) emitted with the
// terminal cursor positioned at (X, Y).
type inlinePlacement struct {
	ImageID uint32
	X       int
	Y       int
	Width   int
	Height  int
	Text    string
}

// rect returns the absolute viewport cells the placement overlays.
func (p inlinePlacement) rect() image.Rectangle {
	return image.Rect(p.X, p.Y, p.X+p.Width, p.Y+p.Height)
}

// overlayRequest describes the Kitty image protocol placement for the media
// modal content rectangle.
type overlayRequest struct {
	Path       string
	Bounds     image.Rectangle
	Columns    int
	Rows       int
	SelectedID int64
	Ready      bool
}

// frameResult is the fully composed application frame.
type frameResult struct {
	Content    string
	Hits       HitMap
	Cursor     renderCursor
	Overlay    overlayRequest
	Inline     []inlinePlacement
	Compositor *lipgloss.Compositor // retained internally for architecture tests
}

// Global Lipgloss Z tiers. Z is effectively global in the pinned v2.0.6
// compositor (a child's Z is not added to its parent's Z), so every layer uses
// an explicit global tier. Layers sharing a tier must not overlap.
const (
	zFrame            = 0
	zStatus           = 10
	zPane             = 20
	zPaneBackground   = 21
	zRowBackground    = 30
	zCardFrame        = 35
	zContent          = 40
	zControl          = 50
	zToastFrame       = 60
	zToastContent     = 70
	zCommandMenuFrame = 80
	zCommandMenuRow   = 85
	zCommandMenuText  = 90
	zModalFrame       = 100
	zModalRow         = 110
	zModalContent     = 120
	zModalControl     = 130
	zAuthFrame        = 200
	zAuthContent      = 210
)

// composeApplication is the full-size root/compositor assembler for the normal
// application frame. It builds a single real component root at (0,0) sized to
// the model viewport, attaches the base and overlay surfaces once, creates
// exactly one compositor, renders it once, and compiles the interactions into
// a stable HitMap. Zero or too-small bounds produce the matching safe frame.
// editorViews is the typed production injection bundle carrying the
// persistent editor Huh Views into the composition seam. Composer feeds the
// conversation layer; Authorization feeds the authorization overlay; PhotoPath
// feeds the Photo send modal input. A zero value field means "no view
// injected" and must never fall back to legacy model text.
type editorViews struct {
	Composer      string
	Authorization string

	PhotoPath string

	MessageSearch string

	ChatSearch string

	ChatSettingsTitle       string
	ChatSettingsDescription string
}

// selectEditorViews selects the first injected bundle, or the zero value when
// the seam is called without a bundle (standalone/direct callers).
func selectEditorViews(views []editorViews) editorViews {
	if len(views) == 0 {
		return editorViews{}
	}
	return views[0]
}

func composeApplication(model ViewModel, location *time.Location, views ...editorViews) frameResult {
	selectedViews := selectEditorViews(views)
	if model.Width <= 0 || model.Height <= 0 {
		return frameResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	bounds := image.Rect(0, 0, model.Width, model.Height)
	if model.Layout.Mode == LayoutTooSmall {
		return composeTooSmall(bounds, fmt.Sprintf("telegram-tui requires at least 60x18; current %dx%d", model.Width, model.Height))
	}

	// The overlay registry owns every concrete modal branch: which overlays are
	// active, which suppress the toast, and how the active ones compose.
	stack := defaultModalStack()
	ctx := modalContext{model: model, location: location, bounds: bounds}

	// Base dim: exactly one registered overlay being active dims the base panes.
	baseStyles := newRenderStyles(stack.anyActive(ctx))
	rootContent := baseStyles.Base.Width(model.Width).Height(model.Height).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)
	ctx.root = root
	ctx.styles = newRenderStyles(false)

	var interactions []layerInteraction
	cursor := renderCursor{X: -1, Y: -1}

	// Base surfaces in fixed order; append interactions in the same order.
	status := buildStatusLayer(model, baseStyles)
	if status.Layer != nil {
		root.AddLayers(status.Layer)
	}
	interactions = append(interactions, status.Interactions...)

	chats := buildChatsLayer(model, location, baseStyles)
	if chats.Layer != nil {
		root.AddLayers(chats.Layer)
	}
	interactions = append(interactions, chats.Interactions...)

	conversation := buildConversationLayer(model, location, baseStyles, selectedViews.Composer)
	if conversation.Layer != nil {
		root.AddLayers(conversation.Layer)
	}
	interactions = append(interactions, conversation.Interactions...)
	if conversation.Cursor.Visible {
		cursor = conversation.Cursor
	}
	inline := append([]inlinePlacement(nil), conversation.Inline...)

	details := buildDetailsLayer(model, baseStyles)
	if details.Layer != nil {
		root.AddLayers(details.Layer)
	}
	interactions = append(interactions, details.Interactions...)

	// The toast stays under a clean top layer: every registered overlay that
	// declares toast suppression hides it. A reaction picker rebuilds it
	// faint/dimmed through the base dim styles instead.
	ctx.views = selectedViews
	ctx.hasViews = len(views) > 0
	if !stack.suppressesToast(ctx) {
		toast := buildToastLayer(model, baseStyles)
		if toast.Layer != nil {
			root.AddLayers(toast.Layer)
		}
	}

	// Modal/picker overlays now come from the registry in fixed precedence order,
	// each replacing (not appending) the base interactions and applying its own
	// cursor, inline-ownership, and Kitty transport policies. defaultModalStack
	// owns the order and per-overlay rules; composeApplication never names a
	// concrete overlay field or builder.
	composed := stack.compose(ctx, modalBase{
		interactions: interactions,
		cursor:       cursor,
		inline:       inline,
	})

	compositor := lipgloss.NewCompositor(root)
	content := compositor.Render()

	return frameResult{
		Content:    content,
		Hits:       compileHits(composed.interactions),
		Cursor:     composed.cursor,
		Overlay:    composed.overlay,
		Inline:     composed.inline,
		Compositor: compositor,
	}
}

// compileHits converts a slice of layer interactions into a stable HitMap in
// ascending Z/order. HitMap's reverse search then selects the visual
// topmost action at any point.
func compileHits(interactions []layerInteraction) HitMap {
	if len(interactions) == 0 {
		return nil
	}
	// Stable ascending Z/order: sort by Z, preserving insertion order for
	// equal Z so the reverse search picks the last-inserted (topmost) action.
	sorted := append([]layerInteraction(nil), interactions...)
	stableSortInteractions(sorted)
	hits := make(HitMap, 0, len(sorted))
	for _, interaction := range sorted {
		if interaction.Rect.Empty() {
			continue
		}
		hits = append(hits, Hit{
			Rect:      interaction.Rect,
			Click:     interaction.Click,
			WheelUp:   interaction.WheelUp,
			WheelDown: interaction.WheelDown,
		})
	}
	return hits
}

// stableSortInteractions sorts interactions by ascending Z while preserving
// the relative order of equal-Z entries.
func stableSortInteractions(interactions []layerInteraction) {
	for i := 1; i < len(interactions); i++ {
		for j := i; j > 0 && interactions[j-1].Z > interactions[j].Z; j-- {
			interactions[j-1], interactions[j] = interactions[j], interactions[j-1]
		}
	}
}
