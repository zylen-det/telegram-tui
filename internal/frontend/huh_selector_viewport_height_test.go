package frontend

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHuhSelectorHostViewUsesEveryRequestedRowForOptions(t *testing.T) {
	options := []selectorOption{
		{ID: "alpha", Label: "Alpha", Value: ActionReceived{Action: CopyMessage, ChatID: 1}},
		{ID: "beta", Label: "Beta", Value: ActionReceived{Action: ReplyMessage, ChatID: 1}},
		{ID: "gamma", Label: "Gamma", Value: ActionReceived{Action: EditMessage, ChatID: 1}},
	}
	host := newSelectorHost()
	_ = host.Sync(selectorIdentity{Kind: selectorMessageActions, RequestID: 1}, options, options[0].Value, true, 24, 3)

	view := host.View()
	rows := strings.Split(view, "\n")
	if len(rows) != 3 {
		t.Fatalf("host View rows = %d, want exact requested 3: %q", len(rows), ansi.Strip(view))
	}
	for index, row := range rows {
		if width := ansi.StringWidth(row); width != 24 {
			t.Fatalf("host View row %d width = %d, want 24: %q", index, width, ansi.Strip(row))
		}
	}
	plain := ansi.Strip(view)
	for _, label := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(plain, label) {
			t.Fatalf("host View omitted option %q from requested rows: %q", label, plain)
		}
	}
}

func TestNewSelectorFieldCompensatesRequestedVisibleHeight(t *testing.T) {
	options := []selectorOption{
		{ID: "first", Label: "First", Value: ActionReceived{Action: CopyMessage, ChatID: 1}},
		{ID: "second", Label: "Second", Value: ActionReceived{Action: ReplyMessage, ChatID: 1}},
	}
	value := options[0].Value
	field := newSelectorField(&value, 12, 2)
	field.Options(selectorHuhOptions(options)...)
	field.Value(&value)
	_ = field.Focus()
	plain := ansi.Strip(clipSelectorHuhView(field.View(), 12, 2))
	if !strings.Contains(plain, "First") || !strings.Contains(plain, "Second") {
		t.Fatalf("newSelectorField omitted an option from requested visible height: %q", plain)
	}
}

func TestHuhSelectorEmptyRebuildRetainsCompensatedFieldHeight(t *testing.T) {
	identity := selectorIdentity{Kind: selectorMessageActions, RequestID: 3}
	options := []selectorOption{
		{ID: "first", Label: "First", Value: ActionReceived{Action: CopyMessage, ChatID: 1}},
		{ID: "second", Label: "Second", Value: ActionReceived{Action: ReplyMessage, ChatID: 1}},
	}
	host := newSelectorHost()
	_ = host.Sync(identity, options, options[0].Value, true, 12, 2)
	_ = host.Sync(identity, nil, ActionReceived{}, true, 12, 2)

	// Probe the fresh replacement field directly before another Sync can
	// reapply dimensions. This isolates the empty-transition constructor path.
	host.value = options[0].Value
	host.field.Options(selectorHuhOptions(options)...)
	host.field.Value(&host.value)
	plain := ansi.Strip(host.View())
	if !strings.Contains(plain, "First") || !strings.Contains(plain, "Second") {
		t.Fatalf("empty rebuild replacement omitted an option: %q", plain)
	}
}

func TestHuhSelectorHostViewClipsInternalPaddingAndWideANSI(t *testing.T) {
	options := []selectorOption{
		{ID: "wide", Label: "界界界", Value: ActionReceived{Action: CopyMessage, ChatID: 1}},
		{ID: "second", Label: "Second", Value: ActionReceived{Action: ReplyMessage, ChatID: 1}},
	}
	host := newSelectorHost()
	_ = host.Sync(selectorIdentity{Kind: selectorMessageActions, RequestID: 2}, options, options[0].Value, true, 8, 2)
	view := host.View()
	rows := strings.Split(view, "\n")
	if len(rows) != 2 {
		t.Fatalf("clipped View rows = %d, want 2: %q", len(rows), ansi.Strip(view))
	}
	for index, row := range rows {
		if width := ansi.StringWidth(row); width > 8 {
			t.Fatalf("clipped View row %d width = %d, want <=8: %q", index, width, ansi.Strip(row))
		}
	}
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "界界界") || !strings.Contains(plain, "Second") {
		t.Fatalf("clipped View lost visible options: %q", plain)
	}
}

func TestClipSelectorHuhViewBounds(t *testing.T) {
	view := "\x1b[31m1234567890界AB\x1b[0m\nsecond界row\nthird-forbidden"
	got := clipSelectorHuhView(view, 10, 2)
	rows := strings.Split(got, "\n")
	if len(rows) != 2 {
		t.Fatalf("clipped rows = %d, want 2: %q", len(rows), got)
	}
	for index, row := range rows {
		if width := ansi.StringWidth(row); width > 10 {
			t.Fatalf("row %d width = %d, want <=10: %q", index, width, row)
		}
	}
	if plain, want := ansi.Strip(got), "1234567890\nsecond界ro"; plain != want {
		t.Fatalf("cell-safe clipped text = %q, want %q", plain, want)
	}
	if strings.Contains(got, "third-forbidden") {
		t.Fatalf("height clip retained third row: %q", got)
	}
	if got := clipSelectorHuhView("non-empty", 0, 2); got != "" {
		t.Fatalf("zero-width clip = %q, want empty", got)
	}
}
