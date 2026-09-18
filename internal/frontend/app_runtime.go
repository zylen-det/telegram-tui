package frontend

import (
	"context"
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
)

const appRuntimeBuffer = 32

// AppRuntime owns the executor/event-channel pump used by AppModel. Handler
// goroutines using a separate process context remain owned by that context.
type AppRuntime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	commands chan app.Command
	events   chan app.Event
	done     chan struct{}
	stopOnce sync.Once
}

// NewAppRuntime starts an executor-backed event-channel pump for an app model.
func NewAppRuntime(parent context.Context, executor *app.Executor) (*AppRuntime, error) {
	if executor == nil {
		return nil, errors.New("frontend: nil app executor")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	runtime := &AppRuntime{
		ctx:      ctx,
		cancel:   cancel,
		commands: make(chan app.Command, appRuntimeBuffer),
		events:   make(chan app.Event, appRuntimeBuffer),
		done:     make(chan struct{}),
	}
	go func() {
		defer close(runtime.done)
		defer cancel()
		executor.Run(ctx, runtime.commands, runtime.events)
	}()
	return runtime, nil
}

func (r *AppRuntime) deliver(commands []app.Command) tea.Cmd {
	if r == nil || len(commands) == 0 {
		return nil
	}
	queued := append([]app.Command(nil), commands...)
	return func() tea.Msg {
		for _, command := range queued {
			select {
			case r.commands <- command:
			case <-r.ctx.Done():
				return nil
			}
		}
		return nil
	}
}

func (r *AppRuntime) waitEvent() tea.Cmd {
	if r == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case event, ok := <-r.events:
			if !ok {
				return appRuntimeStoppedMsg{}
			}
			return appEventMsg{event: event}
		case <-r.ctx.Done():
			return appRuntimeStoppedMsg{}
		}
	}
}

// Close requests runtime shutdown. It is safe to call more than once.
func (r *AppRuntime) Close() {
	if r == nil {
		return
	}
	r.stopOnce.Do(r.cancel)
}

// Wait blocks until the executor/event-channel pump has stopped.
func (r *AppRuntime) Wait() {
	if r == nil {
		return
	}
	<-r.done
}

// Done is closed after the executor/event-channel pump has stopped.
func (r *AppRuntime) Done() <-chan struct{} {
	if r == nil {
		return nil
	}
	return r.done
}
