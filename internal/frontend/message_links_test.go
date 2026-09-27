package frontend

import (
	"fmt"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/tuilegram/internal/domain"
	"github.com/zylen-det/tuilegram/internal/telegram"
)

func TestMessageLinksUseTDLibEntitiesAndOriginalText(t *testing.T) {
	text := "😀go www.one.example and mail"
	message := domain.Message{Text: text, Entities: []domain.TextEntity{
		{Offset: 2, Length: 2, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://hidden.example/path"},
		{Offset: 5, Length: len("www.one.example"), Kind: domain.EntityLink, Link: domain.LinkPlainURL},
		{Offset: 25, Length: 4, Kind: domain.EntityLink},                           // email is styled as a link but is not a TDLib URL entity
		{Offset: 1, Length: 1, Kind: domain.EntityLink, Link: domain.LinkPlainURL}, // invalid UTF-16 boundary
	}}
	want := []messageLink{
		{URL: "https://hidden.example/path", Label: "https://hidden.example/path(go)"},
		{URL: "www.one.example", Label: "www.one.example"},
	}
	if got := messageLinks(message); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %#v, want %#v", got, want)
	}
}

func linkMenuState() State {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 2, ChatID: 9, Kind: domain.MessagePhoto, Text: "first second",
		Entities: []domain.TextEntity{
			{Offset: 0, Length: 5, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://one.example"},
			{Offset: 6, Length: 6, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://two.example"},
		},
	}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	return state
}

func TestMessageLinkMenuNavigationCopyOpenAndBack(t *testing.T) {
	state := linkMenuState()
	if state.MessageMenu == nil || len(state.MessageMenu.Links) != 2 {
		t.Fatalf("link menu = %#v", state.MessageMenu)
	}
	for _, action := range []Action{CopyMessageLink, OpenMessageLink} {
		found := false
		for index, row := range messageActionRows(state.MessageMenu) {
			if row.Action.Action != action {
				continue
			}
			found = true
			selectMessageMenuAction(state.MessageMenu, action)
			if state.MessageMenu.Selected != index || selectedMenuAction(state.MessageMenu) != action {
				t.Fatalf("selected %v at %d, want %d", action, state.MessageMenu.Selected, index)
			}
		}
		if !found {
			t.Fatalf("missing action %v", action)
		}
	}
	key, consumed := mapActionModalKey(state, tea.KeyPressMsg{Code: 'o', Text: "o"})
	if !consumed || key.Action != OpenMessageLink {
		t.Fatalf("open shortcut = %#v / %t", key, consumed)
	}
	updateState(&state, MessagePropertiesLoaded{RequestID: state.MessageMenu.RequestID, ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true, Forward: true}})
	if selectedMenuAction(state.MessageMenu) != OpenMessageLink {
		t.Fatalf("loading completion moved selection to %v", selectedMenuAction(state.MessageMenu))
	}
	updateState(&state, ActionReceived{Action: Activate})
	if state.MessageMenu.LinkAction != OpenMessageLink || state.Focus != FocusModal {
		t.Fatalf("submenu = %#v", state.MessageMenu)
	}
	rows := messageLinkRows(state.MessageMenu)
	if len(rows) != 2 || rows[1].Label != "https://two.example(second)" || rows[1].Action.CommandIndex != 1 {
		t.Fatalf("link rows = %#v", rows)
	}
	updateState(&state, ActionReceived{Action: SelectPrevious})
	if state.MessageMenu.LinkSelected != 1 {
		t.Fatalf("wrap previous = %d", state.MessageMenu.LinkSelected)
	}
	updateState(&state, ActionReceived{Action: Close})
	if state.MessageMenu == nil || state.MessageMenu.LinkAction != NoAction || selectedMenuAction(state.MessageMenu) != OpenMessageLink {
		t.Fatalf("back did not restore parent selection: %#v", state.MessageMenu)
	}
	updateState(&state, ActionReceived{Action: CopyMessageLink})
	// Mouse clicks carry the specific link index; a stale message identity is ignored.
	if effects := updateState(&state, ActionReceived{Action: SelectMessageLink, ChatID: 10, MessageID: 2, CommandIndex: 1}); len(effects) != 0 {
		t.Fatalf("stale link produced effects: %#v", effects)
	}
	effects := updateState(&state, rows[1].Action)
	if !reflect.DeepEqual(effects, []Effect{WriteClipboard{Text: "https://two.example", Label: "Link copied"}}) || state.MessageMenu != nil || state.Focus != FocusConversation {
		t.Fatalf("copy result = %#v, menu=%#v, focus=%v", effects, state.MessageMenu, state.Focus)
	}
	state = linkMenuState()
	updateState(&state, ActionReceived{Action: OpenMessageLink})
	updateState(&state, ActionReceived{Action: SelectNext})
	effects = updateState(&state, ActionReceived{Action: Activate})
	if !reflect.DeepEqual(effects, []Effect{OpenWebLink{URL: "https://two.example"}}) || state.MessageMenu != nil {
		t.Fatalf("open result = %#v, menu=%#v", effects, state.MessageMenu)
	}
}

func TestMessagePlainURLMenuCopiesTaggedTextUnchanged(t *testing.T) {
	state := linkMenuState()
	state.Messages[9][0].Text = "example.com"
	state.Messages[9][0].Entities = []domain.TextEntity{{Offset: 0, Length: len("example.com"), Kind: domain.EntityLink, Link: domain.LinkPlainURL}}
	state.MessageMenu = nil
	state.Focus = FocusConversation
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	updateState(&state, ActionReceived{Action: CopyMessageLink})
	if got := messageLinkRows(state.MessageMenu)[0].Label; got != "example.com" {
		t.Fatalf("plain link label = %q", got)
	}
	effects := updateState(&state, ActionReceived{Action: Activate})
	if !reflect.DeepEqual(effects, []Effect{WriteClipboard{Text: "example.com", Label: "Link copied"}}) {
		t.Fatalf("plain link copy = %#v", effects)
	}
}

func TestMessageLinkMenuRefreshesOnEdit(t *testing.T) {
	state := linkMenuState()
	updateState(&state, ActionReceived{Action: OpenMessageLink})
	updateState(&state, ActionReceived{Action: SelectNext})
	updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{
		ChatID: 9, MessageID: 2, Kind: domain.MessagePhoto, Text: "first second",
		Entities: []domain.TextEntity{{Offset: 0, Length: 5, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://edited.example"}, {Offset: 6, Length: 6, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://two.example"}},
	}})
	if state.MessageMenu == nil || state.MessageMenu.LinkAction != NoAction || !reflect.DeepEqual(state.MessageMenu.Links, []messageLink{{URL: "https://edited.example", Label: "https://edited.example(first)"}, {URL: "https://two.example", Label: "https://two.example(second)"}}) {
		t.Fatalf("edited menu = %#v", state.MessageMenu)
	}
	updateState(&state, ActionReceived{Action: OpenMessageLink})
	if effects := updateState(&state, ActionReceived{Action: SelectMessageLink, ChatID: 9, MessageID: 2, CommandIndex: 0}); !reflect.DeepEqual(effects, []Effect{OpenWebLink{URL: "https://edited.example"}}) {
		t.Fatalf("edited target = %#v", effects)
	}
}

func TestMessageLinkMenuOnlyForLinksAndVisibleSelection(t *testing.T) {
	state := linkMenuState()
	state.MessageMenu.Links = nil
	for _, row := range messageActionRows(state.MessageMenu) {
		if row.Action.Action == CopyMessageLink || row.Action.Action == OpenMessageLink {
			t.Fatalf("empty message has link action: %#v", row)
		}
	}
	if effects := updateState(&state, ActionReceived{Action: CopyMessageLink}); len(effects) != 0 || state.MessageMenu.LinkAction != NoAction {
		t.Fatalf("empty link action changed state: %#v", state.MessageMenu)
	}
	state = linkMenuState()
	state.MessageMenu.Links = nil
	for i := 0; i < 40; i++ {
		url := fmt.Sprintf("https://example.org/%d", i)
		state.MessageMenu.Links = append(state.MessageMenu.Links, messageLink{URL: url, Label: url})
	}
	state.MessageMenu.LinkAction = CopyMessageLink
	state.MessageMenu.LinkSelected = 35
	surface := buildActionModalLayer(ViewModel{Width: 80, Height: 18, MessageMenu: state.MessageMenu}, newRenderStyles(false))
	selected := fmt.Sprintf("message-link:%d", state.MessageMenu.LinkSelected)
	found := false
	for _, hit := range surface.Interactions {
		if hit.ID == selected {
			found = true
		}
	}
	if !found {
		t.Fatal("selected link scrolled outside clickable rows")
	}
	key, consumed := mapActionModalKey(state, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !consumed || key.Action != Close {
		t.Fatalf("submenu back key = %#v / %v", key, consumed)
	}
}
