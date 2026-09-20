package frontend

import (
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestChatSettingsEditorFirstFrameShowsAuthoritativeText(t *testing.T) {
	model := ViewModel{Width: 100, Height: 30, Layout: ViewLayout{Mode: LayoutWide}, Focus: FocusChatSettingsInput,
		ChatSettings: &ChatSettingsState{ChatID: 9, Mode: ChatSettingsTitleEditor, TitleInput: []rune("Existing title")}}
	frame := composeApplication(model, time.Local, editorViews{})
	if !strings.Contains(frame.Content, "Existing title") {
		t.Fatal("first title-editor frame omitted the existing title")
	}
	model.ChatSettings.Mode = ChatSettingsDescriptionEditor
	model.ChatSettings.DescriptionInput = []rune("Existing description")
	frame = composeApplication(model, time.Local, editorViews{})
	if !strings.Contains(frame.Content, "Existing description") {
		t.Fatal("first description-editor frame omitted the existing description")
	}
}

func TestChatSettingsInputKeyboardAndModalMouseParity(t *testing.T) {
	for _, key := range []tea.Key{{Text: "j"}, {Code: tea.KeyDown}} {
		if got, ok := mapKeyPress(FocusChatSettings, tea.KeyPressMsg(key)); !ok || got.Action != SelectNext {
			t.Fatalf("next key %#v -> %#v %t", key, got, ok)
		}
	}
	if got, ok := mapKeyPress(FocusChatSettingsInput, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); !ok || got.Action != SaveChatSetting {
		t.Fatalf("enter -> %#v %t", got, ok)
	}
	model := ViewModel{Width: 100, Height: 30, Layout: ViewLayout{Mode: LayoutWide}, Focus: FocusChatSettings,
		ChatSettings: &ChatSettingsState{ChatID: 9, Mode: ChatSettingsMenu, Snapshot: &telegram.ChatSettings{Kind: domain.ChatSupergroup, CanChangeInfo: true, CanRestrictMembers: true}}}
	layer := buildChatSettingsLayer(model, newRenderStyles(true), "")
	found := false
	for _, hit := range layer.Interactions {
		if hit.Click.Action == EditChatSetting && hit.Click.SettingField == ChatSettingTitle {
			found = true
		}
	}
	if !found || layer.Rect.Empty() {
		t.Fatalf("title row missing: rect=%v interactions=%#v", layer.Rect, layer.Interactions)
	}
	if got := chatSettingsFrame(image.Rect(0, 0, 100, 30)); got.Dx() <= 0 || got.Dy() <= 0 {
		t.Fatalf("invalid frame %v", got)
	}
}
