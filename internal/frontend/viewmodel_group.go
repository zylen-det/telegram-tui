package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

type MessageGroup struct {
	Sender        domain.SenderRef
	SenderName    string
	SenderAvatar  domain.AvatarRef
	ShowAvatar    bool
	Messages      []domain.Message
	ReplyContexts map[domain.MessageID]ReplyContext
}

type ReplyContext struct {
	Available bool
	Sender    string
	Preview   string
}

func GroupMessages(kind domain.ChatKind, messages []domain.Message, location *time.Location) []MessageGroup {
	groups := make([]MessageGroup, 0, len(messages))
	activeChatID := domain.ChatID(0)
	for _, message := range messages {
		if message.ChatID != 0 {
			activeChatID = message.ChatID
			break
		}
	}
	cache := make(map[domain.MessageID]domain.Message)
	for _, message := range messages {
		if message.ChatID == activeChatID {
			cache[message.ID] = message
		}
	}
	for _, message := range messages {
		var replyContext ReplyContext
		if message.HasReply {
			if message.ReplyToMessageID > 0 {
				if message.ChatID == activeChatID {
					if target, ok := cache[message.ReplyToMessageID]; ok {
						replyContext = ReplyContext{Available: true, Sender: target.SenderName, Preview: target.DisplayText()}
						if replyContext.Sender == "" {
							if target.Outgoing {
								replyContext.Sender = "You"
							} else {
								replyContext.Sender = "Unknown"
							}
						}
					}
				}
			}
		}
		if kind == domain.ChatPrivate && !message.Service {
			if message.SenderName == "" {
				if message.Outgoing {
					message.SenderName = "You"
				} else {
					message.SenderName = "Unknown"
				}
			}
			if message.SenderAvatar.UniqueID == "" {
				message.SenderAvatar.UniqueID = "placeholder:" + message.SenderName
			}
		}
		if len(groups) > 0 {
			group := &groups[len(groups)-1]
			left := group.Messages[len(group.Messages)-1]
			if messagesJoin(kind, left, message, location) {
				group.Messages = append(group.Messages, message)
				if message.HasReply {
					if group.ReplyContexts == nil {
						group.ReplyContexts = make(map[domain.MessageID]ReplyContext)
					}
					group.ReplyContexts[message.ID] = replyContext
				}
				continue
			}
		}

		groups = append(groups, MessageGroup{
			Sender:        message.Sender,
			SenderName:    message.SenderName,
			SenderAvatar:  message.SenderAvatar,
			ShowAvatar:    !message.Service && (kind == domain.ChatPrivate || isGroupChat(kind)),
			Messages:      []domain.Message{message},
			ReplyContexts: replyContextsFor(message, replyContext),
		})
	}
	return groups
}

func replyContextsFor(message domain.Message, context ReplyContext) map[domain.MessageID]ReplyContext {
	if !message.HasReply {
		return nil
	}
	return map[domain.MessageID]ReplyContext{message.ID: context}
}

func messagesJoin(kind domain.ChatKind, left, right domain.Message, location *time.Location) bool {
	if !isGroupChat(kind) ||
		left.Outgoing || right.Outgoing ||
		left.Service || right.Service ||
		left.HasReply || right.HasReply ||
		left.HasForward || right.HasForward ||
		left.Sender != right.Sender {
		return false
	}

	leftLocal := left.SentAt.In(location)
	rightLocal := right.SentAt.In(location)
	if leftLocal.Year() != rightLocal.Year() || leftLocal.YearDay() != rightLocal.YearDay() {
		return false
	}

	gap := right.SentAt.Sub(left.SentAt)
	return gap >= 0 && gap <= 5*time.Minute
}

func isGroupChat(kind domain.ChatKind) bool {
	return kind == domain.ChatBasicGroup || kind == domain.ChatSupergroup
}
