package telegram

import (
	"context"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestFakeInviteLinksPagingAndMutation(t *testing.T) {
	chatID := domain.ChatID(7)
	fake := NewFake(FakeData{InviteLinks: map[domain.ChatID][]InviteLink{chatID: {
		{URL: "primary", IsPrimary: true, Date: 1},
		{URL: "one", Date: 2},
		{URL: "two", Date: 3},
	}}})
	page, err := fake.LoadInviteLinks(context.Background(), chatID, InviteLinkCursor{Limit: 2})
	if err != nil || page.Primary == nil || len(page.Links) != 1 || page.NextCursor.OffsetURL != "one" {
		t.Fatalf("first page = %#v, err=%v", page, err)
	}
	next, err := fake.LoadInviteLinks(context.Background(), chatID, page.NextCursor)
	if err != nil || len(next.Links) != 1 || next.Links[0].URL != "two" || !next.Done {
		t.Fatalf("next page = %#v, err=%v", next, err)
	}
	created, err := fake.CreateInviteLink(context.Background(), chatID, "new")
	if err != nil || created.URL == "" {
		t.Fatalf("created = %#v, err=%v", created, err)
	}
	if err := fake.RevokeInviteLink(context.Background(), chatID, created.URL); err != nil {
		t.Fatal(err)
	}
	page, err = fake.LoadInviteLinks(context.Background(), chatID, InviteLinkCursor{Limit: 100})
	if err != nil || len(page.Links) != 2 {
		t.Fatalf("after revoke = %#v, err=%v", page, err)
	}
}
