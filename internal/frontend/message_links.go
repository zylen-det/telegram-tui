package frontend

import (
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/zylen-det/tuilegram/internal/domain"
)

type messageLink struct {
	URL   string // target to copy or open
	Label string // plain URL or target(display text) for text-url entities
}

// messageLinks uses only TDLib's URL and text-url entities, in text/caption order.
// Their ranges are UTF-16; no plain-text URL detection is performed here.
func messageLinks(message domain.Message) []messageLink {
	var links []messageLink
	for _, entity := range message.Entities {
		if entity.Kind != domain.EntityLink || entity.Link == 0 {
			continue
		}
		text, ok := utf16Slice(message.Text, entity.Offset, entity.Length)
		if !ok || strings.IndexFunc(text, unicode.IsControl) >= 0 {
			continue
		}
		switch entity.Link {
		case domain.LinkPlainURL:
			links = append(links, messageLink{URL: text, Label: text})
		case domain.LinkTextURL:
			if entity.URL != "" && strings.IndexFunc(entity.URL, unicode.IsControl) < 0 {
				links = append(links, messageLink{URL: entity.URL, Label: entity.URL + "(" + text + ")"})
			}
		}
	}
	return links
}

func utf16Slice(text string, offset, length int) (string, bool) {
	if offset < 0 || length <= 0 {
		return "", false
	}
	start, end := -1, -1
	position := 0
	for index, r := range text {
		if position == offset {
			start = index
		}
		if position == offset+length {
			end = index
		}
		position += utf16.RuneLen(r)
	}
	if position == offset {
		start = len(text)
	}
	if position == offset+length {
		end = len(text)
	}
	if start < 0 || end < start {
		return "", false
	}
	return text[start:end], true
}
