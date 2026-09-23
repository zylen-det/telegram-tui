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
			opened, _ := updateState(state, ActionReceived{Action: OpenMessageActionMenu})
			menu := opened.MessageMenu
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
	opened, _ := updateState(referenceState(), ActionReceived{Action: OpenMessageActionMenu})
	// Enter activates the same action while Telegram is still resolving message properties.
	selectMessageMenuAction(opened.MessageMenu, GoToReferencedMessage)
	jumping, effects := updateState(opened, ActionReceived{Action: Activate})
	if len(effects) != 1 {
		t.Fatalf("effects = %#v", effects)
	}
	load, ok := effects[0].(LoadSearchMessageContext)
	if !ok || load.ChatID != 9 || load.MessageID != 20 || load.TopicID != 0 || load.RequestID == 0 || jumping.MessageMenu.JumpRequestID != load.RequestID {
		t.Fatalf("jump command/menu = %#v/%#v", effects[0], jumping.MessageMenu)
	}
	if ignored, effects := updateState(jumping, ActionReceived{Action: GoToReferencedMessage}); len(effects) != 0 || ignored.MessageMenu.JumpRequestID != load.RequestID {
		t.Fatalf("duplicate jump = %#v/%#v", ignored.MessageMenu, effects)
	}
	page := telegram.MessagePage{Messages: []domain.Message{
		{ID: 10, ChatID: 9, Kind: domain.MessageText},
		{ID: 20, ChatID: 9, Kind: domain.MessageText, Text: "old"},
		{ID: 30, ChatID: 9, Kind: domain.MessageText, HasReply: true, ReplyToMessageID: 20},
	}}
	landed, _ := updateState(jumping, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: page})
	if landed.MessageMenu != nil || landed.Focus != FocusConversation || landed.SelectedMessage != 20 || landed.SelectedMessageChat != 9 || !landed.History[9].FollowSelection || landed.History[9].ViewOffset != 1 || landed.Drafts[9] != "unsent" {
		t.Fatalf("jump result = menu:%#v focus:%v selection:%d history:%#v draft:%q", landed.MessageMenu, landed.Focus, landed.SelectedMessage, landed.History[9], landed.Drafts[9])
	}
}

func TestReferenceJumpFailureAndCancellation(t *testing.T) {
	opened, _ := updateState(referenceState(), ActionReceived{Action: OpenMessageActionMenu})
	jumping, effects := updateState(opened, ActionReceived{Action: GoToReferencedMessage})
	load := effects[0].(LoadSearchMessageContext)
	wrong, _ := updateState(jumping, SearchMessageContextLoaded{RequestID: load.RequestID + 1, ChatID: 9, MessageID: 20})
	if wrong.MessageMenu == nil || wrong.MessageMenu.JumpRequestID != load.RequestID || wrong.SelectedMessage != 30 {
		t.Fatal("stale result changed the selection")
	}
	failed, _ := updateState(jumping, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{}})
	if failed.SelectedMessage != 30 || failed.MessageMenu == nil || failed.MessageMenu.JumpRequestID != 0 || failed.Toast == nil || failed.Toast.Message != "Could not open referenced message" {
		t.Fatalf("missing target = menu:%#v selected:%d toast:%#v", failed.MessageMenu, failed.SelectedMessage, failed.Toast)
	}
	closed, _ := updateState(jumping, ActionReceived{Action: Close})
	late, _ := updateState(closed, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{Messages: []domain.Message{{ID: 20, ChatID: 9}}}})
	if late.MessageMenu != nil || late.SelectedMessage != 30 || late.Focus != FocusConversation {
		t.Fatalf("canceled jump applied: %#v", late)
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
			opened, _ := updateState(state, ActionReceived{Action: OpenMessageActionMenu})
			jumping, effects := updateState(opened, ActionReceived{Action: GoToReferencedMessage})
			load := effects[0].(LoadSearchMessageContext)
			target := state.Messages[9][0]
			landed, _ := updateState(jumping, SearchMessageContextLoaded{RequestID: load.RequestID, ChatID: 9, MessageID: 20, Page: telegram.MessagePage{Messages: []domain.Message{target}}})
			if landed.ShowAll[9] != tc.showAll || landed.SelectedMessage != 20 || landed.Focus != FocusConversation {
				t.Fatalf("topic jump = all:%t selected:%d focus:%v", landed.ShowAll[9], landed.SelectedMessage, landed.Focus)
			}
			if tc.showAll && !landed.History[9].FollowSelection || !tc.showAll && !landed.TopicHistory[topicKey{ChatID: 9, TopicID: 7}].FollowSelection {
				t.Fatal("wrong history family follows the reference")
			}
		})
	}
}
