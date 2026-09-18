package frontend

import (
	"image"
	"os"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComposerTextRectConsumesHiddenBannerRowsAtNarrowWidth(t *testing.T) {
	t.Run("reply row remains reserved when Cancel cannot render", func(t *testing.T) {
		model := composerModel(8, 5, writable(9))
		model.ReplyTarget = &app.ReplyTarget{ChatID: 9}
		if got, want := composerTextRect(model, image.Rect(0, 0, 8, 5)), image.Rect(1, 1, 2, 3); !got.Eq(want) {
			t.Fatalf("reply text rect = %v, want %v", got, want)
		}
	})

	t.Run("edit and error rows remain reserved when Cancel cannot render", func(t *testing.T) {
		model := composerModel(8, 5, writable(9))
		model.EditTarget = &app.EditTarget{ChatID: 9, Error: &domain.AppError{Message: "opaque"}}
		if got, want := composerTextRect(model, image.Rect(0, 0, 8, 5)), image.Rect(1, 2, 2, 4); !got.Eq(want) {
			t.Fatalf("edit-error text rect = %v, want %v", got, want)
		}
	})
}

func TestComposerBuilderUsesSharedTextHeight(t *testing.T) {
	source, err := os.ReadFile("surface_composer.go")
	if err != nil {
		t.Fatalf("read surface_composer.go: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "textRect := composerTextRect(model, rect)")
	if start < 0 {
		t.Fatal("composer builder does not consume shared text rectangle")
	}
	tail := body[start:]
	if !strings.Contains(tail, "clipComposerTextView(composerStr, textRect.Dx(), textRect.Dy())") {
		t.Fatal("composer Huh renderer does not consume shared text width/height")
	}
}
