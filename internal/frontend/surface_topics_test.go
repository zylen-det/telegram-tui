package frontend

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestTopicsKeyMappings(t *testing.T) {
	cases := map[tea.Key]Action{
		{Code: 'q', Text: "q"}: Close,
		{Code: tea.KeyEscape}:  Close,
		{Code: 'j', Text: "j"}: SelectNext,
		{Code: tea.KeyDown}:    SelectNext,
		{Code: 'k', Text: "k"}: SelectPrevious,
		{Code: tea.KeyUp}:      SelectPrevious,
		{Code: tea.KeyEnter}:   Activate,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(FocusTopics, tea.KeyPressMsg(key))
		if !ok || got.Action != want {
			t.Fatalf("topics key %#v = (%v,%t), want %v", key, got.Action, ok, want)
		}
	}
	// Unrelated keys stay unmapped for the topics modal.
	for _, reserved := range []tea.Key{
		{Code: 'h', Text: "h"},
		{Code: 'l', Text: "l"},
		{Code: 'x', Text: "x"},
	} {
		if got, ok := mapKeyPress(FocusTopics, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved topics key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestConversationTopicsShortcut(t *testing.T) {
	got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	if !ok || got.Action != OpenTopics {
		t.Fatalf("conversation t = (%v,%t), want OpenTopics", got.Action, ok)
	}
	// Existing shortcuts are untouched.
	if got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"})); !ok || got.Action != OpenMessageSearch {
		t.Fatalf("conversation / = (%v,%t), want OpenMessageSearch", got.Action, ok)
	}
	if got, ok := mapKeyPress(FocusConversation, tea.KeyPressMsg(tea.Key{Code: 'p', Text: "p"})); !ok || got.Action != OpenPinnedMessages {
		t.Fatalf("conversation p = (%v,%t), want OpenPinnedMessages", got.Action, ok)
	}
}

func topicsViewModel() ViewModel {
	return ViewModel{
		Width: 100, Height: 30,
		Layout: ViewLayout{Mode: LayoutWide},
		Focus:  FocusTopics,
		Topics: &TopicListState{
			RequestID: 7, ChatID: 9,
			Results: []domain.ForumTopic{
				{ID: 1, ChatID: 9, Name: "General", UnreadCount: 3},
				{ID: 2, ChatID: 9, Name: "Announcements", IsPinned: true, IsClosed: true, Draft: domain.Draft{Text: "draft"}},
			},
			Selected: 1,
		},
	}
}

func TestTopicsRowsCarryChatTopicIdentity(t *testing.T) {
	options := actionableRows(topicsRows(topicsViewModel().Topics))
	if len(options) != 3 {
		t.Fatalf("rows = %#v", options)
	}
	// Row 0 is the ALL pseudo-row matching the reducer's display indexing.
	if options[0].ID != "topic:all" || options[0].Label != "All messages" {
		t.Fatalf("row 0 = %#v", options[0])
	}
	if options[0].Action != (ActionReceived{Action: SelectTopic, ChatID: 9, TopicID: 0}) {
		t.Fatalf("row 0 payload = %#v", options[0].Action)
	}
	if options[1].ID != "topic:1" || options[1].Label != "General (unread 3)" {
		t.Fatalf("row 1 = %#v", options[1])
	}
	if options[1].Action != (ActionReceived{Action: SelectTopic, ChatID: 9, TopicID: 1}) {
		t.Fatalf("row 1 payload = %#v", options[1].Action)
	}
	if options[2].ID != "topic:2" {
		t.Fatalf("row 2 id = %#v", options[2])
	}
	if !strings.Contains(options[2].Label, "Announcements") || !strings.Contains(options[2].Label, "\U0001F4CC") ||
		!strings.Contains(options[2].Label, "\U0001F512") || !strings.Contains(options[2].Label, "…") {
		t.Fatalf("row 2 label = %q", options[2].Label)
	}
	if options[2].Action != (ActionReceived{Action: SelectTopic, ChatID: 9, TopicID: 2}) {
		t.Fatalf("row 2 payload = %#v", options[2].Action)
	}
	if got := actionableRows(topicsRows(nil)); got != nil {
		t.Fatalf("nil topics rows = %#v", got)
	}
}

func TestTopicsLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := topicsViewModel()
	layer := buildTopicsLayer(model, styles)
	if layer.Layer == nil || !layer.IsModal || layer.Rect.Empty() {
		t.Fatalf("layer = nil=%t modal=%t rect=%v", layer.Layer == nil, layer.IsModal, layer.Rect)
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if wantWidth := topicsFrame(bounds).Dx(); layer.Rect.Dx() != wantWidth {
		t.Fatalf("width = %d, want %d", layer.Rect.Dx(), wantWidth)
	}
	for _, interaction := range layer.Interactions {
		if strings.HasPrefix(interaction.ID, "conversation:") || strings.HasPrefix(interaction.ID, "chat:") {
			t.Fatalf("underlying hit leaked: %q", interaction.ID)
		}
	}

	rows := topicsRows(model.Topics)
	if len(rows) != 3 {
		t.Fatalf("rows = %#v", rows)
	}
	// topicsViewModel selects display row 1 (topic 1); only that row paints
	// selected and the ALL row carries the TopicID-0 payload.
	if rows[0].Selected || !rows[1].Selected || rows[2].Selected {
		t.Fatalf("selection = %#v, want row 1 selected only", rows)
	}
	wantActions := []ActionReceived{
		{Action: SelectTopic, ChatID: 9, TopicID: 0},
		{Action: SelectTopic, ChatID: 9, TopicID: 1},
		{Action: SelectTopic, ChatID: 9, TopicID: 2},
	}
	for index, row := range rows {
		if row.Action != wantActions[index] {
			t.Fatalf("row %d action = %#v, want %#v", index, row.Action, wantActions[index])
		}
	}

	// Selected==0 paints only the ALL row selected.
	allSelected := topicsViewModel()
	allSelected.Topics.Selected = 0
	if rows := topicsRows(allSelected.Topics); !rows[0].Selected || rows[1].Selected || rows[2].Selected {
		t.Fatalf("ALL selection = %#v, want row 0 selected only", rows)
	}

	loading := topicsViewModel()
	loading.Topics.Results = nil
	loading.Topics.Loading = true
	if rows := topicsRows(loading.Topics); len(rows) != 2 || rows[1].Label != "Loading topics..." || rows[1].ID != "" {
		t.Fatalf("loading rows = %#v", rows)
	}
	empty := topicsViewModel()
	empty.Topics.Results = nil
	empty.Topics.Done = true
	if rows := topicsRows(empty.Topics); len(rows) != 2 || rows[1].Label != "No topics" {
		t.Fatalf("empty rows = %#v", rows)
	}
	failed := topicsViewModel()
	failed.Topics.Results = nil
	failed.Topics.Error = &domain.AppError{Message: "Could not load topics"}
	if rows := topicsRows(failed.Topics); len(rows) != 2 || rows[1].Label != "Could not load topics" {
		t.Fatalf("error rows = %#v", rows)
	}
}

func TestTopicsRowsWindowCountsALLRow(t *testing.T) {
	// Capacity must count the prepended ALL row so the selected row (even
	// Selected==0) stays visible after windowing.
	base := topicsViewModel()
	for index := range 6 {
		base.Topics.Results = append(base.Topics.Results, domain.ForumTopic{ID: domain.TopicID(3 + index), ChatID: 9, Name: "extra"})
	}
	rows := topicsRows(base.Topics)
	if len(rows) != len(base.Topics.Results)+1 {
		t.Fatalf("rows = %d, want %d (ALL row + %d topics)", len(rows), len(base.Topics.Results)+1, len(base.Topics.Results))
	}
	// Window with capacity 4: the ALL row (row 0) stays first and the
	// selected topic-1 row (display row 1) stays visible.
	windowed := windowTopicsRows(rows, base.Topics.Selected, 4)
	if len(windowed) != 4 || windowed[0].ID != "topic:all" || windowed[1].ID != "topic:1" {
		t.Fatalf("windowed = %#v, want ALL row first with selected topic visible", windowed)
	}
	if !windowed[1].Selected {
		t.Fatalf("selected row lost after windowing: %#v", windowed)
	}
	// Last topic row (Selected = len(Results)) must stay visible as the last
	// windowed row.
	lastSelected := base.Topics
	lastSelected.Selected = len(base.Topics.Results)
	windowed = windowTopicsRows(topicsRows(lastSelected), lastSelected.Selected, 4)
	if len(windowed) != 4 || windowed[len(windowed)-1].Selected != true {
		t.Fatalf("windowed = %#v, want selected last topic visible", windowed)
	}
}

func TestTopicsLayerRowClickCarriesTopicIdentity(t *testing.T) {
	styles := newRenderStyles(true)
	model := topicsViewModel()
	layer := buildTopicsLayer(model, styles)
	var foundAll, foundTopic2 bool
	for _, interaction := range layer.Interactions {
		switch interaction.ID {
		case "topic:all":
			foundAll = true
			if interaction.Click != (ActionReceived{Action: SelectTopic, ChatID: 9, TopicID: 0}) {
				t.Fatalf("ALL row click = %#v", interaction.Click)
			}
		case "topic:2":
			foundTopic2 = true
			if interaction.Click != (ActionReceived{Action: SelectTopic, ChatID: 9, TopicID: 2}) {
				t.Fatalf("row click = %#v", interaction.Click)
			}
		}
	}
	if !foundAll {
		t.Fatalf("topic:all row hit missing")
	}
	if !foundTopic2 {
		t.Fatalf("topic:2 row hit missing")
	}
}

func TestAppModelTopicsKeyboardNavigationIsReducerOwned(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusTopics
	state.Width = 100
	state.Height = 24
	state.Topics = &TopicListState{
		RequestID: 7, ChatID: 9,
		Results:  []domain.ForumTopic{{ID: 1, ChatID: 9, Name: "General"}, {ID: 2, ChatID: 9, Name: "Announcements"}},
		Selected: 1,
		Done:     true,
	}
	model := newAppModelForTest(t, state, newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

	// Selected==1 indexes display rows where row 0 is the ALL pseudo-row.
	// k moves to the ALL row, j returns, and Enter selects the highlighted
	// row -- all through the reducer, the sole authority for Selected.
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if got := model.Snapshot().Topics.Selected; got != 0 {
		t.Fatalf("after up selected = %d, want 0", got)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if got := model.Snapshot().Topics.Selected; got != 1 {
		t.Fatalf("after j selected = %d, want 1", got)
	}
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := model.Snapshot(); got.Focus != FocusConversation {
		t.Fatalf("Enter left focus = %v", got.Focus)
	}
	if got := model.Snapshot().SelectedTopics[9]; got != 1 {
		t.Fatalf("Enter selected topic = %d, want 1 (General)", got)
	}
}
