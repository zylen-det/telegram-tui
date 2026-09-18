package telegram

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeSendAudioRecordsAndReturnsPendingMessage(t *testing.T) {
	fake := NewFake(FakeData{})
	request := SendAudioRequest{ChatID: 9, LocalPath: "/tmp/song.MP3", Caption: "listen", ReplyToMessageID: 44}
	message, err := fake.SendAudio(context.Background(), request)
	if err != nil {
		t.Fatalf("SendAudio error = %v", err)
	}
	if calls := fake.AudioSendCalls(); !reflect.DeepEqual(calls, []SendAudioRequest{request}) {
		t.Fatalf("AudioSendCalls = %#v", calls)
	}
	if message.ID != -1 || message.ChatID != 9 || message.Kind != domain.MessageAudio || message.Text != "listen" || !message.Outgoing || message.SendState != domain.SendPending || !message.HasReply || message.ReplyToMessageID != 44 {
		t.Fatalf("message = %#v", message)
	}
	if message.Media.File.LocalPath != "/tmp/song.MP3" || !message.Media.File.Downloaded {
		t.Fatalf("media file = %#v", message.Media.File)
	}
}

func TestFakeSendAudioErrorAndCancellationConventions(t *testing.T) {
	sentinel := errors.New("deterministic audio failure")
	fake := NewFake(FakeData{AudioSendError: sentinel})
	request := SendAudioRequest{ChatID: 9, LocalPath: "/tmp/song.mp3"}
	if _, err := fake.SendAudio(context.Background(), request); !errors.Is(err, sentinel) {
		t.Fatalf("SendAudio error = %v, want sentinel", err)
	}
	if calls := fake.AudioSendCalls(); !reflect.DeepEqual(calls, []SendAudioRequest{request}) {
		t.Fatalf("error calls = %#v", calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.SendAudio(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SendAudio error = %v", err)
	}
	if calls := fake.AudioSendCalls(); len(calls) != 1 {
		t.Fatalf("canceled call was recorded: %#v", calls)
	}
}
