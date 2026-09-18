package frontend

import (
	"image"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func photoPathAppState(chatID domain.ChatID, value string, focus app.Focus) app.State {
	state := app.InitialState()
	state.Width = 100
	state.Height = 24
	state.Focus = focus
	state.PhotoSend = &app.PhotoSendState{
		ChatID:        chatID,
		Input:         []rune(value),
		PreviousFocus: app.FocusComposer,
	}
	return state
}

func TestAppModelHuhPhotoPathOwnsPersistentHost(t *testing.T) {
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), newBoundedAppRuntimeForModelTest(t))
	if model.photoPathInput == nil {
		t.Fatal("NewAppModel photoPathInput host is nil")
	}
	copied := model
	if copied.photoPathInput != model.photoPathInput {
		t.Fatal("AppModel value copy replaced persistent photo path host")
	}
	other := newAppModelForTest(t, app.NewEngine(app.InitialState()), newBoundedAppRuntimeForModelTest(t))
	if other.photoPathInput == model.photoPathInput {
		t.Fatal("independent AppModels share photo path host")
	}
}

func TestPhotoSendInputRectMatchesSharedModalGeometry(t *testing.T) {
	for _, test := range []struct {
		bounds image.Rectangle
		want   image.Rectangle
	}{
		{image.Rect(0, 0, 100, 24), image.Rect(24, 11, 76, 12)},
		{image.Rect(10, 20, 110, 44), image.Rect(34, 31, 86, 32)},
		{image.Rect(0, 0, 20, 8), image.Rect(2, 3, 18, 4)},
		{image.Rect(7, 9, 27, 17), image.Rect(9, 12, 25, 13)},
		{image.Rect(0, 0, 19, 8), image.Rectangle{}},
		{image.Rect(0, 0, 20, 7), image.Rectangle{}},
	} {
		if got := photoSendInputRect(test.bounds); got != test.want {
			t.Fatalf("photoSendInputRect(%v) = %v, want %v", test.bounds, got, test.want)
		}
	}
}

func TestPhotoSendModalConsumesSharedInputRect(t *testing.T) {
	source, err := os.ReadFile("surface_photo_send_modal.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "inputAbs := photoSendInputRect(bounds)") {
		t.Fatal("Photo send modal does not consume shared input geometry")
	}
	builder, _, found := strings.Cut(text, "func photoSendInputRect")
	if !found {
		t.Fatal("shared Photo send input geometry helper is missing")
	}
	if strings.Contains(builder, "inputLocal := image.Rect(2, 3") {
		t.Fatal("Photo send modal retained a parallel input geometry source")
	}
}

func TestAppModelHuhPhotoPathSynchronizesIdentityValueFocusAndGeometry(t *testing.T) {
	engine := app.NewEngine(photoPathAppState(9, "/tmp/界🙂.png", app.FocusPhotoSend))
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	if cmd := model.syncPhotoPathInputHost(); cmd == nil {
		t.Fatal("initial photo path focus command was discarded")
	}
	if got := model.photoPathInput; got.Identity() != 9 || got.Value() != "/tmp/界🙂.png" || !got.focused || got.width != 52 {
		t.Fatalf("initial photo host = id:%d value:%q focus:%t width:%d", got.Identity(), got.Value(), got.focused, got.width)
	}

	state := photoPathAppState(10, "next", app.FocusConversation)
	engine = app.NewEngine(state)
	model.engine = engine
	if cmd := model.syncPhotoPathInputHost(); cmd != nil {
		t.Fatal("Huh photo path blur returned an unexpected command")
	}
	if got := model.photoPathInput; got.Identity() != 10 || got.Value() != "next" || got.focused || got.width != 52 {
		t.Fatalf("resynchronized photo host = id:%d value:%q focus:%t width:%d", got.Identity(), got.Value(), got.focused, got.width)
	}

	state.PhotoSend = nil
	engine = app.NewEngine(state)
	model.engine = engine
	_ = model.syncPhotoPathInputHost()
	if got := model.photoPathInput; got.Identity() != 0 || got.Value() != "" || got.focused {
		t.Fatalf("closed photo host retained state = id:%d value:%q focus:%t", got.Identity(), got.Value(), got.focused)
	}
}

func TestAppModelHuhPhotoPathRetainsWindowSizeFocusCommand(t *testing.T) {
	engine := app.NewEngine(photoPathAppState(9, "", app.FocusPhotoSend))
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	model, cmd := updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	if cmd == nil {
		t.Fatal("WindowSize discarded the photo path Huh focus command")
	}
	if !model.photoPathInput.focused {
		t.Fatal("WindowSize did not focus the photo path host")
	}
}
