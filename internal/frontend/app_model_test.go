package frontend

import (
	"context"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAppModelMapsInputIntoEngineState(t *testing.T) {
	state := InitialState()
	state.Prompt = &PromptState{Prompt: auth.Prompt{
		ID:     17,
		Kind:   auth.PromptPassword,
		Label:  "Telegram password",
		Secret: true,
	}}
	state.Focus = FocusAuth
	model := newAppModelForTest(t, state, newTestSession(t))

	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a界"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "🙂x"})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))

	if got, want := model.Snapshot().Width, 100; got != want {
		t.Fatalf("state width = %d, want %d", got, want)
	}
	if got, want := string(model.Snapshot().Prompt.Input), "a界🙂"; got != want {
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

func TestNilSessionRejected(t *testing.T) {
	if _, err := NewAppModel(InitialState(), nil); err == nil {
		t.Fatal("NewAppModel accepted a nil session")
	}
}

func TestToastTimerWiringReplacesAndRejectsStaleExpiry(t *testing.T) {
	model := newAppModelForTest(t, InitialState(), newTestSession(t))

	next, firstTimer := model.Update(ClipboardWritten{})
	model = next.(AppModel)
	firstGeneration := model.Snapshot().ToastGeneration
	if firstTimer == nil || firstGeneration == 0 {
		t.Fatal("clipboard completion did not schedule toast expiry")
	}

	next, secondTimer := model.Update(OperationFailed{Error: domain.AppError{Kind: domain.ErrorNetwork, Message: "Try again"}})
	model = next.(AppModel)
	secondGeneration := model.Snapshot().ToastGeneration
	if secondTimer == nil || secondGeneration == firstGeneration {
		t.Fatal("replacement toast did not restart expiry generation")
	}

	next, _ = model.Update(toastExpiredMsg{generation: firstGeneration})
	model = next.(AppModel)
	if model.Snapshot().Toast == nil {
		t.Fatal("stale timer cleared replacement toast")
	}
	next, _ = model.Update(toastExpiredMsg{generation: secondGeneration})
	_ = next.(AppModel)
	if model.Snapshot().Toast != nil {
		t.Fatal("current timer did not clear toast")
	}
}

func newAppModelForTest(t *testing.T, state State, session *Handler) AppModel {
	t.Helper()
	model, err := NewAppModel(state, session)
	if err != nil {
		t.Fatal("NewAppModel returned an unexpected error")
	}
	return model
}

// newTestSession returns a session with no runtime dependencies. Model tests
// exercise state and input; they do not run effects.
func newTestSession(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(context.Background(), nil, nil, nil, nil)
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
	model := newAppModelForTest(t, InitialState(), newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

	// Blur should set TerminalFocused = false.
	model, _ = updateAppModel(t, model, tea.BlurMsg{})
	snap := model.Snapshot()
	if snap.TerminalFocused {
		t.Fatal("TerminalFocused should be false after BlurMsg")
	}

	// Focus should set TerminalFocused = true.
	model, _ = updateAppModel(t, model, tea.FocusMsg{})
	snap = model.Snapshot()
	if !snap.TerminalFocused {
		t.Fatal("TerminalFocused should be true after FocusMsg")
	}
}

func TestAppModelViewReportFocus(t *testing.T) {
	model := newAppModelForTest(t, InitialState(), newTestSession(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})

	view := model.View()
	if !view.ReportFocus {
		t.Fatal("View.ReportFocus should be true")
	}
	if view.BackgroundColor != nil {
		t.Fatalf("View.BackgroundColor type = %T, want nil terminal background", view.BackgroundColor)
	}
}
