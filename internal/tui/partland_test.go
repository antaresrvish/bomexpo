package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/easyeda"
)

// The sidebar shows the land from the board and, under it, the part meant to sit on
// it — seeing 8 pads over 2 is what makes the fault obvious.
func TestSidebarShowsBothFootprints(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 42
	out := stripANSI(strings.Join(m.footprintPanes(48, 20), "\n"))
	if !strings.Contains(out, "Footprint R_0402_1005Metric") {
		t.Errorf("the board's land went missing:\n%s", out)
	}
	if !strings.Contains(out, "Part C2006") {
		t.Errorf("the assigned part's pads went missing:\n%s", out)
	}
	if !strings.Contains(out, "RES-ARRAY-SMD_0402-8P") {
		t.Errorf("the part's package name went missing:\n%s", out)
	}
	if !strings.Contains(out, "part has 8 pads, the land has 2") {
		t.Errorf("the part pane didn't say what was wrong:\n%s", out)
	}
	if land := strings.Index(out, "Footprint R_0402"); land > strings.Index(out, "Part C2006") {
		t.Error("the part came out above the land it sits on")
	}
}

// Two panes in too little room would be two unreadable boxes, so a short sidebar
// keeps the land alone.
func TestShortSidebarKeepsTheLandAlone(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 42
	out := stripANSI(strings.Join(m.footprintPanes(48, 2*minPaneH-1), "\n"))
	if !strings.Contains(out, "Footprint R_0402_1005Metric") {
		t.Errorf("the land went missing on a short sidebar:\n%s", out)
	}
	if strings.Contains(out, "Part C2006") {
		t.Errorf("squeezed two panes into a sidebar too short for them:\n%s", out)
	}
}

func TestPartPaneSaysWhatItKnows(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 42

	cases := []struct {
		name string
		set  func(*Model)
		want string
	}{
		{"nothing assigned", func(m *Model) { m.items[0].LCSC = "" }, "none assigned"},
		{"not fetched yet", func(m *Model) { delete(m.edaLands, "C2006") }, "fetching pads…"},
		{"vendor has none", func(m *Model) {
			m.edaLands["C2006"] = easyeda.Footprint{Code: "C2006"}
		}, "no pads published"},
		{"fits", func(m *Model) {
			m.edaLands["C2006"] = easyeda.Footprint{Code: "C2006", Lands: pads(2)}
		}, "2 pads"},
	}
	for _, c := range cases {
		mm := fitModel()
		mm.w, mm.h = m.w, m.h
		c.set(&mm)
		out := stripANSI(strings.Join(mm.partFootprintHeader(48), "\n"))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: header %q, want it to mention %q", c.name, out, c.want)
		}
	}
}

// Moving through the table asks for the highlighted part's pads, up to the limit —
// each answer freeing the next attempt.
func TestMovingTheCursorFetchesThePartPads(t *testing.T) {
	m := fitModel()
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	m.edaFetching = map[string]bool{}
	code := m.selCode()
	if m.askPadsCmd(code) == nil {
		t.Fatal("the highlighted part's pads were never asked for")
	}
	if m.askPadsCmd(code) != nil {
		t.Error("asked twice for one request")
	}
	// Update reissues once the answer comes back, until the limit
	mm, cmd := m.Update(footprintDoneMsg{code: code, err: errNoSource})
	if cmd == nil {
		t.Error("no retry after a failure")
	}
	mm, cmd = mm.(Model).Update(footprintDoneMsg{code: code, err: errNoSource})
	if cmd != nil {
		t.Errorf("kept asking after %d attempts", maxFitAttempts)
	}
	_ = mm
}
