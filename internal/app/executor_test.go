package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type commandHandlerFunc func(context.Context, Command, func(Event))

var _ CommandHandler = commandHandlerFunc(nil)

func (f commandHandlerFunc) Handle(ctx context.Context, command Command, emit func(Event)) {
	f(ctx, command, emit)
}

func TestNewExecutorClampsWorkerCountToOne(t *testing.T) {
	handler := commandHandlerFunc(func(context.Context, Command, func(Event)) {})

	for _, workers := range []int{-1, 0} {
		executor := NewExecutor(workers, handler)
		if executor.workers != 1 {
			t.Fatalf("NewExecutor(%d) workers = %d, want 1", workers, executor.workers)
		}
	}
}

func TestExecutorLimitsConcurrentHandlers(t *testing.T) {
	const commandCount = 4

	started := make(chan struct{}, commandCount)
	release := make(chan struct{})
	completed := make(chan struct{}, commandCount)
	var active atomic.Int32
	var maximum atomic.Int32
	handler := commandHandlerFunc(func(context.Context, Command, func(Event)) {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		completed <- struct{}{}
	})
	executor := NewExecutor(2, handler)
	commands := make(chan Command, commandCount)
	events := make(chan Event)
	for range commandCount {
		commands <- LoadChats{}
	}

	runDone := make(chan struct{})
	go func() {
		executor.Run(context.Background(), commands, events)
		close(runDone)
	}()

	receiveWithTimeout(t, started, "first handler start")
	receiveWithTimeout(t, started, "second handler start")
	select {
	case <-started:
		t.Fatal("third handler started while both workers were blocked")
	default:
	}

	close(release)
	for range commandCount {
		receiveWithTimeout(t, completed, "handler completion")
	}
	close(commands)
	receiveWithTimeout(t, runDone, "executor return")

	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrent handlers = %d, want 2", got)
	}
}

func TestExecutorForwardsEmittedEvents(t *testing.T) {
	want := Resized{Width: 140, Height: 30}
	handler := commandHandlerFunc(func(_ context.Context, _ Command, emit func(Event)) {
		emit(want)
	})
	executor := NewExecutor(1, handler)
	commands := make(chan Command, 1)
	events := make(chan Event)
	commands <- LoadChats{}
	close(commands)

	runDone := make(chan struct{})
	go func() {
		executor.Run(context.Background(), commands, events)
		close(runDone)
	}()

	got := receiveWithTimeout(t, events, "emitted event")
	if got != want {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
	receiveWithTimeout(t, runDone, "executor return")
}

func TestExecutorReturnsWhenCommandsClose(t *testing.T) {
	handler := commandHandlerFunc(func(context.Context, Command, func(Event)) {
		t.Fatal("handler called after commands channel closed")
	})
	executor := NewExecutor(2, handler)
	commands := make(chan Command)
	events := make(chan Event)
	close(commands)

	runDone := make(chan struct{})
	go func() {
		executor.Run(context.Background(), commands, events)
		close(runDone)
	}()

	receiveWithTimeout(t, runDone, "executor return")
}

func TestExecutorCancellationUnblocksEventEmission(t *testing.T) {
	handlerStarted := make(chan struct{})
	handlerReturned := make(chan struct{})
	handler := commandHandlerFunc(func(_ context.Context, _ Command, emit func(Event)) {
		close(handlerStarted)
		emit(Started{})
		close(handlerReturned)
	})
	executor := NewExecutor(1, handler)
	commands := make(chan Command, 1)
	events := make(chan Event)
	commands <- LoadChats{}
	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan struct{})
	go func() {
		executor.Run(ctx, commands, events)
		close(runDone)
	}()

	receiveWithTimeout(t, handlerStarted, "handler start")
	cancel()
	receiveWithTimeout(t, handlerReturned, "handler return after cancellation")
	receiveWithTimeout(t, runDone, "executor return after cancellation")
}

func receiveWithTimeout[T any](t *testing.T, ch <-chan T, description string) T {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}
