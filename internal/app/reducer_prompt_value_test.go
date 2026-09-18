package app

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
	inputCopy := cloneReducerState(state)
	value := "s界🙂cret"

	got, commands := Reduce(state, PromptValueChanged{PromptID: 41, Value: value})

	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}
	if got.Prompt == nil || string(got.Prompt.Input) != value {
		t.Fatalf("prompt input = %#v, want %q", got.Prompt, value)
	}
	if !reflect.DeepEqual(state, inputCopy) {
		t.Fatalf("input state mutated:\ngot  = %#v\nwant = %#v", state, inputCopy)
	}
}

func TestPromptValueChangedRejectsStaleOrInactiveIdentity(t *testing.T) {
	tests := []struct {
		name  string
		state State
		event PromptValueChanged
	}{
		{name: "wrong prompt ID", state: promptValueState(), event: PromptValueChanged{PromptID: 99, Value: "new"}},
		{name: "zero prompt ID", state: promptValueState(), event: PromptValueChanged{PromptID: 0, Value: "new"}},
		{name: "wrong focus", state: func() State { s := promptValueState(); s.Focus = FocusConversation; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "no prompt", state: func() State { s := promptValueState(); s.Prompt = nil; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "quitting", state: func() State { s := promptValueState(); s.Quitting = true; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputCopy := cloneReducerState(test.state)
			got, commands := Reduce(test.state, test.event)
			if !reflect.DeepEqual(got, inputCopy) {
				t.Fatalf("result changed:\ngot  = %#v\nwant = %#v", got, inputCopy)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %d, want 0", len(commands))
			}
		})
	}
}

func TestPromptValueChangedAcceptsEmptyWholeValue(t *testing.T) {
	state := promptValueState()
	got, commands := Reduce(state, PromptValueChanged{PromptID: 41, Value: ""})
	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}
	if got.Prompt == nil || got.Prompt.Input == nil || len(got.Prompt.Input) != 0 {
		t.Fatalf("prompt input = %#v, want non-nil empty rune slice", got.Prompt)
	}
}
