package app

import (
	"sort"
	"strings"
	"unicode"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func requestBotCommandsIfAbsent(state *State, chatID domain.ChatID) []Command {
	if _, exists := state.BotCommandCatalogs[chatID]; exists {
		return nil
	}
	catalogs := make(map[domain.ChatID]BotCommandCatalogState, len(state.BotCommandCatalogs)+1)
	for existingChatID, catalog := range state.BotCommandCatalogs {
		catalogs[existingChatID] = catalog
	}
	requestID := allocateRequestID(state)
	catalogs[chatID] = BotCommandCatalogState{RequestID: requestID, Loading: true}
	state.BotCommandCatalogs = catalogs
	return []Command{LoadBotCommands{RequestID: requestID, ChatID: chatID}}
}

func syncCommandMenuForValue(state *State, chatID domain.ChatID, value string) []Command {
	query, active := botCommandQuery(value)
	if !active || state.EditTarget != nil {
		state.CommandMenu = nil
		return nil
	}

	catalog, exists := state.BotCommandCatalogs[chatID]
	if !exists {
		commands := requestBotCommandsIfAbsent(state, chatID)
		catalog = state.BotCommandCatalogs[chatID]
		state.CommandMenu = &CommandMenuState{ChatID: chatID, Query: query, Selected: -1, Loading: true}
		return commands
	}

	menu := &CommandMenuState{
		ChatID:   chatID,
		Query:    query,
		Selected: -1,
		Loading:  catalog.Loading,
		Error:    cloneDomainError(catalog.Error),
	}
	if catalog.Loaded {
		menu.Candidates = filterBotCommands(query, catalog.Commands)
		if len(menu.Candidates) == 0 {
			state.CommandMenu = nil
			return nil
		}
		menu.Selected = 0
	}
	state.CommandMenu = menu
	return nil
}

func botCommandQuery(value string) (string, bool) {
	if !strings.HasPrefix(value, "/") {
		return "", false
	}
	query := value[1:]
	if strings.IndexFunc(query, unicode.IsSpace) >= 0 {
		return "", false
	}
	return query, true
}

func filterBotCommands(query string, commands []domain.BotCommand) []domain.BotCommand {
	needle := strings.ToLower(query)
	matches := make([]domain.BotCommand, 0, len(commands))
	for _, command := range commands {
		candidate := strings.ToLower(strings.TrimPrefix(command.Invocation(), "/"))
		if needle == "" || strings.Contains(candidate, needle) {
			matches = append(matches, command)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := strings.ToLower(strings.TrimPrefix(matches[i].Invocation(), "/"))
		right := strings.ToLower(strings.TrimPrefix(matches[j].Invocation(), "/"))
		leftPrefix := needle == "" || strings.HasPrefix(left, needle)
		rightPrefix := needle == "" || strings.HasPrefix(right, needle)
		if leftPrefix != rightPrefix {
			return leftPrefix
		}
		return left < right
	})
	return matches
}

func reduceBotCommandsLoaded(state State, event BotCommandsLoaded) (State, []Command) {
	catalog, exists := state.BotCommandCatalogs[event.ChatID]
	if !exists || catalog.RequestID != event.RequestID {
		return state, nil
	}
	catalog.Loading = false
	catalog.Loaded = true
	catalog.Commands = append([]domain.BotCommand(nil), event.Commands...)
	catalog.Error = nil
	state.BotCommandCatalogs[event.ChatID] = catalog
	if activeID, active := activeChatID(state); active && activeID == event.ChatID && state.EditTarget == nil {
		return state, syncCommandMenuForValue(&state, event.ChatID, state.Drafts[event.ChatID])
	}
	return state, nil
}

func reduceBotCommandsLoadFailed(state State, event BotCommandsLoadFailed) (State, []Command) {
	catalog, exists := state.BotCommandCatalogs[event.ChatID]
	if !exists || catalog.RequestID != event.RequestID {
		return state, nil
	}
	failure := domain.AppError{Kind: domain.ErrorInternal, Op: "load bot commands", Message: "Commands unavailable"}
	catalog.Loading = false
	catalog.Loaded = false
	catalog.Error = &failure
	state.BotCommandCatalogs[event.ChatID] = catalog
	if state.CommandMenu != nil && state.CommandMenu.ChatID == event.ChatID {
		menu := *state.CommandMenu
		menu.Loading = false
		menu.Error = &failure
		state.CommandMenu = &menu
	}
	return state, nil
}

func reduceCommandMenuAction(state State, event ActionReceived) (State, []Command) {
	if state.CommandMenu == nil {
		return state, nil
	}
	menu := *state.CommandMenu
	switch event.Action {
	case CommandMenuDismiss:
		state.CommandMenu = nil
	case CommandMenuNext:
		if menu.Selected >= 0 && menu.Selected+1 < len(menu.Candidates) {
			menu.Selected++
			ensureCommandMenuSelectionVisible(&menu)
			state.CommandMenu = &menu
		}
	case CommandMenuPrevious:
		if menu.Selected > 0 {
			menu.Selected--
			ensureCommandMenuSelectionVisible(&menu)
			state.CommandMenu = &menu
		}
	case CommandMenuActivate:
		index := menu.Selected
		if event.ChatID != 0 {
			if event.ChatID != menu.ChatID {
				return state, nil
			}
			index = event.CommandIndex
		}
		activeID, active := activeChatID(state)
		if !active || activeID != menu.ChatID || state.EditTarget != nil || index < 0 || index >= len(menu.Candidates) {
			return state, nil
		}
		drafts := make(map[domain.ChatID]string, len(state.Drafts)+1)
		for chatID, draft := range state.Drafts {
			drafts[chatID] = draft
		}
		drafts[menu.ChatID] = menu.Candidates[index].Invocation() + " "
		state.Drafts = drafts
		state.CommandMenu = nil
		return state, []Command{queueDraftSave(&state, menu.ChatID)}
	}
	return state, nil
}

func ensureCommandMenuSelectionVisible(menu *CommandMenuState) {
	if menu.Selected < menu.First {
		menu.First = menu.Selected
	}
	if menu.Selected >= menu.First+CommandMenuVisibleRows {
		menu.First = menu.Selected - CommandMenuVisibleRows + 1
	}
	maxFirst := max(0, len(menu.Candidates)-CommandMenuVisibleRows)
	menu.First = max(0, min(menu.First, maxFirst))
}
