package frontend

import (
	"reflect"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestGroupMessagesJoinRules(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+08", 8*60*60)
	start := time.Date(2026, time.January, 2, 9, 0, 0, 0, time.UTC)
	baseLeft := groupTestMessage(1, start)
	baseRight := groupTestMessage(2, start.Add(5*time.Minute))

	tests := []struct {
		name   string
		change func(*domain.Message, *domain.Message)
		joined bool
	}{
		{name: "same timestamp", change: func(left *domain.Message, right *domain.Message) {
			right.SentAt = left.SentAt
		}, joined: true},
		{name: "same sender at five minutes", joined: true},
		{name: "five minutes and one second", change: func(_ *domain.Message, right *domain.Message) {
			right.SentAt = start.Add(5*time.Minute + time.Second)
		}},
		{name: "different sender ID", change: func(_ *domain.Message, right *domain.Message) {
			right.Sender.ID++
		}},
		{name: "different sender kind", change: func(_ *domain.Message, right *domain.Message) {
			right.Sender.Kind = domain.SenderChat
		}},
		{name: "different local day", change: func(left *domain.Message, right *domain.Message) {
			left.SentAt = time.Date(2026, time.January, 2, 15, 59, 0, 0, time.UTC)
			right.SentAt = left.SentAt.Add(2 * time.Minute)
		}},
		{name: "left service", change: func(left *domain.Message, _ *domain.Message) {
			left.Service = true
		}},
		{name: "right service", change: func(_ *domain.Message, right *domain.Message) {
			right.Service = true
		}},
		{name: "left reply", change: func(left *domain.Message, _ *domain.Message) {
			left.HasReply = true
		}},
		{name: "right reply", change: func(_ *domain.Message, right *domain.Message) {
			right.HasReply = true
		}},
		{name: "left forward", change: func(left *domain.Message, _ *domain.Message) {
			left.HasForward = true
		}},
		{name: "right forward", change: func(_ *domain.Message, right *domain.Message) {
			right.HasForward = true
		}},
		{name: "left outgoing", change: func(left *domain.Message, _ *domain.Message) {
			left.Outgoing = true
		}},
		{name: "right outgoing", change: func(_ *domain.Message, right *domain.Message) {
			right.Outgoing = true
		}},
		{name: "negative gap", change: func(_ *domain.Message, right *domain.Message) {
			right.SentAt = start.Add(-time.Second)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			left, right := baseLeft, baseRight
			if test.change != nil {
				test.change(&left, &right)
			}

			got := GroupMessages(domain.ChatBasicGroup, []domain.Message{left, right}, location)
			wantGroups := 2
			if test.joined {
				wantGroups = 1
			}
			if len(got) != wantGroups {
				t.Fatalf("GroupMessages() returned %d groups, want %d", len(got), wantGroups)
			}
		})
	}
}

func TestGroupMessagesUsesProvidedLocation(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+08", 8*60*60)
	tests := []struct {
		name   string
		left   time.Time
		right  time.Time
		joined bool
	}{
		{
			name:   "different UTC dates but same local day join",
			left:   time.Date(2026, time.January, 1, 23, 58, 0, 0, time.UTC),
			right:  time.Date(2026, time.January, 2, 0, 2, 0, 0, time.UTC),
			joined: true,
		},
		{
			name:  "same UTC date but different local days split",
			left:  time.Date(2026, time.January, 1, 15, 58, 0, 0, time.UTC),
			right: time.Date(2026, time.January, 1, 16, 2, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			messages := []domain.Message{
				groupTestMessage(1, test.left),
				groupTestMessage(2, test.right),
			}
			got := GroupMessages(domain.ChatSupergroup, messages, location)
			wantGroups := 2
			if test.joined {
				wantGroups = 1
			}
			if len(got) != wantGroups {
				t.Fatalf("GroupMessages() returned %d groups, want %d", len(got), wantGroups)
			}
		})
	}
}

func TestGroupMessagesUsesInstantGapAcrossDSTTransitions(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation(America/New_York): %v", err)
	}
	tests := []struct {
		name           string
		left           time.Time
		right          time.Time
		wantLeftClock  string
		wantRightClock string
	}{
		{
			name:           "spring forward",
			left:           time.Date(2026, time.March, 8, 6, 59, 0, 0, time.UTC),
			right:          time.Date(2026, time.March, 8, 7, 1, 0, 0, time.UTC),
			wantLeftClock:  "2026-03-08 01:59 EST",
			wantRightClock: "2026-03-08 03:01 EDT",
		},
		{
			name:           "fall back",
			left:           time.Date(2026, time.November, 1, 5, 58, 0, 0, time.UTC),
			right:          time.Date(2026, time.November, 1, 6, 2, 0, 0, time.UTC),
			wantLeftClock:  "2026-11-01 01:58 EDT",
			wantRightClock: "2026-11-01 01:02 EST",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			const clockFormat = "2006-01-02 15:04 MST"
			if got := test.left.In(location).Format(clockFormat); got != test.wantLeftClock {
				t.Fatalf("left local clock = %q, want %q", got, test.wantLeftClock)
			}
			if got := test.right.In(location).Format(clockFormat); got != test.wantRightClock {
				t.Fatalf("right local clock = %q, want %q", got, test.wantRightClock)
			}

			messages := []domain.Message{
				groupTestMessage(1, test.left),
				groupTestMessage(2, test.right),
			}
			got := GroupMessages(domain.ChatBasicGroup, messages, location)
			if len(got) != 1 || !reflect.DeepEqual(got[0].Messages, messages) {
				t.Fatalf("GroupMessages() = %#v, want one group containing both messages", got)
			}
		})
	}
}

func TestGroupMessagesChatKinds(t *testing.T) {
	t.Parallel()

	location := time.UTC
	start := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	messages := []domain.Message{
		groupTestMessage(1, start),
		groupTestMessage(2, start.Add(time.Minute)),
	}
	tests := []struct {
		name       string
		kind       domain.ChatKind
		wantGroups int
	}{
		{name: "private never joins", kind: domain.ChatPrivate, wantGroups: 2},
		{name: "channel never joins", kind: domain.ChatChannel, wantGroups: 2},
		{name: "basic group joins", kind: domain.ChatBasicGroup, wantGroups: 1},
		{name: "supergroup joins", kind: domain.ChatSupergroup, wantGroups: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := GroupMessages(test.kind, messages, location)
			if len(got) != test.wantGroups {
				t.Fatalf("GroupMessages(%v) returned %d groups, want %d", test.kind, len(got), test.wantGroups)
			}
		})
	}
}

func TestGroupMessagesIncomingRunUsesFirstMessageMetadataAndOneAvatar(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 3, 11, 0, 0, 0, time.UTC)
	first := groupTestMessage(1, start)
	first.SenderName = "First name"
	first.SenderAvatar = domain.AvatarRef{FileID: 10, UniqueID: "first"}
	second := groupTestMessage(2, start.Add(time.Minute))
	second.SenderName = "Later name"
	second.SenderAvatar = domain.AvatarRef{FileID: 20, UniqueID: "later"}

	got := GroupMessages(domain.ChatBasicGroup, []domain.Message{first, second}, time.UTC)
	if len(got) != 1 {
		t.Fatalf("GroupMessages() returned %d groups, want 1", len(got))
	}
	want := MessageGroup{
		Sender:       first.Sender,
		SenderName:   first.SenderName,
		SenderAvatar: first.SenderAvatar,
		ShowAvatar:   true,
		Messages:     []domain.Message{first, second},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("GroupMessages()[0] = %#v, want %#v", got[0], want)
	}
}

func TestGroupMessagesJoinsChainUsingPreviousMessage(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.April, 4, 12, 0, 0, 0, time.UTC)
	messages := []domain.Message{
		groupTestMessage(1, start),
		groupTestMessage(2, start.Add(5*time.Minute)),
		groupTestMessage(3, start.Add(10*time.Minute)),
	}

	got := GroupMessages(domain.ChatSupergroup, messages, time.UTC)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Messages, messages) {
		t.Fatalf("GroupMessages() = %#v, want one group containing all three messages", got)
	}
}

func TestGroupMessagesShowAvatarRules(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.May, 8, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		kind    domain.ChatKind
		message domain.Message
		want    bool
	}{
		{name: "incoming basic group", kind: domain.ChatBasicGroup, message: groupTestMessage(1, start), want: true},
		{name: "incoming supergroup", kind: domain.ChatSupergroup, message: groupTestMessage(1, start), want: true},
		{name: "private", kind: domain.ChatPrivate, message: groupTestMessage(1, start), want: true},
		{name: "channel", kind: domain.ChatChannel, message: groupTestMessage(1, start)},
		{name: "outgoing", kind: domain.ChatBasicGroup, want: true, message: func() domain.Message {
			message := groupTestMessage(1, start)
			message.Outgoing = true
			return message
		}()},
		{name: "service", kind: domain.ChatSupergroup, message: func() domain.Message {
			message := groupTestMessage(1, start)
			message.Service = true
			return message
		}()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := GroupMessages(test.kind, []domain.Message{test.message}, time.UTC)
			if len(got) != 1 {
				t.Fatalf("GroupMessages() returned %d groups, want 1", len(got))
			}
			if got[0].ShowAvatar != test.want {
				t.Fatalf("GroupMessages()[0].ShowAvatar = %t, want %t", got[0].ShowAvatar, test.want)
			}
		})
	}
}

func TestGroupMessagesPreservesInputOrderAndOnlyJoinsAdjacent(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.June, 7, 15, 0, 0, 0, time.UTC)
	first := groupTestMessage(1, start)
	middle := groupTestMessage(2, start.Add(time.Minute))
	middle.Sender.ID = 2
	last := groupTestMessage(3, start.Add(2*time.Minute))

	got := GroupMessages(domain.ChatBasicGroup, []domain.Message{first, middle, last}, time.UTC)
	if len(got) != 3 {
		t.Fatalf("GroupMessages() returned %d groups, want 3", len(got))
	}
	for index, want := range []domain.Message{first, middle, last} {
		if len(got[index].Messages) != 1 || !reflect.DeepEqual(got[index].Messages[0], want) {
			t.Errorf("group %d messages = %#v, want [%#v]", index, got[index].Messages, want)
		}
	}
}

func TestGroupMessagesDoesNotReuseOrModifyInput(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.July, 9, 10, 0, 0, 0, time.UTC)
	messages := make([]domain.Message, 2, 4)
	messages[0] = groupTestMessage(1, start)
	messages[1] = groupTestMessage(2, start.Add(time.Minute))
	before := append([]domain.Message(nil), messages...)

	got := GroupMessages(domain.ChatBasicGroup, messages, time.UTC)
	if !reflect.DeepEqual(messages, before) {
		t.Fatalf("GroupMessages() modified input: got %#v, want %#v", messages, before)
	}
	if len(got) != 1 || len(got[0].Messages) != 2 {
		t.Fatalf("GroupMessages() returned %#v, want one two-message group", got)
	}

	got[0].Messages[0].Text = "changed output"
	got[0].Messages = append(got[0].Messages, groupTestMessage(3, start.Add(2*time.Minute)))
	if !reflect.DeepEqual(messages, before) {
		t.Fatalf("mutating output changed input: got %#v, want %#v", messages, before)
	}
}

func TestGroupMessagesEmptyInput(t *testing.T) {
	t.Parallel()

	if got := GroupMessages(domain.ChatBasicGroup, nil, time.UTC); len(got) != 0 {
		t.Fatalf("GroupMessages(nil) returned %#v, want no groups", got)
	}
}

func TestReplyContextProjectsCachedSameChatTarget(t *testing.T) {
	target := groupTestMessage(41, time.Unix(1, 0))
	target.SenderName = "opaque-sender"
	target.Text = "opaque-preview"
	reply := groupTestMessage(42, time.Unix(2, 0))
	reply.HasReply, reply.ReplyToMessageID = true, target.ID

	groups := GroupMessages(domain.ChatBasicGroup, []domain.Message{target, reply}, time.UTC)
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2", len(groups))
	}
	context, ok := groups[1].ReplyContexts[reply.ID]
	if !ok || !context.Available || context.Sender != "opaque-sender" || context.Preview != "opaque-preview" {
		t.Fatal("cached reply context projection mismatch")
	}
}

func TestReplyContextUsesUnavailableAndOutgoingSenderFallbacks(t *testing.T) {
	target := groupTestMessage(51, time.Unix(1, 0))
	target.SenderName, target.Outgoing = "", true
	reply := groupTestMessage(52, time.Unix(2, 0))
	reply.HasReply, reply.ReplyToMessageID = true, target.ID
	missing := groupTestMessage(53, time.Unix(3, 0))
	missing.HasReply, missing.ReplyToMessageID = true, 999
	zero := groupTestMessage(54, time.Unix(4, 0))
	zero.HasReply = true
	crossChatTarget := groupTestMessage(999, time.Unix(5, 0))
	crossChatTarget.ChatID = 200

	groups := GroupMessages(domain.ChatBasicGroup, []domain.Message{target, reply, missing, zero, crossChatTarget}, time.UTC)
	if context := groups[1].ReplyContexts[reply.ID]; !context.Available || context.Sender != "You" {
		t.Fatal("outgoing sender fallback mismatch")
	}
	for _, index := range []int{2, 3} {
		if context := groups[index].ReplyContexts[groups[index].Messages[0].ID]; context.Available {
			t.Fatal("unavailable reply target was projected as available")
		}
	}
}

func TestReactionsSurviveGrouping(t *testing.T) {
	start := time.Date(2026, time.June, 7, 15, 0, 0, 0, time.UTC)
	message := groupTestMessage(1, start)
	message.Reactions = []domain.MessageReaction{{Emoji: "👍", Count: 3, Chosen: true}, {Emoji: "❤️", Count: 1, Chosen: false}}
	groups := GroupMessages(domain.ChatBasicGroup, []domain.Message{message}, time.UTC)
	if len(groups) != 1 || len(groups[0].Messages) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	got := groups[0].Messages[0].Reactions
	if len(got) != 2 || got[0].Emoji != "👍" || got[0].Count != 3 || !got[0].Chosen || got[1].Emoji != "❤️" || got[1].Count != 1 || got[1].Chosen {
		t.Fatalf("grouped reactions = %#v", got)
	}
}

func TestPrivateMessageMetadataFramesEveryNonServiceMessage(t *testing.T) {
	message := groupTestMessage(1, time.Date(2026, time.July, 9, 10, 5, 0, 0, time.UTC))
	groups := GroupMessages(domain.ChatPrivate, []domain.Message{message, message}, time.UTC)
	if len(groups) != 2 {
		t.Fatalf("private groups = %d, want 2", len(groups))
	}
	for _, group := range groups {
		if !group.ShowAvatar || group.SenderName != "Sender" || len(group.Messages) != 1 {
			t.Fatalf("private metadata group = %#v", group)
		}
	}
	outgoing := message
	outgoing.Outgoing, outgoing.SenderName, outgoing.SenderAvatar = true, "", domain.AvatarRef{}
	group := GroupMessages(domain.ChatPrivate, []domain.Message{outgoing}, time.UTC)[0]
	if !group.ShowAvatar || group.SenderName != "You" || group.SenderAvatar.UniqueID == "" {
		t.Fatalf("outgoing fallback metadata = %#v", group)
	}
}

func TestPinnedMarkerFlagSurvivesGrouping(t *testing.T) {
	start := time.Date(2026, time.July, 9, 11, 30, 0, 0, time.UTC)
	first := groupTestMessage(1, start)
	first.Pinned = true
	second := groupTestMessage(2, start.Add(time.Minute))
	second.Pinned = true
	third := groupTestMessage(3, start.Add(2*time.Minute))

	groups := GroupMessages(domain.ChatBasicGroup, []domain.Message{first, second, third}, time.UTC)
	if len(groups) != 1 {
		t.Fatalf("GroupMessages() returned %d groups, want 1", len(groups))
	}
	got := groups[0].Messages
	if len(got) != 3 || !got[0].Pinned || !got[1].Pinned || got[2].Pinned {
		t.Fatalf("pinned flags not preserved through grouping: %#v", got)
	}
}

func groupTestMessage(id int64, sentAt time.Time) domain.Message {
	return domain.Message{
		ID:           domain.MessageID(id),
		ChatID:       100,
		Sender:       domain.SenderRef{Kind: domain.SenderUser, ID: 1},
		SenderName:   "Sender",
		SenderAvatar: domain.AvatarRef{FileID: 1, UniqueID: "avatar"},
		SentAt:       sentAt,
		Kind:         domain.MessageText,
		Text:         "message",
	}
}
