package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
)

func RenderStatusBar(primaryState string, agentCount int, vizURL string, width int) string {
	stateStyle := lipgloss.NewStyle().Foreground(StateColor(primaryState)).Bold(true)
	left := fmt.Sprintf("  本我: %s  │  %d agents  │  %s", stateStyle.Render(primaryState), agentCount, vizURL)
	return StyleStatusBar.Width(width).Render(left)
}
