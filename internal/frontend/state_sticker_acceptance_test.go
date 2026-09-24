package frontend

import (
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func stickerPickerState() State {
	state := InitialState()
	state.Width, state.Height = 100, 30
	state.Layout = layoutForSize(state.Width, state.Height)
	state.Connection = domain.ConnectionOnline
	state.Focus = FocusComposer
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.NextRequestID = 10
	state.NextLocalID = -1
	state.Drafts[9] = "preserve draft"
	state.Messages[9] = []domain.Message{{ID: 44, ChatID: 9, Kind: domain.MessageText, Text: "reply"}}
	return state
}

func testStickers() []domain.StickerRef {
	return []domain.StickerRef{
		{File: domain.MediaFileRef{ID: 101}, Thumbnail: domain.MediaFileRef{ID: 201, CanDownload: true}, Width: 64, Height: 32, Emoji: "🙂"},
		{File: domain.MediaFileRef{ID: 102}, Width: 32, Height: 64},
		{File: domain.MediaFileRef{ID: 103}, Width: 32, Height: 32},
	}
}

func TestStickerPickerGuardsLoadingStaleErrorEmptyNavigationAndResize(t *testing.T) {
	pending := func() State {
		state := stickerPickerState()
		commands := updateState(&state, ActionReceived{Action: OpenStickerPicker})
		if !reflect.DeepEqual(commands, []Effect{LoadStickers{RequestID: 10, ChatID: 9}}) || state.StickerPicker == nil || !state.StickerPicker.Loading || state.Focus != FocusStickerPicker || state.StickerPicker.PreviousFocus != FocusComposer {
			t.Fatalf("open = picker:%#v focus:%v commands:%#v", state.StickerPicker, state.Focus, commands)
		}
		return state
	}

	state := pending()
	staleCommands := updateState(&state, &StickersLoaded{RequestID: 9, ChatID: 9, Stickers: testStickers()})
	if len(staleCommands) != 0 || !state.StickerPicker.Loading || len(state.StickerPicker.Catalog) != 0 || state.StickerPicker.RequestID != 10 {
		t.Fatalf("stale catalog accepted: picker=%#v commands=%#v", state.StickerPicker, staleCommands)
	}
	commands := updateState(&state, StickersLoaded{RequestID: 10, ChatID: 9, Stickers: testStickers()})
	if state.StickerPicker.Loading || len(state.StickerPicker.Catalog) != 3 || len(commands) != 1 {
		t.Fatalf("loaded = %#v commands=%#v", state.StickerPicker, commands)
	}
	thumb := commands[0].(DownloadStickerThumbnail)
	if thumb.File.ID != 201 || thumb.StickerFileID != 101 || thumb.PickerRequestID != 10 {
		t.Fatalf("thumbnail command = %#v", thumb)
	}
	updateState(&state, ActionReceived{Action: StickerMoveRight})
	updateState(&state, ActionReceived{Action: StickerMoveDown})
	if state.StickerPicker.Selected != 2 {
		t.Fatalf("clamped navigation = %d", state.StickerPicker.Selected)
	}
	updateState(&state, Resized{Width: 60, Height: 18})
	_, _, wantColumns, wantRows := StickerGridGeometry(60, 18)
	if state.StickerPicker.Columns != wantColumns || state.StickerPicker.VisibleRows != wantRows {
		t.Fatalf("resize geometry = %dx%d", state.StickerPicker.Columns, state.StickerPicker.VisibleRows)
	}

	failed := pending()
	updateState(&failed, StickersLoadFailed{RequestID: 10, ChatID: 9, Error: domain.AppError{Kind: domain.ErrorNetwork, Message: "private"}})
	if failed.StickerPicker.Error == nil || failed.StickerPicker.Error.Message != "Could not load stickers" || failed.StickerPicker.Loading {
		t.Fatalf("failure = %#v", failed.StickerPicker)
	}
	empty := pending()
	updateState(&empty, StickersLoaded{RequestID: 10, ChatID: 9})
	if empty.StickerPicker == nil || empty.StickerPicker.Loading || len(empty.StickerPicker.Catalog) != 0 {
		t.Fatalf("empty = %#v", empty.StickerPicker)
	}
	updateState(&failed, ActionReceived{Action: Close})
	if failed.StickerPicker != nil || failed.Focus != FocusComposer {
		t.Fatalf("close = picker:%#v focus:%v", failed.StickerPicker, failed.Focus)
	}

	for _, tc := range []struct {
		name   string
		change func(*State)
	}{
		{"wrong focus", func(s *State) { s.Focus = FocusConversation }},
		{"offline", func(s *State) { s.Connection = domain.ConnectionOffline }},
		{"cannot send", func(s *State) { s.Chats[0].CanSend = false }},
		{"editing", func(s *State) { s.EditTarget = &EditTarget{ChatID: 9, MessageID: 44} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := stickerPickerState()
			tc.change(&state)
			commands := updateState(&state, ActionReceived{Action: OpenStickerPicker})
			if len(commands) != 0 || state.StickerPicker != nil || state.Focus == FocusStickerPicker || state.NextRequestID != 10 {
				t.Fatalf("guard accepted open: picker=%#v focus=%v request=%d effects=%#v", state.StickerPicker, state.Focus, state.NextRequestID, commands)
			}
		})
	}
}

func TestStickerPosterIntakeUsesThumbnailAcrossAllIngress(t *testing.T) {
	sticker := domain.Message{ID: 77, ChatID: 9, Kind: domain.MessageSticker, Media: domain.MessageMedia{
		File: domain.MediaFileRef{ID: 700, CanDownload: true}, Thumbnail: domain.MediaFileRef{ID: 701, CanDownload: true},
	}}
	for name, reduce := range map[string]func(*State) []Effect{
		"history": func(state *State) []Effect {
			state.History[9] = HistoryState{RequestID: 5, Loading: true}
			return updateState(state, MessagesLoaded{RequestID: 5, ChatID: 9, Page: telegram.MessagePage{Messages: []domain.Message{sticker}, Done: true}})
		},
		"upsert": func(state *State) []Effect {
			return updateState(state, TelegramEvent{Value: telegram.MessageUpserted{Message: sticker}})
		},
		"content": func(state *State) []Effect {
			state.Messages[9] = []domain.Message{{ID: 77, ChatID: 9, Kind: domain.MessageText}}
			return updateState(state, TelegramEvent{Value: telegram.MessageContentUpdated{ChatID: 9, MessageID: 77, Kind: domain.MessageSticker, Media: sticker.Media}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := stickerPickerState()
			state.NextRequestID = 30
			commands := reduce(&state)
			if len(commands) != 1 {
				t.Fatalf("commands = %#v", commands)
			}
			download := commands[0].(DownloadThumbnail)
			if download.File.ID != 701 || download.File.ID == sticker.Media.File.ID {
				t.Fatalf("downloaded wrong file = %#v", download)
			}
		})
	}
}

func TestStickerStaleQueueIDCollisionIsFullStateNoOp(t *testing.T) {
	sentFixture := func() (State, SendSticker) {
		state := stickerPickerState()
		updateState(&state, ActionReceived{Action: OpenStickerPicker})
		updateState(&state, StickersLoaded{RequestID: 10, ChatID: 9, Stickers: testStickers()})
		commands := updateState(&state, ActionReceived{Action: StickerActivate, RequestID: 10, StickerFileID: 101, At: time.Unix(100, 0)})
		sent := state
		sent.Messages[9] = append(sent.Messages[9], domain.Message{
			ID: 80, ChatID: 9, Kind: domain.MessageSticker,
			Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 980, CanDownload: true}},
		})
		return sent, commands[0].(SendSticker)
	}
	sent, command := sentFixture()
	before, _ := sentFixture()
	staleCommands := updateState(&sent, StickerQueued{
		RequestID: command.RequestID - 1,
		LocalID:   command.LocalID,
		ChatID:    9,
		Message:   domain.Message{ID: 80, ChatID: 9, Kind: domain.MessageSticker},
	})
	if len(staleCommands) != 0 || !reflect.DeepEqual(sent, before) || sent.NextRequestID != before.NextRequestID {
		t.Fatalf("stale ID collision mutated state: commands=%#v next=%d want=%d", staleCommands, sent.NextRequestID, before.NextRequestID)
	}
}

func TestStickerOptimisticSendQueueRetryAndTerminalReconciliation(t *testing.T) {
	setup := func() State {
		state := stickerPickerState()
		updateState(&state, ActionReceived{Action: OpenStickerPicker})
		updateState(&state, StickersLoaded{RequestID: 10, ChatID: 9, Stickers: testStickers()})
		state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 44}
		state.StickerThumbnails[101] = thumbnail.Block{Text: "poster", Width: 5, Height: 2}
		return state
	}
	sentFixture := func() (State, []Effect) {
		state := setup()
		commands := updateState(&state, &ActionReceived{Action: StickerActivate, RequestID: 10, StickerFileID: 101, At: time.Unix(100, 0)})
		return state, commands
	}
	beforeDraft := setup().Drafts[9]
	sent, commands := sentFixture()
	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	command := commands[0].(SendSticker)
	if save := commands[1].(SaveDraft); save.Text != beforeDraft || save.ReplyToMessageID != 0 {
		t.Fatalf("post-sticker draft save = %#v", save)
	}
	message := sent.Messages[9][messageIndex(sent.Messages[9], -1)]
	if message.ID != -1 || message.Kind != domain.MessageSticker || message.Sticker.File.ID != 101 || !message.HasReply || message.ReplyToMessageID != 44 || message.SendState != domain.SendPending || sent.Drafts[9] != beforeDraft || sent.ReplyTarget != nil || sent.StickerPicker != nil || sent.Focus != FocusConversation || sent.Thumbnails[9][-1].Text != "poster" {
		t.Fatalf("optimistic = %#v state=%#v", message, sent)
	}
	if command.ReplyToMessageID != 44 || command.Sticker.File.ID != 101 {
		t.Fatalf("send command = %#v", command)
	}

	// updateState mutates in place, so each scenario below starts from its
	// own freshly built fixture instead of sharing one State copy.

	staleState, _ := sentFixture()
	updateState(&staleState, StickerQueued{RequestID: command.RequestID - 1, LocalID: -1, ChatID: 9, Message: domain.Message{ID: -20, Kind: domain.MessageSticker}})
	if !reflect.DeepEqual(staleState, sent) {
		t.Fatal("stale queue result mutated state")
	}

	queued, _ := sentFixture()
	updateState(&queued, &StickerQueued{RequestID: command.RequestID, LocalID: -1, ChatID: 9, Message: domain.Message{ID: -20, ChatID: 9, Kind: domain.MessageSticker}})
	queuedMessage := queued.Messages[9][messageIndex(queued.Messages[9], -20)]
	if queuedMessage.ID != -20 || queuedMessage.Sticker.File.ID != 101 || queued.Thumbnails[9][-20].Text != "poster" || queued.SelectedMessage != -20 {
		t.Fatalf("queued = %#v", queued)
	}

	failure := domain.AppError{Kind: domain.ErrorNetwork, Op: "send sticker", Message: "Sticker send failed"}
	failedFixture := func() State {
		state, cmds := sentFixture()
		updateState(&state, StickerQueueFailed{RequestID: cmds[0].(SendSticker).RequestID, LocalID: -1, ChatID: 9, Error: failure, FailedAt: time.Unix(101, 0)})
		return state
	}
	failed := failedFixture()
	failedMessage := failed.Messages[9][messageIndex(failed.Messages[9], -1)]
	if failedMessage.SendState != domain.SendFailed || failedMessage.Failure == nil {
		t.Fatalf("queue failure = %#v", failedMessage)
	}
	retried := failedFixture()
	retryCommands := updateState(&retried, ActionReceived{Action: Retry, MessageID: -1, At: time.Unix(102, 0)})
	retriedMessage := retried.Messages[9][messageIndex(retried.Messages[9], -1)]
	if len(retryCommands) != 1 || retriedMessage.SendState != domain.SendPending || retryCommands[0].(SendSticker).Sticker.File.ID != 101 || retryCommands[0].(SendSticker).ReplyToMessageID != 44 {
		t.Fatalf("retry = %#v commands=%#v", retriedMessage, retryCommands)
	}
	invalid := failedFixture()
	invalidBefore := failedFixture()
	invalid.Messages[9][messageIndex(invalid.Messages[9], -1)].Sticker.File.ID = 0
	invalidBefore.Messages[9][messageIndex(invalidBefore.Messages[9], -1)].Sticker.File.ID = 0
	invalidCommands := updateState(&invalid, ActionReceived{Action: Retry, MessageID: -1, At: time.Unix(102, 0)})
	if !reflect.DeepEqual(invalid, invalidBefore) || len(invalidCommands) != 0 {
		t.Fatal("invalid retry mutated state")
	}

	succeeded, _ := sentFixture()
	updateState(&succeeded, TelegramEvent{Value: telegram.MessageSendSucceeded{OldID: -1, Message: domain.Message{ID: 80, ChatID: 9, Kind: domain.MessageSticker, SendState: domain.SendSucceeded}}})
	succeededMessage := succeeded.Messages[9][messageIndex(succeeded.Messages[9], 80)]
	if succeededMessage.Sticker.File.ID != 101 || succeededMessage.Media.Thumbnail.ID != 201 || succeeded.Thumbnails[9][80].Text != "poster" {
		t.Fatalf("terminal success = %#v", succeededMessage)
	}

	terminalFailed, _ := sentFixture()
	updateState(&terminalFailed, TelegramEvent{ReceivedAt: time.Unix(103, 0), Value: telegram.MessageSendFailed{OldID: -1, Message: domain.Message{ID: -2, ChatID: 9, Kind: domain.MessageSticker}, Error: failure}})
	terminalFailedMessage := terminalFailed.Messages[9][messageIndex(terminalFailed.Messages[9], -2)]
	if terminalFailedMessage.Sticker.File.ID != 101 || terminalFailedMessage.Media.Thumbnail.ID != 201 || terminalFailed.Thumbnails[9][-2].Text != "poster" {
		t.Fatalf("terminal failure = %#v", terminalFailedMessage)
	}
}
