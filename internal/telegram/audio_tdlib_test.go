//go:build tdlib

package telegram

import (
	"context"
	"reflect"
	"testing"
	"time"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAudioTDLibNormalizationNormalAndContentUpdate(t *testing.T) {
	content := &td.MessageAudio{
		Caption: &td.FormattedText{Text: "audio caption"},
		Audio: &td.Audio{
			Audio:    &td.File{Id: 31, Size: 100, ExpectedSize: 200, Remote: &td.RemoteFile{UniqueId: "audio-main"}, Local: &td.LocalFile{Path: "/tmp/song.mp3", IsDownloadingCompleted: true}},
			FileName: "song.mp3",
			Duration: 125,
			MimeType: "audio/mpeg",
		},
	}
	wantMedia := domain.MessageMedia{
		File:     domain.MediaFileRef{ID: 31, UniqueID: "audio-main", Size: 100, ExpectedSize: 200, LocalPath: "/tmp/song.mp3", Downloaded: true},
		MIMEType: "audio/mpeg",
		Duration: 2*time.Minute + 5*time.Second,
	}
	n := newNormalizer()
	message := n.message(&td.Message{Id: 7, ChatId: 9, Content: content})
	if message.Kind != domain.MessageAudio || message.Text != "audio caption" || message.FileName != "song.mp3" || message.Media != wantMedia || message.DisplayText() != "[Audio]" {
		t.Fatalf("normalized message = %#v", message)
	}

	update, ok := singleUpdate(t, n.update(&td.UpdateMessageContent{ChatId: 9, MessageId: 7, NewContent: content})).(MessageContentUpdated)
	if !ok || update.Kind != message.Kind || update.Text != message.Text || update.FileName != message.FileName || update.Media != message.Media {
		t.Fatalf("content update = %#v, want same mapping as message %#v", update, message)
	}
}

func TestAudioTDLibNormalizationNilAndNegativeDuration(t *testing.T) {
	kind, text, fileName, media := messageContent(&td.MessageAudio{Caption: &td.FormattedText{Text: "caption without descriptor"}})
	if kind != domain.MessageAudio || text != "caption without descriptor" || fileName != "" || media != (domain.MessageMedia{}) {
		t.Fatalf("nil Audio mapping = %v %q %q %#v", kind, text, fileName, media)
	}

	kind, text, fileName, media = messageContent(&td.MessageAudio{Audio: &td.Audio{Duration: -9, FileName: "bad.mp3", MimeType: "audio/mpeg"}})
	if kind != domain.MessageAudio || text != "" || fileName != "bad.mp3" || media.Duration != 0 || media.MIMEType != "audio/mpeg" {
		t.Fatalf("negative duration mapping = %v %q %q %#v", kind, text, fileName, media)
	}
}

func TestAdapterSendAudioBuildsExactRequestAndPendingMessage(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{Id: -51, ChatId: 77, Content: &td.MessageAudio{Audio: &td.Audio{}}}}
	adapter := adapterForDataTests(transport)
	request := SendAudioRequest{ChatID: 77, LocalPath: "/tmp/song.flac", Caption: "listen", ReplyToMessageID: 22}
	message, err := adapter.SendAudio(context.Background(), request)
	if err != nil {
		t.Fatalf("SendAudio error = %v", err)
	}
	sent := transport.sendMessageRequest
	if sent == nil || sent.ChatId != 77 || sent.TopicId != nil || sent.Options != nil || sent.ReplyMarkup != nil {
		t.Fatalf("SendMessage request = %#v", sent)
	}
	reply, ok := sent.ReplyTo.(*td.InputMessageReplyToMessage)
	if !ok || reply.MessageId != 22 {
		t.Fatalf("reply = %#v", sent.ReplyTo)
	}
	input, ok := sent.InputMessageContent.(*td.InputMessageAudio)
	if !ok {
		t.Fatalf("content = %T", sent.InputMessageContent)
	}
	local, ok := input.Audio.(*td.InputFileLocal)
	if !ok || local.Path != "/tmp/song.flac" || input.Caption == nil || input.Caption.Text != "listen" {
		t.Fatalf("audio input = %#v", input)
	}
	if input.AlbumCoverThumbnail != nil || input.Duration != 0 || input.Title != "" || input.Performer != "" {
		t.Fatalf("optional audio fields not zero/nil: %#v", input)
	}
	wantFile := domain.MediaFileRef{LocalPath: "/tmp/song.flac", Downloaded: true}
	if message.ID != -51 || message.ChatID != 77 || message.Kind != domain.MessageAudio || message.Text != "listen" || !message.Outgoing || message.SendState != domain.SendPending || !message.HasReply || message.ReplyToMessageID != 22 || !reflect.DeepEqual(message.Media.File, wantFile) {
		t.Fatalf("pending message = %#v", message)
	}
}

func TestAudioSendFailureUsesPrivacySafeOperation(t *testing.T) {
	n := newNormalizer()
	updates := n.update(&td.UpdateMessageSendFailed{
		OldMessageId: -1,
		Message:      &td.Message{Id: -2, ChatId: 9, Content: &td.MessageAudio{Audio: &td.Audio{}}},
		Error:        &td.Error{Code: 500, Message: "private detail"},
	})
	failed, ok := singleUpdate(t, updates).(MessageSendFailed)
	if !ok || failed.Error.Op != "send audio" || failed.Error.Message == "private detail" {
		t.Fatalf("send failure = %#v", failed)
	}
}
