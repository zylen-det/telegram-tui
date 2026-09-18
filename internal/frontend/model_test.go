package frontend

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInputAcceptsUnicodePasteAndBackspaceByRune(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.KeyPressMsg(tea.Key{Text: "ab界🙂"}))
	if got, want := string(model.input), "ab界🙂"; got != want {
		t.Fatalf("input after paste = %q, want %q", got, want)
	}

	model, _ = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got, want := string(model.input), "ab界"; got != want {
		t.Fatalf("input after backspace = %q, want %q", got, want)
	}

	model, _ = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !model.submitted {
		t.Fatal("Enter did not record a submit indication")
	}
}

func TestPasteMsgAppendsFullContent(t *testing.T) {
	model := NewModel()
	model.input = []rune("existing:")
	model.submitted = true

	const pasted = "line one\n界👨‍👩‍👧‍👦"
	model, _ = updateModel(t, model, tea.PasteMsg{Content: pasted})

	if got, want := string(model.input), "existing:"+pasted; got != want {
		t.Fatalf("input after PasteMsg = %q, want %q", got, want)
	}
	if model.submitted {
		t.Fatal("PasteMsg did not clear the submitted indication")
	}
}

func TestInputCtrlCRequestsBubbleTeaQuit(t *testing.T) {
	model := NewModel()
	model, cmd := updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("Ctrl-C did not return Bubble Tea's quit command")
	}
	if len(model.input) != 0 {
		t.Fatalf("Ctrl-C changed input to %q", string(model.input))
	}
	if msg := cmd(); msg == nil {
		t.Fatal("Ctrl-C quit command returned a nil message")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("Ctrl-C command returned %T, want tea.QuitMsg", msg)
	}
}

func TestInputEnterShowsAcceptedIndication(t *testing.T) {
	model := NewModel()
	model, _ = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	plain := ansiSequence.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(plain, "Input accepted; waiting for authorization") {
		t.Fatalf("view after Enter does not show accepted indication:\n%s", plain)
	}
}

func updateModel(t *testing.T, model Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := model.Update(msg)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want frontend.Model", next)
	}
	return updated, cmd
}
