package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// SelectModel is a minimal arrow-key chooser returning the chosen index via Chosen.
type SelectModel struct {
	Title   string
	Items   []string
	cursor  int
	Chosen  int
	Aborted bool
}

func NewSelect(title string, items []string) SelectModel {
	return SelectModel{Title: title, Items: items, Chosen: -1}
}

func (m SelectModel) Init() tea.Cmd { return nil }

func (m SelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "q", "esc":
			m.Aborted = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case "enter":
			m.Chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SelectModel) View() string {
	s := Title.Render(m.Title) + "\n\n"
	for i, it := range m.Items {
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		s += cur + it + "\n"
	}
	s += "\n" + Dim.Render("↑/↓ move · enter select · q quit")
	return s
}
