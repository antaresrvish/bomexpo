package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLoadTyping(t *testing.T) {
	m := New(Options{})
	if !m.load.field.Focused() {
		t.Fatal("load field is not focused after New")
	}
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, r := range "~/proj" {
		m = step(m, tea.KeyPressMsg{Text: string(r), Code: r})
	}
	if got := m.load.field.Value(); got != "~/proj" {
		t.Fatalf("typing did not register, value=%q", got)
	}
	// select-all then type replaces everything
	m = step(m, tea.KeyPressMsg{Text: "", Code: 'a', Mod: tea.ModCtrl})
	m = step(m, tea.KeyPressMsg{Text: "x", Code: 'x'})
	if got := m.load.field.Value(); got != "x" {
		t.Fatalf("select-all replace failed, value=%q", got)
	}
}
