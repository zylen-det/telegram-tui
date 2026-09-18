package app

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComposerValueChangedUpdatesMatchingDraft(t *testing.T) {
	state := selectedWritableState()
	state.Drafts[9] = "old"
	state.Drafts[10] = "keep"

	value := "hello\nworld 🎉\n你好世界"
	event := ComposerValueChanged{ChatID: 9, EditMessageID: 0, Value: value}

	inputDraft9 := state.Drafts[9]

	got, commands := Reduce(state, event)

	if len(commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(commands))
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.ChatID != 9 || save.Text != value || save.ReplyToMessageID != 0 || save.RequestID == 0 {
		t.Fatalf("save command = %#v", commands[0])
	}

	if got.Drafts[9] != value {
		t.Fatalf("draft 9 = %q, want %q", got.Drafts[9], value)
	}

	if got.Drafts[10] != "keep" {
		t.Fatalf("draft 10 = %q, want %q", got.Drafts[10], "keep")
	}

	// Input state's draft 9 must remain unchanged (immutability via clone).
	if state.Drafts[9] != inputDraft9 {
		t.Fatalf("input state draft 9 mutated: was %q, still %q", inputDraft9, state.Drafts[9])
	}
}

func TestComposerValueChangedRejectsStaleDraftIdentity(t *testing.T) {
	testCases := []struct {
		name  string
		fn    func() State
		event ComposerValueChanged
	}{
		{
			name: "wrong focus",
			fn: func() State {
				state := selectedWritableState()
				state.Focus = FocusChats
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 0, Value: "new"},
		},
		{
			name: "wrong chat ID",
			fn: func() State {
				return selectedWritableState()
			},
			event: ComposerValueChanged{ChatID: 99, EditMessageID: 0, Value: "new"},
		},
		{
			name: "nonzero edit ID with no edit target",
			fn: func() State {
				return selectedWritableState()
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 42, Value: "new"},
		},
		{
			name: "quitting",
			fn: func() State {
				state := selectedWritableState()
				state.Quitting = true
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 0, Value: "new"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.fn()
			inputCopy := cloneReducerState(input)
			event := tc.event

			got, commands := Reduce(input, event)

			if !reflect.DeepEqual(got, inputCopy) {
				t.Fatalf("result != input (deep-equal failed):\ngot  = %#v\nwant = %#v", got, inputCopy)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %d, want 0", len(commands))
			}
		})
	}
}

func TestComposerValueChangedUpdatesMatchingEdit(t *testing.T) {
	state := selectedWritableState()

	state.Messages[9] = []domain.Message{{
		ID:         22,
		ChatID:     9,
		Kind:       domain.MessageText,
		Text:       "original text",
		Sender:     domain.SenderRef{Kind: domain.SenderUser, ID: 41},
		SenderName: "Alice",
	}}

	state.EditTarget = &EditTarget{
		RequestID:  5,
		ChatID:     9,
		MessageID:  22,
		Original:   "original text",
		Buffer:     "editing this",
		Submitting: false,
	}

	// Unrelated draft should survive intact.
	state.Drafts[10] = "draft 10"

	value := "new editor value 🚀"
	event := ComposerValueChanged{ChatID: 9, EditMessageID: 22, Value: value}

	// Capture input target state for immutability check.
	inputBuffer := state.EditTarget.Buffer
	inputOriginal := state.EditTarget.Original
	inputRequestID := state.EditTarget.RequestID

	got, commands := Reduce(state, event)

	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}

	if got.EditTarget.Buffer != value {
		t.Fatalf("edit buffer = %q, want %q", got.EditTarget.Buffer, value)
	}
	if got.EditTarget.Original != inputOriginal {
		t.Fatalf("edit original changed: was %q, got %q", inputOriginal, got.EditTarget.Original)
	}
	if got.EditTarget.RequestID != inputRequestID {
		t.Fatalf("edit request ID changed: was %d, got %d", inputRequestID, got.EditTarget.RequestID)
	}
	if got.EditTarget.ChatID != 9 {
		t.Fatalf("edit chat ID changed: %d", got.EditTarget.ChatID)
	}
	if got.EditTarget.MessageID != 22 {
		t.Fatalf("edit message ID changed: %d", got.EditTarget.MessageID)
	}
	if got.EditTarget.Submitting {
		t.Fatal("edit submitting changed unexpectedly")
	}
	if got.EditTarget.Error != nil {
		t.Fatal("edit error appeared unexpectedly")
	}
	if got.Drafts[10] != "draft 10" {
		t.Fatalf("draft 10 mutated: %q", got.Drafts[10])
	}

	// Input state target must be unchanged (clone-first immutability).
	if state.EditTarget.Buffer != inputBuffer {
		t.Fatalf("input edit buffer mutated: was %q, still %q", inputBuffer, state.EditTarget.Buffer)
	}
}

func TestComposerValueChangedRejectsStaleOrSubmittingEdit(t *testing.T) {
	testCases := []struct {
		name  string
		fn    func() State
		event ComposerValueChanged
	}{
		{
			name: "zero edit ID with edit target present",
			fn: func() State {
				state := selectedWritableState()
				state.Messages[9] = []domain.Message{{
					ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "edit me",
				}}
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer"}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 0, Value: "new"},
		},
		{
			name: "wrong edit message ID",
			fn: func() State {
				state := selectedWritableState()
				state.Messages[9] = []domain.Message{{
					ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "edit me",
				}}
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer"}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 99, Value: "new"},
		},
		{
			name: "wrong chat ID",
			fn: func() State {
				state := selectedWritableState()
				state.Messages[9] = []domain.Message{{
					ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "edit me",
				}}
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer"}
				return state
			},
			event: ComposerValueChanged{ChatID: 7, EditMessageID: 22, Value: "new"},
		},
		{
			name: "submitting",
			fn: func() State {
				state := selectedWritableState()
				state.Messages[9] = []domain.Message{{
					ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "edit me",
				}}
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer", Submitting: true}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 22, Value: "new"},
		},
		{
			name: "missing target message",
			fn: func() State {
				state := selectedWritableState()
				// No message at 9/22.
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer"}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 22, Value: "new"},
		},
		{
			name: "non-editable target message",
			fn: func() State {
				state := selectedWritableState()
				state.Messages[9] = []domain.Message{{
					ID: 22, ChatID: 9, Kind: domain.MessagePhoto, Text: "photo",
				}}
				state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "buffer"}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 22, Value: "new"},
		},
		{
			name: "stale_edit_target_from_another_chat_with_colliding_message_ID",
			fn: func() State {
				state := selectedWritableState()
				// Active chat is 9.
				state.Messages[7] = []domain.Message{{
					ID: 22, ChatID: 7, Kind: domain.MessageText, Text: "editable",
				}}
				// EditTarget belongs to chat 7, not the active chat 9.
				state.EditTarget = &EditTarget{ChatID: 7, MessageID: 22, Buffer: "stale other chat"}
				return state
			},
			event: ComposerValueChanged{ChatID: 9, EditMessageID: 22, Value: "must not apply"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.fn()
			inputCopy := cloneReducerState(input)
			event := tc.event

			got, commands := Reduce(input, event)

			if !reflect.DeepEqual(got, inputCopy) {
				t.Fatalf("result != input (deep-equal no-op failed):\ngot  = %#v\nwant = %#v", got, inputCopy)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %d, want 0", len(commands))
			}
		})
	}
}
