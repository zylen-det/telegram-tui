package frontend

import (
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
)

func TestVideoPosterAcceptance_StaticPosterCaptionAndMetadataRows(t *testing.T) {
	message := domain.Message{
		ID:       77,
		ChatID:   9,
		Kind:     domain.MessageVideo,
		Text:     "launch clip",
		FileName: "clip.mp4",
		Media: domain.MessageMedia{
			MIMEType: "video/mp4",
			Width:    640,
			Height:   360,
			Duration: 125 * time.Second,
		},
	}
	group := RenderedMessageGroup{MessageGroup: MessageGroup{Messages: []domain.Message{message}}}
	poster := thumbnail.Block{Text: "poster", Width: 20, Height: 2}

	rows, _ := buildMessageRows(group, 100, time.UTC, messageSelection{}, map[domain.MessageID]thumbnail.Block{77: poster})
	got := make([]string, len(rows))
	for index := range rows {
		got[index] = rows[index].text
		if index > 0 && index < len(rows)-1 && (rows[index].chatID != 9 || rows[index].messageID != 77) {
			t.Fatalf("row %d identity = (%d,%d), want (9,77)", index, rows[index].chatID, rows[index].messageID)
		}
	}
	want := []string{"", "", "", "launch clip", "[Video] clip.mp4 · 2:05 · 640×360 · video/mp4", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Video poster rows = %#v, want %#v", got, want)
	}
}

func TestVideoPosterAcceptance_MetadataFallbackOmitsEmptyFields(t *testing.T) {
	message := domain.Message{
		ID:     78,
		ChatID: 9,
		Kind:   domain.MessageVideo,
		Media:  domain.MessageMedia{Duration: 9 * time.Second},
	}
	group := RenderedMessageGroup{MessageGroup: MessageGroup{Messages: []domain.Message{message}}}

	rows, _ := buildMessageRows(group, 80, time.UTC, messageSelection{}, nil)
	if len(rows) != 3 || rows[1].text != "[Video] 0:09" {
		t.Fatalf("Video metadata fallback rows = %#v, want one %q row", rows, "[Video] 0:09")
	}
}
