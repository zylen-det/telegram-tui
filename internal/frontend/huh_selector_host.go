package frontend

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/frontend/components"
)

// selectorKind identifies which selector surface a host instance serves.
type selectorKind uint8

const (
	selectorNone selectorKind = iota
	selectorMessageActions
	selectorReaction
	selectorForward
	selectorMessageSearch
	selectorChatSearch
	selectorPinnedMessages
	selectorMembers
	selectorTopics
	selectorChatActions
)

// selectorIdentity scopes a selector snapshot: the surface kind plus the
// request/chat/message coordinates it was compiled for. Any change means the
// host must reset from authoritative/fallback and never retain prior state.
type selectorIdentity struct {
	Kind      selectorKind
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
}

// selectorOption is one stable selectable entry for a Huh Select surface.
// Value carries the exact semantic payload; display order and ID never are.
// The shared list modal row source derives them (see selectorOptionsFromRows).
type selectorOption struct {
	ID    string
	Label string
	Value ActionReceived
}

// selectorHuhOptions converts stable options into Huh options preserving
// order. The display label is the Huh key; the semantic payload is the Huh
// value. nil/empty in returns nil/empty out. Rendering and mouse hit mapping
// both go through this one function, so they can never disagree.
func selectorHuhOptions(options []selectorOption) []huh.Option[ActionReceived] {
	if len(options) == 0 {
		return nil
	}
	converted := make([]huh.Option[ActionReceived], 0, len(options))
	for _, option := range options {
		converted = append(converted, huh.NewOption(option.Label, option.Value))
	}
	return converted
}

// selectorOptionIndex returns the index of the first option whose semantic
// payload exactly equals value, or -1 for nil/empty/missing.
func selectorOptionIndex(options []selectorOption, value ActionReceived) int {
	for index, option := range options {
		if option.Value == value {
			return index
		}
	}
	return -1
}

// selectorHost wraps the one persistent Huh Select used by all selector
// surfaces. It owns the field, its semantic selection, cached identity and
// options, and its focus state. Enter activation (Submit) is disabled here;
// the AppModel owns activation. The members list forwards down/up/j/k to
// Update; other list surfaces still navigate through their reducers.
type selectorHost struct {
	field    *huh.Select[ActionReceived]
	identity selectorIdentity
	value    ActionReceived
	options  []selectorOption
	focused  bool
	width    int
	height   int
}

// selectorFieldHeight compensates Huh v2 Select's internal trailing padding
// row: Height(n) renders at most n-1 option labels plus one blank row, so the
// embedded field needs one extra internal row to display every requested
// visible row. The public/cached height stays the requested visible value.
func selectorFieldHeight(visibleHeight int) int {
	return max(1, visibleHeight) + 1
}

// clipSelectorHuhView clips a Huh Select view back to the exact requested
// public dimensions: at most height rows and at most width terminal cells per
// row, preserving ANSI state and wide-rune cell boundaries, and pads trailing
// cells with plain spaces so the terminal's default background shows through.
func clipSelectorHuhView(view string, width, height int) string {
	if view == "" || width <= 0 || height <= 0 {
		return ""
	}
	rows := strings.Split(view, "\n")
	if len(rows) > height {
		rows = rows[:height]
	}
	for i, row := range rows {
		truncated := ansi.Truncate(row, width, "")
		plain := ansi.Strip(truncated)
		trimmedPlain := strings.TrimRight(plain, " ")
		visibleWidth := ansi.StringWidth(trimmedPlain)
		origWidth := ansi.StringWidth(truncated)
		if visibleWidth < origWidth {
			// Keep the visible text with its original ANSI and pad with plain spaces.
			visiblePart := ansi.Truncate(truncated, visibleWidth, "")
			trailing := strings.Repeat(" ", origWidth-visibleWidth)
			rows[i] = visiblePart + trailing
		} else if visibleWidth < width {
			// No trailing in truncated (already trimmed), pad with plain spaces.
			visiblePart := truncated
			if visibleWidth < width {
				trailing := strings.Repeat(" ", width-visibleWidth)
				visiblePart += trailing
			}
			rows[i] = visiblePart
		} else {
			rows[i] = truncated
		}
	}
	return strings.Join(rows, "\n")
}

// newSelectorField constructs an actual *huh.Select[ActionReceived]
// bound to the host value, styled with the project theme, carrying a fresh
// explicit keymap whose reserved navigation and activation bindings
// (Next/Prev/Submit) are disabled, and clamped to the requested dimensions.
func newSelectorField(value *ActionReceived, width, height int) *huh.Select[ActionReceived] {
	f := huh.NewSelect[ActionReceived]()
	f.Value(value)
	f.WithTheme(huhTheme())
	km := huhKeyMap()
	km.Select.Next.SetKeys()
	km.Select.Prev.SetKeys()
	km.Select.Submit.SetKeys()
	f.WithKeyMap(km)
	f.WithWidth(width)
	f.Height(selectorFieldHeight(height))
	return f
}

// newSelectorHost constructs a fresh host around one configured Select at
// the 1x1 default geometry.
func newSelectorHost() *selectorHost {
	h := &selectorHost{width: 1, height: 1}
	h.field = newSelectorField(&h.value, h.width, h.height)
	return h
}

// Sync aligns the host with one authoritative selector snapshot: identity,
// stable options, an authoritative semantic value, focus, and geometry.
// Width/height are clamped independently to at least 1 and applied every sync.
//
// Value policy:
//   - identity change: reset from authoritative if present, else first option,
//     else the zero payload; prior selector state is never retained.
//   - same identity, options changed: preserve the current semantic value when
//     still present (even against a stale authoritative index); else a valid
//     authoritative; else first; empty options yield the zero payload.
//   - same identity, options unchanged: accept a valid authoritative value
//     change (legacy navigation/snapshot transition); an invalid
//     authoritative preserves the current valid value, else first/zero.
func (h *selectorHost) Sync(identity selectorIdentity, options []selectorOption, authoritative ActionReceived, focused bool, width, height int) tea.Cmd {
	return h.sync(identity, options, authoritative, focused, width, height, false)
}

// SyncAuthoritative aligns the host exactly like Sync except for the
// same-identity changed non-empty options case, where a valid authoritative
// value is chosen before the current semantic value. This is the bounded
// explicit override for the intentional loading-to-settled PreferEdit
// Edit introduction; every other policy (identity change, unchanged options,
// empty transition/rebuild, focus, dimensions, keys, View, copies, and
// geometry) is identical to Sync. An invalid authoritative never displaces a
// valid current value or fallback.
func (h *selectorHost) SyncAuthoritative(identity selectorIdentity, options []selectorOption, authoritative ActionReceived, focused bool, width, height int) tea.Cmd {
	return h.sync(identity, options, authoritative, focused, width, height, true)
}

// sync is the shared host synchronization mechanics behind Sync and
// SyncAuthoritative. preferAuthoritative only affects the same-identity
// changed non-empty options case.
func (h *selectorHost) sync(identity selectorIdentity, options []selectorOption, authoritative ActionReceived, focused bool, width, height int, preferAuthoritative bool) tea.Cmd {
	width = max(1, width)
	height = max(1, height)

	// Fast path for the common composer-typing case where no selector is
	// active: all parameters are identical to the cached state — avoid
	// WithWidth/Height and option rebinding that would otherwise run on
	// every keystroke (long-press hot path).
	if h.identity == identity && h.focused == focused && h.width == width && h.height == height && selectorOptionsEqual(h.options, options) {
		// When options are equal, the chosen value is deterministically
		// either the authoritative (if valid) or the current value;
		// if that chosen value already equals the cached value, no work.
		authoritativeIndex := selectorOptionIndex(options, authoritative)
		var fastChosen ActionReceived
		switch {
		case authoritativeIndex >= 0:
			fastChosen = authoritative
		case selectorOptionIndex(options, h.value) >= 0:
			fastChosen = h.value
		case len(options) > 0:
			fastChosen = options[0].Value
		}
		if h.value == fastChosen {
			return nil
		}
	}

	h.width = width
	h.height = height
	h.field.WithWidth(width)
	h.field.Height(selectorFieldHeight(height))

	sameIdentity := h.identity == identity
	h.identity = identity
	newOptions := append([]selectorOption(nil), options...)
	sameOptions := selectorOptionsEqual(h.options, newOptions)
	// Huh v2 Select.Options is a no-op for zero entries, so a non-empty to
	// empty transition cannot clear the field in place; the internal Select
	// is replaced instead.
	emptyTransition := !sameOptions && len(h.options) > 0 && len(newOptions) == 0
	h.options = newOptions

	authoritativeIndex := selectorOptionIndex(newOptions, authoritative)
	var chosen ActionReceived
	switch {
	case !sameIdentity:
		if authoritativeIndex >= 0 {
			chosen = authoritative
		} else if len(newOptions) > 0 {
			chosen = newOptions[0].Value
		}
	case !sameOptions:
		if preferAuthoritative && authoritativeIndex >= 0 {
			chosen = authoritative
		} else if selectorOptionIndex(newOptions, h.value) >= 0 {
			chosen = h.value
		} else if authoritativeIndex >= 0 {
			chosen = authoritative
		} else if len(newOptions) > 0 {
			chosen = newOptions[0].Value
		}
	default:
		if authoritativeIndex >= 0 {
			chosen = authoritative
		} else if selectorOptionIndex(newOptions, h.value) >= 0 {
			chosen = h.value
		} else if len(newOptions) > 0 {
			chosen = newOptions[0].Value
		}
	}

	if emptyTransition {
		h.value = chosen
		h.field = newSelectorField(&h.value, h.width, h.height)
	} else if !sameOptions {
		// Huh may mutate the bound value when options remove the selection;
		// the explicit rebind below is the source of truth.
		h.field.Options(selectorHuhOptions(newOptions)...)
	}
	h.value = chosen
	h.field.Value(&h.value)

	var cmd tea.Cmd
	if h.focused != focused {
		h.focused = focused
		if focused {
			cmd = h.field.Focus()
		} else {
			cmd = h.field.Blur()
		}
	} else if emptyTransition && h.focused {
		// The replacement Select is fresh and unfocused; restore the
		// requested focus without changing logical focus ownership.
		cmd = h.field.Focus()
	}
	return cmd
}

// Update forwards a message to the embedded Select, retaining the returned
// concrete pointer. It reports changed only on whole semantic payload
// inequality. Enter is inert because Submit is disabled.
func (h *selectorHost) Update(msg tea.Msg) (changed bool, value ActionReceived, cmd tea.Cmd) {
	before := h.value
	updated, cmd := h.field.Update(msg)
	h.field = updated.(*huh.Select[ActionReceived])
	return h.value != before, h.value, cmd
}

// View returns the live Huh view of the embedded Select clipped to the exact
// requested public dimensions; the compensated internal Huh padding row stays
// outside the public View.
func (h *selectorHost) View() string {
	return clipSelectorHuhView(h.field.View(), h.width, h.height)
}

// Identity returns the cached selector identity.
func (h *selectorHost) Identity() selectorIdentity { return h.identity }

// Value returns the cached semantic selection.
func (h *selectorHost) Value() ActionReceived { return h.value }

// Options returns a defensive copy of the cached options.
func (h *selectorHost) Options() []selectorOption {
	return append([]selectorOption(nil), h.options...)
}

// selectorOptionsEqual reports exact ordered selectorOption equality.
func selectorOptionsEqual(a, b []selectorOption) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// selectorHostRect returns the absolute rectangle occupied by the Huh Select
// option rows inside the shared modal. components.Modal.Layout is the sole
// geometry authority, queried with max(optionCount,rowCount) dummy items so
// status rows affect frame centering/height; only rows whose original index
// is < optionCount are selectable, and their visible rows are unioned.
// Empty bounds, zero selectable options, or no visible selectable rows yield
// an empty rectangle.
func selectorHostRect(bounds image.Rectangle, optionCount, rowCount int) image.Rectangle {
	return selectorHostRectWidth(bounds, optionCount, rowCount, 0)
}

// selectorHostRectWidth returns the selectable row union using an optional
// preferred modal width. A nonpositive width preserves the shared compact
// selector geometry.
func selectorHostRectWidth(bounds image.Rectangle, optionCount, rowCount, width int) image.Rectangle {
	if bounds.Empty() || optionCount <= 0 {
		return image.Rectangle{}
	}
	items := make([]components.Item, max(optionCount, rowCount))
	layout := (components.Modal{Items: items, Width: width}).Layout(bounds)
	rect := image.Rectangle{}
	visible := false
	for _, row := range layout.Rows {
		if row.Index >= optionCount {
			continue
		}
		if !visible {
			rect = row.Rect
			visible = true
		} else {
			rect = rect.Union(row.Rect)
		}
	}
	return rect
}
