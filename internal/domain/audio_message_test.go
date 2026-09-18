package domain

import "testing"

func TestMessageAudioDisplayTextFallback(t *testing.T) {
	message := Message{Kind: MessageAudio, Text: "caption"}
	if got := message.DisplayText(); got != "[Audio]" {
		t.Fatalf("DisplayText() = %q, want [Audio]", got)
	}
}
