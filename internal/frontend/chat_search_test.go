package frontend

import (
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func chatSearchViewModel() ViewModel {
	return ViewModel{
		Width: 100, Height: 30,
		Layout: ViewLayout{Mode: LayoutWide},
		Focus:  FocusChatSearchResults,
		ChatSearch: &ChatSearchState{
			RequestID:   7,
			Input:       []rune("@botfather"),
			Query:       "botfather",
			PublicChats: []domain.Chat{{ID: 99, Kind: domain.ChatPrivate, Title: "BotFather", Username: "BotFather"}},
			Selected:    0,
			Submitted:   true,
		},
	}
}

func TestChatSearchKeyMappings(t *testing.T) {
	slash := tea.Key{Text: "/", Code: '/'}
	if got, ok := mapKeyPress(FocusChats, tea.KeyPressMsg(slash)); !ok || got.Action != OpenChatSearch {
		t.Fatalf("chats / = (%#v,%t)", got, ok)
	}
	if got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(slash)); !ok || got.Action != OpenMessageSearch {
		t.Fatalf("conversation / must stay message search, got (%#v,%t)", got, ok)
	}
	cases := map[tea.Key]Action{
		{Code: tea.KeyDown}:    SelectNext,
		{Code: 'j', Text: "j"}: SelectNext,
		{Code: tea.KeyUp}:      SelectPrevious,
		{Code: 'k', Text: "k"}: SelectPrevious,
		{Code: tea.KeyEnter}:   Activate,
		{Code: tea.KeyEscape}:  Close,
		{Code: 'q', Text: "q"}: Close,
		{Code: '/', Text: "/"}: OpenChatSearch,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(FocusChatSearchResults, tea.KeyPressMsg(key))
		if !ok || got.Action != want {
			t.Fatalf("chat results key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
	if got, ok := mapKeyPress(FocusChatSearchInput, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); !ok || got.Action != Activate {
		t.Fatalf("chat input enter = (%#v,%t), want Activate", got, ok)
	}
	for _, key := range []tea.Key{{Code: tea.KeyDown}, {Code: tea.KeyUp}, {Code: tea.KeyEscape}} {
		want := SelectNext
		if key.Code == tea.KeyUp {
			want = SelectPrevious
		}
		if key.Code == tea.KeyEscape {
			want = Close
		}
		if got, ok := mapKeyPress(FocusChatSearchInput, tea.KeyPressMsg(key)); !ok || got.Action != want {
			t.Fatalf("chat input key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
	if !editableFocus(FocusChatSearchInput) {
		t.Fatal("chat search input must be editable")
	}
}

// TestChatSearchRowsCarryChatIdentity documents the manual chat-search row
// source: section headers and the informational empty/loading/error row stay
// outside the actionable subset. ChatSearch never consumes the shared list
// modal controller (the unified layer deliberately keeps its own sectioned
// rows), so these rows are the renderer's source of truth.
func TestChatSearchRowsCarryChatIdentity(t *testing.T) {
	model := chatSearchViewModel()
	options := selectorOptionsFromRows(buildUnifiedChatSearchRows(model, time.UTC))
	if len(options) != 1 || options[0].ID != "chat-search:pub:99" {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Value != (ActionReceived{Action: SelectChat, ChatID: 99}) {
		t.Fatalf("option payload = %#v", options[0].Value)
	}
	if got := selectorOptionsFromRows(buildUnifiedChatSearchRows(ViewModel{}, time.UTC)); got != nil {
		t.Fatalf("nil search options = %#v", got)
	}
	zero := ViewModel{ChatSearch: &ChatSearchState{PublicChats: []domain.Chat{{ID: 0}}}}
	if got := selectorOptionsFromRows(buildUnifiedChatSearchRows(zero, time.UTC)); len(got) != 0 {
		t.Fatalf("zero-ID result must not become an option: %#v", got)
	}
	// Unified order is local chats, global messages, public chats; message
	// options carry both chat and message identity.
	unified := &ChatSearchState{
		LocalChats:     []domain.Chat{{ID: 7, Title: "Local"}},
		GlobalMessages: []domain.Message{{ID: 5, ChatID: 7, Kind: domain.MessageText, Text: "hi"}},
		PublicChats:    []domain.Chat{{ID: 99, Title: "Pub"}},
	}
	got := selectorOptionsFromRows(buildUnifiedChatSearchRows(ViewModel{ChatSearch: unified}, time.UTC))
	if len(got) != 3 || got[0].ID != "chat-search:local:7" || got[1].ID != "chat-search:msg:7:5" || got[2].ID != "chat-search:pub:99" {
		t.Fatalf("unified options = %#v", got)
	}
	if got[1].Value != (ActionReceived{Action: SelectMessage, ChatID: 7, MessageID: 5}) {
		t.Fatalf("message option payload = %#v", got[1].Value)
	}
}

func TestChatSearchResultLabelSanitizes(t *testing.T) {
	label := chatSearchResultLabel(domain.Chat{Title: " Bot\x00Father ", Username: "@BotFather"})
	if strings.Contains(label, "\x00") || !strings.Contains(label, "Bot") || !strings.Contains(label, "@BotFather") {
		t.Fatalf("label = %q", label)
	}
	if got := chatSearchResultLabel(domain.Chat{Title: "", Username: ""}); got != "Unknown" {
		t.Fatalf("empty label = %q", got)
	}
	if got := chatSearchResultLabel(domain.Chat{Title: "Saved", Username: ""}); got != "Saved" {
		t.Fatalf("username-less label = %q", got)
	}
}

func TestChatSearchLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := chatSearchViewModel()
	layer := buildChatSearchLayer(model, time.UTC, styles, "", "")
	if layer.Layer == nil || !layer.IsModal || layer.Rect.Empty() {
		t.Fatalf("layer = nil=%t modal=%t rect=%v", layer.Layer == nil, layer.IsModal, layer.Rect)
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if wantWidth := chatSearchFrame(bounds).Dx(); layer.Rect.Dx() != wantWidth {
		t.Fatalf("results width = %d, want %d", layer.Rect.Dx(), wantWidth)
	}
	for _, interaction := range layer.Interactions {
		if strings.HasPrefix(interaction.ID, "conversation:") || strings.HasPrefix(interaction.ID, "chat:") {
			t.Fatalf("underlying hit leaked: %q", interaction.ID)
		}
	}
	// Loading state.
	loading := chatSearchViewModel()
	loading.ChatSearch.PublicChats = nil
	loading.ChatSearch.PublicLoading = true
	rows := buildUnifiedChatSearchRows(loading, time.UTC)
	if len(rows) != 1 || rows[0].Label != "Searching..." {
		t.Fatalf("loading rows = %#v", rows)
	}
	// Empty state.
	emptyModel := chatSearchViewModel()
	emptyModel.ChatSearch.PublicChats = nil
	rows = buildUnifiedChatSearchRows(emptyModel, time.UTC)
	if len(rows) != 1 || rows[0].Label != "No results" {
		t.Fatalf("empty rows = %#v", rows)
	}
	// Failed state.
	failed := chatSearchViewModel()
	failed.ChatSearch.PublicChats = nil
	failed.ChatSearch.PublicError = &domain.AppError{Message: "Could not find public chat"}
	rows = buildUnifiedChatSearchRows(failed, time.UTC)
	if len(rows) != 1 || rows[0].Label != "Could not find public chat" {
		t.Fatalf("error rows = %#v", rows)
	}
	rows = buildUnifiedChatSearchRows(model, time.UTC)
	if len(rows) != 2 || !rows[0].Header || !strings.Contains(rows[0].Label, "Public chats") {
		t.Fatalf("public header = %#v", rows)
	}
	if rows[1].Action != (ActionReceived{Action: SelectChat, ChatID: 99}) {
		t.Fatalf("row action = %#v", rows[1].Action)
	}
}

func TestChatSearchInputGeometryIsBounded(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 30)
	frame := chatSearchFrame(bounds)
	if frame.Empty() || frame.Dx() > 72 {
		t.Fatalf("frame = %v", frame)
	}
	input := chatSearchInputRect(bounds)
	if input.Empty() || !input.In(frame) {
		t.Fatalf("input %v not in frame %v", input, frame)
	}
}

func TestChatSearchViewModelClonesWithoutAliasing(t *testing.T) {
	state := InitialState()
	state.Focus = FocusChatSearchResults
	state.ChatSearch = &ChatSearchState{Input: []rune("ab"), PublicChats: []domain.Chat{{ID: 99, Title: "Bot"}}}
	model := Select(state, time.UTC)
	if model.ChatSearch == nil || string(model.ChatSearch.Input) != "ab" || len(model.ChatSearch.PublicChats) != 1 {
		t.Fatalf("projection = %#v", model.ChatSearch)
	}
	model.ChatSearch.Input[0] = 'X'
	model.ChatSearch.PublicChats[0].Title = "mutated"
	if string(state.ChatSearch.Input) != "ab" || state.ChatSearch.PublicChats[0].Title != "Bot" {
		t.Fatal("viewmodel aliased chat search state")
	}
}

func TestChatSearchRowsOrderLocalMessagesPublic(t *testing.T) {
	model := ViewModel{
		Width: 100, Height: 30,
		Layout: ViewLayout{Mode: LayoutWide},
		Focus:  FocusChatSearchInput,
		ChatSearch: &ChatSearchState{
			Input:          []rune("x"),
			Query:          "x",
			Submitted:      true,
			LocalChats:     []domain.Chat{{ID: 7, Title: "Local"}},
			GlobalMessages: []domain.Message{{ID: 5, ChatID: 7, Kind: domain.MessageText, Text: "hit"}},
			PublicChats:    []domain.Chat{{ID: 99, Title: "Pub"}},
		},
	}
	rows := buildUnifiedChatSearchRows(model, time.UTC)
	if len(rows) != 6 {
		t.Fatalf("rows = %#v", rows)
	}
	for index, want := range []string{"Chats", "Messages", "Public chats"} {
		header := rows[index*2]
		if !header.Header || !strings.Contains(header.Label, want) || header.Action.Action != NoAction {
			t.Fatalf("header %d = %#v, want %q", index, header, want)
		}
	}
	if !strings.HasPrefix(rows[1].ID, "chat-search:local:") || rows[1].Action != (ActionReceived{Action: SelectChat, ChatID: 7}) {
		t.Fatalf("local row = %#v", rows[1])
	}
	if !strings.HasPrefix(rows[3].ID, "chat-search:msg:") || rows[3].Action != (ActionReceived{Action: SelectMessage, ChatID: 7, MessageID: 5}) {
		t.Fatalf("message row = %#v", rows[3])
	}
	if !strings.HasPrefix(rows[5].ID, "chat-search:pub:") || rows[5].Action != (ActionReceived{Action: SelectChat, ChatID: 99}) {
		t.Fatalf("public row = %#v", rows[5])
	}
	// Unified layer renders the same rows while the input stays focused.
	layer := buildChatSearchLayer(model, time.UTC, newRenderStyles(true), "", "")
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("live unified layer must render while typing")
	}
	// The input echo and section headers live inside the modal, in both
	// results and input focus.
	for _, focus := range []Focus{FocusChatSearchResults, FocusChatSearchInput} {
		inputModel := model
		inputModel.Focus = focus
		frame := composeApplication(inputModel, time.UTC)
		plain := ansi.Strip(frame.Content)
		if !strings.Contains(plain, "> x") || !strings.Contains(plain, "Chats") ||
			!strings.Contains(plain, "Messages") || !strings.Contains(plain, "Public chats") {
			t.Fatalf("focus %v modal missing input/headers", focus)
		}
	}
	_ = layer
}

func TestChatSearchUnifiedLayerIgnoresSelectorOverlay(t *testing.T) {
	model := chatSearchViewModel()
	// Even with an injected Huh selector view, manual sectioned rows stay the
	// source of truth so labels cannot slide under the wrong headers.
	layer := buildChatSearchLayer(model, time.UTC, newRenderStyles(true), "", "ROUTED_SELECTOR")
	if layer.Layer == nil || !layer.IsModal {
		t.Fatal("unified layer must render with selector view present")
	}
	rows := buildUnifiedChatSearchRows(model, time.UTC)
	if len(rows) != 2 || !rows[0].Header {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestWindowUnifiedChatSearchRowsKeepsHeaderAndSelection(t *testing.T) {
	var chats []domain.Chat
	for id := int64(1); id <= 5; id++ {
		chats = append(chats, domain.Chat{ID: domain.ChatID(id), Title: "Local"})
	}
	var msgs []domain.Message
	for id := int64(1); id <= 5; id++ {
		msgs = append(msgs, domain.Message{ID: domain.MessageID(id), ChatID: 9, Kind: domain.MessageText, Text: "hit"})
	}
	model := ViewModel{
		ChatSearch: &ChatSearchState{
			Input:          []rune("x"),
			Query:          "x",
			Submitted:      true,
			LocalChats:     chats,
			GlobalMessages: msgs,
			Selected:       7,
		},
	}
	rows := buildUnifiedChatSearchRows(model, time.UTC)
	windowed := windowUnifiedChatSearchRows(rows, model.ChatSearch.Selected, 6)
	if len(windowed) > 6 {
		t.Fatalf("windowed len = %d: %#v", len(windowed), windowed)
	}
	// Selected actionable row (global index 7) must stay visible.
	seen := false
	for _, row := range windowed {
		if row.Selected {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("selected row cut: %#v", windowed)
	}
	// No message item may appear without its Messages header above it, and
	// the window must not end on a lone header.
	sawMessagesHeader := false
	for _, row := range windowed {
		if row.Header && strings.Contains(row.Label, "Messages") {
			sawMessagesHeader = true
		}
		if !row.Header && strings.HasPrefix(row.ID, "chat-search:msg:") && !sawMessagesHeader {
			t.Fatalf("orphaned message row: %#v", windowed)
		}
	}
	if windowed[len(windowed)-1].Header {
		t.Fatalf("trailing lone header: %#v", windowed)
	}
}
