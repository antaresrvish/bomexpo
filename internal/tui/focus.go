package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Focus in this app follows one rule: a text field owns the keyboard while it
// has focus, and tab is how you take it back.
//
// Arrow keys drive the list either way — focused or not — so moving through
// results never depends on where focus is. What changes is the letters: with the
// list focused they become commands (p pin, d datasheet, …) instead of text.
//
// Because tab is the focus key, switching tabs moves to [ and ] and the digits.
// Those are printable, so they only act while a list has focus; from a field you
// tab out first.

// focusMark prefixes a pane's header, so which one has the keyboard is never a
// guess.
func focusMark(focused bool) string {
	if focused {
		return accentStyle.Render("▸ ")
	}
	return "  "
}

// tabSwitchKey handles the keys that change tab. It reports false when the key
// wasn't one, so callers can carry on.
//
// Only call it when a list has focus: [ ] and the digits are text everywhere
// else.
func (m Model) tabSwitchKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "]":
		mm, cmd := m.cycleTab(1)
		return mm, cmd, true
	case "[":
		mm, cmd := m.cycleTab(-1)
		return mm, cmd, true
	case "1", "2", "3", "4", "5":
		if md, ok := m.tabMode(int(key[0] - '0')); ok {
			mm, cmd := m.gotoTab(md)
			return mm, cmd, true
		}
		return m, nil, true
	}
	return m, nil, false
}

// tabHint is the help-line entry for switching tabs, worded for whether the
// keys are live right now.
func tabHint(listFocused bool) [2]string {
	if listFocused {
		return [2]string{"[ ]", "tabs"}
	}
	return [2]string{"tab", "to the list"}
}
