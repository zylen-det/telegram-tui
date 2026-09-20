package frontend

import (
	"image"
	"time"

	"charm.land/lipgloss/v2"
)

// This file holds the overlay (modal) registry that composeApplication uses.
// The registry keeps the small declarative shape of a Crush-style modal stack:
// every overlay registers a stable ID, an active predicate, its toast
// suppression metadata, and a render callback. Feature rendering stays in the
// owning surface_*.go builder; a callback only adapts that builder to the
// registry. composeApplication asks the stack exactly three questions: does any
// overlay dim the base, does any active overlay suppress the toast, and what is
// the composed result of the active overlays.

// modalContext is the immutable per-frame input one overlay receives: the
// authoritative ViewModel snapshot, the shared timestamp location, the viewport
// bounds, the composition root the overlay adds its layer to exactly once, the
// undimmed overlay styles, the typed production editor-view bundle, and whether
// AppModel supplied that bundle at all.
type modalContext struct {
	model    ViewModel
	location *time.Location
	bounds   image.Rectangle
	root     *lipgloss.Layer
	styles   renderStyles
	views    editorViews
	hasViews bool
}

// injectedView reproduces the historical optional-argument seam exactly. With a
// production bundle it returns one value, so the injected Huh View (even an
// empty string) owns the paint. Without a bundle it returns no value, so a
// direct/standalone caller keeps the builder's manual compatibility path.
func (ctx modalContext) injectedView(view string) []string {
	if !ctx.hasViews {
		return nil
	}
	return []string{view}
}

// modalInlinePolicy is how an active overlay treats the Kitty inline plane.
// Kitty image transport deliberately stays outside text composition.
type modalInlinePolicy int

const (
	// inlinePolicyShared leaves the conversation placements in place; the
	// overlay's modal frame only clips away the part it covers.
	inlinePolicyShared modalInlinePolicy = iota
	// inlinePolicyOwned replaces the conversation placements with the overlay's
	// own placements: the overlay owns the entire inline plane while it is open
	// (the sticker picker), so conversation placements are absent rather than
	// merely clipped.
	inlinePolicyOwned
)

// modalResult is one overlay's render result: the surfaceResult it painted
// (frame rect, interactions, modal flag) plus the policies that overlay owns,
// namely the frame cursor semantics, the inline-plane ownership policy, whether
// it clears the base interactions, and the optional Kitty transport request.
type modalResult struct {
	surface   surfaceResult
	cursor    renderCursor
	inline    modalInlinePolicy
	clears    bool
	overlay   overlayRequest
	transport bool
}

// hiddenCursorModalResult is the common overlay result: the overlay hides the
// terminal cursor.
func hiddenCursorModalResult(surface surfaceResult) modalResult {
	return modalResult{surface: surface, cursor: renderCursor{X: -1, Y: -1}}
}

// surfaceCursorModalResult is the overlay result whose own surface drives the
// terminal cursor (inputs and list modals that report a cursor).
func surfaceCursorModalResult(surface surfaceResult) modalResult {
	return modalResult{surface: surface, cursor: surface.Cursor}
}

// nonInteractiveModalResult is the authorization result: the overlay is not
// mouse-interactive, so it clears the frame interactions, and it keeps its own
// cursor.
func nonInteractiveModalResult(surface surfaceResult) modalResult {
	result := surfaceCursorModalResult(surface)
	result.clears = true
	return result
}

// modalSpec is one registered overlay. Specs are ordered bottom-most first: a
// later active spec paints above, and therefore replaces the interactions and
// cursor of, every earlier one.
type modalSpec struct {
	// id is the stable registry identity used for ordering and assertions.
	id string
	// active reports whether this overlay participates in the current frame.
	active func(modalContext) bool
	// suppressesToast is the overlay's toast metadata: while active it hides
	// the base toast layer instead of leaving it faint.
	suppressesToast bool
	// render builds the overlay surface through the owning feature builder. The
	// stack, never the callback, attaches the returned layer to the root.
	render func(modalContext) modalResult
}

// modalStack is the ordered overlay registry: a plain value over a slice of
// specs, so a caller (or test) can build its own stack or add a fake spec and
// let a new overlay participate in frame composition without touching
// composeApplication.
type modalStack struct {
	specs []modalSpec
}

// newModalStack builds a stack from the given bottom-most-first specs.
func newModalStack(specs ...modalSpec) modalStack {
	return modalStack{specs: specs}
}

// ids returns the registered stable IDs in precedence order.
func (s modalStack) ids() []string {
	ids := make([]string, 0, len(s.specs))
	for _, spec := range s.specs {
		ids = append(ids, spec.id)
	}
	return ids
}

// anyActive reports whether any registered overlay is active for this frame. It
// is the single base-dim predicate: the base surfaces are Faint exactly when an
// overlay paints above them.
func (s modalStack) anyActive(ctx modalContext) bool {
	for _, spec := range s.specs {
		if spec.active != nil && spec.active(ctx) {
			return true
		}
	}
	return false
}

// suppressesToast reports whether any active overlay hides the toast. The
// policy lives only in each spec's metadata, so the suppression set is exactly
// the registered overlays that declare it.
func (s modalStack) suppressesToast(ctx modalContext) bool {
	for _, spec := range s.specs {
		if !spec.suppressesToast {
			continue
		}
		if spec.active != nil && spec.active(ctx) {
			return true
		}
	}
	return false
}

// activeSpecs returns the active specs in precedence order, bottom-most first.
func (s modalStack) activeSpecs(ctx modalContext) []modalSpec {
	var active []modalSpec
	for _, spec := range s.specs {
		if spec.active == nil || spec.render == nil {
			continue
		}
		if spec.active(ctx) {
			active = append(active, spec)
		}
	}
	return active
}

// modalBase is the base-surface state the stack composes overlays onto: the
// interactions and cursor collected from the base surfaces plus the
// conversation inline placements.
type modalBase struct {
	interactions []layerInteraction
	cursor       renderCursor
	inline       []inlinePlacement
}

// modalStackResult is the structured outcome of overlay composition, returned
// to composeApplication instead of the stack mutating unrelated state. Besides
// the composed interactions, cursor, and inline placements it reports the
// Kitty transport request captured from the transport overlay, the union of
// modal frames used for inline clipping, that transport overlay's own frame
// kept as the historical standalone clip fallback, and the IDs of the overlays
// that actually rendered.
type modalStackResult struct {
	interactions   []layerInteraction
	cursor         renderCursor
	inline         []inlinePlacement
	overlay        overlayRequest
	clipFrame      image.Rectangle
	transportFrame image.Rectangle
	ownsInline     bool
	rendered       []string
}

// compose renders every active overlay onto the context root in precedence
// order and returns the composed frame state. Each active overlay adds its
// layer exactly once, replaces (never appends) the interactions with its own,
// applies its own cursor semantics, and contributes its modal frame rect to the
// inline clip union. The transport overlay's request is captured for the Kitty
// publisher, an inline-plane owner replaces the conversation placements, and a
// non-interactive overlay clears the interactions entirely.
func (s modalStack) compose(ctx modalContext, base modalBase) modalStackResult {
	result := modalStackResult{
		interactions: base.interactions,
		cursor:       base.cursor,
		inline:       base.inline,
	}
	for _, spec := range s.activeSpecs(ctx) {
		rendered := spec.render(ctx)
		if rendered.surface.Layer != nil {
			ctx.root.AddLayers(rendered.surface.Layer)
		}
		result.rendered = append(result.rendered, spec.id)
		if rendered.clears {
			result.interactions = nil
		} else {
			result.interactions = rendered.surface.Interactions
		}
		result.cursor = rendered.cursor
		if rendered.transport {
			result.overlay = rendered.overlay
			result.transportFrame = rendered.surface.Rect
		}
		if rendered.inline == inlinePolicyOwned {
			result.inline = rendered.surface.Inline
			result.ownsInline = true
		}
		if surface := rendered.surface; surface.IsModal && !surface.Rect.Empty() {
			if result.clipFrame.Empty() {
				result.clipFrame = surface.Rect
			} else {
				result.clipFrame = result.clipFrame.Union(surface.Rect)
			}
		}
	}

	// Inline clipping keeps the historical precedence: an inline-plane owner is
	// clipped to the viewport bounds only, otherwise every open modal frame
	// (media, list, photo, and auth share the same path through IsModal)
	// subtracts from the conversation placements, with the transport overlay's
	// own frame as the standalone fallback. The pane clip already happened
	// inside buildHistoryLayer.
	switch {
	case result.ownsInline:
		result.inline = clipInlinePlacements(result.inline, ctx.bounds)
	case !result.clipFrame.Empty():
		result.inline = clipInlinePlacementsWithOverlay(result.inline, ctx.bounds, result.clipFrame)
	case !result.transportFrame.Empty():
		result.inline = clipInlinePlacementsWithOverlay(result.inline, ctx.bounds, result.transportFrame)
	}
	return result
}

// defaultModalStack builds the production registry in the exact frame
// precedence: media modal, message menu, reaction picker, forward picker,
// sticker picker, photo send, message search, chat search, chat actions, pinned
// messages, topics, members (deliberately hidden while the media modal is open
// so a member avatar opened from Members stays topmost while Members state
// survives underneath), invite links, administration, chat settings, and
// authorization last/topmost.
func defaultModalStack() modalStack {
	return newModalStack(
		modalSpec{
			id:              "media",
			active:          func(ctx modalContext) bool { return ctx.model.Modal != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				media, request := buildMediaModalLayer(ctx.model, ctx.styles)
				result := hiddenCursorModalResult(media)
				// The media content rectangle is the Kitty transport request; it
				// is captured for the publisher and stays outside text
				// composition.
				result.overlay, result.transport = request, true
				return result
			},
		},
		modalSpec{
			id:              "message-menu",
			active:          func(ctx modalContext) bool { return ctx.model.MessageMenu != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				// With a production bundle the injected Huh Selector View (even
				// when empty) replaces the manual row labels/selection paint;
				// without one the builder keeps the manual rows compatibility.
				return hiddenCursorModalResult(buildActionModalLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...))
			},
		},
		modalSpec{
			id:     "reaction-picker",
			active: func(ctx modalContext) bool { return ctx.model.ReactionPicker != nil },
			// The reaction picker deliberately keeps the toast: it only dims the
			// base, and therefore the toast, through the base dim styles.
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildReactionPickerLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...))
			},
		},
		modalSpec{
			id:              "forward-picker",
			active:          func(ctx modalContext) bool { return ctx.model.ForwardPicker != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildForwardPickerLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...))
			},
		},
		modalSpec{
			id:              "sticker-picker",
			active:          func(ctx modalContext) bool { return ctx.model.StickerPicker != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				result := hiddenCursorModalResult(buildStickerPickerLayer(ctx.model, ctx.styles))
				// The picker owns the entire inline-image plane while open.
				result.inline = inlinePolicyOwned
				return result
			},
		},
		modalSpec{
			id:              "photo-send",
			active:          func(ctx modalContext) bool { return ctx.model.PhotoSend != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				// With a production bundle the injected Huh Photo View (even
				// when empty) replaces the manual path and terminal cursor.
				return surfaceCursorModalResult(buildPhotoSendModalLayer(ctx.bounds, photoSendModalData{Path: string(ctx.model.PhotoSend.Input)}, ctx.styles, ctx.injectedView(ctx.views.PhotoPath)...))
			},
		},
		modalSpec{
			id:     "message-search",
			active: func(ctx modalContext) bool { return ctx.model.MessageSearch != nil },
			render: func(ctx modalContext) modalResult {
				// Without a production bundle ctx.views is the zero value, which
				// is exactly the empty injection the direct caller got before.
				return surfaceCursorModalResult(buildMessageSearchLayer(ctx.model, ctx.location, ctx.styles, ctx.views.MessageSearch, ctx.views.Selector))
			},
		},
		modalSpec{
			id:              "chat-search",
			active:          func(ctx modalContext) bool { return ctx.model.ChatSearch != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return surfaceCursorModalResult(buildChatSearchLayer(ctx.model, ctx.location, ctx.styles, ctx.views.ChatSearch, ctx.views.Selector))
			},
		},
		modalSpec{
			id:              "chat-actions",
			active:          func(ctx modalContext) bool { return ctx.model.ChatActions != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildChatActionLayer(ctx.model, ctx.styles, ctx.views.Selector))
			},
		},
		modalSpec{
			id:              "pinned-messages",
			active:          func(ctx modalContext) bool { return ctx.model.PinnedMessages != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return surfaceCursorModalResult(buildPinnedMessagesLayer(ctx.model, ctx.location, ctx.styles, ctx.views.Selector))
			},
		},
		modalSpec{
			id:     "topics",
			active: func(ctx modalContext) bool { return ctx.model.Topics != nil },
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildTopicsLayer(ctx.model, ctx.styles, ctx.injectedView(ctx.views.Selector)...))
			},
		},
		modalSpec{
			id:              "members",
			active:          func(ctx modalContext) bool { return ctx.model.Members != nil && ctx.model.Modal == nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return surfaceCursorModalResult(buildMembersLayer(ctx.model, ctx.styles, ctx.views.Selector))
			},
		},
		modalSpec{
			id:              "invite-links",
			active:          func(ctx modalContext) bool { return ctx.model.InviteLinks != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildInviteLinksLayer(ctx.model, ctx.styles))
			},
		},
		modalSpec{
			id:              "administration",
			active:          func(ctx modalContext) bool { return ctx.model.Administration != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				return hiddenCursorModalResult(buildAdministrationLayer(ctx.model, ctx.styles))
			},
		},
		modalSpec{
			id:              "chat-settings",
			active:          func(ctx modalContext) bool { return ctx.model.ChatSettings != nil },
			suppressesToast: true,
			render: func(ctx modalContext) modalResult {
				inputView := ctx.views.ChatSettingsTitle
				if ctx.model.ChatSettings.Mode == ChatSettingsDescriptionEditor {
					inputView = ctx.views.ChatSettingsDescription
				}
				return surfaceCursorModalResult(buildChatSettingsLayer(ctx.model, ctx.styles, inputView))
			},
		},
		modalSpec{
			id: "authorization",
			active: func(ctx modalContext) bool {
				return ctx.model.Prompt != nil || ctx.model.Focus == FocusAuth
			},
			render: func(ctx modalContext) modalResult {
				var data authorizationData
				if ctx.model.Prompt != nil {
					data = authorizationData{
						Label:    ctx.model.Prompt.Prompt.Label,
						Guidance: authorizationPromptGuidance(ctx.model.Prompt.Prompt.Kind),
						Input:    string(ctx.model.Prompt.Input),
						Secret:   ctx.model.Prompt.Prompt.Secret,
						Waiting:  false,
					}
				} else {
					data = authorizationData{Waiting: true}
				}
				// With a production bundle the injected Huh Input View (even when
				// empty) replaces the legacy display and terminal cursor; without
				// one the builder keeps the manual display, masking, placeholder,
				// and cursor compatibility.
				return nonInteractiveModalResult(buildAuthorizationLayer(ctx.bounds, data, ctx.styles, ctx.injectedView(ctx.views.Authorization)...))
			},
		},
	)
}
