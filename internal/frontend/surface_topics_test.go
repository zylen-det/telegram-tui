package frontend

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func TestTopicsKeyMappings(t *testing.T) {
	cases := map[tea.Key]app.Action{
		{Code: 'q', Text: "q"}: app.Close,
		{Code: tea.KeyEscape}:  app.Close,
		{Code: 'j', Text: "j"}: app.SelectNext,
		{Code: tea.KeyDown}:    app.SelectNext,
		{Code: 'k', Text: "k"}: app.SelectPrevious,
		{Code: tea.KeyUp}:      app.SelectPrevious,
		{Code: tea.KeyEnter}:   app.Activate,
	}
	for key, want := range cases {
		got, ok := mapKeyPress(app.FocusTopics, tea.KeyPressMsg(key))
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
		if got, ok := mapKeyPress(app.FocusTopics, tea.KeyPressMsg(reserved)); ok {
			t.Fatalf("reserved topics key %#v mapped to %v", reserved, got.Action)
		}
	}
}

func TestConversationTopicsShortcut(t *testing.T) {
	got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	if !ok || got.Action != app.OpenTopics {
		t.Fatalf("conversation t = (%v,%t), want OpenTopics", got.Action, ok)
	}
	// Existing shortcuts are untouched.
	if got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"})); !ok || got.Action != app.OpenMessageSearch {
		t.Fatalf("conversation / = (%v,%t), want OpenMessageSearch", got.Action, ok)
	}
	if got, ok := mapKeyPress(app.FocusConversation, tea.KeyPressMsg(tea.Key{Code: 'p', Text: "p"})); !ok || got.Action != app.OpenPinnedMessages {
		t.Fatalf("conversation p = (%v,%t), want OpenPinnedMessages", got.Action, ok)
	}
}

func topicsViewModel() ui.ViewModel {
	return ui.ViewModel{
		Width: 100, Height: 30,
		Layout: ui.Layout{Mode: app.LayoutWide},
		Focus:  app.FocusTopics,
		Topics: &app.TopicListState{
			RequestID: 7, ChatID: 9,
			Results: []domain.ForumTopic{
				{ID: 1, ChatID: 9, Name: "General", UnreadCount: 3},
				{ID: 2, ChatID: 9, Name: "Announcements", IsPinned: true, IsClosed: true, Draft: domain.Draft{Text: "draft"}},
			},
			Selected: 1,
		},
	}
}

func TestTopicsSelectorOptionsCarryChatTopicIdentity(t *testing.T) {
	options := selectorOptionsFromRows(topicsRows(topicsViewModel().Topics))
	if len(options) != 3 {
		t.Fatalf("options = %#v", options)
	}
	// Row 0 is the ALL pseudo-row matching the reducer's display indexing.
	if options[0].ID != "topic:all" || options[0].Label != "All messages" {
		t.Fatalf("option 0 = %#v", options[0])
	}
	if options[0].Value != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 0}) {
		t.Fatalf("option 0 payload = %#v", options[0].Value)
	}
	if options[1].ID != "topic:1" || options[1].Label != "General (unread 3)" {
		t.Fatalf("option 1 = %#v", options[1])
	}
	if options[1].Value != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 1}) {
		t.Fatalf("option 1 payload = %#v", options[1].Value)
	}
	if options[2].ID != "topic:2" {
		t.Fatalf("option 2 id = %#v", options[2])
	}
	if !strings.Contains(options[2].Label, "Announcements") || !strings.Contains(options[2].Label, "\U0001F4CC") ||
		!strings.Contains(options[2].Label, "\U0001F512") || !strings.Contains(options[2].Label, "…") {
		t.Fatalf("option 2 label = %q", options[2].Label)
	}
	if options[2].Value != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 2}) {
		t.Fatalf("option 2 payload = %#v", options[2].Value)
	}
	if got := selectorOptionsFromRows(topicsRows(nil)); got != nil {
		t.Fatalf("nil topics options = %#v", got)
	}
}

func TestTopicsLayerIsModalAndClosesUnderlyingHits(t *testing.T) {
	styles := newRenderStyles(true)
	model := topicsViewModel()
	layer := buildTopicsLayer(model, styles, "")
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
	wantActions := []app.ActionReceived{
		{Action: app.SelectTopic, ChatID: 9, TopicID: 0},
		{Action: app.SelectTopic, ChatID: 9, TopicID: 1},
		{Action: app.SelectTopic, ChatID: 9, TopicID: 2},
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
	layer := buildTopicsLayer(model, styles, "")
	var foundAll, foundTopic2 bool
	for _, interaction := range layer.Interactions {
		switch interaction.ID {
		case "topic:all":
			foundAll = true
			if interaction.Click != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 0}) {
				t.Fatalf("ALL row click = %#v", interaction.Click)
			}
		case "topic:2":
			foundTopic2 = true
			if interaction.Click != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 2}) {
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

func TestAppModelSelectorTopicsRouteSync(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = app.FocusTopics
	state.Width = 100
	state.Height = 24
	state.Topics = &app.TopicListState{
		RequestID: 7, ChatID: 9,
		Results:  []domain.ForumTopic{{ID: 1, ChatID: 9, Name: "General"}, {ID: 2, ChatID: 9, Name: "Announcements"}},
		Selected: 1,
		Done:     true,
	}
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	_ = model.syncListModalController()
	if got := model.listModals.host.Identity(); got != (selectorIdentity{Kind: selectorTopics, RequestID: 7, ChatID: 9}) {
		t.Fatalf("selector identity = %#v", got)
	}
	// Selected==1 indexes display rows where row 0 is the ALL pseudo-row, so
	// the synced value is topic 1 (General).
	if got := model.listModals.host.Value(); got != (app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 1}) {
		t.Fatalf("selector value = %#v", got)
	}
	if !model.listModals.host.focused {
		t.Fatal("selector not focused")
	}
	if options := model.listModals.host.Options(); len(options) != 3 || options[0].ID != "topic:all" || options[2].ID != "topic:2" {
		t.Fatalf("selector options = %#v", options)
	}
}
