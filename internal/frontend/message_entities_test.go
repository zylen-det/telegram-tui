package frontend

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/tuilegram/internal/domain"
	"github.com/zylen-det/tuilegram/internal/telegram"
)

func TestWrapEntityTextUTF16AndOverlappingStyles(t *testing.T) {
	text := "A😀b́c\nlink"
	entities := []domain.TextEntity{
		{Offset: 1, Length: 5, Kind: domain.EntityBold}, // emoji + combining cluster + c
		{Offset: 3, Length: 3, Kind: domain.EntityItalic},
		{Offset: 7, Length: 4, Kind: domain.EntityLink},
	}
	rows := wrapEntityText(text, entities, 3)
	if len(rows) != 4 || rows[0].text != "A😀" || rows[1].text != "b́c" || rows[2].text != "lin" || rows[3].text != "k" {
		t.Fatalf("wrapped text = %#v", rows)
	}
	bold := uint32(1 << domain.EntityBold)
	italic := uint32(1 << domain.EntityItalic)
	link := uint32(1 << domain.EntityLink)
	if len(rows[0].spans) != 2 || rows[0].spans[1].mask != bold ||
		len(rows[1].spans) != 1 || rows[1].spans[0].mask != bold|italic ||
		rows[2].spans[0].mask != link || rows[3].spans[0].mask != link {
		t.Fatalf("entity spans = %#v", rows)
	}
	styles := newRenderStyles(false)
	for _, row := range rows {
		styled := renderEntityRow(row, styles)
		if ansi.Strip(styled) != row.text || ansi.StringWidth(styled) != ansi.StringWidth(row.text) {
			t.Fatalf("styling changed visible text: %q, want %q", styled, row.text)
		}
	}
	if styled := renderEntityRow(rows[0], styles); !strings.Contains(styled, "\x1b[") {
		t.Fatalf("bold did not produce terminal styling: %q", styled)
	}
}

func TestMessageEntityContentUpdateReplacesOldRanges(t *testing.T) {
	state := InitialState()
	state.Messages[9] = []domain.Message{{ID: 1, ChatID: 9, Kind: domain.MessageText, Text: "old", Entities: []domain.TextEntity{{Offset: 0, Length: 3, Kind: domain.EntityBold}}}}
	incoming := []domain.TextEntity{{Offset: 0, Length: 3, Kind: domain.EntityCode}}
	updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{ChatID: 9, MessageID: 1, Kind: domain.MessageText, Text: "new", Entities: incoming}})
	incoming[0].Kind = domain.EntityItalic
	if got := state.Messages[9][0].Entities; len(got) != 1 || got[0].Kind != domain.EntityCode {
		t.Fatalf("updated entities = %#v", got)
	}
	updateState(&state, TelegramEvent{Value: telegram.MessageContentUpdated{ChatID: 9, MessageID: 1, Kind: domain.MessageText, Text: "plain"}})
	if len(state.Messages[9][0].Entities) != 0 {
		t.Fatalf("stale entities survived plain edit: %#v", state.Messages[9][0].Entities)
	}
}

func TestMessageRowsStyleTextWithoutChangingSelectionGeometry(t *testing.T) {
	message := testMessage(1, 9, "a😀bc")
	message.Entities = []domain.TextEntity{{Offset: 1, Length: 3, Kind: domain.EntityBold}}
	group := styledMessageGroup("Mina", false, message)
	rows, _ := buildMessageRows(group, 6, nil, messageSelection{ChatID: 9, MessageID: 1}, nil)
	if len(rows) != 4 || rows[1].text != "a😀b" || rows[2].text != "c" || rows[1].messageID != 1 || rows[2].messageID != 1 {
		t.Fatalf("message rows = %#v", rows)
	}
	styled := renderEntityRow(rows[1], newRenderStyles(false))
	if ansi.Strip(styled) != rows[1].text || !strings.Contains(styled, "\x1b[") {
		t.Fatalf("styled message row = %q", styled)
	}
}

func TestWrapEntityTextIgnoresBrokenRanges(t *testing.T) {
	for _, entity := range []domain.TextEntity{
		{Offset: 1, Length: 1, Kind: domain.EntityBold}, // splits the surrogate pair
		{Offset: -1, Length: 2, Kind: domain.EntityBold},
		{Offset: 0, Length: 99, Kind: domain.EntityBold},
	} {
		rows := wrapEntityText("😀x", []domain.TextEntity{entity}, 2)
		if len(rows) != 2 || rows[0].text != "😀" || rows[1].text != "x" || rows[0].spans[0].mask != 0 || rows[1].spans[0].mask != 0 {
			t.Fatalf("invalid range %+v affected text: %#v", entity, rows)
		}
	}
}
