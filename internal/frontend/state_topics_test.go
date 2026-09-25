package frontend

import (
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func topicsBaseState(t *testing.T) State {
	t.Helper()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusChats
	state.Chats = []domain.Chat{{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	return state
}

func testTopic(id domain.TopicID, name string) domain.ForumTopic {
	return domain.ForumTopic{ID: id, ChatID: 7, Name: name}
}

func trackTopic(state *State, topic domain.ForumTopic) {
	if state.ForumTopics[topic.ChatID] == nil {
		state.ForumTopics[topic.ChatID] = make(map[domain.TopicID]domain.ForumTopic)
	}
	state.ForumTopics[topic.ChatID][topic.ID] = topic
}

func TestTopicMessageNavigationSkipsMessagesHiddenByTheConversation(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusConversation
	trackTopic(&state, testTopic(101, "Selected"))
	state.SelectedTopics[7] = 101
	state.Messages[7] = []domain.Message{
		{ID: 1, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "first"},
		{ID: 2, ChatID: 7, TopicID: 102, Kind: domain.MessageText, Text: "hidden"},
		{ID: 3, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "reply target"},
		{ID: 4, ChatID: 7, TopicID: 102, Kind: domain.MessageText, Text: "hidden"},
		{ID: 5, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "edited reply", HasReply: true, ReplyToMessageID: 3, EditedAt: time.Unix(123, 0)},
	}
	state.SelectedMessageChat, state.SelectedMessage = 7, 5
	key := topicKey{ChatID: 7, TopicID: 101}

	for _, want := range []struct {
		action Action
		id     domain.MessageID
		offset int
	}{
		{SelectPreviousMessage, 3, 1},
		{SelectPreviousMessage, 1, 2},
		{SelectPreviousMessage, 1, 2},
		{SelectNextMessage, 3, 1},
		{SelectNextMessage, 5, 0},
	} {
		updateState(&state, ActionReceived{Action: want.action})
		if state.SelectedMessageChat != 7 || state.SelectedMessage != want.id {
			t.Fatalf("selection = %d/%d after %v, want 7/%d", state.SelectedMessageChat, state.SelectedMessage, want.action, want.id)
		}
		if history := state.TopicHistory[key]; !history.FollowSelection || history.ViewOffset != want.offset {
			t.Fatalf("topic history after %v = %#v, want following offset %d", want.action, history, want.offset)
		}
		model := Select(state, time.UTC)
		if index := selectedHistoryGroupIndex(model.Groups, messageSelection{ChatID: 7, MessageID: want.id}); index < 0 {
			t.Fatalf("selected message %d absent from visible conversation", want.id)
		}
	}
	if state.History[7].FollowSelection {
		t.Fatal("topic navigation changed chat-level history")
	}

	activateAllTopics(&state, 7)
	updateState(&state, ActionReceived{Action: SelectPreviousMessage})
	if state.SelectedMessage != 4 || !state.History[7].FollowSelection || state.History[7].ViewOffset != 1 {
		t.Fatalf("ALL navigation = message %d history %#v, want message 4 and chat offset 1", state.SelectedMessage, state.History[7])
	}
}

func TestOpenTopicsGating(t *testing.T) {
	opened := topicsBaseState(t)
	commands := updateState(&opened, ActionReceived{Action: OpenTopics})
	if opened.Topics == nil || opened.Topics.ChatID != 7 || opened.Topics.RequestID != 10 || !opened.Topics.Loading {
		t.Fatalf("open = %#v", opened.Topics)
	}
	if opened.Focus != FocusTopics || opened.Topics.PreviousFocus != FocusChats {
		t.Fatalf("focus=%v previous=%v", opened.Focus, opened.Topics.PreviousFocus)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if command, ok := commands[0].(LoadTopics); !ok || command.RequestID != 10 || command.ChatID != 7 || command.Cursor != (telegram.TopicCursor{Limit: pageSize}) {
		t.Fatalf("command = %#v", commands[0])
	}

	nonForum := topicsBaseState(t)
	nonForum.Chats[0].IsForum = false
	if cmds := updateState(&nonForum, ActionReceived{Action: OpenTopics}); nonForum.Topics != nil || len(cmds) != 0 {
		t.Fatalf("non-forum open = %#v commands=%#v", nonForum.Topics, cmds)
	}

	wrongFocus := topicsBaseState(t)
	wrongFocus.Focus = FocusComposer
	if cmds := updateState(&wrongFocus, ActionReceived{Action: OpenTopics}); wrongFocus.Topics != nil || len(cmds) != 0 {
		t.Fatalf("non-chat focus open = %#v commands=%#v", wrongFocus.Topics, cmds)
	}

	conversation := topicsBaseState(t)
	conversation.Focus = FocusConversation
	commands = updateState(&conversation, ActionReceived{Action: OpenTopics})
	if conversation.Topics == nil || len(commands) != 1 {
		t.Fatalf("conversation open = %#v commands=%#v", conversation.Topics, commands)
	}
}

func TestTopicsCloseRestoresPreviousFocusAndKeepsSelectedTopics(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{RequestID: 10, ChatID: 7, PreviousFocus: FocusChats, Results: []domain.ForumTopic{testTopic(101, "A")}}
	state.SelectedTopics[7] = 101
	commands := updateState(&state, ActionReceived{Action: Close})
	if state.Topics != nil {
		t.Fatalf("topics = %#v", state.Topics)
	}
	if state.Focus != FocusChats {
		t.Fatalf("focus = %v", state.Focus)
	}
	if state.SelectedTopics[7] != 101 {
		t.Fatalf("selected topics = %#v", state.SelectedTopics)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestTopicsSelectionClampsWithoutWrap(t *testing.T) {
	topics := func(selected int) *TopicListState {
		return &TopicListState{
			RequestID: 10,
			ChatID:    7,
			Done:      true,
			Results:   []domain.ForumTopic{testTopic(101, "A"), testTopic(102, "B"), testTopic(103, "C")},
			Selected:  selected,
		}
	}
	// Row 0 is the ALL pseudo-row; row 3 is the last real topic.
	last := topicsBaseState(t)
	last.Focus = FocusTopics
	last.Topics = topics(3)
	commands := updateState(&last, ActionReceived{Action: SelectNext})
	if last.Topics.Selected != 3 || len(commands) != 0 {
		t.Fatalf("next at last: selected=%d commands=%#v", last.Topics.Selected, commands)
	}
	first := topicsBaseState(t)
	first.Focus = FocusTopics
	first.Topics = topics(0)
	commands = updateState(&first, ActionReceived{Action: SelectPrevious})
	if first.Topics.Selected != 0 || len(commands) != 0 {
		t.Fatalf("previous at first: selected=%d commands=%#v", first.Topics.Selected, commands)
	}
}

func TestTopicsSelectNextOnLastEmitsNextPage(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID:  10,
		ChatID:     7,
		NextCursor: telegram.TopicCursor{OffsetDate: 5, OffsetMessageID: 21, OffsetTopicID: 103, Limit: pageSize},
		Results:    []domain.ForumTopic{testTopic(101, "A"), testTopic(102, "B"), testTopic(103, "C")},
		Selected:   3,
	}
	state.NextRequestID = 20
	commands := updateState(&state, ActionReceived{Action: SelectNext})
	if state.Topics.Selected != 3 || !state.Topics.Loading || state.Topics.RequestID != 20 {
		t.Fatalf("topics = %#v", state.Topics)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	command, ok := commands[0].(LoadTopics)
	if !ok || command.RequestID != 20 || command.ChatID != 7 || command.Cursor != state.Topics.NextCursor {
		t.Fatalf("command = %#v", commands[0])
	}
}

func TestTopicsLoadedIgnoresStaleResults(t *testing.T) {
	state := topicsBaseState(t)
	updateState(&state, ActionReceived{Action: OpenTopics})
	page := telegram.TopicPage{Topics: []domain.ForumTopic{testTopic(101, "A")}, TotalCount: 1}
	commands := updateState(&state, TopicsLoaded{RequestID: 99, ChatID: 7, Page: page})
	if len(state.Topics.Results) != 0 || !state.Topics.Loading || len(commands) != 0 {
		t.Fatalf("stale loaded: %#v commands=%#v", state.Topics, commands)
	}
	commands = updateState(&state, TopicsLoaded{RequestID: 10, ChatID: 8, Page: page})
	if len(state.Topics.Results) != 0 || !state.Topics.Loading || len(commands) != 0 {
		t.Fatalf("wrong chat loaded: %#v commands=%#v", state.Topics, commands)
	}
}

func TestTopicsLoadedDeduplicatesInPlaceAndAppendsNew(t *testing.T) {
	state := topicsBaseState(t)
	updateState(&state, ActionReceived{Action: OpenTopics})
	state.Topics.Results = []domain.ForumTopic{testTopic(101, "old A"), testTopic(102, "old B")}
	page := telegram.TopicPage{
		Topics:              []domain.ForumTopic{testTopic(102, "new B"), testTopic(103, "C"), testTopic(101, "new A"), {ID: 0}},
		TotalCount:          3,
		NextOffsetDate:      8,
		NextOffsetMessageID: 30,
		NextOffsetTopicID:   104,
		Done:                true,
	}
	commands := updateState(&state, TopicsLoaded{RequestID: 10, ChatID: 7, Page: page})
	got := state.Topics
	if len(got.Results) != 3 {
		t.Fatalf("results = %#v", got.Results)
	}
	if got.Results[0] != (domain.ForumTopic{ID: 101, ChatID: 7, Name: "new A"}) || got.Results[1] != (domain.ForumTopic{ID: 102, ChatID: 7, Name: "new B"}) || got.Results[2] != (domain.ForumTopic{ID: 103, ChatID: 7, Name: "C"}) {
		t.Fatalf("order/dedup = %#v", got.Results)
	}
	if !got.Done || got.TotalCount != 3 || got.Loading {
		t.Fatalf("pagination = %#v", got)
	}
	if got.NextCursor != (telegram.TopicCursor{OffsetDate: 8, OffsetMessageID: 30, OffsetTopicID: 104, Limit: pageSize}) {
		t.Fatalf("next cursor = %#v", got.NextCursor)
	}
	if got.Selected != 0 {
		t.Fatalf("selected = %d", got.Selected)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestTopicsLoadFailedSetsSanitizedError(t *testing.T) {
	state := topicsBaseState(t)
	updateState(&state, ActionReceived{Action: OpenTopics})
	commands := updateState(&state, TopicsLoadFailed{RequestID: 10, ChatID: 7, Error: domain.AppError{Kind: domain.ErrorNetwork, Op: "load topics", Message: "boom"}})
	if len(commands) != 0 || state.Topics.Loading || state.Topics.Error == nil || state.Topics.Error.Message != "boom" {
		t.Fatalf("failed = %#v commands=%#v", state.Topics, commands)
	}
	updateState(&state, TopicsLoadFailed{RequestID: 99, ChatID: 7, Error: domain.AppError{Message: "stale"}})
	if state.Topics.Error.Message != "boom" {
		t.Fatalf("stale failure applied: %#v", state.Topics.Error)
	}
}

func TestSelectTopicValidatesChatAndRow(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{RequestID: 10, ChatID: 7, Results: []domain.ForumTopic{testTopic(101, "A")}}
	commands := updateState(&state, ActionReceived{Action: SelectTopic, ChatID: 8, TopicID: 101})
	if state.Topics == nil || len(commands) != 0 {
		t.Fatalf("wrong chat: topics=%#v commands=%#v", state.Topics, commands)
	}
	commands = updateState(&state, ActionReceived{Action: SelectTopic, ChatID: 7, TopicID: 999})
	if state.Topics == nil || len(commands) != 0 {
		t.Fatalf("unknown topic: topics=%#v commands=%#v", state.Topics, commands)
	}
	// SelectTopic with no topic list open is a no-op.
	plain := topicsBaseState(t)
	commands = updateState(&plain, ActionReceived{Action: SelectTopic, ChatID: 7, TopicID: 101})
	if plain.Topics != nil || len(commands) != 0 {
		t.Fatalf("no list: topics=%#v commands=%#v", plain.Topics, commands)
	}
}

func TestActivateTopicSeedsDraftsAndRequestsTopicMessages(t *testing.T) {
	state := topicsBaseState(t)
	state.Focus = FocusTopics
	state.Topics = &TopicListState{
		RequestID: 10,
		ChatID:    7,
		Results:   []domain.ForumTopic{{ID: 101, ChatID: 7, Name: "A", IsGeneral: true, Draft: domain.Draft{Text: "cloud draft", ReplyToMessageID: 5, Date: 42}}},
		Selected:  1,
	}
	state.ReplyTarget = &ReplyTarget{ChatID: 7, MessageID: 5, Preview: "reply"}
	state.EditTarget = &EditTarget{ChatID: 7, MessageID: 6}
	state.MessageMenu = &MessageActionMenu{ChatID: 7, MessageID: 6}
	state.Messages[7] = []domain.Message{
		{ID: 3, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "a"},
		{ID: 4, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "b"},
	}
	commands := updateState(&state, ActionReceived{Action: Activate})
	if state.Topics != nil || state.Focus != FocusConversation {
		t.Fatalf("topics=%#v focus=%v", state.Topics, state.Focus)
	}
	if state.ReplyTarget != nil || state.EditTarget != nil || state.MessageMenu != nil {
		t.Fatalf("targets = %#v %#v %#v", state.ReplyTarget, state.EditTarget, state.MessageMenu)
	}
	if state.SelectedTopics[7] != 101 {
		t.Fatalf("selected topics = %#v", state.SelectedTopics)
	}
	if state.ForumTopics[7][101].ID != 101 {
		t.Fatalf("forum topics = %#v", state.ForumTopics[7])
	}
	key := topicKey{ChatID: 7, TopicID: 101}
	if state.TopicDrafts[key] != "cloud draft" || state.TopicDraftReplies[key] != 5 || state.TopicDraftDates[key] != 42 {
		t.Fatalf("drafts = %q reply=%d date=%d", state.TopicDrafts[key], state.TopicDraftReplies[key], state.TopicDraftDates[key])
	}
	if state.SelectedMessageChat != 7 || state.SelectedMessage != 4 {
		t.Fatalf("selection = %d/%d", state.SelectedMessageChat, state.SelectedMessage)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	command, ok := commands[0].(LoadMessages)
	if !ok || command.RequestID != 10 || command.ChatID != 7 || command.TopicID != 101 || command.Cursor != (telegram.MessageCursor{Limit: pageSize}) {
		t.Fatalf("command = %#v", commands[0])
	}
	if history := state.TopicHistory[key]; !history.Loading || history.RequestID != 10 {
		t.Fatalf("topic history = %#v", history)
	}
	if _, exists := state.History[7]; exists {
		t.Fatalf("chat history touched: %#v", state.History[7])
	}

	// Re-activating the same topic does not re-request messages.
	state.Topics = &TopicListState{ChatID: 7, Results: []domain.ForumTopic{testTopic(101, "A")}}
	commands = updateState(&state, ActionReceived{Action: SelectTopic, ChatID: 7, TopicID: 101})
	if state.Topics != nil || len(commands) != 0 {
		t.Fatalf("re-activate: topics=%#v commands=%#v", state.Topics, commands)
	}
}

func TestTopicMessagesLoadedUsesTopicHistoryOnly(t *testing.T) {
	state := topicsBaseState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	state.SelectedTopics[7] = 101
	state.TopicHistory[key] = HistoryState{Loading: true, RequestID: 10}
	state.Messages[7] = []domain.Message{{ID: 1, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "a"}}

	staleCommands := updateState(&state, MessagesLoaded{RequestID: 12, ChatID: 7, TopicID: 101, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 2, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "b"}}}})
	if len(staleCommands) != 0 || !state.TopicHistory[key].Loading || state.TopicHistory[key].RequestID != 10 {
		t.Fatalf("stale request applied: %#v commands=%#v", state.TopicHistory[key], staleCommands)
	}

	commands := updateState(&state, MessagesLoaded{
		RequestID: 10,
		ChatID:    7,
		TopicID:   101,
		Page: telegram.MessagePage{
			Messages: []domain.Message{
				{ID: 2, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "b"},
				{ID: 3, ChatID: 7, TopicID: 102, Kind: domain.MessageText, Text: "c"},
			},
			Done: true,
		},
	})
	if len(state.Messages[7]) != 3 {
		t.Fatalf("messages = %#v", state.Messages[7])
	}
	if _, exists := state.History[7]; exists {
		t.Fatalf("chat history touched: %#v", state.History[7])
	}
	history := state.TopicHistory[key]
	if history.Loading || !history.Done || history.OldestID != 1 || history.Error != nil {
		t.Fatalf("topic history = %#v", history)
	}
	if state.SelectedMessageChat != 7 || state.SelectedMessage != 2 {
		t.Fatalf("selection = %d/%d", state.SelectedMessageChat, state.SelectedMessage)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestTopicMessagesLoadFailedUsesTopicHistoryOnly(t *testing.T) {
	state := topicsBaseState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	state.SelectedTopics[7] = 101
	state.TopicHistory[key] = HistoryState{Loading: true, RequestID: 10}
	updateState(&state, MessagesLoadFailed{RequestID: 10, ChatID: 7, TopicID: 101, Error: domain.AppError{Message: "topic boom"}})
	if _, exists := state.History[7]; exists {
		t.Fatalf("chat history touched: %#v", state.History[7])
	}
	history := state.TopicHistory[key]
	if history.Loading || history.Error == nil || history.Error.Message != "topic boom" {
		t.Fatalf("topic history = %#v", history)
	}
	if state.Toast == nil || state.Toast.Message != "topic boom" {
		t.Fatalf("toast = %#v", state.Toast)
	}

	// A failed topic load for a non-selected topic toasts nothing.
	other := topicsBaseState(t)
	other.TopicHistory[key] = HistoryState{Loading: true, RequestID: 10}
	updateState(&other, MessagesLoadFailed{RequestID: 10, ChatID: 7, TopicID: 101, Error: domain.AppError{Message: "topic boom"}})
	if other.Toast != nil {
		t.Fatalf("unexpected toast = %#v", other.Toast)
	}
	if other.TopicHistory[key].Error == nil {
		t.Fatalf("topic history = %#v", other.TopicHistory[key])
	}
}

func TestForumTopicInfoChangedMergesAllowlist(t *testing.T) {
	newState := func() State {
		state := topicsBaseState(t)
		entry := domain.ForumTopic{
			ID: 101, ChatID: 7, Name: "old", IconColor: 1, IsGeneral: true,
			UnreadCount: 9, LastMessage: "last", Order: 7, Draft: domain.Draft{Text: "d"},
		}
		trackTopic(&state, entry)
		state.Topics = &TopicListState{ChatID: 7, Results: []domain.ForumTopic{entry}}
		return state
	}
	update := telegram.ForumTopicInfoChanged{Topic: domain.ForumTopic{ID: 101, ChatID: 7, Name: "new", IconColor: 2, IsClosed: true, IsGeneral: true}}

	reduced := newState()
	updateState(&reduced, TelegramEvent{Value: update})
	entry := reduced.ForumTopics[7][101]
	if entry.Name != "new" || entry.IconColor != 2 || !entry.IsClosed || !entry.IsGeneral {
		t.Fatalf("merged entry = %#v", entry)
	}
	if entry.IsPinned || entry.UnreadCount != 9 || entry.UnreadMentionCount != 0 || entry.LastMessage != "last" || entry.Order != 7 || entry.Draft.Text != "d" {
		t.Fatalf("untouched fields = %#v", entry)
	}
	if reduced.Topics.Results[0].Name != "new" || !reduced.Topics.Results[0].IsClosed {
		t.Fatalf("list row = %#v", reduced.Topics.Results[0])
	}

	ptr := telegram.ForumTopicInfoChanged{Topic: domain.ForumTopic{ID: 101, ChatID: 7, Name: "ptr", IsClosed: true}}
	reduced = newState()
	updateState(&reduced, TelegramEvent{Value: &ptr})
	if reduced.ForumTopics[7][101].Name != "ptr" {
		t.Fatalf("pointer form = %#v", reduced.ForumTopics[7][101])
	}

	// Updates for unknown chats leave state untouched.
	untouched := newState()
	updateState(&untouched, TelegramEvent{Value: telegram.ForumTopicInfoChanged{Topic: domain.ForumTopic{ID: 555, ChatID: 999, Name: "ghost"}}})
	if _, exists := untouched.ForumTopics[999]; exists {
		t.Fatalf("unknown chat added: %#v", untouched.ForumTopics)
	}
}

func TestForumTopicStateChangedMergesAllowlistAndGuardsDraft(t *testing.T) {
	dirty := topicsBaseState(t)
	key := topicKey{ChatID: 7, TopicID: 101}
	trackTopic(&dirty, domain.ForumTopic{ID: 101, ChatID: 7, Name: "A"})
	dirty.TopicDrafts[key] = "local"
	dirty.TopicDraftSync[key] = DraftSyncState{Dirty: true, Draft: domain.Draft{Text: "local"}}
	update := telegram.ForumTopicStateChanged{ChatID: 7, TopicID: 101, IsPinned: true, UnreadMentionCount: 3, Draft: domain.Draft{Text: "cloud"}}
	updateState(&dirty, TelegramEvent{Value: update})
	entry := dirty.ForumTopics[7][101]
	if !entry.IsPinned || entry.UnreadMentionCount != 3 {
		t.Fatalf("merged entry = %#v", entry)
	}
	if entry.Draft.Text != "" {
		t.Fatalf("dirty draft replaced: %#v", entry.Draft)
	}
	if dirty.TopicDrafts[key] != "local" || !dirty.TopicDraftSync[key].Dirty {
		t.Fatalf("dirty topic draft overwritten: %q %#v", dirty.TopicDrafts[key], dirty.TopicDraftSync[key])
	}

	clean := topicsBaseState(t)
	trackTopic(&clean, domain.ForumTopic{ID: 101, ChatID: 7, Name: "A"})
	updateState(&clean, TelegramEvent{Value: update})
	if clean.TopicDrafts[key] != "cloud" || clean.ForumTopics[7][101].Draft.Text != "cloud" {
		t.Fatalf("clean draft not merged: %q %#v", clean.TopicDrafts[key], clean.ForumTopics[7][101].Draft)
	}
	if clean.TopicDraftSync[key].Dirty || clean.TopicDraftSync[key].Draft.Text != "cloud" {
		t.Fatalf("sync = %#v", clean.TopicDraftSync[key])
	}

	ptr := telegram.ForumTopicStateChanged{ChatID: 7, TopicID: 101, IsPinned: true}
	updateState(&clean, TelegramEvent{Value: &ptr})
	if !clean.ForumTopics[7][101].IsPinned {
		t.Fatalf("pointer form = %#v", clean.ForumTopics[7][101])
	}
}

func forumComposerState(t *testing.T) State {
	t.Helper()
	state := topicsBaseState(t)
	state.Focus = FocusComposer
	state.SelectedTopics[7] = 101
	trackTopic(&state, domain.ForumTopic{ID: 101, ChatID: 7, IsClosed: true})
	state.Drafts[7] = "hello"
	return state
}

func TestClosedTopicBlocksComposeAndSenders(t *testing.T) {
	state := forumComposerState(t)
	commands := updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) != 0 || state.Drafts[7] != "hello" {
		t.Fatalf("submit: commands=%#v draft=%q", commands, state.Drafts[7])
	}
	commands = updateState(&state, ActionReceived{Action: OpenPhotoSend})
	if len(commands) != 0 || state.PhotoSend != nil {
		t.Fatalf("photo send: commands=%#v photo=%#v", commands, state.PhotoSend)
	}
	commands = updateState(&state, ActionReceived{Action: OpenStickerPicker})
	if len(commands) != 0 || state.StickerPicker != nil {
		t.Fatalf("sticker picker: commands=%#v picker=%#v", commands, state.StickerPicker)
	}

	// Open (non-closed) topic sends its topic draft normally.
	open := state.ForumTopics[7][101]
	open.IsClosed = false
	state.ForumTopics[7][101] = open
	state.TopicDrafts[topicKey{ChatID: 7, TopicID: 101}] = "hello"
	commands = updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) == 0 {
		t.Fatalf("open submit commands = %#v", commands)
	}
	send, ok := commands[0].(SendText)
	if !ok || send.TopicID != 101 {
		t.Fatalf("command = %#v", commands[0])
	}
	if state.TopicDrafts[topicKey{ChatID: 7, TopicID: 101}] != "" {
		t.Fatalf("draft = %q", state.TopicDrafts[topicKey{ChatID: 7, TopicID: 101}])
	}

	// Unknown (untracked) topic has no topic draft: submit is a no-op.
	state = forumComposerState(t)
	state.SelectedTopics[7] = 102
	commands = updateState(&state, ActionReceived{Action: ComposerSubmit})
	if len(commands) != 0 {
		t.Fatalf("unknown topic submit commands = %#v", commands)
	}
}

func TestSelectingForumChatDoesNotOpenIt(t *testing.T) {
	base := func() State {
		state := topicsBaseState(t)
		state.Chats = []domain.Chat{
			{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true},
			{ID: 8, Kind: domain.ChatBasicGroup, Title: "Group", CanSend: true},
		}
		state.SelectedChat = 1
		return state
	}
	checkSelection := func(t *testing.T, name string, selected State, wantSelected int) {
		t.Helper()
		if selected.SelectedChat != wantSelected || selected.FocusedChat != 0 {
			t.Fatalf("%s: selected/focused chat = %d/%d, want %d/0", name, selected.SelectedChat, selected.FocusedChat, wantSelected)
		}
		if selected.Focus != FocusChats {
			t.Fatalf("%s: focus = %v, want FocusChats", name, selected.Focus)
		}
		if selected.Topics != nil || selected.ShowAll[7] {
			t.Fatalf("%s: forum activated: topics=%#v showAll=%#v", name, selected.Topics, selected.ShowAll)
		}
	}

	selected := base()
	updateState(&selected, ActionReceived{Action: SelectChat, ChatID: 7})
	checkSelection(t, "select chat", selected, 0)

	selected = base()
	updateState(&selected, ActionReceived{Action: SelectPrevious})
	checkSelection(t, "chat navigation", selected, 1)

	updateState(&selected, ActionReceived{Action: OpenChat})
	if selected.Focus != FocusChats || !selected.ShowAll[7] {
		t.Fatalf("explicit open: focus=%v showAll=%#v", selected.Focus, selected.ShowAll)
	}
}
