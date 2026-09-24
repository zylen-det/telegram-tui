package frontend

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestVideoExternalOpenAcceptance_ActionMenuLabelsAndSemanticParity(t *testing.T) {
	opened := videoExternalOpenFrontendState(domain.MessageVideo)
	updateState(&opened, ActionReceived{Action: OpenMessageActionMenu})
	if opened.MessageMenu == nil {
		t.Fatal("received Video did not open a message action menu")
	}
	options := selectorOptionsFromRows(messageActionRows(opened.MessageMenu))
	if len(options) == 0 || options[0].ID != "action:open-video" || options[0].Label != "Open video" {
		t.Fatalf("Video selector options = %#v, want Open video first", options)
	}
	wantAction := ActionReceived{Action: ViewMessageMedia, ChatID: 9, MessageID: 77}
	if options[0].Value != wantAction {
		t.Fatalf("Video selector action = %#v, want %#v", options[0].Value, wantAction)
	}

	model := Select(opened, time.UTC)
	styles := newRenderStyles(false)
	fallback := buildActionModalLayer(model, styles)
	if fallback.Layer == nil {
		t.Fatal("Video fallback action modal has no layer")
	}
	root := lipgloss.NewLayer(styles.Base.Width(model.Width).Height(model.Height).Render("")).X(0).Y(0).Z(zFrame)
	root.AddLayers(fallback.Layer)
	text := plainText(lipgloss.NewCanvas(model.Width, model.Height).Compose(lipgloss.NewCompositor(root)).Render())
	if !strings.Contains(text, "Open video") || strings.Contains(text, "View image") {
		t.Fatalf("Video fallback modal text = %q", text)
	}
	foundHit := false
	for _, interaction := range fallback.Interactions {
		if interaction.ID == "action:open-video" {
			foundHit = interaction.Primary == wantAction
		}
	}
	if !foundHit {
		t.Fatalf("Video fallback modal interactions = %#v, want semantic Open video hit", fallback.Interactions)
	}
}

func TestVideoExternalOpenAcceptance_PhotoActionLabelUnchanged(t *testing.T) {
	opened := videoExternalOpenFrontendState(domain.MessagePhoto)
	updateState(&opened, ActionReceived{Action: OpenMessageActionMenu})
	options := selectorOptionsFromRows(messageActionRows(opened.MessageMenu))
	if len(options) == 0 || options[0].ID != "action:view-image" || options[0].Label != "View image" {
		t.Fatalf("Photo selector options = %#v, want existing View image first", options)
	}
}

func videoExternalOpenFrontendState(kind domain.MessageKind) State {
	state := InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{{
		ID: 77, ChatID: 9, Kind: kind,
		Media: domain.MessageMedia{File: domain.MediaFileRef{ID: 701, CanDownload: true}},
	}}
	state.SelectedMessageChat, state.SelectedMessage = 9, 77
	return state
}
