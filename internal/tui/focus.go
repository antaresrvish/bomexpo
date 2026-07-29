package tui

import (
	tea "charm.land/bubbletea/v2"
)

// A text field owns the keyboard while it has focus; tab takes it back. Arrows
// always drive the list, and with the list focused the letters are commands.
func focusMark(focused bool) string {
	if focused {
		return accentStyle.Render("▸ ")
	}
	return "  "
}

// tabSwitchKey reports false when the key wasn't a tab switch. Only call it with
// a list focused: [ ] and the digits are text in a field.
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

func tabHint(listFocused bool) [2]string {
	if listFocused {
		return [2]string{"[ ]", "tabs"}
	}
	return [2]string{"tab", "to the list"}
}
