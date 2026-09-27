package frontend

import (
	"strings"
	"unicode/utf16"

	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type messageTextSpan struct {
	text string
	mask uint32
}

// wrapEntityText wraps the original text, not ANSI escapes, so UTF-16 entity
// offsets and grapheme/cell boundaries cannot shift when styling is applied.
func wrapEntityText(text string, entities []domain.TextEntity, width int) []messageRowSpec {
	if text == "" || width <= 0 {
		return nil
	}
	// Reject malformed or split-grapheme ranges rather than styling unrelated
	// text. The conversion includes explicit newlines in the UTF-16 positions.
	boundaries := map[int]bool{0: true}
	position := 0
	for rest := text; rest != ""; {
		cluster, _ := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		for _, r := range cluster {
			position += utf16.RuneLen(r)
		}
		boundaries[position] = true
		rest = rest[len(cluster):]
	}
	valid := make([]domain.TextEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.Kind == 0 || entity.Offset < 0 || entity.Length <= 0 || entity.Offset > position || entity.Length > position-entity.Offset ||
			!boundaries[entity.Offset] || !boundaries[entity.Offset+entity.Length] {
			continue
		}
		valid = append(valid, entity)
	}

	var rows []messageRowSpec
	pos := 0
	for _, logical := range strings.Split(text, "\n") {
		row := messageRowSpec{}
		used := 0
		for rest := logical; rest != ""; {
			cluster, cells := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
			rest = rest[len(cluster):]
			length := 0
			for _, r := range cluster {
				length += utf16.RuneLen(r)
			}
			if used > 0 && used+cells > width {
				rows = append(rows, row)
				row = messageRowSpec{}
				used = 0
			}
			if cells > width {
				cluster, cells = "…", 1
			}
			var mask uint32
			for _, entity := range valid {
				if pos >= entity.Offset && pos+length <= entity.Offset+entity.Length {
					mask |= 1 << entity.Kind
				}
			}
			row.text += cluster
			if n := len(row.spans); n > 0 && row.spans[n-1].mask == mask {
				row.spans[n-1].text += cluster
			} else {
				row.spans = append(row.spans, messageTextSpan{text: cluster, mask: mask})
			}
			used += cells
			pos += length
		}
		rows = append(rows, row)
		pos++ // newline in the original text
	}
	return rows
}

func renderEntityRow(row messageRowSpec, styles renderStyles) string {
	base := messageRowStyle(row.kind, styles)
	var out strings.Builder
	for _, span := range row.spans {
		style := base
		mask := span.mask
		has := func(kind domain.TextEntityKind) bool { return mask&(1<<kind) != 0 }
		if has(domain.EntityBold) {
			style = style.Bold(true)
		}
		if has(domain.EntityItalic) || has(domain.EntityQuote) {
			style = style.Italic(true)
		}
		if has(domain.EntityUnderline) || has(domain.EntityLink) || has(domain.EntityMention) || has(domain.EntityTag) || has(domain.EntityCommand) {
			style = style.Underline(true)
		}
		if has(domain.EntityStrikethrough) {
			style = style.Strikethrough(true)
		}
		if has(domain.EntitySpoiler) {
			style = style.Faint(true) // visual cue only; reveal interaction is separate
		}
		if has(domain.EntityCode) || has(domain.EntityPre) {
			style = style.Reverse(true)
		}
		if has(domain.EntityQuote) || has(domain.EntityLink) || has(domain.EntityMention) || has(domain.EntityTag) || has(domain.EntityCommand) {
			style = style.Foreground(styles.Accent.GetForeground())
		}
		out.WriteString(style.Render(span.text))
	}
	return out.String()
}
