package frontend

import (
	"image"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func selectorTestOptions(ids ...domain.ChatID) []selectorOption {
	options := make([]selectorOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, selectorOption{
			ID:    "chat",
			Label: "Chat",
			Value: app.ActionReceived{Action: app.Activate, ChatID: id},
		})
	}
	return options
}

func TestHuhSelectorHostOwnsSelectAndDisablesSubmit(t *testing.T) {
	host := newSelectorHost()
	if host == nil || host.field == nil {
		t.Fatal("new selector host or Huh Select is nil")
	}
	options := selectorTestOptions(10, 20)
	identity := selectorIdentity{Kind: selectorForward, RequestID: 7, ChatID: 9, MessageID: 2}
	_ = host.Sync(identity, options, options[0].Value, true, 24, 2)
	reserved := []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}),
	}
	for _, binding := range host.field.KeyBinds() {
		for _, reservedKey := range reserved {
			if binding.Enabled() && key.Matches(reservedKey, binding) {
				t.Fatalf("embedded Huh Select retains enabled reserved binding %q: %s", reservedKey.String(), binding.Help().Desc)
			}
		}
	}

	before := host.Value()
	changed, value, _ := host.Update(reserved[0])
	if changed || value != before || host.Value() != before {
		t.Fatalf("disabled Huh submit changed selection: changed=%t value=%#v host=%#v", changed, value, host.Value())
	}
	changed, value, _ = host.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if !changed || value != options[1].Value || host.Value() != options[1].Value {
		t.Fatalf("Huh Down did not navigate semantic selection: changed=%t value=%#v host=%#v", changed, value, host.Value())
	}
}

func TestHuhSelectorHostSynchronizesDynamicSemanticIdentity(t *testing.T) {
	host := newSelectorHost()
	identity := selectorIdentity{Kind: selectorForward, RequestID: 7, ChatID: 9, MessageID: 2}
	first := selectorTestOptions(10, 20, 30)
	selected := first[1].Value
	_ = host.Sync(identity, first, selected, true, 24, 3)
	if host.Identity() != identity || host.Value() != selected || !host.focused || host.width != 24 || host.height != 3 {
		t.Fatalf("initial selector host = id:%#v value:%#v focus:%t geometry:%dx%d", host.Identity(), host.Value(), host.focused, host.width, host.height)
	}
	if got := host.field.GetValue(); got != selected {
		t.Fatalf("initial Huh value = %#v, want %#v", got, selected)
	}

	reordered := []selectorOption{first[1], first[2], first[0]}
	// Simulate a stale legacy index after reorder: authoritativeByIndex now
	// points at ChatID 30, while the still-present semantic selection is 20.
	authoritativeByIndex := reordered[1].Value
	_ = host.Sync(identity, reordered, authoritativeByIndex, true, 18, 2)
	if host.Value() != selected || host.field.GetValue() != selected {
		t.Fatalf("reorder lost semantic selection: host=%#v Huh=%#v", host.Value(), host.field.GetValue())
	}
	if !reflect.DeepEqual(host.Options(), reordered) || host.width != 18 || host.height != 2 {
		t.Fatalf("reordered host options/geometry = %#v %dx%d", host.Options(), host.width, host.height)
	}

	// With unchanged options, an authoritative selection transition is a real
	// navigation/update and must be accepted.
	_ = host.Sync(identity, reordered, reordered[2].Value, true, 18, 2)
	if host.Value() != reordered[2].Value || host.field.GetValue() != reordered[2].Value {
		t.Fatalf("unchanged-options authoritative selection was ignored: host=%#v Huh=%#v", host.Value(), host.field.GetValue())
	}

	// If refresh removes the prior semantic value, use the valid authoritative
	// fallback supplied by the snapshot policy.
	refreshed := []selectorOption{reordered[0], reordered[1]}
	_ = host.Sync(identity, refreshed, refreshed[1].Value, true, 18, 2)
	if host.Value() != refreshed[1].Value || host.field.GetValue() != refreshed[1].Value {
		t.Fatalf("removed semantic value did not use authoritative fallback: host=%#v Huh=%#v", host.Value(), host.field.GetValue())
	}

	// Rebuilding an already-focused field to clear empty options must restore
	// both focus and the reserved-key policy. Repopulation does not itself
	// transition logical focus, so Down proves the replacement was focused.
	_ = host.Sync(identity, nil, app.ActionReceived{}, true, 18, 2)
	_ = host.Sync(identity, refreshed, refreshed[0].Value, true, 18, 2)
	expectedValue := refreshed[0].Value
	expectedField := newSelectorField(&expectedValue, 18, 2)
	expectedField.Options(selectorHuhOptions(refreshed)...)
	expectedField.Value(&expectedValue)
	_ = expectedField.Focus()
	expectedView := clipSelectorHuhView(expectedField.View(), 18, 2)
	if host.View() != expectedView {
		t.Fatalf("empty rebuild did not restore focused Huh view: got=%q want=%q", host.View(), expectedView)
	}
	lines := strings.Split(host.View(), "\n")
	if len(lines) != 2 || ansi.StringWidth(lines[0]) != 18 || ansi.StringWidth(lines[1]) != 18 {
		t.Fatalf("empty rebuild did not restore Huh geometry: lines=%d widths=%d,%d view=%q", len(lines), ansi.StringWidth(lines[0]), ansi.StringWidth(lines[len(lines)-1]), host.View())
	}
	changed, value, _ := host.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if !changed || value != refreshed[1].Value {
		t.Fatalf("focused empty rebuild did not restore Huh navigation: changed=%t value=%#v", changed, value)
	}
	changed, value, _ = host.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if changed || value != refreshed[1].Value {
		t.Fatalf("empty rebuild restored an enabled Enter binding: changed=%t value=%#v", changed, value)
	}
}

func TestHuhSelectorHostIdentityAndAuthoritativeRefreshReset(t *testing.T) {
	host := newSelectorHost()
	firstID := selectorIdentity{Kind: selectorMessageActions, RequestID: 1, ChatID: 9, MessageID: 2}
	first := selectorTestOptions(10, 20)
	_ = host.Sync(firstID, first, first[1].Value, true, 24, 2)

	secondID := selectorIdentity{Kind: selectorReaction, RequestID: 2, ChatID: 9, MessageID: 2}
	second := selectorTestOptions(30, 40)
	_ = host.Sync(secondID, second, second[0].Value, false, 0, 0)
	if host.Identity() != secondID || host.Value() != second[0].Value || host.field.GetValue() != second[0].Value {
		t.Fatalf("identity refresh retained stale selector state: id=%#v host=%#v Huh=%#v", host.Identity(), host.Value(), host.field.GetValue())
	}
	if host.focused || host.width != 1 || host.height != 1 {
		t.Fatalf("identity refresh focus/geometry = %t %dx%d, want false 1x1", host.focused, host.width, host.height)
	}

	// Huh v2 Select.Options is a no-op for an empty slice. A non-empty to
	// empty transition must nevertheless clear both the bound value and the
	// field's internal options; retaining the old field would leave stale rows.
	emptyID := selectorIdentity{Kind: selectorForward, RequestID: 3}
	missing := app.ActionReceived{Action: app.Activate, ChatID: 999}
	_ = host.Sync(emptyID, nil, missing, true, 0, 0)
	if host.Identity() != emptyID || host.Value() != (app.ActionReceived{}) || host.field.GetValue() != (app.ActionReceived{}) {
		t.Fatalf("empty identity reset = id:%#v host:%#v Huh:%#v", host.Identity(), host.Value(), host.field.GetValue())
	}
	if len(host.Options()) != 0 || strings.Contains(host.View(), "Chat") {
		t.Fatalf("empty identity retained stale options/view: options=%#v view=%q", host.Options(), host.View())
	}
	if !host.focused || host.width != 1 || host.height != 1 {
		t.Fatalf("empty identity focus/geometry = %t %dx%d, want true 1x1", host.focused, host.width, host.height)
	}
}

func TestHuhSelectorHostExplicitAuthoritativeOverrideOnOptionChange(t *testing.T) {
	identity := selectorIdentity{Kind: selectorMessageActions, RequestID: 7, ChatID: 9, MessageID: 2}
	copyValue := app.ActionReceived{Action: app.CopyMessage, ChatID: 9, MessageID: 2}
	editValue := app.ActionReceived{Action: app.EditMessage, ChatID: 9, MessageID: 2}
	loading := []selectorOption{{ID: "copy", Label: "Copy", Value: copyValue}}
	settled := []selectorOption{
		{ID: "edit", Label: "Edit", Value: editValue},
		{ID: "copy", Label: "Copy", Value: copyValue},
	}

	preserving := newSelectorHost()
	_ = preserving.Sync(identity, loading, copyValue, true, 18, 1)
	_ = preserving.Sync(identity, settled, editValue, true, 18, 2)
	if preserving.Value() != copyValue {
		t.Fatalf("ordinary Sync stopped preserving current semantic value: %#v", preserving.Value())
	}

	overriding := newSelectorHost()
	_ = overriding.Sync(identity, loading, copyValue, true, 18, 1)
	_ = overriding.SyncAuthoritative(identity, settled, editValue, true, 18, 2)
	if overriding.Value() != editValue || overriding.field.GetValue() != editValue {
		t.Fatalf("explicit authoritative Sync selected host=%#v Huh=%#v, want Edit", overriding.Value(), overriding.field.GetValue())
	}
}

func TestSelectorHostRectUsesSharedModalRowsGeometry(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 24)
	if got, want := selectorHostRect(bounds, 3, 3), image.Rect(28, 10, 52, 13); !got.Eq(want) {
		t.Fatalf("selector host rect = %v, want %v", got, want)
	}
	// A non-selectable loading/error status row affects modal centering/height,
	// but is excluded from the Huh Select host's own row count.
	if got, want := selectorHostRect(bounds, 2, 3), image.Rect(28, 10, 52, 12); !got.Eq(want) {
		t.Fatalf("selector host rect with status = %v, want %v", got, want)
	}
	if got, want := selectorHostRectWidth(bounds, 2, 2, 64), image.Rect(10, 10, 70, 12); !got.Eq(want) {
		t.Fatalf("wide selector host rect = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		bounds                image.Rectangle
		optionCount, rowCount int
	}{
		{image.Rectangle{}, 3, 3},
		{image.Rect(0, 0, 1, 1), 0, 1},
		{image.Rect(4, 5, 4, 5), 1, 1},
	} {
		if got := selectorHostRect(tc.bounds, tc.optionCount, tc.rowCount); !got.Empty() {
			t.Errorf("selectorHostRect(%v,%d,%d) = %v, want empty", tc.bounds, tc.optionCount, tc.rowCount, got)
		}
	}
}
