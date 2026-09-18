package frontend

import (
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func searchViewModel() ui.ViewModel {
	sentAt := time.Unix(1700000000, 0).UTC()
	return ui.ViewModel{
		Width: 100, Height: 30,
		Layout: ui.Layout{Mode: app.LayoutWide},
		Focus:  app.FocusSearchResults,
		MessageSearch: &app.MessageSearchState{
			RequestID: 10, ChatID: 9,
			Input: []rune("needle"),
			Query: "needle",
			Results: []domain.Message{
				{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "new hit", SenderName: "Ada", SentAt: sentAt},
				{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old hit", SenderName: "Bob", SentAt: sentAt},
			},
			Selected:  0,
			Submitted: true,
		},
	}
}

func TestMessageSearchKeyMappings(t *testing.T) {
	slash := tea.Key{Text: "/", Code: '/'}
	if got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(slash)); !ok || got.Action != app.OpenMessageSearch {
		t.Fatalf("conversation / = (%#v,%t)", got, ok)
	}
	cases := map[tea.Key]app.Action{
		{Code: tea.KeyDown}:    app.SelectNext,
		{Code: 'j', Text: "j"}: app.SelectNext,
		{Code: tea.KeyUp}:      app.SelectPrevious,
		{Code: 'k', Text: "k"}: app.SelectPrevious,
		{Code: tea.KeyEnter}:   app.Activate,
		{Code: tea.KeyEscape}:  app.Close,
		{Code: 'q', Text: "q"}: app.Close,
		{Code: '/', Text: "/"}: app.OpenMessageSearch,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(app.FocusSearchResults, tea.KeyPressMsg(key))
		if !ok || got.Action != want {
			t.Fatalf("search results key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
	if got, ok := mapKeyPress(app.FocusSearchInput, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); !ok || got.Action != app.SubmitMessageSearch {
		t.Fatalf("search input enter = (%#v,%t)", got, ok)
	}
}

func TestMessageSearchSelectorOptionsCarryChatMessageIdentity(t *testing.T) {
	model := searchViewModel()
	options := selectorOptionsFromRows(messageSearchRows(model, time.UTC))
	if len(options) != 2 || options[0].ID != "search:30" || options[1].ID != "search:20" {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Value != (app.ActionReceived{Action: app.SelectMessage, ChatID: 9, MessageID: 30}) {
		t.Fatalf("option payload = %#v", options[0].Value)
	}
	if got := selectorOptionsFromRows(messageSearchRows(ui.ViewModel{}, time.UTC)); got != nil {
		t.Fatalf("nil search options = %#v", got)
	}
}

func TestMessageSearchResultLabelSanitizesAndBounds(t *testing.T) {
	label := messageSearchResultLabel(" Ada\x00\x01 ", time.Unix(1700000000, 0).UTC(), "hello\x00world", time.UTC)
	if strings.Contains(label, "\x00") || !strings.Contains(label, "Ada") || !strings.Contains(label, "hello") {
		t.Fatalf("label = %q", label)
	}
	empty := messageSearchResultLabel("", time.Unix(0, 0).UTC(), "", time.UTC)
	if !strings.Contains(empty, "Unknown") || !strings.Contains(empty, "[Unsupported]") {
		t.Fatalf("empty label = %q", empty)
	}
}

func TestMessageSearchLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := searchViewModel()
	layer := buildMessageSearchLayer(model, time.UTC, styles, "", "")
	if layer.Layer == nil || !layer.IsModal || layer.Rect.Empty() {
		t.Fatalf("layer = nil=%t modal=%t rect=%v", layer.Layer == nil, layer.IsModal, layer.Rect)
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	wantWidth := messageSearchFrame(bounds).Dx()
	if layer.Rect.Dx() != wantWidth {
		t.Fatalf("results width = %d, want search frame width %d", layer.Rect.Dx(), wantWidth)
	}
	selectorRect := selectorHostRectWidth(bounds, len(model.MessageSearch.Results), len(model.MessageSearch.Results), wantWidth)
	if selectorRect.Dx() != layer.Rect.Dx()-4 {
		t.Fatalf("selector width = %d, want results content width %d", selectorRect.Dx(), layer.Rect.Dx()-4)
	}
	for _, interaction := range layer.Interactions {
		if strings.HasPrefix(interaction.ID, "conversation:") || strings.HasPrefix(interaction.ID, "chat:") {
			t.Fatalf("underlying hit leaked: %q", interaction.ID)
		}
	}
	// Loading / empty / error rows are informational, not dead ends.
	loading := searchViewModel()
	loading.MessageSearch.Results = nil
	loading.MessageSearch.Loading = true
	if rows := messageSearchRows(loading, time.UTC); len(rows) != 1 || rows[0].Label != "Searching..." {
		t.Fatalf("loading rows = %#v", rows)
	}
	emptyModel := searchViewModel()
	emptyModel.MessageSearch.Results = nil
	if rows := messageSearchRows(emptyModel, time.UTC); len(rows) != 1 || rows[0].Label != "No messages found" {
		t.Fatalf("empty rows = %#v", rows)
	}
	failed := searchViewModel()
	failed.MessageSearch.Results = nil
	failed.MessageSearch.Error = &domain.AppError{Message: "Could not search messages"}
	if rows := messageSearchRows(failed, time.UTC); len(rows) != 1 || rows[0].Label != "Could not search messages" {
		t.Fatalf("error rows = %#v", rows)
	}
	// Result rows preserve mouse parity payloads.
	rows := messageSearchRows(model, time.UTC)
	if rows[0].Action != (app.ActionReceived{Action: app.SelectMessage, ChatID: 9, MessageID: 30}) {
		t.Fatalf("row action = %#v", rows[0].Action)
	}
}

func TestMessageSearchInputGeometryIsBounded(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 30)
	frame := messageSearchFrame(bounds)
	if frame.Empty() || frame.Dx() > 64 {
		t.Fatalf("frame = %v", frame)
	}
	input := messageSearchInputRect(bounds)
	if input.Empty() || !input.In(frame) {
		t.Fatalf("input %v not in frame %v", input, frame)
	}
	if got := messageSearchFrame(image.Rectangle{}); !got.Empty() {
		t.Fatalf("empty bounds frame = %v", got)
	}
}

func TestMessageSearchViewModelClonesWithoutAliasing(t *testing.T) {
	state := app.InitialState()
	state.Focus = app.FocusSearchResults
	state.MessageSearch = &app.MessageSearchState{
		ChatID: 9, Input: []rune("ab"),
		Results: []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "x"}},
	}
	model := ui.Select(state, time.UTC)
	if model.MessageSearch == nil || string(model.MessageSearch.Input) != "ab" || len(model.MessageSearch.Results) != 1 {
		t.Fatalf("projection = %#v", model.MessageSearch)
	}
	model.MessageSearch.Input[0] = 'X'
	model.MessageSearch.Results[0].Text = "mutated"
	if string(state.MessageSearch.Input) != "ab" || state.MessageSearch.Results[0].Text != "x" {
		t.Fatal("viewmodel aliased search state")
	}
}
