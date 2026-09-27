package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/zylen-det/tuilegram/internal/domain"
	"github.com/zylen-det/tuilegram/internal/frontend"
	"github.com/zylen-det/tuilegram/internal/telegram"
)

func TestDemoDataRunsWithoutAccountAndLoadsMedia(t *testing.T) {
	root := t.TempDir()
	photo, err := copyDemoAsset(root, "demo-photo.png")
	if err != nil {
		t.Fatal(err)
	}
	avatar, err := copyDemoAsset(root, "demo-avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (noAccountResolver{}).Resolve(context.Background()); err != nil {
		t.Fatalf("demo resolver asked for credentials: %v", err)
	}
	client := telegram.NewFake(demoData(photo, avatar))
	chats, err := client.LoadChats(context.Background(), telegram.ChatCursor{Limit: 50})
	if err != nil || len(chats.Chats) < 3 || chats.Chats[0].Title != "Terminal Makers" {
		t.Fatalf("demo chats = %+v, err = %v", chats, err)
	}
	page, err := client.LoadMessages(context.Background(), 1, telegram.MessageCursor{})
	if err != nil || len(page.Messages) == 0 {
		t.Fatalf("demo messages = %+v, err = %v", page, err)
	}
	last := page.Messages[len(page.Messages)-1]
	if len(last.Entities) != 1 {
		t.Fatalf("demo link entities = %+v", last.Entities)
	}
	link := last.Entities[0]
	units := utf16.Encode([]rune(last.Text))
	if link.Offset < 0 || link.Length <= 0 || link.Offset+link.Length > len(units) ||
		string(utf16.Decode(units[link.Offset:link.Offset+link.Length])) != "tuilegram" ||
		link.Kind != domain.EntityLink || link.Link != domain.LinkTextURL ||
		link.URL != "https://github.com/zylen-det/tuilegram" {
		t.Fatalf("demo text URL = %+v in %q", link, last.Text)
	}
	var photoRef domain.MediaFileRef
	photos := 0
	for _, message := range page.Messages {
		if message.Kind == domain.MessagePhoto {
			photos++
			photoRef = message.Media.Thumbnail
		}
	}
	if photos != 1 || photoRef.ID == 0 {
		t.Fatalf("demo photo count = %d, thumbnail = %+v", photos, photoRef)
	}
	state := frontend.InitialState()
	state.Chats = chats.Chats
	state.Messages[1] = page.Messages
	groups := frontend.Select(state, time.Local).Groups
	colors := make(map[domain.SenderRef]bool)
	for _, group := range groups {
		if group.HasSenderColor {
			colors[group.Sender] = true
		}
	}
	if len(colors) < 3 || groups[0].SenderColor == groups[1].SenderColor {
		t.Fatalf("demo sender colors = %+v", groups)
	}
	file, err := client.DownloadMedia(context.Background(), photoRef)
	if err != nil || file.Path != photo {
		t.Fatalf("demo image = %+v, err = %v", file, err)
	}
	avatars := 0
	for _, chat := range chats.Chats {
		if chat.Avatar.FileID != 0 {
			avatars++
			file, err := client.DownloadAvatar(context.Background(), chat.Avatar, telegram.AvatarSmall)
			if err != nil || file.Path != avatar {
				t.Fatalf("demo avatar = %+v, err = %v", file, err)
			}
		}
	}
	if avatars != 1 {
		t.Fatalf("demo avatar count = %d, want 1", avatars)
	}
}

func TestDemoSendTextSettlesAndSurvivesReload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newDemoClient(ctx, demoData(filepath.Join(t.TempDir(), "photo.png"), ""))
	updates := make(chan telegram.Update, 4)
	go func() { _ = client.Start(ctx, updates) }()
	select {
	case update := <-updates:
		if _, ok := update.(telegram.Ready); !ok {
			t.Fatalf("first demo update = %T, want Ready", update)
		}
	case <-time.After(time.Second):
		t.Fatal("demo did not start")
	}
	select {
	case update := <-updates:
		connection, ok := update.(telegram.ConnectionChanged)
		if !ok || connection.State != domain.ConnectionOnline {
			t.Fatalf("demo connection = %+v, want online", update)
		}
	case <-time.After(time.Second):
		t.Fatal("demo never became online")
	}
	queued, err := client.SendText(ctx, telegram.SendTextRequest{ChatID: 1, Text: "Hello from the demo"})
	if err != nil || queued.ID >= 0 || queued.SendState != domain.SendPending {
		t.Fatalf("queued = %+v, err = %v", queued, err)
	}
	var settled domain.Message
	select {
	case update := <-updates:
		success, ok := update.(telegram.MessageSendSucceeded)
		if !ok || success.OldID != queued.ID {
			t.Fatalf("send update = %+v", update)
		}
		settled = success.Message
	case <-time.After(time.Second):
		t.Fatal("demo send never completed")
	}
	if settled.ID <= 0 || settled.SendState != domain.SendSucceeded || settled.Text != "Hello from the demo" || !settled.Outgoing {
		t.Fatalf("settled message = %+v", settled)
	}
	page, err := client.LoadMessages(ctx, 1, telegram.MessageCursor{})
	if err != nil || len(page.Messages) == 0 {
		t.Fatalf("reloaded send = %+v, err = %v", page, err)
	}
	last := page.Messages[len(page.Messages)-1]
	if last.ID != settled.ID || last.Text != settled.Text || last.SendState != domain.SendSucceeded {
		t.Fatalf("reloaded send = %+v", last)
	}
}
