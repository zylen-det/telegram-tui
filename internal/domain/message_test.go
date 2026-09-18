package domain

import (
	"testing"
	"time"
)

func TestMessageEditedMetadataIsSeparateFromSentAt(t *testing.T) {
	sent, edited := time.Unix(10, 0), time.Unix(20, 0)
	message := Message{SentAt: sent, EditedAt: edited}
	if message.SentAt != sent || message.EditedAt != edited || !message.Edited() {
		t.Fatal("edited metadata changed sent identity")
	}
}

func TestMessagePropertiesCapabilitiesIncludeEdit(t *testing.T) {
	got := MessageCapabilities{Copy: true, Reply: true, Edit: true}
	if !got.Copy || !got.Reply || !got.Edit {
		t.Fatalf("capabilities = %#v", got)
	}
}

func TestForwardCapabilityIsExplicitAuthoritativeField(t *testing.T) {
	got := MessageCapabilities{Forward: true}
	if !got.Forward {
		t.Fatalf("capabilities = %#v", got)
	}
	none := MessageCapabilities{}
	if none.Forward {
		t.Fatalf("empty capabilities granted Forward")
	}
}

func TestPinMessagePinnedMetadataIsExplicit(t *testing.T) {
	message := Message{ID: 1, Kind: MessageText, Pinned: true}
	if !message.Pinned {
		t.Fatal("pinned metadata was lost")
	}
	message.Pinned = false
	if message.Pinned {
		t.Fatal("pinned metadata could not be cleared")
	}
}

func TestPinMessageCapabilitiesCarryAuthoritativePin(t *testing.T) {
	got := MessageCapabilities{Copy: true, Reply: true, Pin: true}
	if !got.Pin {
		t.Fatalf("capabilities = %#v", got)
	}
	none := MessageCapabilities{}
	if none.Pin {
		t.Fatalf("empty capabilities granted Pin")
	}
}

func TestPinMessageCapabilitiesNeverFromLocalFallback(t *testing.T) {
	tests := []struct {
		message Message
	}{
		{Message{ID: 1, Kind: MessageText}},
		{Message{ID: 1, Kind: MessagePhoto, Pinned: true}},
		{Message{ID: 1, Kind: MessageService, Service: true}},
		{Message{ID: 0, Kind: MessageText}},
		{Message{ID: -1, Kind: MessageText, Outgoing: true, SendState: SendPending}},
	}
	for _, test := range tests {
		if got := test.message.Capabilities().Pin; got {
			t.Fatalf("local Capabilities() granted Pin for %#v", test.message)
		}
	}
}

func TestLocalCapabilitiesFallbackNeverGrantsForward(t *testing.T) {
	tests := []struct {
		message Message
	}{
		{Message{ID: 1, Kind: MessageText}},
		{Message{ID: 1, Kind: MessagePhoto}},
		{Message{ID: 1, Kind: MessageService, Service: true}},
		{Message{ID: 0, Kind: MessageText}},
		{Message{ID: -1, Kind: MessageText, Outgoing: true, SendState: SendPending}},
	}
	for _, test := range tests {
		if got := test.message.Capabilities().Forward; got {
			t.Fatalf("local Capabilities() granted Forward for %#v", test.message)
		}
	}
}

func TestDeleteMessageCapabilitiesCarryAuthoritativeFlags(t *testing.T) {
	got := MessageCapabilities{Copy: true, Reply: true, Edit: true, DeleteForSelf: true, DeleteForAll: true}
	if !got.DeleteForSelf || !got.DeleteForAll {
		t.Fatalf("capabilities = %#v", got)
	}
	selfOnly := MessageCapabilities{DeleteForSelf: true}
	if !selfOnly.DeleteForSelf || selfOnly.DeleteForAll {
		t.Fatalf("self-only capabilities = %#v", selfOnly)
	}
	none := MessageCapabilities{}
	if none.DeleteForSelf || none.DeleteForAll {
		t.Fatalf("empty capabilities = %#v", none)
	}
}

func TestReplyCapabilityRequiresDurableNonServiceMessage(t *testing.T) {
	tests := []struct {
		message Message
		want    bool
	}{
		{Message{ID: 1, Kind: MessageText}, true},
		{Message{ID: 0, Kind: MessageText}, false},
		{Message{ID: -1, Kind: MessageText}, false},
		{Message{ID: 1, Kind: MessageService, Service: true}, false},
	}
	for _, test := range tests {
		if got := test.message.Capabilities().Reply; got != test.want {
			t.Fatalf("Reply capability = %t, want %t", got, test.want)
		}
	}
}

func TestMessageDisplayText(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		want    string
	}{
		{
			name:    "text",
			message: Message{Kind: MessageText, Text: "hello"},
			want:    "hello",
		},
		{
			name:    "photo",
			message: Message{Kind: MessagePhoto},
			want:    "[Photo]",
		},
		{
			name:    "video",
			message: Message{Kind: MessageVideo},
			want:    "[Video]",
		},
		{
			name:    "sticker",
			message: Message{Kind: MessageSticker},
			want:    "[Sticker]",
		},
		{
			name:    "named document",
			message: Message{Kind: MessageDocument, FileName: "report.pdf"},
			want:    "[File: report.pdf]",
		},
		{
			name:    "unnamed document",
			message: Message{Kind: MessageDocument},
			want:    "[File]",
		},
		{
			name:    "service",
			message: Message{Kind: MessageService, Text: "Alice joined"},
			want:    "Alice joined",
		},
		{
			name:    "unsupported",
			message: Message{Kind: MessageUnsupported},
			want:    "[Unsupported message]",
		},
		{
			name:    "unknown",
			message: Message{Kind: MessageKind(255)},
			want:    "[Unsupported message]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.message.DisplayText(); got != tt.want {
				t.Fatalf("DisplayText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessageReactionMetadataExplicitFields(t *testing.T) {
	reaction := MessageReaction{Emoji: "👍", Count: 3, Chosen: true}
	if reaction.Emoji != "👍" || reaction.Count != 3 || !reaction.Chosen {
		t.Fatalf("reaction = %#v", reaction)
	}
	message := Message{ID: 1, Kind: MessageText, Reactions: []MessageReaction{{Emoji: "❤️", Count: 1, Chosen: true}}}
	if len(message.Reactions) != 1 || message.Reactions[0].Emoji != "❤️" || !message.Reactions[0].Chosen {
		t.Fatalf("message reactions = %#v", message.Reactions)
	}
}

func TestMessageReactionFieldsAreComparableByValue(t *testing.T) {
	left := MessageReaction{Emoji: "👍", Count: 2}
	right := MessageReaction{Emoji: "👍", Count: 2}
	if left != right {
		t.Fatalf("reactions differ: %#v vs %#v", left, right)
	}
}

func TestMessageMediaMetadataValueSemantics(t *testing.T) {
	media := MessageMedia{
		File: MediaFileRef{
			ID:           42,
			UniqueID:     "unique-42",
			Size:         1024,
			ExpectedSize: 2048,
			LocalPath:    "/tmp/photo.jpg",
			CanDownload:  true,
			Downloaded:   true,
		},
		Thumbnail: MediaFileRef{
			ID:           43,
			UniqueID:     "thumb-43",
			Size:         256,
			ExpectedSize: 512,
			LocalPath:    "/tmp/thumb.jpg",
			CanDownload:  true,
			Downloaded:   false,
		},
		MIMEType: "image/jpeg",
		Width:    800,
		Height:   600,
		Duration: 30 * time.Second,
	}
	message := Message{
		ID:       1,
		Kind:     MessagePhoto,
		Media:    media,
		Text:     "photo caption",
		FileName: "photo.jpg",
	}
	if message.Media != media {
		t.Fatalf("Media assignment mismatch: got %#v, want %#v", message.Media, media)
	}
	if message.Media.File.ID != 42 || message.Media.File.UniqueID != "unique-42" {
		t.Fatalf("File identity mismatch: %#v", message.Media.File)
	}
	if message.Media.Thumbnail.ID != 43 || message.Media.Thumbnail.UniqueID != "thumb-43" {
		t.Fatalf("Thumbnail identity mismatch: %#v", message.Media.Thumbnail)
	}
	if message.Media.MIMEType != "image/jpeg" || message.Media.Width != 800 || message.Media.Height != 600 {
		t.Fatalf("Media dimensions/MIME mismatch: %#v", message.Media)
	}
	if message.Media.Duration != 30*time.Second {
		t.Fatalf("Media duration mismatch: got %v, want %v", message.Media.Duration, 30*time.Second)
	}
	if message.DisplayText() != "[Photo]" {
		t.Fatalf("Photo DisplayText changed: got %q, want %q", message.DisplayText(), "[Photo]")
	}
	zero := Message{}
	if zero.Media != (MessageMedia{}) {
		t.Fatalf("zero message Media should be zero, got %#v", zero.Media)
	}
	if zero.DisplayText() != "" {
		t.Fatalf("zero message DisplayText changed: got %q, want empty", zero.DisplayText())
	}
	text := Message{Kind: MessageText, Text: "hello", Media: MessageMedia{Width: 100}}
	if text.DisplayText() != "hello" {
		t.Fatalf("text DisplayText with media changed: got %q", text.DisplayText())
	}
	video := Message{Kind: MessageVideo, Media: MessageMedia{Width: 640, Height: 360}}
	if video.DisplayText() != "[Video]" {
		t.Fatalf("video DisplayText with media changed: got %q", video.DisplayText())
	}
	sticker := Message{Kind: MessageSticker, Media: MessageMedia{Width: 512, Height: 512}}
	if sticker.DisplayText() != "[Sticker]" {
		t.Fatalf("sticker DisplayText with media changed: got %q", sticker.DisplayText())
	}
	doc := Message{Kind: MessageDocument, FileName: "report.pdf", Media: MessageMedia{Width: 100}}
	if doc.DisplayText() != "[File: report.pdf]" {
		t.Fatalf("document DisplayText with media changed: got %q", doc.DisplayText())
	}
}
