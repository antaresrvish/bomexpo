package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// Every view with a text field must let tab take the keyboard back, otherwise
// the letters stay locked away behind ctrl combinations.
func TestTabLeavesEveryTextField(t *testing.T) {
	cases := []struct {
		name    string
		enter   func(Model) Model
		focused func(Model) bool
		send    func(Model, tea.KeyPressMsg) Model
	}{
		{
			name: "parts",
			enter: func(m Model) Model {
				mm, _ := m.gotoTab(modeParts)
				return queryFocus(mm.(Model)) // / is the way in, tab is the way out
			},
			focused: func(m Model) bool { return m.parts.field.Focused() },
			send:    func(m Model, k tea.KeyPressMsg) Model { mm, _ := m.updatePartsKey(k); return mm.(Model) },
		},
		{
			name:    "search",
			enter:   func(m Model) Model { mm, _ := m.openSearch(0); return mm.(Model) },
			focused: func(m Model) bool { return m.search.field.Focused() },
			send:    func(m Model, k tea.KeyPressMsg) Model { mm, _ := m.updateSearchKey(k); return mm.(Model) },
		},
		{
			name: "load",
			enter: func(m Model) Model {
				mm, _ := m.gotoTab(modeLoad)
				mm, _ = mm.(Model).updateLoad(key("/"))
				return mm.(Model)
			},
			focused: func(m Model) bool { return m.load.field.Focused() },
			send:    func(m Model, k tea.KeyPressMsg) Model { mm, _ := m.updateLoad(k); return mm.(Model) },
		},
		{
			name:    "nets",
			enter:   func(m Model) Model { mm, _ := m.openNetPicker(); return mm.(Model) },
			focused: func(m Model) bool { return m.nets.field.Focused() },
			send:    func(m Model, k tea.KeyPressMsg) Model { mm, _ := m.updateNetKey(k); return mm.(Model) },
		},
		{
			name:    "check",
			enter:   func(m Model) Model { mm, _ := m.updateCheck(key("e")); return mm.(Model) },
			focused: func(m Model) bool { return m.check.pane == paneOut },
			send:    func(m Model, k tea.KeyPressMsg) Model { mm, _ := m.updateCheck(k); return mm.(Model) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.enter(netModel(t))
			if !tc.focused(m) {
				t.Fatal("the field should own the keyboard on the way in")
			}
			m = tc.send(m, key("tab"))
			if tc.focused(m) {
				t.Error("tab should have handed the keyboard back")
			}
			if m = tc.enter(m); !tc.focused(m) {
				t.Error("the way in should work a second time")
			}
		})
	}
}

// The digits and brackets switch tabs, but only once a list has the keyboard —
// they're printable, so a focused field has to keep them.
func TestTabSwitchKeysDoNotStealFromAField(t *testing.T) {
	m := netModel(t)
	mm, _ := m.gotoTab(modeParts)
	m = queryFocus(mm.(Model))
	for _, r := range "[3]" {
		mm, _ := m.updatePartsKey(key(string(r)))
		m = mm.(Model)
	}
	if m.mode != modeParts {
		t.Errorf("typing in the query changed tab to %v", m.mode)
	}
	if got := m.parts.field.Value(); got != "[3]" {
		t.Errorf("field = %q, want %q", got, "[3]")
	}

	mm, _ = m.updatePartsKey(key("tab")) // hand the list the keyboard
	mm, _ = mm.(Model).updatePartsKey(key("]"))
	if mm.(Model).mode == modeParts {
		t.Error("] should move to the next tab once the list has focus")
	}
}

// Landing on a tab must leave the list in charge: 1-5 and [ ] are printable, so a
// focused field would eat them and tab switching would silently stop working.
func TestArrivingAtATabLeavesTheListInCharge(t *testing.T) {
	base := netModel(t)
	base.parts.pinned = cmpFixture()
	for _, md := range []mode{modeLoad, modeTable, modeParts, modeCheck, modeCompare} {
		mm, _ := base.gotoTab(md)
		m := mm.(Model)
		if m.mode != md {
			t.Fatalf("gotoTab(%v) landed on %v", md, m.mode)
		}
		if m.parts.field.Focused() || m.check.out.Focused() || m.filter.open || m.load.field.Focused() {
			t.Errorf("%v took the keyboard on entry", md)
		}
		// so a digit gets straight through to tab switching
		mm, _ = m.routeKey(key("2"))
		if mm.(Model).mode != modeTable {
			t.Errorf("from %v, pressing 2 went to %v", md, mm.(Model).mode)
		}
	}
}

func TestFocusMarkOnlyMarksTheFocused(t *testing.T) {
	on, off := stripANSI(focusMark(true)), stripANSI(focusMark(false))
	if !strings.Contains(on, "▸") {
		t.Errorf("focused mark = %q, want a ▸", on)
	}
	if strings.TrimSpace(off) != "" {
		t.Errorf("unfocused mark = %q, want blank", off)
	}
	// both take the same room, so nothing shifts when focus moves
	if len(on) < len(off) {
		t.Error("the marks should occupy the same width")
	}
}

// Down walks into the directory listing, up walks back out of it.
func TestLoadDownEntersTheListing(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.kicad_pcb", "b.csv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Options{})
	m.w, m.h = 120, 40
	m.mode = modeLoad
	m.load.field.SetValue(dir + string(filepath.Separator))
	m.load.field.Focus()

	if n := len(m.loadEntries()); n != 2 {
		t.Fatalf("listing has %d entries, want the 2 files written", n)
	}
	mm, _ := m.updateLoad(key("down"))
	m = mm.(Model)
	if m.load.cursor != 0 || m.load.field.Focused() {
		t.Fatalf("down should focus the listing, cursor = %d focused = %v", m.load.cursor, m.load.field.Focused())
	}
	if !strings.Contains(stripANSI(m.renderListing(m.load.field.Value())), "▶") {
		t.Error("the picked entry should be marked")
	}
	mm, _ = m.updateLoad(key("up"))
	m = mm.(Model)
	if m.load.cursor != -1 || !m.load.field.Focused() {
		t.Error("up off the top row should go back to the path field")
	}
}

// Enter on a highlighted file loads it; the path field ends up holding it, so
// there's no way to open something other than what was highlighted.
func TestLoadEnterOpensThePickedFile(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "board.kicad_pcb")
	if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(Options{})
	m.w, m.h = 120, 40
	m.mode = modeLoad
	m.load.field.SetValue(dir + string(filepath.Separator))

	mm, _ := m.updateLoad(key("down"))
	mm, cmd := mm.(Model).updateLoad(key("enter"))
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("enter on a file should start a load")
	}
	if got := m.load.field.Value(); got != want {
		t.Errorf("loading %q, want %q", got, want)
	}
}

// Load has three keyboard states, and each one has to hand off to the others
// without a dead end.
func TestLoadThreeFocusStates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "board.kicad_pcb"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := netModel(t)
	base.w, base.h = 110, 32
	mm, _ := base.gotoTab(modeLoad)
	m := mm.(Model)
	m.load.field.SetValue(dir + string(filepath.Separator))

	// page focused: the field is quiet and the tab keys are live
	if m.load.field.Focused() || m.load.cursor >= 0 {
		t.Fatal("coming back to Load should leave the page in charge")
	}
	mm, _ = m.updateLoad(key("2"))
	if mm.(Model).mode != modeTable {
		t.Error("2 should switch tabs from the Load page")
	}

	// / types, tab picks the listing back up, and neither strands the keyboard
	mm, _ = m.updateLoad(key("/"))
	typing := mm.(Model)
	if !typing.load.field.Focused() {
		t.Fatal("/ should focus the path")
	}
	mm, _ = typing.updateLoad(key("2"))
	if mm.(Model).mode != modeLoad || mm.(Model).load.field.Value() == typing.load.field.Value() {
		t.Error("a digit typed into the path should be text, not a tab switch")
	}
	mm, _ = m.updateLoad(key("down"))
	picked := mm.(Model)
	if picked.load.cursor != 0 || picked.load.field.Focused() {
		t.Fatalf("down should pick a row, cursor = %d", picked.load.cursor)
	}
	mm, _ = picked.updateLoad(key("2"))
	if mm.(Model).mode != modeTable {
		t.Error("2 should switch tabs with a row picked")
	}
	mm, _ = picked.updateLoad(key("tab"))
	if !mm.(Model).load.field.Focused() {
		t.Error("tab from the listing should type the path")
	}
}

// A letter typed at the Parts list is a command, and one typed at the query is
// text — the same key, decided purely by focus.
func TestSameKeyMeansTextOrCommandByFocus(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 0, 0.01), lcscPart("C2", "B", 500, 0.02))

	mm, _ := queryFocus(m).updatePartsKey(key("s"))
	typed := mm.(Model)
	if typed.parts.field.Value() != "s" {
		t.Errorf("with the query focused, s should type: field = %q", typed.parts.field.Value())
	}
	if typed.parts.inStockOnly {
		t.Error("typing must not have flipped the stock filter")
	}

	mm, _ = m.updatePartsKey(key("s"))
	cmd := mm.(Model)
	if !cmd.parts.inStockOnly {
		t.Error("with the list focused, s should flip the stock filter")
	}
	if cmd.parts.field.Value() != "" {
		t.Errorf("a command must not reach the query: %q", cmd.parts.field.Value())
	}
}

// The bottom bar must fit the terminal exactly in every mode and focus state: a
// single column of overflow pads every line of the view and slides the page.
func TestBottomBarFitsTheTerminal(t *testing.T) {
	base := netModel(t)
	base.parts.pinned = cmpFixture()
	states := []struct {
		name string
		set  func(Model) Model
	}{
		{"load", func(m Model) Model { m.mode = modeLoad; return m }},
		{"load-browsing", func(m Model) Model { m.mode = modeLoad; m.load.cursor = 0; return m }},
		{"table", func(m Model) Model { m.mode = modeTable; return m }},
		{"filter", func(m Model) Model { mm, _ := m.openFilter(); return mm.(Model) }},
		{"search", func(m Model) Model { mm, _ := m.openSearch(0); return mm.(Model) }},
		{"search-list", func(m Model) Model {
			mm, _ := m.openSearch(0)
			mm, _ = mm.(Model).updateSearchKey(key("tab"))
			return mm.(Model)
		}},
		{"parts-list", func(m Model) Model { mm, _ := m.gotoTab(modeParts); return mm.(Model) }},
		{"categories", func(m Model) Model {
			mm, _ := m.gotoTab(modeParts)
			mm, _ = mm.(Model).openCategories()
			return mm.(Model)
		}},
		{"parts", func(m Model) Model { mm, _ := m.gotoTab(modeParts); return queryFocus(mm.(Model)) }},
		{"compare", func(m Model) Model { mm, _ := m.gotoTab(modeCompare); return mm.(Model) }},
		{"nets", func(m Model) Model { mm, _ := m.openNetPicker(); return mm.(Model) }},
		{"nets-list", func(m Model) Model {
			mm, _ := m.openNetPicker()
			mm, _ = mm.(Model).updateNetKey(key("tab"))
			return mm.(Model)
		}},
		{"check", func(m Model) Model { mm, _ := m.gotoTab(modeCheck); return mm.(Model) }},
		{"check-path", func(m Model) Model {
			mm, _ := m.gotoTab(modeCheck)
			mm, _ = mm.(Model).updateCheck(key("tab"))
			return mm.(Model)
		}},
	}
	for _, width := range []int{80, 100, 130, 160} {
		for _, st := range states {
			m := base
			m.w, m.h = width, 32
			m = st.set(m)
			if got := lipgloss.Width(m.bottomBar()); got > width {
				t.Errorf("%s at width %d: bar is %d columns", st.name, width, got)
			}
			for i, ln := range strings.Split(m.screen(), "\n") {
				if got := lipgloss.Width(ln); got > width {
					t.Errorf("%s at width %d: line %d is %d columns", st.name, width, i, got)
				}
			}
		}
	}
}

// The Check page's two halves both want the up and down arrows, so tab has to
// pick which one gets them.
func TestCheckPaneRing(t *testing.T) {
	mm, _ := netModel(t).gotoTab(modeCheck)
	m := mm.(Model)
	m.w, m.h = 130, 32
	m.boardv = m.boardv.zoomBy(zoomStep) // panning is a no-op at the fit-everything zoom
	if m.check.pane != paneIssues {
		t.Fatalf("landing on Check starts at pane %v, want the issues", m.check.pane)
	}

	// with the issues focused, down scrolls the list and leaves the board alone
	m.check.top = 0
	before := m.boardv
	mm, _ = m.updateCheck(key("down"))
	m = mm.(Model)
	if m.check.top != 1 {
		t.Errorf("down should scroll the issues, top = %d", m.check.top)
	}
	if m.boardv != before {
		t.Error("down should not have panned the board")
	}

	// tab moves to the board, and now down pans instead of scrolling
	mm, _ = m.updateCheck(key("tab"))
	m = mm.(Model)
	if m.check.pane != paneBoard {
		t.Fatalf("tab went to pane %v, want the board", m.check.pane)
	}
	top, before := m.check.top, m.boardv
	mm, _ = m.updateCheck(key("down"))
	m = mm.(Model)
	if m.boardv == before {
		t.Error("down should pan the board once it has focus")
	}
	if m.check.top != top {
		t.Error("down should no longer scroll the issues")
	}

	// round the ring to the path and back
	mm, _ = m.updateCheck(key("tab"))
	m = mm.(Model)
	if m.check.pane != paneOut || !m.check.out.Focused() {
		t.Fatalf("pane %v, focused %v — want the path, focused", m.check.pane, m.check.out.Focused())
	}
	mm, _ = m.updateCheck(key("tab"))
	m = mm.(Model)
	if m.check.pane != paneIssues || m.check.out.Focused() {
		t.Errorf("the ring should close back on the issues, got %v", m.check.pane)
	}
	// and backwards
	mm, _ = m.updateCheck(key("shift+tab"))
	if got := mm.(Model).check.pane; got != paneOut {
		t.Errorf("shift+tab from the issues gave %v, want the path", got)
	}
}

// Whichever pane has the keys has to say so on screen.
func TestCheckMarksTheFocusedPane(t *testing.T) {
	mm, _ := netModel(t).gotoTab(modeCheck)
	m := mm.(Model)
	m.w, m.h = 130, 32

	seen := map[checkPane]string{}
	for _, p := range []checkPane{paneIssues, paneBoard, paneOut} {
		m.check.setPane(p)
		out := stripANSI(m.viewCheck(m.contentW(), m.contentH()))
		if strings.Count(out, "▸") != 1 {
			t.Errorf("pane %v: %d focus marks on screen, want exactly 1", p, strings.Count(out, "▸"))
		}
		seen[p] = out
	}
	if seen[paneIssues] == seen[paneBoard] {
		t.Error("moving focus from the issues to the board changed nothing on screen")
	}
}

// The tab bar reads as the job in order, and the status line names the next stage
// once this one is done — but only then, so a half-finished board isn't nagged.
func TestNextStepOnlySpeaksWhenTheStageIsDone(t *testing.T) {
	m := filterModel(t)
	m.w, m.h = 130, 24
	mm, _ := m.gotoTab(modeTable)
	m = mm.(Model)

	if got := m.nextStep(); got != "" {
		t.Errorf("half-assigned board suggests %q", stripANSI(got))
	}
	for i := range m.items {
		p := part.Part{Code: "C1", Stock: 100, Desc: m.items[i].Value,
			Prices: []part.Price{{Ladder: 1, USD: 0.01}}}
		m.assigned[i] = &p
		m.items[i].LCSC = "C1"
	}
	if got := stripANSI(m.nextStep()); got != "→ 4 Verify" {
		t.Errorf("finished board suggests %q, want → 4 Verify", got)
	}
	// and the number it names is the tab it means
	if md, ok := m.tabMode(4); !ok || md != modeDiff {
		t.Errorf("tab 4 is %v, but the hint sends you there for Verify", md)
	}

	// Verify only points at Export once nothing serious is left
	mm, _ = m.gotoTab(modeDiff)
	m = mm.(Model)
	m.diff.ran = true
	m.diff.res = kicad.Compare(&kicad.Schematic{Path: "/tmp/x.kicad_sch"}, nil, nil, kicad.SidePCB)
	if got := stripANSI(m.nextStep()); got != "→ 5 Export" {
		t.Errorf("a clean verify suggests %q, want → 5 Export", got)
	}
	m.diff.res.Findings = []kicad.Finding{{Kind: kicad.DiffMissing, Ref: "R1"}}
	if got := m.nextStep(); got != "" {
		t.Errorf("a verify with a serious finding suggests %q", stripANSI(got))
	}
	if md, ok := m.tabMode(5); !ok || md != modeCheck {
		t.Errorf("tab 5 is %v, but the hint sends you there to Export", md)
	}
}

// Compare is reached from Parts and leaves the tab bar unchanged, so the numbers
// never shift under the user.
func TestCompareIsADetourOffParts(t *testing.T) {
	m := filterModel(t)
	m.w, m.h = 130, 24
	m.parts.pinned = cmpFixture()
	before := len(m.tabs())

	mm, _ := m.gotoTab(modeCompare)
	m = mm.(Model)
	if m.mode != modeCompare {
		t.Fatal("could not reach Compare")
	}
	if got := len(m.tabs()); got != before {
		t.Errorf("the tab bar changed from %d to %d tabs", before, got)
	}
	// Parts stays lit while you're in its detour
	bar := stripANSI(m.tabBar())
	for _, want := range []string{"Load", "Components", "Parts", "Verify", "Export"} {
		if !strings.Contains(bar, want) {
			t.Errorf("%q missing from the tab bar: %q", want, bar)
		}
	}
	if strings.Contains(bar, "Compare") {
		t.Errorf("Compare should not be a tab: %q", bar)
	}
}
