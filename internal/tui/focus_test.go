package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
			enter:   func(m Model) Model { mm, _ := m.updateCheck(key("tab")); return mm.(Model) },
			focused: func(m Model) bool { return m.check.out.Focused() },
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
			m = tc.send(m, key("tab"))
			if !tc.focused(m) {
				t.Error("tab should hand it forward again")
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
