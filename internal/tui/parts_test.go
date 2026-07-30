package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/part"
)

func lcscPart(code, mpn string, stock int, usd float64) part.Part {
	return part.Part{
		Source: "lcsc", Code: code, MPN: mpn, Package: "0402", Stock: stock,
		Desc: mpn + " description", Datasheet: "https://x/" + code + ".pdf",
		Prices: []part.Price{{Ladder: 1, USD: usd}},
	}
}

func partsModel(t *testing.T, results ...part.Part) Model {
	t.Helper()
	m := New(Options{})
	m.w, m.h = 140, 40
	m.parts.results = results
	m.parts.total = len(results)
	mm, _ := m.gotoTab(modeParts) // enters with the list focused, like the app does
	m = mm.(Model)
	return m
}

// queryFocus hands the keyboard to the search query the way a user does: / opens
// the category popup, and closing it leaves the query focused.
func queryFocus(m Model) Model {
	mm, _ := m.updatePartsKey(key("/"))
	mm, _ = mm.(Model).updateCatKey(key("esc"))
	return mm.(Model)
}

func TestTogglePinAndCap(t *testing.T) {
	m := partsModel(t,
		lcscPart("C1", "A", 100, 0.01),
		lcscPart("C2", "B", 200, 0.02),
		lcscPart("C3", "C", 300, 0.03),
		lcscPart("C4", "D", 400, 0.04),
		lcscPart("C5", "E", 500, 0.05),
	)

	// pin the first four
	for i := 0; i < 4; i++ {
		m.parts.cursor = i
		mm, cmd := m.togglePin()
		m = mm.(Model)
		if cmd == nil {
			t.Errorf("pinning should fetch the full record for row %d", i)
		}
	}
	if len(m.parts.pinned) != 4 {
		t.Fatalf("pinned %d parts, want 4", len(m.parts.pinned))
	}

	// the fifth is refused, and says why
	m.parts.cursor = 4
	mm, _ := m.togglePin()
	m = mm.(Model)
	if len(m.parts.pinned) != 4 {
		t.Errorf("pinned %d, want the cap to hold at 4", len(m.parts.pinned))
	}
	if !strings.Contains(m.flash, "unpin") {
		t.Errorf("flash %q should tell the user to unpin something", m.flash)
	}

	// pinning an already-pinned row unpins it
	m.parts.cursor = 1
	mm, cmd := m.togglePin()
	m = mm.(Model)
	if len(m.parts.pinned) != 3 || m.parts.pinAt("lcsc", "C2") >= 0 {
		t.Errorf("C2 should be unpinned, have %d pins", len(m.parts.pinned))
	}
	if cmd != nil {
		t.Error("unpinning should not fetch anything")
	}
}

func TestPinAtDistinguishesSources(t *testing.T) {
	s := partsState{pinned: []part.Part{
		{Source: "lcsc", Code: "C1525"},
		{Source: "jlcpcb", Code: "C1525"},
	}}
	if got := s.pinAt("lcsc", "C1525"); got != 0 {
		t.Errorf("lcsc C1525 at %d, want 0", got)
	}
	if got := s.pinAt("jlcpcb", "C1525"); got != 1 {
		t.Errorf("jlcpcb C1525 at %d, want 1", got)
	}
	if got := s.pinAt("mouser", "C1525"); got != -1 {
		t.Errorf("unknown source at %d, want -1", got)
	}
}

// Pins must not alias each other's backing array — Model is copied by value on
// every update, so an in-place append would corrupt an older copy.
func TestUnpinDoesNotAliasBackingArray(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 1, 1), lcscPart("C2", "B", 1, 1), lcscPart("C3", "C", 1, 1))
	for i := 0; i < 3; i++ {
		m.parts.cursor = i
		mm, _ := m.togglePin()
		m = mm.(Model)
	}
	before := append([]part.Part(nil), m.parts.pinned...)

	m.parts.cursor = 1
	mm, _ := m.togglePin() // unpin the middle one
	after := mm.(Model)

	if len(m.parts.pinned) != 3 {
		t.Fatalf("the original model changed under us: %d pins", len(m.parts.pinned))
	}
	for i := range before {
		if m.parts.pinned[i].Code != before[i].Code {
			t.Errorf("original pin %d became %s, want %s", i, m.parts.pinned[i].Code, before[i].Code)
		}
	}
	if len(after.parts.pinned) != 2 {
		t.Errorf("new model has %d pins, want 2", len(after.parts.pinned))
	}
}

func TestPinDetailReplacesOnlyMatchingPin(t *testing.T) {
	m := partsModel(t)
	m.parts.pinned = []part.Part{
		{Source: "lcsc", Code: "C1", MPN: "thin"},
		{Source: "jlcpcb", Code: "C1", MPN: "other source"},
	}

	full := part.Part{Source: "lcsc", Code: "C1", MPN: "thin", Params: []part.Param{{Name: "Tolerance", Value: "±1%"}}}
	mm, _ := m.updatePinDetail(pinDetailMsg{source: "lcsc", code: "C1", part: full})
	m = mm.(Model)

	if len(m.parts.pinned[0].Params) != 1 {
		t.Error("the lcsc pin should have been hydrated")
	}
	if len(m.parts.pinned[1].Params) != 0 {
		t.Error("the jlcpcb pin with the same code must not be touched")
	}

	// a failed or stale fetch leaves the search-result copy alone
	mm, _ = m.updatePinDetail(pinDetailMsg{source: "lcsc", code: "C1", err: errNoSource})
	if got := mm.(Model).parts.pinned[0].MPN; got != "thin" {
		t.Errorf("a failed detail overwrote the pin: %q", got)
	}
}

// The tab bar is the job in order, and it never changes shape: Compare is a detour
// off Parts, not a sixth tab that appears and shifts every number along.
func TestTabBarIsTheJobInOrder(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 1, 1), lcscPart("C2", "B", 1, 1))

	labels := func() string {
		var out []string
		for _, t := range m.tabs() {
			out = append(out, t.label)
		}
		return strings.Join(out, ",")
	}
	if got, want := labels(), "Load,Components,Parts,Verify,Export"; got != want {
		t.Errorf("tabs = %s, want %s", got, want)
	}

	m.parts.cursor = 0
	mm, _ := m.togglePin()
	m = mm.(Model)
	if got := labels(); strings.Contains(got, "Compare") {
		t.Errorf("Compare is a detour off Parts, not a tab: %s", got)
	}

	m.parts.cursor = 1
	mm, _ = m.togglePin()
	m = mm.(Model)
	if got, want := labels(), "Load,Components,Parts,Verify,Export"; got != want {
		t.Errorf("tabs = %s, want %s", got, want)
	}

	// the digit shortcuts follow the visible tabs
	for n, want := range map[int]mode{1: modeLoad, 2: modeTable, 3: modeParts,
		4: modeDiff, 5: modeCheck} {
		got, ok := m.tabMode(n)
		if !ok || got != want {
			t.Errorf("tabMode(%d) = %v/%v, want %v", n, got, ok, want)
		}
	}
	if _, ok := m.tabMode(6); ok {
		t.Error("tabMode(6) should not resolve")
	}
}

func TestGotoCompareRefusedWithoutTwoPins(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 1, 1))
	m.parts.cursor = 0
	mm, _ := m.togglePin()
	m = mm.(Model)

	mm, _ = m.gotoTab(modeCompare)
	m = mm.(Model)
	if m.mode == modeCompare {
		t.Error("compare should refuse to open with one pin")
	}
	if m.flash == "" {
		t.Error("refusing to open compare should explain itself")
	}
}

func TestPartsInStockFilter(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 0, 0.01), lcscPart("C2", "B", 500, 0.02))
	if got := len(m.parts.filtered()); got != 2 {
		t.Fatalf("unfiltered = %d, want 2", got)
	}
	// a plain s with the list focused, and ^s while typing — both must toggle
	for _, k := range []tea.KeyPressMsg{key("s"), key("/"), {Code: 's', Mod: tea.ModCtrl}} {
		mm, _ := m.updatePartsKey(k)
		m = mm.(Model)
	}
	if got := len(m.parts.filtered()); got != 2 {
		t.Fatalf("two toggles should cancel out, got %d rows", got)
	}
	mm, _ := m.updatePartsKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = mm.(Model)
	f := m.parts.filtered()
	if len(f) != 1 || f[0].Code != "C2" {
		t.Errorf("in-stock filter gave %+v, want just C2", f)
	}
}

func TestPartsListFocusTreatsLettersAsCommands(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "A", 100, 0.01))
	for _, k := range []string{"p"} { // pin without a modifier
		mm, _ := m.updatePartsKey(key(k))
		m = mm.(Model)
	}
	if len(m.parts.pinned) != 1 {
		t.Fatalf("p should have pinned the row, pinned = %d", len(m.parts.pinned))
	}
	if m.parts.field.Value() != "" {
		t.Errorf("a command letter leaked into the query: %q", m.parts.field.Value())
	}
	// / reaches for the search, which opens the category popup on the way
	mm, _ := m.updatePartsKey(key("/"))
	if !mm.(Model).cat.open {
		t.Error("/ should open the category popup")
	}
}

func TestPartsTypingDoesNotTriggerActions(t *testing.T) {
	m := queryFocus(partsModel(t))
	// plain letters must reach the search field, not fire commands
	for _, r := range "c1525 x" {
		mm, _ := m.updatePartsKey(tea.KeyPressMsg{Text: string(r), Code: r})
		m = mm.(Model)
	}
	if got := m.parts.field.Value(); got != "c1525 x" {
		t.Errorf("field = %q, want %q", got, "c1525 x")
	}
	if len(m.parts.pinned) != 0 {
		t.Error("typing should not have pinned anything")
	}
}

func TestViewPartsRendersAndMarksPins(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "AAA", 100, 0.01), lcscPart("C2", "BBB", 200, 0.02))
	m.parts.cursor = 1
	mm, _ := m.togglePin()
	m = mm.(Model)

	out := stripANSI(m.viewParts(m.contentW(), m.contentH()))
	if !strings.Contains(out, "C1") || !strings.Contains(out, "C2") {
		t.Fatalf("results missing from the view:\n%s", out)
	}
	if !strings.Contains(out, "◆") {
		t.Error("the pinned row should carry a ◆ marker")
	}
	if !strings.Contains(out, "pinned") {
		t.Error("the footer should list what's pinned")
	}
}

// The shortlist outlives the search that found it: a pinned part stays a row you
// can reach, so it can be unpinned even after the query is gone.
func TestPinnedPartsSurviveAnEmptiedSearch(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "AAA", 100, 0.01), lcscPart("C2", "BBB", 200, 0.02))
	for _, i := range []int{0, 1} {
		m.parts.cursor = i
		mm, _ := m.togglePin()
		m = mm.(Model)
	}

	m.parts.field.SetValue("")
	mm, _ := m.researchParts()
	m = mm.(Model)
	if len(m.parts.results) != 0 {
		t.Fatalf("an empty query should clear the results, got %d", len(m.parts.results))
	}
	rows := m.parts.rows()
	if len(rows) != 2 {
		t.Fatalf("%d rows with an empty search, want the 2 pinned", len(rows))
	}
	out := stripANSI(m.viewParts(m.contentW(), m.contentH()))
	for _, code := range []string{"C1", "C2"} {
		if !strings.Contains(out, code) {
			t.Errorf("%s is not on screen, so it cannot be unpinned:\n%s", code, out)
		}
	}

	// and it can be removed from there
	m.parts.cursor = 0
	mm, _ = m.togglePin()
	if got := len(mm.(Model).parts.pinned); got != 1 {
		t.Errorf("unpinning left %d pins, want 1", got)
	}
}

// A pinned part appears once, in the shortlist, even when the search that's running
// also returns it.
func TestPinnedPartIsNotListedTwice(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "AAA", 100, 0.01), lcscPart("C2", "BBB", 200, 0.02))
	m.parts.cursor = 1
	mm, _ := m.togglePin()
	m = mm.(Model)

	rows := m.parts.rows()
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2", len(rows))
	}
	if rows[0].Code != "C2" {
		t.Errorf("the pinned part should lead, got %s", rows[0].Code)
	}
	seen := map[string]int{}
	for _, p := range rows {
		seen[p.Code]++
	}
	for code, n := range seen {
		if n > 1 {
			t.Errorf("%s appears %d times", code, n)
		}
	}
}

// The filters are for the search results; a pin is a decision and stays put.
func TestFiltersDoNotDropPinnedParts(t *testing.T) {
	m := partsModel(t, lcscPart("C1", "AAA", 0, 0.01), lcscPart("C2", "BBB", 200, 0.02))
	m.parts.cursor = 0 // out of stock
	mm, _ := m.togglePin()
	m = mm.(Model)

	m.parts.inStockOnly = true
	if got := len(m.parts.filtered()); got != 1 {
		t.Fatalf("the in-stock filter kept %d results, want 1", got)
	}
	rows := m.parts.rows()
	if len(rows) != 2 || rows[0].Code != "C1" {
		t.Errorf("rows = %v, want the out-of-stock pin to survive", codesOf(rows))
	}

	m.parts.cat = "nothing matches this"
	if rows := m.parts.rows(); len(rows) != 1 || rows[0].Code != "C1" {
		t.Errorf("a category filter dropped the pin: %v", codesOf(rows))
	}
}

func codesOf(ps []part.Part) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Code
	}
	return out
}
