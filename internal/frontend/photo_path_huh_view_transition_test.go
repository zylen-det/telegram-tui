package frontend

import (
	"image"
	"strings"
	"testing"
)

func TestPhotoSendHuhViewInjectionReplacesManualPathAndCursor(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	injected := "\x1b[31mHUH_PHOTO_VISIBLE\x1b[0m\nSECOND_ROW_FORBIDDEN\nTHIRD_ROW_FORBIDDEN"
	surface := buildPhotoSendModalLayer(
		bounds,
		photoSendModalData{Path: "LEGACY_PATH_FORBIDDEN"},
		newRenderStyles(false),
		injected,
	)
	_, canvas := photoSendModalCanvas(bounds, surface)
	rendered := plainText(canvas.Render())
	if !strings.Contains(rendered, "HUH_PHOTO_VISIBLE") {
		t.Fatalf("injected Huh Photo View is missing\n%s", rendered)
	}
	for _, forbidden := range []string{"LEGACY_PATH_FORBIDDEN", "SECOND_ROW_FORBIDDEN", "THIRD_ROW_FORBIDDEN"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Photo input rendered forbidden %q\n%s", forbidden, rendered)
		}
	}
	if surface.Cursor.Visible {
		t.Fatalf("injected Huh Photo View retained terminal cursor: %+v", surface.Cursor)
	}
}

func TestPhotoSendEmptyInjectedHuhViewNeverFallsBack(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	surface := buildPhotoSendModalLayer(
		bounds,
		photoSendModalData{Path: "EMPTY_INJECTION_MUST_NOT_FALL_BACK"},
		newRenderStyles(false),
		"",
	)
	_, canvas := photoSendModalCanvas(bounds, surface)
	if rendered := plainText(canvas.Render()); strings.Contains(rendered, "EMPTY_INJECTION_MUST_NOT_FALL_BACK") {
		t.Fatalf("empty injected Huh View fell back to manual path\n%s", rendered)
	}
	if surface.Cursor.Visible {
		t.Fatalf("empty injected Huh View retained terminal cursor: %+v", surface.Cursor)
	}
}

func TestPhotoSendStandaloneOmittedViewRetainsCompatibility(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	surface := buildPhotoSendModalLayer(
		bounds,
		photoSendModalData{Path: "STANDALONE_MANUAL_PATH"},
		newRenderStyles(false),
	)
	_, canvas := photoSendModalCanvas(bounds, surface)
	if rendered := plainText(canvas.Render()); !strings.Contains(rendered, "STANDALONE_MANUAL_PATH") {
		t.Fatalf("omitted injection lost standalone manual path\n%s", rendered)
	}
	if !surface.Cursor.Visible {
		t.Fatal("omitted injection lost standalone compatibility cursor")
	}
}

func TestAppModelPhotoPathHuhViewFlowsThroughProductionComposition(t *testing.T) {
	state := photoPathRoutingState("HUH_APP_PHOTO_VISIBLE")
	state.Width, state.Height = 100, 24
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncPhotoPathInputHost()

	view := model.View()
	if rendered := plainText(view.Content); !strings.Contains(rendered, "HUH_APP_PHOTO_VISIBLE") {
		t.Fatalf("AppModel Photo Huh View is not visible\n%s", rendered)
	}
	if view.Cursor != nil {
		t.Fatalf("production Photo Huh View declares terminal cursor: %+v", view.Cursor)
	}
}
