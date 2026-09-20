package frontend

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func botCommandState() State {
	state := InitialState()
	state.Width = 100
	state.Height = 24
	state.Layout = LayoutNormal
	state.Focus = FocusComposer
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatPrivate, Title: "Bot", CanSend: true}}
	state.SelectedChat = 0
	return state
}

func TestBotCommandMenuLoadsWithoutTakingComposerFocus(t *testing.T) {
	input := botCommandState()
	got, commands := updateState(input, ComposerValueChanged{ChatID: 9, Value: "/"})
	if got.Focus != FocusComposer || got.CommandMenu == nil || !got.CommandMenu.Loading || got.Drafts[9] != "/" {
		t.Fatalf("slash transition = focus:%v menu:%#v draft:%q", got.Focus, got.CommandMenu, got.Drafts[9])
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	load, ok := commands[0].(LoadBotCommands)
	if !ok || load.ChatID != 9 || load.RequestID == 0 {
		t.Fatalf("load command = %#v", commands[0])
	}
	if len(input.BotCommandCatalogs) != 0 || input.Drafts[9] != "" {
		t.Fatal("hot-path update mutated its input state")
	}
}

func TestBotCommandMenuFiltersPrefixBeforeSubstringAndCompletesGroupTarget(t *testing.T) {
	state, commands := updateState(botCommandState(), ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	catalog := []domain.BotCommand{
		{Name: "otherhelp", Description: "contains", BotUsername: "tools_bot"},
		{Name: "help", Description: "prefix", BotUsername: "tools_bot"},
		{Name: "hello", Description: "prefix two", BotUsername: "tools_bot"},
	}
	state, _ = updateState(state, BotCommandsLoaded{RequestID: request.RequestID, ChatID: 9, Commands: catalog})
	state, commands = updateState(state, ComposerValueChanged{ChatID: 9, Value: "/he"})
	if len(commands) != 1 || state.CommandMenu == nil {
		t.Fatalf("filter transition = menu:%#v commands:%#v", state.CommandMenu, commands)
	}
	got := state.CommandMenu.Candidates
	want := []string{"/hello@tools_bot", "/help@tools_bot", "/otherhelp@tools_bot"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %#v", got)
	}
	for index := range want {
		if got[index].Invocation() != want[index] {
			t.Fatalf("candidate %d = %q, want %q", index, got[index].Invocation(), want[index])
		}
	}
	state, _ = updateState(state, ActionReceived{Action: CommandMenuNext})
	state, commands = updateState(state, ActionReceived{Action: CommandMenuActivate})
	if len(commands) != 1 {
		t.Fatalf("completion commands = %#v", commands)
	}
	if state.CommandMenu != nil || state.Focus != FocusComposer || state.Drafts[9] != "/help@tools_bot " {
		t.Fatalf("completion = focus:%v menu:%#v draft:%q", state.Focus, state.CommandMenu, state.Drafts[9])
	}
}

func TestBotCommandMenuDismissesWithoutChangingDraftAndClosesAfterWhitespace(t *testing.T) {
	state := botCommandState()
	state.BotCommandCatalogs[9] = BotCommandCatalogState{Loaded: true, Commands: []domain.BotCommand{{Name: "start"}}}
	state, _ = updateState(state, ComposerValueChanged{ChatID: 9, Value: "/st"})
	state, _ = updateState(state, ActionReceived{Action: CommandMenuDismiss})
	if state.CommandMenu != nil || state.Drafts[9] != "/st" || state.Focus != FocusComposer {
		t.Fatalf("dismiss = %#v", state)
	}
	state, _ = updateState(state, ComposerValueChanged{ChatID: 9, Value: "/start arg"})
	if state.CommandMenu != nil || state.Drafts[9] != "/start arg" {
		t.Fatalf("whitespace transition = menu:%#v draft:%q", state.CommandMenu, state.Drafts[9])
	}
}

func TestBotCommandMenuRejectsStaleResultsAndMouseIdentity(t *testing.T) {
	state, commands := updateState(botCommandState(), ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	stale, _ := updateState(state, BotCommandsLoaded{RequestID: request.RequestID + 1, ChatID: 9, Commands: []domain.BotCommand{{Name: "bad"}}})
	if !reflect.DeepEqual(stale, state) {
		t.Fatal("stale command result changed state")
	}
	state, _ = updateState(state, BotCommandsLoaded{RequestID: request.RequestID, ChatID: 9, Commands: []domain.BotCommand{{Name: "start"}, {Name: "help"}}})
	unchanged, _ := updateState(state, ActionReceived{Action: CommandMenuActivate, ChatID: 99, CommandIndex: 1})
	if !reflect.DeepEqual(unchanged, state) {
		t.Fatal("stale mouse identity changed state")
	}
	state, _ = updateState(state, ActionReceived{Action: CommandMenuActivate, ChatID: 9, CommandIndex: 1})
	if state.Drafts[9] != "/start " {
		t.Fatalf("mouse completion draft = %q", state.Drafts[9])
	}
}

func TestBotCommandMenuFailureIsInlineAndEditInputNeverOpensIt(t *testing.T) {
	state, commands := updateState(botCommandState(), ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	state, _ = updateState(state, BotCommandsLoadFailed{RequestID: request.RequestID, ChatID: 9})
	if state.CommandMenu == nil || state.CommandMenu.Loading || state.CommandMenu.Error == nil || state.Toast != nil {
		t.Fatalf("failure state = menu:%#v toast:%#v", state.CommandMenu, state.Toast)
	}

	editing := botCommandState()
	editing.Messages[9] = []domain.Message{{ID: 7, ChatID: 9, Kind: domain.MessageText, Text: "old", Outgoing: true}}
	editing.EditTarget = &EditTarget{ChatID: 9, MessageID: 7, Original: "old", Buffer: "old"}
	got, editCommands := updateState(editing, ComposerValueChanged{ChatID: 9, EditMessageID: 7, Value: "/start"})
	if got.CommandMenu != nil || len(editCommands) != 0 || got.EditTarget.Buffer != "/start" {
		t.Fatalf("edit transition = menu:%#v commands:%#v edit:%#v", got.CommandMenu, editCommands, got.EditTarget)
	}
}

func TestBotCommandMenuSelectionScrollsWithinEightRows(t *testing.T) {
	state := botCommandState()
	commands := make([]domain.BotCommand, 10)
	for index := range commands {
		commands[index] = domain.BotCommand{Name: string(rune('a' + index))}
	}
	state.BotCommandCatalogs[9] = BotCommandCatalogState{Loaded: true, Commands: commands}
	state, _ = updateState(state, ComposerValueChanged{ChatID: 9, Value: "/"})
	for range 9 {
		state, _ = updateState(state, ActionReceived{Action: CommandMenuNext})
	}
	if state.CommandMenu.Selected != 9 || state.CommandMenu.First != 2 {
		t.Fatalf("selection/window = %d/%d", state.CommandMenu.Selected, state.CommandMenu.First)
	}
	state, _ = updateState(state, ActionReceived{Action: CommandMenuNext})
	if state.CommandMenu.Selected != 9 {
		t.Fatalf("selection exceeded end: %d", state.CommandMenu.Selected)
	}
}
