package ui

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestComputeLayoutBreakpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		want          app.Layout
	}{
		{name: "wide boundary", width: 120, height: 24, want: app.LayoutWide},
		{name: "below wide width", width: 119, height: 24, want: app.LayoutNormal},
		{name: "normal boundary", width: 80, height: 20, want: app.LayoutNormal},
		{name: "below normal width", width: 79, height: 20, want: app.LayoutNarrow},
		{name: "narrow boundary", width: 60, height: 18, want: app.LayoutNarrow},
		{name: "below minimum width", width: 59, height: 30, want: app.LayoutTooSmall},
		{name: "below minimum height", width: 120, height: 17, want: app.LayoutTooSmall},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ComputeLayout(test.width, test.height, false, app.FocusChats)
			if got.Mode != test.want {
				t.Fatalf("ComputeLayout(%d, %d).Mode = %v, want %v", test.width, test.height, got.Mode, test.want)
			}
			if want := image.Rect(0, 0, test.width, 1); got.Status != want {
				t.Errorf("ComputeLayout(%d, %d).Status = %v, want %v", test.width, test.height, got.Status, want)
			}
		})
	}
}

func TestComputeLayoutModeMatchesAppResizeReducer(t *testing.T) {
	t.Parallel()

	// Values surround every classifier threshold so a one-sided mutation is detected.
	widths := []int{-1, 0, 1, 59, 60, 61, 79, 80, 81, 119, 120, 121}
	heights := []int{-1, 0, 1, 17, 18, 19, 20, 21, 23, 24, 25}

	for _, width := range widths {
		for _, height := range heights {
			resized, _ := app.Reduce(app.InitialState(), app.Resized{Width: width, Height: height})

			got := ComputeLayout(width, height, false, app.FocusChats).Mode
			if got != resized.Layout {
				t.Errorf("ComputeLayout(%d, %d).Mode = %v, app reducer Layout = %v", width, height, got, resized.Layout)
			}
		}
	}
}

func TestComputeLayoutDegenerateTerminalHasNoRectangles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		details       bool
		focus         app.Focus
	}{
		{name: "zero width", width: 0, height: 24, focus: app.FocusChats},
		{name: "negative width", width: -1, height: 24, details: true, focus: app.FocusDetails},
		{name: "zero height", width: 120, height: 0, focus: app.FocusConversation},
		{name: "negative height", width: 120, height: -1, details: true, focus: app.FocusDetails},
		{name: "both zero", width: 0, height: 0, details: true, focus: app.FocusDetails},
		{name: "both negative", width: -1, height: -1, focus: app.FocusChats},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ComputeLayout(test.width, test.height, test.details, test.focus)
			if got.Mode != app.LayoutTooSmall {
				t.Errorf("ComputeLayout(%d, %d).Mode = %v, want %v", test.width, test.height, got.Mode, app.LayoutTooSmall)
			}
			for name, pane := range map[string]image.Rectangle{
				"Status":       got.Status,
				"Chats":        got.Chats,
				"Conversation": got.Conversation,
				"Details":      got.Details,
			} {
				if !pane.Empty() {
					t.Errorf("ComputeLayout(%d, %d).%s = %v, want empty", test.width, test.height, name, pane)
				}
				if pane != (image.Rectangle{}) {
					t.Errorf("ComputeLayout(%d, %d).%s = %v, want zero rectangle with no out-of-terminal coordinates", test.width, test.height, name, pane)
				}
			}
		})
	}
}

func TestComputeLayoutPanes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		details       bool
		focus         app.Focus
		want          Layout
	}{
		{
			name: "wide without details", width: 120, height: 24, focus: app.FocusChats,
			want: Layout{
				Mode:         app.LayoutWide,
				Status:       image.Rect(0, 0, 120, 1),
				Chats:        image.Rect(0, 1, 32, 24),
				Conversation: image.Rect(32, 1, 120, 24),
			},
		},
		{
			name: "wide with details", width: 140, height: 25, details: true, focus: app.FocusDetails,
			want: Layout{
				Mode:         app.LayoutWide,
				Status:       image.Rect(0, 0, 140, 1),
				Chats:        image.Rect(0, 1, 32, 25),
				Conversation: image.Rect(32, 1, 108, 25),
				Details:      image.Rect(108, 1, 140, 25),
			},
		},
		{
			name: "normal without details", width: 100, height: 22, focus: app.FocusConversation,
			want: Layout{
				Mode:         app.LayoutNormal,
				Status:       image.Rect(0, 0, 100, 1),
				Chats:        image.Rect(0, 1, 30, 22),
				Conversation: image.Rect(30, 1, 100, 22),
			},
		},
		{
			name: "normal with details", width: 100, height: 22, details: true, focus: app.FocusChats,
			want: Layout{
				Mode:    app.LayoutNormal,
				Status:  image.Rect(0, 0, 100, 1),
				Details: image.Rect(0, 1, 100, 22),
			},
		},
		{
			name: "narrow chats focus", width: 79, height: 20, focus: app.FocusChats,
			want: Layout{
				Mode:   app.LayoutNarrow,
				Status: image.Rect(0, 0, 79, 1),
				Chats:  image.Rect(0, 1, 79, 20),
			},
		},
		{
			name: "narrow conversation focus", width: 79, height: 20, focus: app.FocusConversation,
			want: Layout{
				Mode:         app.LayoutNarrow,
				Status:       image.Rect(0, 0, 79, 1),
				Conversation: image.Rect(0, 1, 79, 20),
			},
		},
		{
			name: "narrow details override focus", width: 60, height: 18, details: true, focus: app.FocusChats,
			want: Layout{
				Mode:    app.LayoutNarrow,
				Status:  image.Rect(0, 0, 60, 1),
				Details: image.Rect(0, 1, 60, 18),
			},
		},
		{
			name: "too small has no body panes", width: 120, height: 17, details: true, focus: app.FocusDetails,
			want: Layout{
				Mode:   app.LayoutTooSmall,
				Status: image.Rect(0, 0, 120, 1),
			},
		},
		{
			name: "too small without details has no body panes", width: 59, height: 30, focus: app.FocusChats,
			want: Layout{
				Mode:   app.LayoutTooSmall,
				Status: image.Rect(0, 0, 59, 1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := ComputeLayout(test.width, test.height, test.details, test.focus); got != test.want {
				t.Fatalf("ComputeLayout(%d, %d, %t, %v) = %#v, want %#v", test.width, test.height, test.details, test.focus, got, test.want)
			}
		})
	}
}

func TestComputeLayoutNarrowNonChatFocusShowsConversation(t *testing.T) {
	t.Parallel()

	for _, focus := range []app.Focus{
		app.FocusConversation,
		app.FocusComposer,
		app.FocusDetails,
		app.FocusModal,
		app.FocusAuth,
	} {
		got := ComputeLayout(79, 20, false, focus)
		if got.Chats != (image.Rectangle{}) {
			t.Errorf("focus %v: Chats = %v, want empty", focus, got.Chats)
		}
		if want := image.Rect(0, 1, 79, 20); got.Conversation != want {
			t.Errorf("focus %v: Conversation = %v, want %v", focus, got.Conversation, want)
		}
	}
}

func TestComputeLayoutPanesAreWithinTerminalAndDoNotOverlap(t *testing.T) {
	t.Parallel()

	sizes := []image.Point{
		{X: 120, Y: 24},
		{X: 119, Y: 24},
		{X: 80, Y: 20},
		{X: 79, Y: 20},
		{X: 60, Y: 18},
		{X: 59, Y: 30},
		{X: 120, Y: 17},
	}
	focuses := []app.Focus{app.FocusChats, app.FocusConversation, app.FocusComposer, app.FocusDetails, app.FocusModal, app.FocusAuth}

	for _, size := range sizes {
		for _, details := range []bool{false, true} {
			for _, focus := range focuses {
				layout := ComputeLayout(size.X, size.Y, details, focus)
				terminal := image.Rect(0, 0, size.X, size.Y)
				panes := []image.Rectangle{layout.Status, layout.Chats, layout.Conversation, layout.Details}

				for i, pane := range panes {
					if pane.Empty() {
						continue
					}
					if pane.Intersect(terminal) != pane {
						t.Errorf("size %v details %t focus %v: pane %d %v outside terminal %v", size, details, focus, i, pane, terminal)
					}
					for j := i + 1; j < len(panes); j++ {
						if !panes[j].Empty() && !pane.Intersect(panes[j]).Empty() {
							t.Errorf("size %v details %t focus %v: panes %d %v and %d %v overlap", size, details, focus, i, pane, j, panes[j])
						}
					}
				}
			}
		}
	}
}

func TestHitMapReturnsCompleteClickMetadata(t *testing.T) {
	t.Parallel()

	want := app.ActionReceived{
		Action:      app.SelectChat,
		Rune:        '界',
		ChatID:      domain.ChatID(42),
		MessageID:   domain.MessageID(9),
		AvatarKey:   "avatar-key",
		TargetFocus: app.FocusConversation,
		At:          time.Unix(123, 456),
	}
	hits := HitMap{{Rect: image.Rect(2, 3, 8, 9), Click: want}}

	got, ok := hits.ActionAt(4, 5)
	if !ok {
		t.Fatal("ActionAt() did not find click")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActionAt() = %#v, want %#v", got, want)
	}
}

func TestHitMapUsesReversePriority(t *testing.T) {
	t.Parallel()

	hits := HitMap{
		{Rect: image.Rect(0, 0, 10, 10), Click: app.ActionReceived{Action: app.SelectNext}},
		{Rect: image.Rect(2, 2, 8, 8), Click: app.ActionReceived{Action: app.Activate}},
	}

	got, ok := hits.ActionAt(3, 3)
	if !ok || got.Action != app.Activate {
		t.Fatalf("ActionAt() = (%#v, %t), want topmost Activate", got, ok)
	}
}

func TestHitMapNoActionFallsThroughToLowerHit(t *testing.T) {
	t.Parallel()

	hits := HitMap{
		{Rect: image.Rect(0, 0, 10, 10), Click: app.ActionReceived{Action: app.SelectPrevious}},
		{Rect: image.Rect(0, 0, 10, 10), Click: app.ActionReceived{Action: app.NoAction, AvatarKey: "ignored"}},
	}

	got, ok := hits.ActionAt(5, 5)
	if !ok || got.Action != app.SelectPrevious {
		t.Fatalf("ActionAt() = (%#v, %t), want lower SelectPrevious", got, ok)
	}
}

func TestHitMapHonorsHalfOpenRectangles(t *testing.T) {
	t.Parallel()

	hits := HitMap{{Rect: image.Rect(2, 3, 5, 7), Click: app.ActionReceived{Action: app.Activate}}}

	for _, point := range []image.Point{{X: 5, Y: 4}, {X: 4, Y: 7}, {X: 1, Y: 3}, {X: 2, Y: 2}, {X: 20, Y: 20}} {
		if got, ok := hits.ActionAt(point.X, point.Y); ok {
			t.Errorf("ActionAt(%v) = (%#v, true), want no mapping", point, got)
		}
	}
	if got, ok := hits.ActionAt(4, 6); !ok || got.Action != app.Activate {
		t.Fatalf("ActionAt(interior) = (%#v, %t), want Activate", got, ok)
	}
}

func TestHitMapMapsWheelsIndependently(t *testing.T) {
	t.Parallel()

	wantUp := app.ActionReceived{Action: app.PageUp, ChatID: domain.ChatID(17), At: time.Unix(50, 0)}
	wantDown := app.ActionReceived{Action: app.PageDown, MessageID: domain.MessageID(23), TargetFocus: app.FocusDetails}
	hits := HitMap{{
		Rect:      image.Rect(1, 1, 6, 6),
		WheelUp:   wantUp,
		WheelDown: wantDown,
	}}

	for _, test := range []struct {
		name string
		up   bool
		want app.ActionReceived
	}{{name: "up", up: true, want: wantUp}, {name: "down", up: false, want: wantDown}} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := hits.WheelAt(3, 4, test.up)
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("WheelAt(up=%t) = (%#v, %t), want (%#v, true)", test.up, got, ok, test.want)
			}
		})
	}
}

func TestHitMapWheelPriorityFallbackAndOutside(t *testing.T) {
	t.Parallel()

	hits := HitMap{
		{
			Rect:      image.Rect(0, 0, 10, 10),
			WheelUp:   app.ActionReceived{Action: app.PageUp},
			WheelDown: app.ActionReceived{Action: app.PageDown},
		},
		{
			Rect:      image.Rect(1, 1, 9, 9),
			WheelUp:   app.ActionReceived{Action: app.NoAction},
			WheelDown: app.ActionReceived{Action: app.SelectNext},
		},
	}

	if got, ok := hits.WheelAt(5, 5, true); !ok || got.Action != app.PageUp {
		t.Fatalf("WheelAt(up) = (%#v, %t), want lower PageUp", got, ok)
	}
	if got, ok := hits.WheelAt(5, 5, false); !ok || got.Action != app.SelectNext {
		t.Fatalf("WheelAt(down) = (%#v, %t), want topmost SelectNext", got, ok)
	}
	if got, ok := hits.WheelAt(10, 5, true); ok {
		t.Fatalf("WheelAt(max edge) = (%#v, true), want no mapping", got)
	}
	if got, ok := hits.WheelAt(-1, -1, false); ok {
		t.Fatalf("WheelAt(outside) = (%#v, true), want no mapping", got)
	}
}
