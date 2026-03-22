package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	ActiveChannel = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	DisabledChannel = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	// Messages
	IMAPReceived = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	IMAPSent     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	TGReceived   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	TGSent       = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	EmailMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // cyan
	AgentMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	ToolCall     = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))  // blue
	DiaryMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // dim

	// Title
	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")).
		Padding(0, 1)

	// Input
	InputPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))

	// Borders
	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
)
