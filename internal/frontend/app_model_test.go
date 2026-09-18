package frontend

import (
	"context"
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAppModelMapsInputIntoEngineState(t *testing.T) {
	state := app.InitialState()
	state.Prompt = &app.PromptState{Prompt: auth.Prompt{
		ID:     17,
		Kind:   auth.PromptPassword,
		Label:  "Telegram password",
		Secret: true,
	}}
	state.Focus = app.FocusAuth
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a界"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "🙂x"})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))

	if got, want := engine.Snapshot().Width, 100; got != want {
		t.Fatalf("engine width = %d, want %d", got, want)
	}
	if got, want := string(engine.Snapshot().Prompt.Input), "a界🙂"; got != want {
		t.Fatal("prompt input did not preserve key and paste runes")
	}

	plain := ansiSequence.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(plain, "Telegram password") {
		t.Fatal("authorization view missing prompt label")
	}
	if !strings.Contains(plain, "two-step verification password") {
		t.Fatal("authorization view missing prompt-kind guidance")
	}
	if strings.Contains(plain, "a界🙂") || strings.Contains(plain, "界") || strings.Contains(plain, "🙂") {
		t.Fatal("secret authorization view exposed input")
	}
	inputRect := authorizationInputRect(image.Rect(0, 0, 100, 24))
	inputY := inputRect.Min.Y
	lines := strings.Split(plain, "\n")
	if inputY < 0 || inputY >= len(lines) || strings.TrimSpace(ansi.Cut(lines[inputY], inputRect.Min.X, inputRect.Max.X)) == "" {
		t.Fatal("secret authorization view did not render a non-empty Huh password mask")
	}
}

func TestCommandOrderPreserved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := &AppRuntime{
		ctx:      ctx,
		cancel:   cancel,
		commands: make(chan app.Command, appRuntimeBuffer),
	}
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), runtime)
	deliver := model.deliver([]app.Command{
		app.LoadBootstrap{},
		app.LoadChats{RequestID: 42},
	})
	if deliver == nil {
		t.Fatal("ordered command delivery returned nil")
	}
	if msg := deliver(); msg != nil {
		t.Fatalf("ordered command delivery returned %T, want nil", msg)
	}

	first := <-runtime.commands
	if _, ok := first.(app.LoadBootstrap); !ok {
		t.Fatalf("first delivered command = %T, want app.LoadBootstrap", first)
	}
	second := <-runtime.commands
	loadChats, ok := second.(app.LoadChats)
	if !ok {
		t.Fatalf("second delivered command = %T, want app.LoadChats", second)
	}
	if loadChats.RequestID != 42 {
		t.Fatalf("second delivered request ID = %d, want 42", loadChats.RequestID)
	}
}

func TestRuntimeStoppedQuitsProgram(t *testing.T) {
	engine := app.NewEngine(app.InitialState())
	before := engine.Snapshot()
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	next, cmd := model.Update(appRuntimeStoppedMsg{})
	updated, ok := next.(AppModel)
	if !ok {
		t.Fatalf("Update returned %T, want frontend.AppModel", next)
	}
	if !reflect.DeepEqual(engine.Snapshot(), before) {
		t.Fatal("runtime stop mutated engine state")
	}
	if cmd == nil {
		t.Fatal("runtime stop did not return tea.Quit")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("runtime stop quit command returned nil")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("runtime stop command returned %T, want tea.QuitMsg", msg)
	}
	_ = updated.View()
}

func TestNilRuntimeRejected(t *testing.T) {
	if _, err := NewAppModel(app.NewEngine(app.InitialState()), nil); err == nil {
		t.Fatal("NewAppModel accepted a nil runtime")
	}
}

func TestToastTimerWiringReplacesAndRejectsStaleExpiry(t *testing.T) {
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	next, firstTimer := model.Update(appEventMsg{event: app.ClipboardWritten{}})
	model = next.(AppModel)
	firstGeneration := engine.Snapshot().ToastGeneration
	if firstTimer == nil || firstGeneration == 0 {
		t.Fatal("clipboard completion did not schedule toast expiry")
	}

	next, secondTimer := model.Update(appEventMsg{event: app.OperationFailed{Error: domain.AppError{Kind: domain.ErrorNetwork, Message: "Try again"}}})
	model = next.(AppModel)
	secondGeneration := engine.Snapshot().ToastGeneration
	if secondTimer == nil || secondGeneration == firstGeneration {
		t.Fatal("replacement toast did not restart expiry generation")
	}

	next, _ = model.Update(toastExpiredMsg{generation: firstGeneration})
	model = next.(AppModel)
	if engine.Snapshot().Toast == nil {
		t.Fatal("stale timer cleared replacement toast")
	}
	next, _ = model.Update(toastExpiredMsg{generation: secondGeneration})
	_ = next.(AppModel)
	if engine.Snapshot().Toast != nil {
		t.Fatal("current timer did not clear toast")
	}
}

func TestUpdateNonBlockingWhenRuntimeCommandBufferFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := &AppRuntime{
		ctx:      ctx,
		cancel:   cancel,
		commands: make(chan app.Command, appRuntimeBuffer),
	}
	for range appRuntimeBuffer {
		runtime.commands <- app.LoadBootstrap{}
	}
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), runtime)
	result := make(chan tea.Cmd, 1)
	go func() {
		_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		result <- cmd
	}()

	select {
	case cmd := <-result:
		if cmd == nil {
			t.Fatal("Update returned nil command for app.Quit")
		}
		if got := len(runtime.commands); got != appRuntimeBuffer {
			t.Fatalf("Update delivered command eagerly; buffer length = %d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Update blocked while runtime command buffer was full")
	}
}

func newAppModelForTest(t *testing.T, engine *app.Engine, runtime *AppRuntime) AppModel {
	t.Helper()
	model, err := NewAppModel(engine, runtime)
	if err != nil {
		t.Fatal("NewAppModel returned an unexpected error")
	}
	return model
}

func newBoundedAppRuntimeForModelTest(t *testing.T) *AppRuntime {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runtime, err := NewAppRuntime(ctx, app.NewExecutor(1, nil))
	if err != nil {
		cancel()
		t.Fatal("NewAppRuntime returned an unexpected error")
	}
	t.Cleanup(func() {
		runtime.Close()
		runtime.Wait()
		cancel()
	})
	return runtime
}

func updateAppModel(t *testing.T, model AppModel, msg tea.Msg) (AppModel, tea.Cmd) {
	t.Helper()
	next, cmd := model.Update(msg)
	updated, ok := next.(AppModel)
	if !ok {
		t.Fatalf("Update returned %T, want frontend.AppModel", next)
	}
	return updated, cmd
}

func TestAppModelBlurAndFocusUpdateAuthoritativeState(t *testing.T) {
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

	// Blur should set TerminalFocused = false.
	model, _ = updateAppModel(t, model, tea.BlurMsg{})
	snap := engine.Snapshot()
	if snap.TerminalFocused {
		t.Fatal("TerminalFocused should be false after BlurMsg")
	}

	// Focus should set TerminalFocused = true.
	model, _ = updateAppModel(t, model, tea.FocusMsg{})
	snap = engine.Snapshot()
	if !snap.TerminalFocused {
		t.Fatal("TerminalFocused should be true after FocusMsg")
	}
}

func TestAppModelViewReportFocus(t *testing.T) {
	engine := app.NewEngine(app.InitialState())
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

	view := model.View()
	if !view.ReportFocus {
		t.Fatal("View.ReportFocus should be true")
	}
	if view.BackgroundColor != nil {
		t.Fatalf("View.BackgroundColor type = %T, want nil terminal background", view.BackgroundColor)
	}
}
