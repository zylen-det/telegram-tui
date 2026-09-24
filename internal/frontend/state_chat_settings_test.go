package frontend

import (
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func chatSettingsBase() State {
	return State{Focus: FocusDetails, SelectedChat: 0, Chats: []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, CanChangeInfo: true, CanRestrictMembers: true}}}
}

func TestActivateSelectedChatSettingsRowOpensSettings(t *testing.T) {
	state := chatSettingsBase()
	items := DetailsActionItems(state.Chats[0])
	for index, item := range items {
		if item.Action == OpenChatSettings {
			state.DetailsSelected = index
		}
	}
	commands := updateState(&state, ActionReceived{Action: Activate})
	if state.ChatSettings == nil || state.Modal != nil || state.Focus != FocusChatSettings || len(commands) != 1 {
		t.Fatalf("activate opened wrong surface: focus=%d settings=%#v modal=%#v commands=%#v", state.Focus, state.ChatSettings, state.Modal, commands)
	}
}

func TestChatSettingsLoadEditSaveAndStaleResults(t *testing.T) {
	state := chatSettingsBase()
	commands := updateState(&state, ActionReceived{Action: OpenChatSettings})
	if state.ChatSettings == nil || len(commands) != 1 || state.Focus != FocusChatSettings {
		t.Fatalf("open settings: %#v %#v", state.ChatSettings, commands)
	}
	request := state.ChatSettings.RequestID
	updateState(&state, ChatSettingsLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.ChatSettings{Kind: domain.ChatSupergroup, Title: "Old", Description: "About", SlowModeDelay: 30, CanChangeInfo: true, CanRestrictMembers: true}})
	updateState(&state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingTitle})
	if state.Focus != FocusChatSettingsInput || string(state.ChatSettings.TitleInput) != "Old" || state.ChatSettings.TitleEditorID == 0 {
		t.Fatalf("title editor: %#v", state.ChatSettings)
	}
	editor := state.ChatSettings.TitleEditorID
	updateState(&state, ChatSettingsValueChanged{ChatID: 9, Field: ChatSettingTitle, EditorID: editor, Value: "New"})
	commands = updateState(&state, ActionReceived{Action: SaveChatSetting, ChatID: 9})
	if len(commands) != 1 || !state.ChatSettings.Working || state.ChatSettings.PendingField != ChatSettingTitle {
		t.Fatalf("title save: %#v %#v", state.ChatSettings, commands)
	}
	saveRequest := state.ChatSettings.RequestID
	updateState(&state, ChatSettingSaved{RequestID: saveRequest, ChatID: 9, Field: ChatSettingDescription})
	if !state.ChatSettings.Working || state.Chats[0].Title != "" {
		t.Fatalf("wrong-field success was accepted: working=%t title=%q pending=%d", state.ChatSettings.Working, state.Chats[0].Title, state.ChatSettings.PendingField)
	}
	updateState(&state, ChatSettingSaved{RequestID: saveRequest, ChatID: 9, Field: ChatSettingTitle})
	if state.ChatSettings.Working || state.Chats[0].Title != "New" || state.ChatSettings.Notice != "Chat title saved" {
		t.Fatalf("title success: %#v", state)
	}
}

func TestChatSettingsValidationAndSlowModeChoices(t *testing.T) {
	state := chatSettingsBase()
	updateState(&state, ActionReceived{Action: OpenChatSettings})
	request := state.ChatSettings.RequestID
	updateState(&state, ChatSettingsLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.ChatSettings{Kind: domain.ChatSupergroup, Title: "Old", CanChangeInfo: true, CanRestrictMembers: true, SlowModeDelay: 30}})
	updateState(&state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingTitle})
	state.ChatSettings.TitleInput = []rune(strings.Repeat("x", 129))
	commands := updateState(&state, ActionReceived{Action: SaveChatSetting, ChatID: 9})
	if len(commands) != 0 || state.ChatSettings.Notice != "Title must be 1–128 characters" {
		t.Fatalf("title validation: %#v %#v", state.ChatSettings, commands)
	}
	updateState(&state, ActionReceived{Action: Close, ChatID: 9})
	updateState(&state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingSlowMode})
	items := ChatSettingsMenuItems(state.ChatSettings)
	if len(items) != 8 || items[0].Label != "Off" || items[7].Label != "1 hour" {
		t.Fatalf("slow mode choices: %#v", items)
	}
	commands = updateState(&state, ActionReceived{Action: SelectChatSlowMode, ChatID: 9, SlowModeDelay: 300})
	if len(commands) != 1 || !state.ChatSettings.Working || state.ChatSettings.PendingDelay != 300 {
		t.Fatalf("slow mode save: %#v %#v", state.ChatSettings, commands)
	}
}

func TestChatSettingsTitleAndDescriptionKeepSeparateEditorState(t *testing.T) {
	state := chatSettingsBase()
	updateState(&state, ActionReceived{Action: OpenChatSettings})
	request := state.ChatSettings.RequestID
	updateState(&state, ChatSettingsLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.ChatSettings{Kind: domain.ChatSupergroup, Title: "Old title", Description: "Old description", CanChangeInfo: true}})
	updateState(&state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingTitle})
	titleEditor := state.ChatSettings.TitleEditorID
	updateState(&state, ChatSettingsValueChanged{ChatID: 9, Field: ChatSettingTitle, EditorID: titleEditor, Value: "Draft title"})
	updateState(&state, ActionReceived{Action: Close, ChatID: 9})
	updateState(&state, ActionReceived{Action: EditChatSetting, ChatID: 9, SettingField: ChatSettingDescription})
	if got := string(state.ChatSettings.DescriptionInput); got != "Old description" {
		t.Fatalf("description input = %q, want loaded description", got)
	}
	if got := string(state.ChatSettings.TitleInput); got != "Draft title" {
		t.Fatalf("title input was overwritten by description editor: %q", got)
	}
	before := string(state.ChatSettings.DescriptionInput)
	updateState(&state, ChatSettingsValueChanged{ChatID: 9, Field: ChatSettingTitle, EditorID: titleEditor, Value: "stale"})
	if got := string(state.ChatSettings.DescriptionInput); got != before {
		t.Fatalf("stale title event changed description: %q", got)
	}
}

func TestChatSettingsFailureRetryAndCloseDiscardLateResults(t *testing.T) {
	state := chatSettingsBase()
	updateState(&state, ActionReceived{Action: OpenChatSettings})
	request := state.ChatSettings.RequestID
	updateState(&state, ChatSettingsLoadFailed{RequestID: request, ChatID: 9})
	if state.ChatSettings.Error == nil || len(ChatSettingsMenuItems(state.ChatSettings)) != 1 {
		t.Fatal("load failure did not expose retry")
	}
	commands := updateState(&state, ActionReceived{Action: Retry, ChatID: 9})
	if len(commands) != 1 || !state.ChatSettings.Loading || state.ChatSettings.RequestID == request {
		t.Fatal("retry did not start")
	}
	updateState(&state, ActionReceived{Action: Close, ChatID: 9})
	if state.ChatSettings != nil || state.Focus != FocusDetails {
		t.Fatal("close did not discard failed modal")
	}
	updateState(&state, ChatSettingsLoaded{RequestID: request, ChatID: 9, Snapshot: telegram.ChatSettings{Title: "late"}})
	if state.ChatSettings != nil {
		t.Fatal("late result recreated settings")
	}
}
