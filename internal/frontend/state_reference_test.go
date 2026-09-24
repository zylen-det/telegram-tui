package frontend

import (
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func referenceState() State {
	state := InitialState()
	state.Focus = FocusConversation
	state.Chats = []domain.Chat{{ID: 9}}
	state.Messages[9] = []domain.Message{
		{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old"},
		{ID: 30, ChatID: 9, Kind: domain.MessageText, Text: "reply", HasReply: true, ReplyToMessageID: 20},
	}
	state.SelectedMessageChat, state.SelectedMessage = 9, 30
	state.Drafts[9] = "unsent"
	return state
}

func TestReferenceActionVisibleOnlyForResolvableReply(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hasReply bool
		id       domain.MessageID
		want     bool
	}{
		{"reference", true, 20, true},
		{"missing id", true, 0, false},
		{"no reply", false, 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := referenceState()
			state.Messages[9][1].HasReply = tc.hasReply
			state.Messages[9][1].ReplyToMessageID = tc.id
			updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
			menu := state.MessageMenu
			found := false
			for index, row := range messageActionRows(menu) {
				if row.Action.Action != GoToReferencedMessage {
					continue
				}
				found = true
				if row.Label != "Go to referenced message" || row.Action.ChatID != 9 || row.Action.MessageID != 30 || menu.ReferencedMessageID != 20 {
					t.Fatalf("reference row = %#v, menu = %#v", row, menu)
				}
				menu.Selected = index
				if selectedMenuAction(menu) != GoToReferencedMessage {
					t.Fatalf("selection at %d does not activate reference", index)
				}
				selectMessageMenuAction(menu, GoToReferencedMessage)
				if menu.Selected != index || actionMenuItemCount(menu) != 2 {
					t.Fatalf("selection/count = %d/%d", menu.Selected, actionMenuItemCount(menu))
				}
			}
			if found != tc.want {
				t.Fatalf("row visible = %t, want %t", found, tc.want)
			}
		})
	}
}

func TestReferenceActionLoadsContextAndLandsOnOriginal(t *testing.T) {
	state := referenceState()
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	// Enter activates the same action while Telegram is still resolving message properties.
	selectMessageMenuAction(state.MessageMenu, GoToReferencedMessage)
	effects := updateState(&state, ActionReceived{Action: Activate})
	if len(effects) != 1 {
		t.Fatalf("effects = %#v", effects)
	}
	load, ok := effects[0].(LoadSearchMessageContext)
	if !ok || load.ChatID != 9 || load.MessageID != 20 || load.TopicID != 0 || load.RequestID == 0 || state.MessageMenu.JumpRequestID != load.RequestID {
		t.Fatalf("jump command/menu = %#v/%#v", effects[0], state.MessageMenu)
	}
	// Re-entering the jump while one is in flight stays a no-op and keeps the
	// original pending request id.
	if effects := updateState(&state, ActionReceived{Action: GoToReferencedMessage}); len(effects) != 0 || state.MessageMenu.JumpRequestID != load.RequestID {
		t.Fatalf("duplicate jump = %#v/%#v", state.MessageMenu, effects)
	}
	page := telegram.MessagePage{Messages: []domain.Message{
		{ID: 10, ChatID: 9, Kind: domain.MessageText},
		{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old"},
		{ID: 30, ChatID: 9, Kind: domain.MessageText, HasReply: true, ReplyToMessageID: 20},
	}}
	updateState(&state, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: page})
	if state.MessageMenu != nil || state.Focus != FocusConversation || state.SelectedMessage != 20 || state.SelectedMessageChat != 9 || !state.History[9].FollowSelection || state.History[9].ViewOffset != 1 || state.Drafts[9] != "unsent" {
		t.Fatalf("jump result = menu:%#v focus:%v selection:%d history:%#v draft:%q", state.MessageMenu, state.Focus, state.SelectedMessage, state.History[9], state.Drafts[9])
	}
}

func TestReferenceJumpFailureAndCancellation(t *testing.T) {
	state := referenceState()
	updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
	effects := updateState(&state, ActionReceived{Action: GoToReferencedMessage})
	load := effects[0].(LoadSearchMessageContext)
	updateState(&state, SearchMessageContextLoaded{RequestID: load.RequestID + 1, ChatID: 9, MessageID: 20})
	if state.MessageMenu == nil || state.MessageMenu.JumpRequestID != load.RequestID || state.SelectedMessage != 30 {
		t.Fatal("stale result changed the selection")
	}
	updateState(&state, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{}})
	if state.SelectedMessage != 30 || state.MessageMenu == nil || state.MessageMenu.JumpRequestID != 0 || state.Toast == nil || state.Toast.Message != "Could not open referenced message" {
		t.Fatalf("missing target = menu:%#v selected:%d toast:%#v", state.MessageMenu, state.SelectedMessage, state.Toast)
	}
	updateState(&state, ActionReceived{Action: Close})
	updateState(&state, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 20, ChatID: 9}}}})
	if state.MessageMenu != nil || state.SelectedMessage != 30 || state.Focus != FocusConversation {
		t.Fatalf("canceled jump applied: %#v", state)
	}
}

func TestReferenceJumpCentersWithinTopicOrShowsCrossTopic(t *testing.T) {
	for _, tc := range []struct {
		name        string
		targetTopic domain.TopicID
		showAll     bool
	}{
		{"same topic", 7, false},
		{"cross topic", 8, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := referenceState()
			state.Chats[0].IsForum = true
			state.SelectedTopics[9] = 7
			state.ForumTopics[9] = map[domain.TopicID]domain.ForumTopic{7: {ID: 7, ChatID: 9}}
			state.Messages[9][0].TopicID = tc.targetTopic
			state.Messages[9][1].TopicID = 7
			updateState(&state, ActionReceived{Action: OpenMessageActionMenu})
			effects := updateState(&state, ActionReceived{Action: GoToReferencedMessage})
			load := effects[0].(LoadSearchMessageContext)
			target := state.Messages[9][0]
			updateState(&state, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{Messages: []domain.Message{target}}})
			if state.ShowAll[9] != tc.showAll || state.SelectedMessage != 20 || state.Focus != FocusConversation {
				t.Fatalf("topic jump = all:%t selected:%d focus:%v", state.ShowAll[9], state.SelectedMessage, state.Focus)
			}
			if tc.showAll && !state.History[9].FollowSelection || !tc.showAll && !state.TopicHistory[topicKey{ChatID: 9, TopicID: 7}].FollowSelection {
				t.Fatal("wrong history family follows the reference")
			}
		})
	}
}
