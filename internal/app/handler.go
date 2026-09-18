package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/platform"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type RuntimeResolver interface {
	Resolve(context.Context) (config.Runtime, error)
}

type ClientFactory func(config.Runtime, auth.Prompter) (telegram.Client, error)

type AvatarRenderer interface {
	Render(context.Context, string, domain.AvatarRef, avatar.Role) (pixel.Avatar, error)
}

const (
	thumbnailWidth  = 40
	thumbnailHeight = 40
)

type draftSaveWork struct {
	command SaveDraft
	emit    func(Event)
}

type Handler struct {
	mutex      sync.Mutex
	client     telegram.Client
	resolver   RuntimeResolver
	factory    ClientFactory
	prompts    *auth.Broker
	avatars    AvatarRenderer
	clipboard  platform.Clipboard
	notifier   platform.Notifier
	thumbnails thumbnail.Renderer
	logger     *slog.Logger
	workCtx    context.Context
	cancelWork context.CancelFunc
	now        func() time.Time
	shutdown   sync.Once
	startOnce  sync.Once
	startBegan chan struct{}

	draftMutex   sync.Mutex
	draftPending map[topicKey]draftSaveWork
	draftRunning map[topicKey]bool
}

// SetLogger wires a slog logger for trace logging.
func (h *Handler) SetLogger(l *slog.Logger) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if l == nil {
		h.logger = slog.Default()
	} else {
		h.logger = l
	}
}

func (h *Handler) SetThumbnailRenderer(r thumbnail.Renderer) {
	h.thumbnails = r
}

// SetNotifier injects a platform.Notifier for desktop notification support.
// When the notifier is nil or missing, desktop notification commands are
// silently dropped — the feature is best-effort.
func (h *Handler) SetNotifier(n platform.Notifier) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.notifier = n
}

func NewHandler(processCtx context.Context, resolver RuntimeResolver, factory ClientFactory, prompts *auth.Broker, avatars AvatarRenderer, clipboard ...platform.Clipboard) *Handler {
	if processCtx == nil {
		processCtx = context.Background()
	}
	workCtx, cancelWork := context.WithCancel(processCtx)
	handler := &Handler{
		resolver:     resolver,
		factory:      factory,
		prompts:      prompts,
		avatars:      avatars,
		workCtx:      workCtx,
		cancelWork:   cancelWork,
		now:          time.Now,
		startBegan:   make(chan struct{}),
		draftPending: make(map[topicKey]draftSaveWork),
		draftRunning: make(map[topicKey]bool),
	}
	if len(clipboard) > 0 {
		handler.clipboard = clipboard[0]
	}
	return handler
}

func (h *Handler) Handle(_ context.Context, command Command, emit func(Event)) {
	switch command := command.(type) {
	case LoadBootstrap:
		h.loadBootstrap(emit)
	case LoadChats:
		client, err := h.currentClient("load chats")
		if err != nil {
			emit(ChatsLoadFailed{RequestID: command.RequestID, Error: *err})
			return
		}
		page, loadErr := client.LoadChats(h.workCtx, command.Cursor)
		if loadErr != nil {
			emit(ChatsLoadFailed{RequestID: command.RequestID, Error: safeError("load chats", loadErr)})
			return
		}
		emit(ChatsLoaded{RequestID: command.RequestID, Page: page})
	case LoadMessages:
		client, err := h.currentClient("load messages")
		if err != nil {
			emit(MessagesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: *err})
			return
		}
		cursor := command.Cursor
		cursor.TopicID = command.TopicID
		page, loadErr := client.LoadMessages(h.workCtx, command.ChatID, cursor)
		if loadErr != nil {
			emit(MessagesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: safeError("load messages", loadErr)})
			return
		}
		emit(MessagesLoaded{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Page: page})
	case LoadTopics:
		client, err := h.currentClient("load topics")
		if err != nil {
			emit(TopicsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: topicsLoadError()})
			return
		}
		page, loadErr := client.LoadTopics(h.workCtx, command.ChatID, command.Cursor)
		if loadErr != nil {
			emit(TopicsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: topicsLoadError()})
			return
		}
		emit(TopicsLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Page: page})
	case SearchChatMessages:
		client, err := h.currentClient("search messages")
		if err != nil {
			emit(ChatMessagesSearchFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: messageSearchError()})
			return
		}
		cursor := command.Cursor
		cursor.TopicID = command.TopicID
		page, searchErr := client.SearchChatMessages(h.workCtx, command.ChatID, command.Query, cursor)
		if searchErr != nil {
			emit(ChatMessagesSearchFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: messageSearchError()})
			return
		}
		emit(ChatMessagesSearched{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Page: page})
	case SearchPublicChatCommand:
		client, err := h.currentClient("search public chat")
		if err != nil {
			emit(PublicChatSearchFailed{RequestID: command.RequestID, Error: chatSearchError()})
			return
		}
		chat, searchErr := client.SearchPublicChat(h.workCtx, command.Username)
		if searchErr != nil {
			emit(PublicChatSearchFailed{RequestID: command.RequestID, Error: chatSearchError()})
			return
		}
		emit(PublicChatSearched{RequestID: command.RequestID, Chat: chat})
	case SearchPublicChatsCommand:
		client, err := h.currentClient("search public chats")
		if err != nil {
			emit(PublicChatsSearchFailed{RequestID: command.RequestID, Error: domain.AppError{Kind: domain.ErrorNetwork, Op: "search public chats", Message: "Could not search public chats"}})
			return
		}
		chats, searchErr := client.SearchPublicChats(h.workCtx, command.Query)
		if searchErr != nil {
			emit(PublicChatsSearchFailed{RequestID: command.RequestID, Error: domain.AppError{Kind: domain.ErrorNetwork, Op: "search public chats", Message: "Could not search public chats"}})
			return
		}
		emit(PublicChatsSearched{RequestID: command.RequestID, Chats: chats})
	case SearchAllMessagesCommand:
		client, err := h.currentClient("search all messages")
		if err != nil {
			emit(AllMessagesSearchFailed{RequestID: command.RequestID, Error: domain.AppError{Kind: domain.ErrorNetwork, Op: "search all messages", Message: "Could not search messages"}})
			return
		}
		page, searchErr := client.SearchAllMessages(h.workCtx, command.Query, command.Limit)
		if searchErr != nil {
			emit(AllMessagesSearchFailed{RequestID: command.RequestID, Error: domain.AppError{Kind: domain.ErrorNetwork, Op: "search all messages", Message: "Could not search messages"}})
			return
		}
		emit(AllMessagesSearched{RequestID: command.RequestID, Messages: page.Messages, TotalCount: page.TotalCount})
	case LoadMembers:
		client, err := h.currentClient("load members")
		if err != nil {
			emit(MembersLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: membersError()})
			return
		}
		page, loadErr := client.LoadMembers(h.workCtx, command.ChatID, command.Cursor)
		if loadErr != nil {
			emit(MembersLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: membersError()})
			return
		}
		emit(MembersLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Page: page})
	case LoadInviteLinksCommand:
		client, err := h.currentClient("load invite links")
		if err != nil {
			emit(InviteLinksLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		linksClient, ok := client.(telegram.InviteLinksClient)
		if !ok {
			emit(InviteLinksLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		page, loadErr := linksClient.LoadInviteLinks(h.workCtx, command.ChatID, command.Cursor)
		if loadErr != nil {
			emit(InviteLinksLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		emit(InviteLinksLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Page: page})
	case CreateInviteLinkCommand:
		client, err := h.currentClient("create invite link")
		if err != nil {
			emit(InviteLinkCreateFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		linksClient, ok := client.(telegram.InviteLinksClient)
		if !ok {
			emit(InviteLinkCreateFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		link, createErr := linksClient.CreateInviteLink(h.workCtx, command.ChatID, command.Name)
		if createErr != nil {
			emit(InviteLinkCreateFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		emit(InviteLinkCreated{RequestID: command.RequestID, ChatID: command.ChatID, Link: link})
	case RevokeInviteLinkCommand:
		client, err := h.currentClient("revoke invite link")
		if err != nil {
			emit(InviteLinkRevokeFailed{RequestID: command.RequestID, ChatID: command.ChatID, URL: command.URL})
			return
		}
		linksClient, ok := client.(telegram.InviteLinksClient)
		if !ok || linksClient.RevokeInviteLink(h.workCtx, command.ChatID, command.URL) != nil {
			emit(InviteLinkRevokeFailed{RequestID: command.RequestID, ChatID: command.ChatID, URL: command.URL})
			return
		}
		emit(InviteLinkRevoked{RequestID: command.RequestID, ChatID: command.ChatID, URL: command.URL})
	case CopyInviteLinkCommand:
		if h.clipboard == nil || h.clipboard.WriteText(h.workCtx, command.URL) != nil {
			emit(InviteLinkCopyFailed{RequestID: command.RequestID, ChatID: command.ChatID, URL: command.URL})
			return
		}
		emit(InviteLinkCopied{RequestID: command.RequestID, ChatID: command.ChatID, URL: command.URL})
	case LoadAdministrationCommand:
		client, err := h.currentClient("load administration")
		if err != nil {
			emit(AdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		c, ok := client.(telegram.AdministrationClient)
		if !ok {
			emit(AdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		v, e := c.LoadAdministration(h.workCtx, command.ChatID)
		if e != nil {
			emit(AdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		emit(AdministrationLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Snapshot: v})
	case LoadChatSettingsCommand:
		client, err := h.currentClient("load chat settings")
		if err != nil {
			emit(ChatSettingsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		c, ok := client.(telegram.ChatSettingsClient)
		if !ok {
			emit(ChatSettingsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		v, e := c.LoadChatSettings(h.workCtx, command.ChatID)
		if e != nil {
			emit(ChatSettingsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		emit(ChatSettingsLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Snapshot: v})
	case SaveChatSettingCommand:
		client, err := h.currentClient("save chat setting")
		c, ok := client.(telegram.ChatSettingsClient)
		failed := err != nil || !ok
		if !failed {
			switch command.Field {
			case ChatSettingTitle:
				failed = c.SetChatTitle(h.workCtx, command.ChatID, command.Value) != nil
			case ChatSettingDescription:
				failed = c.SetChatDescription(h.workCtx, command.ChatID, command.Value) != nil
			case ChatSettingSlowMode:
				failed = c.SetChatSlowModeDelay(h.workCtx, command.ChatID, command.Delay) != nil
			default:
				failed = true
			}
		}
		if failed {
			emit(ChatSettingSaveFailed{RequestID: command.RequestID, ChatID: command.ChatID, Field: command.Field})
			return
		}
		emit(ChatSettingSaved{RequestID: command.RequestID, ChatID: command.ChatID, Field: command.Field})
	case LoadMemberAdministrationCommand:
		client, err := h.currentClient("load member administration")
		if err != nil {
			emit(MemberAdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID})
			return
		}
		c, ok := client.(telegram.AdministrationClient)
		if !ok {
			emit(MemberAdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID})
			return
		}
		v, e := c.LoadMemberAdministration(h.workCtx, command.ChatID, command.UserID)
		if e != nil {
			emit(MemberAdministrationLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID})
			return
		}
		emit(MemberAdministrationLoaded{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Status: v})
	case SetDefaultChatPermissionsCommand:
		client, err := h.currentClient("save permissions")
		if err != nil {
			emit(DefaultPermissionsSaveFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		c, ok := client.(telegram.AdministrationClient)
		if !ok || c.SetDefaultChatPermissions(h.workCtx, command.ChatID, command.Permissions) != nil {
			emit(DefaultPermissionsSaveFailed{RequestID: command.RequestID, ChatID: command.ChatID})
			return
		}
		emit(DefaultPermissionsSaved{RequestID: command.RequestID, ChatID: command.ChatID})
	case ApplyMemberAdministrationCommand:
		client, err := h.currentClient("apply member administration")
		req := command.Request
		if err != nil {
			emit(MemberAdministrationApplyFailed{RequestID: command.RequestID, ChatID: req.ChatID, UserID: req.UserID, Action: req.Action})
			return
		}
		c, ok := client.(telegram.AdministrationClient)
		if !ok || c.ApplyMemberAdministration(h.workCtx, req) != nil {
			emit(MemberAdministrationApplyFailed{RequestID: command.RequestID, ChatID: req.ChatID, UserID: req.UserID, Action: req.Action})
			return
		}
		emit(MemberAdministrationApplied{RequestID: command.RequestID, ChatID: req.ChatID, UserID: req.UserID, Action: req.Action})
	case LoadUserInfo:
		client, err := h.currentClient("load user")
		if err != nil {
			emit(UserInfoLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Error: userInfoError()})
			return
		}
		user, loadErr := client.LoadUser(h.workCtx, command.UserID)
		if loadErr != nil {
			emit(UserInfoLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Error: userInfoError()})
			return
		}
		emit(UserInfoLoaded{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, User: user})
	case CopyMemberUsernameCommand:
		if h.clipboard == nil {
			emit(MemberUsernameCopyFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Error: memberCopyError()})
			return
		}
		if err := h.clipboard.WriteText(h.workCtx, command.Text); err != nil {
			emit(MemberUsernameCopyFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Error: memberCopyError()})
			return
		}
		emit(MemberUsernameCopied{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID})
	case AddMemberContactCommand:
		client, err := h.currentClient("add contact")
		if err != nil {
			emit(MemberContactFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: true, Error: memberContactError(true)})
			return
		}
		if err := client.AddContact(h.workCtx, telegram.AddContactRequest{UserID: command.UserID, FirstName: command.FirstName, LastName: command.LastName}); err != nil {
			emit(MemberContactFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: true, Error: memberContactError(true)})
			return
		}
		emit(MemberContactChanged{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: true})
	case RemoveMemberContactCommand:
		client, err := h.currentClient("remove contact")
		if err != nil {
			emit(MemberContactFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: false, Error: memberContactError(false)})
			return
		}
		if err := client.RemoveContact(h.workCtx, telegram.RemoveContactRequest{UserID: command.UserID}); err != nil {
			emit(MemberContactFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: false, Error: memberContactError(false)})
			return
		}
		emit(MemberContactChanged{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Added: false})
	case SetMemberBlockedCommand:
		client, err := h.currentClient("block user")
		if err != nil {
			emit(MemberBlockFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Blocked: command.Blocked, Error: memberBlockError(command.Blocked)})
			return
		}
		if err := client.SetUserBlocked(h.workCtx, telegram.SetUserBlockedRequest{UserID: command.UserID, Blocked: command.Blocked}); err != nil {
			emit(MemberBlockFailed{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Blocked: command.Blocked, Error: memberBlockError(command.Blocked)})
			return
		}
		emit(MemberBlockChanged{RequestID: command.RequestID, ChatID: command.ChatID, UserID: command.UserID, Blocked: command.Blocked})
	case ApplyChatActionCommand:
		client, err := h.currentClient("update chat")
		if err != nil {
			emit(ChatActionFailed{RequestID: command.RequestID, ChatID: command.ChatID, Action: command.Action, Error: chatActionError(command.Action)})
			return
		}
		if err := client.ApplyChatAction(h.workCtx, telegram.ChatActionRequest{ChatID: command.ChatID, Action: command.Action}); err != nil {
			emit(ChatActionFailed{RequestID: command.RequestID, ChatID: command.ChatID, Action: command.Action, Error: chatActionError(command.Action)})
			return
		}
		emit(ChatActionApplied{RequestID: command.RequestID, ChatID: command.ChatID, Action: command.Action})
	case LoadPinnedMessages:
		client, err := h.currentClient("load pinned messages")
		if err != nil {
			emit(PinnedMessagesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: pinnedSearchError()})
			return
		}
		cursor := command.Cursor
		cursor.TopicID = command.TopicID
		page, searchErr := client.SearchPinnedMessages(h.workCtx, command.ChatID, cursor)
		if searchErr != nil {
			emit(PinnedMessagesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Error: pinnedSearchError()})
			return
		}
		emit(PinnedMessagesLoaded{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, Page: page})
	case LoadPinnedMessageContext:
		client, err := h.currentClient("load pinned message context")
		if err != nil {
			emit(PinnedMessageContextFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Error: pinnedContextError()})
			return
		}
		var (
			page       telegram.MessagePage
			contextErr error
		)
		if command.TopicID != 0 {
			page, contextErr = client.LoadTopicMessageContext(h.workCtx, command.ChatID, command.TopicID, command.MessageID)
		} else {
			page, contextErr = client.LoadMessageContext(h.workCtx, command.ChatID, command.MessageID)
		}
		if contextErr != nil {
			emit(PinnedMessageContextFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Error: pinnedContextError()})
			return
		}
		emit(PinnedMessageContextLoaded{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Page: page})
	case LoadBotCommands:
		client, err := h.currentClient("load bot commands")
		if err != nil {
			emit(BotCommandsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: *err})
			return
		}
		commands, loadErr := client.LoadBotCommands(h.workCtx, command.ChatID)
		if loadErr != nil {
			emit(BotCommandsLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: safeError("load bot commands", loadErr)})
			return
		}
		emit(BotCommandsLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Commands: append([]domain.BotCommand(nil), commands...)})
	case LoadSearchMessageContext:
		client, err := h.currentClient("load message context")
		if err != nil {
			emit(SearchMessageContextFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Error: messageContextError()})
			return
		}
		var (
			page       telegram.MessagePage
			contextErr error
		)
		if command.TopicID != 0 {
			page, contextErr = client.LoadTopicMessageContext(h.workCtx, command.ChatID, command.TopicID, command.MessageID)
		} else {
			page, contextErr = client.LoadMessageContext(h.workCtx, command.ChatID, command.MessageID)
		}
		if contextErr != nil {
			emit(SearchMessageContextFailed{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Error: messageContextError()})
			return
		}
		emit(SearchMessageContextLoaded{RequestID: command.RequestID, ChatID: command.ChatID, TopicID: command.TopicID, MessageID: command.MessageID, Page: page})
	case LoadStickers:
		client, err := h.currentClient("load stickers")
		if err != nil {
			emit(StickersLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: *err})
			return
		}
		stickers, loadErr := client.LoadStickers(h.workCtx)
		if loadErr != nil {
			emit(StickersLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, Error: safeError("load stickers", loadErr)})
			return
		}
		emit(StickersLoaded{RequestID: command.RequestID, ChatID: command.ChatID, Stickers: append([]domain.StickerRef(nil), stickers...)})
	case GetMessageProperties:
		client, err := h.currentClient("get message properties")
		if err != nil {
			emit(MessagePropertiesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: messagePropertiesError(nil)})
			return
		}
		capabilities, loadErr := client.GetMessageProperties(h.workCtx, command.ChatID, command.MessageID)
		if loadErr != nil {
			emit(MessagePropertiesLoadFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: messagePropertiesError(loadErr)})
			return
		}
		emit(MessagePropertiesLoaded{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Capabilities: capabilities})
	case SaveDraft:
		h.saveDraft(command, emit)
	case SendText:
		client, err := h.currentClient("send text")
		if err != nil {
			emit(TextQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, Error: *err, FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendText(h.workCtx, telegram.SendTextRequest{ChatID: command.ChatID, TopicID: command.TopicID, Text: command.Text, ReplyToMessageID: command.ReplyToMessageID})
		if sendErr != nil {
			emit(TextQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, Error: safeError("send text", sendErr), FailedAt: h.now()})
			return
		}
		emit(TextQueued{RequestID: command.RequestID, LocalID: command.LocalID, Message: message})
	case SendPhoto:
		client, err := h.currentClient("send photo")
		if err != nil {
			emit(PhotoQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: *err, FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendPhoto(h.workCtx, telegram.SendPhotoRequest{
			ChatID:           command.ChatID,
			TopicID:          command.TopicID,
			LocalPath:        command.LocalPath,
			Caption:          command.Caption,
			ReplyToMessageID: command.ReplyToMessageID,
		})
		if sendErr != nil {
			emit(PhotoQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: safeError("send photo", sendErr), FailedAt: h.now()})
			return
		}
		emit(PhotoQueued{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Message: message})
	case SendVideo:
		client, err := h.currentClient("send video")
		if err != nil {
			emit(VideoQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: *err, FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendVideo(h.workCtx, telegram.SendVideoRequest{
			ChatID:           command.ChatID,
			TopicID:          command.TopicID,
			LocalPath:        command.LocalPath,
			Caption:          command.Caption,
			ReplyToMessageID: command.ReplyToMessageID,
		})
		if sendErr != nil {
			emit(VideoQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: safeError("send video", sendErr), FailedAt: h.now()})
			return
		}
		emit(VideoQueued{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Message: message})
	case SendSticker:
		client, err := h.currentClient("send sticker")
		if err != nil {
			emit(StickerQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: *err, FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendSticker(h.workCtx, telegram.SendStickerRequest{
			ChatID: command.ChatID, TopicID: command.TopicID, Sticker: command.Sticker, ReplyToMessageID: command.ReplyToMessageID,
		})
		if sendErr != nil {
			emit(StickerQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: safeError("send sticker", sendErr), FailedAt: h.now()})
			return
		}
		emit(StickerQueued{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Message: message})
	case SendAudio:
		client, err := h.currentClient("send audio")
		if err != nil {
			emit(AudioQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: *err, FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendAudio(h.workCtx, telegram.SendAudioRequest{
			ChatID:           command.ChatID,
			TopicID:          command.TopicID,
			LocalPath:        command.LocalPath,
			Caption:          command.Caption,
			ReplyToMessageID: command.ReplyToMessageID,
		})
		if sendErr != nil {
			emit(AudioQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: safeError("send audio", sendErr), FailedAt: h.now()})
			return
		}
		emit(AudioQueued{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Message: message})
	case SendDocument:
		client, err := h.currentClient("send document")
		if err != nil {
			emit(DocumentQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: documentSendError(*err), FailedAt: h.now()})
			return
		}
		message, sendErr := client.SendDocument(h.workCtx, telegram.SendDocumentRequest{
			ChatID:           command.ChatID,
			TopicID:          command.TopicID,
			LocalPath:        command.LocalPath,
			Caption:          command.Caption,
			ReplyToMessageID: command.ReplyToMessageID,
		})
		if sendErr != nil {
			emit(DocumentQueueFailed{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Error: documentSendError(sendErr), FailedAt: h.now()})
			return
		}
		emit(DocumentQueued{RequestID: command.RequestID, LocalID: command.LocalID, ChatID: command.ChatID, Message: message})
	case EditText:
		client, err := h.currentClient("edit text")
		if err != nil {
			emit(TextEditFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: editTextError(nil)})
			return
		}
		message, editErr := client.EditText(h.workCtx, telegram.EditTextRequest{ChatID: command.ChatID, MessageID: command.MessageID, Text: command.Text})
		if editErr != nil {
			emit(TextEditFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: editTextError(editErr)})
			return
		}
		emit(TextEdited{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Message: message})
	case DeleteMessageCommand:
		client, err := h.currentClient("delete message")
		if err != nil {
			emit(MessageDeleteFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: deleteError(nil)})
			return
		}
		deleteErr := client.DeleteMessage(h.workCtx, telegram.DeleteMessageRequest{ChatID: command.ChatID, MessageID: command.MessageID, Revoke: command.Revoke})
		if deleteErr != nil {
			emit(MessageDeleteFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: deleteError(deleteErr)})
			return
		}
		emit(MessageDeleted{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID})
	case PinMessageCommand:
		client, err := h.currentClient("pin message")
		if err != nil {
			emit(MessagePinFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: pinError(nil)})
			return
		}
		pinErr := client.PinMessage(h.workCtx, telegram.PinMessageRequest{ChatID: command.ChatID, MessageID: command.MessageID, Unpin: command.Unpin})
		if pinErr != nil {
			emit(MessagePinFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: pinError(pinErr)})
			return
		}
		emit(MessagePinChanged{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Pinned: !command.Unpin})
	case ReactToMessage:
		client, err := h.currentClient("react to message")
		if err != nil {
			emit(ReactionFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: reactError(err)})
			return
		}
		reactErr := client.ReactToMessage(h.workCtx, telegram.ReactToMessageRequest{ChatID: command.ChatID, MessageID: command.MessageID, Emoji: command.Emoji, Remove: command.Remove})
		if reactErr != nil {
			emit(ReactionFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: reactError(reactErr)})
			return
		}
		emit(ReactionChanged{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Emoji: command.Emoji, Removed: command.Remove})
	case ForwardMessageCommand:
		client, err := h.currentClient("forward message")
		if err != nil {
			emit(MessageForwardFailed{RequestID: command.RequestID, DestinationChatID: command.DestinationChatID, Error: forwardError(nil)})
			return
		}
		forwardErr := client.ForwardMessage(h.workCtx, telegram.ForwardMessageRequest{SourceChatID: command.SourceChatID, SourceMessageID: command.SourceMessageID, DestinationChatID: command.DestinationChatID})
		if forwardErr != nil {
			emit(MessageForwardFailed{RequestID: command.RequestID, DestinationChatID: command.DestinationChatID, Error: forwardError(forwardErr)})
			return
		}
		emit(MessageForwarded{RequestID: command.RequestID, DestinationChatID: command.DestinationChatID})
	case OpenChatCommand:
		h.openChat(command)
	case CloseChatCommand:
		h.closeChat(command)
	case RenderAvatar:
		h.renderAvatar(command, emit)
	case OpenAvatar:
		h.openAvatar(command, emit)
	case OpenMessageMediaFile:
		h.openMessageMedia(command, emit)
	case DownloadStickerThumbnail:
		h.downloadStickerThumbnail(command, emit)
	case DownloadThumbnail:
		h.downloadThumbnail(command, emit)
	case SubmitPrompt:
		if h.prompts == nil {
			emit(OperationFailed{Error: safeError("submit authorization prompt", errors.New("prompt broker unavailable"))})
			return
		}
		if err := h.prompts.Submit(h.workCtx, command.Response); err != nil {
			emit(OperationFailed{Error: safeError("submit authorization prompt", err)})
		}
	case BeginShutdown:
		h.beginShutdown(emit)
	case WriteClipboard:
		if h.clipboard == nil {
			emit(ClipboardWriteFailed{Error: clipboardError(errors.New("clipboard unavailable"))})
			return
		}
		if err := h.clipboard.WriteText(h.workCtx, command.Text); err != nil {
			emit(ClipboardWriteFailed{Error: clipboardError(err)})
			return
		}
		emit(ClipboardWritten{})
	case ShowDesktopNotification:
		h.mutex.Lock()
		n := h.notifier
		h.mutex.Unlock()
		if n == nil {
			// Best-effort: silently drop when notifier is missing.
			return
		}
		if err := n.Notify(h.workCtx, platform.Notification{
			Title: command.Title,
			Body:  command.Body,
		}); err != nil {
			// Best-effort: silently drop on failure; no toast or event.
			return
		}
	default:
		emit(OperationFailed{Error: safeError("handle command", errors.New("unsupported command"))})
	}
}

func (h *Handler) saveDraft(command SaveDraft, emit func(Event)) {
	key := topicKey{ChatID: command.ChatID, TopicID: command.TopicID}
	h.draftMutex.Lock()
	if h.draftPending == nil {
		h.draftPending = make(map[topicKey]draftSaveWork)
	}
	if h.draftRunning == nil {
		h.draftRunning = make(map[topicKey]bool)
	}
	h.draftPending[key] = draftSaveWork{command: command, emit: emit}
	if h.draftRunning[key] {
		h.draftMutex.Unlock()
		return
	}
	h.draftRunning[key] = true
	h.draftMutex.Unlock()

	for {
		h.draftMutex.Lock()
		work, ok := h.draftPending[key]
		if ok {
			delete(h.draftPending, key)
		} else {
			delete(h.draftRunning, key)
		}
		h.draftMutex.Unlock()
		if !ok {
			return
		}

		client, unavailable := h.currentClient("sync draft")
		if unavailable != nil {
			work.emit(DraftSaveFailed{RequestID: work.command.RequestID, ChatID: work.command.ChatID, TopicID: work.command.TopicID, Error: draftSyncError()})
			continue
		}
		err := client.SetDraft(h.workCtx, telegram.SetDraftRequest{
			ChatID:           work.command.ChatID,
			TopicID:          work.command.TopicID,
			Text:             work.command.Text,
			ReplyToMessageID: work.command.ReplyToMessageID,
		})
		if err != nil {
			work.emit(DraftSaveFailed{RequestID: work.command.RequestID, ChatID: work.command.ChatID, TopicID: work.command.TopicID, Error: draftSyncError()})
			continue
		}
		now := time.Now
		if h.now != nil {
			now = h.now
		}
		work.emit(DraftSaved{RequestID: work.command.RequestID, ChatID: work.command.ChatID, TopicID: work.command.TopicID, Date: now().Unix()})
	}
}

func (h *Handler) openChat(command OpenChatCommand) {
	client, err := h.currentClient("open chat")
	if err != nil {
		return
	}
	_ = client.OpenChat(h.workCtx, command.ChatID)
}

func (h *Handler) closeChat(command CloseChatCommand) {
	client, err := h.currentClient("close chat")
	if err != nil {
		return
	}
	_ = client.CloseChat(h.workCtx, command.ChatID)
}

func (h *Handler) loadBootstrap(emit func(Event)) {
	h.mutex.Lock()
	if h.client != nil {
		h.mutex.Unlock()
		return
	}
	if h.resolver == nil || h.factory == nil {
		h.mutex.Unlock()
		emit(StartupFailed{Error: safeError("start Telegram", errors.New("startup dependencies unavailable"))})
		return
	}
	runtime, err := h.resolveRuntime(emit)
	if err != nil {
		h.mutex.Unlock()
		emit(StartupFailed{Error: safeError("resolve runtime", err)})
		return
	}
	client, err := h.factory(runtime, h.prompts)
	if err != nil {
		h.mutex.Unlock()
		emit(StartupFailed{Error: safeError("create Telegram client", err)})
		return
	}
	if client == nil {
		h.mutex.Unlock()
		emit(StartupFailed{Error: safeError("create Telegram client", errors.New("client factory returned no client"))})
		return
	}
	h.client = client
	updates := make(chan telegram.Update, 64)
	done := make(chan error, 1)
	go func() {
		h.startOnce.Do(func() { close(h.startBegan) })
		done <- client.Start(h.workCtx, updates)
		close(updates)
	}()
	h.mutex.Unlock()
	go func() {
		ready := false
		var promptEvents <-chan auth.Prompt
		if h.prompts != nil {
			promptEvents = h.prompts.Prompts()
		}
		for {
			select {
			case prompt := <-promptEvents:
				emit(PromptRequested{Prompt: prompt})
			case update, open := <-updates:
				if !open {
					startErr := <-done
					if h.workCtx.Err() != nil && (startErr == nil || errors.Is(startErr, context.Canceled) || errors.Is(startErr, context.DeadlineExceeded)) {
						return
					}
					if startErr == nil {
						startErr = errors.New("Telegram client stopped")
					}
					failure := safeError("receive Telegram updates", startErr)
					if ready {
						emit(OperationFailed{Error: failure})
					} else {
						emit(StartupFailed{Error: failure})
					}
					return
				}
				if isReadyUpdate(update) {
					ready = true
				}
				emit(TelegramEvent{Value: update, ReceivedAt: h.now()})
			}
		}
	}()
}

type runtimeResult struct {
	runtime config.Runtime
	err     error
}

func (h *Handler) resolveRuntime(emit func(Event)) (config.Runtime, error) {
	results := make(chan runtimeResult, 1)
	go func() {
		runtime, err := h.resolver.Resolve(h.workCtx)
		results <- runtimeResult{runtime: runtime, err: err}
	}()
	var promptEvents <-chan auth.Prompt
	if h.prompts != nil {
		promptEvents = h.prompts.Prompts()
	}
	for {
		select {
		case prompt := <-promptEvents:
			emit(PromptRequested{Prompt: prompt})
		case result := <-results:
			return result.runtime, result.err
		case <-h.workCtx.Done():
			return config.Runtime{}, h.workCtx.Err()
		}
	}
}

func isReadyUpdate(update telegram.Update) bool {
	switch update := update.(type) {
	case telegram.Ready:
		return true
	case *telegram.Ready:
		return update != nil
	default:
		return false
	}
}

func (h *Handler) renderAvatar(command RenderAvatar, emit func(Event)) {
	client, err := h.currentClient("render avatar")
	if err != nil {
		emit(AvatarRenderFailed{Key: command.Key, Error: *err})
		return
	}
	file, downloadErr := client.DownloadAvatar(h.workCtx, command.Ref, telegram.AvatarSmall)
	if downloadErr != nil {
		emit(AvatarRenderFailed{Key: command.Key, Error: safeError("download avatar", downloadErr)})
		return
	}
	if h.avatars == nil {
		emit(AvatarRenderFailed{Key: command.Key, Error: safeError("render avatar", errors.New("avatar renderer unavailable"))})
		return
	}
	cells, renderErr := h.avatars.Render(h.workCtx, file.Path, command.Ref, command.Role)
	if renderErr != nil {
		emit(AvatarRenderFailed{Key: command.Key, Error: safeError("render avatar", renderErr)})
		return
	}
	emit(AvatarRendered{Key: command.Key, Cells: cells})
}

func (h *Handler) openAvatar(command OpenAvatar, emit func(Event)) {
	client, err := h.currentClient("open avatar")
	if err != nil {
		emit(AvatarOpenFailed{RequestID: command.RequestID, Error: *err})
		return
	}
	file, downloadErr := client.DownloadAvatar(h.workCtx, command.Ref, telegram.AvatarOriginal)
	if downloadErr != nil {
		emit(AvatarOpenFailed{RequestID: command.RequestID, Error: safeError("open avatar", downloadErr)})
		return
	}
	emit(AvatarOpened{RequestID: command.RequestID, Title: command.Title, Path: file.Path})
}

func (h *Handler) openMessageMedia(command OpenMessageMediaFile, emit func(Event)) {
	client, err := h.currentClient("open message media")
	if err != nil {
		emit(MessageMediaOpenFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: mediaOpenError(command.Title, *err)})
		return
	}
	file, downloadErr := client.DownloadMedia(h.workCtx, command.File)
	if downloadErr != nil {
		f := mediaOpenError(command.Title, downloadErr)
		emit(MessageMediaOpenFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: f})
		return
	}
	updatedFile := command.File
	updatedFile.Downloaded = true
	updatedFile.LocalPath = file.Path
	// Video, Audio, and attachment paths launch the external system opener.
	if command.Title == "Video" || command.Title == "Audio" || command.Title == "File" ||
		command.Title == "Animation" || command.Title == "Voice note" || command.Title == "Video note" {
		if err := h.workCtx.Err(); err != nil {
			emit(MessageMediaOpenFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: mediaOpenError(command.Title, nil)})
			return
		}
		if err := platform.OpenFile(h.workCtx, file.Path); err != nil {
			emit(MessageMediaOpenFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: mediaOpenError(command.Title, nil)})
			return
		}
	}
	emit(MessageMediaOpened{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Title: command.Title, File: updatedFile})
}

func (h *Handler) downloadStickerThumbnail(command DownloadStickerThumbnail, emit func(Event)) {
	client, err := h.currentClient("load stickers")
	if err != nil {
		emit(StickerThumbnailFailed{RequestID: command.RequestID, PickerRequestID: command.PickerRequestID, StickerFileID: command.StickerFileID})
		return
	}
	file, downloadErr := client.DownloadMedia(h.workCtx, command.File)
	if downloadErr != nil || h.thumbnails == nil {
		emit(StickerThumbnailFailed{RequestID: command.RequestID, PickerRequestID: command.PickerRequestID, StickerFileID: command.StickerFileID})
		return
	}
	block, renderErr := h.thumbnails.Render(file.Path, 12, 6)
	if renderErr != nil {
		emit(StickerThumbnailFailed{RequestID: command.RequestID, PickerRequestID: command.PickerRequestID, StickerFileID: command.StickerFileID})
		return
	}
	emit(StickerThumbnailRendered{RequestID: command.RequestID, PickerRequestID: command.PickerRequestID, StickerFileID: command.StickerFileID, Block: block})
}

func (h *Handler) downloadThumbnail(command DownloadThumbnail, emit func(Event)) {
	client, err := h.currentClient("download thumbnail")
	if err != nil {
		emit(ThumbnailDownloadFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: *err})
		return
	}
	file, downloadErr := client.DownloadMedia(h.workCtx, command.File)
	if downloadErr != nil {
		emit(ThumbnailDownloadFailed{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Error: safeError("download thumbnail", downloadErr)})
		return
	}
	updatedFile := command.File
	updatedFile.Downloaded = true
	updatedFile.LocalPath = file.Path
	emit(ThumbnailDownloaded{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, File: updatedFile})
	if h.thumbnails != nil {
		if block, renderErr := h.thumbnails.Render(file.Path, thumbnailWidth, thumbnailHeight); renderErr == nil {
			emit(ThumbnailRendered{RequestID: command.RequestID, ChatID: command.ChatID, MessageID: command.MessageID, Block: block})
		}
	}
}

func messageSearchError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "search messages", Message: "Could not search messages"}
}

func topicsLoadError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load topics", Message: "Could not load topics"}
}

func messageContextError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load message context", Message: "Could not open search result"}
}

func (h *Handler) currentClient(operation string) (telegram.Client, *domain.AppError) {
	h.mutex.Lock()
	client := h.client
	h.mutex.Unlock()
	if client == nil {
		failure := safeError(operation, errors.New("Telegram client unavailable"))
		return nil, &failure
	}
	return client, nil
}

func (h *Handler) beginShutdown(emit func(Event)) {
	h.shutdown.Do(func() {
		h.cancelWork()
		h.mutex.Lock()
		client := h.client
		h.mutex.Unlock()
		if client == nil {
			emit(ShutdownComplete{})
			return
		}
		select {
		case <-h.startBegan:
		case <-time.After(10 * time.Second):
			failure := domain.AppError{Kind: domain.ErrorInternal, Op: "start Telegram", Message: "Telegram startup did not begin before shutdown"}
			emit(ShutdownComplete{Error: &failure})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Close(ctx); err != nil {
			failure := domain.AppError{Kind: domain.ErrorInternal, Op: "close Telegram", Message: "could not close Telegram safely", Cause: err}
			emit(ShutdownComplete{Error: &failure})
			return
		}
		emit(ShutdownComplete{})
	})
}

func membersError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load members", Message: "Could not load members"}
}

func userInfoError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "load user", Message: "Could not load user info"}
}

func memberCopyError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "copy username", Message: "Could not copy username"}
}

func memberContactError(added bool) domain.AppError {
	message := "Could not remove contact"
	op := "remove contact"
	if added {
		message = "Could not add contact"
		op = "add contact"
	}
	return domain.AppError{Kind: domain.ErrorNetwork, Op: op, Message: message}
}

func memberBlockError(blocked bool) domain.AppError {
	message := "Could not unblock user"
	op := "block user"
	if blocked {
		message = "Could not block user"
	}
	return domain.AppError{Kind: domain.ErrorNetwork, Op: op, Message: message}
}

func draftSyncError() domain.AppError {
	return domain.AppError{Kind: domain.ErrorNetwork, Op: "sync draft", Message: "Could not sync draft"}
}

func documentSendError(err error) domain.AppError {
	failure := safeError("send document", err)
	failure.Message = "Could not send document"
	return failure
}

func safeError(operation string, err error) domain.AppError {
	var appError domain.AppError
	if errors.As(err, &appError) {
		appError.Cause = err
		if appError.Op == "" {
			appError.Op = operation
		}
		return appError
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domain.AppError{Kind: domain.ErrorNetwork, Op: operation, Message: "operation canceled or timed out", Cause: err}
	}
	return domain.AppError{Kind: domain.ErrorInternal, Op: operation, Message: "operation failed; retry or inspect the sanitized log", Cause: err}
}

func clipboardError(err error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "write clipboard", Message: "Could not copy message", Cause: err}
}

func messagePropertiesError(err error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "get message properties", Message: "Could not load message actions", Cause: err}
}

func editTextError(error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "edit text", Message: "Edit failed"}
}

func deleteError(error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "delete message", Message: "Delete failed"}
}

func pinError(error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "pin message", Message: "Pin failed"}
}

func forwardError(error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "forward message", Message: "Forward failed"}
}

func reactError(error) domain.AppError {
	return domain.AppError{Kind: domain.ErrorInternal, Op: "react to message", Message: "Reaction failed"}
}

func mediaOpenError(title string, cause error) domain.AppError {
	message := "Could not open image"
	switch title {
	case "Video":
		message = "Could not open video"
	case "Audio":
		message = "Could not open audio"
	case "File":
		message = "Could not open file"
	case "Animation":
		message = "Could not open animation"
	case "Voice note":
		message = "Could not open voice note"
	case "Video note":
		message = "Could not open video note"
	}
	failure := domain.AppError{Kind: domain.ErrorMedia, Op: "open message media", Message: message}
	// External-opener failures must not retain transport or local-path
	// details. Photo keeps its accepted internal cause behavior.
	switch title {
	case "Video", "Audio", "File", "Animation", "Voice note", "Video note":
	default:
		failure.Cause = cause
	}
	return failure
}

var _ AvatarRenderer = avatar.Renderer{}
