package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/export"
)

func (m Model) updateOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab":
		return m.cycleTab(1)
	case "shift+tab":
		return m.cycleTab(-1)
	case "1":
		return m.gotoTab(modeLoad)
	case "3":
		return m.gotoTab(modeTable)
	case "4":
		return m.gotoTab(modeBoard)
	case "5":
		return m.gotoTab(modeCheck)
	case "a":
		return m.startAutoAssign()
	case "w":
		return m, m.saveCmd()
	case "enter", " ":
		return m.gotoTab(m.recommendedTab())
	}
	return m, nil
}

func (m Model) mouseOverview(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	if click && ms.Button == tea.MouseLeft && ms.Y >= 2 {
		return m.gotoTab(m.recommendedTab())
	}
	return m, nil
}

func (m Model) recommendedTab() mode {
	for i := range m.items {
		switch m.stateOf(i) {
		case stUnassigned, stOutOfStock:
			return modeTable
		}
	}
	return modeCheck
}

func (m Model) viewOverview(w, h int) string {
	var unassigned, oos, mismatch int
	for i := range m.items {
		switch m.stateOf(i) {
		case stUnassigned:
			unassigned++
		case stOutOfStock:
			oos++
		case stMismatch:
			mismatch++
		}
	}
	assigned, _ := m.counts()
	active := m.activeCount()
	excluded := m.excludedCount()

	tiles := []string{
		statTile("assigned", fmt.Sprintf("%d/%d", assigned, active), okStyle),
		statTile("unassigned", fmt.Sprintf("%d", unassigned), hotStyle(unassigned, warnStyle)),
		statTile("no stock", fmt.Sprintf("%d", oos), hotStyle(oos, badStyle)),
		statTile("mismatch", fmt.Sprintf("%d", mismatch), hotStyle(mismatch, warnStyle)),
		statTile("excluded", fmt.Sprintf("%d", excluded), hotStyle(excluded, subtleStyle)),
	}
	var cells []string
	for i, t := range tiles {
		if i > 0 {
			cells = append(cells, " ")
		}
		cells = append(cells, t)
	}
	tileRow := lipgloss.JoinHorizontal(lipgloss.Top, cells...)

	frac := 0.0
	if active > 0 {
		frac = float64(assigned) / float64(active)
	}
	ready := subtleStyle.Render(fmt.Sprintf("%d%%   %d/%d assigned", int(frac*100+0.5), assigned, active))
	readiness := dimStyle.Render(pad("readiness", 11)) + progressBar(frac, 26) + "  " + ready

	board := lipgloss.NewStyle().Width(34).Render(lipgloss.JoinVertical(lipgloss.Left,
		accentStyle.Render("BOARD"),
		kv("size", boardSize(m.boardW, m.boardH)),
		kv("layers", dash(m.layers > 0, fmt.Sprintf("%d", m.layers))),
	))
	cost, complete := m.costAt(1)
	costStr := fmt.Sprintf("$%.2f", cost)
	if !complete {
		costStr += dimStyle.Render(" *")
	}
	nRot := len(export.RotationFixes(m.placements, m.excludeSet()))
	order := lipgloss.JoinVertical(lipgloss.Left,
		accentStyle.Render("ORDER"),
		kv("line items", fmt.Sprintf("%d", len(m.items))),
		kv("components", fmt.Sprintf("%d", len(m.placements))),
		kv("est. cost", costStr+dimStyle.Render("  qty 1")),
		kv("rotation", dash(nRot > 0, fmt.Sprintf("%d corrected", nRot))),
	)
	cols := lipgloss.JoinHorizontal(lipgloss.Top, board, order)

	var lines []string
	head := accentStyle.Render(m.name)
	if m.pcbPath != "" {
		head += "   " + dimStyle.Render(m.pcbPath)
	}
	lines = append(lines, head, "")
	lines = append(lines, strings.Split(tileRow, "\n")...)
	lines = append(lines, "", readiness, "")
	lines = append(lines, strings.Split(cols, "\n")...)
	lines = append(lines, "", m.actionBar(unassigned, oos, mismatch, active))
	return strings.Join(lines, "\n")
}

func (m Model) actionBar(unassigned, oos, mismatch, active int) string {
	var action, hint string
	icon := accentStyle.Render("▸")
	switch {
	case active == 0:
		icon, action = badStyle.Render("✗"), "no active line items to order"
	case unassigned > 0:
		action = fmt.Sprintf("%d line items still need an LCSC part", unassigned)
		hint = "enter → assign · a → auto-assign"
	case oos > 0:
		action = fmt.Sprintf("%d assigned parts are out of stock", oos)
		hint = "enter → swap in Components · a → auto-assign"
	case mismatch > 0:
		action = fmt.Sprintf("%d values differ from the schematic", mismatch)
		hint = "enter → review in Check"
	default:
		icon, action = okStyle.Render("✓"), "everything assigned, in stock and value-matched"
		hint = "enter → export the order"
	}
	bar := icon + " " + lipgloss.NewStyle().Foreground(cFg).Render(action)
	if hint != "" {
		bar += "    " + dimStyle.Render(hint)
	}
	return bar
}

func statTile(label, value string, vs lipgloss.Style) string {
	inner := lipgloss.JoinVertical(lipgloss.Center, dimStyle.Render(label), vs.Bold(true).Render(value))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cPanel).
		Padding(0, 1).
		Width(16).
		Align(lipgloss.Center).
		Render(inner)
}

func hotStyle(n int, hot lipgloss.Style) lipgloss.Style {
	if n == 0 {
		return dimStyle
	}
	return hot
}

func progressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fill := int(frac*float64(width) + 0.5)
	return okStyle.Render(strings.Repeat("█", fill)) + dimStyle.Render(strings.Repeat("░", width-fill))
}

func kv(k, v string) string {
	return dimStyle.Render(pad(k, 12)) + lipgloss.NewStyle().Foreground(cFg).Render(v)
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
