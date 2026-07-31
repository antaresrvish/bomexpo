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

	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
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
		"tab switch to Components": func(m Model) (tea.Model, tea.Cmd) {
			return m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
		},
		"enter on an export issue": func(m Model) (tea.Model, tea.Cmd) {
			mm, _ := m.gotoTab(modeCheck)
			mc := mm.(Model)
			mc.check.setPane(paneIssues)
			mc.edaLands, mc.edaTried, mc.edaFetching = nil, map[string]int{}, map[string]bool{}
			return mc.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		},
		"arrow in the table": func(m Model) (tea.Model, tea.Cmd) {
			return m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		},
		"click on a row": func(m Model) (tea.Model, tea.Cmd) {
			return m.Update(tea.MouseClickMsg{X: 10, Y: m.dataTop() + 1, Button: tea.MouseLeft})
		},
		"sorting a column": func(m Model) (tea.Model, tea.Cmd) {
			return m.Update(tea.MouseClickMsg{X: 10, Y: m.dataTop() - 2, Button: tea.MouseLeft})
		},
		"a message that is not a key at all": func(m Model) (tea.Model, tea.Cmd) {
			return m.Update(tea.WindowSizeMsg{Width: 132, Height: 46})
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

// A request already in flight must not spend a second attempt: two unrelated
// messages arriving while one is out would exhaust the budget on a part that was
// about to answer.
func TestInFlightRequestIsNotAskedTwice(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 46
	m.mode = modeTable
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	m.edaFetching = map[string]bool{}

	code := m.selCode()
	if m.askPadsCmd(code) == nil {
		t.Fatal("the first ask was refused")
	}
	if c := m.askPadsCmd(code); c != nil {
		t.Error("asked again while a request was still out")
	}
	if m.edaTried[code] != 1 {
		t.Errorf("spent %d attempts on one request", m.edaTried[code])
	}

	// an answer frees it, and Update reissues on the spot rather than waiting for a key
	mm, cmd := m.Update(footprintDoneMsg{code: code, err: errNoSource})
	m = mm.(Model)
	if cmd == nil {
		t.Error("a failed request was not retried")
	}
	if m.edaTried[code] != 2 {
		t.Errorf("attempts = %d, want the retry counted", m.edaTried[code])
	}
	// and that is the last one
	mm, cmd = m.Update(footprintDoneMsg{code: code, err: errNoSource})
	if cmd != nil {
		t.Errorf("kept asking past %d attempts", maxFitAttempts)
	}
	if mm.(Model).edaFetching[code] {
		t.Error("left marked in flight with nothing out")
	}
}
