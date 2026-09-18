package telegram

import (
	"context"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

type ChatCursor struct{ Limit int }

type ChatPage struct {
	Chats []domain.Chat
	Done  bool
}

type MessageCursor struct {
	FromMessageID domain.MessageID
	TopicID       domain.TopicID
	Limit         int
	OnlyLocal     bool
}

// TopicCursor continues the TDLib forum topic listing from a previous page.
type TopicCursor struct {
	OffsetDate      int64
	OffsetMessageID domain.MessageID
	OffsetTopicID   domain.TopicID
	Limit           int
}

type MessagePage struct {
	Messages []domain.Message
	Done     bool
}

// TopicPage is one page of forum topics with TDLib's next listing offsets.
type TopicPage struct {
	Topics              []domain.ForumTopic
	TotalCount          int
	NextOffsetDate      int64
	NextOffsetMessageID domain.MessageID
	NextOffsetTopicID   domain.TopicID
	Done                bool
}

type MemberCursor struct {
	Offset int
	Limit  int
}

type MemberPage struct {
	Members    []domain.ChatMember
	TotalCount int
	NextOffset int
	Done       bool
}

// MessageSearchCursor continues a chat-local search from an older result.
type MessageSearchCursor struct {
	FromMessageID domain.MessageID
	TopicID       domain.TopicID
	Limit         int
}

// MessageSearchPage preserves TDLib's reverse-chronological result order.
type MessageSearchPage struct {
	Messages          []domain.Message
	NextFromMessageID domain.MessageID
	TotalCount        int
	Done              bool
}

// PublicChatsPage carries a live list of public chats matched by query.
type PublicChatsPage struct {
	Chats []domain.Chat
}

// InviteLink is the Telegram-independent presentation of an administrator's invite link.
type InviteLink struct {
	URL                     string
	Name                    string
	IsPrimary               bool
	IsRevoked               bool
	Date                    int64
	ExpirationDate          int64
	MemberLimit             int
	MemberCount             int
	CreatesJoinRequest      bool
	PendingJoinRequestCount int
}

type InviteLinkCursor struct {
	OffsetDate int64
	OffsetURL  string
	Limit      int
}

type InviteLinkPage struct {
	Primary    *InviteLink
	Links      []InviteLink
	TotalCount int
	NextCursor InviteLinkCursor
	Done       bool
}

// InviteLinksClient is optional so existing Client implementations remain valid.
type InviteLinksClient interface {
	LoadInviteLinks(context.Context, domain.ChatID, InviteLinkCursor) (InviteLinkPage, error)
	CreateInviteLink(context.Context, domain.ChatID, string) (InviteLink, error)
	RevokeInviteLink(context.Context, domain.ChatID, string) error
}

type ChatSettings struct {
	Kind               domain.ChatKind
	Title              string
	Description        string
	SlowModeDelay      int
	CanChangeInfo      bool
	CanRestrictMembers bool
}

// ChatSettingsClient is optional so existing Client implementations remain valid.
type ChatSettingsClient interface {
	LoadChatSettings(context.Context, domain.ChatID) (ChatSettings, error)
	SetChatTitle(context.Context, domain.ChatID, string) error
	SetChatDescription(context.Context, domain.ChatID, string) error
	SetChatSlowModeDelay(context.Context, domain.ChatID, int) error
}

func validSlowModeDelay(delay int) bool {
	switch delay {
	case 0, 5, 10, 30, 60, 300, 900, 3600:
		return true
	default:
		return false
	}
}

type ChatPermissions struct {
	CanSendBasicMessages bool
	CanSendAudios        bool
	CanSendDocuments     bool
	CanSendPhotos        bool
	CanSendVideos        bool
	CanSendVideoNotes    bool
	CanSendVoiceNotes    bool
	CanSendPolls         bool
	CanSendOtherMessages bool
	CanAddLinkPreviews   bool
	CanReactToMessages   bool
	CanEditTag           bool
	CanChangeInfo        bool
	CanInviteUsers       bool
	CanPinMessages       bool
	CanCreateTopics      bool
}

type AdministratorRights struct {
	CanManageChat           bool
	CanChangeInfo           bool
	CanPostMessages         bool
	CanEditMessages         bool
	CanDeleteMessages       bool
	CanInviteUsers          bool
	CanRestrictMembers      bool
	CanPinMessages          bool
	CanManageTopics         bool
	CanPromoteMembers       bool
	CanManageVideoChats     bool
	CanPostStories          bool
	CanEditStories          bool
	CanDeleteStories        bool
	CanManageDirectMessages bool
	CanManageTags           bool
	IsAnonymous             bool
}

type AdministrationSnapshot struct {
	Kind               domain.ChatKind
	DefaultPermissions ChatPermissions
	IsOwner            bool
	CanRestrictMembers bool
	CanPromoteMembers  bool
	OwnRights          AdministratorRights
}

type MemberAdministrationStatus struct {
	Role          domain.ChatMemberRole
	IsCurrentUser bool
	CanBeEdited   bool
	Rights        AdministratorRights
	Permissions   ChatPermissions
}

type MemberAdministrationAction uint8

const (
	MemberAdministrationPromote MemberAdministrationAction = iota + 1
	MemberAdministrationDemote
	MemberAdministrationRestrict
	MemberAdministrationUnrestrict
	MemberAdministrationRemove
	MemberAdministrationBan
)

type MemberAdministrationRequest struct {
	ChatID      domain.ChatID
	UserID      domain.UserID
	Action      MemberAdministrationAction
	Rights      AdministratorRights
	Permissions ChatPermissions
}

type AdministrationClient interface {
	LoadAdministration(context.Context, domain.ChatID) (AdministrationSnapshot, error)
	LoadMemberAdministration(context.Context, domain.ChatID, domain.UserID) (MemberAdministrationStatus, error)
	SetDefaultChatPermissions(context.Context, domain.ChatID, ChatPermissions) error
	ApplyMemberAdministration(context.Context, MemberAdministrationRequest) error
}

type MessageIdentity struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
}

type SendTextRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Text             string
	ReplyToMessageID domain.MessageID
}

type SetDraftRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Text             string
	ReplyToMessageID domain.MessageID
}

type EditTextRequest struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Text      string
}

type DeleteMessageRequest struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Revoke    bool
}

type SendPhotoRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendVideoRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendAudioRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendDocumentRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendStickerRequest struct {
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Sticker          domain.StickerRef
	ReplyToMessageID domain.MessageID
}

type ForwardMessageRequest struct {
	SourceChatID      domain.ChatID
	SourceMessageID   domain.MessageID
	DestinationChatID domain.ChatID
}

type PinMessageRequest struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Unpin     bool
}

type ReactToMessageRequest struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Emoji     string
	Remove    bool
}

type AddContactRequest struct {
	UserID    domain.UserID
	FirstName string
	LastName  string
}

type RemoveContactRequest struct {
	UserID domain.UserID
}

type SetUserBlockedRequest struct {
	UserID  domain.UserID
	Blocked bool
}

type ChatAction uint8

const (
	ChatActionMarkRead ChatAction = iota + 1
	ChatActionMarkUnread
	ChatActionMute
	ChatActionUnmute
	ChatActionPin
	ChatActionUnpin
	ChatActionArchive
	ChatActionUnarchive
	ChatActionClearHistory
	ChatActionDeleteConversation
	ChatActionDeleteChat
	ChatActionLeaveChat
	ChatActionJoinChat
)

type ChatActionRequest struct {
	ChatID domain.ChatID
	Action ChatAction
}

type LocalFile struct {
	Path string
}

type AvatarSize uint8

const (
	AvatarSmall AvatarSize = iota
	AvatarOriginal
)

type Update interface{ isUpdate() }

type Ready struct{}
type Closed struct{}
type ConnectionChanged struct{ State domain.ConnectionState }
type ChatUpserted struct{ Chat domain.Chat }
type DraftChanged struct {
	ChatID domain.ChatID
	Draft  domain.Draft
}
type UserUpserted struct{ User domain.User }
type MessageUpserted struct{ Message domain.Message }
type MessageSendSucceeded struct {
	OldID   domain.MessageID
	Message domain.Message
}
type MessageSendFailed struct {
	OldID   domain.MessageID
	Message domain.Message
	Error   domain.AppError
}
type MessageContentUpdated struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Kind      domain.MessageKind
	Text      string
	FileName  string
	Media     domain.MessageMedia
	Sticker   domain.StickerRef
}
type MessageEdited struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	EditedAt  time.Time
}
type MessagePinnedUpdated struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Pinned    bool
}
type MessageReactionsUpdated struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Reactions []domain.MessageReaction
}
type MessagesDeleted struct {
	ChatID     domain.ChatID
	MessageIDs []domain.MessageID
	FromCache  bool
}

// ForumTopicInfoChanged reports new information for an existing topic.
type ForumTopicInfoChanged struct{ Topic domain.ForumTopic }

// ForumTopicStateChanged reports the topic state carried by the update, with
// unavailable fields left at their zero values.
type ForumTopicStateChanged struct {
	ChatID             domain.ChatID
	TopicID            domain.TopicID
	IsPinned           bool
	UnreadMentionCount int
	Draft              domain.Draft
}

func (Ready) isUpdate()                   {}
func (Closed) isUpdate()                  {}
func (ConnectionChanged) isUpdate()       {}
func (ChatUpserted) isUpdate()            {}
func (DraftChanged) isUpdate()            {}
func (UserUpserted) isUpdate()            {}
func (MessageUpserted) isUpdate()         {}
func (MessageSendSucceeded) isUpdate()    {}
func (MessageSendFailed) isUpdate()       {}
func (MessageContentUpdated) isUpdate()   {}
func (MessageEdited) isUpdate()           {}
func (MessagePinnedUpdated) isUpdate()    {}
func (MessageReactionsUpdated) isUpdate() {}
func (MessagesDeleted) isUpdate()         {}
func (ForumTopicInfoChanged) isUpdate()   {}
func (ForumTopicStateChanged) isUpdate()  {}

type Client interface {
	Start(ctx context.Context, updates chan<- Update) error
	LoadChats(ctx context.Context, cursor ChatCursor) (ChatPage, error)
	LoadMessages(ctx context.Context, chatID domain.ChatID, cursor MessageCursor) (MessagePage, error)
	LoadTopics(ctx context.Context, chatID domain.ChatID, cursor TopicCursor) (TopicPage, error)
	LoadMembers(ctx context.Context, chatID domain.ChatID, cursor MemberCursor) (MemberPage, error)
	LoadUser(ctx context.Context, userID domain.UserID) (domain.User, error)
	SearchChatMessages(ctx context.Context, chatID domain.ChatID, query string, cursor MessageSearchCursor) (MessageSearchPage, error)
	SearchPinnedMessages(ctx context.Context, chatID domain.ChatID, cursor MessageSearchCursor) (MessageSearchPage, error)
	SearchPublicChat(ctx context.Context, username string) (domain.Chat, error)
	SearchPublicChats(ctx context.Context, query string) ([]domain.Chat, error)
	SearchAllMessages(ctx context.Context, query string, limit int) (MessageSearchPage, error)
	LoadMessageContext(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (MessagePage, error)
	LoadTopicMessageContext(ctx context.Context, chatID domain.ChatID, topicID domain.TopicID, messageID domain.MessageID) (MessagePage, error)
	OpenChat(ctx context.Context, chatID domain.ChatID) error
	CloseChat(ctx context.Context, chatID domain.ChatID) error
	GetMessageProperties(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (domain.MessageCapabilities, error)
	LoadBotCommands(ctx context.Context, chatID domain.ChatID) ([]domain.BotCommand, error)
	LoadStickers(ctx context.Context) ([]domain.StickerRef, error)
	SetDraft(ctx context.Context, request SetDraftRequest) error
	SendText(ctx context.Context, request SendTextRequest) (domain.Message, error)
	SendPhoto(ctx context.Context, request SendPhotoRequest) (domain.Message, error)
	SendVideo(ctx context.Context, request SendVideoRequest) (domain.Message, error)
	SendAudio(ctx context.Context, request SendAudioRequest) (domain.Message, error)
	SendDocument(ctx context.Context, request SendDocumentRequest) (domain.Message, error)
	SendSticker(ctx context.Context, request SendStickerRequest) (domain.Message, error)
	EditText(ctx context.Context, request EditTextRequest) (domain.Message, error)
	DeleteMessage(ctx context.Context, request DeleteMessageRequest) error
	PinMessage(ctx context.Context, request PinMessageRequest) error
	ReactToMessage(ctx context.Context, request ReactToMessageRequest) error
	ForwardMessage(ctx context.Context, request ForwardMessageRequest) error
	AddContact(ctx context.Context, request AddContactRequest) error
	RemoveContact(ctx context.Context, request RemoveContactRequest) error
	SetUserBlocked(ctx context.Context, request SetUserBlockedRequest) error
	ApplyChatAction(ctx context.Context, request ChatActionRequest) error
	DownloadAvatar(ctx context.Context, ref domain.AvatarRef, size AvatarSize) (LocalFile, error)
	DownloadMedia(ctx context.Context, ref domain.MediaFileRef) (LocalFile, error)
	Close(ctx context.Context) error
}

var (
	_ Update = Ready{}
	_ Update = Closed{}
	_ Update = ConnectionChanged{}
	_ Update = ChatUpserted{}
	_ Update = DraftChanged{}
	_ Update = UserUpserted{}
	_ Update = MessageUpserted{}
	_ Update = MessageSendSucceeded{}
	_ Update = MessageSendFailed{}
	_ Update = MessageContentUpdated{}
	_ Update = MessageEdited{}
	_ Update = MessagePinnedUpdated{}
	_ Update = MessageReactionsUpdated{}
	_ Update = MessagesDeleted{}
	_ Update = ForumTopicInfoChanged{}
	_ Update = ForumTopicStateChanged{}
)
