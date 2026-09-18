package telegram

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeLoadBotCommandsClonesCatalogAndReturnsConfiguredError(t *testing.T) {
	catalog := []domain.BotCommand{{Name: "start", Description: "Start"}}
	fake := NewFake(FakeData{BotCommands: map[domain.ChatID][]domain.BotCommand{9: catalog}})
	got, err := fake.LoadBotCommands(context.Background(), 9)
	if err != nil || !reflect.DeepEqual(got, catalog) {
		t.Fatalf("LoadBotCommands = %#v, %v", got, err)
	}
	got[0].Name = "changed"
	again, _ := fake.LoadBotCommands(context.Background(), 9)
	if again[0].Name != "start" {
		t.Fatal("fake command catalog leaked caller mutation")
	}

	sentinel := errors.New("unavailable")
	_, err = NewFake(FakeData{BotCommandsError: sentinel}).LoadBotCommands(context.Background(), 9)
	if !errors.Is(err, sentinel) {
		t.Fatalf("configured error = %v", err)
	}
}
