package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/auth"
)

func promptValueState() State {
	state := InitialState()
	state.Focus = FocusAuth
	state.Prompt = &PromptState{
		Prompt: auth.Prompt{ID: 41, Kind: auth.PromptPassword, Label: "Password", Secret: true},
		Input:  []rune("old"),
	}
	return state
}

func TestPromptValueChangedReplacesMatchingWholeValue(t *testing.T) {
	state := promptValueState()
	value := "s界🙂cret"

	commands := updateState(&state, PromptValueChanged{PromptID: 41, Value: value})

	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}
	if state.Prompt == nil || string(state.Prompt.Input) != value {
		t.Fatalf("prompt input = %#v, want %q", state.Prompt, value)
	}
}

func TestPromptValueChangedRejectsStaleOrInactiveIdentity(t *testing.T) {
	tests := []struct {
		name  string
		fn    func() State
		event PromptValueChanged
	}{
		{name: "wrong prompt ID", fn: promptValueState, event: PromptValueChanged{PromptID: 99, Value: "new"}},
		{name: "zero prompt ID", fn: promptValueState, event: PromptValueChanged{PromptID: 0, Value: "new"}},
		{name: "wrong focus", fn: func() State { s := promptValueState(); s.Focus = FocusConversation; return s }, event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "no prompt", fn: func() State { s := promptValueState(); s.Prompt = nil; return s }, event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "quitting", fn: func() State { s := promptValueState(); s.Quitting = true; return s }, event: PromptValueChanged{PromptID: 41, Value: "new"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := test.fn()
			unchanged := test.fn()
			commands := updateState(&state, test.event)
			if !reflect.DeepEqual(state, unchanged) {
				t.Fatalf("result changed:\ngot  = %#v\nwant = %#v", state, unchanged)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %d, want 0", len(commands))
			}
		})
	}
}

func TestPromptValueChangedAcceptsEmptyWholeValue(t *testing.T) {
	state := promptValueState()
	commands := updateState(&state, PromptValueChanged{PromptID: 41, Value: ""})
	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}
	if state.Prompt == nil || state.Prompt.Input == nil || len(state.Prompt.Input) != 0 {
		t.Fatalf("prompt input = %#v, want non-nil empty rune slice", state.Prompt)
	}
}
