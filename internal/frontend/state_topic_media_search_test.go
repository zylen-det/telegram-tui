package frontend

import (
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func topicMediaBaseState(t *testing.T) State {
	t.Helper()
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.SelectedMessageChat = 7
	state.SelectedMessage = 0
	state.NextRequestID = 10
	state.ForumTopics[7] = map[domain.TopicID]domain.ForumTopic{101: {ID: 101, ChatID: 7, Name: "General"}}
	state.SelectedTopics[7] = 101
	state.Messages[7] = []domain.Message{{ID: 5, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "seed"}}
	return state
}

func forumPhotoSendPath(t *testing.T, state State, path string) (State, []Effect) {
	t.Helper()
	state, _ = updateState(state, ActionReceived{Action: OpenPhotoSend})
	if state.PhotoSend == nil {
		t.Fatalf("photo send not opened")
	}
	state.PhotoSend.Input = []rune(path)
	state, _ = updateState(state, ActionReceived{Action: PhotoSendSubmit, At: time.Unix(100, 0)})
	return state, nil
}

func TestPhotoSendOpenCapturesActiveTopic(t *testing.T) {
	state := topicMediaBaseState(t)
	opened, _ := updateState(state, ActionReceived{Action: OpenPhotoSend})
	if opened.PhotoSend == nil {
		t.Fatal("photo send not opened")
	}
	if opened.PhotoSend.ChatID != 7 || opened.PhotoSend.TopicID != 101 {
		t.Fatalf("photo send = %#v, want chat 7 topic 101", opened.PhotoSend)
	}

	// Ordinary chat keeps TopicID 0.
	plain := InitialState()
	plain.Connection = domain.ConnectionOnline
	plain.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	plain.SelectedChat = 0
	plain, _ = updateState(plain, ActionReceived{Action: OpenPhotoSend})
	if plain.PhotoSend == nil || plain.PhotoSend.TopicID != 0 {
		t.Fatalf("ordinary chat photo send = %#v, want TopicID 0", plain.PhotoSend)
	}
}

func TestPhotoSendSubmitCarriesTopicID(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	state := topicMediaBaseState(t)
	state, _ = updateState(state, ActionReceived{Action: OpenPhotoSend})
	state.PhotoSend.Input = []rune("/p.jpg")
	submitted, cmds := updateState(state, ActionReceived{Action: PhotoSendSubmit, At: now})
	if len(cmds) != 2 {
		t.Fatalf("cmds = %#v", cmds)
	}
	cmd, ok := cmds[0].(SendPhoto)
	if !ok || cmd.TopicID != 101 || cmd.ChatID != 7 {
		t.Fatalf("send photo = %#v", cmds[0])
	}
	if submitted.PhotoSend != nil {
		t.Fatal("photo send not closed")
	}
	var msg domain.Message
	for _, m := range submitted.Messages[7] {
		if m.Outgoing && m.SendState == domain.SendPending {
			msg = m
		}
	}
	if msg.TopicID != 101 {
		t.Fatalf("optimistic message topic = %d, want 101", msg.TopicID)
	}
	_ = now
}

func TestMediaSendSubmitCarriesTopicIDForVideoAudioDocument(t *testing.T) {
	tests := []struct {
		name string
		path string
		want func(Effect) bool
	}{
		{name: "video", path: "/v.mp4", want: func(c Effect) bool { _, ok := c.(SendVideo); return ok }},
		{name: "audio", path: "/a.mp3", want: func(c Effect) bool { _, ok := c.(SendAudio); return ok }},
		{name: "document", path: "/d.pdf", want: func(c Effect) bool { _, ok := c.(SendDocument); return ok }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := topicMediaBaseState(t)
			state, _ = updateState(state, ActionReceived{Action: OpenPhotoSend})
			state.PhotoSend.Input = []rune(tc.path)
			submitted, cmds := updateState(state, ActionReceived{Action: PhotoSendSubmit, At: time.Unix(100, 0)})
			if len(cmds) == 0 || !tc.want(cmds[0]) {
				t.Fatalf("cmds = %#v", cmds)
			}
			var topic domain.TopicID
			switch c := cmds[0].(type) {
			case SendVideo:
				topic = c.TopicID
			case SendAudio:
				topic = c.TopicID
			case SendDocument:
				topic = c.TopicID
			}
			if topic != 101 {
				t.Fatalf("topic = %d, want 101", topic)
			}
			if submitted.PhotoSend != nil {
				t.Fatal("photo send not closed")
			}
		})
	}
}

func TestStickerPickerOpenCapturesTopicAndLoadCarriesTopic(t *testing.T) {

	state := topicMediaBaseState(t)
	state.Focus = FocusComposer
	opened, cmds := updateState(state, ActionReceived{Action: OpenStickerPicker})
	if opened.StickerPicker == nil {
		t.Fatal("sticker picker not opened")
	}
	if opened.StickerPicker.ChatID != 7 || opened.StickerPicker.TopicID != 101 {
		t.Fatalf("picker = %#v", opened.StickerPicker)
	}
	if len(cmds) != 1 {
		t.Fatalf("cmds = %#v", cmds)
	}
	cmd, ok := cmds[0].(LoadStickers)
	if !ok || cmd.ChatID != 7 || cmd.TopicID != 101 {
		t.Fatalf("load stickers = %#v", cmds[0])
	}
}

func TestStickerSubmitCarriesTopicID(t *testing.T) {
	state := topicMediaBaseState(t)
	state.Focus = FocusComposer
	opened, _ := updateState(state, ActionReceived{Action: OpenStickerPicker})
	state = opened
	state, _ = updateState(state, StickersLoaded{RequestID: state.StickerPicker.RequestID, ChatID: 7, Stickers: []domain.StickerRef{{File: domain.MediaFileRef{ID: 3}}}})
	submitted, cmds := updateState(state, ActionReceived{Action: StickerActivate, At: time.Unix(100, 0), StickerFileID: 3, RequestID: state.StickerPicker.RequestID})
	if len(cmds) != 1 {
		t.Fatalf("cmds = %#v", cmds)
	}
	cmd, ok := cmds[0].(SendSticker)
	if !ok || cmd.TopicID != 101 || cmd.ChatID != 7 {
		t.Fatalf("send sticker = %#v", cmds[0])
	}
	var msg domain.Message
	for _, m := range submitted.Messages[7] {
		if m.Kind == domain.MessageSticker {
			msg = m
		}
	}
	if msg.TopicID != 101 {
		t.Fatalf("sticker message topic = %d, want 101", msg.TopicID)
	}
}

func TestRetryMessageCarriesMessageTopicID(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	kinds := map[domain.MessageKind]domain.MessageKind{
		domain.MessagePhoto:    domain.MessagePhoto,
		domain.MessageVideo:    domain.MessageVideo,
		domain.MessageAudio:    domain.MessageAudio,
		domain.MessageDocument: domain.MessageDocument,
		domain.MessageSticker:  domain.MessageSticker,
	}
	kindNames := map[domain.MessageKind]string{
		domain.MessagePhoto:    "photo",
		domain.MessageVideo:    "video",
		domain.MessageAudio:    "audio",
		domain.MessageDocument: "document",
		domain.MessageSticker:  "sticker",
	}
	for kind := range kinds {
		name := kindNames[kind]
		t.Run(name, func(t *testing.T) {
			state := topicMediaBaseState(t)
			localID := domain.MessageID(-11)
			msg := domain.Message{
				ID: localID, ChatID: 7, TopicID: 101, SentAt: now, Kind: kind,
				Outgoing: true, SendState: domain.SendFailed,
			}
			msg.Media.File.LocalPath = "/f.bin"
			msg.Sticker = domain.StickerRef{File: domain.MediaFileRef{ID: 3}}
			state.Messages[7] = append(state.Messages[7], msg)
			state.SelectedMessage = localID
			state.SelectedMessageChat = 7
			cmd, ok := retryMessage(&state, localID, now)
			if !ok || cmd == nil {
				t.Fatalf("retry = %#v, ok = %v", cmd, ok)
			}
			topic := domain.TopicID(0)
			switch c := cmd.(type) {
			case SendPhoto:
				topic = c.TopicID
			case SendVideo:
				topic = c.TopicID
			case SendAudio:
				topic = c.TopicID
			case SendDocument:
				topic = c.TopicID
			case SendSticker:
				topic = c.TopicID
			default:
				t.Fatalf("cmd = %#v", cmd)
			}
			if topic != 101 {
				t.Fatalf("retry topic = %d, want 101", topic)
			}
		})
	}
}

func TestQueuedReplacementPreservesTopicIDWhenOmitted(t *testing.T) {
	localID := domain.MessageID(-7)
	now := time.Unix(300, 0).UTC()
	state := topicMediaBaseState(t)
	optimistic := domain.Message{
		ID: localID, ChatID: 7, TopicID: 101, SentAt: now, Kind: domain.MessagePhoto,
		Outgoing: true, SendState: domain.SendPending,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/p.jpg", Downloaded: true}},
	}
	state.Messages[7] = append(state.Messages[7], optimistic)
	state.PhotoSendRequests[localID] = 12

	queued := domain.Message{ID: 900, ChatID: 7, Kind: domain.MessagePhoto, Text: ""}
	replaceQueuedPhoto(&state, PhotoQueued{RequestID: 12, LocalID: localID, ChatID: 7, Message: queued})
	msg := state.Messages[7][len(state.Messages[7])-1]
	if msg.ID != 900 {
		t.Fatalf("replacement = %#v", msg)
	}
	if msg.TopicID != 101 {
		t.Fatalf("topic = %d, want preserved 101", msg.TopicID)
	}
}

func TestReplaceSentMessagePreservesTopicIDWhenOmitted(t *testing.T) {
	localID := domain.MessageID(-8)
	state := topicMediaBaseState(t)
	state.Messages[7] = append(state.Messages[7], domain.Message{
		ID: localID, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "hi",
		Outgoing: true, SendState: domain.SendPending,
	})
	replaceSentMessage(&state, localID, domain.Message{ID: 901, ChatID: 7, Kind: domain.MessageText, Text: "hi"})
	msg := state.Messages[7][len(state.Messages[7])-1]
	if msg.ID != 901 {
		t.Fatalf("replacement = %#v", msg)
	}
	if msg.TopicID != 101 {
		t.Fatalf("topic = %d, want preserved 101", msg.TopicID)
	}
}

func TestFailQueuedMessagePreservesTopicIdentity(t *testing.T) {
	localID := domain.MessageID(-9)
	state := topicMediaBaseState(t)
	state.Messages[7] = append(state.Messages[7], domain.Message{
		ID: localID, ChatID: 7, TopicID: 101, Kind: domain.MessagePhoto,
		Outgoing: true, SendState: domain.SendPending,
		Media: domain.MessageMedia{File: domain.MediaFileRef{LocalPath: "/p.jpg", Downloaded: true}},
	})
	state.PhotoSendRequests[localID] = 13
	failQueuedPhoto(&state, PhotoQueueFailed{RequestID: 13, LocalID: localID, ChatID: 7, Error: domain.AppError{Kind: domain.ErrorNetwork, Message: "x"}, FailedAt: time.Unix(400, 0)})
	msg := state.Messages[7][len(state.Messages[7])-1]
	if msg.SendState != domain.SendFailed {
		t.Fatalf("send state = %v", msg.SendState)
	}
	if msg.TopicID != 101 {
		t.Fatalf("topic = %d, want 101", msg.TopicID)
	}
}

func topicSearchState(t *testing.T) State {
	t.Helper()
	state := topicMediaBaseState(t)
	opened, commands := updateState(state, ActionReceived{Action: OpenMessageSearch})
	if opened.MessageSearch == nil || opened.Focus != FocusSearchInput {
		t.Fatalf("open = %#v %#v", opened.MessageSearch, opened.Focus)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
	return opened
}

func TestMessageSearchTopicScopeOpenSubmitPaginate(t *testing.T) {
	opened := topicSearchState(t)
	if opened.MessageSearch.TopicID != 101 {
		t.Fatalf("topic = %d, want 101", opened.MessageSearch.TopicID)
	}

	// Forum without a selected topic: no-op.
	noTopic := topicMediaBaseState(t)
	delete(noTopic.SelectedTopics, 7)
	unchanged, commands := updateState(noTopic, ActionReceived{Action: OpenMessageSearch})
	if unchanged.MessageSearch != nil || len(commands) != 0 {
		t.Fatalf("no-topic open = %#v %#v", unchanged.MessageSearch, commands)
	}

	submitted, commands := updateState(opened, MessageSearchValueChanged{ChatID: 7, Value: "needle"})
	_, commands = updateState(submitted, ActionReceived{Action: SubmitMessageSearch})
	if len(commands) != 1 {
		t.Fatalf("cmds = %#v", commands)
	}
	cmd, ok := commands[0].(SearchChatMessages)
	if !ok || cmd.ChatID != 7 || cmd.TopicID != 101 || cmd.Query != "needle" {
		t.Fatalf("search = %#v", commands[0])
	}

	// Paginate carries TopicID.
	s := submitted
	s.MessageSearch.NextFromMessageID = 40
	s.MessageSearch.Results = []domain.Message{{ID: 40, ChatID: 7, TopicID: 101}}
	s.MessageSearch.Selected = 0
	s.MessageSearch.Loading = false
	s.MessageSearch.Done = false
	_, cmds := maybePaginateMessageSearch(s)
	if len(cmds) != 1 {
		t.Fatalf("paginate cmds = %#v", cmds)
	}
	pcmd, ok := cmds[0].(SearchChatMessages)
	if !ok || pcmd.TopicID != 101 {
		t.Fatalf("paginate = %#v", cmds[0])
	}
}

func TestMessageSearchRejectsMismatchedTopicID(t *testing.T) {
	opened := topicSearchState(t)
	submitted, commands := updateState(opened, MessageSearchValueChanged{ChatID: 7, Value: "needle"})
	submitted, _ = updateState(submitted, ActionReceived{Action: SubmitMessageSearch})
	requestID := submitted.MessageSearch.RequestID
	_ = commands

	rejected, commands := updateState(submitted, ChatMessagesSearched{
		RequestID: requestID, ChatID: 7, TopicID: 202,
		Page: telegram.MessageSearchPage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 202}}},
	})
	if len(rejected.MessageSearch.Results) != 0 {
		t.Fatalf("mismatched topic results merged: %#v", rejected.MessageSearch.Results)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}

	failed, commands := updateState(submitted, ChatMessagesSearchFailed{RequestID: requestID, ChatID: 7, TopicID: 202})
	if failed.MessageSearch.Error != nil {
		t.Fatalf("mismatched topic failure applied: %#v", failed.MessageSearch.Error)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}

	// Activate emits LoadSearchMessageContext with TopicID.
	loaded, _ := updateState(submitted, ChatMessagesSearched{
		RequestID: requestID, ChatID: 7, TopicID: 101,
		Page: telegram.MessageSearchPage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 101}}},
	})
	activated, cmds := updateState(loaded, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate cmds = %#v", cmds)
	}
	jump, ok := cmds[0].(LoadSearchMessageContext)
	if !ok || jump.ChatID != 7 || jump.TopicID != 101 || jump.MessageID != 80 {
		t.Fatalf("jump = %#v", cmds[0])
	}

	// Context loaded with mismatched TopicID is rejected.
	rejectedCtx, commands := updateState(activated, SearchMessageContextLoaded{
		RequestID: jump.RequestID, ChatID: 7, TopicID: 202, MessageID: 80,
		Page: telegram.MessagePage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 202}}},
	})
	if rejectedCtx.MessageSearch == nil || rejectedCtx.MessageSearch.JumpMessageID != 80 {
		t.Fatalf("mismatched context applied: %#v", rejectedCtx.MessageSearch)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}

	// Matching TopicID loads fine.
	matched, _ := updateState(activated, SearchMessageContextLoaded{
		RequestID: jump.RequestID, ChatID: 7, TopicID: 101, MessageID: 80,
		Page: telegram.MessagePage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 101}}},
	})
	if matched.MessageSearch != nil {
		t.Fatalf("matching context not applied: %#v", matched.MessageSearch)
	}
}

func TestPinnedMessagesTopicScope(t *testing.T) {
	state := topicMediaBaseState(t)
	opened, cmds := updateState(state, ActionReceived{Action: OpenPinnedMessages})
	if opened.PinnedMessages == nil {
		t.Fatal("pinned not opened")
	}
	if opened.PinnedMessages.TopicID != 101 {
		t.Fatalf("topic = %d, want 101", opened.PinnedMessages.TopicID)
	}
	if len(cmds) != 1 {
		t.Fatalf("cmds = %#v", cmds)
	}
	load, ok := cmds[0].(LoadPinnedMessages)
	if !ok || load.ChatID != 7 || load.TopicID != 101 {
		t.Fatalf("load = %#v", cmds[0])
	}
	requestID := opened.PinnedMessages.RequestID

	// Forum without topic: no-op.
	noTopic := topicMediaBaseState(t)
	delete(noTopic.SelectedTopics, 7)
	unchanged, commands := updateState(noTopic, ActionReceived{Action: OpenPinnedMessages})
	if unchanged.PinnedMessages != nil || len(commands) != 0 {
		t.Fatalf("no-topic pinned = %#v %#v", unchanged.PinnedMessages, commands)
	}

	// Mismatched topic page rejected.
	rejected, commands := updateState(opened, PinnedMessagesLoaded{
		RequestID: requestID, ChatID: 7, TopicID: 202,
		Page: telegram.MessageSearchPage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 202}}},
	})
	if len(rejected.PinnedMessages.Results) != 0 {
		t.Fatalf("mismatched pinned results = %#v", rejected.PinnedMessages.Results)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
	rejectedFailed, commands := updateState(opened, PinnedMessagesLoadFailed{RequestID: requestID, ChatID: 7, TopicID: 202})
	if rejectedFailed.PinnedMessages.Error != nil {
		t.Fatalf("mismatched pinned failure applied: %#v", rejectedFailed.PinnedMessages.Error)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}

	// Matching page accepted; activate carries TopicID.
	accepted, _ := updateState(opened, PinnedMessagesLoaded{
		RequestID: requestID, ChatID: 7, TopicID: 101,
		Page: telegram.MessageSearchPage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 101}}},
	})
	if len(accepted.PinnedMessages.Results) != 1 {
		t.Fatalf("results = %#v", accepted.PinnedMessages.Results)
	}
	activated, cmds := updateState(accepted, ActionReceived{Action: Activate})
	if len(cmds) != 1 {
		t.Fatalf("activate cmds = %#v", cmds)
	}
	jump, ok := cmds[0].(LoadPinnedMessageContext)
	if !ok || jump.ChatID != 7 || jump.TopicID != 101 || jump.MessageID != 80 {
		t.Fatalf("jump = %#v", cmds[0])
	}

	// Context loaded with mismatched TopicID is rejected.
	rejectedCtx, commands := updateState(activated, PinnedMessageContextLoaded{
		RequestID: jump.RequestID, ChatID: 7, TopicID: 202, MessageID: 80,
		Page: telegram.MessagePage{Messages: []domain.Message{{ID: 80, ChatID: 7, TopicID: 202}}},
	})
	if rejectedCtx.PinnedMessages == nil || rejectedCtx.PinnedMessages.JumpMessageID != 80 {
		t.Fatalf("mismatched pinned context applied: %#v", rejectedCtx.PinnedMessages)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestReconcilePinnedMessagesIgnoresCrossTopicPinEvents(t *testing.T) {
	state := topicMediaBaseState(t)
	state.Messages[7] = append(state.Messages[7],
		domain.Message{ID: 200, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "a", Pinned: true},
		domain.Message{ID: 300, ChatID: 7, TopicID: 202, Kind: domain.MessageText, Text: "b", Pinned: true},
	)
	state.PinnedMessages = &PinnedMessagesState{
		ChatID: 7, TopicID: 101, RequestID: 10, PreviousFocus: FocusConversation,
		Results:  []domain.Message{{ID: 200, ChatID: 7, TopicID: 101, Pinned: true}},
		Selected: 0, TotalCount: 1,
	}

	// Pin in another topic must not leak into this topic's view.
	got, _ := updateState(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 7, MessageID: 300, Pinned: true}})
	if len(got.PinnedMessages.Results) != 1 {
		t.Fatalf("results = %#v, want only topic 101 message", got.PinnedMessages.Results)
	}
	if got.PinnedMessages.TotalCount != 1 {
		t.Fatalf("count = %d, want 1", got.PinnedMessages.TotalCount)
	}

	// Unpin within the same topic still reconciles.
	unpinned, _ := updateState(state, TelegramEvent{Value: telegram.MessagePinnedUpdated{ChatID: 7, MessageID: 200, Pinned: false}})
	if len(unpinned.PinnedMessages.Results) != 0 {
		t.Fatalf("results = %#v, want empty", unpinned.PinnedMessages.Results)
	}
	if unpinned.PinnedMessages.TotalCount != 0 {
		t.Fatalf("count = %d, want 0", unpinned.PinnedMessages.TotalCount)
	}
}

func TestGlobalSearchJumpToTopicMessageSelectsTopicAndLoadsTopicContext(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusChats
	state.Chats = []domain.Chat{
		{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true},
	}
	state.SelectedChat = 0
	state.NextRequestID = 10
	msg := domain.Message{ID: 80, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "hit"}
	opened, commands := openChatFromMessageResult(state, msg)
	var cmd Effect
	for _, c := range commands {
		if jump, ok := c.(LoadSearchMessageContext); ok {
			cmd = jump
			break
		}
	}
	if cmd == nil {
		t.Fatalf("no LoadSearchMessageContext in %#v", commands)
	}
	if cmd.(LoadSearchMessageContext).ChatID != 7 || cmd.(LoadSearchMessageContext).TopicID != 101 || cmd.(LoadSearchMessageContext).MessageID != 80 {
		t.Fatalf("cmd = %#v", cmd)
	}
	if opened.SelectedChat != 0 || opened.Chats[opened.SelectedChat].ID != 7 {
		t.Fatalf("selected chat = %d", opened.SelectedChat)
	}
	if opened.SelectedTopics[7] != 101 {
		t.Fatalf("selected topic = %d, want 101", opened.SelectedTopics[7])
	}
	tracked := opened.ForumTopics[7][101]
	if tracked.ID != 101 || tracked.ChatID != 7 || tracked.Name != "Topic" {
		t.Fatalf("forum topic = %#v, want minimal fallback", tracked)
	}
	if opened.ChatSearch != nil {
		t.Fatal("chat search not closed")
	}
	if opened.Focus != FocusConversation {
		t.Fatalf("focus = %v", opened.Focus)
	}
	if opened.MessageSearch == nil || opened.MessageSearch.TopicID != 101 || opened.MessageSearch.JumpMessageID != 80 {
		t.Fatalf("message search = %#v", opened.MessageSearch)
	}
}

func TestGlobalSearchJumpToTopicMessageIgnoresNonForumChat(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusChats
	state.Chats = []domain.Chat{{ID: 7, Title: "Plain", CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	msg := domain.Message{ID: 80, ChatID: 7, TopicID: 101, Kind: domain.MessageText, Text: "hit"}
	before := state
	_, commands := openChatFromMessageResult(before, msg)
	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none for non-forum topic jump", commands)
	}
}

func TestGlobalSearchJumpToOrdinaryMessageUnchanged(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusChats
	state.Chats = []domain.Chat{{ID: 7, Title: "Plain", CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	msg := domain.Message{ID: 80, ChatID: 7, Kind: domain.MessageText, Text: "hit"}
	opened, commands := openChatFromMessageResult(state, msg)
	var cmd Effect
	for _, c := range commands {
		if jump, ok := c.(LoadSearchMessageContext); ok {
			cmd = jump
			break
		}
	}
	if cmd == nil {
		t.Fatalf("no LoadSearchMessageContext in %#v", commands)
	}
	if j := cmd.(LoadSearchMessageContext); j.ChatID != 7 || j.TopicID != 0 || j.MessageID != 80 {
		t.Fatalf("cmd = %#v", j)
	}
	if opened.MessageSearch == nil || opened.MessageSearch.TopicID != 0 || opened.MessageSearch.JumpMessageID != 80 {
		t.Fatalf("message search = %#v", opened.MessageSearch)
	}
}

func TestChatSearchForumActivationEntersAllDirectly(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusChatSearchResults
	state.Chats = []domain.Chat{{ID: 7, Kind: domain.ChatSupergroup, Title: "Forum", IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	chat := state.Chats[0]
	opened, commands := openChatFromSearchResult(state, chat)
	if chatIndex(opened.Chats, 7) < 0 {
		t.Fatal("chat not selected")
	}
	if opened.Topics != nil {
		t.Fatalf("topics modal opened = %#v, want direct ALL entry", opened.Topics)
	}
	if !opened.ShowAll[7] {
		t.Fatalf("ShowAll = %#v, want ALL mode", opened.ShowAll)
	}
	if opened.Focus != FocusConversation {
		t.Fatalf("focus = %v, want FocusConversation", opened.Focus)
	}
	history, exists := opened.History[7]
	if !exists || !history.Loading {
		t.Fatal("ALL history must be requested for forum activation")
	}
	sawLoadMessages := false
	for _, c := range commands {
		if _, ok := c.(LoadTopics); ok {
			t.Fatalf("unexpected LoadTopics command: %#v", c)
		}
		if load, ok := c.(LoadMessages); ok {
			if load.ChatID != 7 || load.TopicID != 0 {
				t.Fatalf("unexpected history command: %#v", c)
			}
			sawLoadMessages = true
		}
	}
	if !sawLoadMessages {
		t.Fatalf("no ALL history load in %#v", commands)
	}
}

func TestActivateTopicClearsCommandMenu(t *testing.T) {
	state := topicMediaBaseState(t)
	state.CommandMenu = &CommandMenuState{ChatID: 7, Query: "/h"}
	state.Focus = FocusTopics
	state.Topics = &TopicListState{RequestID: 10, ChatID: 7, PreviousFocus: FocusChats, Results: []domain.ForumTopic{{ID: 101, ChatID: 7, Name: "General"}}}
	opened, _ := activateTopic(state, domain.ForumTopic{ID: 101, ChatID: 7, Name: "General"})
	if opened.CommandMenu != nil {
		t.Fatalf("command menu = %#v, want cleared", opened.CommandMenu)
	}
	if opened.SelectedTopics[7] != 101 {
		t.Fatalf("selected topic = %d, want 101", opened.SelectedTopics[7])
	}
}
