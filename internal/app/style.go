package app

import "github.com/charmbracelet/lipgloss"

// Shared lipgloss styles for the interactive CLI app. Mirrors the palette used
// by internal/install so the installer and the app look consistent.
var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")) // magenta
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // bright-black
	styleCursor  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))            // cyan
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))             // green
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // red
	styleFooter  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
