package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type ChatSettingsMode uint8

const (
	ChatSettingsMenu ChatSettingsMode = iota
	ChatSettingsTitleEditor
	ChatSettingsDescriptionEditor
	ChatSettingsSlowModeMenu
)

type ChatSettingsState struct {
	RequestID           uint64
	ChatID              domain.ChatID
	PreviousFocus       Focus
	Mode                ChatSettingsMode
	Selected            int
	Loading             bool
	Working             bool
	Error               *domain.AppError
	Snapshot            *telegram.ChatSettings
	TitleInput          []rune
	DescriptionInput    []rune
	TitleEditorID       uint64
	DescriptionEditorID uint64
	PendingField        ChatSettingField
	PendingValue        string
	PendingDelay        int
	Notice              string
}
type ChatSettingsMenuItem struct {
	Label  string
	Action ActionReceived
	Header bool
}

func chatSettingsAction(s *ChatSettingsState, action Action) ActionReceived {
	return ActionReceived{Action: action, ChatID: s.ChatID}
}
func ChatSettingsMenuItems(s *ChatSettingsState) []ChatSettingsMenuItem {
	if s == nil || s.Loading || s.Working {
		return nil
	}
	if s.Error != nil {
		return []ChatSettingsMenuItem{{Label: "Retry", Action: chatSettingsAction(s, Retry)}}
	}
	switch s.Mode {
	case ChatSettingsTitleEditor, ChatSettingsDescriptionEditor:
		return []ChatSettingsMenuItem{{Label: "Save", Action: chatSettingsAction(s, SaveChatSetting)}, {Label: "Cancel", Action: chatSettingsAction(s, Close)}}
	case ChatSettingsSlowModeMenu:
		vals := []int{0, 5, 10, 30, 60, 300, 900, 3600}
		labels := []string{"Off", "5 seconds", "10 seconds", "30 seconds", "1 minute", "5 minutes", "15 minutes", "1 hour"}
		out := make([]ChatSettingsMenuItem, len(vals))
		for i, v := range vals {
			a := chatSettingsAction(s, SelectChatSlowMode)
			a.SlowModeDelay = v
			out[i] = ChatSettingsMenuItem{Label: labels[i], Action: a}
		}
		return out
	default:
		if s.Snapshot == nil {
			return nil
		}
		out := []ChatSettingsMenuItem{}
		if s.Snapshot.CanChangeInfo {
			a := chatSettingsAction(s, EditChatSetting)
			a.SettingField = ChatSettingTitle
			out = append(out, ChatSettingsMenuItem{Label: "Edit title", Action: a}, ChatSettingsMenuItem{Label: "Edit description", Action: func() ActionReceived {
				a := chatSettingsAction(s, EditChatSetting)
				a.SettingField = ChatSettingDescription
				return a
			}()})
		}
		if s.Snapshot.Kind == domain.ChatSupergroup && s.Snapshot.CanRestrictMembers {
			a := chatSettingsAction(s, EditChatSetting)
			a.SettingField = ChatSettingSlowMode
			out = append(out, ChatSettingsMenuItem{Label: "Slow mode", Action: a})
		}
		return out
	}
}

func openChatSettings(state *State) []Effect {
	chat, ok := detailsChat(*state)
	if state.Focus != FocusDetails || !ok {
		return nil
	}
	if chat.Kind != domain.ChatBasicGroup && chat.Kind != domain.ChatSupergroup && chat.Kind != domain.ChatChannel {
		return nil
	}
	if !chat.CanChangeInfo && !(chat.Kind == domain.ChatSupergroup && chat.CanRestrictMembers) {
		return nil
	}
	id := allocateRequestID(state)
	state.ChatSettings = &ChatSettingsState{RequestID: id, ChatID: chat.ID, PreviousFocus: FocusDetails, Loading: true}
	state.Focus = FocusChatSettings
	items := DetailsActionItems(chat)
	for i := range items {
		if items[i].Action == OpenChatSettings {
			state.DetailsSelected = i
		}
	}
	return []Effect{LoadChatSettingsCommand{RequestID: id, ChatID: chat.ID}}
}

func beginChatSettingEditor(state *State, field ChatSettingField) []Effect {
	s := state.ChatSettings
	if s == nil || s.Snapshot == nil {
		return nil
	}
	if field != ChatSettingTitle && field != ChatSettingDescription && field != ChatSettingSlowMode {
		return nil
	}
	if field == ChatSettingTitle && !s.Snapshot.CanChangeInfo || field == ChatSettingDescription && !s.Snapshot.CanChangeInfo || field == ChatSettingSlowMode && !(s.Snapshot.Kind == domain.ChatSupergroup && s.Snapshot.CanRestrictMembers) {
		return nil
	}
	s.PendingField = field
	s.Notice = ""
	s.Error = nil
	s.Selected = 0
	if field == ChatSettingSlowMode {
		s.Mode = ChatSettingsSlowModeMenu
		return nil
	}
	s.Mode = ChatSettingsTitleEditor
	if field == ChatSettingDescription {
		s.Mode = ChatSettingsDescriptionEditor
	}
	state.NextEditorID++
	if field == ChatSettingTitle {
		s.TitleInput = []rune(s.Snapshot.Title)
		s.TitleEditorID = state.NextEditorID
	} else {
		s.DescriptionInput = []rune(s.Snapshot.Description)
		s.DescriptionEditorID = state.NextEditorID
	}
	state.Focus = FocusChatSettingsInput
	return nil
}
func chatSettingSave(state *State) []Effect {
	s := state.ChatSettings
	if s == nil || s.Snapshot == nil || s.Working {
		return nil
	}
	value := string(s.TitleInput)
	if s.Mode == ChatSettingsDescriptionEditor {
		value = string(s.DescriptionInput)
	}
	if s.Mode == ChatSettingsTitleEditor {
		if len([]rune(value)) < 1 || len([]rune(value)) > 128 {
			s.Notice = "Title must be 1–128 characters"
			return nil
		}
		if value == s.Snapshot.Title {
			s.Mode = ChatSettingsMenu
			state.Focus = FocusChatSettings
			return nil
		}
	}
	if s.Mode == ChatSettingsDescriptionEditor {
		if len([]rune(value)) > 255 {
			s.Notice = "Description must be at most 255 characters"
			return nil
		}
		if value == s.Snapshot.Description {
			s.Mode = ChatSettingsMenu
			state.Focus = FocusChatSettings
			return nil
		}
	}
	id := allocateRequestID(state)
	s.RequestID = id
	s.Working = true
	s.PendingValue = value
	s.Notice = ""
	return []Effect{SaveChatSettingCommand{RequestID: id, ChatID: s.ChatID, Field: s.PendingField, Value: value}}
}
func reduceChatSettingsAction(state *State, e ActionReceived) []Effect {
	s := state.ChatSettings
	if s == nil || state.Focus != FocusChatSettings && state.Focus != FocusChatSettingsInput {
		return nil
	}
	if e.ChatID != 0 && e.ChatID != s.ChatID {
		return nil
	}
	if e.Action == Close {
		if s.Working {
			state.ChatSettings = nil
			state.Focus = s.PreviousFocus
			return nil
		}
		if s.Mode != ChatSettingsMenu {
			s.Mode = ChatSettingsMenu
			state.Focus = FocusChatSettings
			return nil
		}
		state.ChatSettings = nil
		state.Focus = s.PreviousFocus
		return nil
	}
	if s.Loading || s.Working {
		return nil
	}
	if s.Error != nil {
		if e.Action == Retry || e.Action == Activate {
			id := allocateRequestID(state)
			s.RequestID = id
			s.Loading = true
			s.Error = nil
			return []Effect{LoadChatSettingsCommand{RequestID: id, ChatID: s.ChatID}}
		}
		return nil
	}
	if state.Focus == FocusChatSettingsInput && e.Action == NoAction && e.Rune != 0 {
		if s.Mode == ChatSettingsTitleEditor {
			s.TitleInput = append(s.TitleInput, e.Rune)
		} else if s.Mode == ChatSettingsDescriptionEditor {
			s.DescriptionInput = append(s.DescriptionInput, e.Rune)
		}
		return nil
	}
	items := ChatSettingsMenuItems(s)
	switch e.Action {
	case SelectNext, SelectPrevious:
		if len(items) > 0 {
			if e.Action == SelectNext {
				s.Selected = (s.Selected + 1) % len(items)
			} else {
				s.Selected = (s.Selected + len(items) - 1) % len(items)
			}
		}
	case Activate:
		if s.Selected >= 0 && s.Selected < len(items) {
			return reduceChatSettingsAction(state, items[s.Selected].Action)
		}
	case EditChatSetting:
		return beginChatSettingEditor(state, e.SettingField)
	case SelectChatSlowMode:
		if s.Mode != ChatSettingsSlowModeMenu || s.Snapshot.Kind != domain.ChatSupergroup || !s.Snapshot.CanRestrictMembers || !validChatSlowMode(e.SlowModeDelay) {
			return nil
		}
		if e.SlowModeDelay == s.Snapshot.SlowModeDelay {
			s.Mode = ChatSettingsMenu
			return nil
		}
		id := allocateRequestID(state)
		s.RequestID = id
		s.PendingField = ChatSettingSlowMode
		s.PendingDelay = e.SlowModeDelay
		s.Working = true
		return []Effect{SaveChatSettingCommand{RequestID: id, ChatID: s.ChatID, Field: ChatSettingSlowMode, Delay: e.SlowModeDelay}}
	case SaveChatSetting:
		if s.Mode != ChatSettingsTitleEditor && s.Mode != ChatSettingsDescriptionEditor {
			return nil
		}
		return chatSettingSave(state)
	}
	return nil
}
func validChatSlowMode(v int) bool {
	switch v {
	case 0, 5, 10, 30, 60, 300, 900, 3600:
		return true
	}
	return false
}

func chatSettingsActive(state State, id domain.ChatID) bool {
	return id != 0 && chatIndex(state.Chats, id) >= 0
}
func reduceChatSettingsLoaded(state *State, e ChatSettingsLoaded) []Effect {
	s := state.ChatSettings
	if s != nil && chatSettingsActive(*state, e.ChatID) && s.RequestID == e.RequestID && s.ChatID == e.ChatID && s.Loading && state.Focus == FocusChatSettings {
		s.Loading = false
		v := e.Snapshot
		s.Snapshot = &v
		s.Mode = ChatSettingsMenu
		s.Selected = 0
	}
	return nil
}
func reduceChatSettingsLoadFailed(state *State, e ChatSettingsLoadFailed) []Effect {
	s := state.ChatSettings
	if s != nil && chatSettingsActive(*state, e.ChatID) && s.RequestID == e.RequestID && s.ChatID == e.ChatID && s.Loading {
		s.Loading = false
		s.Error = &domain.AppError{Kind: domain.ErrorNetwork, Op: "load chat settings", Message: "Could not load chat settings"}
	}
	return nil
}
func reduceChatSettingSaved(state *State, e ChatSettingSaved) []Effect {
	s := state.ChatSettings
	if s == nil || !chatSettingsActive(*state, e.ChatID) || s.RequestID != e.RequestID || s.ChatID != e.ChatID || s.PendingField != e.Field || !s.Working {
		return nil
	}
	s.Working = false
	if e.Field == ChatSettingTitle {
		s.Snapshot.Title = s.PendingValue
		s.Notice = "Chat title saved"
		if i := chatIndex(state.Chats, s.ChatID); i >= 0 {
			state.Chats[i].Title = s.PendingValue
		}
	} else if e.Field == ChatSettingDescription {
		s.Snapshot.Description = s.PendingValue
		s.Notice = "Chat description saved"
	} else {
		s.Snapshot.SlowModeDelay = s.PendingDelay
		s.Notice = "Slow mode saved"
	}
	s.Mode = ChatSettingsMenu
	state.Focus = FocusChatSettings
	return nil
}
func reduceChatSettingSaveFailed(state *State, e ChatSettingSaveFailed) []Effect {
	s := state.ChatSettings
	if s != nil && chatSettingsActive(*state, e.ChatID) && s.RequestID == e.RequestID && s.ChatID == e.ChatID && s.PendingField == e.Field && s.Working {
		s.Working = false
		switch e.Field {
		case ChatSettingTitle:
			s.Notice = "Could not save chat title"
		case ChatSettingDescription:
			s.Notice = "Could not save chat description"
		default:
			s.Notice = "Could not save slow mode"
		}
	}
	return nil
}
func reduceChatSettingsValueChanged(state *State, e ChatSettingsValueChanged) []Effect {
	active := state.ChatSettings
	if active == nil || active.Working || state.Focus != FocusChatSettingsInput || active.ChatID != e.ChatID {
		return nil
	}
	s := active
	if e.Field == ChatSettingTitle && s.Mode == ChatSettingsTitleEditor && s.TitleEditorID == e.EditorID {
		s.TitleInput = []rune(e.Value)
	} else if e.Field == ChatSettingDescription && s.Mode == ChatSettingsDescriptionEditor && s.DescriptionEditorID == e.EditorID {
		s.DescriptionInput = []rune(e.Value)
	} else {
		return nil
	}
	return nil
}
