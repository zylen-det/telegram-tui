package frontend

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
)

func authorizationAppState(id uint64, input string, secret bool) State {
	state := InitialState()
	state.Width = 100
	state.Height = 24
	state.Focus = FocusAuth
	state.Prompt = &PromptState{
		Prompt: auth.Prompt{ID: id, Kind: auth.PromptPassword, Label: "Password", Secret: secret},
		Input:  []rune(input),
	}
	return state
}

func TestAppModelHuhAuthorizationOwnsPersistentHost(t *testing.T) {
	model := newAppModelForTest(t, InitialState(), newTestSession(t))
	if model.authorizationInput == nil {
		t.Fatal("NewAppModel authorizationInput host is nil")
	}
	copied := model
	if copied.authorizationInput != model.authorizationInput {
		t.Fatal("AppModel value copy replaced persistent authorization host")
	}
	other := newAppModelForTest(t, InitialState(), newTestSession(t))
	if other.authorizationInput == model.authorizationInput {
		t.Fatal("independent AppModels share authorization host")
	}
}

func TestAppModelHuhAuthorizationRetainsFocusSyncCommand(t *testing.T) {
	model := newAppModelForTest(t, authorizationAppState(7, "", false), newTestSession(t))

	model, cmd := updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	if cmd == nil {
		t.Fatal("WindowSize discarded the authorization Huh focus command")
	}
	if !model.authorizationInput.focused {
		t.Fatal("WindowSize did not focus the authorization host")
	}
}

func TestAuthorizationInputRectMatchesSharedSurfaceGeometry(t *testing.T) {
	for _, test := range []struct {
		bounds image.Rectangle
		want   image.Rectangle
	}{
		{image.Rect(0, 0, 100, 24), image.Rect(24, 11, 76, 12)},
		{image.Rect(10, 20, 110, 44), image.Rect(34, 31, 86, 32)},
		{image.Rect(0, 0, 20, 12), image.Rect(4, 5, 16, 6)},
		{image.Rect(7, 9, 27, 21), image.Rect(11, 14, 23, 15)},
		{image.Rect(0, 0, 8, 8), image.Rectangle{}},
	} {
		if got := authorizationInputRect(test.bounds); got != test.want {
			t.Fatalf("authorizationInputRect(%v) = %v, want %v", test.bounds, got, test.want)
		}
	}
}

func TestAppModelHuhAuthorizationSynchronizesPromptIdentityPrivacyAndGeometry(t *testing.T) {
	model := newAppModelForTest(t, authorizationAppState(11, "s界🙂cret", true), newTestSession(t))
	_ = model.syncAuthorizationInputHost()
	if got := model.authorizationInput; got.Identity() != 11 || got.Value() != "s界🙂cret" || !got.focused || !got.secret || got.width != 52 {
		t.Fatalf("initial authorization host = id:%d value:%q focus:%t secret:%t width:%d", got.Identity(), got.Value(), got.focused, got.secret, got.width)
	}

	state := authorizationAppState(12, "plain", false)
	model.state = &state
	_ = model.syncAuthorizationInputHost()
	if got := model.authorizationInput; got.Identity() != 12 || got.Value() != "plain" || !got.focused || got.secret || got.width != 52 {
		t.Fatalf("new prompt host retained stale state: id:%d value:%q focus:%t secret:%t width:%d", got.Identity(), got.Value(), got.focused, got.secret, got.width)
	}

	state = model.Snapshot()
	state.Focus = FocusChats
	model.state = &state
	_ = model.syncAuthorizationInputHost()
	if model.authorizationInput.focused {
		t.Fatal("authorization host stayed focused outside FocusAuth")
	}
}

func TestAppModelHuhAuthorizationRoutesWholeValueEditingAndSubmit(t *testing.T) {
	model := newAppModelForTest(t, authorizationAppState(21, "", false), newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "a界"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "🙂x"})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got := string(model.Snapshot().Prompt.Input); got != "a界🙂" {
		t.Fatalf("authoritative prompt input = %q, want a界🙂", got)
	}
	if got := model.authorizationInput.Value(); got != "a界🙂" {
		t.Fatalf("host value = %q, want a界🙂", got)
	}

	model, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("authorization submit delivery command was dropped")
	}
	if model.Snapshot().Prompt != nil {
		t.Fatal("Enter no longer submits and clears authoritative prompt")
	}
	if got := model.authorizationInput; got.Identity() != 0 || got.Value() != "" || got.focused {
		t.Fatalf("submitted host not cleared/blurred: id:%d value:%q focus:%t", got.Identity(), got.Value(), got.focused)
	}
}

func TestAppModelHuhAuthorizationDoesNotRouteModifiedText(t *testing.T) {
	model := newAppModelForTest(t, authorizationAppState(31, "keep", false), newTestSession(t))
	for _, key := range []tea.Key{
		{Text: "x", Code: 'x', Mod: tea.ModAlt},
		{Text: "u", Code: 'u', Mod: tea.ModCtrl},
		{Code: tea.KeyBackspace, Mod: tea.ModAlt},
	} {
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(key))
	}
	if got := string(model.Snapshot().Prompt.Input); got != "keep" {
		t.Fatalf("modified key changed prompt input: %q", got)
	}
}
