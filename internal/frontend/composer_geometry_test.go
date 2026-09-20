package frontend

import (
	"fmt"
	"image"
	"os"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComposerSurfaceRectMatchesConversationSplit(t *testing.T) {
	model := ViewModel{
		Width:      120,
		Height:     30,
		Focus:      FocusComposer,
		ActiveChat: domain.Chat{ID: 9, CanSend: true},
	}
	model.Layout = ComputeLayout(model.Width, model.Height, false, model.Focus)

	if got, want := composerSurfaceRect(model), image.Rect(33, 26, 119, 29); !got.Eq(want) {
		t.Fatalf("composer surface rect = %v, want %v", got, want)
	}

	model.Layout.Conversation = image.Rect(0, 0, 1, 2)
	if got := composerSurfaceRect(model); !got.Empty() {
		t.Fatalf("tiny conversation produced composer rect %v", got)
	}
}

func TestComposerTextRectUsesControlsBannersAndAvailableRows(t *testing.T) {
	testCases := []struct {
		name  string
		model ViewModel
		rect  image.Rectangle
		want  image.Rectangle
	}{
		{
			name:  "writable controls reserve width",
			model: composerModel(20, 5, writable(9)),
			rect:  image.Rect(0, 0, 20, 5),
			want:  image.Rect(1, 0, 5, 2),
		},
		{
			name: "reply banner advances top",
			model: func() ViewModel {
				model := composerModel(20, 5, writable(9))
				model.ReplyTarget = &ReplyTarget{ChatID: 9}
				return model
			}(),
			rect: image.Rect(0, 0, 20, 5),
			want: image.Rect(1, 1, 5, 3),
		},
		{
			name: "edit error leaves one row",
			model: func() ViewModel {
				model := composerModel(20, 3, writable(9))
				model.EditTarget = &EditTarget{ChatID: 9, Error: &domain.AppError{Message: "opaque"}}
				return model
			}(),
			rect: image.Rect(0, 0, 20, 3),
			want: image.Rect(1, 2, 5, 3),
		},
		{
			name:  "nonzero origin stays absolute",
			model: composerModel(100, 40, writable(9)),
			rect:  image.Rect(15, 7, 60, 12),
			want:  image.Rect(16, 7, 35, 9),
		},
		{
			name:  "no active chat has no text",
			model: composerModel(20, 5, domain.Chat{}),
			rect:  image.Rect(0, 0, 20, 5),
			want:  image.Rectangle{},
		},
		{
			name:  "read only has no text",
			model: composerModel(20, 5, readOnly(9)),
			rect:  image.Rect(0, 0, 20, 5),
			want:  image.Rectangle{},
		},
		{
			name:  "one cell width has no text",
			model: composerModel(20, 5, writable(9)),
			rect:  image.Rect(3, 2, 4, 5),
			want:  image.Rectangle{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if got := composerTextRect(test.model, test.rect); !got.Eq(test.want) {
				t.Fatalf("composer text rect = %v, want %v", got, test.want)
			}
		})
	}
}

func TestComposerTextRectStopsBeforeVisibleControlsWideAndNarrow(t *testing.T) {
	for _, width := range []int{60, 30} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			model := composerModel(width, 3, writable(9))
			rect := image.Rect(0, 0, width, 3)
			textRect := composerTextRect(model, rect)
			surface := buildComposerLayer(model, rect, newRenderStyles(false))
			seen := map[string]bool{}
			for _, interaction := range surface.Interactions {
				if interaction.ID != "composer:sticker" && interaction.ID != "composer:photo" && interaction.ID != "composer:send" {
					continue
				}
				seen[interaction.ID] = true
				if textRect.Overlaps(interaction.Rect) || textRect.Max.X > interaction.Rect.Min.X {
					t.Errorf("text rect %v overlaps/reaches control %s %v", textRect, interaction.ID, interaction.Rect)
				}
			}
			for _, id := range []string{"composer:sticker", "composer:photo", "composer:send"} {
				if !seen[id] {
					t.Errorf("missing control hit %q", id)
				}
			}
		})
	}
}

func TestComposerGeometryIsUsedByProductionBuilders(t *testing.T) {
	conversation, err := os.ReadFile("surface_conversation.go")
	if err != nil {
		t.Fatalf("read surface_conversation.go: %v", err)
	}
	if !strings.Contains(string(conversation), "composerSurfaceRect(model)") {
		t.Fatal("conversation builder does not consume shared composer surface geometry")
	}

	composer, err := os.ReadFile("surface_composer.go")
	if err != nil {
		t.Fatalf("read surface_composer.go: %v", err)
	}
	if !strings.Contains(string(composer), "composerTextRect(model, rect)") {
		t.Fatal("composer builder does not consume shared composer text geometry")
	}
}
