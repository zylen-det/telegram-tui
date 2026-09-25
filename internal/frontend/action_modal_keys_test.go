package frontend

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestActionModalMnemonicContract(t *testing.T) {
	for _, tc := range []struct {
		action Action
		key    string
	}{
		{ViewMessageMedia, "v"}, {ReplyMessage, "r"}, {GoToReferencedMessage, "g"},
		{ForwardMessageSource, "f"}, {EditMessage, "e"}, {CopyMessage, "y"},
		{ViewUserInfo, "i"}, {ReactMessage, "a"}, {PinMessage, "p"},
		{DeleteMessage, "d"}, {DeleteForEveryone, "D"},
		{OpenChat, "o"}, {ViewChatInfo, "i"}, {ArchiveChat, "a"},
		{PinChat, "p"}, {MuteChat, "m"}, {MarkChatRead, "r"},
		{ClearChatHistory, "c"}, {DeleteConversation, "d"},
		{LeaveChat, "l"}, {JoinChat, "J"},
		{CancelChatAction, "c"}, {ConfirmChatAction, "y"},
	} {
		if got := actionModalShortcut(tc.action); got != tc.key {
			t.Errorf("action %v shortcut = %q, want %q", tc.action, got, tc.key)
		}
	}
}

func TestActionModalKeysMatchVisibleRows(t *testing.T) {
	message := &MessageActionMenu{
		ChatID: 9, MessageID: 2, ReferencedMessageID: 1, UserID: 3, CanReact: true,
		MediaFile:    domain.MediaFileRef{ID: 1, CanDownload: true},
		Capabilities: domain.MessageCapabilities{Reply: true, Forward: true, Edit: true, Copy: true, Pin: true, DeleteForSelf: true, DeleteForAll: true},
	}
	chat := domain.Chat{ID: 9, Kind: domain.ChatPrivate, CanDeleteForSelf: true}
	group := domain.Chat{ID: 9, Kind: domain.ChatSupergroup, IsMember: false, CanDeleteForAll: true}
	for _, tc := range []struct {
		name  string
		state State
		rows  []modalRowSpec
	}{
		{"message", State{Focus: FocusModal, MessageMenu: message}, messageActionRows(message)},
		{"loading message", State{Focus: FocusModal, MessageMenu: &MessageActionMenu{ChatID: 9, MessageID: 2, Loading: true, Capabilities: message.Capabilities}}, messageActionRows(&MessageActionMenu{ChatID: 9, MessageID: 2, Loading: true, Capabilities: message.Capabilities})},
		{"chat", State{Focus: FocusChatActions, ChatActions: &ChatActionMenuState{ChatID: 9}, Chats: []domain.Chat{chat}}, chatActionRows(chat, &ChatActionMenuState{ChatID: 9})},
		{"group", State{Focus: FocusChatActions, ChatActions: &ChatActionMenuState{ChatID: 9}, Chats: []domain.Chat{group}}, chatActionRows(group, &ChatActionMenuState{ChatID: 9})},
		{"confirm", State{Focus: FocusChatActions, ChatActions: &ChatActionMenuState{ChatID: 9, Confirming: DeleteConversation}, Chats: []domain.Chat{chat}}, chatActionRows(chat, &ChatActionMenuState{ChatID: 9, Confirming: DeleteConversation})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(map[string]bool)
			for _, row := range tc.rows {
				if !rowSelectable(row) {
					if row.Key != "" {
						t.Errorf("informational row %q has key %q", row.Label, row.Key)
					}
					continue
				}
				if row.Key == "" || seen[row.Key] {
					t.Fatalf("missing/duplicate shortcut for %q: %q", row.Label, row.Key)
				}
				seen[row.Key] = true
				got, consumed := mapActionModalKey(tc.state, tea.KeyPressMsg(tea.Key{Text: row.Key, Code: []rune(row.Key)[0]}))
				if !consumed || got != row.Action {
					t.Errorf("%q: got (%+v, %v), want %+v", row.Key, got, consumed, row.Action)
				}
			}
			unbound := []tea.Key{{Text: "h", Code: 'h'}, {Text: "i", Code: 'i', Mod: tea.ModCtrl}, {Code: tea.KeyTab}}
			if tc.name == "loading message" {
				unbound = append(unbound, tea.Key{Text: "f", Code: 'f'})
			}
			for _, key := range unbound {
				got, consumed := mapActionModalKey(tc.state, tea.KeyPressMsg(key))
				if !consumed || got.Action != NoAction {
					t.Errorf("unbound %v escaped modal: %+v, %v", key, got, consumed)
				}
			}
			for _, keyCase := range []struct {
				key  tea.Key
				want Action
			}{
				{tea.Key{Text: "j", Code: 'j'}, SelectNext},
				{tea.Key{Text: "k", Code: 'k'}, SelectPrevious},
				{tea.Key{Text: "q", Code: 'q'}, Close},
				{tea.Key{Code: tea.KeyEscape}, Close},
				{tea.Key{Code: tea.KeyEnter}, Activate},
				{tea.Key{Code: 'c', Text: "c", Mod: tea.ModCtrl}, Quit},
			} {
				got, consumed := mapActionModalKey(tc.state, tea.KeyPressMsg(keyCase.key))
				if !consumed || got.Action != keyCase.want {
					t.Errorf("%v = %+v, %v; want %v", keyCase.key, got, consumed, keyCase.want)
				}
			}
		})
	}
}

func TestActionModalKeyRoutingAndConfirmation(t *testing.T) {
	state := modalRouteMenuState()
	state.MessageMenu.Capabilities = domain.MessageCapabilities{Reply: true, Forward: true}
	state.Messages = map[domain.ChatID][]domain.Message{9: {{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "hi"}}}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "h", Code: 'h'}))
	if got := model.Snapshot(); got.Focus != FocusModal || got.MessageMenu == nil {
		t.Fatalf("unbound h left modal: focus=%v menu=%v", got.Focus, got.MessageMenu)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "f", Code: 'f'}))
	if got := model.Snapshot(); got.ForwardPicker == nil || got.MessageMenu != nil {
		t.Fatalf("forward shortcut did not activate: %+v", got.ForwardPicker)
	}

	chat := domain.Chat{ID: 9, Kind: domain.ChatPrivate, CanDeleteForSelf: true}
	chatState := InitialState()
	chatState.Chats = []domain.Chat{chat}
	chatState.Focus = FocusChatActions
	chatState.ChatActions = &ChatActionMenuState{ChatID: 9, PreviousFocus: FocusChats}
	chatModel := newAppModelForTest(t, chatState, newTestSession(t))
	chatModel, _ = updateAppModel(t, chatModel, tea.WindowSizeMsg{Width: 80, Height: 24})
	chatModel, _ = updateAppModel(t, chatModel, tea.KeyPressMsg(tea.Key{Text: "d", Code: 'd'}))
	if got := chatModel.Snapshot().ChatActions; got == nil || got.Confirming != DeleteConversation {
		t.Fatalf("delete shortcut did not enter confirmation: %+v", got)
	}
	chatModel, _ = updateAppModel(t, chatModel, tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
	if got := chatModel.Snapshot().ChatActions; got == nil || got.Confirming != NoAction {
		t.Fatalf("cancel shortcut did not return to action list: %+v", got)
	}
}

func TestChatActionShortcutPaintAndWidth(t *testing.T) {
	chat := domain.Chat{ID: 9, Kind: domain.ChatPrivate, CanDeleteForSelf: true}
	model := ViewModel{Width: 80, Height: 24, Focus: FocusChatActions, ActiveChat: chat, ChatActions: &ChatActionMenuState{ChatID: 9, Confirming: DeleteConversation}}
	surface := buildChatActionLayer(model, newRenderStyles(false))
	if surface.Rect.Dx() != 54 {
		t.Fatalf("chat modal width = %d", surface.Rect.Dx())
	}
	_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
	for _, hit := range surface.Interactions[1:] {
		want := "c"
		if hit.Click.Action == ConfirmChatAction {
			want = "y"
		}
		if cell := canvas.CellAt(hit.Rect.Max.X-1, hit.Rect.Min.Y); cell == nil || cell.Content != want || colorOf(cell.Style.Fg) != rgba(mutedTextColor) {
			t.Errorf("%s shortcut cell = %+v, want %s in secondary color", hit.ID, cell, want)
		}
	}
}

func TestActionModalShortcutPaintMatchesHitRows(t *testing.T) {
	menu := &MessageActionMenu{ChatID: 9, MessageID: 2, ReferencedMessageID: 1, Capabilities: domain.MessageCapabilities{Reply: true, Copy: true}, Selected: 1}
	model := ViewModel{Width: 80, Height: 24, Focus: FocusModal, MessageMenu: menu}
	surface := buildActionModalLayer(model, newRenderStyles(false))
	_, canvas := listModalCanvas(image.Rect(0, 0, 80, 24), surface)
	for _, hit := range surface.Interactions[1:] {
		var row modalRowSpec
		for _, candidate := range messageActionRows(menu) {
			if candidate.ID == hit.ID {
				row = candidate
			}
		}
		if got := canvas.CellAt(hit.Rect.Max.X-1, hit.Rect.Min.Y); got == nil || got.Content != row.Key || colorOf(got.Style.Fg) != rgba(mutedTextColor) {
			t.Errorf("%s key at right edge: cell=%+v, want %q in secondary color", hit.ID, got, row.Key)
		} else if row.Selected && colorOf(got.Style.Bg) != rgba(selectedColor) {
			t.Errorf("selected shortcut lost row background: %+v", got.Style)
		}
	}
	model.Width = 26
	surface = buildActionModalLayer(model, newRenderStyles(false))
	_, canvas = listModalCanvas(image.Rect(0, 0, 26, 24), surface)
	if surface.Rect.Dx() != 26 {
		t.Fatalf("clipped modal escaped viewport: %v", surface.Rect)
	}
	for _, hit := range surface.Interactions[1:] {
		for _, row := range messageActionRows(menu) {
			if hit.ID == row.ID {
				if cell := canvas.CellAt(hit.Rect.Max.X-1, hit.Rect.Min.Y); cell == nil || cell.Content != row.Key {
					t.Errorf("narrow shortcut %s = %+v, want %s", hit.ID, cell, row.Key)
				}
			}
		}
	}
}
