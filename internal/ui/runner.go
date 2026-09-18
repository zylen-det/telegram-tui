package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"syscall"
	"time"

	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type ImageManager interface {
	Show(image.Rectangle, image.Image) error
	Clear() error
}

type Runner struct {
	Backend    Backend
	Root       *Root
	Engine     *app.Engine
	Commands   chan<- app.Command
	Events     <-chan app.Event
	Prompts    <-chan auth.Prompt
	Images     ImageManager
	Signals    <-chan os.Signal
	Location   *time.Location
	CellPixels func(columns, rows int) image.Point
	Suspend    func() error
	Resume     func() (Backend, error)
	Now        func() time.Time

	placement  imagePlacement
	wantedRect image.Rectangle
	imageSize  image.Point
	suspended  bool
}

type imagePlacement struct {
	path string
	rect image.Rectangle
}

func (r *Runner) Run(ctx context.Context) (err error) {
	if r.Backend == nil || r.Root == nil || r.Engine == nil {
		return errors.New("ui runner requires backend, root, and engine")
	}
	backend := r.Backend
	defer func() { backend.Close() }()
	if r.Images != nil {
		defer func() {
			if clearErr := r.Images.Clear(); err == nil && clearErr != nil {
				err = clearErr
			}
		}()
	}

	width, height := backend.TerminalDimensions()
	if err := r.apply(ctx, app.Resized{Width: width, Height: height}, true); err != nil {
		return err
	}
	eventStream := backend.PollEventsWithContext(ctx)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("terminal lifecycle interrupted: %w", ctx.Err())
		case terminalEvent, ok := <-eventStream:
			if !ok {
				eventStream = nil
				continue
			}
			event, mapped := r.mapTerminalEvent(terminalEvent)
			if mapped {
				if err := r.apply(ctx, event, true); err != nil {
					return err
				}
			}
		case event, ok := <-r.Events:
			if !ok {
				r.Events = nil
				continue
			}
			shutdown, shutdownErr := shutdownResult(event)
			if err := r.apply(ctx, event, true); err != nil {
				return err
			}
			if shutdown {
				state := r.Engine.Snapshot()
				if state.Fatal != nil {
					return *state.Fatal
				}
				return shutdownErr
			}
		case prompt, ok := <-r.Prompts:
			if !ok {
				r.Prompts = nil
				continue
			}
			if err := r.apply(ctx, app.PromptRequested{Prompt: prompt}, true); err != nil {
				return err
			}
		case signal, ok := <-r.Signals:
			if !ok {
				r.Signals = nil
				continue
			}
			switch signal {
			case syscall.SIGTSTP:
				if r.Images != nil {
					_ = r.Images.Clear()
				}
				r.placement = imagePlacement{}
				backend.Close()
				r.suspended = true
				if r.Suspend != nil {
					if err := r.Suspend(); err != nil {
						return fmt.Errorf("suspend terminal: %w", err)
					}
				}
			case syscall.SIGCONT:
				if !r.suspended {
					continue
				}
				if r.Resume == nil {
					return errors.New("resume terminal: no resume hook")
				}
				resumed, resumeErr := r.Resume()
				if resumeErr != nil {
					return fmt.Errorf("resume terminal: %w", resumeErr)
				}
				backend = resumed
				r.Backend = resumed
				r.suspended = false
				eventStream = backend.PollEventsWithContext(ctx)
				backend.Clear()
				r.render(backend)
				if err := r.reconcileImage(); err != nil {
					return err
				}
			case os.Interrupt, syscall.SIGTERM:
				if err := r.apply(ctx, app.ActionReceived{Action: app.Quit, At: r.now()}, true); err != nil {
					return err
				}
			}
		}
	}
}

func (r *Runner) mapTerminalEvent(event gotui.Event) (app.Event, bool) {
	if event.Type == gotui.ResizeEvent {
		resize, ok := event.Payload.(gotui.Resize)
		if !ok {
			return nil, false
		}
		return app.Resized{Width: resize.Width, Height: resize.Height}, true
	}
	state := r.Engine.Snapshot()
	if action, ok := MapKey(state.Focus, event); ok {
		if action.At.IsZero() {
			action.At = r.now()
		}
		return action, true
	}
	if action, ok := MapMouse(event, r.Root.Hits()); ok {
		if action.At.IsZero() {
			action.At = r.now()
		}
		return action, true
	}
	return nil, false
}

func (r *Runner) apply(ctx context.Context, event app.Event, render bool) error {
	before := r.Engine.Snapshot()
	if shouldClearPlacement(before, event) {
		if err := r.clearImage(); err != nil {
			return err
		}
	}
	commands := r.Engine.Apply(event)
	state := r.Engine.Snapshot()
	if placementInvalidated(before, state) {
		if err := r.clearImage(); err != nil {
			return err
		}
	}
	for _, command := range commands {
		if state.Quitting {
			if _, allowed := command.(app.BeginShutdown); !allowed {
				continue
			}
		}
		if r.Commands == nil {
			continue
		}
		select {
		case r.Commands <- command:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	model := Select(state, r.Location)
	r.prepareModalRect(&model)
	r.Root.Update(model)
	if render && !r.suspended {
		r.render(r.Backend)
		return r.reconcileImage()
	}
	return nil
}

func (r *Runner) render(backend Backend) { backend.Render(r.Root) }

func (r *Runner) prepareModalRect(model *ViewModel) {
	r.wantedRect = image.Rectangle{}
	if model.Modal == nil || model.Modal.Loading || model.Modal.Error != nil || model.Modal.Path == "" {
		return
	}
	bounds := modalContentRect(*model)
	cell := image.Pt(1, 2)
	if r.CellPixels != nil {
		cell = r.CellPixels(model.Width, model.Height)
	}
	if r.placement.path == model.Modal.Path && !r.placement.rect.Empty() {
		r.wantedRect = fitImageRect(bounds, r.imageSize, cell)
		model.ModalImage = r.wantedRect
		return
	}
	decoded, err := decodeImage(model.Modal.Path)
	if err != nil {
		return
	}
	r.imageSize = decoded.Bounds().Size()
	r.wantedRect = fitImageRect(bounds, r.imageSize, cell)
	model.ModalImage = r.wantedRect
}

func modalContentRect(model ViewModel) image.Rectangle {
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if bounds.Empty() {
		return image.Rectangle{}
	}
	frame := centeredRectangle(bounds, max(4, bounds.Dx()*80/100), max(4, bounds.Dy()*80/100))
	inner := insetRectangle(frame, 2)
	if inner.Empty() {
		return frame
	}
	return inner
}

func (r *Runner) reconcileImage() error {
	if r.Images == nil {
		return nil
	}
	model := r.Root.Model()
	if model.Modal == nil || model.Modal.Path == "" || model.Modal.Loading || model.Modal.Error != nil || r.wantedRect.Empty() {
		return r.clearImage()
	}
	wanted := imagePlacement{path: model.Modal.Path, rect: r.wantedRect}
	if wanted == r.placement {
		return nil
	}
	if r.placement.path != "" {
		if err := r.Images.Clear(); err != nil {
			return err
		}
	}
	decoded, err := decodeImage(wanted.path)
	if err != nil {
		return fmt.Errorf("decode modal image: %w", err)
	}
	if err := r.Images.Show(wanted.rect, decoded); err != nil {
		return fmt.Errorf("show modal image: %w", err)
	}
	r.placement = wanted
	return nil
}

func (r *Runner) clearImage() error {
	if r.Images == nil || r.placement.path == "" {
		return nil
	}
	if err := r.Images.Clear(); err != nil {
		return err
	}
	r.placement = imagePlacement{}
	return nil
}

func decodeImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoded, _, err := image.Decode(file)
	return decoded, err
}

func shouldClearPlacement(before app.State, event app.Event) bool {
	if before.Modal == nil || before.Modal.Path == "" {
		return false
	}
	switch event := event.(type) {
	case app.Resized:
		return true
	case app.ActionReceived:
		return event.Action == app.Close || event.Action == app.SelectChat || event.Action == app.Quit
	case app.ShutdownComplete:
		return true
	}
	return false
}

func placementInvalidated(before, after app.State) bool {
	if before.Modal == nil || before.Modal.Path == "" {
		return false
	}
	if after.Modal == nil || after.Modal.Path != before.Modal.Path {
		return true
	}
	return selectedChatID(before) != selectedChatID(after) || before.Width != after.Width || before.Height != after.Height
}

func selectedChatID(state app.State) domain.ChatID {
	if state.SelectedChat < 0 || state.SelectedChat >= len(state.Chats) {
		return 0
	}
	return state.Chats[state.SelectedChat].ID
}

func shutdownResult(event app.Event) (bool, error) {
	complete, ok := event.(app.ShutdownComplete)
	if !ok {
		return false, nil
	}
	if complete.Error != nil {
		return true, *complete.Error
	}
	return true, nil
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
