package frontend

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestAppModelHuhComposerOwnsPersistentHostAndSingleSnapshot(t *testing.T) {
	model := newAppModelForTest(t, InitialState(), newTestSession(t))
	if model.composerText == nil {
		t.Fatal("NewAppModel composerText host is nil")
	}
	copied := model
	if copied.composerText != model.composerText {
		t.Fatal("AppModel value copy replaced the persistent composer host")
	}
	other := newAppModelForTest(t, InitialState(), newTestSession(t))
	if other.composerText == model.composerText {
		t.Fatal("independent AppModels share one composer host")
	}

	source, err := os.ReadFile("app_model.go")
	if err != nil {
		t.Fatalf("read app_model.go: %v", err)
	}
	body := huhComposerFunctionBody(string(source), "func (m AppModel) syncComposerTextHost()")
	if got, want := strings.Count(body, "m.Snapshot()"), 1; got != want {
		t.Fatalf("syncComposerTextHost snapshot calls = %d, want %d\n%s", got, want, body)
	}
}

func TestAppModelHuhComposerSynchronizesAuthoritativeIdentity(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}, {ID: 10, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	state.Width = 100
	state.Height = 24
	state.Drafts[9] = "draft界"
	state.Drafts[10] = "other"
	model := newAppModelForTest(t, state, newTestSession(t))

	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft界", true, 42, 2)

	state = model.Snapshot()
	state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "old"}}
	state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Original: "old", Buffer: "edit🙂"}
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9, EditMessageID: 22}, "edit🙂", true, 42, 2)

	state = model.Snapshot()
	state.SelectedChat = 1
	state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "stale"}
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 10}, "other", true, 42, 2)
	if strings.Contains(model.composerText.View(), "stale") {
		t.Fatal("stale edit value survived chat identity change")
	}
}

func TestAppModelHuhComposerRoutesEditingMessages(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	model := newAppModelForTest(t, state, newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a"}))
	assertHuhComposerValue(t, model, "a")
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "界🙂"})
	assertHuhComposerValue(t, model, "a界🙂")
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	assertHuhComposerValue(t, model, "a界")
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}))
	assertHuhComposerValue(t, model, "a界\n")

	state = model.Snapshot()
	state.Drafts[9] = "draft-stays"
	state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "old"}}
	state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Original: "old", Buffer: "edit"}
	model.state = &state
	_ = model.syncComposerTextHost()
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "X"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "界"})
	snapshot := model.Snapshot()
	if got, want := snapshot.EditTarget.Buffer, "editX界"; got != want {
		t.Fatalf("edit buffer = %q, want %q", got, want)
	}
	if got, want := snapshot.Drafts[9], "draft-stays"; got != want {
		t.Fatalf("draft changed during edit: %q, want %q", got, want)
	}
	if got, want := model.composerText.Identity(), (composerTextIdentity{ChatID: 9, EditMessageID: 22}); got != want {
		t.Fatalf("edit identity = %#v, want %#v", got, want)
	}
}

func TestAppModelHuhComposerAppliesCleanRemoteDraftImmediately(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	state.Width, state.Height = 100, 24
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncComposerTextHost()

	model, _ = updateAppModel(t, model, TelegramEvent{Value: telegram.DraftChanged{
		ChatID: 9,
		Draft:  domain.Draft{Text: "remote cloud draft", Date: 123},
	}})
	if got := model.Snapshot().Drafts[9]; got != "remote cloud draft" {
		t.Fatalf("authoritative remote draft = %q", got)
	}
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "remote cloud draft", true, 42, 2)
}

func TestAppModelHuhComposerPreservesReservedAndModifiedKeys(t *testing.T) {
	state := InitialState()
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	state.Drafts[9] = "keep"
	model := newAppModelForTest(t, state, newTestSession(t))

	for _, key := range []tea.Key{
		{Code: tea.KeyBackspace, Mod: tea.ModAlt},
		{Code: 'q', Text: "q", Mod: tea.ModMeta},
		{Code: 'u', Text: "u", Mod: tea.ModCtrl},
	} {
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(key))
	}
	if got, want := model.Snapshot().Drafts[9], "keep"; got != want {
		t.Fatalf("modified key changed draft: %q, want %q", got, want)
	}

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o", Mod: tea.ModCtrl}))
	snapshot := model.Snapshot()
	if snapshot.PhotoSend == nil || snapshot.Focus != FocusPhotoSend {
		t.Fatal("Ctrl-O no longer opens Photo send")
	}
	if got, want := snapshot.Drafts[9], "keep"; got != want {
		t.Fatalf("Ctrl-O changed draft: %q, want %q", got, want)
	}

	authState := InitialState()
	authState.Focus = FocusAuth
	authState.Prompt = &PromptState{Prompt: auth.Prompt{ID: 17, Kind: auth.PromptPhone, Label: "Phone"}}
	authModel := newAppModelForTest(t, authState, newTestSession(t))
	authModel, _ = updateAppModel(t, authModel, tea.KeyPressMsg(tea.Key{Text: "a界"}))
	authModel, _ = updateAppModel(t, authModel, tea.PasteMsg{Content: "🙂x"})
	authModel, _ = updateAppModel(t, authModel, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got, want := string(authModel.Snapshot().Prompt.Input), "a界🙂"; got != want {
		t.Fatalf("auth input = %q, want %q", got, want)
	}
	if authModel.composerText.Identity() != (composerTextIdentity{}) || authModel.composerText.Value() != "" {
		t.Fatal("non-composer input contaminated composer host")
	}
}

func TestAppModelHuhComposerSynchronizesSemanticAndExternalState(t *testing.T) {
	t.Run("submit clears host immediately and preserves delivery", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusComposer
		state.Drafts[9] = "send me"
		model := newAppModelForTest(t, state, newTestSession(t))
		_ = model.syncComposerTextHost()

		model, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if cmd == nil {
			t.Fatal("ComposerSubmit delivery command was dropped")
		}
		if got := model.Snapshot().Drafts[9]; got != "" {
			t.Fatalf("authoritative draft after submit = %q, want empty", got)
		}
		assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "", true, 1, 1)
	})

	t.Run("TextEdited clears edit identity back to draft", func(t *testing.T) {
		state := InitialState()
		state.Connection = domain.ConnectionOnline
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusComposer
		state.Drafts[9] = "draft"
		state.Messages[9] = []domain.Message{{ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "old"}}
		state.EditTarget = &EditTarget{RequestID: 77, ChatID: 9, MessageID: 22, Original: "old", Buffer: "edited", Submitting: true}
		model := newAppModelForTest(t, state, newTestSession(t))
		_ = model.syncComposerTextHost()

		model, _ = updateAppModel(t, model, TextEdited{
			RequestID: 77,
			ChatID:    9,
			MessageID: 22,
			Message:   domain.Message{ID: 22, ChatID: 9, Kind: domain.MessageText, Text: "edited"},
		})
		if model.Snapshot().EditTarget != nil {
			t.Fatal("TextEdited did not clear authoritative EditTarget")
		}
		assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 1, 1)
	})

	t.Run("resize updates host geometry", func(t *testing.T) {
		state := InitialState()
		state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
		state.SelectedChat = 0
		state.Focus = FocusComposer
		model := newAppModelForTest(t, state, newTestSession(t))
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 88, Height: 24})
		assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "", true, 30, 2)
	})
}

func assertHuhComposerValue(t *testing.T, model AppModel, want string) {
	t.Helper()
	if got := model.Snapshot().Drafts[9]; got != want {
		t.Fatalf("draft = %q, want %q", got, want)
	}
	if got := model.composerText.Value(); got != want {
		t.Fatalf("host value = %q, want %q", got, want)
	}
	if got, identity := model.composerText.Identity(), (composerTextIdentity{ChatID: 9}); got != identity {
		t.Fatalf("host identity = %#v, want %#v", got, identity)
	}
}

func assertHuhComposerHost(t *testing.T, model AppModel, identity composerTextIdentity, value string, focused bool, width, height int) {
	t.Helper()
	if got := model.composerText.Identity(); got != identity {
		t.Fatalf("host identity = %#v, want %#v", got, identity)
	}
	if got := model.composerText.Value(); got != value {
		t.Fatalf("host value = %q, want %q", got, value)
	}
	if got := model.composerText.focused; got != focused {
		t.Fatalf("host focused = %v, want %v", got, focused)
	}
	if got := model.composerText.width; got != width {
		t.Fatalf("host width = %d, want %d", got, width)
	}
	if got := model.composerText.height; got != height {
		t.Fatalf("host height = %d, want %d", got, height)
	}
}

func huhComposerFunctionBody(source, signature string) string {
	start := strings.Index(source, signature)
	if start < 0 {
		return "<function not found>"
	}
	brace := strings.Index(source[start:], "{")
	if brace < 0 {
		return "<opening brace not found>"
	}
	depth := 0
	for index := start + brace; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : index+1]
			}
		}
	}
	return source[start:]
}

func TestAppModelHuhComposerIdentitySwitchesOnTopic(t *testing.T) {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatSupergroup, IsForum: true, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	state.Width = 100
	state.Height = 24
	state.Drafts[9] = "chat draft"
	state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{
		1: {ID: 1, ChatID: 9, Name: "General"},
		2: {ID: 2, ChatID: 9, Name: "Announcements"},
	}
	state.TopicDrafts[TopicKey{ChatID: 9, TopicID: 1}] = "topic one draft"
	state.TopicDrafts[TopicKey{ChatID: 9, TopicID: 2}] = "topic two draft"
	model := newAppModelForTest(t, state, newTestSession(t))

	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "chat draft", true, 42, 2)

	state = model.Snapshot()
	state.SelectedTopics[9] = 1
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9, TopicID: 1}, "topic one draft", true, 42, 2)

	state = model.Snapshot()
	state.SelectedTopics[9] = 2
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9, TopicID: 2}, "topic two draft", true, 42, 2)
	if strings.Contains(model.composerText.View(), "topic one draft") {
		t.Fatal("stale topic one value survived topic identity change")
	}
}
