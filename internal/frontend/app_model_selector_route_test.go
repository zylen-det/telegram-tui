package frontend

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func selectorRouteMenuState() State {
	state := selectorSyncState()
	state.MessageMenu.Loading = false
	state.MessageMenu.Error = nil
	state.MessageMenu.Capabilities = domain.MessageCapabilities{
		Reply: true, Forward: true, Edit: true, Copy: true, Pin: true,
		DeleteForSelf: true, DeleteForAll: true,
	}
	state.MessageMenu.PreferEdit = false
	state.Focus = FocusModal
	state.MessageMenu.PreviousFocus = FocusConversation
	// Stable identity so MessagePropertiesLoaded does not flip Selection.
	state.Chats = []domain.Chat{{ID: 9, Title: "self"}}
	state.SelectedChat = 0
	return state
}

func selectorRouteReactionState() State {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusReactionPicker
	state.ReactionPicker = &ReactionPicker{
		RequestID: 31, ChatID: 9, MessageID: 2, Selected: 0,
	}
	state.Chats = []domain.Chat{{ID: 9}}
	return state
}

func selectorRouteForwardState() State {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusForwardPicker
	state.ForwardPicker = &ForwardPicker{
		RequestID: 41, SourceChatID: 9, SourceMessageID: 2, SelectedChat: 0,
	}
	state.Chats = []domain.Chat{
		{ID: 11, Title: "alpha"},
		{ID: 22, Title: "beta"},
		{ID: 33, Title: "gamma"},
	}
	return state
}

func TestAppModelSelectorRouteMenuNextPreviousClose(t *testing.T) {
	state := selectorRouteMenuState()
	// Pre-populate a message so Edit/Forward/Delete are eligible (Loading=false already).
	state.Messages = map[domain.ChatID][]domain.Message{
		9: {{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi"}},
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	start := model.Snapshot().MessageMenu.Selected
	total := menuVisibleOptions(model.Snapshot().MessageMenu)
	// advance twice
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	wantAfter := (start + 2) % total
	if got := model.Snapshot().MessageMenu.Selected; got != wantAfter {
		t.Fatalf("after j j selected = %d, want %d", got, wantAfter)
	}
	// go back
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "k"}))
	wantBack := (start + 1) % total
	if got := model.Snapshot().MessageMenu.Selected; got != wantBack {
		t.Fatalf("after k selected = %d, want %d", got, wantBack)
	}

	// Esc closes the menu and reverts focus.
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if model.Snapshot().MessageMenu != nil {
		t.Fatalf("Esc left MessageMenu: %#v", model.Snapshot().MessageMenu)
	}
	if model.Snapshot().Focus != FocusConversation {
		t.Fatalf("Esc left focus: %v", model.Snapshot().Focus)
	}
}

func TestAppModelSelectorRouteReactionNextPreviousClose(t *testing.T) {
	state := selectorRouteReactionState()
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	start := model.Snapshot().ReactionPicker.Selected
	palette := len(ReactionPalette)

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := model.Snapshot().ReactionPicker.Selected; got != (start+1)%palette {
		t.Fatalf("reaction j selected = %d, want %d", got, (start+1)%palette)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := model.Snapshot().ReactionPicker.Selected; got != (start+2)%palette {
		t.Fatalf("reaction j x2 selected = %d, want %d", got, (start+2)%palette)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "k"}))
	if got := model.Snapshot().ReactionPicker.Selected; got != (start+1)%palette {
		t.Fatalf("reaction k selected = %d, want %d", got, (start+1)%palette)
	}

	// q closes; Esc also closes for Forward/Reaction tests.
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if model.Snapshot().ReactionPicker != nil || model.Snapshot().Focus != FocusConversation {
		t.Fatalf("Esc left state picker=%#v focus=%v", model.Snapshot().ReactionPicker, model.Snapshot().Focus)
	}
}

func TestAppModelSelectorRouteForwardNextPreviousClose(t *testing.T) {
	state := selectorRouteForwardState()
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	start := model.Snapshot().ForwardPicker.SelectedChat
	count := len(model.Snapshot().Chats)

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := model.Snapshot().ForwardPicker.SelectedChat; got != (start+1)%count {
		t.Fatalf("forward j selected = %d, want %d", got, (start+1)%count)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "k"}))
	if got := model.Snapshot().ForwardPicker.SelectedChat; got != start%count {
		t.Fatalf("forward k selected = %d, want %d", got, start%count)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if model.Snapshot().ForwardPicker != nil || model.Snapshot().Focus != FocusConversation {
		t.Fatalf("Esc left state picker=%#v focus=%v", model.Snapshot().ForwardPicker, model.Snapshot().Focus)
	}
}

// menuVisibleOptions mirrors reducer.actionMenuItemCount exactly so the
// frozen test's selector-cycle assertion stays consistent with the
// authoritative reducer. Any drift between these two functions is a
// production regression.
func menuVisibleOptions(menu *MessageActionMenu) int {
	count := 0
	if (menu.MediaFile.Downloaded && menu.MediaFile.LocalPath != "") ||
		(menu.MediaFile.ID != 0 && menu.MediaFile.CanDownload) {
		count++
	}
	if menu.Capabilities.Reply {
		count++
	}
	if menu.Capabilities.Forward {
		count++
	}
	if menu.Capabilities.Edit {
		count++
	}
	if menu.Capabilities.Copy {
		count++
	}
	if menu.CanReact && !menu.Loading && menu.Error == nil {
		count++
	}
	if menu.Capabilities.Pin {
		count++
	}
	if menu.Capabilities.DeleteForSelf {
		count++
	}
	if menu.Capabilities.DeleteForAll {
		count++
	}
	return count
}

func TestAppModelSelectorRouteMenuVisibleOptionsParity(t *testing.T) {
	tests := []struct {
		name string
		menu *MessageActionMenu
	}{
		{name: "empty", menu: &MessageActionMenu{}},
		{name: "downloadable media", menu: &MessageActionMenu{
			MediaFile: domain.MediaFileRef{ID: 44, CanDownload: true},
		}},
		{name: "downloaded media", menu: &MessageActionMenu{
			MediaFile: domain.MediaFileRef{Downloaded: true, LocalPath: "/tmp/photo.jpg"},
		}},
		{name: "reaction settled", menu: &MessageActionMenu{CanReact: true}},
		{name: "reaction loading hidden", menu: &MessageActionMenu{CanReact: true, Loading: true}},
		{name: "all capabilities", menu: &MessageActionMenu{
			CanReact: true,
			Capabilities: domain.MessageCapabilities{
				Reply: true, Forward: true, Edit: true, Copy: true, Pin: true,
				DeleteForSelf: true, DeleteForAll: true,
			},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, want := menuVisibleOptions(test.menu), len(selectorOptionsFromRows(messageActionRows(test.menu))); got != want {
				t.Fatalf("visible options = %d, compiler options = %d", got, want)
			}
		})
	}
}

func TestAppModelSelectorRouteMenuQClose(t *testing.T) {
	state := selectorRouteMenuState()
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "q"}))
	if model.Snapshot().MessageMenu != nil || model.Snapshot().Focus != FocusConversation {
		t.Fatalf("q left state menu=%#v focus=%v", model.Snapshot().MessageMenu, model.Snapshot().Focus)
	}
}

// TestAppModelSelectorRouteForwardEnterActivatesHighlightedChat locks the
// ForwardPicker Enter contract via a negative oracle: when the source message
// is missing, the reducer's reduceForwardPicker branch closes the picker
// synchronously and flips focus to FocusConversation. If Enter fails to
// dispatch as Activate, the picker stays open and focus remains
// FocusForwardPicker — that is the failure this oracle catches.
func TestAppModelSelectorRouteForwardEnterActivatesHighlightedChat(t *testing.T) {
	state := selectorRouteForwardState()
	state.ForwardPicker.SelectedChat = 1
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.Snapshot().ForwardPicker != nil {
		t.Fatalf("Enter did not dispatch Activate: ForwardPicker retained, state=%#v", model.Snapshot().ForwardPicker)
	}
	if model.Snapshot().Focus != FocusConversation {
		t.Fatalf("Enter did not flip focus: %v", model.Snapshot().Focus)
	}
}

// TestAppModelSelectorRouteMenuEnterResolvesSelectedAction locks the
// MessageMenu Enter contract: pressing Enter re-resolves the highlighted row
// through reducer.selectedMenuAction, so changes to the action table since
// the menu was opened cannot silently keep the user on a stale action.
func TestAppModelSelectorRouteMenuEnterResolvesSelectedAction(t *testing.T) {
	state := selectorRouteMenuState()
	state.MessageMenu.Capabilities = domain.MessageCapabilities{
		Copy: true, Reply: true, Forward: true, Edit: true,
	}
	state.MessageMenu.Selected = 0
	state.Messages = map[domain.ChatID][]domain.Message{
		9: {{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi"}},
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	// j advances Selected to 1, then Enter must agree.
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	// After Enter the reducer accepts selectedMenuAction(ForwardMessageSource)
	// because Reply is index 0 and Forward is index 1. That opens the forward
	// picker for this exact source message. If Enter mapping is removed, the
	// message menu remains open and this oracle fails.
	got := model.Snapshot()
	if got.MessageMenu != nil || got.ForwardPicker == nil {
		t.Fatalf("Enter did not open ForwardPicker: menu=%#v picker=%#v", got.MessageMenu, got.ForwardPicker)
	}
	if got.ForwardPicker.SourceChatID != 9 || got.ForwardPicker.SourceMessageID != 2 || got.Focus != FocusForwardPicker {
		t.Fatalf("Enter opened wrong forward state: picker=%#v focus=%v", got.ForwardPicker, got.Focus)
	}
}

// TestAppModelSelectorRouteReactionEnterResolvesPalette locks the
// ReactionPicker Enter contract: pressing Enter must dispatch Activate and
// the reducer's reduceReactionPicker branch accepts whatever event.Rune
// encodes (zero for the default cursor selection), keeping Selected pinned
// until the async ReactToMessage result lands.
func TestAppModelSelectorRouteReactionEnterResolvesPalette(t *testing.T) {
	state := selectorRouteReactionState()
	state.ReactionPicker.Selected = 2
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	// Negative contract: the source message is intentionally absent. Activate
	// must therefore close the picker synchronously and restore conversation
	// focus. If Enter mapping is removed, both values remain unchanged.
	if model.Snapshot().ReactionPicker != nil || model.Snapshot().Focus != FocusConversation {
		t.Fatalf("Enter did not dispatch reaction Activate: picker=%#v focus=%v", model.Snapshot().ReactionPicker, model.Snapshot().Focus)
	}
}

// TestAppModelSelectorRouteMenuEmptyHasZeroRows keeps the helper/reducer
// equivalence explicit: an empty menu has zero actionable rows and navigation
// is therefore a no-op.
func TestAppModelSelectorRouteMenuEmptyHasZeroRows(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusModal
	state.MessageMenu = &MessageActionMenu{
		RequestID: 5, ChatID: 9, MessageID: 1, Loading: false,
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := menuVisibleOptions(model.Snapshot().MessageMenu); got != 0 {
		t.Fatalf("empty menu visible options = %d, want 0", got)
	}

	// actionMenuItemCount returns 0, so the reducer does not modulo and leaves
	// Selected unchanged.
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := model.Snapshot().MessageMenu.Selected; got != 0 {
		t.Fatalf("empty menu j shifted Selected = %d, want 0", got)
	}
}
