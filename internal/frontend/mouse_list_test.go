package frontend

import (
	"image"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func clickListHit(t *testing.T, model AppModel, hit Hit) AppModel {
	t.Helper()
	model, _ = updateAppModel(t, model, tea.MouseClickMsg{X: hit.Rect.Min.X, Y: hit.Rect.Min.Y, Button: tea.MouseLeft})
	return model
}

func findListHit(t *testing.T, model AppModel, wanted ActionReceived) Hit {
	t.Helper()
	model.View()
	for _, hit := range model.hitRegions() {
		if hit.ListRow && hit.Click == wanted {
			return hit
		}
	}
	t.Fatalf("no list hit for %#v", wanted)
	return Hit{}
}

func TestMouseChatRowFocusThenOpen(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 1, Title: "First", Kind: domain.ChatPrivate})
	state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Second", Kind: domain.ChatPrivate})
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	wanted := ActionReceived{Action: FocusChat, ChatID: 2}
	hit := findListHit(t, model, wanted)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.FocusedChat != 1 || got.SelectedChat != 0 || got.Focus != FocusChats {
		t.Fatalf("single click changed selected conversation: focused=%d selected=%d focus=%v", got.FocusedChat, got.SelectedChat, got.Focus)
	}
	hit = findListHit(t, model, wanted)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.SelectedChat != 1 || got.FocusedChat != 1 {
		t.Fatalf("double click did not open chat 2: focused=%d selected=%d", got.FocusedChat, got.SelectedChat)
	}
}

func TestMouseActionModalFocusAndConfirm(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Group", Kind: domain.ChatSupergroup, IsMember: true})
	updateState(&state, ActionReceived{Action: OpenChatActionMenu})
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	hit := findListHit(t, model, ActionReceived{Action: ViewChatInfo, ChatID: 9})
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.ChatActions == nil || got.ChatActions.Selected != 1 || got.DetailsOpen {
		t.Fatalf("single click activated chat info instead of focusing: %#v", got.ChatActions)
	}
	hit = findListHit(t, model, hit.Click)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.ChatActions != nil || !got.DetailsOpen || got.Focus != FocusDetails {
		t.Fatalf("double click did not activate chat info: focus=%v menu=%#v", got.Focus, got.ChatActions)
	}
}

func TestMouseListDoubleClickRequiresSameRowAndNoInterveningInput(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 1, Title: "First", Kind: domain.ChatPrivate})
	state.Chats = append(state.Chats, domain.Chat{ID: 2, Title: "Second", Kind: domain.ChatPrivate})
	model := newAppModelForTest(t, state, newTestSession(t))
	model.setHitRegions(HitMap{
		{ID: "chat:1", Rect: image.Rect(0, 0, 1, 1), Click: ActionReceived{Action: FocusChat, ChatID: 1}, ListRow: true},
		{ID: "chat:2", Rect: image.Rect(1, 0, 2, 1), Click: ActionReceived{Action: FocusChat, ChatID: 2}, ListRow: true},
	})
	first := Hit{Rect: image.Rect(0, 0, 1, 1)}
	second := Hit{Rect: image.Rect(1, 0, 2, 1)}
	model = clickListHit(t, model, first)
	model = clickListHit(t, model, second)
	if model.Snapshot().SelectedChat != 0 {
		t.Fatal("two different rows opened a chat")
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	model = clickListHit(t, model, second)
	if model.Snapshot().SelectedChat != 0 {
		t.Fatal("keypress did not cancel pending click")
	}
	model.pendingListClick.at = time.Now().Add(-doubleClickInterval - time.Millisecond)
	model = clickListHit(t, model, second)
	if model.Snapshot().SelectedChat != 0 {
		t.Fatal("expired double click opened a chat")
	}
	model = clickListHit(t, model, second)
	if model.Snapshot().SelectedChat != 1 {
		t.Fatal("fresh double click did not open the focused chat")
	}
}

func TestMouseChatInfoActionFocusThenEnter(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Group", Kind: domain.ChatSupergroup})
	state.DetailsOpen = true
	state.DetailsChatID = 9
	state.Focus = FocusDetails
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 28})
	action := ActionReceived{Action: OpenMembers}
	hit := findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.DetailsSelected != 1 || got.Members != nil || got.Focus != FocusDetails {
		t.Fatalf("single click activated details action: selected=%d members=%#v", got.DetailsSelected, got.Members)
	}
	hit = findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.Members == nil || got.Focus != FocusMembers {
		t.Fatalf("double click did not open members: focus=%v members=%#v", got.Focus, got.Members)
	}
}

func TestMouseBotCommandFocusThenComplete(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Bot", Kind: domain.ChatPrivate, CanSend: true})
	state.Focus = FocusComposer
	state.CommandMenu = &CommandMenuState{ChatID: 9, Selected: 0, Candidates: []domain.BotCommand{{Name: "help"}, {Name: "start"}}}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	action := ActionReceived{Action: CommandMenuActivate, ChatID: 9, CommandIndex: 1}
	hit := findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.CommandMenu == nil || got.CommandMenu.Selected != 1 || got.Drafts[9] != "" {
		t.Fatalf("single click completed instead of focusing: menu=%#v draft=%q", got.CommandMenu, got.Drafts[9])
	}
	hit = findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.CommandMenu != nil || got.Drafts[9] != "/start " {
		t.Fatalf("double click did not complete command: menu=%#v draft=%q", got.CommandMenu, got.Drafts[9])
	}
}

func TestMouseSearchResultFocusThenJump(t *testing.T) {
	state := chatActionState(domain.Chat{ID: 9, Title: "Chat", Kind: domain.ChatPrivate})
	state.Focus = FocusSearchResults
	state.MessageSearch = &MessageSearchState{ChatID: 9, Submitted: true, Results: []domain.Message{{ID: 10, ChatID: 9, Text: "first"}, {ID: 11, ChatID: 9, Text: "second"}}}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	action := ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 11}
	hit := findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot().MessageSearch; got.Selected != 1 || got.JumpMessageID != 0 {
		t.Fatalf("single click jumped rather than focused: %#v", got)
	}
	hit = findListHit(t, model, action)
	model = clickListHit(t, model, hit)
	if got := model.Snapshot().MessageSearch; got.JumpMessageID != 11 {
		t.Fatalf("double click did not jump: %#v", got)
	}
}

func TestMouseSearchDuplicateChatRowsFocusPublicResult(t *testing.T) {
	chat := domain.Chat{ID: 9, Title: "Repeated", Kind: domain.ChatPrivate}
	state := chatActionState(chat)
	state.ChatSearch = &ChatSearchState{LocalChats: []domain.Chat{chat}, PublicChats: []domain.Chat{chat}, Submitted: true}
	state.Focus = FocusChatSearchInput
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	model.View()
	var hit Hit
	for _, row := range model.hitRegions() {
		if row.ID == "chat-search:pub:9" {
			hit = row
		}
	}
	if hit.Rect.Empty() || !hit.ListRow {
		t.Fatal("public result not a selectable list row")
	}
	model = clickListHit(t, model, hit)
	if got := model.Snapshot(); got.ChatSearch == nil || got.ChatSearch.Selected != 1 {
		t.Fatalf("public duplicate was not focused: %#v", got.ChatSearch)
	}
}
