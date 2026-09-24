package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

const (
	StickerTileWidth  = 14
	StickerTileHeight = 7
)

// StickerGridGeometry is the shared deterministic picker geometry contract.
// The frontend owns placement, while State owns the resulting navigation
// columns and visible rows.
func StickerGridGeometry(width, height int) (modalWidth, modalHeight, columns, rows int) {
	modalWidth = min(76, max(32, width-6))
	modalHeight = min(24, max(12, height-4))
	columns = max(1, (modalWidth-2)/StickerTileWidth)
	rows = max(1, (modalHeight-4)/StickerTileHeight)
	return
}

func resizeStickerPicker(state *State) {
	if state.StickerPicker == nil {
		return
	}
	_, _, columns, rows := StickerGridGeometry(state.Width, state.Height)
	state.StickerPicker.Columns = columns
	state.StickerPicker.VisibleRows = rows
	keepStickerSelectionVisible(state.StickerPicker)
}

func openStickerPicker(state *State) []Effect {
	if state.Focus != FocusComposer || state.Connection != domain.ConnectionOnline || state.EditTarget != nil {
		return nil
	}
	chatID, ok := activeChatID(*state)
	if !ok {
		return nil
	}
	index := chatIndex(state.Chats, chatID)
	if index < 0 || !state.Chats[index].CanSend || forumTopicClosed(*state, chatID) {
		return nil
	}
	requestID := allocateRequestID(state)
	topicID := domain.TopicID(0)
	if state.Chats[index].IsForum {
		topicID = state.SelectedTopics[chatID]
	}
	_, _, columns, rows := StickerGridGeometry(state.Width, state.Height)
	state.StickerPicker = &StickerPickerState{
		RequestID: requestID, ChatID: chatID, TopicID: topicID, PreviousFocus: state.Focus,
		Loading: true, Columns: columns, VisibleRows: rows,
	}
	state.StickerThumbnailRequests = make(map[int32]uint64)
	state.Focus = FocusStickerPicker
	return []Effect{LoadStickers{RequestID: requestID, ChatID: chatID, TopicID: topicID}}
}

func reduceStickersLoaded(state *State, event StickersLoaded) []Effect {
	picker := state.StickerPicker
	if picker == nil || picker.RequestID != event.RequestID || picker.ChatID != event.ChatID {
		return nil
	}
	picker.Loading = false
	picker.Error = nil
	picker.Catalog = append([]domain.StickerRef(nil), event.Stickers...)
	picker.Selected = 0
	picker.FirstRow = 0
	commands := make([]Effect, 0, len(picker.Catalog))
	if state.StickerThumbnailRequests == nil {
		state.StickerThumbnailRequests = make(map[int32]uint64)
	}
	for _, sticker := range picker.Catalog {
		if sticker.File.ID == 0 {
			continue
		}
		if block := state.StickerThumbnails[sticker.File.ID]; block.Width > 0 && block.Height > 0 {
			continue
		}
		thumb := sticker.Thumbnail
		if thumb.ID == 0 || (!thumb.CanDownload && !(thumb.Downloaded && thumb.LocalPath != "")) {
			continue
		}
		requestID := allocateRequestID(state)
		state.StickerThumbnailRequests[sticker.File.ID] = requestID
		commands = append(commands, DownloadStickerThumbnail{
			RequestID: requestID, PickerRequestID: picker.RequestID,
			StickerFileID: sticker.File.ID, File: thumb,
		})
	}
	return commands
}

func reduceStickersLoadFailed(state *State, event StickersLoadFailed) []Effect {
	picker := state.StickerPicker
	if picker == nil || picker.RequestID != event.RequestID || picker.ChatID != event.ChatID {
		return nil
	}
	failure := domain.AppError{Kind: event.Error.Kind, Op: "load stickers", Message: "Could not load stickers"}
	picker.Loading = false
	picker.Error = &failure
	picker.Catalog = nil
	picker.Selected = 0
	picker.FirstRow = 0
	return nil
}

func reduceStickerThumbnailRendered(state *State, event StickerThumbnailRendered) []Effect {
	picker := state.StickerPicker
	requestID, requested := state.StickerThumbnailRequests[event.StickerFileID]
	if picker == nil || event.RequestID == 0 || picker.RequestID != event.PickerRequestID || !requested || requestID != event.RequestID {
		return nil
	}
	if event.Block.Width <= 0 || event.Block.Height <= 0 {
		return nil
	}
	if state.StickerThumbnails == nil {
		state.StickerThumbnails = make(map[int32]thumbnail.Block)
	}
	state.StickerThumbnails[event.StickerFileID] = event.Block
	delete(state.StickerThumbnailRequests, event.StickerFileID)
	return nil
}

func reduceStickerThumbnailFailed(state *State, event StickerThumbnailFailed) []Effect {
	picker := state.StickerPicker
	requestID, requested := state.StickerThumbnailRequests[event.StickerFileID]
	if picker == nil || event.RequestID == 0 || picker.RequestID != event.PickerRequestID || !requested || requestID != event.RequestID {
		return nil
	}
	delete(state.StickerThumbnailRequests, event.StickerFileID)
	return nil
}

func reduceStickerPicker(state *State, event ActionReceived) []Effect {
	picker := state.StickerPicker
	switch event.Action {
	case Close:
		state.Focus = picker.PreviousFocus
		state.StickerPicker = nil
		state.StickerThumbnailRequests = make(map[int32]uint64)
		return nil
	case StickerMoveLeft:
		moveStickerSelection(picker, -1)
	case StickerMoveRight:
		moveStickerSelection(picker, 1)
	case StickerMoveUp:
		moveStickerSelection(picker, -max(1, picker.Columns))
	case StickerMoveDown:
		moveStickerSelection(picker, max(1, picker.Columns))
	case StickerActivate:
		return sendSelectedSticker(state, event)
	default:
		return nil
	}
	return nil
}

func moveStickerSelection(picker *StickerPickerState, delta int) {
	if picker == nil || len(picker.Catalog) == 0 {
		return
	}
	picker.Selected = max(0, min(len(picker.Catalog)-1, picker.Selected+delta))
	keepStickerSelectionVisible(picker)
}

func keepStickerSelectionVisible(picker *StickerPickerState) {
	if picker == nil {
		return
	}
	columns := max(1, picker.Columns)
	rows := max(1, picker.VisibleRows)
	selectedRow := max(0, picker.Selected) / columns
	if selectedRow < picker.FirstRow {
		picker.FirstRow = selectedRow
	}
	if selectedRow >= picker.FirstRow+rows {
		picker.FirstRow = selectedRow - rows + 1
	}
	maxRow := 0
	if len(picker.Catalog) > 0 {
		maxRow = (len(picker.Catalog) - 1) / columns
	}
	picker.FirstRow = max(0, min(picker.FirstRow, max(0, maxRow-rows+1)))
}

func sendSelectedSticker(state *State, event ActionReceived) []Effect {
	picker := state.StickerPicker
	if picker == nil || event.RequestID != picker.RequestID || picker.Loading || picker.Error != nil {
		return nil
	}
	chatID, ok := activeChatID(*state)
	if !ok || chatID != picker.ChatID || state.Connection != domain.ConnectionOnline || state.EditTarget != nil {
		return nil
	}
	chatIndex := chatIndex(state.Chats, chatID)
	if chatIndex < 0 || !state.Chats[chatIndex].CanSend {
		return nil
	}
	selected := picker.Selected
	if event.StickerFileID != 0 {
		selected = -1
		for i, sticker := range picker.Catalog {
			if sticker.File.ID == event.StickerFileID {
				selected = i
				break
			}
		}
	}
	if selected < 0 || selected >= len(picker.Catalog) {
		return nil
	}
	sticker := picker.Catalog[selected]
	if sticker.File.ID == 0 {
		return nil
	}
	replyID := domain.MessageID(0)
	if state.ReplyTarget != nil && state.ReplyTarget.ChatID == chatID {
		replyID = state.ReplyTarget.MessageID
	}
	localID := allocateLocalID(state)
	requestID := allocateRequestID(state)
	message := domain.Message{
		ID: localID, ChatID: chatID, TopicID: picker.TopicID, SentAt: event.At, Kind: domain.MessageSticker,
		Sticker: sticker, Outgoing: true, SendState: domain.SendPending,
		Media: domain.MessageMedia{File: sticker.File, Thumbnail: sticker.Thumbnail, Width: sticker.Width, Height: sticker.Height},
	}
	if replyID > 0 {
		message.HasReply = true
		message.ReplyToMessageID = replyID
	}
	state.Messages[chatID] = limitMessages(mergeMessages(state.Messages[chatID], []domain.Message{message}))
	if block := state.StickerThumbnails[sticker.File.ID]; block.Width > 0 && block.Height > 0 {
		if state.Thumbnails[chatID] == nil {
			state.Thumbnails[chatID] = make(map[domain.MessageID]thumbnail.Block)
		}
		state.Thumbnails[chatID][localID] = block
	}
	if state.ReplyTarget != nil && state.ReplyTarget.ChatID == chatID {
		state.ReplyTarget = nil
		delete(state.DraftReplies, chatID)
	}
	state.StickerPicker = nil
	state.StickerThumbnailRequests = make(map[int32]uint64)
	state.Focus = FocusConversation
	state.SelectedMessageChat = chatID
	state.SelectedMessage = localID
	if state.StickerSendRequests == nil {
		state.StickerSendRequests = make(map[domain.MessageID]uint64)
	}
	state.StickerSendRequests[localID] = requestID
	commands := []Effect{SendSticker{
		RequestID: requestID, LocalID: localID, ChatID: chatID, TopicID: picker.TopicID,
		Sticker: sticker, ReplyToMessageID: replyID,
	}}
	if replyID > 0 {
		commands = append(commands, queueDraftSave(state, chatID))
	}
	return commands
}

func stickerCorrelation(state State, requestID uint64, localID domain.MessageID, chatID domain.ChatID) (int, bool) {
	if chatID == 0 || state.StickerSendRequests[localID] != requestID {
		return -1, false
	}
	index := messageIndex(state.Messages[chatID], localID)
	if index < 0 {
		return -1, false
	}
	message := state.Messages[chatID][index]
	return index, message.Kind == domain.MessageSticker && message.SendState == domain.SendPending
}

func replaceQueuedSticker(state *State, event StickerQueued) bool {
	index, ok := stickerCorrelation(*state, event.RequestID, event.LocalID, event.ChatID)
	if !ok || event.Message.ID == 0 || event.Message.Kind != domain.MessageSticker || (event.Message.ChatID != 0 && event.Message.ChatID != event.ChatID) {
		return false
	}
	original := state.Messages[event.ChatID][index]
	replacement := event.Message
	replacement.ChatID = event.ChatID
	replacement.Kind = domain.MessageSticker
	replacement.Outgoing = true
	replacement.SendState = domain.SendPending
	replacement.Failure = nil
	mergeStickerMedia(original, &replacement)
	if replacement.SentAt.IsZero() {
		replacement.SentAt = original.SentAt
	}
	preserveReplyIdentity(original, &replacement)
	preserveTopicIdentity(original, &replacement)
	moveThumbnail(state, event.ChatID, event.LocalID, replacement.ID)
	removeMessageAt(state, event.ChatID, index)
	state.Messages[event.ChatID] = limitMessages(mergeMessages(state.Messages[event.ChatID], []domain.Message{replacement}))
	if state.SelectedMessage == event.LocalID {
		state.SelectedMessage = replacement.ID
	}
	reconcileMenuIdentity(state, event.ChatID, event.LocalID, replacement)
	delete(state.StickerSendRequests, event.LocalID)
	return true
}

func failQueuedSticker(state *State, event StickerQueueFailed) {
	index, ok := stickerCorrelation(*state, event.RequestID, event.LocalID, event.ChatID)
	if !ok {
		return
	}
	message := &state.Messages[event.ChatID][index]
	message.SendState = domain.SendFailed
	failure := event.Error
	message.Failure = &failure
	message.RetryAt = event.FailedAt.Add(event.Error.RetryAfter)
	delete(state.StickerSendRequests, event.LocalID)
}

func mergeStickerMedia(original domain.Message, replacement *domain.Message) {
	if replacement == nil {
		return
	}
	if replacement.Sticker.File.ID == 0 {
		replacement.Sticker.File = original.Sticker.File
	}
	if replacement.Sticker.Thumbnail.ID == 0 {
		replacement.Sticker.Thumbnail = original.Sticker.Thumbnail
	}
	if replacement.Sticker.Width == 0 {
		replacement.Sticker.Width = original.Sticker.Width
	}
	if replacement.Sticker.Height == 0 {
		replacement.Sticker.Height = original.Sticker.Height
	}
	if replacement.Sticker.Emoji == "" {
		replacement.Sticker.Emoji = original.Sticker.Emoji
	}
	if replacement.Media.File.ID == 0 {
		replacement.Media.File = original.Media.File
	}
	if replacement.Media.Thumbnail.ID == 0 {
		replacement.Media.Thumbnail = original.Media.Thumbnail
	}
	if replacement.Media.Width == 0 {
		replacement.Media.Width = original.Media.Width
	}
	if replacement.Media.Height == 0 {
		replacement.Media.Height = original.Media.Height
	}
}

func moveThumbnail(state *State, chatID domain.ChatID, oldID, newID domain.MessageID) {
	if oldID == newID || newID == 0 || state.Thumbnails[chatID] == nil {
		return
	}
	if block, ok := state.Thumbnails[chatID][oldID]; ok {
		state.Thumbnails[chatID][newID] = block
		delete(state.Thumbnails[chatID], oldID)
	}
}
