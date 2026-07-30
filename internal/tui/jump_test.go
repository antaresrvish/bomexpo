package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

// verifyAt is Verify with a report on screen and the cursor on the named row.
func verifyAt(t *testing.T, ref string) Model {
	t.Helper()
	m := filterModel(t)
	m.w, m.h = 140, 26
	m.pcbPath = "/tmp/x.kicad_pcb"
	mm, _ := m.gotoTab(modeCheck)
	mm, _ = mm.(Model).openVerify()
	m = mm.(Model)
	m.diff.ran = true
	m.diff.res = kicad.SchDiff{SchPath: "/tmp/x.kicad_sch", BOMPath: "/tmp/theirs.csv",
		Rows: []kicad.Row{
			{Designator: ref, Sch: kicad.Cell{Present: true, Value: "wrong"},
				Kinds: []kicad.DiffKind{kicad.DiffValue}, Sides: []kicad.Side{kicad.SideBOM}},
		}}
	m.diff.cursor = 0
	return m
}

// A report that only tells you is half a tool: enter on a finding opens that part in
// Components, with the cursor on it.
func TestVerifyEnterOpensTheComponent(t *testing.T) {
	m := verifyAt(t, "R1")
	mm, _ := m.updateDiffKey(key("enter"))
	m = mm.(Model)

	if m.mode != modeTable {
		t.Fatalf("enter left us on %v, want Components", m.mode)
	}
	if i := m.at(m.cursor); i < 0 || m.items[i].ID() != "R1" {
		t.Errorf("cursor is on %d, which is not R1", m.cursor)
	}
	if !strings.Contains(m.flash, "R1") {
		t.Errorf("flash = %q, want it to name where it went", m.flash)
	}
}

// A designator hidden behind a filter is reached by clearing it, and the clearing is
// announced — landing somewhere else silently would be worse than not moving.
func TestVerifyEnterClearsAFilterInTheWay(t *testing.T) {
	m := verifyAt(t, "R1")
	m.filter.field.SetValue("ref:C1")
	m.filter.f = parseFilter("ref:C1")
	m = m.reindex()
	if m.rowOf(m.itemIndexOf("R1")) >= 0 {
		t.Fatal("the filter was supposed to hide R1")
	}

	mm, _ := m.updateDiffKey(key("enter"))
	m = mm.(Model)
	if m.filter.f.active() {
		t.Error("the filter still hides the row we jumped to")
	}
	if i := m.at(m.cursor); i < 0 || m.items[i].ID() != "R1" {
		t.Errorf("cursor did not land on R1 after clearing the filter")
	}
	if !strings.Contains(m.flash, "cleared the filter") {
		t.Errorf("flash = %q, want it to say the filter went", m.flash)
	}
}

// A BOM can name parts the board never had. Enter on one of those says so rather
// than moving the cursor somewhere arbitrary.
func TestVerifyEnterOnAPartTheDesignLacksSaysSo(t *testing.T) {
	m := verifyAt(t, "H99")
	mm, _ := m.updateDiffKey(key("enter"))
	m = mm.(Model)

	if m.mode != modeDiff {
		t.Errorf("it moved to %v for a part that isn't in the design", m.mode)
	}
	if !strings.Contains(m.flash, "not in this design") {
		t.Errorf("flash = %q, want it to explain", m.flash)
	}
}

// A grouped line item is reachable by any of its designators, not just the first.
func TestJumpFindsADesignatorInsideAGroup(t *testing.T) {
	m := filterModel(t)
	// group C1 and C2 onto one line item, as GroupItems would
	m.items[0].Designators = []string{"C1", "C2"}
	m.items[0].Bases = []string{"C1"}
	m = m.reindex()

	for _, ref := range []string{"C1", "C2", "c2"} {
		if got := m.itemIndexOf(ref); got != 0 {
			t.Errorf("itemIndexOf(%q) = %d, want the group at 0", ref, got)
		}
	}
	if got := m.itemIndexOf("Z9"); got != -1 {
		t.Errorf("itemIndexOf(Z9) = %d, want -1", got)
	}
}

// r re-runs the comparison; that job moved off enter when enter took on the jump.
func TestVerifyRRerunsTheComparison(t *testing.T) {
	m := verifyAt(t, "R1")
	m.diff.field.SetValue("/tmp/theirs.csv")
	_, cmd := m.updateDiffKey(key("r"))
	if cmd == nil {
		t.Error("r should start the comparison again")
	}
}
