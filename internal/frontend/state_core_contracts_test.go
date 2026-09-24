package frontend

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestLayoutBreakpoints(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   Layout
	}{
		{width: 120, height: 24, want: LayoutWide},
		{width: 119, height: 24, want: LayoutNormal},
		{width: 80, height: 20, want: LayoutNormal},
		{width: 79, height: 20, want: LayoutNarrow},
		{width: 60, height: 18, want: LayoutNarrow},
		{width: 59, height: 30, want: LayoutTooSmall},
		{width: 120, height: 17, want: LayoutTooSmall},
	}

	for _, tt := range tests {
		t.Run(layoutName(tt.width, tt.height), func(t *testing.T) {
			state := InitialState()
			commands := updateState(&state, Resized{Width: tt.width, Height: tt.height})

			if state.Layout != tt.want {
				t.Fatalf("Layout = %v, want %v", state.Layout, tt.want)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
		})
	}
}

func TestAvatarResultAndStaleResultPreserveModalFocus(t *testing.T) {
	state := InitialState()
	state.Focus = FocusModal
	state.Modal = &ModalState{RequestID: 42, Loading: true, PreviousFocus: FocusConversation}

	commands := updateState(&state, AvatarOpened{RequestID: 42, Title: "Mina", Path: "/tmp/mina.jpg"})
	if len(commands) != 0 || state.Modal == nil || state.Modal.Loading || state.Modal.Title != "Mina" || state.Modal.Path != "/tmp/mina.jpg" {
		t.Fatalf("avatar result = %#v commands=%#v", state.Modal, commands)
	}
	assertModalFocusInvariant(t, state)

	commands = updateState(&state, AvatarOpened{RequestID: 41, Title: "Other", Path: "/tmp/other.jpg"})
	if len(commands) != 0 || state.Modal.Title != "Mina" || state.Modal.Path != "/tmp/mina.jpg" {
		t.Fatalf("stale result changed modal: %#v commands=%#v", state.Modal, commands)
	}
	assertModalFocusInvariant(t, state)

	commands = updateState(&state, ActionReceived{Action: Close})
	if len(commands) != 0 || state.Modal != nil || state.Focus != FocusConversation {
		t.Fatalf("close = modal:%#v focus:%v commands:%#v", state.Modal, state.Focus, commands)
	}
	assertModalFocusInvariant(t, state)
}

func TestPromptSubmitClearsSensitiveInputAndRestoresFocus(t *testing.T) {
	state := InitialState()
	state.Focus = FocusAuth
	promptState := &PromptState{
		Prompt: auth.Prompt{
			ID:     44,
			Kind:   auth.PromptPassword,
			Label:  "Password",
			Secret: true,
		},
		Input:         []rune("秘密"),
		PreviousFocus: FocusConversation,
	}
	state.Prompt = promptState

	commands := updateState(&state, ActionReceived{Action: ComposerSubmit})
	got := state

	if got.Prompt != nil || promptState.Input != nil {
		t.Fatalf("sensitive prompt retained: state=%#v input=%q", got.Prompt, string(promptState.Input))
	}
	if got.Focus != FocusConversation {
		t.Fatalf("Focus = %v, want %v", got.Focus, FocusConversation)
	}
	assertCommands(t, commands, []Effect{SubmitPrompt{Response: auth.Response{
		PromptID: 44,
		Value:    "秘密",
	}}})
}

func TestTypedNilPointerEventsAreNoOps(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{name: "Started", event: (*Started)(nil)},
		{name: "Resized", event: (*Resized)(nil)},
		{name: "ActionReceived", event: (*ActionReceived)(nil)},
		{name: "ChatsLoaded", event: (*ChatsLoaded)(nil)},
		{name: "MessagesLoaded", event: (*MessagesLoaded)(nil)},
		{name: "TelegramEvent", event: (*TelegramEvent)(nil)},
		{name: "AvatarOpened", event: (*AvatarOpened)(nil)},
		{name: "OperationFailed", event: (*OperationFailed)(nil)},
		{name: "ShutdownComplete", event: (*ShutdownComplete)(nil)},
		{name: "PromptRequested", event: (*PromptRequested)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := populatedModalState()
			before := cloneState(state)

			commands := updateState(&state, tt.event)
			got := state

			if !reflect.DeepEqual(got, before) {
				t.Fatalf("state changed:\n got: %#v\nwant: %#v", got, before)
			}
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
		})
	}
}

func layoutName(width, height int) string {
	return fmt.Sprintf("%dx%d", width, height)
}

func assertModalFocusInvariant(t *testing.T, state State) {
	t.Helper()
	if state.Modal != nil && state.Focus != FocusModal {
		t.Fatalf("active modal has Focus = %v, want %v", state.Focus, FocusModal)
	}
	if state.Modal == nil && state.Focus == FocusModal {
		t.Fatal("Focus = FocusModal with no active modal")
	}
}

func cloneState(state State) State {
	clone := state
	clone.Chats = append([]domain.Chat(nil), state.Chats...)
	clone.Messages = make(map[domain.ChatID][]domain.Message, len(state.Messages))
	for chatID, messages := range state.Messages {
		clone.Messages[chatID] = append([]domain.Message(nil), messages...)
	}
	clone.Drafts = make(map[domain.ChatID]string, len(state.Drafts))
	for chatID, draft := range state.Drafts {
		clone.Drafts[chatID] = draft
	}
	if state.Modal != nil {
		modal := *state.Modal
		clone.Modal = &modal
	}
	if state.Prompt != nil {
		prompt := *state.Prompt
		if state.Prompt.Input != nil {
			prompt.Input = append([]rune{}, state.Prompt.Input...)
		}
		clone.Prompt = &prompt
	}
	if state.Toast != nil {
		toast := *state.Toast
		clone.Toast = &toast
	}
	return clone
}

func populatedModalState() State {
	state := InitialState()
	state.Width = 90
	state.Height = 22
	state.Layout = LayoutNormal
	state.Focus = FocusModal
	state.FocusBeforeInfo = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 7, Title: "Mina"}, {ID: 9, Title: "Team"}}
	state.SelectedChat = 1
	state.Messages[9] = []domain.Message{{ID: 11, ChatID: 9, Text: "hello"}}
	state.SelectedMessage = 11
	state.Drafts[9] = "draft text"
	state.DetailsOpen = true
	state.Modal = &ModalState{
		RequestID:     42,
		Title:         "Mina",
		Ref:           domain.AvatarRef{FileID: 5, UniqueID: "mina"},
		Path:          "/tmp/mina.jpg",
		Loading:       true,
		PreviousFocus: FocusDetails,
	}
	state.Toast = &domain.AppError{Kind: domain.ErrorMedia, Message: "still loading"}
	return state
}
