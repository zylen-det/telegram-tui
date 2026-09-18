package app

import (
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestVideoPosterAcceptance_AllMessageIngressRequestsThumbnail(t *testing.T) {
	video := domain.Message{
		ID:     77,
		ChatID: 9,
		Kind:   domain.MessageVideo,
		Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{
			ID:          701,
			UniqueID:    "video-poster",
			CanDownload: true,
		}},
	}

	t.Run("history page", func(t *testing.T) {
		state := videoPosterAcceptanceState(40)
		state.History[9] = HistoryState{Loading: true, RequestID: 12}

		_, commands := Reduce(state, MessagesLoaded{
			RequestID: 12,
			ChatID:    9,
			Page:      telegram.MessagePage{Messages: []domain.Message{video}, Done: true},
		})
		want := []Command{DownloadThumbnail{RequestID: 40, ChatID: 9, MessageID: 77, File: video.Media.Thumbnail}}
		if !reflect.DeepEqual(commands, want) {
			t.Fatalf("history Video poster commands = %#v, want %#v", commands, want)
		}
	})

	t.Run("live upsert", func(t *testing.T) {
		state := videoPosterAcceptanceState(50)
		_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageUpserted{Message: video}})
		want := []Command{DownloadThumbnail{RequestID: 50, ChatID: 9, MessageID: 77, File: video.Media.Thumbnail}}
		if !reflect.DeepEqual(commands, want) {
			t.Fatalf("live Video poster commands = %#v, want %#v", commands, want)
		}
	})

	t.Run("content update", func(t *testing.T) {
		state := videoPosterAcceptanceState(60)
		state.Messages[9] = []domain.Message{{ID: 77, ChatID: 9, Kind: domain.MessageVideo}}
		_, commands := Reduce(state, TelegramEvent{Value: telegram.MessageContentUpdated{
			ChatID: 9, MessageID: 77, Kind: domain.MessageVideo, Media: video.Media,
		}})
		want := []Command{DownloadThumbnail{RequestID: 60, ChatID: 9, MessageID: 77, File: video.Media.Thumbnail}}
		if !reflect.DeepEqual(commands, want) {
			t.Fatalf("updated Video poster commands = %#v, want %#v", commands, want)
		}
	})
}

func TestVideoPosterAcceptance_PreservesPhotoAndRejectsUnavailableVideoPoster(t *testing.T) {
	photo := domain.Message{ID: 1, ChatID: 9, Kind: domain.MessagePhoto, Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 101, CanDownload: true}}}
	video := domain.Message{ID: 2, ChatID: 9, Kind: domain.MessageVideo, Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 202, CanDownload: true}}}
	unavailable := domain.Message{ID: 3, ChatID: 9, Kind: domain.MessageVideo, Media: domain.MessageMedia{Thumbnail: domain.MediaFileRef{ID: 303}}}
	state := videoPosterAcceptanceState(70)

	commands := requestMissingThumbnails(&state, []domain.Message{photo, video, unavailable})
	want := []Command{
		DownloadThumbnail{RequestID: 70, ChatID: 9, MessageID: 1, File: photo.Media.Thumbnail},
		DownloadThumbnail{RequestID: 71, ChatID: 9, MessageID: 2, File: video.Media.Thumbnail},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("Photo-or-Video poster commands = %#v, want %#v", commands, want)
	}
}

func videoPosterAcceptanceState(nextRequestID uint64) State {
	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9}}
	state.SelectedChat = 0
	state.NextRequestID = nextRequestID
	return state
}
