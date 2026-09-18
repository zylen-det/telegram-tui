package ui

import (
	"context"

	gotui "github.com/metaspartan/gotui/v5"
)

type Backend interface {
	PollEventsWithContext(context.Context) <-chan gotui.Event
	TerminalDimensions() (int, int)
	Clear()
	Render(...gotui.Drawable)
	Close()
}
