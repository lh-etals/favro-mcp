package app

import "github.com/charmbracelet/lipgloss"

// Shared lipgloss styles for the interactive CLI app. The palette is the one
// the installer uses - the same one sana-mcp and interactive-terminal-mcp
// share - so the app and the setup flow look like one program.
var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleCursor  = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleFooter  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)
