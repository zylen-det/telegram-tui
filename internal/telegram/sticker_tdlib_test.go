//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func tdSticker(fileID, thumbID int32, width, height int32, emoji string) *td.Sticker {
	var thumb *td.Thumbnail
	if thumbID != 0 {
		thumb = &td.Thumbnail{File: &td.File{Id: thumbID}}
	}
	var file *td.File
	if fileID != 0 {
		file = &td.File{Id: fileID}
	}
	return &td.Sticker{Sticker: file, Thumbnail: thumb, Width: width, Height: height, Emoji: emoji}
}

func TestAdapterLoadStickersFavoriteRecentOrderDedupeFilterAndExactRecentRequest(t *testing.T) {
	transport := &dataTransport{
		favorites: &td.Stickers{Stickers: []*td.Sticker{tdSticker(3, 30, 64, 32, "fav"), nil, tdSticker(0, 1, 1, 1, "bad")}},
		recent:    &td.Stickers{Stickers: []*td.Sticker{tdSticker(3, 31, 2, 2, "duplicate"), tdSticker(4, 40, -2, -3, "recent")}},
	}
	got, err := adapterForDataTests(transport).LoadStickers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if transport.getRecentStickersRequest == nil || transport.getRecentStickersRequest.IsAttached {
		t.Fatalf("GetRecentStickers request = %#v", transport.getRecentStickersRequest)
	}
	if len(got) != 2 || got[0].File.ID != 3 || got[0].Thumbnail.ID != 30 || got[0].Emoji != "fav" || got[1].File.ID != 4 || got[1].Width != 0 || got[1].Height != 0 {
		t.Fatalf("catalog = %#v", got)
	}
}

func TestAdapterLoadStickersNilAndFailureAreSafe(t *testing.T) {
	got, err := adapterForDataTests(&dataTransport{}).LoadStickers(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("nil lists = %#v, %v", got, err)
	}
	for _, transport := range []*dataTransport{
		{favoriteErr: errors.New("private favorite token")},
		{favorites: &td.Stickers{}, recentErr: errors.New("private recent token")},
	} {
		_, err := adapterForDataTests(transport).LoadStickers(context.Background())
		var appError domain.AppError
		if !errors.As(err, &appError) || appError.Op != "load stickers" || strings.Contains(err.Error(), "token") {
			t.Fatalf("load failure = %#v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapterForDataTests(&dataTransport{}).LoadStickers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled load = %v", err)
	}
}

func TestStickerNormalizationPreservesReferenceAndNilIsSafe(t *testing.T) {
	n := newNormalizer()
	ref := tdSticker(71, 72, -4, 45, "🙂")
	message := n.message(&td.Message{Id: 1, ChatId: 9, Content: &td.MessageSticker{Sticker: ref}})
	if message.Kind != domain.MessageSticker || message.Sticker.File.ID != 71 || message.Sticker.Thumbnail.ID != 72 || message.Sticker.Width != 0 || message.Sticker.Height != 45 || message.Sticker.Emoji != "🙂" || message.Media.File.ID != 71 || message.Media.Thumbnail.ID != 72 {
		t.Fatalf("normalized sticker = %#v", message)
	}
	nilSticker := n.message(&td.Message{Id: 2, ChatId: 9, Content: &td.MessageSticker{}})
	if nilSticker.Kind != domain.MessageSticker || nilSticker.Sticker != (domain.StickerRef{}) || nilSticker.Media != (domain.MessageMedia{}) {
		t.Fatalf("nil sticker = %#v", nilSticker)
	}
}

func TestStickerSendFailureUsesPrivacySafeOperation(t *testing.T) {
	n := newNormalizer()
	private := "private sticker transport detail"
	updates := n.update(&td.UpdateMessageSendFailed{
		OldMessageId: -1,
		Message:      &td.Message{Id: -2, ChatId: 9, Content: &td.MessageSticker{Sticker: tdSticker(71, 72, 64, 64, "🙂")}},
		Error:        &td.Error{Code: 500, Message: private},
	})
	failed, ok := singleUpdate(t, updates).(MessageSendFailed)
	if !ok || failed.Error.Op != "send sticker" || failed.Message.Failure == nil || failed.Message.Failure.Op != "send sticker" {
		t.Fatalf("send failure = %#v", failed)
	}
	if strings.Contains(failed.Error.Message, private) || strings.Contains(failed.Message.Failure.Message, private) {
		t.Fatalf("send failure leaked private detail: %#v", failed)
	}
}

func TestAdapterSendStickerExactInputAndPreservesOmittedMedia(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{Id: -8, ChatId: 9, Content: &td.MessageSticker{}}}
	ref := domain.StickerRef{File: domain.MediaFileRef{ID: 71}, Thumbnail: domain.MediaFileRef{ID: 72}, Width: 90, Height: 45, Emoji: "🙂"}
	message, err := adapterForDataTests(transport).SendSticker(context.Background(), SendStickerRequest{ChatID: 9, Sticker: ref, ReplyToMessageID: 22})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.sendMessageRequest
	content, ok := request.InputMessageContent.(*td.InputMessageSticker)
	file, fileOK := content.Sticker.(*td.InputFileId)
	if request.ChatId != 9 || request.ReplyTo == nil || !ok || !fileOK || file.Id != 71 || content.Thumbnail != nil || content.Width != 90 || content.Height != 45 || content.Emoji != "🙂" {
		t.Fatalf("request = %#v content=%#v", request, content)
	}
	if message.Kind != domain.MessageSticker || message.Sticker != ref || message.Media.Thumbnail.ID != 72 || !message.HasReply || message.ReplyToMessageID != 22 || message.SendState != domain.SendPending {
		t.Fatalf("pending message = %#v", message)
	}
}
