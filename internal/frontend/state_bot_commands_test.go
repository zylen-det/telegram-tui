package frontend

import (
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
	state := botCommandState()
	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	if state.Focus != FocusComposer || state.CommandMenu == nil || !state.CommandMenu.Loading || state.Drafts[9] != "/" {
		t.Fatalf("slash transition = focus:%v menu:%#v draft:%q", state.Focus, state.CommandMenu, state.Drafts[9])
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	load, ok := commands[0].(LoadBotCommands)
	if !ok || load.ChatID != 9 || load.RequestID == 0 {
		t.Fatalf("load command = %#v", commands[0])
	}
}

func TestBotCommandMenuFiltersPrefixBeforeSubstringAndCompletesGroupTarget(t *testing.T) {
	state := botCommandState()
	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	catalog := []domain.BotCommand{
		{Name: "otherhelp", Description: "contains", BotUsername: "tools_bot"},
		{Name: "help", Description: "prefix", BotUsername: "tools_bot"},
		{Name: "hello", Description: "prefix two", BotUsername: "tools_bot"},
	}
	updateState(&state, BotCommandsLoaded{RequestID: request.RequestID, ChatID: 9, Commands: catalog})
	commands = updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/he"})
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
	updateState(&state, ActionReceived{Action: CommandMenuNext})
	commands = updateState(&state, ActionReceived{Action: CommandMenuActivate})
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
	updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/st"})
	updateState(&state, ActionReceived{Action: CommandMenuDismiss})
	if state.CommandMenu != nil || state.Drafts[9] != "/st" || state.Focus != FocusComposer {
		t.Fatalf("dismiss = %#v", state)
	}
	updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/start arg"})
	if state.CommandMenu != nil || state.Drafts[9] != "/start arg" {
		t.Fatalf("whitespace transition = menu:%#v draft:%q", state.CommandMenu, state.Drafts[9])
	}
}

func TestBotCommandMenuRejectsStaleResultsAndMouseIdentity(t *testing.T) {
	state := botCommandState()
	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	staleCommands := updateState(&state, BotCommandsLoaded{RequestID: request.RequestID + 1, ChatID: 9, Commands: []domain.BotCommand{{Name: "bad"}}})
	catalog := state.BotCommandCatalogs[9]
	if len(staleCommands) != 0 || !catalog.Loading || catalog.Loaded || len(catalog.Commands) != 0 || state.CommandMenu == nil || !state.CommandMenu.Loading {
		t.Fatalf("stale result changed catalog/menu: %#v %#v, effects=%#v", catalog, state.CommandMenu, staleCommands)
	}
	updateState(&state, BotCommandsLoaded{RequestID: request.RequestID, ChatID: 9, Commands: []domain.BotCommand{{Name: "start"}, {Name: "help"}}})
	wrongIdentityCommands := updateState(&state, ActionReceived{Action: CommandMenuActivate, ChatID: 99, CommandIndex: 1})
	if len(wrongIdentityCommands) != 0 || state.CommandMenu == nil || state.Drafts[9] != "/" {
		t.Fatalf("wrong chat activated command menu: menu=%#v draft=%q effects=%#v", state.CommandMenu, state.Drafts[9], wrongIdentityCommands)
	}
	updateState(&state, ActionReceived{Action: CommandMenuActivate, ChatID: 9, CommandIndex: 1})
	if state.CommandMenu != nil || state.Drafts[9] != "/start " {
		t.Fatalf("mouse completion = menu:%#v draft:%q", state.CommandMenu, state.Drafts[9])
	}
}

func TestBotCommandMenuFailureIsInlineAndEditInputNeverOpensIt(t *testing.T) {
	state := botCommandState()
	commands := updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	request := commands[0].(LoadBotCommands)
	updateState(&state, BotCommandsLoadFailed{RequestID: request.RequestID, ChatID: 9})
	if state.CommandMenu == nil || state.CommandMenu.Loading || state.CommandMenu.Error == nil || state.Toast != nil {
		t.Fatalf("failure state = menu:%#v toast:%#v", state.CommandMenu, state.Toast)
	}

	editing := botCommandState()
	editing.Messages[9] = []domain.Message{{ID: 7, ChatID: 9, Kind: domain.MessageText, Text: "old", Outgoing: true}}
	editing.EditTarget = &EditTarget{ChatID: 9, MessageID: 7, Original: "old", Buffer: "old"}
	editCommands := updateState(&editing, ComposerValueChanged{ChatID: 9, EditMessageID: 7, Value: "/start"})
	if editing.CommandMenu != nil || len(editCommands) != 0 || editing.EditTarget.Buffer != "/start" {
		t.Fatalf("edit transition = menu:%#v commands:%#v edit:%#v", editing.CommandMenu, editCommands, editing.EditTarget)
	}
}

func TestBotCommandMenuSelectionScrollsWithinEightRows(t *testing.T) {
	state := botCommandState()
	commands := make([]domain.BotCommand, 10)
	for index := range commands {
		commands[index] = domain.BotCommand{Name: string(rune('a' + index))}
	}
	state.BotCommandCatalogs[9] = BotCommandCatalogState{Loaded: true, Commands: commands}
	updateState(&state, ComposerValueChanged{ChatID: 9, Value: "/"})
	for range 9 {
		updateState(&state, ActionReceived{Action: CommandMenuNext})
	}
	if state.CommandMenu.Selected != 9 || state.CommandMenu.First != 2 {
		t.Fatalf("selection/window = %d/%d", state.CommandMenu.Selected, state.CommandMenu.First)
	}
	updateState(&state, ActionReceived{Action: CommandMenuNext})
	if state.CommandMenu.Selected != 9 {
		t.Fatalf("selection exceeded end: %d", state.CommandMenu.Selected)
	}
}
