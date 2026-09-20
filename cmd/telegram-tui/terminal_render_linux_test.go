//go:build linux

package main

import (
	"context"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/frontend"
	"github.com/zylen-det/telegram-tui/internal/telegram"
	"golang.org/x/term"
)

func TestOutputOverlayPreservesTerminalFileDescriptor(t *testing.T) {
	primary, replica, err := pty.Open()
	if err != nil {
		t.Fatal("could not open PTY")
	}
	defer primary.Close()
	defer replica.Close()

	overlay := frontend.NewOutputOverlay(replica, nil, nil)
	if got, want := overlay.Fd(), replica.Fd(); got != want {
		t.Fatalf("overlay Fd = %d, want terminal fd %d", got, want)
	}
	if !term.IsTerminal(int(overlay.Fd())) {
		t.Fatal("Bubble Tea would not recognize OutputOverlay as a terminal")
	}
}

func TestProductionPathRendersAuthorizationInRealPTY(t *testing.T) {
	primary, replica, err := pty.Open()
	if err != nil {
		t.Fatal("could not open PTY")
	}
	defer primary.Close()
	defer replica.Close()
	if err := pty.Setsize(replica, &pty.Winsize{Rows: 24, Cols: 100}); err != nil {
		t.Fatal("could not size PTY")
	}

	before, err := term.GetState(int(replica.Fd()))
	if err != nil {
		t.Fatal("could not read initial terminal state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	model := productionTestAuthorizationModel(t, ctx)
	output := frontend.NewOutputOverlay(replica, nil, nil)
	model.SetOutputOverlay(output)
	program := newProductionProgram(ctx, replica, output, model)

	type result struct{ err error }
	finished := make(chan result, 1)
	go func() {
		_, runErr := program.Run()
		finished <- result{err: runErr}
	}()

	captured := readPTYUntil(t, primary, func(value string) bool {
		plain := ansiStripForPTY.ReplaceAllString(value, "")
		return strings.Contains(plain, "Authorization") && strings.Contains(plain, "Telegram API ID")
	})
	plain := ansiStripForPTY.ReplaceAllString(captured, "")
	if !strings.Contains(plain, "Enter the numeric api_id") {
		t.Fatal("production PTY frame omitted API ID guidance")
	}
	if state := model.Snapshot(); state.Width != 100 || state.Height != 24 {
		t.Fatalf("production model size = %dx%d, want 100x24", state.Width, state.Height)
	}

	program.Send(frontend.ProcessQuitMsg{})
	select {
	case run := <-finished:
		if run.err != nil {
			t.Fatal("production Bubble Tea program returned an unexpected error")
		}
	case <-time.After(3 * time.Second):
		program.Kill()
		t.Fatal("production Bubble Tea program did not stop")
	}
	after, err := term.GetState(int(replica.Fd()))
	if err != nil {
		t.Fatal("could not read restored terminal state")
	}
	if !sameTerminalState(before, after) {
		t.Fatal("Bubble Tea did not restore PTY terminal state")
	}
}

func readPTYUntil(t *testing.T, primary *os.File, condition func(string) bool) string {
	t.Helper()
	result := make(chan []byte, 1)
	go func() {
		var all []byte
		buffer := make([]byte, 8192)
		for {
			count, err := primary.Read(buffer)
			if count > 0 {
				all = append(all, buffer[:count]...)
				if condition(string(all)) {
					result <- all
					return
				}
			}
			if err != nil {
				result <- all
				return
			}
		}
	}()
	select {
	case content := <-result:
		if !condition(string(content)) {
			t.Fatal("timed out before a visible authorization frame")
		}
		return string(content)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for production authorization frame")
		return ""
	}
}

var _ io.ReadWriteCloser = (*frontend.OutputOverlay)(nil)

// productionPTYResolver asks the broker for the API ID so the production
// authorization frame renders, then blocks until shutdown cancels its context.
type productionPTYResolver struct{ broker *auth.Broker }

func (r productionPTYResolver) Resolve(ctx context.Context) (config.Runtime, error) {
	if _, err := r.broker.Ask(ctx, auth.Prompt{Kind: auth.PromptAPIID, Label: "Telegram API ID"}); err != nil {
		return config.Runtime{}, err
	}
	return config.Runtime{APIID: 12345}, nil
}

func productionTestAuthorizationModel(t *testing.T, ctx context.Context) frontend.AppModel {
	t.Helper()
	broker := auth.NewBroker()
	handler := frontend.NewHandler(ctx, productionPTYResolver{broker: broker}, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return telegram.NewFake(telegram.FakeData{}), nil
	}, broker, nil)
	model, err := frontend.NewAppModel(frontend.InitialState(), handler)
	if err != nil {
		t.Fatal("could not create production test model")
	}
	return model
}

func newProductionProgram(ctx context.Context, input io.Reader, output io.Writer, model frontend.AppModel) *tea.Program {
	return tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithEnvironment([]string{"TERM=xterm-kitty", "COLORTERM=truecolor"}),
		tea.WithoutSignalHandler(),
	)
}

var ansiStripForPTY = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|_[^\x1b]*(?:\x1b\\))`)

func sameTerminalState(left, right *term.State) bool { return reflect.DeepEqual(left, right) }
