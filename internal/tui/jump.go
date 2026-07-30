package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// A report that only tells you is half a tool. Verify names a designator; this is
// how you get to it in Components and fix it.

// itemIndexOf is the line item carrying a designator, or -1. Line items are
// grouped, so a search for C4 lands on the group that holds it.
func (m Model) itemIndexOf(ref string) int {
	want := strings.ToUpper(strings.TrimSpace(ref))
	if want == "" {
		return -1
	}
	for i, it := range m.items {
		for _, d := range it.Designators {
			if strings.EqualFold(strings.TrimSpace(d), want) {
				return i
			}
		}
	}
	return -1
}

// jumpToComponent opens Components with the cursor on a designator. A filter
// hiding that row is cleared rather than silently landing somewhere else, and a
// designator this design doesn't have says so instead of moving the cursor at
// random — a BOM can name parts the board never had.
func (m Model) jumpToComponent(ref string) (tea.Model, tea.Cmd) {
	i := m.itemIndexOf(ref)
	if i < 0 {
		m.flash = ref + " is not in this design — nothing to open"
		return m, nil
	}

	if m.rowOf(i) < 0 && m.filter.f.active() {
		mm, _ := m.closeFilter(true)
		m = mm.(Model)
		m.flash = "cleared the filter to reach " + ref
	}
	row := m.rowOf(i)
	if row < 0 {
		m.flash = ref + " is in the design but not on screen"
		return m, nil
	}

	mm, cmd := m.gotoTab(modeTable)
	m = mm.(Model)
	m.cursor = row
	m.clampScroll()
	if m.flash == "" {
		m.flash = "→ " + m.items[i].ID()
	}
	return m, cmd
}
