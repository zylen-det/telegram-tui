package domain

import "testing"

func TestChatCanReactIsExplicitPermissionField(t *testing.T) {
	chat := Chat{ID: 1, CanSend: true, CanReact: true}
	if !chat.CanReact {
		t.Fatalf("chat = %#v", chat)
	}
	chat.CanReact = false
	if chat.CanReact {
		t.Fatal("CanReact could not be cleared")
	}
}
