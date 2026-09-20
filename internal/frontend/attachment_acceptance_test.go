package frontend

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAttachmentMetadataAndFallbacks(t *testing.T) {
	cases := []struct {
		name    string
		message domain.Message
		want    string
	}{
		{
			name: "document",
			message: domain.Message{Kind: domain.MessageDocument, FileName: "report.pdf", Media: domain.MessageMedia{
				File: domain.MediaFileRef{ExpectedSize: 1536}, MIMEType: "application/pdf",
			}},
			want: "[File] report.pdf · 1.5 KB · application/pdf",
		},
		{
			name: "animation",
			message: domain.Message{Kind: domain.MessageAnimation, FileName: "cat.gif", Media: domain.MessageMedia{
				File: domain.MediaFileRef{Size: 2048}, Duration: 5 * time.Second, Width: 256, Height: 128, MIMEType: "image/gif",
			}},
			want: "[Animation] cat.gif · 0:05 · 256×128 · 2 KB · image/gif",
		},
		{
			name: "voice note",
			message: domain.Message{Kind: domain.MessageVoiceNote, Media: domain.MessageMedia{
				File: domain.MediaFileRef{Size: 512}, Duration: 62 * time.Second, MIMEType: "audio/ogg",
			}},
			want: "[Voice note] 1:02 · 512 B · audio/ogg",
		},
		{
			name: "video note",
			message: domain.Message{Kind: domain.MessageVideoNote, Media: domain.MessageMedia{
				File: domain.MediaFileRef{Size: 1024}, Duration: 9 * time.Second, Width: 480, Height: 480,
			}},
			want: "[Video note] 0:09 · 480×480 · 1 KB",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := attachmentMetadata(test.message); got != test.want {
				t.Fatalf("metadata = %q, want %q", got, test.want)
			}
			test.message.FileName = ""
			test.message.Media = domain.MessageMedia{}
			if got := attachmentMetadata(test.message); got != test.message.DisplayText() {
				t.Fatalf("fallback = %q, want %q", got, test.message.DisplayText())
			}
		})
	}
}

func TestAttachmentMetadataSanitizesControlsAndBoundsRow(t *testing.T) {
	message := domain.Message{
		Kind:     domain.MessageDocument,
		FileName: "safe\x1b[31m\u009b2J\nname.pdf",
		Media:    domain.MessageMedia{MIMEType: "application/\x07pdf"},
	}
	metadata := fileMetadata(message)
	for _, r := range metadata {
		if unicode.IsControl(r) {
			t.Fatalf("metadata contains control rune %U: %q", r, metadata)
		}
	}
	row := clipLine(metadata, 18)
	if displayWidth(row) > 18 || strings.Contains(row, "\x1b") || strings.Contains(row, "\u009b") {
		t.Fatalf("bounded row = %q width=%d", row, displayWidth(row))
	}
}

func TestAttachmentActionOptionsAreExactAndPreserveAcceptedMedia(t *testing.T) {
	cases := []struct {
		kind  domain.MessageKind
		id    string
		label string
	}{
		{domain.MessageDocument, "action:open-file", "Open file"},
		{domain.MessageAnimation, "action:open-animation", "Open animation"},
		{domain.MessageVoiceNote, "action:open-voice-note", "Open voice note"},
		{domain.MessageVideoNote, "action:open-video-note", "Open video note"},
		{domain.MessageVideo, "action:open-video", "Open video"},
		{domain.MessageAudio, "action:open-audio", "Open audio"},
		{domain.MessagePhoto, "action:view-image", "View image"},
	}
	for _, test := range cases {
		t.Run(test.label, func(t *testing.T) {
			menu := &MessageActionMenu{
				ChatID: 9, MessageID: 77, MediaKind: test.kind,
				MediaFile: domain.MediaFileRef{ID: 1, CanDownload: true},
				Loading:   true,
			}
			options := selectorOptionsFromRows(messageActionRows(menu))
			if len(options) == 0 || options[0].ID != test.id || options[0].Label != test.label || options[0].Value.Action != ViewMessageMedia {
				t.Fatalf("options = %#v", options)
			}
			menu.Loading = false
			menu.Error = &domain.AppError{Kind: domain.ErrorInternal}
			options = selectorOptionsFromRows(messageActionRows(menu))
			if len(options) == 0 || options[0].ID != test.id {
				t.Fatalf("error-state options = %#v", options)
			}
		})
	}
}
