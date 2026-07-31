package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/easyeda"
)

// Arriving on a row asks for its pads; when they land the panel must show them
// without another keypress.
func TestPadsAppearWithoutAnotherKey(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 46
	m.mode = modeTable
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}

	mm, cmd := m.updateTable(tea.KeyPressMsg{Code: tea.KeyDown})
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("moving the cursor asked for nothing")
	}
	before := stripANSI(strings.Join(m.footprintPanes(48, 26), "\n"))
	if !strings.Contains(before, "fetching pads") {
		t.Fatalf("expected the waiting state first:\n%s", before)
	}

	// the fetch lands
	mm, _ = m.Update(footprintDoneMsg{
		code: m.items[m.sel()].LCSC,
		fp:   easyeda.Footprint{Code: m.items[m.sel()].LCSC, Package: "R0402", Lands: pads(2)},
	})
	m = mm.(Model)

	after := stripANSI(strings.Join(m.footprintPanes(48, 26), "\n"))
	if strings.Contains(after, "fetching pads") {
		t.Errorf("the panel still waits after the pads arrived:\n%s", after)
	}
	if !strings.Contains(after, "R0402") {
		t.Errorf("the pads arrived but the panel doesn't show them:\n%s", after)
	}
}

// Every way of arriving on a row must ask for its pads. Only the table's own key
// handler did, so landing here from Export or a tab switch showed "fetching pads…"
// with nothing actually fetching — it sat there until you pressed an arrow.
func TestEveryArrivalAsksForPads(t *testing.T) {
	arrive := map[string]func(Model) (tea.Model, tea.Cmd){
		"tab switch to Components": func(m Model) (tea.Model, tea.Cmd) { return m.gotoTab(modeTable) },
		"enter on a finding":       func(m Model) (tea.Model, tea.Cmd) { return m.jumpToComponent("R12") },
		"arrow in the table": func(m Model) (tea.Model, tea.Cmd) {
			return m.updateTable(tea.KeyPressMsg{Code: tea.KeyDown})
		},
	}
	for name, act := range arrive {
		m := fitModel()
		m.w, m.h = 132, 46
		m.mode = modeTable
		m.edaLands = map[string]easyeda.Footprint{}
		m.edaTried = map[string]int{}

		mm, cmd := act(m)
		m = mm.(Model)
		if cmd == nil {
			t.Errorf("%s: asked for nothing, so the panel would wait forever", name)
			continue
		}
		// and the panel must not claim a fetch that was never started
		out := stripANSI(strings.Join(m.footprintPanes(48, 26), "\n"))
		if strings.Contains(out, "fetching") && len(m.edaTried) == 0 {
			t.Errorf("%s: says fetching with nothing requested:\n%s", name, out)
		}
	}
}

// Giving up is not the same as still trying, and the panel must not say otherwise.
func TestPanelStopsSayingFetchingWhenItGaveUp(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 46
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{m.items[0].LCSC: maxFitAttempts}
	out := stripANSI(strings.Join(m.partFootprintHeader(48), "\n"))
	if strings.Contains(out, "fetching") {
		t.Errorf("still claims a fetch after giving up: %q", out)
	}
	if !strings.Contains(out, "could not reach") {
		t.Errorf("gave up without saying so: %q", out)
	}
}
