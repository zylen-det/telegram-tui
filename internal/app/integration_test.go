package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestVerticalCommandUpdateSendAvatarAndShutdownFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chat := domain.Chat{
		ID:      9,
		Title:   "Weekend",
		CanSend: true,
		Order:   10,
		Avatar:  domain.AvatarRef{UniqueID: "weekend-small", OriginalUniqueID: "weekend-large"},
	}
	fake := telegram.NewFake(telegram.FakeData{
		Chats: []domain.Chat{chat},
		Messages: map[domain.ChatID][]domain.Message{
			9: {{ID: 10, ChatID: 9, Text: "cached", SentAt: time.Unix(10, 0)}},
		},
		Avatars: map[string]string{"weekend-small": "/tmp/small.png", "weekend-large": "/tmp/original.png"},
	})
	// Reserve -1 in the fake so the reducer's local optimistic ID and TDLib's
	// temporary ID are provably distinct during the vertical flow.
	if _, err := fake.SendText(ctx, telegram.SendTextRequest{ChatID: 99, Text: "temporary-id-primer"}); err != nil {
		t.Fatalf("prime fake temporary ID: %v", err)
	}
	client := &closingIntegrationClient{Fake: fake, closed: make(chan struct{})}
	renderer := &integrationAvatarRenderer{}
	handler := NewHandler(ctx, &handlerResolver{}, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, auth.NewBroker(), renderer)

	commands := make(chan Command, 32)
	events := make(chan Event, 64)
	executorDone := make(chan struct{})
	go func() {
		NewExecutor(4, handler).Run(ctx, commands, events)
		close(executorDone)
	}()
	engine := NewEngine(InitialState())
	engine.Apply(Resized{Width: 140, Height: 30})
	queueCommands(commands, engine.Apply(Started{}))

	seenReady := false
	driveEngineUntil(t, engine, commands, events, func(event Event, state State) bool {
		if telegramEvent, ok := event.(TelegramEvent); ok {
			_, seenReady = telegramEvent.Value.(telegram.Ready)
		}
		return seenReady
	})
	if err := fake.Emit(ctx, telegram.ConnectionChanged{State: domain.ConnectionOnline}); err != nil {
		t.Fatalf("Emit(connection) error = %v", err)
	}
	driveEngineUntil(t, engine, commands, events, func(_ Event, state State) bool {
		return state.Connection == domain.ConnectionOnline && state.ChatsLoaded && len(state.Messages[9]) == 1 && !state.History[9].Loading
	})

	incoming := domain.Message{ID: 11, ChatID: 9, Text: "incoming", SentAt: time.Unix(11, 0), SenderAvatar: domain.AvatarRef{UniqueID: "sender"}}
	if err := fake.Emit(ctx, telegram.MessageUpserted{Message: incoming}); err != nil {
		t.Fatalf("Emit(incoming) error = %v", err)
	}
	driveEngineUntil(t, engine, commands, events, func(_ Event, state State) bool {
		return messageIndex(state.Messages[9], 11) >= 0
	})

	queueCommands(commands, engine.Apply(ActionReceived{Action: FocusPane, TargetFocus: FocusComposer}))
	engine.Apply(ActionReceived{Rune: 'h'})
	engine.Apply(ActionReceived{Rune: 'i'})
	queueCommands(commands, engine.Apply(ActionReceived{Action: ComposerSubmit, At: time.Unix(12, 0)}))
	if engine.Snapshot().Drafts[9] != "" {
		t.Fatal("submit did not clear draft immediately")
	}
	driveEngineUntil(t, engine, commands, events, func(event Event, _ State) bool {
		_, ok := event.(TextQueued)
		return ok
	})
	queued := engine.Snapshot().Messages[9]
	if messageIndex(queued, -1) >= 0 || messageIndex(queued, -2) < 0 {
		t.Fatalf("local optimistic ID was not replaced by distinct Telegram temporary ID: %#v", queued)
	}
	if err := fake.Emit(ctx, telegram.MessageSendSucceeded{OldID: -2, Message: domain.Message{ID: 100, ChatID: 9, Text: "hi", SentAt: time.Unix(13, 0), Outgoing: true, SendState: domain.SendSucceeded}}); err != nil {
		t.Fatalf("Emit(send success) error = %v", err)
	}
	driveEngineUntil(t, engine, commands, events, func(_ Event, state State) bool {
		return messageIndex(state.Messages[9], 100) >= 0 && messageIndex(state.Messages[9], -1) < 0
	})

	engine.Apply(ActionReceived{Action: ToggleDetails})
	queueCommands(commands, engine.Apply(ActionReceived{Action: Activate}))
	driveEngineUntil(t, engine, commands, events, func(_ Event, state State) bool {
		return state.Modal != nil && !state.Modal.Loading && state.Modal.Path == "/tmp/original.png"
	})
	if err := fake.Emit(ctx, telegram.ConnectionChanged{State: domain.ConnectionOffline}); err != nil {
		t.Fatalf("Emit(offline) error = %v", err)
	}
	offlineMessage := domain.Message{ID: 12, ChatID: 9, Text: "offline cached", SentAt: time.Unix(14, 0)}
	if err := fake.Emit(ctx, telegram.MessageUpserted{Message: offlineMessage}); err != nil {
		t.Fatalf("Emit(offline incoming) error = %v", err)
	}
	driveEngineUntil(t, engine, commands, events, func(_ Event, state State) bool {
		return state.Connection == domain.ConnectionOffline && messageIndex(state.Messages[9], 12) >= 0
	})

	queueCommands(commands, engine.Apply(ActionReceived{Action: Quit}))
	driveEngineUntil(t, engine, commands, events, func(event Event, _ State) bool {
		_, ok := event.(ShutdownComplete)
		return ok
	})
	select {
	case <-client.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("client was not closed")
	}
	close(commands)
	cancel()
	select {
	case <-executorDone:
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not stop")
	}
}

func driveEngineUntil(t *testing.T, engine *Engine, commands chan<- Command, events <-chan Event, done func(Event, State) bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			queueCommands(commands, engine.Apply(event))
			if done(event, engine.Snapshot()) {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out; state = %#v", engine.Snapshot())
		}
	}
}

func queueCommands(target chan<- Command, commands []Command) {
	for _, command := range commands {
		target <- command
	}
}

type closingIntegrationClient struct {
	*telegram.Fake
	closed chan struct{}
	once   sync.Once
}

func (c *closingIntegrationClient) Close(ctx context.Context) error {
	c.once.Do(func() { close(c.closed) })
	return c.Fake.Close(ctx)
}

type integrationAvatarRenderer struct{}

func (r *integrationAvatarRenderer) Render(_ context.Context, _ string, _ domain.AvatarRef, role avatar.Role) (pixel.Avatar, error) {
	return pixel.Avatar{Width: 1, Height: 1, Cells: []pixel.Cell{{Rune: 'A'}}}, nil
}
