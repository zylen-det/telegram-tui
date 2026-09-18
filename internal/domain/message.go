package domain

import "time"

type MessageReaction struct {
	Emoji  string
	Count  int
	Chosen bool
}

type MediaFileRef struct {
	ID           int32
	UniqueID     string
	Size         int64
	ExpectedSize int64
	LocalPath    string
	CanDownload  bool
	Downloaded   bool
}

// StickerRef is a Telegram-independent reference to an existing Telegram
// sticker. File.ID is the send identity; Thumbnail is the only file eligible
// for automatic download and terminal rendering.
type StickerRef struct {
	File      MediaFileRef
	Thumbnail MediaFileRef
	Width     int
	Height    int
	Emoji     string
}

type MessageMedia struct {
	File      MediaFileRef
	Thumbnail MediaFileRef
	MIMEType  string
	Width     int
	Height    int
	Duration  time.Duration
}

type MessageKind uint8

const (
	MessageText MessageKind = iota
	MessagePhoto
	MessageVideo
	MessageAudio
	MessageSticker
	MessageDocument
	MessageService
	MessageUnsupported
	// MessageAnimation, MessageVoiceNote and MessageVideoNote are file-backed
	// attachment kinds appended after the accepted kinds so existing ordinal
	// values stay stable.
	MessageAnimation
	MessageVoiceNote
	MessageVideoNote
)

type SendState uint8

const (
	SendNone SendState = iota
	SendPending
	SendSucceeded
	SendFailed
)

type Message struct {
	ID               MessageID
	ChatID           ChatID
	TopicID          TopicID
	Sender           SenderRef
	SenderName       string
	SenderAvatar     AvatarRef
	SentAt           time.Time
	EditedAt         time.Time
	Kind             MessageKind
	Text             string
	FileName         string
	Media            MessageMedia
	Sticker          StickerRef
	Outgoing         bool
	Service          bool
	HasReply         bool
	ReplyToMessageID MessageID
	HasForward       bool
	Pinned           bool
	Reactions        []MessageReaction
	SendState        SendState
	Failure          *AppError
	RetryAt          time.Time
}

func (m Message) Edited() bool { return !m.EditedAt.IsZero() }

type MessageCapabilities struct {
	Copy          bool
	Reply         bool
	Forward       bool
	Edit          bool
	Pin           bool
	DeleteForSelf bool
	DeleteForAll  bool
}

func (m Message) Capabilities() MessageCapabilities {
	// Pin is authoritative-only: the local fallback never grants it.
	return MessageCapabilities{Copy: true, Reply: m.ID > 0 && !m.Service && m.Kind != MessageService}
}

func (m Message) DisplayText() string {
	switch m.Kind {
	case MessageText:
		return m.Text
	case MessagePhoto:
		return "[Photo]"
	case MessageVideo:
		return "[Video]"
	case MessageAudio:
		return "[Audio]"
	case MessageSticker:
		return "[Sticker]"
	case MessageDocument:
		if m.FileName == "" {
			return "[File]"
		}
		return "[File: " + m.FileName + "]"
	case MessageAnimation:
		return "[Animation]"
	case MessageVoiceNote:
		return "[Voice note]"
	case MessageVideoNote:
		return "[Video note]"
	case MessageService:
		return m.Text
	default:
		return "[Unsupported message]"
	}
}
