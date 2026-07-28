package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

func hotStyle(n int, hot lipgloss.Style) lipgloss.Style {
	if n == 0 {
		return dimStyle
	}
	return hot
}

func boardSize(w, h float64) string {
	if w <= 0 || h <= 0 {
		return dimStyle.Render("—")
	}
	return fmt.Sprintf("%.1f × %.1f mm", w, h)
}

func dash(ok bool, s string) string {
	if !ok {
		return dimStyle.Render("—")
	}
	return s
}

// sideTile is a compact boxed stat for the components side panel.
func sideTile(label, value string, vs lipgloss.Style, width int) string {
	inner := lipgloss.JoinVertical(lipgloss.Center, dimStyle.Render(label), vs.Bold(true).Render(value))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cPanel).
		Width(width).
		Align(lipgloss.Center).
		Render(inner)
}
