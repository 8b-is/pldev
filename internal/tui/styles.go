package tui

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	OK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	Warn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	Crit  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	Dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)
