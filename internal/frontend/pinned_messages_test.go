package frontend

import (
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func pinnedViewModel() ViewModel {
	sentAt := time.Unix(1700000000, 0).UTC()
	return ViewModel{
		Width: 100, Height: 30,
		Layout: ViewLayout{Mode: LayoutWide},
		Focus:  FocusPinnedResults,
		PinnedMessages: &PinnedMessagesState{
			RequestID: 10, ChatID: 9,
			Results: []domain.Message{
				{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "new pin", SenderName: "Ada", SentAt: sentAt},
				{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old pin", SenderName: "Bob", SentAt: sentAt},
			},
			Selected: 0,
		},
	}
}

func TestPinnedMessageKeyMappings(t *testing.T) {
	if got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(tea.Key{Code: 'p'})); !ok || got.Action != OpenPinnedMessages {
		t.Fatalf("conversation p = (%#v,%t), want OpenPinnedMessages", got, ok)
	}
	cases := map[tea.Key]Action{
		{Code: tea.KeyDown}:    SelectNext,
		{Code: 'j', Text: "j"}: SelectNext,
		{Code: tea.KeyUp}:      SelectPrevious,
		{Code: 'k', Text: "k"}: SelectPrevious,
		{Code: tea.KeyEnter}:   Activate,
		{Code: tea.KeyEscape}:  Close,
		{Code: 'q', Text: "q"}: Close,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(FocusPinnedResults, tea.KeyPressMsg(key))
		if !ok || got.Action != want {
			t.Fatalf("pinned results key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
}

func TestPinnedMessageSelectorOptionsCarryChatMessageIdentity(t *testing.T) {
	model := pinnedViewModel()
	options := selectorOptionsFromRows(pinnedMessagesRows(model, time.UTC))
	if len(options) != 2 || options[0].ID != "pinned:30" || options[1].ID != "pinned:20" {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Value != (ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 30}) {
		t.Fatalf("option payload = %#v", options[0].Value)
	}
	if got := selectorOptionsFromRows(pinnedMessagesRows(ViewModel{}, time.UTC)); got != nil {
		t.Fatalf("nil pinned options = %#v", got)
	}
}

func TestPinnedMessageResultLabelSanitizesAndBounds(t *testing.T) {
	label := pinnedMessageResultLabel(" Ada\x00\x01 ", time.Unix(1700000000, 0).UTC(), "hello\x00world", time.UTC)
	if strings.Contains(label, "\x00") || !strings.Contains(label, "Ada") || !strings.Contains(label, "hello") {
		t.Fatalf("label = %q", label)
	}
	empty := pinnedMessageResultLabel("", time.Unix(0, 0).UTC(), "", time.UTC)
	if !strings.Contains(empty, "Unknown") || !strings.Contains(empty, "[Unsupported]") {
		t.Fatalf("empty label = %q", empty)
	}
}

func TestPinnedMessageLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := pinnedViewModel()
	layer := buildPinnedMessagesLayer(model, time.UTC, styles, "")
	if layer.Layer == nil || !layer.IsModal || layer.Rect.Empty() {
		t.Fatalf("layer = nil=%t modal=%t rect=%v", layer.Layer == nil, layer.IsModal, layer.Rect)
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	wantWidth := pinnedMessagesFrame(bounds).Dx()
	if layer.Rect.Dx() != wantWidth {
		t.Fatalf("results width = %d, want pinned frame width %d", layer.Rect.Dx(), wantWidth)
	}
	for _, interaction := range layer.Interactions {
		if strings.HasPrefix(interaction.ID, "conversation:") || strings.HasPrefix(interaction.ID, "chat:") {
			t.Fatalf("underlying hit leaked: %q", interaction.ID)
		}
	}
	// Loading / empty / error rows are informational, not dead ends.
	loading := pinnedViewModel()
	loading.PinnedMessages.Results = nil
	loading.PinnedMessages.Loading = true
	if rows := pinnedMessagesRows(loading, time.UTC); len(rows) != 1 || rows[0].Label != "Loading pinned messages..." {
		t.Fatalf("loading rows = %#v", rows)
	}
	emptyModel := pinnedViewModel()
	emptyModel.PinnedMessages.Results = nil
	if rows := pinnedMessagesRows(emptyModel, time.UTC); len(rows) != 1 || rows[0].Label != "No pinned messages" {
		t.Fatalf("empty rows = %#v", rows)
	}
	failed := pinnedViewModel()
	failed.PinnedMessages.Results = nil
	failed.PinnedMessages.Error = &domain.AppError{Message: "Could not load pinned messages"}
	if rows := pinnedMessagesRows(failed, time.UTC); len(rows) != 1 || rows[0].Label != "Could not load pinned messages" {
		t.Fatalf("error rows = %#v", rows)
	}
	// Result rows preserve mouse parity payloads.
	rows := pinnedMessagesRows(model, time.UTC)
	if rows[0].Action != (ActionReceived{Action: SelectMessage, ChatID: 9, MessageID: 30}) {
		t.Fatalf("row action = %#v", rows[0].Action)
	}
}

func TestPinnedMessageInputGeometryIsBounded(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 30)
	frame := pinnedMessagesFrame(bounds)
	if frame.Empty() || frame.Dx() > 64 {
		t.Fatalf("frame = %v", frame)
	}
	if got := pinnedMessagesFrame(image.Rectangle{}); !got.Empty() {
		t.Fatalf("empty bounds frame = %v", got)
	}
}

func TestPinnedMessageViewModelClonesWithoutAliasing(t *testing.T) {
	state := InitialState()
	state.Focus = FocusPinnedResults
	state.PinnedMessages = &PinnedMessagesState{
		ChatID:  9,
		Results: []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "x"}},
	}
	model := Select(state, time.UTC)
	if model.PinnedMessages == nil || len(model.PinnedMessages.Results) != 1 {
		t.Fatalf("projection = %#v", model.PinnedMessages)
	}
	model.PinnedMessages.Results[0].Text = "mutated"
	if state.PinnedMessages.Results[0].Text != "x" {
		t.Fatal("viewmodel aliased pinned state")
	}
}
