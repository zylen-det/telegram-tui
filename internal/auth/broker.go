package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	errNoActivePrompt   = errors.New("no active authorization prompt")
	errPromptActive     = errors.New("authorization prompt already active")
	errResponseMismatch = errors.New("authorization response ID mismatch")
)

type PromptKind uint8

const (
	PromptAPIID PromptKind = iota
	PromptAPIHash
	PromptPhone
	PromptCode
	PromptPassword
)

type Prompt struct {
	ID     uint64
	Kind   PromptKind
	Label  string
	Secret bool
}

type Response struct {
	PromptID uint64
	Value    string
}

type Prompter interface {
	Ask(context.Context, Prompt) (string, error)
}

type Broker struct {
	nextID  atomic.Uint64
	prompts chan Prompt
	mu      sync.Mutex
	active  *activeRequest
}

type activeRequest struct {
	promptID uint64
	reply    chan Response
	done     chan struct{}
}

func NewBroker() *Broker {
	return &Broker{
		prompts: make(chan Prompt),
	}
}

func (b *Broker) Prompts() <-chan Prompt {
	return b.prompts
}

func (b *Broker) Ask(ctx context.Context, prompt Prompt) (string, error) {
	prompt.ID = b.nextID.Add(1)
	request := &activeRequest{
		promptID: prompt.ID,
		reply:    make(chan Response),
		done:     make(chan struct{}),
	}

	b.mu.Lock()
	if b.active != nil {
		b.mu.Unlock()
		return "", errPromptActive
	}
	b.active = request
	b.mu.Unlock()
	defer b.finish(request)

	select {
	case b.prompts <- prompt:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	select {
	case response := <-request.reply:
		if response.PromptID != prompt.ID {
			return "", errResponseMismatch
		}
		return response.Value, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (b *Broker) Submit(ctx context.Context, response Response) error {
	b.mu.Lock()
	request := b.active
	if request == nil {
		b.mu.Unlock()
		return errNoActivePrompt
	}
	if response.PromptID != request.promptID {
		b.mu.Unlock()
		return errResponseMismatch
	}
	b.mu.Unlock()

	select {
	case request.reply <- response:
		// Do not let a successful submit race the next authorization prompt.
		// Ask clears the active request in its deferred finish after receiving
		// this response; waiting here makes sequential TDLib prompts atomic.
		select {
		case <-request.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-request.done:
		return errNoActivePrompt
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Broker) finish(request *activeRequest) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.active == request {
		b.active = nil
		close(request.done)
	}
}
