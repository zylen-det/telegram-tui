package frontend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestAuthorizationVerticalUsesRealAppRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := auth.NewBroker()
	resolved := make(chan string, 1)
	resolver := &authorizationTestResolver{broker: broker, resolved: resolved}
	client := &closingFrontendClient{
		Fake:   telegram.NewFake(telegram.FakeData{}),
		closed: make(chan struct{}),
	}
	handler := app.NewHandler(ctx, resolver, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, broker, nil)
	runtime := newAppRuntimeForTest(t, ctx, app.NewExecutor(2, handler))
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, runtime)
	messages := make(chan tea.Msg, 32)

	model, resizeCmd := updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	if resizeCmd != nil {
		t.Fatal("resize unexpectedly returned a command")
	}
	runFrontendCmd(model.Init(), messages)

	model = driveAppModelUntil(t, model, messages, func(msg tea.Msg, state app.State) bool {
		_, promptEvent := appEventFromMsg(msg).(app.PromptRequested)
		return promptEvent && state.Prompt != nil
	})
	if got, want := engine.Snapshot().Prompt.Prompt.Label, "Account password"; got != want {
		t.Fatalf("prompt label = %q, want %q", got, want)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "s界"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "🙂cret"})
	if got, want := string(engine.Snapshot().Prompt.Input), "s界🙂cret"; got != want {
		t.Fatal("prompt input did not preserve typed and pasted runes")
	}
	plain := ansiSequence.ReplaceAllString(model.View().Content, "")
	if containsAny(plain, "s界🙂cret", "界", "🙂") {
		t.Fatal("secret value appeared in rendered output")
	}

	model, submitCmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if submitCmd == nil {
		t.Fatal("Enter did not return asynchronous prompt command delivery")
	}
	if engine.Snapshot().Prompt != nil {
		t.Fatal("Enter did not clear the engine prompt")
	}
	runFrontendCmd(submitCmd, messages)
	select {
	case got := <-resolved:
		if want := "s界🙂cret"; got != want {
			t.Fatal("resolver received an unexpected authorization value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolver did not receive broker response")
	}

	model = driveAppModelUntil(t, model, messages, func(msg tea.Msg, _ app.State) bool {
		_, ready := appEventFromMsg(msg).(app.TelegramEvent)
		return ready
	})
	assertGracefulQuit(t, model, engine, runtime, client, messages)
}

func TestGracefulQuitWaitsForShutdownComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := auth.NewBroker()
	resolver := &immediateTestResolver{}
	client := &closingFrontendClient{
		Fake:   telegram.NewFake(telegram.FakeData{}),
		closed: make(chan struct{}),
	}
	handler := app.NewHandler(ctx, resolver, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, broker, nil)
	runtime := newAppRuntimeForTest(t, ctx, app.NewExecutor(2, handler))
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, runtime)
	messages := make(chan tea.Msg, 32)
	runFrontendCmd(model.Init(), messages)
	model = driveAppModelUntil(t, model, messages, func(msg tea.Msg, _ app.State) bool {
		_, ready := appEventFromMsg(msg).(app.TelegramEvent)
		return ready
	})

	assertGracefulQuit(t, model, engine, runtime, client, messages)
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := newAppRuntimeForTest(t, ctx, app.NewExecutor(1, nil))

	var closes sync.WaitGroup
	for range 8 {
		closes.Add(1)
		go func() {
			defer closes.Done()
			runtime.Close()
		}()
	}
	closes.Wait()
	runtime.Close()

	waited := make(chan struct{})
	go func() {
		runtime.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("AppRuntime.Wait did not return after Close")
	}
	select {
	case <-runtime.Done():
	default:
		t.Fatal("AppRuntime.Done was not closed after Wait returned")
	}
}

func TestNilDependenciesRejectedSynchronously(t *testing.T) {
	runtime, err := NewAppRuntime(context.Background(), nil)
	if err == nil {
		t.Fatal("NewAppRuntime accepted a nil executor")
	}
	if runtime != nil {
		t.Fatal("NewAppRuntime returned a runtime for a nil executor")
	}

	if _, err := NewAppModel(nil, nil); err == nil {
		t.Fatal("NewAppModel accepted a nil engine")
	}
}

func TestProgramRunWaitsForRuntimeDone(t *testing.T) {
	processCtx, cancelProcess := context.WithCancel(context.Background())
	runtimeCtx, cancelRuntime := context.WithCancel(processCtx)
	allowRuntimeStop := make(chan struct{})
	var allowRuntimeStopOnce sync.Once
	runtime := &AppRuntime{
		ctx:      runtimeCtx,
		cancel:   cancelRuntime,
		commands: make(chan app.Command, appRuntimeBuffer),
		events:   make(chan app.Event, appRuntimeBuffer),
		done:     make(chan struct{}),
	}
	go func() {
		<-runtimeCtx.Done()
		<-allowRuntimeStop
		close(runtime.done)
	}()

	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), runtime)
	program := tea.NewProgram(
		shutdownCompleteOnInitModel{AppModel: model},
		tea.WithContext(processCtx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
	)
	type programResult struct {
		model tea.Model
		err   error
	}
	result := make(chan programResult, 1)
	resultReceived := false
	defer func() {
		cancelProcess()
		runtime.Close()
		allowRuntimeStopOnce.Do(func() { close(allowRuntimeStop) })
		runtime.Wait()
		if resultReceived {
			return
		}
		program.Kill()
		select {
		case <-result:
		case <-time.After(2 * time.Second):
			t.Errorf("real Bubble Tea shutdown program cleanup did not return")
		}
	}()
	go func() {
		finalModel, err := program.Run()
		result <- programResult{model: finalModel, err: err}
	}()

	select {
	case <-runtimeCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("real Bubble Tea program did not request runtime shutdown")
	}
	select {
	case <-result:
		resultReceived = true
		t.Fatal("Program.Run returned before AppRuntime.Done closed")
	case <-time.After(50 * time.Millisecond):
	}
	allowRuntimeStopOnce.Do(func() { close(allowRuntimeStop) })

	select {
	case runResult := <-result:
		resultReceived = true
		if runResult.err != nil {
			t.Fatal("Program.Run returned an unexpected error")
		}
		if _, ok := runResult.model.(AppModel); !ok {
			t.Fatalf("Program.Run() model = %T, want frontend.AppModel", runResult.model)
		}
		select {
		case <-runtime.Done():
		default:
			t.Fatal("AppRuntime.Done was open when Program.Run returned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Program.Run did not return after AppRuntime.Done closed")
	}
}

func TestPrivacySafeDiagnostics(t *testing.T) {
	for _, filename := range []string{"app_model_test.go", "app_runtime_test.go"} {
		source, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal("could not read an owned frontend test source")
		}
		for _, forbidden := range []string{
			"broker response = %" + "q",
			"%#" + "v",
			"error = %" + "v",
			"source: %" + "v",
			"error: %" + "v",
			"\\n%s\"" + ", plain",
			"\\n%s\"" + ", rendered",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatal("privacy-unsafe test diagnostic remains in owned frontend tests")
			}
		}
	}
}

func TestRealProgramCancellationClosesRuntime(t *testing.T) {
	processCtx, cancelProcess := context.WithTimeout(context.Background(), 5*time.Second)

	started := make(chan struct{})
	sourceStopped := make(chan struct{})
	resolver := &cancellationTestResolver{started: started, stopped: sourceStopped}
	broker := auth.NewBroker()
	handler := app.NewHandler(processCtx, resolver, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return telegram.NewFake(telegram.FakeData{}), nil
	}, broker, nil)
	runtime := newAppRuntimeForTest(t, processCtx, app.NewExecutor(1, handler))
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), runtime)
	program := tea.NewProgram(
		model,
		tea.WithContext(processCtx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
	)

	result := make(chan error, 1)
	resultReceived := false
	defer func() {
		cancelProcess()
		runtime.Close()
		if !resultReceived {
			program.Kill()
			select {
			case <-result:
			case <-time.After(2 * time.Second):
				t.Errorf("real Bubble Tea cancellation program cleanup did not return")
			}
		}
		runtime.Wait()
	}()
	go func() {
		_, err := program.Run()
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("real Bubble Tea program did not start the app runtime")
	}
	cancelProcess()

	select {
	case err := <-result:
		resultReceived = true
		if !errors.Is(err, tea.ErrProgramKilled) {
			t.Fatal("Program.Run did not return the expected killed classification")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("real Bubble Tea program did not return after cancellation")
	}
	select {
	case <-runtime.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("external cancellation did not stop the app runtime pump")
	}
	select {
	case <-sourceStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("external cancellation did not stop the shared-context Handler source")
	}
}

func TestRealProgramGracefulAuthorizationAndQuit(t *testing.T) {
	processCtx, cancelProcess := context.WithTimeout(context.Background(), 5*time.Second)

	broker := auth.NewBroker()
	resolved := make(chan string, 1)
	resolver := &authorizationTestResolver{broker: broker, resolved: resolved}
	client := &closingFrontendClient{
		Fake:   telegram.NewFake(telegram.FakeData{}),
		closed: make(chan struct{}),
	}
	handler := app.NewHandler(processCtx, resolver, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, broker, nil)
	runtime := newAppRuntimeForTest(t, processCtx, app.NewExecutor(2, handler))
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, runtime)
	observed := &programLifecycleObservations{
		promptRequested:  make(chan struct{}, 1),
		telegramReady:    make(chan struct{}, 1),
		shutdownComplete: make(chan struct{}, 1),
		states:           make(chan programStateObservation, 32),
	}
	output := &synchronizedTestOutput{}
	program := tea.NewProgram(
		observedAppModel{model: model, observed: observed},
		tea.WithContext(processCtx),
		tea.WithInput(nil),
		tea.WithOutput(output),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithWindowSize(80, 24),
		tea.WithFPS(120),
		tea.WithoutSignalHandler(),
	)

	type programResult struct {
		model tea.Model
		err   error
	}
	result := make(chan programResult, 1)
	resultReceived := false
	defer func() {
		cancelProcess()
		runtime.Close()
		if !resultReceived {
			program.Kill()
			select {
			case <-result:
			case <-time.After(2 * time.Second):
				t.Errorf("real Bubble Tea program cleanup did not return")
			}
		}
		runtime.Wait()
	}()
	go func() {
		finalModel, err := program.Run()
		result <- programResult{model: finalModel, err: err}
	}()

	waitForFrontendSignal(t, observed.promptRequested, "PromptRequested")
	waitForFrontendCondition(t, "authorization prompt render", func() bool {
		return strings.Contains(output.String(), "Account password")
	})

	program.Send(tea.WindowSizeMsg{Width: 92, Height: 28})
	program.Send(tea.KeyPressMsg(tea.Key{Text: "s界"}))
	program.Send(tea.PasteMsg{Content: "🙂cret"})
	waitForFrontendState(t, observed.states, "resize and authorization input", func(state programStateObservation) bool {
		return state.width == 92 && state.height == 28 && state.promptInput == "s界🙂cret"
	})

	program.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	select {
	case got := <-resolved:
		if want := "s界🙂cret"; got != want {
			t.Fatal("broker response did not match submitted authorization value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("broker did not receive the authorization response")
	}
	waitForFrontendSignal(t, observed.telegramReady, "TelegramEvent")

	program.Send(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	waitForFrontendState(t, observed.states, "app.Quit application", func(state programStateObservation) bool {
		return state.quitting
	})
	select {
	case <-client.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("BeginShutdown did not close the Telegram client")
	}
	waitForFrontendSignal(t, observed.shutdownComplete, "ShutdownComplete")

	select {
	case runResult := <-result:
		resultReceived = true
		if runResult.err != nil {
			t.Fatal("Program.Run returned an unexpected error")
		}
		if _, ok := runResult.model.(observedAppModel); !ok {
			t.Fatalf("Program.Run() model = %T, want frontend.observedAppModel", runResult.model)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Program.Run did not return after graceful shutdown")
	}
	select {
	case <-runtime.Done():
	default:
		t.Fatal("AppRuntime.Done was open when Program.Run returned")
	}
	if rendered := output.String(); containsAny(rendered, "s界🙂cret", "界", "🙂") {
		t.Fatal("secret authorization response appeared in program output")
	}
}

func newAppRuntimeForTest(t *testing.T, ctx context.Context, executor *app.Executor) *AppRuntime {
	t.Helper()
	runtime, err := NewAppRuntime(ctx, executor)
	if err != nil {
		t.Fatal("NewAppRuntime returned an unexpected error")
	}
	return runtime
}

func assertGracefulQuit(t *testing.T, model AppModel, engine *app.Engine, runtime *AppRuntime, client *closingFrontendClient, messages chan tea.Msg) {
	t.Helper()
	model, command := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if command == nil {
		t.Fatal("Ctrl-C did not return asynchronous shutdown command delivery")
	}
	if !engine.Snapshot().Quitting {
		t.Fatal("Ctrl-C did not apply app.Quit")
	}
	result := command()
	if _, quit := result.(tea.QuitMsg); quit {
		t.Fatal("Ctrl-C returned tea.Quit before app.ShutdownComplete")
	}

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case msg := <-messages:
			event, isAppEvent := msg.(appEventMsg)
			next, cmd := model.Update(msg)
			model = next.(AppModel)
			if isAppEvent {
				if _, complete := event.event.(app.ShutdownComplete); complete {
					if cmd == nil {
						t.Fatal("ShutdownComplete did not return tea.Quit")
					}
					if quitMsg := cmd(); quitMsg == nil {
						t.Fatal("shutdown quit command returned nil")
					} else if _, ok := quitMsg.(tea.QuitMsg); !ok {
						t.Fatalf("shutdown command returned %T, want tea.QuitMsg", quitMsg)
					}
					select {
					case <-client.closed:
					case <-time.After(2 * time.Second):
						t.Fatal("Telegram client was not closed")
					}
					select {
					case <-runtime.done:
					case <-time.After(2 * time.Second):
						t.Fatal("app runtime did not stop")
					}
					return
				}
			}
			runFrontendCmd(cmd, messages)
		case <-deadline.C:
			t.Fatalf("timed out waiting for graceful quit; %s", frontendStateDiagnostic(engine.Snapshot()))
		}
	}
}

func driveAppModelUntil(t *testing.T, model AppModel, messages chan tea.Msg, done func(tea.Msg, app.State) bool) AppModel {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case msg := <-messages:
			next, cmd := model.Update(msg)
			model = next.(AppModel)
			runFrontendCmd(cmd, messages)
			if done(msg, model.engine.Snapshot()) {
				return model
			}
		case <-deadline.C:
			t.Fatalf("timed out driving app model; %s", frontendStateDiagnostic(model.engine.Snapshot()))
		}
	}
}

func frontendStateDiagnostic(state app.State) string {
	promptPresent := state.Prompt != nil
	promptInputLength := 0
	if promptPresent {
		promptInputLength = len(state.Prompt.Input)
	}
	return fmt.Sprintf(
		"width=%d height=%d focus=%v quitting=%t prompt_present=%t prompt_input_length=%d",
		state.Width,
		state.Height,
		state.Focus,
		state.Quitting,
		promptPresent,
		promptInputLength,
	)
}

func runFrontendCmd(cmd tea.Cmd, messages chan tea.Msg) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, batched := range batch {
				runFrontendCmd(batched, messages)
			}
			return
		}
		if msg != nil {
			messages <- msg
		}
	}()
}

func appEventFromMsg(msg tea.Msg) app.Event {
	event, ok := msg.(appEventMsg)
	if !ok {
		return nil
	}
	return event.event
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func waitForFrontendSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForFrontendCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

func waitForFrontendState(t *testing.T, states <-chan programStateObservation, description string, condition func(programStateObservation) bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case state := <-states:
			if condition(state) {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

type programLifecycleObservations struct {
	promptRequested  chan struct{}
	telegramReady    chan struct{}
	shutdownComplete chan struct{}
	states           chan programStateObservation
}

type programStateObservation struct {
	width       int
	height      int
	promptInput string
	quitting    bool
}

type observedAppModel struct {
	model    AppModel
	observed *programLifecycleObservations
}

type shutdownCompleteOnInitModel struct {
	AppModel
}

func (m shutdownCompleteOnInitModel) Init() tea.Cmd {
	return func() tea.Msg {
		return appEventMsg{event: app.ShutdownComplete{}}
	}
}

func (m observedAppModel) Init() tea.Cmd {
	return m.model.Init()
}

func (m observedAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.model.Update(msg)
	updated := next.(AppModel)
	state := updated.engine.Snapshot()
	promptInput := ""
	if state.Prompt != nil {
		promptInput = string(state.Prompt.Input)
	}
	observation := programStateObservation{
		width:       state.Width,
		height:      state.Height,
		promptInput: promptInput,
		quitting:    state.Quitting,
	}
	select {
	case m.observed.states <- observation:
	default:
	}
	if event, ok := msg.(appEventMsg); ok {
		switch event.event.(type) {
		case app.PromptRequested:
			signalFrontendObservation(m.observed.promptRequested)
		case app.TelegramEvent:
			signalFrontendObservation(m.observed.telegramReady)
		case app.ShutdownComplete:
			signalFrontendObservation(m.observed.shutdownComplete)
		}
	}
	return observedAppModel{model: updated, observed: m.observed}, cmd
}

func (m observedAppModel) View() tea.View {
	return m.model.View()
}

func signalFrontendObservation(signal chan<- struct{}) {
	select {
	case signal <- struct{}{}:
	default:
	}
}

type synchronizedTestOutput struct {
	mu      sync.Mutex
	builder strings.Builder
}

func (w *synchronizedTestOutput) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.builder.Write(value)
}

func (w *synchronizedTestOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.builder.String()
}

type authorizationTestResolver struct {
	broker   *auth.Broker
	resolved chan<- string
}

func (r *authorizationTestResolver) Resolve(ctx context.Context) (config.Runtime, error) {
	value, err := r.broker.Ask(ctx, auth.Prompt{
		Kind:   auth.PromptPassword,
		Label:  "Account password",
		Secret: true,
	})
	if err != nil {
		return config.Runtime{}, err
	}
	r.resolved <- value
	return config.Runtime{}, nil
}

type immediateTestResolver struct{}

func (*immediateTestResolver) Resolve(context.Context) (config.Runtime, error) {
	return config.Runtime{}, nil
}

type cancellationTestResolver struct {
	started     chan struct{}
	stopped     chan struct{}
	startedOnce sync.Once
	stoppedOnce sync.Once
}

func (r *cancellationTestResolver) Resolve(ctx context.Context) (config.Runtime, error) {
	r.startedOnce.Do(func() { close(r.started) })
	<-ctx.Done()
	r.stoppedOnce.Do(func() { close(r.stopped) })
	return config.Runtime{}, ctx.Err()
}

type closingFrontendClient struct {
	*telegram.Fake
	closed chan struct{}
	once   sync.Once
}

func (c *closingFrontendClient) Close(ctx context.Context) error {
	c.once.Do(func() { close(c.closed) })
	return c.Fake.Close(ctx)
}
