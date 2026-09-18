package ui

import (
	"image"
	"image/color"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v3"
	gotui "github.com/metaspartan/gotui/v5"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/testutil"
)

func TestRootClearsEveryFrameAndHandlesEmptyBuffers(t *testing.T) {
	root := NewRoot()
	model := rootFixture(100, 24)
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, 100, 24))
	root.Draw(buffer)
	if !strings.Contains(testutil.BufferText(buffer), "Weekend") {
		t.Fatal("first draw does not contain active title")
	}

	empty := ViewModel{Width: 100, Height: 24, Focus: app.FocusChats}
	empty.Layout = ComputeLayout(empty.Width, empty.Height, false, empty.Focus)
	root.Update(empty)
	root.Draw(buffer)
	if got := testutil.BufferText(buffer); strings.Contains(got, "Weekend") || strings.Contains(got, "draft") {
		t.Fatalf("second draw retained old content:\n%s", got)
	}

	root.Draw(nil)
	root.Draw(gotui.NewBuffer(image.Rectangle{}))
}

func TestRootSecretPromptNeverRendersSecretValue(t *testing.T) {
	model := rootFixture(100, 24)
	model.Focus = app.FocusAuth
	model.Prompt = &app.PromptState{
		Prompt: auth.Prompt{ID: 7, Kind: auth.PromptPassword, Label: "Password", Secret: true},
		Input:  []rune("private-value"),
	}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, 100, 24))
	root.Draw(buffer)
	text := testutil.BufferText(buffer)
	if strings.Contains(text, "private-value") {
		t.Fatalf("secret value leaked into buffer:\n%s", text)
	}
	if !strings.Contains(text, strings.Repeat("•", len(model.Prompt.Input))) {
		t.Fatalf("secret bullets not rendered:\n%s", text)
	}
}

func TestSpecificAvatarRetryHitWinsOverChatRow(t *testing.T) {
	model := rootFixture(100, 24)
	model.Focus = app.FocusChats
	model.Layout = ComputeLayout(model.Width, model.Height, false, model.Focus)
	model.Chats[0].AvatarError = &domain.AppError{Kind: domain.ErrorMedia, Message: "avatar failed"}
	root := renderRoot(t, model)

	var retry Hit
	found := false
	for _, hit := range root.Hits() {
		if hit.Click.Action == app.Retry && hit.Click.AvatarKey == model.Chats[0].AvatarKey {
			retry = hit
			found = true
			break
		}
	}
	if !found || retry.Rect.Dx() != 6 || retry.Rect.Dy() != 3 {
		t.Fatalf("chat avatar retry hit = %#v, found=%t", retry, found)
	}
	got, ok := root.Hits().ActionAt(retry.Rect.Min.X, retry.Rect.Min.Y)
	if !ok || got.Action != app.Retry || got.AvatarKey != model.Chats[0].AvatarKey {
		t.Fatalf("avatar point action = (%#v, %t), want Retry", got, ok)
	}
}

func TestConversationInfoControlTogglesClosedDetails(t *testing.T) {
	model := rootFixture(100, 24)
	if model.DetailsOpen {
		t.Fatal("fixture details unexpectedly open")
	}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)

	var control Hit
	found := false
	for _, hit := range root.Hits() {
		if hit.Click.Action == app.ToggleDetails {
			control = hit
			found = true
			break
		}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	if !found || control.Rect.Empty() || !control.Rect.In(bounds) {
		t.Fatalf("conversation info hit = %#v, found=%t, want bounded ToggleDetails", control, found)
	}
	if got := buffer.GetCell(control.Rect.Min).Rune; got != 'ⓘ' {
		t.Fatalf("conversation info cell = %q, want ⓘ", got)
	}
	got, ok := root.Hits().ActionAt(control.Rect.Min.X, control.Rect.Min.Y)
	if !ok || got.Action != app.ToggleDetails {
		t.Fatalf("conversation info action = (%#v, %t), want ToggleDetails", got, ok)
	}
}

func TestChatLoadStatusKeepsCachedRowsVisible(t *testing.T) {
	for _, model := range []ViewModel{
		func() ViewModel {
			model := rootFixture(100, 24)
			model.ChatsLoading = true
			return model
		}(),
		func() ViewModel {
			model := rootFixture(100, 24)
			model.ChatsError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "Refresh failed"}
			return model
		}(),
	} {
		root := NewRoot()
		root.Update(model)
		buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
		root.Draw(buffer)
		if text := testutil.BufferText(buffer); !strings.Contains(text, "Mina Chen") {
			t.Fatalf("cached row hidden by load status:\n%s", text)
		}
	}
}

func TestChatsViewportKeepsSelectedRowVisible(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*ViewModel)
		wantChatID []domain.ChatID
	}{
		{name: "normal", configure: func(*ViewModel) {}, wantChatID: []domain.ChatID{3, 4, 5, 6, 7}},
		{name: "loading", configure: func(model *ViewModel) { model.ChatsLoading = true }, wantChatID: []domain.ChatID{4, 5, 6, 7}},
		{name: "error", configure: func(model *ViewModel) {
			model.ChatsError = &domain.AppError{Kind: domain.ErrorNetwork, Message: "Refresh failed"}
		}, wantChatID: []domain.ChatID{4, 5, 6, 7}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := rootFixture(100, 24)
			model.Layout.Chats = image.Rect(0, 1, 30, 23)
			model.Chats = make([]ChatRow, 7)
			for index := range model.Chats {
				model.Chats[index] = ChatRow{Chat: domain.Chat{ID: domain.ChatID(index + 1), Title: "chat"}}
			}
			model.Chats[6].Chat.Title = "Selected target"
			model.Chats[6].Selected = true
			model.Chats[6].Avatar = fixtureAvatar(6, 3, color.NRGBA{R: 250, G: 11, B: 17, A: 255}, color.NRGBA{A: 255})
			test.configure(&model)

			root := NewRoot()
			root.Update(model)
			buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
			root.Draw(buffer)
			if text := testutil.BufferText(buffer); !strings.Contains(text, "Selected target") {
				t.Fatalf("selected title is outside viewport:\n%s", text)
			}
			if got := countForeground(buffer, model.Layout.Chats, gotui.NewRGBColor(250, 11, 17)); got != 6*3 {
				t.Fatalf("selected avatar cells = %d, want %d", got, 6*3)
			}
			var gotChatID []domain.ChatID
			for _, hit := range root.Hits() {
				if hit.Click.Action == app.SelectChat {
					gotChatID = append(gotChatID, hit.Click.ChatID)
				}
			}
			if !reflect.DeepEqual(gotChatID, test.wantChatID) {
				t.Fatalf("visible chat hit IDs = %v, want %v", gotChatID, test.wantChatID)
			}
		})
	}
}

func TestConversationDrawsOneAvatarPerIncomingGroup(t *testing.T) {
	model := groupBoundaryFixture()
	avatarColor := gotui.NewRGBColor(190, 80, 60)
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)

	if got := countForeground(buffer, model.Layout.Conversation, avatarColor); got != 2*4*2 {
		t.Fatalf("group avatar cells = %d, want %d (one 4x2 avatar per group)", got, 2*4*2)
	}
}

func TestIncomingGroupWrapDoesNotDropBoundaryGrapheme(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghij"
	model := rootFixture(100, 24)
	model.Groups = []RenderedMessageGroup{{MessageGroup: MessageGroup{
		SenderName: "Iris", ShowAvatar: true,
		Messages: []domain.Message{{ID: 80, Kind: domain.MessageText, Text: token, SentAt: fixtureTime(18, 0)}},
	}}}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	inner := insetRectangle(model.Layout.Conversation, 1)
	textX := inner.Min.X + 5
	bodyBottom := inner.Max.Y - 3
	first := strings.TrimSpace(cellsText(buffer, image.Rect(textX, bodyBottom-2, inner.Max.X, bodyBottom-1)))
	second := strings.TrimSpace(cellsText(buffer, image.Rect(textX, bodyBottom-1, inner.Max.X, bodyBottom)))
	if first+second != token {
		t.Fatalf("wrapped token was changed or dropped:\n%s", testutil.BufferText(buffer))
	}
}

func TestFullWidthSendStateMarkersRemainVisible(t *testing.T) {
	model := rootFixture(100, 24)
	inner := insetRectangle(model.Layout.Conversation, 1)
	historyWidth := inner.Dx()
	model.Groups = []RenderedMessageGroup{
		{
			MessageGroup: MessageGroup{
				SenderName: "Iris", ShowAvatar: true,
				Messages: []domain.Message{{
					ID: 201, Kind: domain.MessageText, Text: strings.Repeat("I", historyWidth-5),
					SentAt: fixtureTime(19, 0), SendState: domain.SendPending,
				}},
			},
		},
		{
			MessageGroup: MessageGroup{
				Messages: []domain.Message{{
					ID: 202, Kind: domain.MessageText, Text: strings.Repeat("O", historyWidth-2),
					SentAt: fixtureTime(19, 1), Outgoing: true, SendState: domain.SendFailed,
				}},
			},
		},
	}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	text := testutil.BufferText(buffer)
	if !strings.ContainsRune(text, '…') || !strings.ContainsRune(text, '!') {
		t.Fatalf("full-width send state markers are missing:\n%s", text)
	}
	foundFailed := false
	for y := inner.Min.Y; y < inner.Max.Y-3; y++ {
		for x := inner.Min.X; x < inner.Max.X; x++ {
			cell := buffer.GetCell(image.Pt(x, y))
			if cell.Rune != '!' {
				continue
			}
			foundFailed = true
			if cell.Style.Fg != errorColor || x != inner.Max.X-2 {
				t.Fatalf("failed marker at (%d,%d) style=%#v, want error style right-aligned", x, y, cell.Style)
			}
		}
	}
	if !foundFailed {
		t.Fatal("failed marker cell not found")
	}
}

func TestPrivateAndOutgoingMessagesDoNotDrawMessageAvatar(t *testing.T) {
	tests := []struct {
		name string
		kind domain.ChatKind
	}{
		{name: "private", kind: domain.ChatPrivate},
		{name: "outgoing group", kind: domain.ChatSupergroup},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := rootFixture(100, 24)
			model.ActiveChat.Kind = test.kind
			model.Groups = []RenderedMessageGroup{{MessageGroup: MessageGroup{
				ShowAvatar: false,
				Messages:   []domain.Message{{ID: 90, Kind: domain.MessageText, Text: "no avatar", Outgoing: test.name == "outgoing group", SentAt: fixtureTime(12, 0)}},
			}}}
			unique := gotui.NewRGBColor(251, 7, 13)
			model.Groups[0].Avatar = fixtureAvatar(4, 2, color.NRGBA{R: 251, G: 7, B: 13, A: 255}, color.NRGBA{A: 255})
			root := NewRoot()
			root.Update(model)
			buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
			root.Draw(buffer)
			if got := countForeground(buffer, model.Layout.Conversation, unique); got != 0 {
				t.Fatalf("message avatar cells = %d, want 0", got)
			}
		})
	}
}

func TestModalCloseHitsExcludeModalInteriorAndDoNotOverlap(t *testing.T) {
	model := modalFixture(false, nil)
	root := renderRoot(t, model)
	hits := root.Hits()
	center := image.Pt(model.Width/2, model.Height/2)
	if got, ok := hits.ActionAt(center.X, center.Y); ok && got.Action == app.Close {
		t.Fatalf("modal interior maps to Close: %#v", got)
	}
	if got, ok := hits.ActionAt(0, 0); !ok || got.Action != app.Close {
		t.Fatalf("outside modal action = (%#v, %t), want Close", got, ok)
	}

	closeRects := make([]image.Rectangle, 0, 5)
	for _, hit := range hits {
		if hit.Click.Action == app.Close {
			closeRects = append(closeRects, hit.Rect)
		}
	}
	if len(closeRects) != 5 {
		t.Fatalf("Close hit count = %d, want top-right plus four outside regions", len(closeRects))
	}
	for i := range closeRects {
		for j := i + 1; j < len(closeRects); j++ {
			if !closeRects[i].Intersect(closeRects[j]).Empty() {
				t.Errorf("Close hits overlap: %v and %v", closeRects[i], closeRects[j])
			}
		}
	}
}

func TestModalErrorRetryHasNoAvatarKey(t *testing.T) {
	model := modalFixture(false, &domain.AppError{Kind: domain.ErrorMedia, Message: "retry image"})
	root := renderRoot(t, model)
	for _, hit := range root.Hits() {
		if hit.Click.Action != app.Retry {
			continue
		}
		if hit.Click.AvatarKey != "" {
			t.Fatalf("modal retry AvatarKey = %q, want empty", hit.Click.AvatarKey)
		}
		return
	}
	t.Fatal("modal Retry hit missing")
}

func TestReadOnlyConversationHasNoSubmitHit(t *testing.T) {
	model := readOnlyFixture()
	root := renderRoot(t, model)
	for _, hit := range root.Hits() {
		if hit.Click.Action == app.ComposerSubmit {
			t.Fatalf("read-only conversation has submit hit: %#v", hit)
		}
	}
}

func TestReadOnlyComposerKeepsConversationFocus(t *testing.T) {
	model := readOnlyFixture()
	root := renderRoot(t, model)
	inner := insetRectangle(model.Layout.Conversation, 1)
	point := image.Pt(inner.Min.X+1, inner.Max.Y-2)
	got, ok := root.Hits().ActionAt(point.X, point.Y)
	if !ok || got.Action != app.FocusPane || got.TargetFocus != app.FocusConversation {
		t.Fatalf("read-only composer action = (%#v, %t), want FocusConversation", got, ok)
	}
}

func TestNoActiveChatGatesConversationControls(t *testing.T) {
	model := chatsStateFixture("empty")
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	if text := testutil.BufferText(buffer); !strings.Contains(text, "No conversation") || strings.ContainsRune(text, 'ⓘ') {
		t.Fatalf("empty conversation state is not neutral:\n%s", text)
	}
	for _, hit := range root.Hits() {
		if hit.Click.Action == app.ToggleDetails || hit.Click.Action == app.ComposerSubmit ||
			(hit.Click.Action == app.FocusPane && hit.Click.TargetFocus == app.FocusComposer) {
			t.Fatalf("inactive conversation exposes control: %#v", hit)
		}
	}
}

func TestNoActiveChatDetailsOnlyOffersClose(t *testing.T) {
	model := chatsStateFixture("empty")
	model.DetailsOpen = true
	model.Focus = app.FocusDetails
	model.Layout = ComputeLayout(model.Width, model.Height, true, model.Focus)
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	if text := testutil.BufferText(buffer); !strings.Contains(text, "No conversation") || strings.Contains(text, "View image") {
		t.Fatalf("empty details state exposes profile action:\n%s", text)
	}
	toggleCount := 0
	for _, hit := range root.Hits() {
		if hit.Click.Action == app.Activate {
			t.Fatalf("empty details exposes Activate: %#v", hit)
		}
		if hit.Click.Action == app.ToggleDetails {
			toggleCount++
		}
	}
	if toggleCount != 1 {
		t.Fatalf("empty details ToggleDetails hits = %d, want close only", toggleCount)
	}
}

func TestToastDoesNotCoverNewestMessage(t *testing.T) {
	model := rootFixture(100, 24)
	model.Groups = []RenderedMessageGroup{{MessageGroup: MessageGroup{Messages: []domain.Message{{
		ID: 101, Kind: domain.MessageText, Text: "newest-visible-message", SentAt: fixtureTime(17, 0),
	}}}}}
	model.Toast = &domain.AppError{Kind: domain.ErrorNetwork, Message: "Temporarily offline"}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	text := testutil.BufferText(buffer)
	if !strings.Contains(text, "newest-visible-message") || !strings.Contains(text, "[network] Temporarily offline") {
		t.Fatalf("message and toast must both remain visible:\n%s", text)
	}
}

func TestHitsAreBoundedSpecificAndCopied(t *testing.T) {
	model := rootFixture(100, 24)
	root := renderRoot(t, model)
	bounds := image.Rect(0, 0, model.Width, model.Height)
	hits := root.Hits()
	assertHitsBounded(t, hits, bounds)
	if len(hits) == 0 {
		t.Fatal("Hits() is empty")
	}
	original := hits[0].Rect
	hits[0].Rect = image.Rect(-100, -100, -1, -1)
	if got := root.Hits()[0].Rect; got != original {
		t.Fatalf("Hits() returned shared storage: got %v, want %v", got, original)
	}
}

func TestRootHandleEventIsRunnerOwned(t *testing.T) {
	if NewRoot().HandleEvent(gotui.Event{Type: gotui.KeyboardEvent, ID: "x"}) {
		t.Fatal("HandleEvent() = true, want false")
	}
}

func TestRootUpdateDrawAndHitsAreRaceSafe(t *testing.T) {
	root := NewRoot()
	models := []ViewModel{rootFixture(100, 24), detailsFixture(), modalFixture(true, nil), rootFixture(70, 22)}
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				model := models[(worker+iteration)%len(models)]
				root.Update(model)
				buffer := gotui.NewBuffer(image.Rect(0, 0, max(140, model.Width), max(30, model.Height)))
				root.Draw(buffer)
				_ = root.Hits()
			}
		}(worker)
	}
	wait.Wait()
}

func TestModalDimsUnderlyingSurface(t *testing.T) {
	model := modalFixture(true, nil)
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	cell := buffer.GetCell(image.Pt(0, 0))
	if cell.Style.Modifier&tcell.AttrDim == 0 {
		t.Fatalf("underlying status cell modifier = %v, want dim", cell.Style.Modifier)
	}
}

func TestModalDrawsAboveToast(t *testing.T) {
	model := detailsFixture()
	model.Focus = app.FocusModal
	model.Modal = &app.ModalState{Title: "Avatar", Loading: true, PreviousFocus: app.FocusDetails}
	model.Toast = &domain.AppError{Kind: domain.ErrorNetwork, Message: "TOAST_OVERLAY_SENTINEL"}
	root := NewRoot()
	root.Update(model)
	buffer := gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height))
	root.Draw(buffer)
	if text := testutil.BufferText(buffer); strings.Contains(text, "TOAST_OVERLAY_SENTINEL") {
		t.Fatalf("toast was drawn above modal:\n%s", text)
	}
}

func renderRoot(t *testing.T, model ViewModel) *Root {
	t.Helper()
	root := NewRoot()
	root.Update(model)
	root.Draw(gotui.NewBuffer(image.Rect(0, 0, model.Width, model.Height)))
	return root
}

func countForeground(buffer *gotui.Buffer, rectangle image.Rectangle, foreground gotui.Color) int {
	count := 0
	for y := rectangle.Min.Y; y < rectangle.Max.Y; y++ {
		for x := rectangle.Min.X; x < rectangle.Max.X; x++ {
			if buffer.GetCell(image.Pt(x, y)).Style.Fg == foreground {
				count++
			}
		}
	}
	return count
}
