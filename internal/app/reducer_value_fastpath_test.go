package app

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

// These tests pin the narrow copy-on-write fast path for value events. Those
// events are dispatched before cloneReducerState, so each reducer must clone
// only the overlay state it owns: a successful event has to leave its input
// State (and everything reachable from it) untouched, while unrelated overlays
// stay shared instead of being deep-cloned.

// requireInputUnchanged fails when Reduce changed anything reachable from the
// State it was handed. pre is the clone taken before the call; both sides go
// through cloneReducerState so its nil/empty normalization cannot fake a pass.
func requireInputUnchanged(t *testing.T, label string, input, pre State) {
	t.Helper()
	if !reflect.DeepEqual(cloneReducerState(input), pre) {
		t.Fatalf("%s: Reduce mutated its input state:\nafter  = %#v\nbefore = %#v", label, input, pre)
	}
}

// requireUnchangedResult fails when a rejected value event returned a state
// that differs from its input.
func requireUnchangedResult(t *testing.T, label string, pre, got State) {
	t.Helper()
	if !reflect.DeepEqual(cloneReducerState(got), pre) {
		t.Fatalf("%s: rejected value event changed state:\ngot  = %#v\nwant = %#v", label, got, pre)
	}
}

// requireNarrowClone checks the fast path stayed narrow: the owned overlay must
// be replaced, while an untouched overlay stays the very same pointer.
func requireNarrowClone(t *testing.T, label string, input, got State, ownedReplaced bool) {
	t.Helper()
	if input.Modal == nil || got.Modal != input.Modal {
		t.Fatalf("%s: value fast path cloned or dropped an unrelated overlay: modal=%#v", label, got.Modal)
	}
	if !ownedReplaced {
		t.Fatalf("%s: value fast path published the caller's overlay pointer, so both states alias it", label)
	}
}

func withSentinelModal(state State) State {
	state.Modal = &ModalState{RequestID: 77, Title: "sentinel", PreviousFocus: FocusChats}
	return state
}

func promptFastPathState() State {
	state := InitialState()
	state.Focus = FocusAuth
	state.Prompt = &PromptState{
		Prompt:        auth.Prompt{ID: 41, Kind: auth.PromptPassword, Label: "Password", Secret: true},
		Input:         []rune("old"),
		PreviousFocus: FocusChats,
	}
	return withSentinelModal(state)
}

func photoFastPathState() State {
	state := InitialState()
	state.Focus = FocusPhotoSend
	state.PhotoSend = &PhotoSendState{
		ChatID:        domain.ChatID(9),
		TopicID:       domain.TopicID(3),
		Input:         []rune("before"),
		PreviousFocus: FocusComposer,
	}
	return withSentinelModal(state)
}

func messageSearchFastPathState(t *testing.T) State {
	t.Helper()
	state := openSearch(t, searchBaseState())
	state, _ = Reduce(state, MessageSearchValueChanged{ChatID: 9, Value: "needle"})
	state.MessageSearch.Results = []domain.Message{{ID: 100, ChatID: 9, Kind: domain.MessageText, Text: "new"}}
	state.MessageSearch.Error = &domain.AppError{Kind: domain.ErrorNetwork, Op: "search", Message: "boom"}
	return withSentinelModal(state)
}

func chatSearchFastPathState(t *testing.T) State {
	t.Helper()
	state, commands := Reduce(chatSearchBaseState(), ActionReceived{Action: OpenChatSearch})
	if len(commands) != 0 || state.ChatSearch == nil || state.Focus != FocusChatSearchInput {
		t.Fatalf("open chat search = %#v %#v", state.ChatSearch, commands)
	}
	return withSentinelModal(state)
}

func chatSettingsFastPathState(t *testing.T) State {
	t.Helper()
	state := InitialState()
	state.Focus = FocusDetails
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanChangeInfo: true, CanRestrictMembers: true}}
	var commands []Command
	state, commands = Reduce(state, ActionReceived{Action: OpenChatSettings})
	if state.ChatSettings == nil || len(commands) != 1 {
		t.Fatalf("open chat settings = %#v %#v", state.ChatSettings, commands)
	}
	state, _ = Reduce(state, ChatSettingsLoaded{
		RequestID: state.ChatSettings.RequestID,
		ChatID:    9,
		Snapshot: telegram.ChatSettings{
			Kind:          domain.ChatSupergroup,
			Title:         "Old title",
			Description:   "Old description",
			CanChangeInfo: true,
		},
	})
	state, _ = Reduce(state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingTitle})
	if state.Focus != FocusChatSettingsInput || state.ChatSettings.TitleEditorID == 0 {
		t.Fatalf("title editor: focus=%v %#v", state.Focus, state.ChatSettings)
	}
	state, _ = Reduce(state, ChatSettingsValueChanged{
		ChatID:   9,
		Field:    ChatSettingDescription,
		EditorID: state.ChatSettings.DescriptionEditorID,
		Value:    "typed description",
	})
	return withSentinelModal(state)
}

func TestPromptValueChangedFastPathClonesOnlyPrompt(t *testing.T) {
	input := promptFastPathState()
	pre := cloneReducerState(input)
	const value = "s界🙂cret"

	got, commands := Reduce(input, PromptValueChanged{PromptID: 41, Value: value})

	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	if got.Prompt == nil || string(got.Prompt.Input) != value {
		t.Fatalf("prompt input = %#v, want %q", got.Prompt, value)
	}
	if got.Prompt.Prompt != input.Prompt.Prompt || got.Prompt.PreviousFocus != input.Prompt.PreviousFocus {
		t.Fatalf("prompt identity changed: %#v", got.Prompt)
	}
	got.Prompt.Input[0] = 'Z'
	if string(input.Prompt.Input) != "old" {
		t.Fatalf("returned prompt aliases the caller's buffer: %q", string(input.Prompt.Input))
	}
	requireNarrowClone(t, "prompt", input, got, got.Prompt != input.Prompt)
	requireInputUnchanged(t, "prompt", input, pre)
}

func TestPromptValueChangedFastPathNoOpsStayNoOps(t *testing.T) {
	base := promptFastPathState()
	tests := []struct {
		name  string
		state State
		event PromptValueChanged
	}{
		{name: "wrong prompt ID", state: base, event: PromptValueChanged{PromptID: 99, Value: "new"}},
		{name: "zero prompt ID", state: base, event: PromptValueChanged{PromptID: 0, Value: "new"}},
		{name: "wrong focus", state: func() State { s := base; s.Focus = FocusConversation; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "no prompt", state: func() State { s := base; s.Prompt = nil; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
		{name: "quitting", state: func() State { s := base; s.Quitting = true; return s }(), event: PromptValueChanged{PromptID: 41, Value: "new"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pre := cloneReducerState(test.state)
			got, commands := Reduce(test.state, test.event)
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
			requireUnchangedResult(t, test.name, pre, got)
			if test.state.Prompt != nil && got.Prompt != test.state.Prompt {
				t.Fatal("no-op replaced the prompt overlay for nothing")
			}
			requireInputUnchanged(t, "prompt no-op", test.state, pre)
		})
	}
}

func TestPhotoPathValueChangedFastPathClonesOnlyPhotoSend(t *testing.T) {
	input := photoFastPathState()
	pre := cloneReducerState(input)

	got, commands := Reduce(input, PhotoPathValueChanged{ChatID: 9, Value: " /tmp/界🙂\nnext\rline "})

	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want none", commands)
	}
	if want := " /tmp/界🙂nextline "; string(got.PhotoSend.Input) != want {
		t.Fatalf("photo path = %q, want %q", string(got.PhotoSend.Input), want)
	}
	if got.PhotoSend.ChatID != domain.ChatID(9) || got.PhotoSend.TopicID != domain.TopicID(3) || got.PhotoSend.PreviousFocus != FocusComposer {
		t.Fatalf("photo-send identity changed: %#v", got.PhotoSend)
	}
	got.PhotoSend.Input[0] = 'Z'
	if string(input.PhotoSend.Input) != "before" {
		t.Fatalf("returned photo path aliases the caller's buffer: %q", string(input.PhotoSend.Input))
	}
	requireNarrowClone(t, "photo path", input, got, got.PhotoSend != input.PhotoSend)
	requireInputUnchanged(t, "photo path", input, pre)
}

func TestPhotoPathValueChangedFastPathNoOpsStayNoOps(t *testing.T) {
	base := photoFastPathState()
	tests := []struct {
		name  string
		state State
		event PhotoPathValueChanged
	}{
		{name: "zero chat ID", state: base, event: PhotoPathValueChanged{Value: "after"}},
		{name: "wrong chat ID", state: base, event: PhotoPathValueChanged{ChatID: 10, Value: "after"}},
		{name: "wrong focus", state: func() State { s := base; s.Focus = FocusComposer; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "no photo send", state: func() State { s := base; s.PhotoSend = nil; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
		{name: "quitting", state: func() State { s := base; s.Quitting = true; return s }(), event: PhotoPathValueChanged{ChatID: 9, Value: "after"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pre := cloneReducerState(test.state)
			got, commands := Reduce(test.state, test.event)
			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want none", commands)
			}
			requireUnchangedResult(t, test.name, pre, got)
			if test.state.PhotoSend != nil && got.PhotoSend != test.state.PhotoSend {
				t.Fatal("no-op replaced the photo-send overlay for nothing")
			}
			requireInputUnchanged(t, "photo path no-op", test.state, pre)
		})
	}
}

func TestSearchAndSettingsValueFastPathsOwnOnlyTheirOverlay(t *testing.T) {
	t.Run("message search", func(t *testing.T) {
		input := messageSearchFastPathState(t)
		pre := cloneReducerState(input)
		got, commands := Reduce(input, MessageSearchValueChanged{ChatID: 9, Value: "replacement"})
		if len(commands) != 0 || string(got.MessageSearch.Input) != "replacement" {
			t.Fatalf("message search result = %#v, commands=%#v", got.MessageSearch, commands)
		}
		if got.MessageSearch == input.MessageSearch || got.Modal != input.Modal {
			t.Fatal("message-search fast path cloned the wrong branch")
		}
		requireInputUnchanged(t, "message search", input, pre)
	})

	t.Run("chat search", func(t *testing.T) {
		input := chatSearchFastPathState(t)
		pre := cloneReducerState(input)
		got, commands := Reduce(input, ChatSearchValueChanged{Value: "beta"})
		if len(commands) != 2 || got.ChatSearch.Query != "beta" {
			t.Fatalf("chat search result = %#v, commands=%#v", got.ChatSearch, commands)
		}
		if got.ChatSearch == input.ChatSearch || got.Modal != input.Modal {
			t.Fatal("chat-search fast path cloned the wrong branch")
		}
		requireInputUnchanged(t, "chat search", input, pre)
	})

	t.Run("chat settings", func(t *testing.T) {
		input := InitialState()
		input.Focus = FocusChatSettingsInput
		input.ChatSettings = &ChatSettingsState{ChatID: 9, Mode: ChatSettingsTitleEditor, TitleEditorID: 41, TitleInput: []rune("old")}
		input = withSentinelModal(input)
		pre := cloneReducerState(input)
		got, commands := Reduce(input, ChatSettingsValueChanged{ChatID: 9, Field: ChatSettingTitle, EditorID: 41, Value: "new"})
		if len(commands) != 0 || string(got.ChatSettings.TitleInput) != "new" {
			t.Fatalf("chat settings result = %#v, commands=%#v", got.ChatSettings, commands)
		}
		if got.ChatSettings == input.ChatSettings || got.Modal != input.Modal {
			t.Fatal("chat-settings fast path cloned the wrong branch")
		}
		requireInputUnchanged(t, "chat settings", input, pre)
	})
}

func TestComposerValueFastPathOwnsDraftSyncSnapshots(t *testing.T) {
	input := selectedWritableState()
	input.Drafts[9] = "old"
	input.DraftSync[9] = DraftSyncState{RequestID: 77, Draft: domain.Draft{Text: "old"}}
	input.Chats[0].Draft = domain.Draft{Text: "old"}
	pre := cloneReducerState(input)

	got, commands := Reduce(input, ComposerValueChanged{ChatID: 9, Value: "new"})

	if len(commands) != 1 || got.DraftSync[9].Draft.Text != "new" || got.Chats[0].Draft.Text != "new" {
		t.Fatalf("composer result: sync=%#v chat=%#v commands=%#v", got.DraftSync[9], got.Chats[0], commands)
	}
	requireInputUnchanged(t, "composer draft metadata", input, pre)
}
