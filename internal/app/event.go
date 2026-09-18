package app

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type Event interface{ isEvent() }

type Started struct{}
type Resized struct{ Width, Height int }
type ActionReceived struct {
	Action            Action
	Rune              rune
	ChatID            domain.ChatID
	MessageID         domain.MessageID
	UserID            domain.UserID
	AvatarKey         string
	TargetFocus       Focus
	RequestID         uint64
	StickerFileID     int32
	CommandIndex      int
	At                time.Time
	TopicID           domain.TopicID
	InviteURL         string
	AdminIndex        int
	SettingField      ChatSettingField
	SlowModeDelay     int
	MemberAdminAction telegram.MemberAdministrationAction
}
type AdministrationLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Snapshot  telegram.AdministrationSnapshot
}
type ChatSettingsValueChanged struct {
	ChatID   domain.ChatID
	Field    ChatSettingField
	EditorID uint64
	Value    string
}
type ChatSettingsLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Snapshot  telegram.ChatSettings
}
type ChatSettingsLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type ChatSettingSaved struct {
	RequestID uint64
	ChatID    domain.ChatID
	Field     ChatSettingField
}
type ChatSettingSaveFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Field     ChatSettingField
}
type AdministrationLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type MemberAdministrationLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Status    telegram.MemberAdministrationStatus
}
type MemberAdministrationLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
}
type DefaultPermissionsSaved struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type DefaultPermissionsSaveFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type MemberAdministrationApplied struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Action    telegram.MemberAdministrationAction
}
type MemberAdministrationApplyFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Action    telegram.MemberAdministrationAction
}
type ChatsLoaded struct {
	RequestID uint64
	Page      telegram.ChatPage
}
type ChatsLoadFailed struct {
	RequestID uint64
	Error     domain.AppError
}
type TopicsLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Page      telegram.TopicPage
}
type TopicsLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Error     domain.AppError
}
type MessagesLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Page      telegram.MessagePage
}
type MessagesLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Error     domain.AppError
}
type ChatMessagesSearched struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Page      telegram.MessageSearchPage
}
type ChatMessagesSearchFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Error     domain.AppError
}
type SearchMessageContextLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
	Page      telegram.MessagePage
}
type SearchMessageContextFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
	Error     domain.AppError
}
type MembersLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Page      telegram.MemberPage
}
type MembersLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Error     domain.AppError
}
type InviteLinksLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Page      telegram.InviteLinkPage
}
type InviteLinksLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type InviteLinkCreated struct {
	RequestID uint64
	ChatID    domain.ChatID
	Link      telegram.InviteLink
}
type InviteLinkCreateFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
}
type InviteLinkRevoked struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type InviteLinkRevokeFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type InviteLinkCopied struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type InviteLinkCopyFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	URL       string
}
type UserInfoLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	User      domain.User
}
type UserInfoLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Error     domain.AppError
}
type MemberUsernameCopied struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
}
type MemberUsernameCopyFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Error     domain.AppError
}
type MemberContactChanged struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Added     bool
}
type MemberContactFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Added     bool
	Error     domain.AppError
}
type MemberBlockChanged struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Blocked   bool
}
type MemberBlockFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	UserID    domain.UserID
	Blocked   bool
	Error     domain.AppError
}
type ChatActionApplied struct {
	RequestID uint64
	ChatID    domain.ChatID
	Action    telegram.ChatAction
}
type ChatActionFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Action    telegram.ChatAction
	Error     domain.AppError
}
type PinnedMessagesLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Page      telegram.MessageSearchPage
}
type PinnedMessagesLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Error     domain.AppError
}
type PinnedMessageContextLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
	Page      telegram.MessagePage
}
type PinnedMessageContextFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
	Error     domain.AppError
}
type BotCommandsLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Commands  []domain.BotCommand
}
type BotCommandsLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Error     domain.AppError
}
type TelegramEvent struct {
	Value      telegram.Update
	ReceivedAt time.Time
}
type DraftSaved struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Date      int64
}
type DraftSaveFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	Error     domain.AppError
}
type TextQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	Message   domain.Message
}
type TextQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	Error     domain.AppError
	FailedAt  time.Time
}
type PhotoQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Message   domain.Message
}
type PhotoQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Error     domain.AppError
	FailedAt  time.Time
}
type VideoQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Message   domain.Message
}
type VideoQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Error     domain.AppError
	FailedAt  time.Time
}
type AudioQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Message   domain.Message
}
type AudioQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Error     domain.AppError
	FailedAt  time.Time
}
type DocumentQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Message   domain.Message
}
type DocumentQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Error     domain.AppError
	FailedAt  time.Time
}
type StickersLoaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	Stickers  []domain.StickerRef
}
type StickersLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	Error     domain.AppError
}
type StickerThumbnailRendered struct {
	RequestID       uint64
	PickerRequestID uint64
	StickerFileID   int32
	Block           thumbnail.Block
}
type StickerThumbnailFailed struct {
	RequestID       uint64
	PickerRequestID uint64
	StickerFileID   int32
}
type StickerQueued struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Message   domain.Message
}
type StickerQueueFailed struct {
	RequestID uint64
	LocalID   domain.MessageID
	ChatID    domain.ChatID
	Error     domain.AppError
	FailedAt  time.Time
}
type AvatarRendered struct {
	Key   string
	Cells pixel.Avatar
}
type AvatarRenderFailed struct {
	Key   string
	Error domain.AppError
}
type AvatarOpenFailed struct {
	RequestID uint64
	Error     domain.AppError
}
type AvatarOpened struct {
	RequestID uint64
	Title     string
	Path      string
}
type StartupFailed struct{ Error domain.AppError }
type ShutdownComplete struct{ Error *domain.AppError }
type OperationFailed struct{ Error domain.AppError }
type PromptRequested struct{ Prompt auth.Prompt }
type ClipboardWritten struct{}
type ClipboardWriteFailed struct{ Error domain.AppError }
type ToastExpired struct{ Generation uint64 }
type MessagePropertiesLoaded struct {
	RequestID    uint64
	ChatID       domain.ChatID
	MessageID    domain.MessageID
	Capabilities domain.MessageCapabilities
}
type MessagePropertiesLoadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}
type TextEdited struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Message   domain.Message
}
type TextEditFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}
type MessageDeleted struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
}
type MessageDeleteFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}
type MessageForwarded struct {
	RequestID         uint64
	DestinationChatID domain.ChatID
}
type MessageForwardFailed struct {
	RequestID         uint64
	DestinationChatID domain.ChatID
	Error             domain.AppError
}
type MessagePinChanged struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Pinned    bool
}
type MessagePinFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}
type ReactionChanged struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Emoji     string
	Removed   bool
}
type ReactionFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}
type MessageMediaOpened struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Title     string
	File      domain.MediaFileRef
}
type MessageMediaOpenFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}

// PublicChatSearched is emitted when the adapter returns a chat from a public
// username lookup. The chat may be a user, basic group, or supergroup.
type PublicChatSearched struct {
	RequestID uint64
	Chat      domain.Chat
}

// PublicChatSearchFailed records a failed public chat lookup.
type PublicChatSearchFailed struct {
	RequestID uint64
	Error     domain.AppError
}

// PublicChatsSearched is emitted when the adapter returns public chats
// matched by a query string. The query is case-insensitive and the
// result list is already hydrated via GetChat.
type PublicChatsSearched struct {
	RequestID uint64
	Chats     []domain.Chat
}

// PublicChatsSearchFailed records a failed public-chat search.
type PublicChatsSearchFailed struct {
	RequestID uint64
	Error     domain.AppError
}

// AllMessagesSearched carries global-message search results across all chats.
type AllMessagesSearched struct {
	RequestID  uint64
	Messages   []domain.Message
	TotalCount int
}

// AllMessagesSearchFailed records a failed all-messages search.
type AllMessagesSearchFailed struct {
	RequestID uint64
	Error     domain.AppError
}

// ChatSearchValueChanged is the input-host event that pushes typed runes into
// the global chat search input buffer.
type ChatSearchValueChanged struct {
	Value string
}

func (ChatSearchValueChanged) isEvent() {}

func (Started) isEvent()                         {}
func (Resized) isEvent()                         {}
func (ActionReceived) isEvent()                  {}
func (ChatsLoaded) isEvent()                     {}
func (ChatsLoadFailed) isEvent()                 {}
func (TopicsLoaded) isEvent()                    {}
func (TopicsLoadFailed) isEvent()                {}
func (MessagesLoaded) isEvent()                  {}
func (MessagesLoadFailed) isEvent()              {}
func (ChatMessagesSearched) isEvent()            {}
func (ChatMessagesSearchFailed) isEvent()        {}
func (SearchMessageContextLoaded) isEvent()      {}
func (SearchMessageContextFailed) isEvent()      {}
func (MembersLoaded) isEvent()                   {}
func (UserInfoLoaded) isEvent()                  {}
func (UserInfoLoadFailed) isEvent()              {}
func (MembersLoadFailed) isEvent()               {}
func (InviteLinksLoaded) isEvent()               {}
func (InviteLinksLoadFailed) isEvent()           {}
func (InviteLinkCreated) isEvent()               {}
func (InviteLinkCreateFailed) isEvent()          {}
func (InviteLinkRevoked) isEvent()               {}
func (InviteLinkRevokeFailed) isEvent()          {}
func (InviteLinkCopied) isEvent()                {}
func (InviteLinkCopyFailed) isEvent()            {}
func (AdministrationLoaded) isEvent()            {}
func (ChatSettingsValueChanged) isEvent()        {}
func (ChatSettingsLoaded) isEvent()              {}
func (ChatSettingsLoadFailed) isEvent()          {}
func (ChatSettingSaved) isEvent()                {}
func (ChatSettingSaveFailed) isEvent()           {}
func (AdministrationLoadFailed) isEvent()        {}
func (MemberAdministrationLoaded) isEvent()      {}
func (MemberAdministrationLoadFailed) isEvent()  {}
func (DefaultPermissionsSaved) isEvent()         {}
func (DefaultPermissionsSaveFailed) isEvent()    {}
func (MemberAdministrationApplied) isEvent()     {}
func (MemberAdministrationApplyFailed) isEvent() {}
func (MemberUsernameCopied) isEvent()            {}
func (MemberUsernameCopyFailed) isEvent()        {}
func (MemberContactChanged) isEvent()            {}
func (MemberContactFailed) isEvent()             {}
func (MemberBlockChanged) isEvent()              {}
func (MemberBlockFailed) isEvent()               {}
func (PinnedMessagesLoaded) isEvent()            {}
func (PinnedMessagesLoadFailed) isEvent()        {}
func (PinnedMessageContextLoaded) isEvent()      {}
func (PinnedMessageContextFailed) isEvent()      {}
func (BotCommandsLoaded) isEvent()               {}
func (BotCommandsLoadFailed) isEvent()           {}
func (TelegramEvent) isEvent()                   {}
func (DraftSaved) isEvent()                      {}
func (DraftSaveFailed) isEvent()                 {}
func (TextQueued) isEvent()                      {}
func (TextQueueFailed) isEvent()                 {}
func (PhotoQueued) isEvent()                     {}
func (PhotoQueueFailed) isEvent()                {}
func (VideoQueued) isEvent()                     {}
func (VideoQueueFailed) isEvent()                {}
func (AudioQueued) isEvent()                     {}
func (AudioQueueFailed) isEvent()                {}
func (DocumentQueued) isEvent()                  {}
func (DocumentQueueFailed) isEvent()             {}
func (StickersLoaded) isEvent()                  {}
func (StickersLoadFailed) isEvent()              {}
func (StickerThumbnailRendered) isEvent()        {}
func (StickerThumbnailFailed) isEvent()          {}
func (StickerQueued) isEvent()                   {}
func (StickerQueueFailed) isEvent()              {}
func (AvatarRendered) isEvent()                  {}
func (AvatarRenderFailed) isEvent()              {}
func (AvatarOpenFailed) isEvent()                {}
func (AvatarOpened) isEvent()                    {}
func (StartupFailed) isEvent()                   {}
func (ShutdownComplete) isEvent()                {}
func (OperationFailed) isEvent()                 {}
func (PromptRequested) isEvent()                 {}
func (ClipboardWritten) isEvent()                {}
func (ClipboardWriteFailed) isEvent()            {}
func (ToastExpired) isEvent()                    {}
func (MessagePropertiesLoaded) isEvent()         {}
func (MessagePropertiesLoadFailed) isEvent()     {}
func (TextEdited) isEvent()                      {}
func (TextEditFailed) isEvent()                  {}
func (MessageDeleted) isEvent()                  {}
func (MessageDeleteFailed) isEvent()             {}
func (MessageForwarded) isEvent()                {}
func (MessageForwardFailed) isEvent()            {}
func (MessagePinChanged) isEvent()               {}
func (MessagePinFailed) isEvent()                {}
func (ReactionChanged) isEvent()                 {}
func (ReactionFailed) isEvent()                  {}
func (MessageMediaOpened) isEvent()              {}
func (MessageMediaOpenFailed) isEvent()          {}
func (PublicChatSearched) isEvent()              {}
func (PublicChatSearchFailed) isEvent()          {}
func (PublicChatsSearched) isEvent()             {}
func (PublicChatsSearchFailed) isEvent()         {}
func (AllMessagesSearched) isEvent()             {}
func (AllMessagesSearchFailed) isEvent()         {}

// ThumbnailDownloaded reports a successfully auto-downloaded thumbnail.
type ThumbnailDownloaded struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	File      domain.MediaFileRef
}

// ThumbnailDownloadFailed reports a failure to auto-download a thumbnail.
type ThumbnailDownloadFailed struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Error     domain.AppError
}

func (ThumbnailDownloaded) isEvent()     {}
func (ThumbnailDownloadFailed) isEvent() {}

// ThumbnailRendered reports a successfully rendered thumbnail block.
type ThumbnailRendered struct {
	RequestID uint64
	ChatID    domain.ChatID
	MessageID domain.MessageID
	Block     thumbnail.Block
}

func (ThumbnailRendered) isEvent() {}

// ComposerValueChanged reports a new whole value for the composer or
// active edit target.
type ComposerValueChanged struct {
	ChatID        domain.ChatID
	EditMessageID domain.MessageID
	Value         string
}

func (ChatActionApplied) isEvent() {}
func (ChatActionFailed) isEvent()  {}

func (ComposerValueChanged) isEvent() {}

// PromptValueChanged reports a whole-value update for an active auth prompt.
type PromptValueChanged struct {
	PromptID uint64
	Value    string
}

func (PromptValueChanged) isEvent() {}

// PhotoPathValueChanged reports a whole-value update for the active photo-send path.
type PhotoPathValueChanged struct {
	ChatID domain.ChatID
	Value  string
}

func (PhotoPathValueChanged) isEvent() {}

// MessageSearchValueChanged reports the whole query value for the active chat.
type MessageSearchValueChanged struct {
	ChatID domain.ChatID
	Value  string
}

func (MessageSearchValueChanged) isEvent() {}

// TerminalFocusChanged reports that the terminal gained or lost focus.
// Focused=true means the terminal is now visible and accepting input;
// Focused=false means it was blurred.
type TerminalFocusChanged struct {
	Focused bool
}

func (TerminalFocusChanged) isEvent() {}
