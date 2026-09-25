package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type Action uint8

const (
	NoAction Action = iota
	SelectChat
	SelectNext
	SelectPrevious
	FocusPane
	FocusNext
	FocusPrevious
	Activate
	ToggleDetails
	Close
	PageUp
	PageDown
	ComposerBackspace
	ComposerNewline
	ComposerSubmit
	Retry
	Quit
	SelectMessage
	OpenMessageActionMenu
	CopyMessage
	ViewUserInfo
	SelectNextMessage
	SelectPreviousMessage
	ReplyMessage
	GoToReferencedMessage
	CancelReply
	EditMessage
	CancelEdit
	DeleteMessage
	DeleteForEveryone
	ForwardMessageSource
	PinMessage
	ReactMessage
	ViewMessageMedia
	OpenPhotoSend
	PhotoSendSubmit
	OpenStickerPicker
	StickerMoveLeft
	StickerMoveRight
	StickerMoveUp
	StickerMoveDown
	StickerActivate
	OpenMessageSearch
	SubmitMessageSearch
	OpenPinnedMessages
	SelectNextUnread
	SelectNextMention
	CommandMenuNext
	CommandMenuPrevious
	CommandMenuActivate
	CommandMenuDismiss
	OpenChatSearch
	SubmitChatSearch
	OpenMembers
	OpenInviteLinks
	OpenInviteLinkDetail
	CopyInviteLink
	CreateInviteLink
	RevokeInviteLink
	ConfirmRevokeInviteLink
	CancelRevokeInviteLink
	OpenDetailsAvatar
	OpenMemberDetail
	CloseMemberDetail
	ViewMemberAvatar
	CopyMemberUsername
	AddMemberContact
	RemoveMemberContact
	BlockMember
	UnblockMember
	OpenTopics
	SelectTopic
	SelectAllMessages
	OpenChatActionMenu
	OpenChat
	OpenGroupPermissions
	OpenChatSettings
	EditChatSetting
	SelectChatSlowMode
	SaveChatSetting
	OpenMemberAdministration
	ToggleAdministrationItem
	SaveAdministration
	SelectAdministrationAction
	ConfirmAdministrationAction
	CancelAdministrationAction
	ViewChatInfo
	MarkChatRead
	MarkChatUnread
	MuteChat
	UnmuteChat
	PinChat
	UnpinChat
	ArchiveChat
	UnarchiveChat
	ClearChatHistory
	DeleteConversation
	DeleteChat
	LeaveChat
	JoinChat
	ConfirmChatAction
	CancelChatAction
	FocusChat
)

type Focus uint8

const (
	FocusChats Focus = iota
	FocusConversation
	FocusComposer
	FocusDetails
	FocusModal
	FocusForwardPicker
	FocusReactionPicker
	FocusAuth
	FocusPhotoSend
	FocusStickerPicker
	FocusSearchInput
	FocusSearchResults
	FocusPinnedResults
	FocusChatSearchInput
	FocusChatSearchResults
	FocusMembers
	FocusInviteLinks
	FocusTopics
	FocusChatActions
	FocusAdministration
	FocusChatSettings
	FocusChatSettingsInput
)

// ActionReceived carries a frontend interaction and any identity selected by a
// hit target. It is handled only inside Bubble Tea's model.
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

// ChatSettingField identifies a mutable Telegram chat setting.
type ChatSettingField uint8

const (
	ChatSettingTitle ChatSettingField = iota + 1
	ChatSettingDescription
	ChatSettingSlowMode
)
