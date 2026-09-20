package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type Effect = any

type LoadBootstrap struct{}
type LoadChats struct {
	RequestID uint64
	Cursor    telegram.ChatCursor
}
type LoadTopics struct {
	RequestID uint64
	ChatID    domain.ChatID
	Cursor    telegram.TopicCursor
}
type LoadMessages struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Cursor    telegram.MessageCursor
}
type SearchChatMessages struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Query     string
	Cursor    telegram.MessageSearchCursor
}
type LoadSearchMessageContext struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
}
type LoadMembers struct {
	RequestID uint64
	ChatID    domain.ChatID
	Cursor    telegram.MemberCursor
}
type LoadInviteLinksCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	Cursor    telegram.InviteLinkCursor
}
type CreateInviteLinkCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	Name      string
}
type RevokeInviteLinkCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type CopyInviteLinkCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type LoadAdministrationCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type LoadChatSettingsCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type SaveChatSettingCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	Field     ChatSettingField
	Value     string
	Delay     int
}
type LoadMemberAdministrationCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
}
type SetDefaultChatPermissionsCommand struct {
	RequestID   uint64
	ChatID      domain.ChatID
	Permissions telegram.ChatPermissions
}
type ApplyMemberAdministrationCommand struct {
	RequestID uint64
	Request   telegram.MemberAdministrationRequest
}
type LoadUserInfo struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
}
type AddMemberContactCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	FirstName string
	LastName  string
}
type RemoveMemberContactCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
}
type CopyMemberUsernameCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Text      string
}
type SetMemberBlockedCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Blocked   bool
}
type ApplyChatActionCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	Action    telegram.ChatAction
}
type LoadPinnedMessages struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Cursor    telegram.MessageSearchCursor
}
type LoadPinnedMessageContext struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
}
type LoadBotCommands struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type SaveDraft struct {
	RequestID        uint64
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Text             string
	ReplyToMessageID domain.MessageID
}
type SendText struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Text             string
	ReplyToMessageID domain.MessageID
}
type RenderAvatar struct {
	Key  string
	Ref  domain.AvatarRef
	Role avatar.Role
}
type OpenAvatar struct {
	RequestID uint64
	Title     string
	Ref       domain.AvatarRef
}
type SubmitPrompt struct{ Response auth.Response }
type BeginShutdown struct{}
type WriteClipboard struct{ Text string }
type GetMessageProperties struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
}
type EditText struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Text      string
}
type DeleteMessageCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Revoke    bool
}
type ForwardMessageCommand struct {
	RequestID         uint64
	SourceChatID      domain.ChatID
	SourceMessageID   domain.MessageID
	DestinationChatID domain.ChatID
}
type PinMessageCommand struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Unpin     bool
}
type ReactToMessage struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Emoji     string
	Remove    bool
}
type OpenChatCommand struct {
	ChatID domain.ChatID
}
type CloseChatCommand struct {
	ChatID domain.ChatID
}
type OpenMessageMediaFile struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Title     string
	File      domain.MediaFileRef
}

type SendPhoto struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendVideo struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendAudio struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type SendDocument struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	LocalPath        string
	Caption          string
	ReplyToMessageID domain.MessageID
}

type LoadStickers struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
}

type DownloadStickerThumbnail struct {
	RequestID       uint64
	PickerRequestID uint64
	StickerFileID   int32
	File            domain.MediaFileRef
}

// SearchPublicChatCommand requests a public chat lookup by Telegram username.
type SearchPublicChatCommand struct {
	RequestID uint64
	Username  string
}

// SearchPublicChatsCommand triggers a public-chats search by query string.
type SearchPublicChatsCommand struct {
	RequestID uint64
	Query     string
}

// SearchAllMessagesCommand triggers a global message search across all chats.
type SearchAllMessagesCommand struct {
	RequestID uint64
	Query     string
	Limit     int
}

type SendSticker struct {
	RequestID        uint64
	LocalID          domain.MessageID
	ChatID           domain.ChatID
	TopicID          domain.TopicID
	Sticker          domain.StickerRef
	ReplyToMessageID domain.MessageID
}

// DownloadThumbnail requests background thumbnail auto-download for a photo,
// video poster, or Sticker poster. It is issued by the reducer when media arrives with an
// undownloaded, downloadable thumbnail.
type DownloadThumbnail struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	File      domain.MediaFileRef
}

// ShowDesktopNotification requests a one-shot desktop toast with the
// given presentation title and body.  The command carries presentation-
// only data; no Telegram identifiers or raw payloads leak through it.
type ShowDesktopNotification struct {
	Title string
	Body  string
}
