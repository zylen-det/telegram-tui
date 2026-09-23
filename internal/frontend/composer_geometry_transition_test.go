package frontend

import (
	"image"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComposerTextRectConsumesHiddenBannerRowsAtNarrowWidth(t *testing.T) {
	t.Run("reply row remains reserved when Cancel cannot render", func(t *testing.T) {
		model := composerModel(8, 5, writable(9))
		model.ReplyTarget = &ReplyTarget{ChatID: 9}
		if got, want := composerTextRect(model, image.Rect(0, 0, 8, 5)), image.Rect(1, 1, 2, 3); !got.Eq(want) {
			t.Fatalf("reply text rect = %v, want %v", got, want)
		}
	})

	t.Run("edit and error rows remain reserved when Cancel cannot render", func(t *testing.T) {
		model := composerModel(8, 5, writable(9))
		model.EditTarget = &EditTarget{ChatID: 9, Error: &domain.AppError{Message: "opaque"}}
		if got, want := composerTextRect(model, image.Rect(0, 0, 8, 5)), image.Rect(1, 2, 2, 4); !got.Eq(want) {
			t.Fatalf("edit-error text rect = %v, want %v", got, want)
		}
	})
}
