package frontend

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestMainSurfacesRenderApplicationState(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	plain := plainAppView(model)

	for _, want := range []string{
		"telegram-tui", "online", "Chats", "Weekend dev", "Mina Chen",
		"Iris", "Hello from the group", "draft reply", "[Send]", "ⓘ",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("main surface missing %q:\n%s", want, plain)
		}
	}
	if model.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want MouseModeCellMotion", model.View().MouseMode)
	}
}

func TestHistoryViewportShowsExplicitLoadingAndEmptyStates(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	chatID := state.Chats[state.SelectedChat].ID
	state.Messages[chatID] = nil
	state.History[chatID] = HistoryState{Loading: true}
	model.state = &state
	loading := plainAppView(model)
	if !strings.Contains(loading, "Loading messages...") || strings.Contains(loading, "No messages") {
		t.Fatal("history viewport did not render an exclusive loading state")
	}

	state.History[chatID] = HistoryState{Done: true}
	model.state = &state
	if plain := plainAppView(model); !strings.Contains(plain, "No messages") {
		t.Fatal("history viewport omitted its empty state")
	}
	state.History[chatID] = HistoryState{Loading: true}
	state.Messages[chatID] = []domain.Message{{ID: 9001, ChatID: chatID, Kind: domain.MessageText, Text: "existing-history-marker", SentAt: time.Unix(9001, 0)}}
	model.state = &state
	olderLoading := plainAppView(model)
	if !strings.Contains(olderLoading, "Loading older messages...") || !strings.Contains(olderLoading, "existing-history-marker") {
		t.Fatal("non-empty history omitted loading indicator or existing content")
	}
}

func TestResponsiveLayoutsExposeExpectedPane(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		focus         Focus
		want          []string
		dontWant      []string
	}{
		{name: "wide", width: 140, height: 30, focus: FocusConversation, want: []string{"Chats", "Weekend dev"}},
		{name: "normal", width: 100, height: 24, focus: FocusConversation, want: []string{"Chats", "Weekend dev"}},
		{name: "narrow chats", width: 70, height: 22, focus: FocusChats, want: []string{"Chats", "Mina Chen"}, dontWant: []string{"Hello from the group"}},
		{name: "narrow conversation", width: 70, height: 22, focus: FocusConversation, want: []string{"Hello from the group", "[Send]"}, dontWant: []string{"Mina Chen"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := mainSurfaceModel(t, test.width, test.height)
			state := model.Snapshot()
			state.Focus = test.focus
			model.state = &state
			plain := plainAppView(model)
			assertFullWindow(t, plain, test.width, test.height)
			for _, want := range test.want {
				if !strings.Contains(plain, want) {
					t.Errorf("responsive surface missing %q", want)
				}
			}
			for _, unwanted := range test.dontWant {
				if strings.Contains(plain, unwanted) {
					t.Errorf("responsive surface unexpectedly contains %q", unwanted)
				}
			}
		})
	}

	model := mainSurfaceModel(t, 59, 17)
	plain := plainAppView(model)
	assertFullWindow(t, plain, 59, 17)
	if !strings.Contains(plain, "requires at least 60x18; current 59x17") {
		t.Fatalf("too-small surface missing dimensions:\n%s", plain)
	}
}

func TestNarrowChatKeysSeparateActionsFromMessages(t *testing.T) {
	model := mainSurfaceModel(t, 70, 22)
	state := model.Snapshot()
	state.Focus = FocusChats
	model.state = &state
	actions, _ := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	if state := actions.Snapshot(); state.ChatActions == nil || state.Focus != FocusChatActions {
		t.Fatalf("a = focus %v actions %#v", state.Focus, state.ChatActions)
	}
	if plain := plainAppView(actions); !strings.Contains(plain, "Open chat") || strings.Contains(plain, "Hello from the group") {
		t.Fatalf("a did not leave the action modal over the chat list:\n%s", plain)
	}

	model = mainSurfaceModel(t, 70, 22)
	state = model.Snapshot()
	state.Focus = FocusChats
	model.state = &state
	opened, _ := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if state := opened.Snapshot(); state.ChatActions != nil || state.Focus != FocusConversation {
		t.Fatalf("Enter = focus %v actions %#v", state.Focus, state.ChatActions)
	}
	if plain := plainAppView(opened); !strings.Contains(plain, "Hello from the group") {
		t.Fatalf("Enter did not open messages:\n%s", plain)
	}
}

func TestChatInfoAndMessageInputKeys(t *testing.T) {
	for _, width := range []int{70, 100, 140} {
		model := mainSurfaceModel(t, width, 24)
		state := model.Snapshot()
		state.Focus = FocusConversation
		model.state = &state

		info, _ := updateAppModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'K', Text: "K", Mod: tea.ModShift}))
		if got := info.Snapshot(); !got.DetailsOpen || got.Focus != FocusDetails {
			t.Fatalf("width %d: K = details %t focus %v", width, got.DetailsOpen, got.Focus)
		}
		info, _ = updateAppModel(t, info, tea.KeyPressMsg(tea.Key{Code: 'K', Text: "K", Mod: tea.ModShift}))
		if got := info.Snapshot(); got.DetailsOpen || got.Focus != FocusConversation {
			t.Fatalf("width %d: second K = details %t focus %v", width, got.DetailsOpen, got.Focus)
		}
		input, _ := updateAppModel(t, info, tea.KeyPressMsg(tea.Key{Code: 'i', Text: "i"}))
		if got := input.Snapshot(); got.Focus != FocusComposer {
			t.Fatalf("width %d: i = focus %v, want composer", width, got.Focus)
		}
	}
}

func TestModalShellIncludesStatesAndBoundedControls(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.Focus = FocusModal
	state.Modal = &ModalState{Title: "Weekend image", Loading: true, PreviousFocus: FocusConversation}
	model.state = &state
	plain := plainAppView(model)
	for _, want := range []string{"Weekend image", "Loading image...", "×", "╭", "╯"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("loading modal missing %q:\n%s", want, plain)
		}
	}

	state.Modal.Loading = false
	state.Modal.Error = &domain.AppError{Kind: domain.ErrorMedia, Message: "Image unavailable"}
	model.state = &state
	_ = model.View()
	plain = plainAppView(model)
	for _, want := range []string{"Image unavailable", "Retry"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("error modal missing %q:\n%s", want, plain)
		}
	}
	for _, hit := range model.hitRegions() {
		if hit.Rect.Min.X < 0 || hit.Rect.Min.Y < 0 || hit.Rect.Max.X > 100 || hit.Rect.Max.Y > 24 {
			t.Fatalf("modal hit outside window: %v", hit.Rect)
		}
	}
}

func TestMessageSelectionAndActionMenuSurfacesPublishSemanticHits(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.SelectedMessageChat, state.SelectedMessage = 2, 7
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true}}
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "Copy") {
		t.Fatal("action menu did not render Copy")
	}
	var selectHit, copyHit bool
	for _, hit := range model.hitRegions() {
		selectHit = selectHit || (hit.Click.Action == SelectMessage && hit.Click.ChatID == 2 && hit.Click.MessageID == 7)
		copyHit = copyHit || hit.Click.Action == CopyMessage
	}
	if selectHit || !copyHit {
		t.Fatalf("modal semantic hit availability = underlying-select:%t copy:%t", selectHit, copyHit)
	}
}

func TestAuthoritativeMessageActionsLoadingAndErrorNeverRenderEdit(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Loading: true, Capabilities: domain.MessageCapabilities{Copy: true, Edit: true}}
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "Loading actions...") || strings.Contains(plain, "Edit") || !strings.Contains(plain, "Copy") {
		t.Fatalf("loading modal = %q", plain)
	}
	state.MessageMenu.Loading = false
	state.MessageMenu.Error = &domain.AppError{Message: "Could not load message actions"}
	model.state = &state
	plain = plainAppView(model)
	if !strings.Contains(plain, "Actions unavailable") || strings.Contains(plain, "Edit") {
		t.Fatalf("error modal = %q", plain)
	}
}

func TestMessageActionRowsRenderInCanonicalOrderWithForwardAndGating(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true, Forward: true, Edit: true, DeleteForSelf: true, DeleteForAll: true}}
	model.state = &state
	plain := plainAppView(model)
	for _, label := range []string{"Reply", "Forward", "Edit", "Copy", "Delete", "Delete for everyone"} {
		if !strings.Contains(plain, label) {
			t.Fatalf("canonical row %q missing from %q", label, plain)
		}
	}
	positions := []int{
		strings.Index(plain, "Reply"),
		strings.Index(plain, "Forward"),
		strings.Index(plain, "Edit"),
		strings.Index(plain, "Copy"),
		strings.Index(plain, "Delete"),
		strings.Index(plain, "Delete for everyone"),
	}
	for index := 1; index < len(positions); index++ {
		if positions[index-1] >= positions[index] {
			t.Fatalf("row order violated: %q at %d before %q at %d", plain[positions[index-1]:], positions[index-1], plain[positions[index]:], positions[index])
		}
	}

	// Forward is hidden without the capability.
	state.MessageMenu.Capabilities = domain.MessageCapabilities{Copy: true, Reply: true, Edit: true, DeleteForSelf: true, DeleteForAll: true}
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, "Forward") {
		t.Fatalf("Forward row rendered without capability: %q", plain)
	}

	state.MessageMenu.Capabilities = domain.MessageCapabilities{Copy: true, Forward: true, Edit: true, DeleteForSelf: true}
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, "Delete for everyone") {
		t.Fatalf("self-only rows wrong: %q", plain)
	}

	state.MessageMenu.Capabilities = domain.MessageCapabilities{Copy: true, Forward: true, Edit: true, DeleteForAll: true}
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, "Delete for everyone") && strings.Index(plain, "Delete") != strings.Index(plain, "Delete for everyone") {
		t.Fatalf("for-all rows wrong (standalone self-delete leaked): %q", plain)
	}

	state.MessageMenu.Capabilities = domain.MessageCapabilities{Copy: true, Reply: true, Forward: true, Edit: true, DeleteForSelf: true, DeleteForAll: true}
	state.MessageMenu.Loading = true
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, "Forward") {
		t.Fatalf("Forward row rendered while loading: %q", plain)
	}
	state.MessageMenu.Loading = false
	state.MessageMenu.Error = &domain.AppError{Message: "Could not load message actions"}
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, "Forward") {
		t.Fatalf("Forward row rendered on error: %q", plain)
	}
}

func TestPinMessageRowRendersLabelAndGating(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)

	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, " Pin ") || strings.Contains(plain, " Unpin ") {
		t.Fatalf("pin label = %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Pinned: true, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}
	model.state = &state
	plain = plainAppView(model)
	if !strings.Contains(plain, " Unpin ") || strings.Contains(plain, " Pin ") {
		t.Fatalf("unpin label = %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Pinned: true, Loading: true, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}
	model.state = &state
	plain = plainAppView(model)
	if containsTrimmedLine(plain, "Unpin") || containsTrimmedLine(plain, "Pin") || !strings.Contains(plain, "Loading actions...") {
		t.Fatalf("pin row rendered while loading: %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Pinned: true, Error: &domain.AppError{Message: "Could not load message actions"}, Capabilities: domain.MessageCapabilities{Copy: true, Pin: true}}
	model.state = &state
	plain = plainAppView(model)
	if containsTrimmedLine(plain, "Unpin") || containsTrimmedLine(plain, "Pin") || !strings.Contains(plain, "Actions unavailable") {
		t.Fatalf("pin row rendered on error: %q", plain)
	}
}

func TestPinMessageActionModalRowOrderIsBetweenCopyAndDelete(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true, Forward: true, Edit: true, Pin: true, DeleteForSelf: true, DeleteForAll: true}}
	model.state = &state
	lines := strings.Split(plainAppView(model), "\n")
	labels := []string{"Reply", "Forward", "Edit", "Copy", "Pin", "Delete", "Delete for everyone"}
	positions := make([]int, len(labels))
	for index, label := range labels {
		found := -1
		for lineIndex, line := range lines {
			if modalLineHasWord(line, label) {
				found = lineIndex
				break
			}
		}
		if found < 0 {
			t.Fatalf("row %q missing", label)
		}
		positions[index] = found
	}
	for index := 1; index < len(positions); index++ {
		if positions[index-1] >= positions[index] {
			t.Fatalf("row order violated between %q and %q", labels[index-1], labels[index])
		}
	}
}

func TestPinnedMarkerRendersUnderCard(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	chatID := state.Chats[state.SelectedChat].ID
	state.Messages[chatID][0].Pinned = true
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "pinned") {
		t.Fatalf("pinned marker missing from %q", plain)
	}
}

func containsTrimmedLine(plain, label string) bool {
	for _, line := range strings.Split(plain, "\n") {
		if strings.TrimSpace(strings.ReplaceAll(line, "│", "")) == label {
			return true
		}
	}
	return false
}

func modalLineHasWord(line, word string) bool {
	re := regexp.MustCompile(`(^|[^A-Za-z])` + regexp.QuoteMeta(word) + `($|[^A-Za-z])`)
	return re.MatchString(line)
}

func TestForwardPickerSurfaceListsChatTitlesWithoutContentPreview(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.Focus = FocusForwardPicker
	state.ForwardPicker = &ForwardPicker{SourceChatID: 2, SourceMessageID: 7, SelectedChat: 0, RequestID: 5}
	state.MessageMenu = nil
	model.state = &state
	plain := plainAppView(model)
	for _, title := range []string{"Mina Chen", "Weekend dev"} {
		if !strings.Contains(plain, title) {
			t.Fatalf("forward picker missing chat title %q", title)
		}
	}
	if !strings.Contains(plain, "Forward to") {
		t.Fatal("forward picker title missing")
	}
	var forwardHit, destination1, destination2 bool
	underlying := 0
	for _, hit := range model.hitRegions() {
		switch {
		case hit.Click.Action == Activate && hit.Click.ChatID == 1:
			destination1 = true
		case hit.Click.Action == Activate && hit.Click.ChatID == 2:
			destination2 = true
		case hit.Click.Action == FocusChat:
			underlying++
		}
	}
	forwardHit = destination1 || destination2
	if !forwardHit {
		t.Fatal("forward picker did not publish destination hits")
	}
	if underlying != 0 {
		t.Fatalf("forward picker leaked %d underlying chat-list hits", underlying)
	}
	if !destination1 || !destination2 {
		t.Fatalf("destination hit coverage = (1:%t, 2:%t)", destination1, destination2)
	}
}

func TestReactionRowRendersBetweenCopyAndPin(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, CanReact: true, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true, Forward: true, Edit: true, Pin: true, DeleteForSelf: true, DeleteForAll: true}}
	model.state = &state
	lines := strings.Split(plainAppView(model), "\n")
	labels := []string{"Reply", "Forward", "Edit", "Copy", "React", "Pin", "Delete", "Delete for everyone"}
	positions := make([]int, len(labels))
	for index, label := range labels {
		found := -1
		for lineIndex, line := range lines {
			if modalLineHasWord(line, label) {
				found = lineIndex
				break
			}
		}
		if found < 0 {
			t.Fatalf("row %q missing", label)
		}
		positions[index] = found
	}
	for index := 1; index < len(positions); index++ {
		if positions[index-1] >= positions[index] {
			t.Fatalf("row order violated between %q and %q", labels[index-1], labels[index])
		}
	}
}

func TestReactionRowGatedOnLoadingErrorAndLocalCapability(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, CanReact: true, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true}}
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, " React ") {
		t.Fatalf("React row missing when locally capable: %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, CanReact: true, Loading: true, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true}}
	model.state = &state
	plain = plainAppView(model)
	if strings.Contains(plain, " React ") || !strings.Contains(plain, "Loading actions...") {
		t.Fatalf("React row rendered while loading: %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, CanReact: true, Error: &domain.AppError{Message: "Could not load message actions"}, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true}}
	model.state = &state
	plain = plainAppView(model)
	if containsTrimmedLine(plain, "React") || !strings.Contains(plain, "Actions unavailable") {
		t.Fatalf("React row rendered on error: %q", plain)
	}

	state = model.Snapshot()
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, CanReact: false, Capabilities: domain.MessageCapabilities{Copy: true, Reply: true}}
	model.state = &state
	plain = plainAppView(model)
	if containsTrimmedLine(plain, "React") {
		t.Fatalf("React row rendered without local capability: %q", plain)
	}
}

func TestLiveReactionUpdateRendersChipEndToEnd(t *testing.T) {
	fake := telegram.NewFake(telegram.FakeData{})
	updates := make(chan telegram.Update, 4)
	startCtx, cancelStart := context.WithCancel(context.Background())
	defer cancelStart()
	startDone := make(chan error, 1)
	go func() { startDone <- fake.Start(startCtx, updates) }()
	receiveSurfaceUpdate(t, updates) // Ready

	want := telegram.MessageReactionsUpdated{
		ChatID:    9,
		MessageID: 2,
		Reactions: []domain.MessageReaction{{Emoji: "👍", Count: 1, Chosen: true}},
	}
	if err := fake.Emit(context.Background(), want); err != nil {
		t.Fatalf("Emit(MessageReactionsUpdated) error = %v", err)
	}
	got := receiveSurfaceUpdate(t, updates)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emitted update = %#v, want %#v", got, want)
	}

	state := InitialState()
	state.Chats = []domain.Chat{{ID: 9, Kind: domain.ChatPrivate}}
	state.Messages[9] = []domain.Message{{ID: 2, ChatID: 9, Kind: domain.MessageText, Text: "opaque-body"}}
	updateState(&state, TelegramEvent{Value: got})
	if len(state.Messages[9][0].Reactions) != 1 || state.Messages[9][0].Reactions[0].Emoji != "👍" {
		t.Fatalf("reducer did not apply live reactions: %#v", state.Messages[9][0].Reactions)
	}

	groups := GroupMessages(domain.ChatPrivate, state.Messages[9], time.UTC)
	styles := newRenderStyles(false)
	result := buildMessageGroupLayer(RenderedMessageGroup{MessageGroup: groups[0]}, 40, time.UTC, messageSelection{}, nil, styles)
	var chipsLine messageRowSpec
	found := false
	for _, row := range result.Rows {
		if len(row.chips) > 0 {
			chipsLine = row
			found = true
		}
	}
	if !found || chipsLine.text != "👍1" || !chipsLine.chips[0].chosen {
		t.Fatalf("live reaction chip missing: rows=%#v", result.Rows)
	}
}

func receiveSurfaceUpdate(t *testing.T, updates <-chan telegram.Update) telegram.Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for telegram update")
		return nil
	}
}

func TestReactionPickerSurfaceListsPaletteEmojis(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.Focus = FocusReactionPicker
	state.ReactionPicker = &ReactionPicker{ChatID: 2, MessageID: 7, RequestID: 5, Selected: 1}
	state.MessageMenu = nil
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "React") {
		t.Fatal("reaction picker title missing")
	}
	for _, emoji := range ReactionPalette {
		if !strings.Contains(plain, emoji) {
			t.Fatalf("reaction picker missing emoji %q in %q", emoji, plain)
		}
	}
	var activateHits, underlying int
	for _, hit := range model.hitRegions() {
		switch {
		case hit.Click.Action == Activate:
			activateHits++
		case hit.Click.Action == FocusChat:
			underlying++
		}
	}
	if activateHits != len(ReactionPalette) {
		t.Fatalf("reaction picker activate hits = %d, want %d", activateHits, len(ReactionPalette))
	}
	if underlying != 0 {
		t.Fatalf("reaction picker leaked %d underlying chat-list hits", underlying)
	}
}

func TestRoundedMessageSelectionSurroundsFullCard(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.SelectedMessageChat, state.SelectedMessage = 2, 7
	model.state = &state
	lines := strings.Split(plainAppView(model), "\n")
	messageLine := -1
	for index, value := range lines {
		if strings.Contains(value, "Hello from the group") {
			messageLine = index
			break
		}
	}
	if messageLine < 2 || messageLine+1 >= len(lines) || !strings.Contains(lines[messageLine-1], "╭") || !strings.Contains(lines[messageLine], "│") || !strings.Contains(lines[messageLine+1], "╰") {
		t.Fatalf("selected full card lacks rounded border near line %d", messageLine)
	}
}

func TestPrivateMessageMetadataShowsAvatarNameAndTimePerMessage(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.SelectedChat = 0
	state.Messages[1] = []domain.Message{{ID: 1, ChatID: 1, SenderName: "Mina Chen", SentAt: time.Date(2026, time.July, 20, 9, 41, 0, 0, time.Local), Kind: domain.MessageText, Text: "private one"}, {ID: 2, ChatID: 1, Outgoing: true, SentAt: time.Date(2026, time.July, 20, 9, 42, 0, 0, time.Local), Kind: domain.MessageText, Text: "private two"}}
	model.state = &state
	plain := plainAppView(model)
	for _, want := range []string{"Mina Chen  09:41", "You  09:42", "private one", "private two"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("private metadata missing %q", want)
		}
	}
}

func TestReplyComposerBannerClipsAndPublishesCancelHit(t *testing.T) {
	model := mainSurfaceModel(t, 80, 20)
	state := model.Snapshot()
	state.Focus = FocusComposer
	state.ReplyTarget = &ReplyTarget{ChatID: state.Chats[state.SelectedChat].ID, MessageID: 7, Sender: "Sender", Preview: "bounded"}
	state.Drafts[state.ReplyTarget.ChatID] = "first line that wraps across the composer width and a second line"
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "Reply to Sender: bounded") || !strings.Contains(plain, "[Cancel]") || !strings.Contains(plain, "[Send]") {
		t.Fatal("reply composer banner or controls missing")
	}
	cancelHit := false
	for _, hit := range model.hitRegions() {
		cancelHit = cancelHit || hit.Click.Action == CancelReply
	}
	if !cancelHit {
		t.Fatal("reply composer did not publish cancel hit")
	}
}

func TestMessageEditComposerAndEditedMarker(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	chatID := state.Chats[state.SelectedChat].ID
	state.Focus = FocusComposer
	state.Drafts[chatID] = "opaque-ordinary"
	state.EditTarget = &EditTarget{ChatID: chatID, MessageID: 7, Original: "opaque-original", Buffer: "opaque-buffer", Error: &domain.AppError{Message: "Edit failed"}}
	state.Messages[chatID][0].EditedAt = time.Unix(20, 0)
	model.state = &state
	_ = model.syncComposerTextHost()
	plain := plainAppView(model)
	if !strings.Contains(plain, "Editing message") || !strings.Contains(plain, "Edit failed") || !strings.Contains(plain, "edited") || !strings.Contains(plain, "opaque-buffer") || strings.Contains(plain, "opaque-ordinary") {
		t.Fatal("edit composer or marker missing")
	}
	cancelHit := false
	for _, hit := range model.hitRegions() {
		cancelHit = cancelHit || hit.Click.Action == CancelEdit
	}
	if !cancelHit {
		t.Fatal("edit composer did not publish cancel hit")
	}
}

func TestActionModalIsCenteredAndSuppressesUnderlyingHits(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.SelectedMessageChat, state.SelectedMessage = 2, 7
	state.MessageMenu = &MessageActionMenu{ChatID: 2, MessageID: 7, Capabilities: domain.MessageCapabilities{Copy: true}}
	state.Focus = FocusModal
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "Message actions") || !strings.Contains(plain, "Copy") {
		t.Fatal("centered action modal content missing")
	}
	for _, hit := range model.hitRegions() {
		if hit.Click.Action == FocusChat || hit.Click.Action == FocusPane {
			t.Fatalf("action modal retained underlying hit: %#v", hit)
		}
	}
}

func TestToastComponentFloatsAtBottomRightWithoutReservingHistory(t *testing.T) {
	model := mainSurfaceModel(t, 100, 24)
	state := model.Snapshot()
	state.Toast = &domain.AppError{Message: "Message copied"}
	state.ToastGeneration, state.ToastDuration = 1, 2*time.Second
	model.state = &state
	plain := plainAppView(model)
	if !strings.Contains(plain, "Message copied") || !strings.Contains(plain, "Hello from the group") {
		t.Fatal("toast displaced history or did not render")
	}
}

func TestAuthorizationGuidanceParity(t *testing.T) {
	tests := []struct {
		kind auth.PromptKind
		want string
	}{
		{auth.PromptAPIID, "Enter the numeric api_id from my.telegram.org/apps"},
		{auth.PromptAPIHash, "Enter the api_hash from my.telegram.org/apps (saved in local config)"},
		{auth.PromptPhone, "Enter your Telegram phone number, including country code"},
		{auth.PromptCode, "Enter the verification code sent by Telegram"},
		{auth.PromptPassword, "Enter your Telegram two-step verification password"},
		{auth.PromptKind(255), "Type a value and press Enter"},
	}
	for _, test := range tests {
		if got := authorizationPromptGuidance(test.kind); got != test.want {
			t.Errorf("authorizationPromptGuidance(%v) = %q, want %q", test.kind, got, test.want)
		}
	}
}

func TestStaleHitClearedAfterResizeAndModalTopologyChange(t *testing.T) {
	t.Run("resize update then view", func(t *testing.T) {
		model := mainSurfaceModel(t, 100, 24)
		_ = model.View()
		stale := focusChatHit(t, model.hitRegions(), 1)
		model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 59, Height: 17})
		_ = model.View()
		before := model.Snapshot().SelectedChat
		model, _ = updateAppModel(t, model, tea.MouseClickMsg{X: stale.Rect.Min.X, Y: stale.Rect.Min.Y, Button: tea.MouseLeft})
		if got := model.Snapshot().SelectedChat; got != before {
			t.Fatalf("stale resize hit changed selected chat from %d to %d", before, got)
		}
	})

	t.Run("modal view excludes underlying hits", func(t *testing.T) {
		model := mainSurfaceModel(t, 100, 24)
		_ = model.View()
		stale := focusChatHit(t, model.hitRegions(), 1)
		state := model.Snapshot()
		state.Focus = FocusModal
		state.Modal = &ModalState{Title: "Avatar", Loading: true, PreviousFocus: FocusConversation}
		model.state = &state
		_ = model.View()
		for _, hit := range model.hitRegions() {
			if hit.Click.Action == FocusChat || hit.Click.Action == FocusPane {
				t.Fatalf("modal topology retained underlying hit: %#v", hit)
			}
		}
		before := model.Snapshot().SelectedChat
		model, _ = updateAppModel(t, model, tea.MouseClickMsg{X: stale.Rect.Max.X - 1, Y: stale.Rect.Min.Y, Button: tea.MouseLeft})
		state = model.Snapshot()
		if state.SelectedChat != before || state.Modal == nil {
			t.Fatalf("stale modal hit changed topology: selected %d -> %d, modal = %#v", before, state.SelectedChat, state.Modal)
		}
	})
}

func patternedAvatar(t *testing.T, width, height int) pixel.Avatar {
	t.Helper()
	avatar := pixel.Avatar{}
	value := reflect.ValueOf(&avatar).Elem()
	value.FieldByName("Width").SetInt(int64(width))
	value.FieldByName("Height").SetInt(int64(height))
	cellsField := value.FieldByName("Cells")
	cells := reflect.MakeSlice(cellsField.Type(), width*height, width*height)
	for index := 0; index < cells.Len(); index++ {
		cell := cells.Index(index)
		cell.FieldByName("Rune").SetInt(int64('a' + index))
		foreground := cell.FieldByName("Foreground")
		foreground.FieldByName("R").SetUint(uint64(index + 1))
		background := cell.FieldByName("Background")
		background.FieldByName("B").SetUint(uint64(index + 61))
	}
	cellsField.Set(cells)
	return avatar
}

func focusChatHit(t *testing.T, hits HitMap, chatID domain.ChatID) Hit {
	t.Helper()
	for _, hit := range hits {
		if hit.Click.Action == FocusChat && hit.Click.ChatID == chatID {
			return hit
		}
	}
	t.Fatalf("no FocusChat hit for chat %d in %#v", chatID, hits)
	return Hit{}
}

func mainSurfaceModel(t *testing.T, width, height int) AppModel {
	t.Helper()
	state := InitialState()
	state.Width = width
	state.Height = height
	state.Focus = FocusConversation
	state.Connection = domain.ConnectionOnline
	state.ChatsLoaded = true
	state.Chats = []domain.Chat{
		{ID: 1, Kind: domain.ChatPrivate, Title: "Mina Chen", LastMessage: "Ship it", LastMessageAt: time.Date(2026, time.July, 20, 13, 2, 0, 0, time.Local).Unix(), CanSend: true},
		{ID: 2, Kind: domain.ChatSupergroup, Title: "Weekend dev", Username: "weekend_dev", LastMessage: "Hello", LastMessageAt: time.Date(2026, time.July, 20, 14, 35, 0, 0, time.Local).Unix(), UnreadCount: 3, CanSend: true},
	}
	state.SelectedChat = 1
	state.Messages[2] = []domain.Message{{
		ID: 7, ChatID: 2, SenderName: "Iris", SentAt: time.Date(2026, time.July, 20, 14, 30, 0, 0, time.Local), Kind: domain.MessageText, Text: "Hello from the group",
	}}
	state.Drafts[2] = "draft reply"
	updateState(&state, Resized{Width: width, Height: height})
	model := newAppModelForTest(t, state, newTestSession(t))
	_ = model.syncComposerTextHost()
	return model
}

func plainView(content string) string {
	return ansiSequence.ReplaceAllString(content, "")
}

// plainAppView mirrors production's post-Update host state when a test swaps
// the model-owned state directly instead of driving AppModel.Update. View
// itself remains pure and snapshots the owned state exactly once.
func plainAppView(model AppModel) string {
	_ = model.syncListModalController()
	return plainView(model.View().Content)
}

func assertFullWindow(t *testing.T, content string, width, height int) {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) != height {
		t.Fatalf("view line count = %d, want %d", len(lines), height)
	}
	for y, line := range lines {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("view line %d width = %d, want <= %d", y, got, width)
		}
	}
}
