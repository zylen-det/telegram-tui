//go:build tdlib

package telegram

import (
	"context"
	"reflect"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAdapterLoadBotCommandsForPrivateBot(t *testing.T) {
	transport := &dataTransport{
		chatByID: map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypePrivate{UserId: 70}}},
		userFull: map[int64]*td.UserFullInfo{70: {BotInfo: &td.BotInfo{Commands: []*td.BotCommand{
			{Command: "start", Description: "Start bot"},
			{Command: "/help", Description: "Show help"},
		}}}},
	}
	got, err := adapterForDataTests(transport).LoadBotCommands(context.Background(), 9)
	want := []domain.BotCommand{{Name: "start", Description: "Start bot"}, {Name: "help", Description: "Show help"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadBotCommands = %#v, %v; want %#v", got, err, want)
	}
}

func TestAdapterLoadBotCommandsForGroupsIncludesBotUsername(t *testing.T) {
	for _, test := range []struct {
		name      string
		chatType  td.ChatType
		configure func(*dataTransport)
	}{
		{
			name:     "basic group",
			chatType: &td.ChatTypeBasicGroup{BasicGroupId: 80},
			configure: func(transport *dataTransport) {
				transport.basicFull = map[int64]*td.BasicGroupFullInfo{80: {BotCommands: []*td.BotCommands{{BotUserId: 70, Commands: []*td.BotCommand{{Command: "help", Description: "Show help"}}}}}}
			},
		},
		{
			name:     "supergroup",
			chatType: &td.ChatTypeSupergroup{SupergroupId: 81},
			configure: func(transport *dataTransport) {
				transport.superFull = map[int64]*td.SupergroupFullInfo{81: {BotCommands: []*td.BotCommands{{BotUserId: 70, Commands: []*td.BotCommand{{Command: "help", Description: "Show help"}}}}}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &dataTransport{
				chatByID: map[int64]*td.Chat{9: {Id: 9, Type: test.chatType}},
				userByID: map[int64]*td.User{70: {Id: 70, Usernames: &td.Usernames{ActiveUsernames: []string{"helper_bot"}}}},
			}
			test.configure(transport)
			got, err := adapterForDataTests(transport).LoadBotCommands(context.Background(), 9)
			want := []domain.BotCommand{{Name: "help", Description: "Show help", BotUsername: "helper_bot"}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("LoadBotCommands = %#v, %v; want %#v", got, err, want)
			}
		})
	}
}

func TestAdapterLoadBotCommandsReturnsEmptyForNonBotPrivateChat(t *testing.T) {
	transport := &dataTransport{
		chatByID: map[int64]*td.Chat{9: {Id: 9, Type: &td.ChatTypePrivate{UserId: 70}}},
		userFull: map[int64]*td.UserFullInfo{70: {}},
	}
	got, err := adapterForDataTests(transport).LoadBotCommands(context.Background(), 9)
	if err != nil || len(got) != 0 {
		t.Fatalf("LoadBotCommands = %#v, %v; want empty", got, err)
	}
}
