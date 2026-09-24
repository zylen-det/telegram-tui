package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func photoPathValueState() State {
	state := InitialState()
	state.Focus = FocusPhotoSend
	state.PhotoSend = &PhotoSendState{
		ChatID:        domain.ChatID(9),
		Input:         []rune("before"),
		PreviousFocus: FocusComposer,
	}
	return state
}

func TestPhotoPathValueChangedReplacesWholeSingleLineValue(t *testing.T) {
	state := photoPathValueState()
	commands := updateState(&state, PhotoPathValueChanged{ChatID: 9, Value: " /tmp/界🙂\nnext\rline "})
	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	if want := " /tmp/界🙂nextline "; string(state.PhotoSend.Input) != want {
		t.Fatalf("photo path = %q, want %q", string(state.PhotoSend.Input), want)
	}
	if state.PhotoSend.ChatID != 9 || state.PhotoSend.PreviousFocus != FocusComposer {
		t.Fatal("whole-value update changed photo-send identity or focus metadata")
	}
}

func TestPhotoPathValueChangedEmptyIsNonNil(t *testing.T) {
	state := photoPathValueState()
	updateState(&state, PhotoPathValueChanged{ChatID: 9, Value: ""})
	if state.PhotoSend.Input == nil || len(state.PhotoSend.Input) != 0 {
		t.Fatalf("empty photo path = %#v, want non-nil empty rune slice", state.PhotoSend.Input)
	}
}

func TestPhotoPathValueChangedIdentityAndLifecycleMismatchesAreNoOps(t *testing.T) {
	for _, test := range []struct {
		name  string
		fn    func() State
		event PhotoPathValueChanged
	}{
		{name: "zero chat ID", fn: photoPathValueState, event: PhotoPathValueChanged{Value: "after"}},
		{name: "wrong chat ID", fn: photoPathValueState, event: PhotoPathValueChanged{ChatID: 10, Value: "after"}},
		{name: "wrong focus", fn: func() State { s := photoPathValueState(); s.Focus = FocusComposer; return s }, event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "no photo send", fn: func() State { s := photoPathValueState(); s.PhotoSend = nil; return s }, event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "quitting", fn: func() State { s := photoPathValueState(); s.Quitting = true; return s }, event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := test.fn()
			unchanged := test.fn()
			commands := updateState(&state, test.event)
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
			if !reflect.DeepEqual(state, unchanged) {
				t.Fatalf("mismatch changed state\n got: %#v\nwant: %#v", state, unchanged)
			}
		})
	}
}
