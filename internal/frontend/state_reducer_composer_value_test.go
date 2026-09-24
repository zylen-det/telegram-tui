package frontend

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

	commands := updateState(&state, event)

	if len(commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(commands))
	}
	save, ok := commands[0].(SaveDraft)
	if !ok || save.ChatID != 9 || save.Text != value || save.ReplyToMessageID != 0 || save.RequestID == 0 {
		t.Fatalf("save command = %#v", commands[0])
	}

	if state.Drafts[9] != value {
		t.Fatalf("draft 9 = %q, want %q", state.Drafts[9], value)
	}

	if state.Drafts[10] != "keep" {
		t.Fatalf("draft 10 = %q, want %q", state.Drafts[10], "keep")
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
			state := tc.fn()
			unchanged := tc.fn()

			commands := updateState(&state, tc.event)

			if !reflect.DeepEqual(state, unchanged) {
				t.Fatalf("result != input (deep-equal failed):\ngot  = %#v\nwant = %#v", state, unchanged)
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

	commands := updateState(&state, event)

	if len(commands) != 0 {
		t.Fatalf("commands = %d, want 0", len(commands))
	}

	if state.EditTarget.Buffer != value {
		t.Fatalf("edit buffer = %q, want %q", state.EditTarget.Buffer, value)
	}
	// Only the buffer may change; identity and lifecycle fields stay as configured.
	if state.EditTarget.RequestID != 5 || state.EditTarget.Original != "original text" ||
		state.EditTarget.ChatID != 9 || state.EditTarget.MessageID != 22 ||
		state.EditTarget.Submitting || state.EditTarget.Error != nil {
		t.Fatalf("edit target = %#v, want only buffer replaced", state.EditTarget)
	}
	if state.Drafts[10] != "draft 10" {
		t.Fatalf("draft 10 mutated: %q", state.Drafts[10])
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
			state := tc.fn()
			unchanged := tc.fn()

			commands := updateState(&state, tc.event)

			if !reflect.DeepEqual(state, unchanged) {
				t.Fatalf("result != input (deep-equal no-op failed):\ngot  = %#v\nwant = %#v", state, unchanged)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %d, want 0", len(commands))
			}
		})
	}
}
