package frontend

import (
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAppModelHuhComposerSynchronizesSharedTextGeometry(t *testing.T) {
	state := InitialState()
	state.Width = 100
	state.Height = 24
	state.Chats = []domain.Chat{{ID: 9, CanSend: true}}
	state.SelectedChat = 0
	state.Focus = FocusComposer
	state.Drafts[9] = "draft"
	model := newAppModelForTest(t, state, newTestSession(t))

	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 42, 2)

	state = model.Snapshot()
	state.ReplyTarget = &ReplyTarget{ChatID: 9, MessageID: 7}
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9}, "draft", true, 42, 2)

	state = model.Snapshot()
	state.ReplyTarget = nil
	state.EditTarget = &EditTarget{ChatID: 9, MessageID: 22, Buffer: "edit", Error: &domain.AppError{Message: "opaque"}}
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{ChatID: 9, EditMessageID: 22}, "edit", true, 42, 1)

	state = InitialState()
	state.Width = 100
	state.Height = 24
	state.Focus = FocusComposer
	model.state = &state
	_ = model.syncComposerTextHost()
	assertHuhComposerHost(t, model, composerTextIdentity{}, "", true, 1, 1)
}
