package app

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
	before := cloneState(state)
	got, commands := Reduce(state, PhotoPathValueChanged{ChatID: 9, Value: " /tmp/界🙂\nnext\rline "})
	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	if want := " /tmp/界🙂nextline "; string(got.PhotoSend.Input) != want {
		t.Fatalf("photo path = %q, want %q", string(got.PhotoSend.Input), want)
	}
	if got.PhotoSend.ChatID != before.PhotoSend.ChatID || got.PhotoSend.PreviousFocus != before.PhotoSend.PreviousFocus {
		t.Fatal("whole-value update changed photo-send identity or focus metadata")
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatal("Reduce mutated its input state")
	}
}

func TestPhotoPathValueChangedEmptyIsNonNil(t *testing.T) {
	got, _ := Reduce(photoPathValueState(), PhotoPathValueChanged{ChatID: 9, Value: ""})
	if got.PhotoSend.Input == nil || len(got.PhotoSend.Input) != 0 {
		t.Fatalf("empty photo path = %#v, want non-nil empty rune slice", got.PhotoSend.Input)
	}
}

func TestPhotoPathValueChangedIdentityAndLifecycleMismatchesAreNoOps(t *testing.T) {
	base := photoPathValueState()
	for _, test := range []struct {
		name  string
		state State
		event PhotoPathValueChanged
	}{
		{name: "zero chat ID", state: base, event: PhotoPathValueChanged{Value: "after"}},
		{name: "wrong chat ID", state: base, event: PhotoPathValueChanged{ChatID: 10, Value: "after"}},
		{name: "wrong focus", state: func() State { s := base; s.Focus = FocusComposer; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "no photo send", state: func() State { s := base; s.PhotoSend = nil; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "quitting", state: func() State { s := base; s.Quitting = true; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := cloneState(test.state)
			got, commands := Reduce(test.state, test.event)
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
			if !reflect.DeepEqual(got, before) {
				t.Fatalf("mismatch changed state\n got: %#v\nwant: %#v", got, before)
			}
			if !reflect.DeepEqual(test.state, before) {
				t.Fatal("Reduce mutated mismatch input state")
			}
		})
	}
}
