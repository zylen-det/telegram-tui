package frontend

import (
	"image"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type ChatRow struct {
	Chat        domain.Chat
	Draft       domain.Draft
	Focused     bool
	Selected    bool
	AvatarKey   string
	Avatar      pixel.Avatar
	AvatarError *domain.AppError
}

type ViewModel struct {
	Width                  int
	Height                 int
	Layout                 ViewLayout
	Focus                  Focus
	Connection             domain.ConnectionState
	Chats                  []ChatRow
	ChatsLoading           bool
	ChatsLoaded            bool
	ChatsError             *domain.AppError
	ActiveChat             domain.Chat
	Topics                 *TopicListState
	ShowAllTopics          bool
	ActiveTopic            domain.ForumTopic
	ActiveTopicKnown       bool
	Groups                 []RenderedMessageGroup
	HistoryOffset          int
	HistoryFollowSelection bool
	HistoryLoading         bool
	HistoryDone            bool
	HistoryError           *domain.AppError
	Draft                  string
	DetailsOpen            bool
	DetailsChat            domain.Chat
	DetailsSelected        int
	MemberDetailAvatar     pixel.Avatar
	Modal                  *ModalState
	ModalImage             image.Rectangle
	Prompt                 *PromptState
	Toast                  *domain.AppError
	ToastGeneration        uint64
	ToastDuration          time.Duration
	SelectedMessageChat    domain.ChatID
	SelectedMessage        domain.MessageID
	MessageMenu            *MessageActionMenu
	ForwardPicker          *ForwardPicker
	ReactionPicker         *ReactionPicker
	ReplyTarget            *ReplyTarget
	EditTarget             *EditTarget
	PhotoSend              *PhotoSendState
	StickerPicker          *StickerPickerState
	CommandMenu            *CommandMenuState
	MessageSearch          *MessageSearchState
	ChatSearch             *ChatSearchState
	PinnedMessages         *PinnedMessagesState
	Members                *MembersState
	InviteLinks            *InviteLinksState
	Administration         *AdministrationState
	ChatSettings           *ChatSettingsState
	ChatActions            *ChatActionMenuState
	StickerThumbnails      []RenderedStickerThumbnail
	InlineThumbnails       []RenderedThumbnail
}

// RenderedThumbnail is the per-message half-block thumbnail carried by the
// view model for photo, video-poster, and Sticker messages whose app-side thumbnail render has completed.
// Width and Height are the cell geometry; Text is the multi-line ANSI string.
type RenderedStickerThumbnail struct {
	StickerFileID int32
	Block         thumbnail.Block
}

type RenderedThumbnail struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Block     thumbnail.Block
}

type RenderedMessageGroup struct {
	MessageGroup
	AvatarKey   string
	Avatar      pixel.Avatar
	AvatarError *domain.AppError
}

func Select(state State, location *time.Location) ViewModel {
	if location == nil {
		location = time.Local
	}

	model := ViewModel{
		Width:               state.Width,
		Height:              state.Height,
		Layout:              ComputeLayout(state.Width, state.Height, state.DetailsOpen, state.Focus),
		Focus:               state.Focus,
		Connection:          state.Connection,
		DetailsOpen:         state.DetailsOpen,
		Modal:               cloneModal(state.Modal),
		Prompt:              clonePrompt(state.Prompt),
		Toast:               cloneAppError(state.Toast),
		ToastGeneration:     state.ToastGeneration,
		ToastDuration:       state.ToastDuration,
		SelectedMessageChat: state.SelectedMessageChat,
		SelectedMessage:     state.SelectedMessage,
		MessageMenu:         cloneMessageMenu(state.MessageMenu),
		ForwardPicker:       cloneForwardPicker(state.ForwardPicker),
		ReactionPicker:      cloneReactionPicker(state.ReactionPicker),
		ReplyTarget:         cloneReplyTarget(state.ReplyTarget),
		EditTarget:          cloneEditTarget(state.EditTarget),
		PhotoSend:           clonePhotoSend(state.PhotoSend),
		StickerPicker:       cloneStickerPicker(state.StickerPicker),
		CommandMenu:         cloneCommandMenu(state.CommandMenu),
		MessageSearch:       cloneMessageSearch(state.MessageSearch),
		ChatSearch:          cloneChatSearch(state.ChatSearch),
		PinnedMessages:      clonePinnedMessages(state.PinnedMessages),
		Members:             cloneMembers(state.Members),
		InviteLinks:         cloneInviteLinks(state.InviteLinks),
		Administration:      cloneAdministration(state.Administration),
		ChatSettings:        cloneChatSettings(state.ChatSettings),
		ChatActions:         cloneChatActions(state.ChatActions),
		Topics:              cloneTopics(state.Topics),
		DetailsSelected:     state.DetailsSelected,
		MemberDetailAvatar:  selectMemberDetailAvatar(state),
		Chats:               make([]ChatRow, len(state.Chats)),
		ChatsLoading:        state.ChatsLoading,
		ChatsLoaded:         state.ChatsLoaded,
		ChatsError:          cloneAppError(state.ChatsError),
	}

	for fileID, block := range state.StickerThumbnails {
		model.StickerThumbnails = append(model.StickerThumbnails, RenderedStickerThumbnail{StickerFileID: fileID, Block: block})
	}

	focusedChat := focusedChatIndex(state)
	detailsIndex := detailsChatIndex(state)
	for index, chat := range state.Chats {
		key := chat.Avatar.UniqueID + ":chat-list"
		avatar := state.Avatars[key]
		model.Chats[index] = ChatRow{
			Chat: chat,
			Draft: domain.Draft{
				Text:             state.Drafts[chat.ID],
				ReplyToMessageID: state.DraftReplies[chat.ID],
				Date:             state.DraftDates[chat.ID],
			},
			Focused:     index == focusedChat,
			Selected:    index == state.SelectedChat,
			AvatarKey:   key,
			Avatar:      cloneAvatar(avatar.Cells),
			AvatarError: cloneAppError(avatar.Error),
		}
		if index == detailsIndex {
			model.DetailsChat = chat
		}
	}

	if state.SelectedChat < 0 || state.SelectedChat >= len(state.Chats) {
		return model
	}

	model.ActiveChat = state.Chats[state.SelectedChat]
	model.ShowAllTopics = state.ShowAll[model.ActiveChat.ID]
	topicID := state.SelectedTopics[model.ActiveChat.ID]
	topicKey := TopicKey{ChatID: model.ActiveChat.ID, TopicID: topicID}
	// ALL-messages mode ignores even a stale selected topic.
	model.ActiveTopic, model.ActiveTopicKnown = visibleConversationTopic(state, model.ActiveChat.ID)
	messages := visibleConversationMessages(state, model.ActiveChat.ID)
	groups := GroupMessages(model.ActiveChat.Kind, messages, location)
	model.Groups = make([]RenderedMessageGroup, len(groups))
	for index, group := range groups {
		key := group.SenderAvatar.UniqueID + ":message-group"
		avatar := state.Avatars[key]
		model.Groups[index] = RenderedMessageGroup{
			MessageGroup: cloneMessageGroup(group),
			AvatarKey:    key,
			Avatar:       cloneAvatar(avatar.Cells),
			AvatarError:  cloneAppError(avatar.Error),
		}
	}
	model.Draft = state.Drafts[model.ActiveChat.ID]
	if state.EditTarget != nil && state.EditTarget.ChatID == model.ActiveChat.ID {
		model.Draft = state.EditTarget.Buffer
	}
	history := state.History[model.ActiveChat.ID]
	if model.ActiveTopicKnown {
		model.Draft = state.TopicDrafts[topicKey]
		if state.EditTarget != nil && state.EditTarget.ChatID == model.ActiveChat.ID {
			model.Draft = state.EditTarget.Buffer
		}
		history = state.TopicHistory[topicKey]
	} else if model.ShowAllTopics {
		// ALL-messages mode: the ALL draft lives in the TopicID-0 family;
		// history stays on the shared chat-level path.
		model.Draft = state.TopicDrafts[TopicKey{ChatID: model.ActiveChat.ID, TopicID: 0}]
		if state.EditTarget != nil && state.EditTarget.ChatID == model.ActiveChat.ID {
			model.Draft = state.EditTarget.Buffer
		}
	}
	model.HistoryOffset = history.ViewOffset
	model.HistoryFollowSelection = history.FollowSelection
	model.HistoryLoading = history.Loading
	model.HistoryDone = history.Done
	model.HistoryError = cloneAppError(history.Error)
	if inner := state.Thumbnails[model.ActiveChat.ID]; len(inner) > 0 {
		for msgID, block := range inner {
			model.InlineThumbnails = append(model.InlineThumbnails, RenderedThumbnail{
				ChatID:    model.ActiveChat.ID,
				MessageID: msgID,
				Block:     block,
			})
		}
	}
	return model
}

func cloneChatActions(menu *ChatActionMenuState) *ChatActionMenuState {
	if menu == nil {
		return nil
	}
	clone := *menu
	return &clone
}

func cloneModal(modal *ModalState) *ModalState {
	if modal == nil {
		return nil
	}
	clone := *modal
	clone.Error = cloneAppError(modal.Error)
	return &clone
}

func clonePrompt(prompt *PromptState) *PromptState {
	if prompt == nil {
		return nil
	}
	clone := *prompt
	clone.Input = append([]rune(nil), prompt.Input...)
	return &clone
}

func cloneMessageMenu(menu *MessageActionMenu) *MessageActionMenu {
	if menu == nil {
		return nil
	}
	clone := *menu
	return &clone
}

func cloneForwardPicker(picker *ForwardPicker) *ForwardPicker {
	if picker == nil {
		return nil
	}
	clone := *picker
	return &clone
}

func cloneReactionPicker(picker *ReactionPicker) *ReactionPicker {
	if picker == nil {
		return nil
	}
	clone := *picker
	return &clone
}

func cloneReplyTarget(target *ReplyTarget) *ReplyTarget {
	if target == nil {
		return nil
	}
	clone := *target
	return &clone
}

func cloneEditTarget(target *EditTarget) *EditTarget {
	if target == nil {
		return nil
	}
	clone := *target
	clone.Error = cloneAppError(target.Error)
	return &clone
}

func cloneStickerPicker(picker *StickerPickerState) *StickerPickerState {
	if picker == nil {
		return nil
	}
	clone := *picker
	clone.Catalog = append([]domain.StickerRef(nil), picker.Catalog...)
	clone.Error = cloneAppError(picker.Error)
	return &clone
}

func cloneCommandMenu(menu *CommandMenuState) *CommandMenuState {
	if menu == nil {
		return nil
	}
	clone := *menu
	clone.Candidates = append([]domain.BotCommand(nil), menu.Candidates...)
	clone.Error = cloneAppError(menu.Error)
	return &clone
}

func cloneMessageSearch(search *MessageSearchState) *MessageSearchState {
	if search == nil {
		return nil
	}
	clone := *search
	clone.Input = append([]rune(nil), search.Input...)
	clone.Results = append([]domain.Message(nil), search.Results...)
	for index := range clone.Results {
		clone.Results[index].Failure = cloneAppError(search.Results[index].Failure)
		clone.Results[index].Reactions = append([]domain.MessageReaction(nil), search.Results[index].Reactions...)
	}
	clone.Error = cloneAppError(search.Error)
	return &clone
}

func cloneChatSearch(search *ChatSearchState) *ChatSearchState {
	if search == nil {
		return nil
	}
	clone := *search
	clone.Input = append([]rune(nil), search.Input...)
	clone.LocalChats = append([]domain.Chat(nil), search.LocalChats...)
	clone.PublicChats = append([]domain.Chat(nil), search.PublicChats...)
	clone.GlobalMessages = append([]domain.Message(nil), search.GlobalMessages...)
	clone.PublicError = cloneAppError(search.PublicError)
	clone.MessagesError = cloneAppError(search.MessagesError)
	return &clone
}

// selectMemberDetailAvatar resolves the open detail member's rendered avatar
// cells from the shared cache. Absent, loading, or failed entries yield no
// avatar and the detail view falls back to text info.
func selectMemberDetailAvatar(state State) pixel.Avatar {
	if state.Members == nil || state.Members.Detail == nil {
		return pixel.Avatar{}
	}
	key := state.Members.Detail.AvatarKey
	if key == "" {
		return pixel.Avatar{}
	}
	entry, ok := state.Avatars[key]
	if !ok || entry.Loading || len(entry.Cells.Cells) == 0 {
		return pixel.Avatar{}
	}
	// Failed renders carry deterministic placeholder cells, matching the
	// Info pane fallback.
	return cloneAvatar(entry.Cells)
}

func cloneMembers(members *MembersState) *MembersState {
	if members == nil {
		return nil
	}
	clone := *members
	clone.Results = append([]domain.ChatMember(nil), members.Results...)
	clone.Error = cloneAppError(members.Error)
	if members.Detail != nil {
		detail := *members.Detail
		clone.Detail = &detail
	}
	return &clone
}

func cloneInviteLinks(links *InviteLinksState) *InviteLinksState {
	if links == nil {
		return nil
	}
	clone := *links
	clone.Links = append([]telegram.InviteLink(nil), links.Links...)
	clone.Error = cloneAppError(links.Error)
	if links.Primary != nil {
		primary := *links.Primary
		clone.Primary = &primary
	}
	return &clone
}

func cloneAdministration(admin *AdministrationState) *AdministrationState {
	if admin == nil {
		return nil
	}
	clone := *admin
	clone.Error = cloneAppError(admin.Error)
	if admin.Snapshot != nil {
		snapshot := *admin.Snapshot
		clone.Snapshot = &snapshot
	}
	if admin.MemberStatus != nil {
		member := *admin.MemberStatus
		clone.MemberStatus = &member
	}
	if admin.ReturnMembers != nil {
		clone.ReturnMembers = cloneMembers(admin.ReturnMembers)
	}
	return &clone
}

func cloneChatSettings(settings *ChatSettingsState) *ChatSettingsState {
	if settings == nil {
		return nil
	}
	clone := *settings
	clone.TitleInput = append([]rune(nil), settings.TitleInput...)
	clone.DescriptionInput = append([]rune(nil), settings.DescriptionInput...)
	clone.Error = cloneAppError(settings.Error)
	if settings.Snapshot != nil {
		snapshot := *settings.Snapshot
		clone.Snapshot = &snapshot
	}
	return &clone
}

func clonePinnedMessages(pinned *PinnedMessagesState) *PinnedMessagesState {
	if pinned == nil {
		return nil
	}
	clone := *pinned
	clone.Results = append([]domain.Message(nil), pinned.Results...)
	for index := range clone.Results {
		clone.Results[index].Failure = cloneAppError(pinned.Results[index].Failure)
		clone.Results[index].Reactions = append([]domain.MessageReaction(nil), pinned.Results[index].Reactions...)
	}
	clone.Error = cloneAppError(pinned.Error)
	return &clone
}

func clonePhotoSend(ps *PhotoSendState) *PhotoSendState {
	if ps == nil {
		return nil
	}
	clone := *ps
	clone.Input = append([]rune(nil), ps.Input...)
	return &clone
}

func cloneTopics(topics *TopicListState) *TopicListState {
	if topics == nil {
		return nil
	}
	clone := *topics
	clone.Results = append([]domain.ForumTopic(nil), topics.Results...)
	clone.Error = cloneAppError(topics.Error)
	return &clone
}

func cloneAppError(appError *domain.AppError) *domain.AppError {
	if appError == nil {
		return nil
	}
	clone := *appError
	clone.Cause = nil
	return &clone
}

func cloneAvatar(avatar pixel.Avatar) pixel.Avatar {
	avatar.Cells = append([]pixel.Cell(nil), avatar.Cells...)
	return avatar
}

func cloneMessageGroup(group MessageGroup) MessageGroup {
	group.Messages = append([]domain.Message(nil), group.Messages...)
	contexts := group.ReplyContexts
	group.ReplyContexts = make(map[domain.MessageID]ReplyContext, len(contexts))
	for messageID, context := range contexts {
		group.ReplyContexts[messageID] = context
	}
	for index := range group.Messages {
		group.Messages[index].Failure = cloneAppError(group.Messages[index].Failure)
	}
	return group
}
