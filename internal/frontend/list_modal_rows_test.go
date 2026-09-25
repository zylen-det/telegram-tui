package frontend

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

// actionableRow is the test-side summary of one displayed list row: exactly
// the modalRowSpec identity fields the manual paint and hit map consume.
type actionableRow struct {
	ID     string
	Label  string
	Action ActionReceived
}

// actionableRows is the test-only mirror of the production rowSelectable
// predicate: a row joins the list only when it carries an ID and a real
// action and is not a structural header. Order and exact semantic payloads
// are preserved.
func actionableRows(rows []modalRowSpec) []actionableRow {
	var out []actionableRow
	for _, row := range rows {
		if !rowSelectable(row) {
			continue
		}
		out = append(out, actionableRow{ID: row.ID, Label: row.Label, Action: row.Action})
	}
	return out
}

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
	got := actionableRows(messageActionRows(menu))
	want := []actionableRow{
		{ID: "action:view-image", Label: "View image", Action: ActionReceived{Action: ViewMessageMedia, ChatID: 9, MessageID: 22}},
		{ID: "action:reply", Label: "Reply", Action: ActionReceived{Action: ReplyMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:forward", Label: "Forward", Action: ActionReceived{Action: ForwardMessageSource, ChatID: 9, MessageID: 22}},
		{ID: "action:edit", Label: "Edit", Action: ActionReceived{Action: EditMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:copy", Label: "Copy", Action: ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:react", Label: "React", Action: ActionReceived{Action: ReactMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:pin", Label: "Unpin", Action: ActionReceived{Action: PinMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:delete", Label: "Delete", Action: ActionReceived{Action: DeleteMessage, ChatID: 9, MessageID: 22}},
		{ID: "action:delete-all", Label: "Delete for everyone", Action: ActionReceived{Action: DeleteForEveryone, ChatID: 9, MessageID: 22}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled Message actions = %#v, want %#v", got, want)
	}

	// The selected flag tracks the authoritative Selected index only.
	rows := messageActionRows(menu)
	for index, row := range rows {
		if row.Selected != (index == menu.Selected) {
			t.Errorf("row %d (%s) selected = %t, want %t", index, row.ID, row.Selected, index == menu.Selected)
		}
	}
	menu.Selected = 3
	rows = messageActionRows(menu)
	for index, row := range rows {
		if row.Selected != (index == 3) {
			t.Errorf("settled row %d (%s) selected = %t, want %t", index, row.ID, row.Selected, index == 3)
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
			got := actionableRows(messageActionRows(&tc.menu))
			want := []actionableRow{
				{ID: "action:reply", Label: "Reply", Action: ActionReceived{Action: ReplyMessage, ChatID: 9, MessageID: 22}},
				{ID: "action:copy", Label: "Copy", Action: ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("compiled gated rows = %#v, want %#v", got, want)
			}
		})
	}
	if got := actionableRows(messageActionRows(nil)); got != nil {
		t.Fatalf("nil Message menu rows = %#v, want nil", got)
	}
}

func TestListModalRowsReactionPaletteExactSemanticPayloads(t *testing.T) {
	picker := &ReactionPicker{ChatID: 9, MessageID: 22}
	got := actionableRows(reactionRows(picker))
	if len(got) != len(ReactionPalette) {
		t.Fatalf("reaction rows = %d, want %d", len(got), len(ReactionPalette))
	}
	for index, emoji := range ReactionPalette {
		want := actionableRow{
			ID:     "reaction:" + strconv.Itoa(index),
			Label:  emoji,
			Action: ActionReceived{Action: Activate, ChatID: 9, MessageID: 22, Rune: rune(index + 0x10000)},
		}
		if got[index] != want {
			t.Errorf("reaction row %d = %#v, want %#v", index, got[index], want)
		}
	}
	if got := actionableRows(reactionRows(nil)); got != nil {
		t.Fatalf("nil Reaction picker rows = %#v, want nil", got)
	}
}

func TestListModalRowsForwardUsesChatIdentityAcrossReorder(t *testing.T) {
	picker := &ForwardPicker{SourceChatID: 9, SourceMessageID: 22}
	first := []ChatRow{
		{Chat: domain.Chat{ID: 101, Title: "Alpha"}},
		{Chat: domain.Chat{ID: 202, Title: "Bravo"}},
	}
	got := actionableRows(forwardRows(picker, first))
	selected := got[1].Action
	if selected != (ActionReceived{Action: Activate, ChatID: 202}) {
		t.Fatalf("forward selected identity = %#v, want ChatID 202", selected)
	}

	reorderedRows := forwardRows(picker, []ChatRow{first[1], first[0]})
	index := -1
	for rowIndex, row := range reorderedRows {
		if row.Action == selected {
			index = rowIndex
		}
	}
	if index != 0 || reorderedRows[index].ID != "forward:0:202" || reorderedRows[index].Label != "Bravo" {
		t.Fatalf("reordered stable identity index=%d rows=%#v", index, reorderedRows)
	}
	if got := actionableRows(forwardRows(nil, first)); got != nil {
		t.Fatalf("nil Forward picker rows = %#v, want nil", got)
	}
}

// TestActionableRowsKeepsOnlyActionableDisplayedRows documents the one
// predicate shared by every list modal: a row joins the interactive hit list
// (and keyboard selection) only when it carries an ID and a real action and
// is not a header, order and exact semantic payload are preserved, and
// nothing else is dropped or reordered.
func TestActionableRowsKeepsOnlyActionableDisplayedRows(t *testing.T) {
	first := ActionReceived{Action: CopyMessage, ChatID: 9, MessageID: 22}
	second := ActionReceived{Action: EditMessage, ChatID: 9, MessageID: 22}
	rows := []modalRowSpec{
		{ID: "section", Label: "Chats", Action: first, Header: true},
		{ID: "", Label: "Loading...", Action: first},
		{ID: "no-action", Label: "Informational"},
		{ID: "one", Label: "Copy", Action: first},
		{ID: "two", Label: "Edit", Action: second, Selected: true},
	}
	want := []actionableRow{
		{ID: "one", Label: "Copy", Action: first},
		{ID: "two", Label: "Edit", Action: second},
	}
	if got := actionableRows(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("derived rows = %#v, want %#v", got, want)
	}
	// Informational, header, and empty rows never become selectable, so a
	// list without a single actionable row derives no interactive rows.
	if got := actionableRows(rows[:3]); got != nil {
		t.Fatalf("non-actionable rows derived interactive rows: %#v", got)
	}
	if got := actionableRows(nil); got != nil {
		t.Fatalf("nil rows derived interactive rows: %#v", got)
	}
	// The paint-side selection is the selected actionable row; header and
	// informational rows stay out of it.
	selectedActionable := func(specs []modalRowSpec) ActionReceived {
		for _, row := range specs {
			if row.Selected && rowSelectable(row) {
				return row.Action
			}
		}
		return ActionReceived{}
	}
	if got := selectedActionable(rows); got != second {
		t.Fatalf("selected actionable = %#v, want %#v", got, second)
	}
	unselected := append([]modalRowSpec(nil), rows...)
	unselected[4].Selected = false
	if got := selectedActionable(unselected); got != (ActionReceived{}) {
		t.Fatalf("unselected rows authoritative = %#v, want zero payload", got)
	}
	headerOnly := append([]modalRowSpec(nil), rows...)
	headerOnly[0].Selected = true
	headerOnly[4].Selected = false
	if got := selectedActionable(headerOnly); got != (ActionReceived{}) {
		t.Fatalf("header selection leaked into paint: %#v", got)
	}
}
