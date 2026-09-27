//go:build tdlib

package telegram

import (
	"reflect"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestMessageEntitiesFormattingTypes(t *testing.T) {
	cases := []struct {
		name   string
		entity td.TextEntityType
		want   domain.TextEntityKind
	}{
		{"code", &td.TextEntityTypeCode{}, domain.EntityCode},
		{"pre", &td.TextEntityTypePre{}, domain.EntityPre},
		{"pre code", &td.TextEntityTypePreCode{}, domain.EntityPre},
		{"quote", &td.TextEntityTypeBlockQuote{}, domain.EntityQuote},
		{"expandable quote", &td.TextEntityTypeExpandableBlockQuote{}, domain.EntityQuote},
		{"spoiler", &td.TextEntityTypeSpoiler{}, domain.EntitySpoiler},
		{"mention", &td.TextEntityTypeMention{}, domain.EntityMention},
		{"hashtag", &td.TextEntityTypeHashtag{}, domain.EntityTag},
		{"command", &td.TextEntityTypeBotCommand{}, domain.EntityCommand},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			content := &td.MessageText{Text: &td.FormattedText{Text: "x", Entities: []*td.TextEntity{{Offset: 0, Length: 1, Type: test.entity}}}}
			got := messageEntities(content)
			if len(got) != 1 || got[0].Kind != test.want {
				t.Fatalf("entities = %#v, want %v", got, test.want)
			}
		})
	}
}

func TestMessageEntitiesLinkTypes(t *testing.T) {
	text := &td.FormattedText{Text: "https://one.example labeled email", Entities: []*td.TextEntity{
		{Offset: 0, Length: 19, Type: &td.TextEntityTypeUrl{}},
		{Offset: 20, Length: 7, Type: &td.TextEntityTypeTextUrl{Url: "https://two.example"}},
		{Offset: 28, Length: 5, Type: &td.TextEntityTypeEmailAddress{}},
	}}
	got := messageEntities(&td.MessageText{Text: text})
	want := []domain.TextEntity{
		{Offset: 0, Length: 19, Kind: domain.EntityLink, Link: domain.LinkPlainURL},
		{Offset: 20, Length: 7, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://two.example"},
		{Offset: 28, Length: 5, Kind: domain.EntityLink},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("link entities = %#v, want %#v", got, want)
	}
}

func TestMessageEntitiesNormalizeAndUpdate(t *testing.T) {
	text := &td.FormattedText{Text: "😀link", Entities: []*td.TextEntity{
		{Offset: 2, Length: 4, Type: &td.TextEntityTypeTextUrl{Url: "https://example.org"}},
		{Offset: 0, Length: 2, Type: &td.TextEntityTypeBold{}},
		{Offset: 0, Length: 2, Type: &td.TextEntityTypeCustomEmoji{}},
	}}
	want := []domain.TextEntity{{Offset: 2, Length: 4, Kind: domain.EntityLink, Link: domain.LinkTextURL, URL: "https://example.org"}, {Offset: 0, Length: 2, Kind: domain.EntityBold}}
	n := newNormalizer()
	message := n.message(&td.Message{Id: 1, Content: &td.MessageText{Text: text}})
	if message.Text != text.Text || !reflect.DeepEqual(message.Entities, want) {
		t.Fatalf("message entities = %#v", message.Entities)
	}
	update, ok := singleUpdate(t, n.update(&td.UpdateMessageContent{MessageId: 1, NewContent: &td.MessageText{Text: text}})).(MessageContentUpdated)
	if !ok || !reflect.DeepEqual(update.Entities, want) {
		t.Fatalf("content update entities = %#v", update)
	}
	for _, content := range []td.MessageContent{
		&td.MessagePhoto{Caption: text}, &td.MessageVideo{Caption: text},
		&td.MessageAudio{Caption: text},
	} {
		caption := n.message(&td.Message{Id: 2, Content: content})
		if caption.Text != text.Text || !reflect.DeepEqual(caption.Entities, want) {
			t.Fatalf("%T caption entities = %#v", content, caption.Entities)
		}
	}
}
