package tui

import tea "github.com/charmbracelet/bubbletea"

// MultiSelectItem is one toggleable row. Header rows (Header=true) are non-selectable labels.
type MultiSelectItem struct {
	Label  string
	Key    string // identity returned in Chosen (empty for headers)
	Header bool
}

// MultiSelect is a space-toggle list returning chosen Keys on enter.
type MultiSelect struct {
	Title   string
	Items   []MultiSelectItem
	cursor  int
	checked map[int]bool
	Chosen  []string
	Aborted bool
}

func NewMultiSelect(title string, items []MultiSelectItem) MultiSelect {
	m := MultiSelect{Title: title, Items: items, checked: map[int]bool{}}
	// start cursor on first selectable
	for i, it := range items {
		if !it.Header {
			m.cursor = i
			break
		}
	}
	return m
}

func (m MultiSelect) Init() tea.Cmd { return nil }

func (m MultiSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "ctrl+c", "q", "esc":
		m.Aborted = true
		return m, tea.Quit
	case "up", "k":
		for i := m.cursor - 1; i >= 0; i-- {
			if !m.Items[i].Header {
				m.cursor = i
				break
			}
		}
	case "down", "j":
		for i := m.cursor + 1; i < len(m.Items); i++ {
			if !m.Items[i].Header {
				m.cursor = i
				break
			}
		}
	case " ":
		if !m.Items[m.cursor].Header {
			m.checked[m.cursor] = !m.checked[m.cursor]
		}
	case "enter":
		for i, it := range m.Items {
			if m.checked[i] && !it.Header {
				m.Chosen = append(m.Chosen, it.Key)
			}
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m MultiSelect) View() string {
	s := Title.Render(m.Title) + "\n\n"
	for i, it := range m.Items {
		if it.Header {
			s += "\n" + Dim.Render(it.Label) + "\n"
			continue
		}
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		box := "[ ]"
		if m.checked[i] {
			box = "[x]"
		}
		s += cur + box + " " + it.Label + "\n"
	}
	s += "\n" + Dim.Render("↑/↓ move · space toggle · enter confirm · q quit")
	return s
}
