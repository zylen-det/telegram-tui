package frontend

import (
	"os"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAppModelHuhComposerSynchronizesSharedTextGeometry(t *testing.T) {
	state := app.InitialState()
	state.Width = 100
	state.Height = 24
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = app.FocusComposer
	state.Drafts[9] = "draft"
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))

	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 42, 2)

	state = engine.Snapshot()
	state.ReplyTarget = &app.ReplyTarget{ChatID: 9, MessageID: 7}
	engine = app.NewEngine(state)
	model.engine = engine
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 42, 2)

	state = engine.Snapshot()
	state.ReplyTarget = nil
	state.EditTarget = &app.EditTarget{ChatID: 9, MessageID: 22, Buffer: "edit", Error: &domain.AppError{Message: "opaque"}}
	engine = app.NewEngine(state)
	model.engine = engine
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9, EditMessageID: 22}, "edit", true, 42, 1)

	state = app.InitialState()
	state.Width = 100
	state.Height = 24
	state.Focus = app.FocusComposer
	engine = app.NewEngine(state)
	model.engine = engine
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{}, "", true, 1, 1)
}

func TestAppModelHuhComposerSyncConsumesSharedGeometry(t *testing.T) {
	source, err := os.ReadFile("app_model.go")
	if err != nil {
		t.Fatalf("read app_model.go: %v", err)
	}
	body := huhComposerFunctionBody(string(source), "func (m AppModel) syncComposerTextHost()")
	for _, required := range []string{
		"ui.Select(snap, m.location)",
		"composerSurfaceRect(model)",
		"composerTextRect(model, composerRect)",
		"textRect.Dx()",
		"textRect.Dy()",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("syncComposerTextHost missing shared geometry expression %q\n%s", required, body)
		}
	}
	if strings.Contains(body, "snap.Width") {
		t.Fatalf("syncComposerTextHost still passes terminal width directly\n%s", body)
	}
}
