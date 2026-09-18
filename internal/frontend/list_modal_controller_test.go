package frontend

import (
	"image"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// selectorSyncState is a loading message action menu that also asks for the
// bounded PreferEdit override, used by the selector route tests.
func selectorSyncState() app.State {
	state := app.InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = app.FocusModal
	state.MessageMenu = &app.MessageActionMenu{
		RequestID:    7,
		ChatID:       9,
		MessageID:    2,
		Loading:      true,
		PreferEdit:   true,
		Capabilities: domain.MessageCapabilities{Copy: true},
	}
	return state
}

func TestListModalControllerOwnsPersistentHost(t *testing.T) {
	model := newAppModelForTest(t, app.NewEngine(app.InitialState()), newBoundedAppRuntimeForModelTest(t))
	if model.listModals.host == nil || model.listModals.host.field == nil {
		t.Fatal("NewAppModel selector host is nil")
	}
	copied := model
	if copied.listModals.host != model.listModals.host {
		t.Fatal("AppModel value copy replaced persistent selector host")
	}
	other := newAppModelForTest(t, app.NewEngine(app.InitialState()), newBoundedAppRuntimeForModelTest(t))
	if other.listModals.host == model.listModals.host {
		t.Fatal("independent AppModels share selector host")
	}
}

func TestListModalControllerSynchronizesMessageLifecycleAndPreferEdit(t *testing.T) {
	state := selectorSyncState()
	engine := app.NewEngine(state)
	model := newAppModelForTest(t, engine, newBoundedAppRuntimeForModelTest(t))
	_ = model.syncListModalController()

	loadingOptions := selectorOptionsFromRows(messageActionRows(state.MessageMenu))
	wantIdentity := selectorIdentity{Kind: selectorMessageActions, RequestID: 7, ChatID: 9, MessageID: 2}
	wantRect := selectorHostRect(image.Rect(0, 0, 80, 24), len(loadingOptions), len(loadingOptions)+1)
	if model.listModals.host.Identity() != wantIdentity || model.listModals.host.Value() != loadingOptions[0].Value || !model.listModals.host.focused {
		t.Fatalf("loading message selector = id:%#v value:%#v focus:%t", model.listModals.host.Identity(), model.listModals.host.Value(), model.listModals.host.focused)
	}
	if model.listModals.host.width != wantRect.Dx() || model.listModals.host.height != wantRect.Dy() {
		t.Fatalf("loading message geometry = %dx%d, want %dx%d", model.listModals.host.width, model.listModals.host.height, wantRect.Dx(), wantRect.Dy())
	}

	// The async properties result introduces Edit while Copy remains present.
	// PreferEdit must intentionally override the generic reorder-preservation
	// policy exactly for this transition.
	model, _ = updateAppModel(t, model, appEventMsg{event: app.MessagePropertiesLoaded{
		RequestID:    7,
		ChatID:       9,
		MessageID:    2,
		Capabilities: domain.MessageCapabilities{Copy: true, Edit: true},
	}})
	settled := engine.Snapshot()
	settledOptions := selectorOptionsFromRows(messageActionRows(settled.MessageMenu))
	edit := app.ActionReceived{Action: app.EditMessage, ChatID: 9, MessageID: 2}
	if model.listModals.host.Value() != edit || selectorOptionIndex(settledOptions, edit) < 0 {
		t.Fatalf("PreferEdit properties transition selected %#v, want Edit", model.listModals.host.Value())
	}

	// PreferEdit is not a permanent force mode. Once Edit is already present,
	// an option-order change with a stale legacy index must preserve the user's
	// current semantic Copy selection.
	copyValue := app.ActionReceived{Action: app.CopyMessage, ChatID: 9, MessageID: 2}
	settled.MessageMenu.Selected = selectorOptionIndex(settledOptions, copyValue)
	model.engine = app.NewEngine(settled)
	_ = model.syncListModalController()
	if model.listModals.host.Value() != copyValue {
		t.Fatalf("unchanged settled selector ignored authoritative Copy: %#v", model.listModals.host.Value())
	}
	settled.MessageMenu.MediaFile = domain.MediaFileRef{ID: 44, CanDownload: true}
	// Selected remains the old Copy index and now points at Edit after View image
	// is inserted. Semantic Copy is still present and must survive.
	model.engine = app.NewEngine(settled)
	_ = model.syncListModalController()
	if model.listModals.host.Value() != copyValue {
		t.Fatalf("PreferEdit permanently forced stale reordered index: %#v", model.listModals.host.Value())
	}
}

func TestListModalControllerSynchronizesReactionForwardReorderAndClose(t *testing.T) {
	state := app.InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = app.FocusReactionPicker
	state.ReactionPicker = &app.ReactionPicker{RequestID: 8, ChatID: 9, MessageID: 2, Selected: 2}
	model := newAppModelForTest(t, app.NewEngine(state), newBoundedAppRuntimeForModelTest(t))
	_ = model.syncListModalController()
	reactionOptions := selectorOptionsFromRows(reactionRows(state.ReactionPicker))
	if model.listModals.host.Identity() != (selectorIdentity{Kind: selectorReaction, RequestID: 8, ChatID: 9, MessageID: 2}) || model.listModals.host.Value() != reactionOptions[2].Value {
		t.Fatalf("reaction selector = id:%#v value:%#v", model.listModals.host.Identity(), model.listModals.host.Value())
	}

	state.Focus = app.FocusForwardPicker
	state.ReactionPicker = nil
	state.ForwardPicker = &app.ForwardPicker{RequestID: 9, SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1}
	state.Chats = []domain.Chat{{ID: 11, Title: "A"}, {ID: 22, Title: "B"}}
	model.engine = app.NewEngine(state)
	_ = model.syncListModalController()
	if model.listModals.host.Identity() != (selectorIdentity{Kind: selectorForward, RequestID: 9, ChatID: 9, MessageID: 2}) || model.listModals.host.Value().ChatID != 22 {
		t.Fatalf("forward selector = id:%#v value:%#v", model.listModals.host.Identity(), model.listModals.host.Value())
	}

	state.Chats = []domain.Chat{{ID: 22, Title: "B"}, {ID: 11, Title: "A"}}
	// Legacy SelectedChat=1 is stale after reorder; semantic destination 22 wins.
	model.engine = app.NewEngine(state)
	_ = model.syncListModalController()
	if model.listModals.host.Value().ChatID != 22 {
		t.Fatalf("forward reorder lost semantic ChatID: %#v", model.listModals.host.Value())
	}

	state.Focus = app.FocusConversation
	state.ForwardPicker = nil
	model.engine = app.NewEngine(state)
	_ = model.syncListModalController()
	if model.listModals.host.Identity() != (selectorIdentity{}) || model.listModals.host.Value() != (app.ActionReceived{}) || len(model.listModals.host.Options()) != 0 || model.listModals.host.focused {
		t.Fatalf("closed selector retained state: id:%#v value:%#v options:%#v focus:%t", model.listModals.host.Identity(), model.listModals.host.Value(), model.listModals.host.Options(), model.listModals.host.focused)
	}
}

func TestAppModelSyncsListModalControllerOnEveryPostEngineBatch(t *testing.T) {
	state := app.InitialState()
	state.Width, state.Height = 80, 24
	state.Focus = app.FocusReactionPicker
	state.ReactionPicker = &app.ReactionPicker{RequestID: 12, ChatID: 9, MessageID: 2, Selected: 1}
	model := newAppModelForTest(t, app.NewEngine(state), newBoundedAppRuntimeForModelTest(t))
	model, _ = updateAppModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	if model.listModals.host.Identity().Kind != selectorReaction || model.listModals.host.Value() != selectorOptionsFromRows(reactionRows(state.ReactionPicker))[1].Value {
		t.Fatalf("WindowSize did not synchronize selector: id=%#v value=%#v", model.listModals.host.Identity(), model.listModals.host.Value())
	}

	data, err := os.ReadFile("app_model.go")
	if err != nil {
		t.Fatal(err)
	}
	postSyncBatches := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "return m, tea.Batch") && strings.Contains(line, "m.syncPhotoPathInputHost()") {
			postSyncBatches++
			if !strings.Contains(line, "m.syncListModalController()") {
				t.Fatalf("post-engine frontend host batch omitted selector sync: %s", strings.TrimSpace(line))
			}
		}
	}
	if postSyncBatches != 18 {
		t.Fatalf("post-engine frontend host batches = %d, want frozen 18", postSyncBatches)
	}
}

// listModalDescriptorCase is one supported list modal surface: the view model
// that activates it and the exact descriptor the shared synchronization path
// must resolve from it.
type listModalDescriptorCase struct {
	name        string
	model       ui.ViewModel
	wantKind    selectorKind
	wantFocused bool
	wantIDs     []string
	wantLabels  []string
	wantValue   app.ActionReceived
}

func TestListModalControllerDescriptorCoversEverySupportedSurface(t *testing.T) {
	tests := []listModalDescriptorCase{
		{
			name: "chat actions",
			model: ui.ViewModel{
				Width: 100, Height: 24, Layout: ui.Layout{Mode: app.LayoutWide},
				Focus:      app.FocusChatActions,
				ActiveChat: domain.Chat{ID: 9, Title: "Team", Kind: domain.ChatSupergroup, IsMember: true},
				ChatActions: &app.ChatActionMenuState{
					RequestID: 4, ChatID: 9, Selected: 1, Working: true,
				},
			},
			wantKind:    selectorChatActions,
			wantFocused: true,
			wantLabels:  []string{"Open chat"},
			wantValue:   app.ActionReceived{Action: app.ViewChatInfo, ChatID: 9},
		},
		{
			name: "message actions",
			model: ui.ViewModel{
				Width: 100, Height: 24, Layout: ui.Layout{Mode: app.LayoutWide},
				Focus: app.FocusModal,
				MessageMenu: &app.MessageActionMenu{
					RequestID: 7, ChatID: 9, MessageID: 2, Loading: true,
					Capabilities: domain.MessageCapabilities{Reply: true, Copy: true},
					Selected:     1,
				},
			},
			wantKind:    selectorMessageActions,
			wantFocused: true,
			wantIDs:     []string{"action:reply", "action:copy"},
			wantValue:   app.ActionReceived{Action: app.CopyMessage, ChatID: 9, MessageID: 2},
		},
		{
			name: "reaction picker",
			model: ui.ViewModel{
				Width: 100, Height: 24, Layout: ui.Layout{Mode: app.LayoutWide},
				Focus:          app.FocusReactionPicker,
				ReactionPicker: &app.ReactionPicker{RequestID: 8, ChatID: 9, MessageID: 2, Selected: 2},
			},
			wantKind:    selectorReaction,
			wantFocused: true,
			wantIDs:     reactionOptionIDs(),
			wantValue:   app.ActionReceived{Action: app.Activate, ChatID: 9, MessageID: 2, Rune: rune(2 + 0x10000)},
		},
		{
			name: "forward picker",
			model: ui.ViewModel{
				Width: 100, Height: 24, Layout: ui.Layout{Mode: app.LayoutWide},
				Focus:         app.FocusForwardPicker,
				ForwardPicker: &app.ForwardPicker{RequestID: 9, SourceChatID: 9, SourceMessageID: 2, SelectedChat: 1},
				Chats: []ui.ChatRow{
					{Chat: domain.Chat{ID: 11, Title: "A"}},
					{Chat: domain.Chat{ID: 22, Title: "B"}},
				},
			},
			wantKind:    selectorForward,
			wantFocused: true,
			wantIDs:     []string{"forward:0:11", "forward:1:22"},
			wantValue:   app.ActionReceived{Action: app.Activate, ChatID: 22},
		},
		{
			name:        "message search results",
			model:       withSelected(searchViewModel(), 1),
			wantKind:    selectorMessageSearch,
			wantFocused: true,
			wantIDs:     []string{"search:30", "search:20"},
			wantValue:   app.ActionReceived{Action: app.SelectMessage, ChatID: 9, MessageID: 20},
		},
		{
			name:        "members list",
			model:       withSelected(membersViewModel(), 1),
			wantKind:    selectorMembers,
			wantFocused: true,
			wantIDs:     []string{"member:1", "member:2"},
			wantValue:   app.ActionReceived{Action: app.OpenMemberDetail, ChatID: 9, UserID: 2},
		},
		{
			name:        "members detail",
			model:       withSelected(membersDetailViewModel(), 0),
			wantKind:    selectorMembers,
			wantFocused: true,
			wantIDs: []string{"member:back", "member-action:copy-username", "member-action:add-contact",
				"member-action:remove-contact", "member-action:block", "member-action:unblock"},
			wantValue: app.ActionReceived{Action: app.CloseMemberDetail, ChatID: 9},
		},
		{
			name:        "pinned messages",
			model:       withSelected(pinnedViewModel(), 1),
			wantKind:    selectorPinnedMessages,
			wantFocused: true,
			wantIDs:     []string{"pinned:30", "pinned:20"},
			wantValue:   app.ActionReceived{Action: app.SelectMessage, ChatID: 9, MessageID: 20},
		},
		{
			name:        "topics",
			model:       topicsViewModel(),
			wantKind:    selectorTopics,
			wantFocused: true,
			wantIDs:     []string{"topic:all", "topic:1", "topic:2"},
			wantValue:   app.ActionReceived{Action: app.SelectTopic, ChatID: 9, TopicID: 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := activeListModalDescriptor(test.model, time.UTC)
			if !ok {
				t.Fatal("surface resolved to no descriptor")
			}
			if descriptor.identity.Kind != test.wantKind {
				t.Fatalf("kind = %v, want %v", descriptor.identity.Kind, test.wantKind)
			}
			if descriptor.focused != test.wantFocused {
				t.Fatalf("focused = %t, want %t", descriptor.focused, test.wantFocused)
			}
			// Options and the authoritative selection always come from the
			// descriptor's own displayed rows, in display order.
			if got, want := optionIDsOf(descriptor.options), optionIDsOf(selectorOptionsFromRows(descriptor.rows)); !reflect.DeepEqual(got, want) {
				t.Fatalf("options are not the derived row subset: %#v vs %#v", got, want)
			}
			if got, want := optionIDsOf(descriptor.options), test.wantIDs; test.wantIDs != nil && !reflect.DeepEqual(got, want) {
				t.Fatalf("option ids = %#v, want %#v", got, want)
			}
			for _, label := range test.wantLabels {
				if descriptor.options[0].Label != label {
					t.Fatalf("first option label = %q, want %q", descriptor.options[0].Label, label)
				}
			}
			if descriptor.authoritative != test.wantValue {
				t.Fatalf("authoritative = %#v, want %#v", descriptor.authoritative, test.wantValue)
			}
			controller := newListModalController()
			_ = controller.Sync(test.model, time.UTC)
			if controller.host.Identity() != descriptor.identity {
				t.Fatalf("host identity = %#v, want %#v", controller.host.Identity(), descriptor.identity)
			}
			if got, want := optionIDsOf(controller.host.Options()), optionIDsOf(descriptor.options); !reflect.DeepEqual(got, want) {
				t.Fatalf("host options = %#v, want %#v", got, want)
			}
			if controller.host.Value() != descriptor.authoritative {
				t.Fatalf("host value = %#v, want authoritative %#v", controller.host.Value(), descriptor.authoritative)
			}
			if !controller.host.focused {
				t.Fatal("host did not take the requested focus")
			}
		})
	}
}

// TestListModalControllerDescriptorRowsAreThePaintedRows locks the shared row
// source for the windowed surfaces: the descriptor sees exactly the rows the
// frame paints, so option count, geometry, and selection follow the visible
// window instead of the full reducer result list.
func TestListModalControllerDescriptorRowsAreThePaintedRows(t *testing.T) {
	tests := []struct {
		name   string
		model  ui.ViewModel
		rows   func(ui.ViewModel) []modalRowSpec
		remote string
		paint  func(ui.ViewModel, renderStyles) surfaceResult
	}{
		{
			name:   "members",
			model:  overflowMembersViewModel(),
			rows:   func(model ui.ViewModel) []modalRowSpec { return membersRows(model.Members) },
			remote: "Member 1",
			paint: func(model ui.ViewModel, styles renderStyles) surfaceResult {
				return buildMembersLayer(model, styles, "")
			},
		},
		{
			name:   "topics",
			model:  overflowTopicsViewModel(),
			rows:   func(model ui.ViewModel) []modalRowSpec { return topicsRows(model.Topics) },
			remote: "Topic 1",
			paint:  func(model ui.ViewModel, styles renderStyles) surfaceResult { return buildTopicsLayer(model, styles) },
		},
		{
			name:   "pinned messages",
			model:  overflowPinnedViewModel(),
			rows:   func(model ui.ViewModel) []modalRowSpec { return pinnedMessagesRows(model, time.UTC) },
			remote: "far off",
			paint: func(model ui.ViewModel, styles renderStyles) surfaceResult {
				return buildPinnedMessagesLayer(model, time.UTC, styles, "")
			},
		},
		{
			name:   "message search",
			model:  overflowSearchViewModel(),
			rows:   func(model ui.ViewModel) []modalRowSpec { return messageSearchRows(model, time.UTC) },
			remote: "far off",
			paint: func(model ui.ViewModel, styles renderStyles) surfaceResult {
				return buildMessageSearchLayer(model, time.UTC, styles, "", "")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := activeListModalDescriptor(test.model, time.UTC)
			if !ok {
				t.Fatal("overflow surface resolved to no descriptor")
			}
			capacity := max(1, test.model.Height-5)
			full := test.rows(test.model)
			if len(full) <= capacity {
				t.Fatalf("fixture must overflow: %d rows, capacity %d", len(full), capacity)
			}
			if len(descriptor.rows) != capacity {
				t.Fatalf("descriptor rows = %d, want the painted window %d", len(descriptor.rows), capacity)
			}
			painted := paintedListModalText(test.model, test.paint(test.model, newRenderStyles(false)))
			for _, row := range descriptor.rows {
				if !strings.Contains(painted, ansi.Strip(row.Label)) {
					t.Fatalf("descriptor row %q is not painted\n%s", row.Label, painted)
				}
			}
			if strings.Contains(painted, test.remote) {
				t.Fatalf("windowed descriptor painted a row outside the window:\n%s", painted)
			}
			// The selector geometry follows the same windowed row count, so the
			// host can never show more option rows than the frame paints.
			controller := newListModalController()
			_ = controller.Sync(test.model, time.UTC)
			if visible := len(selectorOptionsFromRows(descriptor.rows)); controller.host.height != visible {
				t.Fatalf("host height = %d, want the %d visible option rows", controller.host.height, visible)
			}
		})
	}
}

// TestListModalControllerChatSearchStaysManual pins the deliberate exclusion:
// chat search keeps its sectioned manual rows and never owns the shared
// selector, so the controller resets instead of compiling options.
func TestListModalControllerChatSearchStaysManual(t *testing.T) {
	model := chatSearchViewModel()
	if _, ok := activeListModalDescriptor(model, time.UTC); ok {
		t.Fatal("ChatSearch resolved a descriptor; it must stay manual")
	}
	if listModalSnapshotActive(app.State{ChatSearch: model.ChatSearch, Focus: app.FocusChatSearchResults}) {
		t.Fatal("ChatSearch must stay out of the list modal fast path")
	}
	controller := newListModalController()
	_ = controller.Sync(model, time.UTC)
	if controller.host.Identity() != (selectorIdentity{}) || len(controller.host.Options()) != 0 || controller.host.focused {
		t.Fatalf("ChatSearch left selector state behind: id:%#v options:%#v focus:%t",
			controller.host.Identity(), controller.host.Options(), controller.host.focused)
	}
	if rows := buildUnifiedChatSearchRows(model, time.UTC); len(selectorOptionsFromRows(rows)) == 0 {
		t.Fatal("chat search manual rows carry no actionable identity")
	}
}

// reactionOptionIDs is one stable id per app.ReactionPalette entry.
func reactionOptionIDs() []string {
	ids := make([]string, 0, len(app.ReactionPalette))
	for index := range app.ReactionPalette {
		ids = append(ids, "reaction:"+strconv.Itoa(index))
	}
	return ids
}

func optionIDsOf(options []selectorOption) []string {
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	return ids
}

func withSelected(model ui.ViewModel, selected int) ui.ViewModel {
	if model.Members != nil {
		model.Members.Selected = selected
	}
	if model.MessageSearch != nil {
		model.MessageSearch.Selected = selected
	}
	if model.PinnedMessages != nil {
		model.PinnedMessages.Selected = selected
	}
	if model.Topics != nil {
		model.Topics.Selected = selected
	}
	return model
}

func membersDetailViewModel() ui.ViewModel {
	model := membersViewModel()
	model.Members.Detail = &app.MemberDetail{UserID: 1, Name: "Ada", Username: "ada"}
	return model
}

func overflowRows(count int) []domain.Message {
	rows := make([]domain.Message, 0, count)
	for index := 1; index <= count; index++ {
		text := "hit " + strconv.Itoa(index)
		if index == 1 {
			text = "far off"
		}
		rows = append(rows, domain.Message{
			ID: domain.MessageID(index), ChatID: 9, Kind: domain.MessageText,
			Text: text, SenderName: "Ada", SentAt: time.Unix(1700000000, 0).UTC(),
		})
	}
	return rows
}

func overflowMembersViewModel() ui.ViewModel {
	model := membersViewModel()
	model.Height = 24
	members := make([]domain.ChatMember, 0, 40)
	for index := 1; index <= 40; index++ {
		members = append(members, domain.ChatMember{User: domain.User{ID: domain.UserID(index), Name: "Member " + strconv.Itoa(index)}})
	}
	model.Members.Results = members
	model.Members.Selected = 39
	return model
}

func overflowTopicsViewModel() ui.ViewModel {
	model := topicsViewModel()
	model.Height = 24
	topics := make([]domain.ForumTopic, 0, 40)
	for index := 1; index <= 40; index++ {
		topics = append(topics, domain.ForumTopic{ID: domain.TopicID(index), ChatID: 9, Name: "Topic " + strconv.Itoa(index)})
	}
	model.Topics.Results = topics
	model.Topics.Selected = 40
	return model
}

func overflowPinnedViewModel() ui.ViewModel {
	model := pinnedViewModel()
	model.Height = 24
	model.PinnedMessages.Results = overflowRows(40)
	model.PinnedMessages.Selected = 39
	return model
}

func overflowSearchViewModel() ui.ViewModel {
	model := searchViewModel()
	model.Height = 24
	model.MessageSearch.Results = overflowRows(40)
	model.MessageSearch.Selected = 39
	return model
}

// paintedListModalText composes one surface layer over the model bounds and
// returns the plain painted text, the same row list the descriptor claims.
func paintedListModalText(model ui.ViewModel, surface surfaceResult) string {
	_, canvas := listModalCanvas(image.Rect(0, 0, model.Width, model.Height), surface)
	return ansi.Strip(canvas.Render())
}
