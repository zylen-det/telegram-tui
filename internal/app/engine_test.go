package app

import (
	"reflect"
	"testing"
)

func TestEngineAppliesEventsInOrder(t *testing.T) {
	initial := InitialState()
	initial.Focus = FocusConversation
	engine := NewEngine(initial)

	engine.Apply(Resized{Width: 140, Height: 30})
	engine.Apply(ActionReceived{Action: ToggleDetails})

	got := engine.Snapshot()
	if got.Width != 140 || got.Height != 30 {
		t.Fatalf("dimensions = %dx%d, want 140x30", got.Width, got.Height)
	}
	if got.Layout != LayoutWide {
		t.Fatalf("Layout = %v, want %v", got.Layout, LayoutWide)
	}
	if !got.DetailsOpen {
		t.Fatal("DetailsOpen = false, want true")
	}
	if got.Focus != FocusDetails {
		t.Fatalf("Focus = %v, want %v", got.Focus, FocusDetails)
	}
	if got.FocusBeforeInfo != FocusConversation {
		t.Fatalf("FocusBeforeInfo = %v, want %v", got.FocusBeforeInfo, FocusConversation)
	}
}

func TestEngineReturnsReducerCommandsWithoutDiscardingPriorState(t *testing.T) {
	engine := NewEngine(InitialState())
	engine.Apply(Resized{Width: 140, Height: 30})

	commands := engine.Apply(Started{})

	wantCommands := []Command{LoadBootstrap{}}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", commands, wantCommands)
	}
	got := engine.Snapshot()
	if got.Width != 140 || got.Height != 30 || got.Layout != LayoutWide {
		t.Fatalf("snapshot after Started = %#v, want prior resize preserved", got)
	}
}
