package domain

type ChatID int64

type MessageID int64

type TopicID int32

type UserID int64

type SenderKind uint8

const (
	SenderUser SenderKind = iota
	SenderChat
)

type SenderRef struct {
	Kind SenderKind
	ID   int64
}

type ChatKind uint8

const (
	ChatPrivate ChatKind = iota
	ChatBasicGroup
	ChatSupergroup
	ChatChannel
)

type ConnectionState uint8

const (
	ConnectionWaiting ConnectionState = iota
	ConnectionOnline
	ConnectionOffline
	ConnectionReconnecting
)
