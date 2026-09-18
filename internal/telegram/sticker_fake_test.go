package telegram

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeStickerCatalogCopiesAndCancellation(t *testing.T) {
	catalog := []domain.StickerRef{{File: domain.MediaFileRef{ID: 7}, Thumbnail: domain.MediaFileRef{ID: 8}, Width: 64, Height: 32, Emoji: "🙂"}}
	fake := NewFake(FakeData{Stickers: catalog})
	catalog[0].File.ID = 99
	got, err := fake.LoadStickers(context.Background())
	if err != nil || got[0].File.ID != 7 {
		t.Fatalf("LoadStickers = %#v, %v", got, err)
	}
	got[0].File.ID = 55
	again, _ := fake.LoadStickers(context.Background())
	if again[0].File.ID != 7 {
		t.Fatal("LoadStickers returned shared storage")
	}
	sentinel := errors.New("catalog failure")
	if _, err := NewFake(FakeData{StickerLoadError: sentinel}).LoadStickers(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("catalog error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.LoadStickers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled LoadStickers error = %v", err)
	}
}

func TestFakeSendStickerRecordsFullRequestAndDeterministicError(t *testing.T) {
	sticker := domain.StickerRef{File: domain.MediaFileRef{ID: 17}, Thumbnail: domain.MediaFileRef{ID: 18}, Width: 80, Height: 40, Emoji: "🎉"}
	request := SendStickerRequest{ChatID: 9, Sticker: sticker, ReplyToMessageID: 44}
	fake := NewFake(FakeData{})
	message, err := fake.SendSticker(context.Background(), request)
	if err != nil || !reflect.DeepEqual(fake.StickerSendCalls(), []SendStickerRequest{request}) {
		t.Fatalf("SendSticker = %#v, %v calls=%#v", message, err, fake.StickerSendCalls())
	}
	if message.ID != -1 || message.Kind != domain.MessageSticker || message.Sticker != sticker || message.Media.File.ID != 17 || !message.HasReply || message.ReplyToMessageID != 44 || message.SendState != domain.SendPending {
		t.Fatalf("pending Sticker = %#v", message)
	}

	sentinel := errors.New("deterministic sticker failure")
	failed := NewFake(FakeData{StickerSendError: sentinel})
	if _, err := failed.SendSticker(context.Background(), request); !errors.Is(err, sentinel) {
		t.Fatalf("SendSticker error = %v", err)
	}
	if !reflect.DeepEqual(failed.StickerSendCalls(), []SendStickerRequest{request}) {
		t.Fatalf("error calls = %#v", failed.StickerSendCalls())
	}
}
