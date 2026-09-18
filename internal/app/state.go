package app

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type Layout uint8

const (
	LayoutTooSmall Layout = iota
	LayoutNarrow
	LayoutNormal
	LayoutWide
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

type ModalState struct {
	RequestID      uint64
	Title          string
	Ref            domain.AvatarRef
	MediaChatID    domain.ChatID
	MediaMessageID domain.MessageID
	MediaFile      domain.MediaFileRef
	Path           string
	Loading        bool
	Error          *domain.AppError
	PreviousFocus  Focus
}

type PromptState struct {
	Prompt        auth.Prompt
	Input         []rune
	PreviousFocus Focus
}

type AvatarState struct {
	Loading bool
	Cells   pixel.Avatar
	Error   *domain.AppError
	Ref     domain.AvatarRef
	Role    avatar.Role
	Label   string
}

type HistoryState struct {
	Loading         bool
	Done            bool
	OldestID        domain.MessageID
	RequestID       uint64
	ViewOffset      int
	FollowSelection bool
	Error           *domain.AppError
}

type MessageActionMenu struct {
	RequestID     uint64
	ChatID        domain.ChatID
	MessageID     domain.MessageID
	UserID        domain.UserID
	Capabilities  domain.MessageCapabilities
	Pinned        bool
	Loading       bool
	Error         *domain.AppError
	Selected      int
	PreviousFocus Focus
	PreferEdit    bool
	CanReact      bool
	MediaFile     domain.MediaFileRef
	MediaKind     domain.MessageKind
}

type ForwardPicker struct {
	SourceChatID    domain.ChatID
	SourceMessageID domain.MessageID
	SelectedChat    int
	RequestID       uint64
}

type ReactionPicker struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
	RequestID uint64
	Selected  int
}

type topicKey struct {
	ChatID  domain.ChatID
	TopicID domain.TopicID
}

// TopicKey is the exported name of the unexported topicKey map key used by
// the topic-keyed State maps, so read-only projections (the ui package) can
// index TopicDrafts / TopicHistory without the app package re-exporting state.
type TopicKey = topicKey

type ReplyTarget struct {
	ChatID    domain.ChatID
	TopicID   domain.TopicID
	MessageID domain.MessageID
	Sender    string
	Preview   string
}

type PhotoSendState struct {
	ChatID        domain.ChatID
	TopicID       domain.TopicID
	Input         []rune
	PreviousFocus Focus
}

type StickerPickerState struct {
	RequestID     uint64
	ChatID        domain.ChatID
	TopicID       domain.TopicID
	PreviousFocus Focus
	Loading       bool
	Error         *domain.AppError
	Catalog       []domain.StickerRef
	Selected      int
	FirstRow      int
	Columns       int
	VisibleRows   int
}

const CommandMenuVisibleRows = 8

// BotCommandCatalogState caches one chat's advertised command catalog.
type BotCommandCatalogState struct {
	RequestID uint64
	Loading   bool
	Loaded    bool
	Commands  []domain.BotCommand
	Error     *domain.AppError
}

// CommandMenuState is an inline completion menu; composer focus remains active.
type CommandMenuState struct {
	ChatID     domain.ChatID
	Query      string
	Candidates []domain.BotCommand
	Selected   int
	First      int
	Loading    bool
	Error      *domain.AppError
}

type DraftSyncState struct {
	RequestID uint64
	Draft     domain.Draft
	Pending   bool
	Dirty     bool
}

type EditTarget struct {
	RequestID  uint64
	ChatID     domain.ChatID
	MessageID  domain.MessageID
	Original   string
	Buffer     string
	Submitting bool
	Error      *domain.AppError
}

// TopicListState owns the forum-topic list overlay for one chat.
type TopicListState struct {
	RequestID     uint64
	ChatID        domain.ChatID
	PreviousFocus Focus
	Loading       bool
	Error         *domain.AppError
	Results       []domain.ForumTopic
	Selected      int
	TotalCount    int
	NextCursor    telegram.TopicCursor
	Done          bool
}

// MessageSearchState owns one chat-scoped search overlay and its pagination.
type MessageSearchState struct {
	RequestID         uint64
	ChatID            domain.ChatID
	TopicID           domain.TopicID
	Input             []rune
	Query             string
	PreviousFocus     Focus
	Loading           bool
	Error             *domain.AppError
	Results           []domain.Message
	Selected          int
	NextFromMessageID domain.MessageID
	TotalCount        int
	Done              bool
	Submitted         bool
	JumpMessageID     domain.MessageID
}

// ChatSearchState owns the unified global search overlay from the Chats pane.
// Users type a query and the reducer performs live local-chats filter plus
// remote public-chat and all-messages search. The view shows three sections
// in fixed order: Chats, Messages, Public chats.
type ChatSearchState struct {
	RequestID       uint64
	Input           []rune
	Query           string
	PreviousFocus   Focus
	LocalChats      []domain.Chat
	PublicChats     []domain.Chat
	GlobalMessages  []domain.Message
	PublicLoading   bool
	MessagesLoading bool
	PublicError     *domain.AppError
	MessagesError   *domain.AppError
	Selected        int
	Submitted       bool
}

// PinnedMessagesState owns the chat-scoped pinned-message overlay and pagination.
// MemberDetail is the in-modal detail view for one member. The Members modal
// shows either the member list (Detail == nil) or one member's info plus
// actions, never a second stacked modal.
type MemberDetail struct {
	RequestID       uint64
	UserID          domain.UserID
	Name            string
	Username        string
	Role            domain.ChatMemberRole
	Tag             string
	Avatar          domain.AvatarRef
	AvatarKey       string
	Selected        int
	Working         bool
	CanManageInChat bool
	IsCurrent       bool
}

type ChatActionMenuState struct {
	RequestID     uint64
	ChatID        domain.ChatID
	PreviousFocus Focus
	Selected      int
	Working       bool
	Confirming    Action
}

type MembersState struct {
	RequestID     uint64
	ChatID        domain.ChatID
	PreviousFocus Focus
	Loading       bool
	Error         *domain.AppError
	Results       []domain.ChatMember
	Selected      int
	NextOffset    int
	TotalCount    int
	Done          bool
	Detail        *MemberDetail
	Notice        string
	// Single opens the modal directly for one user (e.g. from a message's
	// User info action) with no member list behind the detail view.
	Single bool
}
type InviteLinksState struct {
	RequestID      uint64
	ChatID         domain.ChatID
	PreviousFocus  Focus
	Loading        bool
	Working        bool
	Error          *domain.AppError
	Primary        *telegram.InviteLink
	Links          []telegram.InviteLink
	Selected       int
	NextCursor     telegram.InviteLinkCursor
	Done           bool
	DetailURL      string
	DetailSelected int
	Confirming     bool
	Notice         string
}

type AdministrationMode uint8

const (
	AdministrationDefaultPermissions AdministrationMode = iota + 1
	AdministrationMemberMenu
	AdministrationAdminRightsEditor
	AdministrationRestrictionsEditor
	AdministrationConfirmation
)

type AdministrationState struct {
	RequestID         uint64
	ChatID            domain.ChatID
	UserID            domain.UserID
	PreviousFocus     Focus
	ReturnMembers     *MembersState
	Mode              AdministrationMode
	IsForum           bool
	Selected          int
	Loading           bool
	MemberLoading     bool
	Working           bool
	Error             *domain.AppError
	Snapshot          *telegram.AdministrationSnapshot
	MemberStatus      *telegram.MemberAdministrationStatus
	EditedPermissions telegram.ChatPermissions
	EditedRights      telegram.AdministratorRights
	PendingAction     telegram.MemberAdministrationAction
	Notice            string
}

type PinnedMessagesState struct {
	RequestID         uint64
	ChatID            domain.ChatID
	TopicID           domain.TopicID
	PreviousFocus     Focus
	Loading           bool
	Error             *domain.AppError
	Results           []domain.Message
	Selected          int
	NextFromMessageID domain.MessageID
	TotalCount        int
	Done              bool
	JumpMessageID     domain.MessageID
}

type attachmentOpenKey struct {
	ChatID    domain.ChatID
	MessageID domain.MessageID
}

type attachmentOpenRequest struct {
	RequestID uint64
	Kind      domain.MessageKind
	Title     string
	File      domain.MediaFileRef
}

type State struct {
	Width                    int
	Height                   int
	Layout                   Layout
	Focus                    Focus
	FocusBeforeInfo          Focus
	Connection               domain.ConnectionState
	TerminalFocused          bool // true when the terminal is visible/focused; conservative default
	Chats                    []domain.Chat
	SelectedChat             int
	Messages                 map[domain.ChatID][]domain.Message
	SelectedMessageChat      domain.ChatID
	SelectedMessage          domain.MessageID
	MessageMenu              *MessageActionMenu
	ForwardPicker            *ForwardPicker
	ReactionPicker           *ReactionPicker
	ReplyTarget              *ReplyTarget
	EditTarget               *EditTarget
	Drafts                   map[domain.ChatID]string
	DraftReplies             map[domain.ChatID]domain.MessageID
	DraftDates               map[domain.ChatID]int64
	DraftSync                map[domain.ChatID]DraftSyncState
	Avatars                  map[string]AvatarState
	History                  map[domain.ChatID]HistoryState
	NextRequestID            uint64
	NextEditorID             uint64
	NextLocalID              domain.MessageID
	ChatRequestID            uint64
	Fatal                    *domain.AppError
	ChatsLoading             bool
	ChatsLoaded              bool
	ChatsError               *domain.AppError
	DetailsOpen              bool
	Modal                    *ModalState
	Prompt                   *PromptState
	PhotoSend                *PhotoSendState
	PhotoSendRequests        map[domain.MessageID]uint64
	VideoSendRequests        map[domain.MessageID]uint64
	AudioSendRequests        map[domain.MessageID]uint64
	DocumentSendRequests     map[domain.MessageID]uint64
	StickerSendRequests      map[domain.MessageID]uint64
	StickerPicker            *StickerPickerState
	BotCommandCatalogs       map[domain.ChatID]BotCommandCatalogState
	CommandMenu              *CommandMenuState
	MessageSearch            *MessageSearchState
	ChatSearch               *ChatSearchState
	PinnedMessages           *PinnedMessagesState
	Members                  *MembersState
	InviteLinks              *InviteLinksState
	Administration           *AdministrationState
	ChatSettings             *ChatSettingsState
	ChatActions              *ChatActionMenuState
	DetailsSelected          int
	StickerThumbnails        map[int32]thumbnail.Block
	StickerThumbnailRequests map[int32]uint64
	Topics                   *TopicListState
	ShowAll                  map[domain.ChatID]bool
	ForumTopics              map[domain.ChatID]map[domain.TopicID]domain.ForumTopic
	SelectedTopics           map[domain.ChatID]domain.TopicID
	TopicHistory             map[topicKey]HistoryState
	TopicDrafts              map[topicKey]string
	TopicDraftReplies        map[topicKey]domain.MessageID
	TopicDraftDates          map[topicKey]int64
	TopicDraftSync           map[topicKey]DraftSyncState
	Toast                    *domain.AppError
	ToastGeneration          uint64
	ToastDuration            time.Duration
	NextToastGeneration      uint64
	Quitting                 bool
	Thumbnails               map[domain.ChatID]map[domain.MessageID]thumbnail.Block
	VideoOpenPending         map[domain.MessageID]uint64
	AudioOpenPending         map[domain.MessageID]uint64
	AttachmentOpenPending    map[attachmentOpenKey]attachmentOpenRequest
}

func InitialState() State {
	return State{
		Layout:                   LayoutTooSmall,
		Focus:                    FocusChats,
		TerminalFocused:          true,
		Connection:               domain.ConnectionWaiting,
		Messages:                 make(map[domain.ChatID][]domain.Message),
		Drafts:                   make(map[domain.ChatID]string),
		DraftReplies:             make(map[domain.ChatID]domain.MessageID),
		DraftDates:               make(map[domain.ChatID]int64),
		DraftSync:                make(map[domain.ChatID]DraftSyncState),
		Avatars:                  make(map[string]AvatarState),
		History:                  make(map[domain.ChatID]HistoryState),
		PhotoSendRequests:        make(map[domain.MessageID]uint64),
		VideoSendRequests:        make(map[domain.MessageID]uint64),
		AudioSendRequests:        make(map[domain.MessageID]uint64),
		DocumentSendRequests:     make(map[domain.MessageID]uint64),
		StickerSendRequests:      make(map[domain.MessageID]uint64),
		StickerThumbnails:        make(map[int32]thumbnail.Block),
		StickerThumbnailRequests: make(map[int32]uint64),
		BotCommandCatalogs:       make(map[domain.ChatID]BotCommandCatalogState),
		NextRequestID:            1,
		NextEditorID:             1,
		NextLocalID:              -1,
		NextToastGeneration:      1,
		Thumbnails:               make(map[domain.ChatID]map[domain.MessageID]thumbnail.Block),
		VideoOpenPending:         make(map[domain.MessageID]uint64),
		AudioOpenPending:         make(map[domain.MessageID]uint64),
		AttachmentOpenPending:    make(map[attachmentOpenKey]attachmentOpenRequest),
		ShowAll:                  make(map[domain.ChatID]bool),
		ForumTopics:              make(map[domain.ChatID]map[domain.TopicID]domain.ForumTopic),
		SelectedTopics:           make(map[domain.ChatID]domain.TopicID),
		TopicHistory:             make(map[topicKey]HistoryState),
		TopicDrafts:              make(map[topicKey]string),
		TopicDraftReplies:        make(map[topicKey]domain.MessageID),
		TopicDraftDates:          make(map[topicKey]int64),
		TopicDraftSync:           make(map[topicKey]DraftSyncState),
	}
}

func layoutForSize(width, height int) Layout {
	if width < 60 || height < 18 {
		return LayoutTooSmall
	}
	if width >= 120 && height >= 24 {
		return LayoutWide
	}
	if width >= 80 && height >= 20 {
		return LayoutNormal
	}
	return LayoutNarrow
}
