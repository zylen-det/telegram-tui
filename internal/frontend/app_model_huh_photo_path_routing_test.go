package frontend

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func photoPathRoutingState(value string) State {
	state := photoPathAppState(9, value, FocusPhotoSend)
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	return state
}

func TestAppModelHuhPhotoPathRoutesCursorEditingAndWholeValues(t *testing.T) {
	model := newAppModelForTest(t, photoPathRoutingState("ab"), newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "X"}))
	model, _ = updateAppModel(t, model, tea.PasteMsg{Content: "界\n🙂\r"})
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))

	if got, want := string(model.Snapshot().PhotoSend.Input), "aX界b"; got != want {
		t.Fatalf("authoritative photo path = %q, want %q", got, want)
	}
	if got, want := model.photoPathInput.Value(), "aX界b"; got != want {
		t.Fatalf("photo Huh host value = %q, want %q", got, want)
	}
}

func TestAppModelHuhPhotoPathRoutesDeleteHomeAndEnd(t *testing.T) {
	model := newAppModelForTest(t, photoPathRoutingState("abc"), newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDelete}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Text: "Z"}))

	if got, want := string(model.Snapshot().PhotoSend.Input), "bcZ"; got != want {
		t.Fatalf("home/delete/end photo path = %q, want %q", got, want)
	}
}

func TestAppModelHuhPhotoPathEnterSubmitsAndClearsHost(t *testing.T) {
	model := newAppModelForTest(t, photoPathRoutingState("/tmp/photo.png"), newTestSession(t))
	_ = model.syncPhotoPathInputHost()

	model, cmd := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("Photo submit delivery command was dropped")
	}
	snapshot := model.Snapshot()
	if snapshot.PhotoSend != nil || snapshot.Focus != FocusConversation {
		t.Fatalf("Enter did not submit/close PhotoSend: focus=%v photo=%#v", snapshot.Focus, snapshot.PhotoSend)
	}
	if model.photoPathInput.Identity() != 0 || model.photoPathInput.Value() != "" || model.photoPathInput.focused {
		t.Fatalf("submitted host retained state: id=%d value=%q focus=%t", model.photoPathInput.Identity(), model.photoPathInput.Value(), model.photoPathInput.focused)
	}
}

func TestAppModelHuhPhotoPathEscapeClosesAndClearsHost(t *testing.T) {
	model := newAppModelForTest(t, photoPathRoutingState("keep"), newTestSession(t))
	_ = model.syncPhotoPathInputHost()

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	snapshot := model.Snapshot()
	if snapshot.PhotoSend != nil || snapshot.Focus != FocusComposer {
		t.Fatalf("Escape did not restore prior focus: focus=%v photo=%#v", snapshot.Focus, snapshot.PhotoSend)
	}
	if model.photoPathInput.Identity() != 0 || model.photoPathInput.Value() != "" || model.photoPathInput.focused {
		t.Fatal("closed Photo path host retained stale identity/value/focus")
	}
}

func TestAppModelHuhPhotoPathModifiedKeysAreNoOps(t *testing.T) {
	model := newAppModelForTest(t, photoPathRoutingState("keep"), newTestSession(t))
	for _, key := range []tea.Key{
		{Text: "x", Code: 'x', Mod: tea.ModAlt},
		{Text: "u", Code: 'u', Mod: tea.ModCtrl},
		{Code: tea.KeyBackspace, Mod: tea.ModAlt},
		{Code: tea.KeyLeft, Mod: tea.ModCtrl},
	} {
		model, _ = updateAppModel(t, model, tea.KeyPressMsg(key))
	}
	if got := string(model.Snapshot().PhotoSend.Input); got != "keep" {
		t.Fatalf("modified key changed photo path: %q", got)
	}
}

func TestAppModelHuhPhotoPathCtrlOFocusesHost(t *testing.T) {
	state := InitialState()
	state.Width, state.Height = 100, 24
	state.Connection = domain.ConnectionOnline
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	model := newAppModelForTest(t, state, newTestSession(t))

	model, _ = updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o", Mod: tea.ModCtrl}))
	if model.Snapshot().Focus != FocusPhotoSend || model.Snapshot().PhotoSend == nil {
		t.Fatal("Ctrl-O no longer opens PhotoSend")
	}
	if model.photoPathInput.Identity() != 9 || !model.photoPathInput.focused || model.photoPathInput.width != 52 {
		t.Fatalf("opened Photo host = id:%d focus:%t width:%d", model.photoPathInput.Identity(), model.photoPathInput.focused, model.photoPathInput.width)
	}
}

func TestAppModelHuhPhotoPathRoutingStructure(t *testing.T) {
	source, err := os.ReadFile("app_model.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "return key.Mod&^tea.ModShift == 0") {
		t.Fatal("Photo Huh routing no longer rejects Alt/Ctrl/Meta modifiers")
	}
	photoBranch := strings.Index(text, "if focus == FocusPhotoSend && photoEditKeyAllowed(msg.Key())")
	if photoBranch < 0 {
		t.Fatal("Photo Huh routing branch is missing")
	}
	for _, forbidden := range []string{"m.applyRunes(", "func (m AppModel) applyRunes", "if editableFocus(focus) && textInputAllowed(msg.Key())", "if editableFocus(m.Snapshot().Focus)"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("AppModel retained superseded generic per-rune fallback %q", forbidden)
		}
	}
	changedPostSync := "return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())"
	unchangedPostSync := "return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())"
	if got := strings.Count(text, changedPostSync); got != 3 {
		t.Fatalf("Photo changed-value post-sync seams = %d, want 3", got)
	}
	if got := strings.Count(text, unchangedPostSync); got != 6 {
		t.Fatalf("controlled-input unchanged-value Photo post-sync seams = %d, want 6", got)
	}
}
