package frontend

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestListModalRowsMessageActionsCanonicalSemanticPayloads(t *testing.T) {
	menu := &MessageActionMenu{
		ChatID:    9,
		MessageID: 22,
		Capabilities: domain.MessageCapabilities{
			Reply: true, Forward: true, Edit: true, Copy: true,
			Pin: true, DeleteForSelf: true, DeleteForAll: true,
		},
		Pinned:    true,
		CanReact:  true,
		MediaFile: domain.MediaFileRef{ID: 4, CanDownload: true},
	}
	got := selectorOptionsFromRows(messageActionRows(menu))
	want := []selectorOption{
		{ID: "action:view-image", Label: "View image", Value: ActionReceived{Action: ViewMessageMedia, ChatID: 9, MessageID: 22}},
		{ID: "action:reply", Label: "Reply", Value: ActionReceived{Action: ReplyMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:forward", Label: "Forward", Value: ActionReceived{Action: ForwardMessageSource, ChatID: 9, MessageID: 22}},
		{ID: "action:edit", Label: "Edit", Value: ActionReceived{Action: EditMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:copy", Label: "Copy", Value: ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:react", Label: "React", Value: ActionReceived{Action: ReactMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:pin", Label: "Unpin", Value: ActionReceived{Action: PinMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:delete", Label: "Delete", Value: ActionReceived{Action: DeleteMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:delete-all", Label: "Delete for everyone", Value: ActionReceived{Action: DeleteForEveryone, ChatID: 9, MessageID: 22}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled Message actions = %#v, want %#v", got, want)
	}

	huhOptions := selectorHuhOptions(got)
	if len(huhOptions) != len(want) {
		t.Fatalf("Huh options = %d, want %d", len(huhOptions), len(want))
	}
	for index, option := range huhOptions {
		if option.Key != want[index].Label || option.Value != want[index].Value {
			t.Errorf("Huh option %d = {%q %#v}, want {%q %#v}", index, option.Key, option.Value, want[index].Label, want[index].Value)
		}
	}
}

func TestListModalRowsMessageActionLoadingErrorGating(t *testing.T) {
	base := MessageActionMenu{
		ChatID: 9, MessageID: 22, CanReact: true,
		Capabilities: domain.MessageCapabilities{
			Reply: true, Forward: true, Edit: true, Copy: true,
			Pin: true, DeleteForSelf: true, DeleteForAll: true,
		},
	}
	for _, tc := range []struct {
		name string
		menu MessageActionMenu
	}{
		{name: "loading", menu: func() MessageActionMenu { m := base; m.Loading = true; return m }()},
		{name: "error", menu: func() MessageActionMenu { m := base; m.Error = &domain.AppError{Message: "unavailable"}; return m }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := selectorOptionsFromRows(messageActionRows(&tc.menu))
			want := []selectorOption{
				{ID: "action:reply", Label: "Reply", Value: ActionReceived{Action: ReplyMessage, ChatID: 9, MessageID: 22}},
				{ID: "action:copy", Label: "Copy", Value: ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("compiled gated options = %#v, want %#v", got, want)
			}
		})
	}
	if got := selectorOptionsFromRows(messageActionRows(nil)); got != nil {
		t.Fatalf("nil Message menu options = %#v, want nil", got)
	}
}

func TestListModalRowsReactionPaletteExactSemanticPayloads(t *testing.T) {
	picker := &ReactionPicker{ChatID: 9, MessageID: 22}
	got := selectorOptionsFromRows(reactionRows(picker))
	if len(got) != len(ReactionPalette) {
		t.Fatalf("reaction options = %d, want %d", len(got), len(ReactionPalette))
	}
	for index, emoji := range ReactionPalette {
		want := selectorOption{
			ID:    "reaction:" + strconv.Itoa(index),
			Label: emoji,
			Value: ActionReceived{Action: Activate, ChatID: 9, MessageID: 22, Rune: rune(index + 0x10000)},
		}
		if got[index] != want {
			t.Errorf("reaction option %d = %#v, want %#v", index, got[index], want)
		}
	}
	if got := selectorOptionsFromRows(reactionRows(nil)); got != nil {
		t.Fatalf("nil Reaction picker options = %#v, want nil", got)
	}
}

func TestListModalRowsForwardUsesChatIdentityAcrossReorder(t *testing.T) {
	picker := &ForwardPicker{SourceChatID: 9, SourceMessageID: 22}
	first := []ChatRow{
		{Chat: domain.Chat{ID: 101, Title: "Alpha"}},
		{Chat: domain.Chat{ID: 202, Title: "Bravo"}},
	}
	got := selectorOptionsFromRows(forwardRows(picker, first))
	selected := got[1].Value
	if selected != (ActionReceived{Action: Activate, ChatID: 202}) {
		t.Fatalf("forward selected identity = %#v, want ChatID 202", selected)
	}

	reordered := []ChatRow{first[1], first[0]}
	next := selectorOptionsFromRows(forwardRows(picker, reordered))
	index := selectorOptionIndex(next, selected)
	if index != 0 || next[index].ID != "forward:0:202" || next[index].Label != "Bravo" {
		t.Fatalf("reordered stable identity index=%d options=%#v", index, next)
	}
	if got := selectorOptionsFromRows(forwardRows(nil, first)); got != nil {
		t.Fatalf("nil Forward picker options = %#v, want nil", got)
	}
}

func TestSelectorOptionIndexRejectsMissingIdentity(t *testing.T) {
	options := []selectorOption{{Value: ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}}}
	if got := selectorOptionIndex(options, ActionReceived{Action: ReplyMessage, ChatID: 9, MessageID: 22}); got != -1 {
		t.Fatalf("missing semantic identity index = %d, want -1", got)
	}
}

// TestSelectorOptionsFromRowsKeepsOnlyActionableDisplayedRows documents the one
// generic derivation shared by every list modal: a row joins the selector only
// when it carries both an ID and a real action, order and exact semantic
// payload are preserved, and nothing else is dropped or reordered.
func TestSelectorOptionsFromRowsKeepsOnlyActionableDisplayedRows(t *testing.T) {
	first := ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}
	second := ActionReceived{Action: EditMessage, ChatID: 9, MessageID: 22}
	rows := []modalRowSpec{
		{ID: "section", Label: "Chats", Action: first, Header: true},
		{ID: "", Label: "Loading...", Action: first},
		{ID: "no-action", Label: "Informational"},
		{ID: "one", Label: "Copy", Action: first},
		{ID: "two", Label: "Edit", Action: second, Selected: true},
	}
	want := []selectorOption{
		{ID: "one", Label: "Copy", Value: first},
		{ID: "two", Label: "Edit", Value: second},
	}
	if got := selectorOptionsFromRows(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("derived options = %#v, want %#v", got, want)
	}
	// Informational, header, and empty rows never become selectable, so a list
	// without a single actionable row derives no options at all.
	if got := selectorOptionsFromRows(rows[:3]); got != nil {
		t.Fatalf("non-actionable rows derived options: %#v", got)
	}
	if got := selectorOptionsFromRows(nil); got != nil {
		t.Fatalf("nil rows derived options: %#v", got)
	}
	// The authoritative selection is the selected actionable row; header and
	// informational rows stay out of it.
	if got := selectorRowsAuthoritative(rows); got != second {
		t.Fatalf("authoritative = %#v, want %#v", got, second)
	}
	unselected := append([]modalRowSpec(nil), rows...)
	unselected[4].Selected = false
	if got := selectorRowsAuthoritative(unselected); got != (ActionReceived{}) {
		t.Fatalf("unselected rows authoritative = %#v, want zero payload", got)
	}
	headerOnly := append([]modalRowSpec(nil), rows...)
	headerOnly[0].Selected = true
	headerOnly[4].Selected = false
	if got := selectorRowsAuthoritative(headerOnly); got != (ActionReceived{}) {
		t.Fatalf("header selection leaked into authoritative: %#v", got)
	}
	if !rowSelectable(rows[3]) || rowSelectable(rows[0]) || rowSelectable(rows[1]) || rowSelectable(rows[2]) {
		t.Fatal("row selection predicate misclassified rows")
	}
}
