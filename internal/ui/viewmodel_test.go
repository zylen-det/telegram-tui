package ui

import (
	"errors"
	"image/color"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

func TestSelectPublishesHistoryErrorWithoutCause(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Title: "Chat"}}
	state.SelectedChat = 0
	state.History[9] = app.HistoryState{Error: &domain.AppError{Kind: domain.ErrorNetwork, Message: "offline", Cause: errors.New("raw")}}
	model := Select(state, time.UTC)
	if model.HistoryError == nil || model.HistoryError.Message != "offline" || model.HistoryError.Cause != nil {
		t.Fatalf("HistoryError = %#v", model.HistoryError)
	}
}

func TestLiveHistoryViewModelPublishesLoadingAndDone(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.History[9] = app.HistoryState{Loading: true, Done: true}
	model := Select(state, time.UTC)
	if !model.HistoryLoading || !model.HistoryDone {
		t.Fatalf("history lifecycle = loading %t, done %t", model.HistoryLoading, model.HistoryDone)
	}
}

func TestForwardPickerProjectionPassesThroughExactFields(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Title: "Source"}, {ID: 10, Title: "Dest"}}
	state.ForwardPicker = &app.ForwardPicker{SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1, RequestID: 42}
	model := Select(state, time.UTC)
	if model.ForwardPicker == nil || model.ForwardPicker.SourceChatID != 9 || model.ForwardPicker.SourceMessageID != 2 || model.ForwardPicker.SelectedChat != 1 || model.ForwardPicker.RequestID != 42 {
		t.Fatalf("picker projection = %#v", model.ForwardPicker)
	}
	model.ForwardPicker.SelectedChat = 5
	model.ForwardPicker.RequestID = 99
	if state.ForwardPicker.SelectedChat != 1 || state.ForwardPicker.RequestID != 42 {
		t.Fatal("picker projection mutated state")
	}
	if model.ForwardPicker == nil || model.ForwardPicker.SelectedChat != 5 {
		t.Fatal("picker projection did not own its copy")
	}
}

func TestSelectBuildsRenderModel(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatPrivate}}
	state.SelectedChat = 0
	target := domain.Message{ID: 1, ChatID: 9, SenderName: "opaque-sender", Kind: domain.MessageText, Text: "opaque-preview"}
	reply := domain.Message{ID: 2, ChatID: 9, HasReply: true, ReplyToMessageID: 1, Kind: domain.MessageText, Text: "opaque-body"}
	state.Messages[9] = []domain.Message{target, reply}
	model := Select(state, time.UTC)
	context := model.Groups[1].ReplyContexts[2]
	if !context.Available {
		t.Fatal("reply context unavailable in render model")
	}
	if context.Sender != "opaque-sender" {
		t.Fatal("reply context sender mismatch")
	}
	if context.Preview != "opaque-preview" {
		t.Fatal("reply context preview mismatch")
	}
}

func TestPinnedMarkerMenuProjectionSurvivesSelect(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "private", Pinned: true}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &app.MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}, Pinned: true}
	model := Select(state, time.UTC)
	if model.MessageMenu == nil || !model.MessageMenu.Pinned {
		t.Fatal("view model lost menu pinned state")
	}
	model.MessageMenu.Pinned = false
	if !state.MessageMenu.Pinned {
		t.Fatal("view model mutated menu state")
	}
}

func TestMessageActionMenuProjectionExposesOnlyCopyCapability(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "private"}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 2
	state.MessageMenu = &app.MessageActionMenu{ChatID: 9, MessageID: 2, Capabilities: domain.MessageCapabilities{Copy: true}}
	model := Select(state, time.UTC)
	if model.SelectedMessageChat != 9 || model.SelectedMessage != 2 || model.MessageMenu == nil || !model.MessageMenu.Capabilities.Copy {
		t.Fatalf("message interaction projection = chat:%d message:%d menu:%#v", model.SelectedMessageChat, model.SelectedMessage, model.MessageMenu)
	}
}

func TestSelectBuildsRenderModelContinuation(t *testing.T) {
	t.Parallel()

	state := app.InitialState()
	state.Width, state.Height = 140, 30
	state.Layout = app.LayoutTooSmall
	state.Focus = app.FocusDetails
	state.Connection = domain.ConnectionReconnecting
	state.DetailsOpen = true
	state.SelectedChat = 1
	state.Chats = []domain.Chat{
		{ID: 7, Kind: domain.ChatPrivate, Title: "Mina", Avatar: domain.AvatarRef{UniqueID: "mina-avatar"}},
		{ID: 9, Kind: domain.ChatSupergroup, Title: "Weekend", Avatar: domain.AvatarRef{UniqueID: "weekend-avatar"}, CanSend: true},
	}
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 3}, SenderName: "Iris", SenderAvatar: domain.AvatarRef{UniqueID: "iris-avatar"}, SentAt: time.Unix(100, 0), Kind: domain.MessageText, Text: "hello"},
		{ID: 2, ChatID: 9, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 3}, SenderName: "Iris", SenderAvatar: domain.AvatarRef{UniqueID: "iris-avatar"}, SentAt: time.Unix(160, 0), Kind: domain.MessageText, Text: "again"},
	}
	state.Drafts[9] = "draft"
	state.DraftReplies[9] = 2
	state.DraftDates[9] = 123
	state.History[9] = app.HistoryState{ViewOffset: 3, FollowSelection: true}
	state.ChatsLoading = true
	state.ChatsLoaded = true
	state.ChatsError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "refreshing cached chats"}
	state.Avatars["mina-avatar:chat-list"] = app.AvatarState{Cells: testAvatar('M')}
	state.Avatars["weekend-avatar:chat-list"] = app.AvatarState{Cells: testAvatar('W')}
	state.Avatars["iris-avatar:message-group"] = app.AvatarState{Cells: testAvatar('I')}

	got := Select(state, time.UTC)

	if got.Width != 140 || got.Height != 30 {
		t.Fatalf("dimensions = %dx%d, want 140x30", got.Width, got.Height)
	}
	wantLayout := ComputeLayout(140, 30, true, app.FocusDetails)
	if got.Layout != wantLayout {
		t.Fatalf("Layout = %#v, want freshly computed %#v", got.Layout, wantLayout)
	}
	if got.Focus != app.FocusDetails || got.Connection != domain.ConnectionReconnecting || !got.DetailsOpen {
		t.Fatalf("top-level model = %#v", got)
	}
	if len(got.Chats) != 2 {
		t.Fatalf("Chats length = %d, want 2", len(got.Chats))
	}
	if got.Chats[0].Selected || !got.Chats[1].Selected {
		t.Fatalf("selected rows = [%t, %t], want [false, true]", got.Chats[0].Selected, got.Chats[1].Selected)
	}
	if got.Chats[0].AvatarKey != "mina-avatar:chat-list" || got.Chats[0].Avatar.Cells[0].Rune != 'M' {
		t.Fatalf("first chat avatar = (%q, %#v)", got.Chats[0].AvatarKey, got.Chats[0].Avatar)
	}
	if got.Chats[1].AvatarKey != "weekend-avatar:chat-list" || got.Chats[1].Avatar.Cells[0].Rune != 'W' {
		t.Fatalf("second chat avatar = (%q, %#v)", got.Chats[1].AvatarKey, got.Chats[1].Avatar)
	}
	if got.Chats[1].Draft != (domain.Draft{Text: "draft", ReplyToMessageID: 2, Date: 123}) {
		t.Fatalf("chat-row draft = %#v", got.Chats[1].Draft)
	}
	if got.ActiveChat.Title != "Weekend" || got.Draft != "draft" {
		t.Fatalf("active chat and draft = (%#v, %q)", got.ActiveChat, got.Draft)
	}
	if got.HistoryOffset != 3 || !got.HistoryFollowSelection {
		t.Fatalf("history viewport = offset:%d follow:%t, want 3/true", got.HistoryOffset, got.HistoryFollowSelection)
	}
	if !got.ChatsLoading || !got.ChatsLoaded || got.ChatsError == nil || got.ChatsError.Message != "refreshing cached chats" {
		t.Fatalf("chat load projection = (%t, %t, %#v)", got.ChatsLoading, got.ChatsLoaded, got.ChatsError)
	}
	if len(got.Groups) != 1 || !got.Groups[0].ShowAvatar || len(got.Groups[0].Messages) != 2 {
		t.Fatalf("Groups = %#v, want one two-message avatar group", got.Groups)
	}
	if got.Groups[0].AvatarKey != "iris-avatar:message-group" || got.Groups[0].Avatar.Cells[0].Rune != 'I' {
		t.Fatalf("group avatar = (%q, %#v)", got.Groups[0].AvatarKey, got.Groups[0].Avatar)
	}
}

func TestSelectInvalidSelectedChatLeavesConversationEmpty(t *testing.T) {
	t.Parallel()

	for _, selected := range []int{-1, 2} {
		t.Run(selectedName(selected), func(t *testing.T) {
			t.Parallel()

			state := app.InitialState()
			state.SelectedChat = selected
			state.Chats = []domain.Chat{{ID: 7, Title: "Mina"}, {ID: 9, Title: "Weekend"}}
			state.Messages[7] = []domain.Message{{ID: 1, ChatID: 7, Text: "hidden"}}
			state.Drafts[7] = "hidden draft"

			got := Select(state, time.UTC)

			for index, row := range got.Chats {
				if row.Selected {
					t.Errorf("Chats[%d].Selected = true for invalid selection %d", index, selected)
				}
			}
			if got.ActiveChat != (domain.Chat{}) || len(got.Groups) != 0 || got.Draft != "" {
				t.Fatalf("conversation = (%#v, %#v, %q), want empty", got.ActiveChat, got.Groups, got.Draft)
			}
		})
	}
}

func TestSelectNilLocationFallsBackSafely(t *testing.T) {
	t.Parallel()

	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatBasicGroup}}
	start := time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC)
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9, Sender: domain.SenderRef{ID: 3}, SentAt: start},
		{ID: 2, ChatID: 9, Sender: domain.SenderRef{ID: 3}, SentAt: start.Add(time.Minute)},
	}

	got := Select(state, nil)
	if len(got.Groups) != 1 {
		t.Fatalf("Groups length = %d, want 1", len(got.Groups))
	}
}

func TestInitialStateInitializesAvatars(t *testing.T) {
	t.Parallel()

	state := app.InitialState()
	if state.Avatars == nil {
		t.Fatal("Avatars = nil, want non-nil empty map")
	}
	state.Avatars["key"] = app.AvatarState{Loading: true}
}

func TestSelectReturnsMutationIsolatedSnapshot(t *testing.T) {
	t.Parallel()

	state := app.InitialState()
	state.Width, state.Height = 100, 22
	state.Chats = []domain.Chat{{
		ID:     9,
		Kind:   domain.ChatSupergroup,
		Title:  "Original chat",
		Avatar: domain.AvatarRef{UniqueID: "chat-avatar"},
	}}
	messageCause := &mutableCause{message: "original message cause"}
	modalCause := &mutableCause{message: "original modal cause"}
	toastCause := &mutableCause{message: "original toast cause"}
	chatAvatarCause := &mutableCause{message: "original chat avatar cause"}
	groupAvatarCause := &mutableCause{message: "original group avatar cause"}
	messageFailure := &domain.AppError{Kind: domain.ErrorPermission, Op: "send", Message: "original message failure", Cause: messageCause}
	state.Messages[9] = []domain.Message{{
		ID:           1,
		ChatID:       9,
		Sender:       domain.SenderRef{Kind: domain.SenderUser, ID: 3},
		SenderName:   "Iris",
		SenderAvatar: domain.AvatarRef{UniqueID: "sender-avatar"},
		SentAt:       time.Date(2026, time.February, 2, 8, 0, 0, 0, time.UTC),
		Kind:         domain.MessageText,
		Text:         "original message",
		Failure:      messageFailure,
	}}
	state.Prompt = &app.PromptState{
		Prompt:        auth.Prompt{ID: 5, Kind: auth.PromptPassword, Label: "Original prompt", Secret: true},
		Input:         []rune("secret"),
		PreviousFocus: app.FocusConversation,
	}
	state.Modal = &app.ModalState{
		Title: "Original modal",
		Error: &domain.AppError{Kind: domain.ErrorMedia, Op: "download", Message: "original modal error", Cause: modalCause},
	}
	state.Toast = &domain.AppError{Kind: domain.ErrorNetwork, Op: "receive", Message: "original toast", Cause: toastCause}
	state.Avatars["chat-avatar:chat-list"] = app.AvatarState{
		Cells: testAvatar('C'),
		Error: &domain.AppError{Kind: domain.ErrorMedia, Message: "original chat avatar error", Cause: chatAvatarCause},
	}
	state.Avatars["sender-avatar:message-group"] = app.AvatarState{
		Cells: testAvatar('G'),
		Error: &domain.AppError{Kind: domain.ErrorStorage, Message: "original group avatar error", Cause: groupAvatarCause},
	}

	model := Select(state, time.UTC)
	for _, test := range []struct {
		name  string
		cause error
	}{
		{name: "modal", cause: model.Modal.Error.Cause},
		{name: "toast", cause: model.Toast.Cause},
		{name: "chat avatar", cause: model.Chats[0].AvatarError.Cause},
		{name: "group avatar", cause: model.Groups[0].AvatarError.Cause},
		{name: "message failure", cause: model.Groups[0].Messages[0].Failure.Cause},
	} {
		if test.cause != nil {
			t.Errorf("%s render error Cause = %v, want nil", test.name, test.cause)
		}
	}

	model.Prompt.Prompt.Label = "Changed prompt"
	model.Prompt.Input[0] = 'X'
	model.Modal.Title = "Changed modal"
	model.Modal.Error.Message = "changed modal error"
	model.Toast.Message = "changed toast"
	model.Chats[0].Chat.Title = "Changed chat"
	model.Chats[0].Avatar.Cells[0].Rune = 'X'
	model.Chats[0].AvatarError.Message = "changed chat avatar error"
	model.Groups[0].Avatar.Cells[0].Rune = 'Y'
	model.Groups[0].AvatarError.Message = "changed group avatar error"
	model.Groups[0].Messages[0].Text = "changed message"
	model.Groups[0].Messages[0].Failure.Message = "changed message failure"

	if state.Prompt.Prompt.Label != "Original prompt" || string(state.Prompt.Input) != "secret" {
		t.Errorf("prompt mutated through model: %#v", state.Prompt)
	}
	if state.Modal.Title != "Original modal" || state.Modal.Error.Message != "original modal error" {
		t.Errorf("modal mutated through model: %#v", state.Modal)
	}
	if state.Toast.Message != "original toast" {
		t.Errorf("toast mutated through model: %#v", state.Toast)
	}
	if state.Chats[0].Title != "Original chat" {
		t.Errorf("chat mutated through model: %#v", state.Chats[0])
	}
	chatAvatar := state.Avatars["chat-avatar:chat-list"]
	if chatAvatar.Cells.Cells[0].Rune != 'C' || chatAvatar.Error.Message != "original chat avatar error" {
		t.Errorf("chat avatar mutated through model: %#v", chatAvatar)
	}
	groupAvatar := state.Avatars["sender-avatar:message-group"]
	if groupAvatar.Cells.Cells[0].Rune != 'G' || groupAvatar.Error.Message != "original group avatar error" {
		t.Errorf("group avatar mutated through model: %#v", groupAvatar)
	}
	if got := state.Messages[9][0]; got.Text != "original message" || got.Failure.Message != "original message failure" {
		t.Errorf("message mutated through model: %#v", got)
	}
	for _, test := range []struct {
		name    string
		got     error
		want    *mutableCause
		message string
	}{
		{name: "modal", got: state.Modal.Error.Cause, want: modalCause, message: "original modal cause"},
		{name: "toast", got: state.Toast.Cause, want: toastCause, message: "original toast cause"},
		{name: "chat avatar", got: chatAvatar.Error.Cause, want: chatAvatarCause, message: "original chat avatar cause"},
		{name: "group avatar", got: groupAvatar.Error.Cause, want: groupAvatarCause, message: "original group avatar cause"},
		{name: "message failure", got: state.Messages[9][0].Failure.Cause, want: messageCause, message: "original message cause"},
	} {
		if test.got != test.want || test.want.message != test.message {
			t.Errorf("state %s Cause = (%v, %q), want original pointer with %q", test.name, test.got, test.want.message, test.message)
		}
	}
}

type mutableCause struct {
	message string
}

func (cause *mutableCause) Error() string {
	return cause.message
}

func testAvatar(r rune) pixel.Avatar {
	return pixel.Avatar{
		Width:  1,
		Height: 1,
		Cells: []pixel.Cell{{
			Rune:       r,
			Foreground: color.NRGBA{R: 1, G: 2, B: 3, A: 255},
			Background: color.NRGBA{R: 4, G: 5, B: 6, A: 255},
		}},
	}
}

func TestSelectPhotoSend(t *testing.T) {
	// Nil PhotoSend passes through as nil.
	state := app.InitialState()
	state.SelectedChat = 0
	state.Chats = []domain.Chat{{ID: 1, Title: "Chat"}}
	model := Select(state, time.UTC)
	if model.PhotoSend != nil {
		t.Fatalf("PhotoSend = %v, want nil", model.PhotoSend)
	}

	// Fields survive projection.
	state.PhotoSend = &app.PhotoSendState{
		ChatID:        42,
		Input:         []rune("/tmp/photo.jpg"),
		PreviousFocus: app.FocusConversation,
	}
	model = Select(state, time.UTC)
	if model.PhotoSend == nil {
		t.Fatal("PhotoSend nil, want non-nil")
	}
	if model.PhotoSend.ChatID != 42 {
		t.Errorf("ChatID = %d, want 42", model.PhotoSend.ChatID)
	}
	if string(model.PhotoSend.Input) != "/tmp/photo.jpg" {
		t.Errorf("Input = %q, want /tmp/photo.jpg", string(model.PhotoSend.Input))
	}
	if model.PhotoSend.PreviousFocus != app.FocusConversation {
		t.Errorf("PreviousFocus = %v, want %v", model.PhotoSend.PreviousFocus, app.FocusConversation)
	}

	// Invalid selected chat: PhotoSend still selected and cloned.
	state.SelectedChat = -1
	model = Select(state, time.UTC)
	if model.PhotoSend == nil {
		t.Fatal("invalid chat: PhotoSend nil, want non-nil")
	}
	if string(model.PhotoSend.Input) != "/tmp/photo.jpg" {
		t.Errorf("invalid chat: Input = %q, want /tmp/photo.jpg", string(model.PhotoSend.Input))
	}

	// Bidirectional mutation resistance.
	state.SelectedChat = 0
	state.PhotoSend = &app.PhotoSendState{
		ChatID: 99,
		Input:  []rune("original"),
	}
	model = Select(state, time.UTC)
	model.PhotoSend.Input[0] = 'X'
	model.PhotoSend.ChatID = 77
	if state.PhotoSend.Input[0] != 'o' || string(state.PhotoSend.Input) != "original" {
		t.Errorf("state mutated through model: Input=%q", string(state.PhotoSend.Input))
	}
	if state.PhotoSend.ChatID != 99 {
		t.Errorf("state mutated through model: ChatID=%d", state.PhotoSend.ChatID)
	}
	state.PhotoSend.Input[0] = 'Y'
	state.PhotoSend.ChatID = 11
	// Model should retain its own copy: Input[0]='X' (set above), ChatID=77.
	if model.PhotoSend.Input[0] != 'X' {
		t.Errorf("model mutated through state: Input[0]=%c, want X", model.PhotoSend.Input[0])
	}
	if model.PhotoSend.ChatID != 77 {
		t.Errorf("model mutated through state: ChatID=%d, want 77", model.PhotoSend.ChatID)
	}
}

func selectedName(selected int) string {
	if selected < 0 {
		return "negative"
	}
	return "past end"
}

func forumTopicState() app.State {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, IsForum: true, Title: "Forum"}}
	state.SelectedChat = 0
	return state
}

func forumMessage(id domain.MessageID, topicID domain.TopicID, text string) domain.Message {
	return domain.Message{ID: id, ChatID: 9, TopicID: topicID, Sender: domain.SenderRef{Kind: domain.SenderUser, ID: 3}, SenderName: "Iris", SentAt: time.Unix(100, 0), Kind: domain.MessageText, Text: text}
}

func TestSelectClonesTopicListState(t *testing.T) {
	state := forumTopicState()
	state.Topics = &app.TopicListState{
		RequestID:  7,
		ChatID:     9,
		Loading:    true,
		Results:    []domain.ForumTopic{{ID: 1, Name: "General"}, {ID: 2, Name: "Announcements"}},
		TotalCount: 2,
	}

	model := Select(state, time.UTC)

	if model.Topics == nil {
		t.Fatal("Topics = nil, want projected topic list")
	}
	if model.Topics.RequestID != 7 || model.Topics.ChatID != 9 || !model.Topics.Loading || model.Topics.TotalCount != 2 {
		t.Fatalf("Topics projection = %#v", model.Topics)
	}
	if model.Topics.Results[1].Name != "Announcements" {
		t.Fatalf("Topics.Results = %#v", model.Topics.Results)
	}

	// Mutating the model's copy must not leak into state.
	model.Topics.Results[0].Name = "changed"
	model.Topics.TotalCount = 99
	model.Topics.Loading = false
	if state.Topics.Results[0].Name != "General" || state.Topics.TotalCount != 2 || !state.Topics.Loading {
		t.Fatalf("state.Topics mutated through model: %#v", state.Topics)
	}
	if model.Topics.Results[0].Name != "changed" || model.Topics.TotalCount != 99 || model.Topics.Loading {
		t.Fatalf("model.Topics did not own its copy: %#v", model.Topics)
	}
}

func TestSelectNilTopicsProjectsNil(t *testing.T) {
	state := forumTopicState()
	state.Topics = nil
	model := Select(state, time.UTC)
	if model.Topics != nil {
		t.Fatalf("Topics = %#v, want nil", model.Topics)
	}
}

func TestSelectResolvesActiveTopic(t *testing.T) {
	state := forumTopicState()
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{
		1: {ID: 1, ChatID: 9, Name: "General", IsGeneral: true},
		2: {ID: 2, ChatID: 9, Name: "Announcements"},
	}
	state.SelectedTopics[9] = 2
	state.Messages[9] = []domain.Message{
		forumMessage(1, 1, "general body"),
		forumMessage(2, 2, "announcement body"),
	}

	model := Select(state, time.UTC)

	if !model.ActiveTopicKnown {
		t.Fatal("ActiveTopicKnown = false, want true")
	}
	if model.ActiveTopic.ID != 2 || model.ActiveTopic.Name != "Announcements" {
		t.Fatalf("ActiveTopic = %#v, want topic 2 Announcements", model.ActiveTopic)
	}
	if len(model.Groups) != 1 {
		t.Fatalf("Groups = %#v, want only the active topic's message", model.Groups)
	}
	if model.Groups[0].Messages[0].ID != 2 || model.Groups[0].Messages[0].Text != "announcement body" {
		t.Fatalf("Groups filtered = %#v, want only topic 2 message", model.Groups[0].Messages)
	}
}

func TestSelectUnknownSelectedTopicIsNotKnown(t *testing.T) {
	state := forumTopicState()
	// Selected topic has no entry in ForumTopics[9]; it is not "known".
	state.SelectedTopics[9] = 99
	state.Messages[9] = []domain.Message{forumMessage(1, 99, "orphan")}

	model := Select(state, time.UTC)

	if model.ActiveTopicKnown {
		t.Fatalf("ActiveTopicKnown = true, want false for missing forum topic")
	}
	if model.ActiveTopic != (domain.ForumTopic{}) {
		t.Fatalf("ActiveTopic = %#v, want zero", model.ActiveTopic)
	}
}

func TestSelectPlainChatGoldenUnchanged(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, Title: "Plain"}}
	state.SelectedChat = 0
	state.Messages[9] = []domain.Message{
		{ID: 1, ChatID: 9, Sender: domain.SenderRef{ID: 3}, SentAt: time.Unix(100, 0), Kind: domain.MessageText, Text: "all"},
		{ID: 2, ChatID: 9, Sender: domain.SenderRef{ID: 3}, SentAt: time.Unix(160, 0), Kind: domain.MessageText, Text: "all 2"},
	}
	state.Drafts[9] = "plain draft"
	state.History[9] = app.HistoryState{ViewOffset: 3, Done: true}

	model := Select(state, time.UTC)

	if model.Topics != nil || model.ActiveTopicKnown || model.ActiveTopic != (domain.ForumTopic{}) {
		t.Fatalf("plain chat topic projection = (%#v, %t, %#v)", model.Topics, model.ActiveTopicKnown, model.ActiveTopic)
	}
	if len(model.Groups) != 1 || len(model.Groups[0].Messages) != 2 {
		t.Fatalf("plain Groups = %#v, want both messages grouped", model.Groups)
	}
	if model.Draft != "plain draft" {
		t.Fatalf("plain Draft = %q, want chat-level draft", model.Draft)
	}
	if model.HistoryOffset != 3 || !model.HistoryDone {
		t.Fatalf("plain history = (%d, %t), want chat-level 3/true", model.HistoryOffset, model.HistoryDone)
	}
}

func TestSelectTopicDraftAndHistory(t *testing.T) {
	state := forumTopicState()
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{1: {ID: 1, ChatID: 9, Name: "General"}}
	state.SelectedTopics[9] = 1
	state.Drafts[9] = "chat-level draft (must be ignored)"
	state.History[9] = app.HistoryState{ViewOffset: 99}
	state.TopicDrafts[app.TopicKey{ChatID: 9, TopicID: 1}] = "topic draft"
	state.TopicHistory[app.TopicKey{ChatID: 9, TopicID: 1}] = app.HistoryState{ViewOffset: 4, FollowSelection: true, Done: true}

	model := Select(state, time.UTC)

	if !model.ActiveTopicKnown {
		t.Fatal("ActiveTopicKnown = false, want true")
	}
	if model.Draft != "topic draft" {
		t.Fatalf("topic Draft = %q, want topic draft (not chat-level)", model.Draft)
	}
	if model.HistoryOffset != 4 || !model.HistoryFollowSelection || !model.HistoryDone {
		t.Fatalf("topic history = (%d, %t, %t), want 4/true/true from TopicHistory", model.HistoryOffset, model.HistoryFollowSelection, model.HistoryDone)
	}
}

func TestSelectTopicDraftZeroHistoryWhenAbsent(t *testing.T) {
	state := forumTopicState()
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{1: {ID: 1, ChatID: 9, Name: "General"}}
	state.SelectedTopics[9] = 1
	state.History[9] = app.HistoryState{ViewOffset: 99, Done: true}
	// No TopicDrafts / TopicHistory entries for the topic.

	model := Select(state, time.UTC)

	if !model.ActiveTopicKnown {
		t.Fatal("ActiveTopicKnown = false, want true")
	}
	if model.Draft != "" {
		t.Fatalf("topic Draft = %q, want empty (absent TopicDrafts entry)", model.Draft)
	}
	if model.HistoryOffset != 0 || model.HistoryDone {
		t.Fatalf("topic history = (%d, %t), want zero HistoryState when absent", model.HistoryOffset, model.HistoryDone)
	}
}

func TestSelectShowAllRoutesDraftAndChatHistory(t *testing.T) {
	state := forumTopicState()
	state.ShowAll[9] = true
	state.SelectedTopics[9] = 1 // stale value; ALL mode ignores it
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{1: {ID: 1, ChatID: 9, Name: "General"}}
	state.Messages[9] = []domain.Message{
		forumMessage(1, 1, "general body"),
		forumMessage(2, 2, "other topic body"),
	}
	// Distinct senders keep the two topics' messages in separate groups, so
	// ALL mode visibly surfaces both (topic mode would show only one).
	state.Messages[9][0].Sender = domain.SenderRef{Kind: domain.SenderUser, ID: 3}
	state.Messages[9][1].Sender = domain.SenderRef{Kind: domain.SenderUser, ID: 4}
	state.Messages[9][1].SenderName = "Rhea"
	state.Drafts[9] = "chat-level draft (must be ignored)"
	state.TopicDrafts[app.TopicKey{ChatID: 9, TopicID: 0}] = "ALL draft"
	state.History[9] = app.HistoryState{ViewOffset: 5, Done: true}
	state.TopicHistory[app.TopicKey{ChatID: 9, TopicID: 1}] = app.HistoryState{ViewOffset: 42}

	model := Select(state, time.UTC)

	if !model.ShowAllTopics {
		t.Fatal("ShowAllTopics = false, want true")
	}
	// ALL mode keeps ActiveTopicKnown false so history/draft use the
	// chat-level paths, even with a stale SelectedTopics entry.
	if model.ActiveTopicKnown {
		t.Fatal("ActiveTopicKnown = true, want false in ALL mode")
	}
	if model.ActiveTopic != (domain.ForumTopic{}) {
		t.Fatalf("ActiveTopic = %#v, want zero in ALL mode", model.ActiveTopic)
	}
	if model.Draft != "ALL draft" {
		t.Fatalf("ALL Draft = %q, want TopicDrafts[{9,0}]", model.Draft)
	}
	if model.HistoryOffset != 5 || !model.HistoryDone {
		t.Fatalf("ALL history = (%d, %t), want chat-level 5/true", model.HistoryOffset, model.HistoryDone)
	}
	if len(model.Groups) != 2 {
		t.Fatalf("ALL Groups = %d, want all messages grouped", len(model.Groups))
	}
}

func TestSelectShowAllEditTargetOverridesDraft(t *testing.T) {
	state := forumTopicState()
	state.ShowAll[9] = true
	state.TopicDrafts[app.TopicKey{ChatID: 9, TopicID: 0}] = "ALL draft"
	state.EditTarget = &app.EditTarget{ChatID: 9, Buffer: "editing buffer"}

	model := Select(state, time.UTC)

	if model.Draft != "editing buffer" {
		t.Fatalf("ALL Draft = %q, want edit-target buffer override", model.Draft)
	}
}

func TestSelectShowAllFalseForOrdinaryForums(t *testing.T) {
	state := forumTopicState()
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{1: {ID: 1, ChatID: 9, Name: "General"}}
	state.SelectedTopics[9] = 1
	state.TopicDrafts[app.TopicKey{ChatID: 9, TopicID: 1}] = "topic draft"

	model := Select(state, time.UTC)

	if model.ShowAllTopics {
		t.Fatal("ShowAllTopics = true, want false for a topic-selected forum")
	}
	if !model.ActiveTopicKnown {
		t.Fatal("ActiveTopicKnown = false, want true")
	}
	if model.Draft != "topic draft" {
		t.Fatalf("topic Draft = %q, want topic draft (not chat-level)", model.Draft)
	}

}

func TestInlineThumbnailsProjectedForActiveChat(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Title: "Chat"}}
	state.SelectedChat = 0
	state.Thumbnails[9] = map[domain.MessageID]thumbnail.Block{
		42: {Text: "\x1b[38;5;196mHI\x1b[0m", Width: 20, Height: 8},
	}
	model := Select(state, time.UTC)
	if len(model.InlineThumbnails) != 1 {
		t.Fatalf("InlineThumbnails length = %d, want 1", len(model.InlineThumbnails))
	}
	item := model.InlineThumbnails[0]
	if item.ChatID != 9 {
		t.Errorf("ChatID = %d, want 9", item.ChatID)
	}
	if item.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", item.MessageID)
	}
	if item.Block.Text != "\x1b[38;5;196mHI\x1b[0m" {
		t.Errorf("Block.Text = %q, want ANSI red HI", item.Block.Text)
	}
	if item.Block.Width != 20 {
		t.Errorf("Block.Width = %d, want 20", item.Block.Width)
	}
	if item.Block.Height != 8 {
		t.Errorf("Block.Height = %d, want 8", item.Block.Height)
	}
}

func TestInlineThumbnailsEmptyWhenStateHasNone(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{{ID: 9, Title: "Chat"}}
	state.SelectedChat = 0
	model := Select(state, time.UTC)
	if model.InlineThumbnails != nil {
		t.Fatalf("InlineThumbnails = %#v, want nil", model.InlineThumbnails)
	}
}

func TestInlineThumbnailsScopedToActiveChat(t *testing.T) {
	state := app.InitialState()
	state.Chats = []domain.Chat{
		{ID: 10, Title: "ChatA"},
		{ID: 20, Title: "ChatB"},
	}
	state.SelectedChat = 0 // ChatA
	state.Thumbnails[10] = map[domain.MessageID]thumbnail.Block{
		1: {Text: "a", Width: 20, Height: 8},
	}
	state.Thumbnails[20] = map[domain.MessageID]thumbnail.Block{
		2: {Text: "b", Width: 20, Height: 8},
	}
	model := Select(state, time.UTC)
	if len(model.InlineThumbnails) != 1 {
		t.Fatalf("InlineThumbnails length = %d, want 1", len(model.InlineThumbnails))
	}
	if model.InlineThumbnails[0].ChatID != 10 {
		t.Errorf("ChatID = %d, want 10 (scoped to active)", model.InlineThumbnails[0].ChatID)
	}
}
