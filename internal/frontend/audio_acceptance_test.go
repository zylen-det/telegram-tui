package frontend

import (
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAudioRenderingCaptionMetadataOmissionFallbackAndIdentity(t *testing.T) {
	message := domain.Message{
		ID: 77, ChatID: 9, Kind: domain.MessageAudio, Text: "listen now", FileName: "song.flac",
		Media: domain.MessageMedia{Duration: 125 * time.Second, MIMEType: "audio/flac", Width: 999, Height: 888},
	}
	group := RenderedMessageGroup{MessageGroup: MessageGroup{Messages: []domain.Message{message}}}
	rows, _ := buildMessageRows(group, 100, time.UTC, messageSelection{}, nil)
	got := make([]string, len(rows))
	for i, row := range rows {
		got[i] = row.text
		if row.chatID != 9 || row.messageID != 77 {
			t.Fatalf("row %d identity = (%d,%d)", i, row.chatID, row.messageID)
		}
	}
	want := []string{"listen now", "[Audio] song.flac · 2:05 · audio/flac"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}

	pending := message
	pending.Outgoing = true
	pending.SendState = domain.SendPending
	pendingRows, _ := buildMessageRows(RenderedMessageGroup{MessageGroup: MessageGroup{Messages: []domain.Message{pending}}}, 100, time.UTC, messageSelection{}, nil)
	if len(pendingRows) != 2 || !pendingRows[1].outgoing || pendingRows[1].kind != messageRowPanel {
		t.Fatalf("pending Audio rows = %#v", pendingRows)
	}
	failed := pending
	failed.SendState = domain.SendFailed
	failedRows, _ := buildMessageRows(RenderedMessageGroup{MessageGroup: MessageGroup{Messages: []domain.Message{failed}}}, 100, time.UTC, messageSelection{}, nil)
	if len(failedRows) != 2 || failedRows[1].kind != messageRowError {
		t.Fatalf("failed Audio rows = %#v", failedRows)
	}

	for _, test := range []struct {
		name    string
		message domain.Message
		want    string
	}{
		{name: "duration only", message: domain.Message{Kind: domain.MessageAudio, Media: domain.MessageMedia{Duration: 9 * time.Second}}, want: "[Audio] 0:09"},
		{name: "mime only", message: domain.Message{Kind: domain.MessageAudio, Media: domain.MessageMedia{MIMEType: " audio/ogg "}}, want: "[Audio] audio/ogg"},
		{name: "fallback", message: domain.Message{Kind: domain.MessageAudio}, want: "[Audio]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := audioMetadata(test.message); got != test.want {
				t.Fatalf("audioMetadata = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAudioActionLabelsAndPhotoVideoNonRegression(t *testing.T) {
	file := domain.MediaFileRef{ID: 7, CanDownload: true}
	for _, test := range []struct {
		kind  domain.MessageKind
		id    string
		label string
	}{
		{kind: domain.MessageAudio, id: "action:open-audio", label: "Open audio"},
		{kind: domain.MessageVideo, id: "action:open-video", label: "Open video"},
		{kind: domain.MessagePhoto, id: "action:view-image", label: "View image"},
	} {
		menu := &MessageActionMenu{ChatID: 9, MessageID: 77, MediaFile: file, MediaKind: test.kind}
		options := selectorOptionsFromRows(messageActionRows(menu))
		if len(options) != 1 || options[0].ID != test.id || options[0].Label != test.label || options[0].Value.Action != ViewMessageMedia {
			t.Fatalf("kind %v options = %#v", test.kind, options)
		}
	}
}
