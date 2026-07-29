package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// filterModel is a small board: two caps on GND/+3V3, a resistor on a signal,
// an unassigned inductor, an excluded test point and a DNP part.
func filterModel(t *testing.T) Model {
	t.Helper()
	m := New(Options{})
	m.w, m.h = 140, 40
	m.items = []kicad.Item{
		{Bases: []string{"C1"}, Designators: []string{"C1"}, Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 1, LCSC: "C1525", Nets: []string{"+3V3", "GND"}},
		{Bases: []string{"C2"}, Designators: []string{"C2"}, Value: "10uF", Footprint: "C_0805_2012Metric", Quantity: 1, LCSC: "C1713", Nets: []string{"+5V", "GND"}},
		{Bases: []string{"R1"}, Designators: []string{"R1"}, Value: "10k", Footprint: "R_0402_1005Metric", Quantity: 1, LCSC: "C25744", Nets: []string{"SPI_SCK", "GND"}},
		{Bases: []string{"L1"}, Designators: []string{"L1"}, Value: "4.7uH", Footprint: "L_0805", Quantity: 1, Nets: []string{"+5V", "VBUS"}},
		{Bases: []string{"TP1"}, Designators: []string{"TP1"}, Value: "TestPoint", Footprint: "TestPoint_Pad", Quantity: 1},
		{Bases: []string{"D1"}, Designators: []string{"D1"}, Value: "LED", Footprint: "LED_0603", Quantity: 1, LCSC: "C2286", DNP: true, Nets: []string{"GND"}},
	}
	m.assigned = []*part.Part{
		{Code: "C1525", Desc: "100nF 16V X7R", Stock: 1000, Lib: part.LibBasic},
		{Code: "C1713", Desc: "10uF 25V X5R", Stock: 500, Lib: part.LibExtended},
		{Code: "C25744", Desc: "10kΩ ±1%", Stock: 0, Lib: part.LibBasic}, // out of stock
		nil,
		nil,
		{Code: "C2286", Desc: "LED red", Stock: 900, Lib: part.LibExtended},
	}
	m.excluded = []bool{false, false, false, false, true, true} // TP1 by hand, D1 is DNP
	return m.reindex()
}

// shown lists the visible references, which is what a filter is judged on.
func shown(m Model) string {
	var out []string
	for row := 0; row < m.rows(); row++ {
		out = append(out, m.items[m.at(row)].ID())
	}
	return strings.Join(out, ",")
}

func applyFilter(t *testing.T, m Model, q string) Model {
	t.Helper()
	m.filter.f = parseFilter(q)
	return m.reindex()
}

func TestFilterKeys(t *testing.T) {
	m := filterModel(t)

	for _, c := range []struct{ query, want string }{
		{"", "C1,C2,R1,L1,TP1,D1"},
		{"net:GND", "C1,C2,R1,D1"},
		{"net:gnd", "C1,C2,R1,D1"}, // case-insensitive
		{"net:3v3", "C1"},          // substring, so +3V3 matches
		{"net:+5V", "C2,L1"},
		{"ref:C", "C1,C2"},
		{"ref:TP1", "TP1"},
		// substring, so "10" also finds 100nF — narrow it if that's not wanted
		{"val:10", "C1,C2,R1"},
		{"val:10u", "C2"},
		{"val:100nF", "C1"},
		{"fp:0402", "C1,R1"},
		{"lcsc:C25744", "R1"},
		{"lib:basic", "C1,R1"},
		{"lib:extended", "C2,D1"},
		{"lib:none", "L1,TP1"}, // nothing assigned, so no library data
		{"st:unassigned", "L1"},
		{"st:oos", "R1"},
		{"st:excluded", "TP1,D1"},
		{"st:dnp", "D1"},
		{"st:ok", "C1,C2"},
		{"0402", "C1,R1"}, // bare text hits the footprint
		{"10k", "R1"},     // and the value
	} {
		if got := shown(applyFilter(t, m, c.query)); got != c.want {
			t.Errorf("%-18q → %s, want %s", c.query, got, c.want)
		}
	}
}

func TestFilterCombinesAndNegates(t *testing.T) {
	m := filterModel(t)

	for _, c := range []struct{ query, want string }{
		// terms are ANDed
		{"net:GND fp:0402", "C1,R1"},
		{"net:GND lib:basic", "C1,R1"},
		// a minus inverts one term
		{"net:GND -st:dnp", "C1,C2,R1"},
		{"-st:excluded", "C1,C2,R1,L1"},
		{"net:GND -fp:0402", "C2,D1"},
		// no match at all
		{"net:NOPE", ""},
		{"fp:0402 fp:0805", ""},
	} {
		if got := shown(applyFilter(t, m, c.query)); got != c.want {
			t.Errorf("%-22q → %q, want %q", c.query, got, c.want)
		}
	}
}

func TestFilterReportsUnknownKeys(t *testing.T) {
	f := parseFilter("nte:GND val:100nF")
	if len(f.unknown) != 1 || f.unknown[0] != "nte" {
		t.Errorf("unknown = %v, want [nte]", f.unknown)
	}
	// the bad term is dropped, the good one still applies
	if len(f.terms) != 1 || f.terms[0].key != "val" {
		t.Errorf("terms = %+v, want just the val term", f.terms)
	}

	// something that isn't key-shaped stays a plain text search
	f = parseFilter("Capacitor_SMD:C_0402")
	if len(f.unknown) != 0 {
		t.Errorf("unknown = %v, want none for a non-key token", f.unknown)
	}
	if len(f.terms) != 1 || f.terms[0].key != "" {
		t.Errorf("terms = %+v, want one bare term", f.terms)
	}
}

// A value query must not match a longer number that merely contains it: 1k is
// not 5.1k, and asking for one and getting the other is worse than useless.
func TestFilterValueMatchesTokenStartsOnly(t *testing.T) {
	m := filterModel(t)
	m.items = []kicad.Item{
		{Bases: []string{"R1"}, Designators: []string{"R1"}, Value: "1k", Footprint: "R_0402_1005Metric", Quantity: 1},
		{Bases: []string{"R2"}, Designators: []string{"R2"}, Value: "5.1k", Footprint: "R_0402_1005Metric", Quantity: 1},
		{Bases: []string{"R3"}, Designators: []string{"R3"}, Value: "1kΩ ±1%", Footprint: "R_0603_1608Metric", Quantity: 1},
		{Bases: []string{"R4"}, Designators: []string{"R4"}, Value: "21k", Footprint: "R_0402_1005Metric", Quantity: 1},
		{Bases: []string{"R5"}, Designators: []string{"R5"}, Value: "10k", Footprint: "R_0402_1005Metric", Quantity: 1},
	}
	m.assigned = make([]*part.Part, len(m.items))
	m.excluded = make([]bool, len(m.items))
	m = m.reindex()

	for _, c := range []struct{ query, want string }{
		{"val:1k", "R1,R3"}, // not 5.1k, not 21k
		{"val:5.1k", "R2"},
		{"val:10k", "R5"},
		{"val:1", "R1,R3,R5"}, // a prefix still narrows as you type
		{"1k", "R1,R3"},       // bare text follows the same rule
	} {
		if got := shown(applyFilter(t, m, c.query)); got != c.want {
			t.Errorf("%-12q → %s, want %s", c.query, got, c.want)
		}
	}

	// footprints stay substring, since their names are compound
	if got, want := shown(applyFilter(t, m, "fp:0402")), "R1,R2,R4,R5"; got != want {
		t.Errorf("fp:0402 → %s, want %s", got, want)
	}
}

// The dropdown has to narrow the same way, or it offers values the query then
// throws away.
func TestSuggestNarrowsLikeTheFilter(t *testing.T) {
	m := filterModel(t)
	m.items = []kicad.Item{
		{Bases: []string{"R1"}, Designators: []string{"R1"}, Value: "1k", Quantity: 1},
		{Bases: []string{"R2"}, Designators: []string{"R2"}, Value: "5.1k", Quantity: 1},
	}
	m.assigned = make([]*part.Part, 2)
	m.excluded = make([]bool, 2)
	mm, _ := m.reindex().openFilter()
	m = mm.(Model)
	m.filter.field.SetValue("val:1k")

	if got := offered(m); got != "1k=1" {
		t.Errorf("val:1k offered %q, want just 1k", got)
	}
}

func TestFilterParseEdgeCases(t *testing.T) {
	if parseFilter("   ").active() {
		t.Error("whitespace should not count as a filter")
	}
	if parseFilter("net:").active() {
		t.Error("a key with no value should not count as a term")
	}
	if parseFilter("-").active() {
		t.Error("a lone minus should not count as a term")
	}
	f := parseFilter("  net:GND   fp:0402  ")
	if len(f.terms) != 2 {
		t.Errorf("terms = %+v, want 2 from padded input", f.terms)
	}
}

func TestFilterKeysDriveTheBar(t *testing.T) {
	m := filterModel(t)

	// / opens the bar and it takes a row
	mm, _ := m.updateTable(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = mm.(Model)
	if !m.filter.open || !m.filterBarVisible() {
		t.Fatal("/ should open the filter bar")
	}
	if m.dataTop() != dataTop+1 {
		t.Errorf("dataTop = %d, want it pushed down by the bar", m.dataTop())
	}

	// typing filters live, and letters do not fire table commands
	for _, r := range "net:GND" {
		mm, _ = m.updateTable(tea.KeyPressMsg{Text: string(r), Code: r})
		m = mm.(Model)
	}
	if got, want := shown(m), "C1,C2,R1,D1"; got != want {
		t.Errorf("live filter → %s, want %s", got, want)
	}
	if m.filter.field.Value() != "net:GND" {
		t.Errorf("field = %q, want net:GND", m.filter.field.Value())
	}

	// enter keeps the filter but hands the keyboard back
	mm, _ = m.updateTable(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mm.(Model)
	if m.filter.open {
		t.Error("enter should close the bar")
	}
	if !m.filter.f.active() || shown(m) != "C1,C2,R1,D1" {
		t.Error("enter should keep the filter in force")
	}
	if !m.filterBarVisible() {
		t.Error("an active filter must stay visible after the bar closes")
	}

	// esc clears it
	mm, _ = m.updateTable(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = mm.(Model)
	if m.filter.f.active() || m.filterBarVisible() {
		t.Error("esc should clear the filter")
	}
	if got, want := shown(m), "C1,C2,R1,L1,TP1,D1"; got != want {
		t.Errorf("after clearing → %s, want %s", got, want)
	}
}

// The cursor must keep pointing at a real line item as rows disappear.
func TestFilterKeepsTheCursorValid(t *testing.T) {
	m := filterModel(t)
	m.cursor = 5 // D1, the last row
	m.clampScroll()
	if m.sel() != 5 {
		t.Fatalf("sel = %d, want 5", m.sel())
	}

	m = applyFilter(t, m, "net:3v3") // only C1 survives
	if m.rows() != 1 {
		t.Fatalf("rows = %d, want 1", m.rows())
	}
	if m.cursor != 0 || m.sel() != 0 {
		t.Errorf("cursor/sel = %d/%d, want 0/0", m.cursor, m.sel())
	}

	// filtering to nothing must not leave a dangling selection
	m = applyFilter(t, m, "net:NOPE")
	if m.rows() != 0 {
		t.Fatalf("rows = %d, want 0", m.rows())
	}
	if m.sel() != -1 {
		t.Errorf("sel = %d, want -1 when nothing is shown", m.sel())
	}
	// and the actions that read the selection must cope
	if _, cmd := m.openSearch(m.cursor); cmd != nil {
		t.Error("openSearch should do nothing with no selection")
	}
	mm, _ := m.cycleRotOverride()
	if mm.(Model).flash != "" {
		t.Error("rotation override should do nothing with no selection")
	}
}

func TestFilterBarShowsCountsAndProblems(t *testing.T) {
	m := applyFilter(t, filterModel(t), "net:GND")
	if got := stripANSI(m.filterBar(120)); !strings.Contains(got, "4 of 6") {
		t.Errorf("bar = %q, want it to say 4 of 6", got)
	}

	m = applyFilter(t, filterModel(t), "net:NOPE")
	if got := stripANSI(m.filterBar(120)); !strings.Contains(got, "nothing matches") {
		t.Errorf("bar = %q, want it to say nothing matches", got)
	}

	m = applyFilter(t, filterModel(t), "nte:GND")
	if got := stripANSI(m.filterBar(120)); !strings.Contains(got, "unknown: nte") {
		t.Errorf("bar = %q, want it to flag the unknown key", got)
	}
}

func TestMatchedRefsFollowTheFilter(t *testing.T) {
	m := filterModel(t)
	if m.matchedRefs() != nil {
		t.Error("with no filter nothing should be dimmed, so matchedRefs is nil")
	}

	m = applyFilter(t, m, "net:3v3")
	refs := m.matchedRefs()
	if len(refs) != 1 || !refs["C1"] {
		t.Errorf("matchedRefs = %v, want just C1", refs)
	}
}

// A filtered table must still render the right number of lines, or the panel
// scrolls or clips.
func TestFilteredTableGeometry(t *testing.T) {
	m := applyFilter(t, filterModel(t), "net:GND")
	h := m.contentH()
	block := m.tableBlock(layoutCols(m.tableW()), m.tableW(), h)
	if len(block) != h {
		t.Fatalf("table block has %d lines, want %d", len(block), h)
	}
	if got := stripANSI(block[0]); !strings.Contains(got, "4 of 6") {
		t.Errorf("first line should be the filter bar, got %q", got)
	}
	if got := stripANSI(block[1]); !strings.Contains(got, "REF") {
		t.Errorf("second line should be the column header, got %q", got)
	}
	if len(m.vScrollCol(h)) != h {
		t.Error("the scrollbar column must match the block height")
	}
}
