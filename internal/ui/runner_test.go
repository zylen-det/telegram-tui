package ui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type recordingBackend struct {
	mu      sync.Mutex
	events  chan gotui.Event
	width   int
	height  int
	calls   *[]string
	renders chan ViewModel
	root    *Root
}

func newRecordingBackend(calls *[]string, root *Root) *recordingBackend {
	return &recordingBackend{events: make(chan gotui.Event, 8), width: 100, height: 24, calls: calls, renders: make(chan ViewModel, 16), root: root}
}
func (b *recordingBackend) PollEventsWithContext(context.Context) <-chan gotui.Event { return b.events }
func (b *recordingBackend) TerminalDimensions() (int, int)                           { return b.width, b.height }
func (b *recordingBackend) Clear()                                                   { b.record("clear") }
func (b *recordingBackend) Close()                                                   { b.record("close") }
func (b *recordingBackend) Render(...gotui.Drawable) {
	b.record("render")
	b.renders <- b.root.Model()
}
func (b *recordingBackend) record(call string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	*b.calls = append(*b.calls, call)
}

type recordingImages struct {
	mu    sync.Mutex
	calls *[]string
	shows chan image.Rectangle
}

func (i *recordingImages) Show(rect image.Rectangle, _ image.Image) error {
	i.mu.Lock()
	*i.calls = append(*i.calls, "show")
	i.mu.Unlock()
	i.shows <- rect
	return nil
}
func (i *recordingImages) Clear() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	*i.calls = append(*i.calls, "image-clear")
	return nil
}

func runnerHarness(t *testing.T, state app.State) (*Runner, *recordingBackend, *recordingImages, chan app.Event, chan auth.Prompt, chan os.Signal, chan app.Command, *[]string) {
	t.Helper()
	calls := &[]string{}
	root := NewRoot()
	backend := newRecordingBackend(calls, root)
	images := &recordingImages{calls: calls, shows: make(chan image.Rectangle, 8)}
	events := make(chan app.Event, 8)
	prompts := make(chan auth.Prompt, 8)
	signals := make(chan os.Signal, 8)
	commands := make(chan app.Command, 8)
	runner := &Runner{Backend: backend, Root: root, Engine: app.NewEngine(state), Commands: commands, Events: events, Prompts: prompts, Images: images, Signals: signals, Location: time.UTC, CellPixels: func(int, int) image.Point { return image.Pt(8, 16) }, Now: func() time.Time { return time.Unix(123, 0) }}
	return runner, backend, images, events, prompts, signals, commands, calls
}

func runAsync(t *testing.T, runner *Runner) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	return cancel, done
}

func receive[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func TestRunnerInitialResizePrecedesFirstRenderAndSourcesShareLoop(t *testing.T) {
	runner, backend, _, events, prompts, _, commands, _ := runnerHarness(t, app.InitialState())
	cancel, done := runAsync(t, runner)
	first := receive(t, backend.renders, "initial render")
	if first.Width != 100 || first.Height != 24 {
		t.Fatalf("initial render size = %dx%d", first.Width, first.Height)
	}

	backend.events <- gotui.Event{Type: gotui.KeyboardEvent, ID: "j"}
	second := receive(t, backend.renders, "keyboard render")
	if second.Focus != app.FocusChats {
		t.Fatalf("keyboard snapshot focus = %v", second.Focus)
	}
	backend.events <- gotui.Event{Type: gotui.ResizeEvent, Payload: gotui.Resize{Width: 120, Height: 30}}
	if got := receive(t, backend.renders, "resize render"); got.Width != 120 {
		t.Fatalf("resize width = %d", got.Width)
	}
	runner.Root.mu.Lock()
	runner.Root.hits = HitMap{{Rect: image.Rect(1, 1, 3, 3), Click: app.ActionReceived{Action: app.FocusPane, TargetFocus: app.FocusConversation}}}
	runner.Root.mu.Unlock()
	backend.events <- gotui.Event{Type: gotui.MouseEvent, ID: "<MouseLeft>", Payload: gotui.Mouse{X: 2, Y: 2}}
	if got := receive(t, backend.renders, "mouse render"); got.Focus != app.FocusConversation {
		t.Fatalf("mouse snapshot focus = %v", got.Focus)
	}
	prompts <- auth.Prompt{ID: 9, Label: "Code"}
	if got := receive(t, backend.renders, "prompt render"); got.Prompt == nil || got.Prompt.Prompt.ID != 9 {
		t.Fatalf("prompt snapshot = %#v", got.Prompt)
	}
	events <- app.OperationFailed{Error: domain.AppError{Message: "failed"}}
	if got := receive(t, backend.renders, "app render"); got.Toast == nil {
		t.Fatal("app event did not render toast")
	}

	backend.events <- gotui.Event{Type: gotui.KeyboardEvent, ID: "<C-c>"}
	if _, ok := receive(t, commands, "shutdown command").(app.BeginShutdown); !ok {
		t.Fatal("quit did not emit BeginShutdown")
	}
	events <- app.ShutdownComplete{}
	if err := receive(t, done, "runner completion"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	cancel()
}

func TestRunnerReadyModalShowsAfterFrameAndCachesPlacement(t *testing.T) {
	state := app.InitialState()
	state.Width, state.Height = 100, 24
	state.Focus = app.FocusModal
	state.Modal = &app.ModalState{RequestID: 4, Loading: true}
	runner, backend, images, events, _, _, _, calls := runnerHarness(t, state)
	path := writeTestPNG(t, 40, 20)
	cancel, done := runAsync(t, runner)
	_ = receive(t, backend.renders, "initial render")
	events <- app.AvatarOpened{RequestID: 4, Title: "Avatar", Path: path}
	_ = receive(t, backend.renders, "modal frame")
	model := runner.Root.Model()
	if model.ModalImage.Empty() {
		t.Fatal("modal image rectangle was not published to the view model")
	}
	rect := receive(t, images.shows, "image show")
	if rect.Empty() || !rect.In(image.Rect(0, 0, 100, 24)) {
		t.Fatalf("show rect = %v", rect)
	}
	if indexOf(*calls, "render") > lastIndexOf(*calls, "show") {
		t.Fatalf("calls = %v, want render before show", *calls)
	}
	events <- app.OperationFailed{Error: domain.AppError{Message: "redraw"}}
	_ = receive(t, backend.renders, "ordinary redraw")
	select {
	case <-images.shows:
		t.Fatal("unchanged modal was retransmitted")
	default:
	}
	cancel()
	if err := receive(t, done, "cancelled runner"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerClearsImageBeforeShutdownBackendClose(t *testing.T) {
	state := app.InitialState()
	state.Focus = app.FocusModal
	state.Modal = &app.ModalState{RequestID: 1, Path: writeTestPNG(t, 10, 10)}
	runner, backend, images, events, _, _, _, calls := runnerHarness(t, state)
	cancel, done := runAsync(t, runner)
	defer cancel()
	_ = receive(t, backend.renders, "initial frame")
	_ = receive(t, images.shows, "initial image")
	events <- app.ShutdownComplete{}
	if err := receive(t, done, "shutdown"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if lastIndexOf(*calls, "image-clear") > lastIndexOf(*calls, "close") {
		t.Fatalf("calls = %v", *calls)
	}
}

func TestRunnerSuspendResumeReconcilesFrameAndImage(t *testing.T) {
	state := app.InitialState()
	state.Focus = app.FocusModal
	state.Modal = &app.ModalState{RequestID: 1, Path: writeTestPNG(t, 10, 10)}
	runner, backend, images, _, _, signals, _, calls := runnerHarness(t, state)
	resumed := newRecordingBackend(calls, runner.Root)
	runner.Suspend = func() error { *calls = append(*calls, "suspend"); return nil }
	runner.Resume = func() (Backend, error) { *calls = append(*calls, "resume"); return resumed, nil }
	cancel, done := runAsync(t, runner)
	_ = receive(t, backend.renders, "initial frame")
	_ = receive(t, images.shows, "initial image")
	signals <- syscall.SIGTSTP
	// SIGCONT is intentionally accepted while suspended by the same owner loop.
	signals <- syscall.SIGCONT
	_ = receive(t, resumed.renders, "resumed frame")
	_ = receive(t, images.shows, "resumed image")
	if lastIndexOf(*calls, "clear") > lastIndexOf(*calls, "render") || lastIndexOf(*calls, "render") > lastIndexOf(*calls, "show") {
		t.Fatalf("resume calls = %v", *calls)
	}
	cancel()
	_ = receive(t, done, "runner completion")
}

func TestRunnerResumeFailureStillCleansImageAndBackend(t *testing.T) {
	runner, backend, _, _, _, signals, _, calls := runnerHarness(t, app.InitialState())
	runner.Suspend = func() error { return nil }
	resumeErr := errors.New("resume failed")
	runner.Resume = func() (Backend, error) { return nil, resumeErr }
	_, done := runAsync(t, runner)
	_ = receive(t, backend.renders, "initial render")
	signals <- syscall.SIGTSTP
	signals <- syscall.SIGCONT
	if err := receive(t, done, "resume failure"); !errors.Is(err, resumeErr) {
		t.Fatalf("Run() error = %v", err)
	}
	if lastIndexOf(*calls, "image-clear") < 0 || lastIndexOf(*calls, "close") < 0 {
		t.Fatalf("cleanup calls = %v", *calls)
	}
}

func TestRunnerTerminationSignalsBeginShutdown(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			runner, backend, _, events, _, signals, commands, _ := runnerHarness(t, app.InitialState())
			cancel, done := runAsync(t, runner)
			defer cancel()
			_ = receive(t, backend.renders, "initial render")
			signals <- signal
			if _, ok := receive(t, commands, "shutdown command").(app.BeginShutdown); !ok {
				t.Fatal("signal did not begin shutdown")
			}
			events <- app.ShutdownComplete{}
			if err := receive(t, done, "completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlacementInvalidation(t *testing.T) {
	base := app.InitialState()
	base.Width, base.Height = 100, 24
	base.Chats = []domain.Chat{{ID: 1}, {ID: 2}}
	base.Modal = &app.ModalState{Path: "/tmp/avatar.png"}
	tests := []struct {
		name   string
		mutate func(*app.State)
		want   bool
	}{
		{name: "unchanged", mutate: func(*app.State) {}, want: false},
		{name: "modal close", mutate: func(state *app.State) { state.Modal = nil }, want: true},
		{name: "chat switch", mutate: func(state *app.State) { state.SelectedChat = 1 }, want: true},
		{name: "resize", mutate: func(state *app.State) { state.Width++ }, want: true},
		{name: "replacement path", mutate: func(state *app.State) { state.Modal.Path = "/tmp/other.png" }, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := base
			modal := *base.Modal
			after.Modal = &modal
			test.mutate(&after)
			if got := placementInvalidated(base, after); got != test.want {
				t.Fatalf("placementInvalidated() = %t, want %t", got, test.want)
			}
		})
	}
}

func writeTestPNG(t *testing.T, width, height int) string {
	t.Helper()
	path := t.TempDir() + "/image.png"
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.White)
	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
	return path
}
func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
func lastIndexOf(values []string, want string) int {
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] == want {
			return i
		}
	}
	return -1
}
