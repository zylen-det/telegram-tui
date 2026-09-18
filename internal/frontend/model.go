package frontend

import tea "charm.land/bubbletea/v2"

// Model is the standalone authorization shell state.
type Model struct {
	width     int
	height    int
	input     []rune
	submitted bool
}

// NewModel creates the initial authorization shell model.
func NewModel() Model {
	return Model{width: 80, height: 24}
}

func (Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(0, msg.Width)
		m.height = max(0, msg.Height)
	case tea.PasteMsg:
		m.input = append(m.input, []rune(msg.Content)...)
		m.submitted = false
	case tea.KeyPressMsg:
		key := msg.Key()
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			m.submitted = false
		case "enter":
			m.submitted = true
		default:
			if key.Text != "" {
				m.input = append(m.input, []rune(key.Text)...)
				m.submitted = false
			}
		}
	}
	return m, nil
}
