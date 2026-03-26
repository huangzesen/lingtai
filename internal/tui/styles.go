package tui

import "github.com/charmbracelet/lipgloss"

var (
	ColorActive    = lipgloss.Color("#48bb78")
	ColorIdle      = lipgloss.Color("#a0aec0")
	ColorStuck     = lipgloss.Color("#ed8936")
	ColorAsleep    = lipgloss.Color("#ecc94b")
	ColorSuspended = lipgloss.Color("#e53e3e")
	ColorMail      = lipgloss.Color("#63b3ed")
	ColorHuman     = lipgloss.Color("#a0aec0")
	ColorSubtle    = lipgloss.Color("#4a5568")

	StyleTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	StyleSubtle    = lipgloss.NewStyle().Foreground(ColorSubtle)
	StyleStatusBar = lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Padding(0, 1)
)

func StateColor(state string) lipgloss.Color {
	switch state {
	case "ACTIVE":
		return ColorActive
	case "IDLE":
		return ColorIdle
	case "STUCK":
		return ColorStuck
	case "ASLEEP":
		return ColorAsleep
	case "SUSPENDED":
		return ColorSuspended
	default:
		return ColorSubtle
	}
}
