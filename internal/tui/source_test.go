package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// searchModel is a model parked in the search view with results already in
// hand, so the view can be rendered without touching the network.
func searchModel(t *testing.T, results []part.Part) Model {
	t.Helper()
	m := New("", "")
	m.w, m.h = 140, 40
	m.items = []kicad.Item{{Bases: []string{"C1"}, Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 1}}
	m.assigned = make([]*part.Part, 1)
	m.excluded = make([]bool, 1)
	m.mode = modeSearch
	m.search.results = results
	m.search.total = len(results)
	return m
}

func TestSearchColsGiveLibraryOnlyToCapableSource(t *testing.T) {
	m := searchModel(t, nil)

	if got := m.resultCols(136); got.lib != 0 {
		t.Errorf("lcsc should get no library column, got width %d", got.lib)
	}
	lcscDesc := m.resultCols(136).desc

	m = m.nextSrc() // → jlcpcb
	jl := m.resultCols(136)
	if jl.lib != scLib {
		t.Fatalf("jlcpcb should get a library column, got width %d", jl.lib)
	}
	// the column is paid for out of the description, not the fixed columns
	if jl.desc != lcscDesc-scLib-3 {
		t.Errorf("desc = %d, want %d (lcsc %d minus the library column and its separator)",
			jl.desc, lcscDesc-scLib-3, lcscDesc)
	}
}

// TestSearchDatasheetSpan is the geometry guard: the clickable datasheet span
// must line up with where the column actually renders, in both layouts.
func TestSearchDatasheetSpan(t *testing.T) {
	results := []part.Part{{
		Code: "C1525", MPN: "CL05B104KO5NNNC", Package: "0402", Stock: 1000,
		Datasheet: "https://x/y.pdf", Desc: "100nF X7R", Lib: part.LibBasic,
		Prices: []part.Price{{Ladder: 1, USD: 0.005}},
	}}

	for _, srcName := range []string{"lcsc", "jlcpcb"} {
		m := searchModel(t, results)
		if srcName == "jlcpcb" {
			m = m.nextSrc()
		}
		if m.srcID() != srcName {
			t.Fatalf("expected to be on %s, got %s", srcName, m.srcID())
		}

		w := m.contentW()
		lo, _ := m.resultCols(w).dsRange()

		var row string
		for _, ln := range strings.Split(m.viewSearch(w, m.contentH()), "\n") {
			if strings.Contains(stripANSI(ln), "datasheet") {
				row = stripANSI(ln)
				break
			}
		}
		if row == "" {
			t.Fatalf("%s: no rendered row contained the datasheet cell", srcName)
		}
		// the row string starts at content x=2, so add that back
		at := 2 + lipgloss.Width(row[:strings.Index(row, "datasheet")])
		if at != lo {
			t.Errorf("%s: datasheet renders at x=%d but dsRange says %d", srcName, at, lo)
		}
	}
}

func TestSwitchSourceCyclesAndClearsResults(t *testing.T) {
	m := searchModel(t, []part.Part{{Code: "C1", Desc: "100nF"}})
	m.search.field.SetValue("100nF")
	m.search.basicOnly = false

	start := m.srcID()
	mm, cmd := m.switchSource()
	m = mm.(Model)
	if m.srcID() == start {
		t.Fatalf("source did not change from %s", start)
	}
	if m.search.results != nil || m.search.total != 0 {
		t.Error("stale results survived the source switch")
	}
	if !m.search.loading {
		t.Error("switching source should kick off a fresh search")
	}
	if cmd == nil {
		t.Error("switching source with a keyword should return a search command")
	}
	if !strings.Contains(m.flash, m.srcLabel()) {
		t.Errorf("flash %q should name the new source %q", m.flash, m.srcLabel())
	}

	// cycling all the way round comes home
	for m.srcID() != start {
		mm, _ = m.switchSource()
		m = mm.(Model)
	}
}

func TestSwitchSourceDropsUnsupportedFilter(t *testing.T) {
	m := searchModel(t, nil)
	m = m.nextSrc() // jlcpcb, which supports basic-only
	m.search.basicOnly = true

	mm, _ := m.switchSource() // back to lcsc, which doesn't
	m = mm.(Model)
	if m.srcID() != "lcsc" {
		t.Fatalf("expected lcsc, got %s", m.srcID())
	}
	if m.search.basicOnly {
		t.Error("basic-only should be dropped on a source that can't honour it")
	}
}

func TestBasicOnlyKeyRefusedOnIncapableSource(t *testing.T) {
	m := searchModel(t, nil)
	if m.srcID() != "lcsc" {
		t.Fatalf("expected to start on lcsc, got %s", m.srcID())
	}
	mm, _ := m.updateSearchKey(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	m = mm.(Model)
	if m.search.basicOnly {
		t.Error("ctrl+b should not set basic-only on lcsc")
	}
	if m.flash == "" {
		t.Error("ctrl+b on an incapable source should explain itself")
	}
}

func TestLibraryBreakdownIgnoresExcluded(t *testing.T) {
	m := Model{
		items: []kicad.Item{
			{Bases: []string{"C1"}}, {Bases: []string{"C2"}},
			{Bases: []string{"C3"}}, {Bases: []string{"R1"}},
		},
		assigned: []*part.Part{
			{Lib: part.LibBasic}, {Lib: part.LibExtended},
			{Lib: part.LibExtended}, nil, // nil: nothing assigned yet
		},
		excluded: []bool{false, false, true, false},
	}
	b, pref, ext, known := m.libBreakdown()
	if b != 1 || pref != 0 || ext != 1 || known != 2 {
		t.Errorf("breakdown = basic %d preferred %d extended %d known %d; want 1/0/1/2", b, pref, ext, known)
	}
	if got := m.extCount(); got != 1 {
		t.Errorf("extCount = %d, want 1 (the excluded extended part doesn't count)", got)
	}
}

func TestLibTextAndCellHandleUnknown(t *testing.T) {
	if got := libText(part.LibUnknown); got != "—" {
		t.Errorf("libText(unknown) = %q, want —", got)
	}
	if got := libText(part.LibExtended); got != "Extended" {
		t.Errorf("libText(extended) = %q, want Extended", got)
	}
	// an unassigned code stays dim whatever the library says
	if codeCell("—", part.LibBasic, "—") != dimStyle.Render("—") {
		t.Error("an unassigned code should render dim")
	}
}
