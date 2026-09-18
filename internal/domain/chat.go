package domain

type AvatarRef struct {
	FileID           int32
	UniqueID         string
	OriginalFileID   int32
	OriginalUniqueID string
}

type User struct {
	ID        UserID
	Name      string
	Username  string
	Avatar    AvatarRef
	IsCurrent bool
}

type ChatMemberRole uint8

const (
	ChatMemberRoleMember ChatMemberRole = iota
	ChatMemberRoleOwner
	ChatMemberRoleAdministrator
	ChatMemberRoleRestricted
)

// ChatMember is the Telegram-independent presentation data for one visible
// member of a basic group, supergroup, or channel.
type ChatMember struct {
	User User
	Role ChatMemberRole
	Tag  string
}

// Draft is the supported plain-text portion of a Telegram cloud draft.
// A zero value means that no supported text draft is present.
type Draft struct {
	Text             string
	ReplyToMessageID MessageID
	Date             int64
}

type Chat struct {
	ID                   ChatID
	Kind                 ChatKind
	IsForum              bool
	Title                string
	Username             string
	Avatar               AvatarRef
	LastMessage          string
	LastMessageAt        int64
	UnreadCount          int
	UnreadMentionCount   int
	Muted                bool
	CanSend              bool
	CanReact             bool
	IsPinned             bool
	IsArchived           bool
	IsMarkedUnread       bool
	CanDeleteForSelf     bool
	CanDeleteForAll      bool
	IsMember             bool
	CanManageInviteLinks bool
	CanRestrictMembers   bool
	CanPromoteMembers    bool
	CanChangeInfo        bool
	Order                int64
	Draft                Draft
}

// ForumTopic is the Telegram-independent state of one topic in a forum
// supergroup or a bot chat with topics.
type ForumTopic struct {
	ID                 TopicID
	ChatID             ChatID
	Name               string
	IconColor          int32
	IsGeneral          bool
	IsClosed           bool
	IsHidden           bool
	IsPinned           bool
	UnreadCount        int
	UnreadMentionCount int
	LastMessage        string
	LastMessageAt      int64
	Order              int64
	Draft              Draft
}

// BotCommand is one command advertised by a Telegram bot for a chat.
// BotUsername is empty in a private bot chat and identifies the target bot in
// a group when TDLib provides it.
type BotCommand struct {
	Name        string
	Description string
	BotUsername string
}

// Invocation is the text inserted into the composer for this command.
func (command BotCommand) Invocation() string {
	text := "/" + command.Name
	if command.BotUsername != "" {
		text += "@" + command.BotUsername
	}
	return text
}
