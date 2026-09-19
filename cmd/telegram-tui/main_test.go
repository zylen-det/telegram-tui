package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/blacktop/go-termimg"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/frontend"
	"github.com/zylen-det/telegram-tui/internal/platform"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestCopyProductionCompositionInjectsClipboard(t *testing.T) {
	clipboard := &platform.FakeClipboard{Matches: func(value string) bool { return value == "opaque" }}
	handler := newProductionHandler(context.Background(), nil, nil, nil, nil, clipboard, nil, termimg.Halfblocks)
	events := make([]app.Event, 0, 1)
	handler.Handle(context.Background(), app.WriteClipboard{Text: "opaque"}, func(event app.Event) {
		events = append(events, event)
	})
	if clipboard.Writes != 1 || !clipboard.Matched || len(events) != 1 {
		t.Fatalf("production clipboard composition = writes:%d matched:%t events:%d", clipboard.Writes, clipboard.Matched, len(events))
	}
	if _, ok := events[0].(app.ClipboardWritten); !ok {
		t.Fatalf("production clipboard event type = %T", events[0])
	}
}

func TestRunVersionDoesNotInitializeRuntime(t *testing.T) {
	original := startApplication
	called := false
	startApplication = func(context.Context, appOptions) error { called = true; return nil }
	t.Cleanup(func() { startApplication = original })

	for _, argument := range []string{"-v", "--version"} {
		t.Run(argument, func(t *testing.T) {
			called = false
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), []string{argument}, strings.NewReader(""), &stdout, &stderr); code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			if called || strings.TrimSpace(stdout.String()) == "" || stderr.Len() != 0 {
				t.Fatalf("version output/runtime = %q/%v stderr=%q", stdout.String(), called, stderr.String())
			}
		})
	}
}

func TestRunHelpDoesNotInitializeRuntime(t *testing.T) {
	original := startApplication
	called := false
	startApplication = func(context.Context, appOptions) error { called = true; return nil }
	t.Cleanup(func() { startApplication = original })

	for _, argument := range []string{"-h", "--help"} {
		t.Run(argument, func(t *testing.T) {
			called = false
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), []string{argument}, strings.NewReader(""), &stdout, &stderr); code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			if called || stdout.String() != helpText+"\n" || stderr.Len() != 0 {
				t.Fatalf("help output/runtime = %q/%v stderr=%q", stdout.String(), called, stderr.String())
			}
		})
	}
}

func TestRunRejectsUnknownArgumentSafely(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--api-hash=secret"}, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("run() code = %d", code)
	}
	if strings.Contains(stderr.String(), "secret") || !strings.Contains(stderr.String(), "supports only -h") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReportsSanitizedApplicationFailure(t *testing.T) {
	original := startApplication
	startApplication = func(context.Context, appOptions) error { return errors.New("raw secret failure") }
	t.Cleanup(func() { startApplication = original })
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("run() code = %d", code)
	}
	if strings.Contains(stderr.String(), "raw secret") || !strings.Contains(stderr.String(), "telegram-tui could not start") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReportsAlreadyRunningActionablyAndSafely(t *testing.T) {
	original := startApplication
	startApplication = func(context.Context, appOptions) error { return &platform.AlreadyRunningError{} }
	t.Cleanup(func() { startApplication = original })
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("run() code = %d", code)
	}
	const want = "another telegram-tui instance is already running; close it normally before retrying\n"
	if stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("already-running output = stdout %q stderr %q, want stderr %q", stdout.String(), stderr.String(), want)
	}
}

func TestProductionOwnershipStopsBeforeConstructionAndReleasesInstance(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	first, err := platform.AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	constructed := false
	err = withInstanceOwnership(stateDir, func() error {
		constructed = true
		return nil
	})
	if !errors.As(err, new(*platform.AlreadyRunningError)) {
		t.Fatalf("second invocation error = %T, want *platform.AlreadyRunningError", err)
	}
	if constructed {
		t.Fatal("second invocation reached application construction")
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("startup failed")
	err = withInstanceOwnership(stateDir, func() error {
		constructed = true
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("owned invocation error = %v, want %v", err, wantErr)
	}

	third, err := platform.AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatalf("ownership was not released after startup error: %v", err)
	}
	defer third.Close()
}

func TestInstanceOwnershipReleasesAfterSuccessfulApplication(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := withInstanceOwnership(stateDir, func() error { return nil }); err != nil {
		t.Fatalf("owned invocation error = %v", err)
	}
	lock, err := platform.AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatalf("ownership was not released after success: %v", err)
	}
	defer lock.Close()
}

func TestEnsureRuntimeDirectoriesCreatesPrivatePaths(t *testing.T) {
	root := t.TempDir()
	paths := config.ResolvePaths(func(key string) string {
		return filepath.Join(root, strings.ToLower(key))
	}, root)
	if err := ensureRuntimeDirectories(paths); err != nil {
		t.Fatalf("ensureRuntimeDirectories() error = %v", err)
	}
	for _, path := range []string{paths.StateDir, paths.DataDir, paths.TDLibDatabase, paths.TDLibFiles, paths.AvatarCacheDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %q info=%#v err=%v", path, info, err)
		}
	}
}

func TestProductionCleanupRunsForProgramFailure(t *testing.T) {
	calls := []string{}
	ctx, baseCancel := context.WithCancel(context.Background())
	cancel := func() {
		calls = append(calls, "cancel")
		baseCancel()
	}
	runtime := &productionRuntimeStub{ctx: ctx, calls: &calls}
	overlay := &productionOverlayStub{calls: &calls}
	wantErr := errors.New("program failed")
	program := &productionProgramStub{calls: &calls, err: wantErr}

	err := runProductionBubbleTea(context.Background(), cancel, runtime, overlay, program, productionStatusStub{}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runProductionBubbleTea() error = %v, want %v", err, wantErr)
	}
	wantCalls := []string{"program-run", "cancel", "runtime-close", "runtime-wait", "image-clear"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("cleanup calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestProductionCleanupTreatsParentCancellationAsControlled(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	parentCancel()
	ctx, cancel := context.WithCancel(parent)
	calls := []string{}
	err := runProductionBubbleTea(parent, cancel,
		&productionRuntimeStub{ctx: ctx, calls: &calls},
		&productionOverlayStub{calls: &calls},
		&productionProgramStub{calls: &calls, err: tea.ErrProgramKilled},
		productionStatusStub{},
		nil,
	)
	if err != nil {
		t.Fatalf("parent-cancelled run error = %v, want nil", err)
	}

	activeParent := context.Background()
	activeContext, activeCancel := context.WithCancel(activeParent)
	err = runProductionBubbleTea(activeParent, activeCancel,
		&productionRuntimeStub{ctx: activeContext, calls: &calls},
		&productionOverlayStub{calls: &calls},
		&productionProgramStub{calls: &calls, err: tea.ErrProgramKilled},
		productionStatusStub{},
		nil,
	)
	if !errors.Is(err, tea.ErrProgramKilled) {
		t.Fatalf("uncontrolled killed run error = %v, want ErrProgramKilled", err)
	}
}

func TestProductionSignalGracefulSIGINT(t *testing.T) {
	testProductionSignalGraceful(t, syscall.SIGINT)
}

func TestProductionSignalGracefulSIGTERM(t *testing.T) {
	testProductionSignalGraceful(t, syscall.SIGTERM)
}

func TestProductionSignalRealAppLifecycleSIGINT(t *testing.T) {
	testProductionSignalRealAppLifecycle(t, syscall.SIGINT)
}

func TestProductionSignalRealAppLifecycleSIGTERM(t *testing.T) {
	testProductionSignalRealAppLifecycle(t, syscall.SIGTERM)
}

func testProductionSignalRealAppLifecycle(t *testing.T, received os.Signal) {
	t.Helper()
	processCtx, cancel := context.WithCancel(context.Background())
	broker := auth.NewBroker()
	resolved := make(chan struct{}, 1)
	resolver := &productionSignalResolver{broker: broker, resolved: resolved}
	client := &productionSignalClient{
		Fake:   telegram.NewFake(telegram.FakeData{}),
		closed: make(chan struct{}),
	}
	clientCreated := make(chan struct{}, 1)
	observed := &productionSignalHandler{
		promptRequested:  make(chan struct{}, 1),
		shutdownComplete: make(chan struct{}, 1),
	}
	observed.Handler = app.NewHandler(processCtx, resolver, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		clientCreated <- struct{}{}
		return client, nil
	}, broker, nil)
	runtime, err := frontend.NewAppRuntime(processCtx, app.NewExecutor(2, observed))
	if err != nil {
		t.Fatal("NewAppRuntime returned an unexpected error")
	}
	engine := app.NewEngine(app.InitialState())
	model, err := frontend.NewAppModel(engine, runtime)
	if err != nil {
		t.Fatal("NewAppModel returned an unexpected error")
	}
	rendered := &productionSynchronizedOutput{}
	output := frontend.NewOutputOverlay(rendered, nil, nil)
	model.SetOutputOverlay(output)
	program := tea.NewProgram(
		model,
		tea.WithContext(processCtx),
		tea.WithInput(nil),
		tea.WithOutput(output),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithWindowSize(80, 24),
		tea.WithoutSignalHandler(),
	)
	programReturned := make(chan productionRealProgramResult, 1)
	wrappedProgram := &productionRealProgram{
		Program:  program,
		runtime:  runtime,
		returned: programReturned,
	}
	overlayCleared := make(chan struct{}, 1)
	wrappedOverlay := &productionRealOverlay{OutputOverlay: output, cleared: overlayCleared}
	signals := make(chan os.Signal, 1)
	topReturned := make(chan error, 1)
	go func() {
		topReturned <- runProductionBubbleTea(processCtx, cancel, runtime, wrappedOverlay, wrappedProgram, nil, signals)
	}()

	cleanupComplete := false
	t.Cleanup(func() {
		if cleanupComplete {
			return
		}
		cancel()
		program.Kill()
		runtime.Close()
		select {
		case <-topReturned:
		case <-time.After(2 * time.Second):
			t.Error("real production lifecycle cleanup did not return")
		}
		waited := make(chan struct{})
		go func() { runtime.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
			t.Error("real production runtime cleanup did not stop")
		}
	})

	waitProductionSignal(t, observed.promptRequested, "PromptRequested")
	waitProductionOutput(t, rendered, "Account password")
	program.Send(tea.KeyPressMsg(tea.Key{Text: "ready"}))
	program.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	waitProductionSignal(t, resolved, "resolver response")
	waitProductionSignal(t, clientCreated, "fake client creation")

	signals <- received
	waitProductionSignal(t, client.closed, "fake client Close")
	waitProductionSignal(t, observed.shutdownComplete, "ShutdownComplete")

	programResult := waitProductionValue(t, programReturned, "Program.Run return")
	if !programResult.runtimeDone {
		t.Fatal("Program.Run returned before AppRuntime.Done closed")
	}
	if programResult.err != nil {
		t.Fatal("Program.Run returned an unexpected error")
	}
	if _, ok := programResult.model.(frontend.AppModel); !ok {
		t.Fatalf("Program.Run() model = %T, want frontend.AppModel", programResult.model)
	}
	if !engine.Snapshot().Quitting {
		t.Fatal("production signal did not apply app.Quit through the formal reducer")
	}
	topErr := waitProductionValue(t, topReturned, "runProductionBubbleTea return")
	cleanupComplete = true
	if topErr != nil {
		t.Fatalf("runProductionBubbleTea returned an unexpected error: %v", topErr)
	}
	waitProductionSignal(t, overlayCleared, "overlay clear")
	select {
	case <-runtime.Done():
	default:
		t.Fatal("AppRuntime.Done was open after production returned")
	}
}

func testProductionSignalGraceful(t *testing.T, signal os.Signal) {
	t.Helper()
	recorder := &productionLifecycleRecorder{}
	runtimeDone := make(chan struct{})
	model := &productionLifecycleModel{recorder: recorder, runtimeDone: runtimeDone}
	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(&bytes.Buffer{}),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)
	wrapped := &recordingProductionProgram{Program: program, recorder: recorder}
	signals := make(chan os.Signal, 1)
	signals <- signal
	ctx, cancel := context.WithCancel(context.Background())

	err := runProductionBubbleTea(context.Background(), cancel,
		&productionLifecycleRuntime{done: runtimeDone, recorder: recorder},
		&productionLifecycleOverlay{recorder: recorder},
		wrapped,
		model,
		signals,
	)
	if err != nil {
		t.Fatal("graceful signal returned an error")
	}
	if ctx.Err() == nil {
		t.Fatal("production context was not cancelled")
	}
	want := []string{
		"process-quit", "client-close", "shutdown-complete", "runtime-done",
		"program-return", "runtime-close", "runtime-wait", "image-clear",
	}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("production lifecycle calls = %#v", got)
	}
}

func TestProductionFatalShutdownReturnsSanitizedFailure(t *testing.T) {
	original := startApplication
	t.Cleanup(func() { startApplication = original })
	startApplication = func(parent context.Context, _ appOptions) error {
		ctx, cancel := context.WithCancel(parent)
		calls := []string{}
		return runProductionBubbleTea(parent, cancel,
			&productionRuntimeStub{ctx: ctx, calls: &calls},
			&productionOverlayStub{calls: &calls},
			&productionProgramStub{calls: &calls},
			productionStatusStub{err: errors.New("private shutdown payload")},
			nil,
		)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("fatal shutdown exit code = %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "could not start or close safely") {
		t.Fatal("fatal shutdown did not use the sanitized top-level failure")
	}
	if strings.Contains(stderr.String(), "private shutdown payload") {
		t.Fatal("fatal shutdown payload reached user output")
	}
}

type productionProgramStub struct {
	calls *[]string
	err   error
}

func (p *productionProgramStub) Run() (tea.Model, error) {
	*p.calls = append(*p.calls, "program-run")
	return nil, p.err
}

func (p *productionProgramStub) Send(tea.Msg) {}

type productionRuntimeStub struct {
	ctx   context.Context
	calls *[]string
}

func (r *productionRuntimeStub) Close() {
	if r.ctx.Err() == nil {
		panic("runtime closed before shared context cancellation")
	}
	*r.calls = append(*r.calls, "runtime-close")
}

func (r *productionRuntimeStub) Wait() { *r.calls = append(*r.calls, "runtime-wait") }

type productionOverlayStub struct{ calls *[]string }

func (o *productionOverlayStub) Clear() error {
	*o.calls = append(*o.calls, "image-clear")
	return nil
}

type productionStatusStub struct{ err error }

func (s productionStatusStub) ShutdownError() error { return s.err }

type productionLifecycleRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *productionLifecycleRecorder) add(call string) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *productionLifecycleRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type productionLifecycleCompleteMsg struct{}

type productionLifecycleModel struct {
	recorder    *productionLifecycleRecorder
	runtimeDone chan struct{}
}

func (m *productionLifecycleModel) Init() tea.Cmd { return nil }

func (m *productionLifecycleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case frontend.ProcessQuitMsg:
		m.recorder.add("process-quit")
		return m, func() tea.Msg {
			m.recorder.add("client-close")
			return productionLifecycleCompleteMsg{}
		}
	case productionLifecycleCompleteMsg:
		m.recorder.add("shutdown-complete")
		close(m.runtimeDone)
		m.recorder.add("runtime-done")
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *productionLifecycleModel) View() tea.View { return tea.NewView("") }

func (m *productionLifecycleModel) ShutdownError() error { return nil }

type recordingProductionProgram struct {
	*tea.Program
	recorder *productionLifecycleRecorder
}

func (p *recordingProductionProgram) Run() (tea.Model, error) {
	model, err := p.Program.Run()
	p.recorder.add("program-return")
	return model, err
}

type productionLifecycleRuntime struct {
	done     <-chan struct{}
	recorder *productionLifecycleRecorder
}

func (r *productionLifecycleRuntime) Close() {
	select {
	case <-r.done:
	default:
		panic("runtime closed before ShutdownComplete")
	}
	r.recorder.add("runtime-close")
}

func (r *productionLifecycleRuntime) Wait() { r.recorder.add("runtime-wait") }

type productionLifecycleOverlay struct{ recorder *productionLifecycleRecorder }

func (o *productionLifecycleOverlay) Clear() error {
	o.recorder.add("image-clear")
	return nil
}

type productionSignalResolver struct {
	broker   *auth.Broker
	resolved chan<- struct{}
}

func (r *productionSignalResolver) Resolve(ctx context.Context) (config.Runtime, error) {
	_, err := r.broker.Ask(ctx, auth.Prompt{
		Kind:   auth.PromptPassword,
		Label:  "Account password",
		Secret: true,
	})
	if err != nil {
		return config.Runtime{}, err
	}
	r.resolved <- struct{}{}
	return config.Runtime{}, nil
}

type productionSignalClient struct {
	*telegram.Fake
	closed chan struct{}
	once   sync.Once
}

func (c *productionSignalClient) Close(ctx context.Context) error {
	c.once.Do(func() { close(c.closed) })
	return c.Fake.Close(ctx)
}

type productionSignalHandler struct {
	*app.Handler
	promptRequested  chan struct{}
	shutdownComplete chan struct{}
}

func (h *productionSignalHandler) Handle(ctx context.Context, command app.Command, emit func(app.Event)) {
	h.Handler.Handle(ctx, command, func(event app.Event) {
		switch event.(type) {
		case app.PromptRequested:
			signalProductionObservation(h.promptRequested)
		case app.ShutdownComplete:
			signalProductionObservation(h.shutdownComplete)
		}
		emit(event)
	})
}

type productionRealProgramResult struct {
	model       tea.Model
	err         error
	runtimeDone bool
}

type productionRealProgram struct {
	*tea.Program
	runtime  *frontend.AppRuntime
	returned chan<- productionRealProgramResult
}

func (p *productionRealProgram) Run() (tea.Model, error) {
	model, err := p.Program.Run()
	runtimeDone := false
	select {
	case <-p.runtime.Done():
		runtimeDone = true
	default:
	}
	p.returned <- productionRealProgramResult{model: model, err: err, runtimeDone: runtimeDone}
	return model, err
}

type productionRealOverlay struct {
	*frontend.OutputOverlay
	cleared chan<- struct{}
	once    sync.Once
}

func (o *productionRealOverlay) Clear() error {
	err := o.OutputOverlay.Clear()
	o.once.Do(func() { o.cleared <- struct{}{} })
	return err
}

func signalProductionObservation(signal chan<- struct{}) {
	select {
	case signal <- struct{}{}:
	default:
	}
}

func waitProductionSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitProductionValue[T any](t *testing.T, values <-chan T, name string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		var zero T
		return zero
	}
}

type productionSynchronizedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (w *productionSynchronizedOutput) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(value)
}

func (w *productionSynchronizedOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func waitProductionOutput(t *testing.T, output *productionSynchronizedOutput, wanted string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(output.String(), wanted) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for rendered %q", wanted)
		}
	}
}
