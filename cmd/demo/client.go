package main

import (
	"context"
	"sync"
	"time"

	"github.com/zylen-det/tuilegram/internal/domain"
	"github.com/zylen-det/tuilegram/internal/telegram"
)

// demoClient announces an online fake account and settles locally sent texts.
// The regular Fake only announces Ready and leaves sends permanently pending.
type demoClient struct {
	*telegram.Fake
	ctx    context.Context
	mu     sync.Mutex
	nextID domain.MessageID
	sent   map[domain.MessageID]domain.Message
}

func newDemoClient(ctx context.Context, data telegram.FakeData) *demoClient {
	return &demoClient{Fake: telegram.NewFake(data), ctx: ctx, nextID: 1_000_000, sent: make(map[domain.MessageID]domain.Message)}
}

func (c *demoClient) Start(ctx context.Context, updates chan<- telegram.Update) error {
	bridge := make(chan telegram.Update, 64)
	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		for update := range bridge {
			select {
			case updates <- update:
			case <-ctx.Done():
				return
			}
			if _, ready := update.(telegram.Ready); ready {
				select {
				case updates <- telegram.ConnectionChanged{State: domain.ConnectionOnline}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	err := c.Fake.Start(ctx, bridge)
	close(bridge)
	<-forwarded
	return err
}

func (c *demoClient) SendText(ctx context.Context, request telegram.SendTextRequest) (domain.Message, error) {
	queued, err := c.Fake.SendText(ctx, request)
	if err != nil {
		return queued, err
	}
	c.mu.Lock()
	settled := queued
	settled.ID = c.nextID
	c.nextID++
	settled.Sender = domain.SenderRef{Kind: domain.SenderUser, ID: 1}
	settled.SenderName = "You"
	settled.SendState = domain.SendSucceeded
	c.sent[queued.ID] = settled
	c.mu.Unlock()

	// The queue result reaches the frontend first. The later update replaces
	// its temporary ID just as the real Telegram update stream does.
	go func() {
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = c.Fake.Emit(c.ctx, telegram.MessageSendSucceeded{OldID: queued.ID, Message: settled})
		case <-c.ctx.Done():
		}
	}()
	return queued, nil
}

func (c *demoClient) LoadMessages(ctx context.Context, chatID domain.ChatID, cursor telegram.MessageCursor) (telegram.MessagePage, error) {
	page, err := c.Fake.LoadMessages(ctx, chatID, cursor)
	if err != nil {
		return page, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, message := range page.Messages {
		if settled, ok := c.sent[message.ID]; ok {
			page.Messages[i] = settled
		}
	}
	return page, nil
}

func (c *demoClient) GetMessageProperties(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (domain.MessageCapabilities, error) {
	c.mu.Lock()
	for _, message := range c.sent {
		if message.ID == messageID && message.ChatID == chatID {
			c.mu.Unlock()
			return domain.MessageCapabilities{Copy: true, Reply: true, Forward: true}, nil
		}
	}
	c.mu.Unlock()
	return c.Fake.GetMessageProperties(ctx, chatID, messageID)
}
