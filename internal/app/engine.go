package app

type Engine struct {
	state State
}

func NewEngine(initial State) *Engine {
	return &Engine{state: initial}
}

func (e *Engine) Apply(event Event) []Command {
	state, commands := Reduce(e.state, event)
	e.state = state
	return commands
}

func (e *Engine) Snapshot() State {
	return e.state
}
