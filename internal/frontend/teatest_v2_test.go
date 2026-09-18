package frontend

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

func TestBubbleTeaTeatestRendersAuthorizationShell(t *testing.T) {
	tm := teatest.NewTestModel(t, NewModel(), teatest.WithInitialTermSize(100, 24))
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		plain := ansiSequence.ReplaceAllString(string(output), "")
		return strings.Contains(plain, "Authorization") &&
			strings.Contains(plain, "Telegram API ID") &&
			strings.Contains(plain, "Enter the numeric api_id")
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	for _, value := range "12345" {
		tm.Send(tea.KeyPressMsg(tea.Key{Code: value, Text: string(value)}))
	}
	// Bubble Tea v2 renders frames as incremental cell diffs (only changed
	// cells, addressed by cursor movement), so teatest's accumulated output
	// never shows rapidly-typed digits contiguously even though they all land
	// in the input. Resizing the terminal forces a full repaint that renders
	// the accumulated input as one contiguous string.
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 23})
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(ansiSequence.ReplaceAllString(string(output), ""), "12345")
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	tm.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(ansiSequence.ReplaceAllString(string(output), ""), "Input accepted; waiting for authorization")
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	tm.Send(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if _, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second))); err != nil {
		t.Fatal("could not read final teatest output")
	}
}
