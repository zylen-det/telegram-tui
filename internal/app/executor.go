package app

import (
	"context"
	"sync"
)

type CommandHandler interface {
	Handle(ctx context.Context, command Command, emit func(Event))
}

type Executor struct {
	workers int
	handler CommandHandler
}

func NewExecutor(workers int, handler CommandHandler) *Executor {
	if workers < 1 {
		workers = 1
	}
	return &Executor{workers: workers, handler: handler}
}

func (e *Executor) Run(ctx context.Context, commands <-chan Command, events chan<- Event) {
	var workers sync.WaitGroup
	workers.Add(e.workers)
	for range e.workers {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case command, ok := <-commands:
					if !ok {
						return
					}
					e.handler.Handle(ctx, command, func(event Event) {
						select {
						case events <- event:
						case <-ctx.Done():
						}
					})
				}
			}
		}()
	}
	workers.Wait()
}
