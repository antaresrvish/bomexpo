package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// suggestModel is filterModel with the query field open and holding q.
func suggestModel(t *testing.T, q string) Model {
	t.Helper()
	m := filterModel(t)
	mm, _ := m.openFilter()
	m = mm.(Model)
	m.filter.field.SetValue(q)
	m.filter.f = parseFilter(q)
	return m.reindex()
}

// offered renders the dropdown contents as "label=count" so a test can state
// both the order and the numbers.
func offered(m Model) string {
	var out []string
	for _, s := range m.suggestions() {
		if s.count < 0 {
			out = append(out, s.label)
			continue
		}
		out = append(out, fmt.Sprintf("%s=%d", s.label, s.count))
	}
	return strings.Join(out, ",")
}

func TestSuggestOffersTheKeysFirst(t *testing.T) {
	m := suggestModel(t, "")
	got := offered(m)
	for _, want := range []string{"net:", "val:", "fp:", "st:", "lib:", "ref:", "lcsc:"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from the key list: %s", want, got)
		}
	}
	// each key comes with a word on what it does — that's the discoverability
	if !strings.Contains(got, "what a part connects to") {
		t.Errorf("keys should be described: %s", got)
	}

	// typing narrows to the matching keys
	if got := offered(suggestModel(t, "l")); !strings.HasPrefix(got, "lib:") || strings.Contains(got, "net:") {
		t.Errorf("l → %s, want the lib/lcsc keys only", got)
	}
}

func TestSuggestOffersRealNetsBusiestFirst(t *testing.T) {
	m := suggestModel(t, "net:")
	// GND on 4 line items, +5V on 2, the rest on 1 each and then alphabetical
	if got, want := offered(m), "GND=4,+5V=2,+3V3=1,SPI_SCK=1,VBUS=1"; got != want {
		t.Errorf("net: → %s, want %s", got, want)
	}

	// partials match anywhere in the name, so 3v3 finds +3V3
	if got, want := offered(suggestModel(t, "net:3v3")), "+3V3=1"; got != want {
		t.Errorf("net:3v3 → %s, want %s", got, want)
	}
	if got := offered(suggestModel(t, "net:nope")); got != "" {
		t.Errorf("net:nope → %s, want nothing", got)
	}
}

func TestSuggestOffersValuesFootprintsAndCodes(t *testing.T) {
	if got, want := offered(suggestModel(t, "val:10")), "100nF=1,10k=1,10uF=1"; got != want {
		t.Errorf("val:10 → %s, want %s", got, want)
	}
	if got := offered(suggestModel(t, "fp:0402")); !strings.Contains(got, "C_0402_1005Metric=1") {
		t.Errorf("fp:0402 → %s", got)
	}
	if got := offered(suggestModel(t, "lcsc:")); !strings.Contains(got, "C1525=1") {
		t.Errorf("lcsc: → %s", got)
	}
	if got := offered(suggestModel(t, "ref:C")); !strings.Contains(got, "C1=1") {
		t.Errorf("ref:C → %s", got)
	}
}

// Fixed vocabularies keep their own order and list every option, including the
// ones that would show nothing — you still want to know they exist.
func TestSuggestFixedVocabularies(t *testing.T) {
	if got, want := offered(suggestModel(t, "st:")),
		"unassigned=1,oos=1,mismatch=0,ok=2,excluded=2,dnp=1,rot=0"; got != want {
		t.Errorf("st: → %s, want %s", got, want)
	}
	if got, want := offered(suggestModel(t, "lib:")),
		"basic=2,preferred=0,extended=2,none=2"; got != want {
		t.Errorf("lib: → %s, want %s", got, want)
	}
}

// A count says what adding the term would show, so it has to respect the terms
// already typed rather than reporting a board-wide total.
func TestSuggestCountsRespectTheRestOfTheQuery(t *testing.T) {
	if got, want := offered(suggestModel(t, "st:")), "unassigned=1,oos=1,mismatch=0,ok=2,excluded=2,dnp=1,rot=0"; got != want {
		t.Fatalf("unfiltered st: → %s, want %s", got, want)
	}
	// narrowed to the four GND parts, the state counts follow
	if got, want := offered(suggestModel(t, "net:GND st:")),
		"unassigned=0,oos=1,mismatch=0,ok=2,excluded=1,dnp=1,rot=0"; got != want {
		t.Errorf("net:GND st: → %s, want %s", got, want)
	}
	// and so do the net counts
	if got, want := offered(suggestModel(t, "fp:0402 net:")), "GND=2,+3V3=1,SPI_SCK=1"; got != want {
		t.Errorf("fp:0402 net: → %s, want %s", got, want)
	}
}

func TestAcceptCompletesTheWord(t *testing.T) {
	// a bare key gets no trailing space: it still needs a value
	m := suggestModel(t, "ne")
	m = m.acceptSuggestion()
	if got := m.filter.field.Value(); got != "net:" {
		t.Errorf("accepted → %q, want net:", got)
	}

	// a finished value does, so the next term can be typed straight away
	m = suggestModel(t, "net:sp")
	m = m.acceptSuggestion()
	if got := m.filter.field.Value(); got != "net:SPI_SCK " {
		t.Errorf("accepted → %q, want %q", got, "net:SPI_SCK ")
	}
	if got, want := shown(m), "R1"; got != want {
		t.Errorf("the filter should already apply: %s, want %s", got, want)
	}

	// earlier terms are left alone
	m = suggestModel(t, "fp:0402 net:gn")
	m = m.acceptSuggestion()
	if got := m.filter.field.Value(); got != "fp:0402 net:GND " {
		t.Errorf("accepted → %q", got)
	}

	// a negated term keeps its minus
	m = suggestModel(t, "-net:gn")
	m = m.acceptSuggestion()
	if got := m.filter.field.Value(); got != "-net:GND " {
		t.Errorf("accepted → %q, want the minus kept", got)
	}
}

func TestSuggestNavigationAndAcceptKeys(t *testing.T) {
	m := suggestModel(t, "net:")

	// ctrl+n walks the dropdown and wraps
	ctrl := func(m Model, c rune) Model {
		mm, _ := m.updateFilterKey(tea.KeyPressMsg{Code: c, Mod: tea.ModCtrl})
		return mm.(Model)
	}
	m = ctrl(ctrl(m, 'n'), 'n')
	if m.filter.sug != 2 {
		t.Fatalf("sug = %d, want 2 after two ctrl+n", m.filter.sug)
	}
	m = ctrl(m, 'p')
	if m.filter.sug != 1 {
		t.Errorf("sug = %d, want 1 after a ctrl+p", m.filter.sug)
	}

	// tab takes the highlighted one, which is +5V
	mm, _ := m.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m = mm.(Model)
	if got := m.filter.field.Value(); got != "net:+5V " {
		t.Errorf("tab → %q, want net:+5V ", got)
	}
	if !m.filter.open {
		t.Error("tab should leave the bar open for more terms")
	}
}

// The arrows belong to the dropdown while the query has focus — that's where
// you're picking. Enter is what hands the keyboard to the rows.
func TestArrowsStayInTheDropdown(t *testing.T) {
	m := suggestModel(t, "net:")

	mm, _ := m.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m = mm.(Model)
	if !m.filter.open {
		t.Fatal("down should not leave the query")
	}
	if m.filter.sug != 1 {
		t.Errorf("sug = %d, want 1 — down moves the highlight", m.filter.sug)
	}
	if m.suggestBox(80) == nil {
		t.Error("the dropdown should still be open")
	}
	mm, _ = m.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := mm.(Model).filter.sug; got != 0 {
		t.Errorf("sug = %d, want 0 after an up", got)
	}
}

func TestEnterHandsOverToTheTable(t *testing.T) {
	m := suggestModel(t, "net:GND")
	mm, _ := m.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mm.(Model)

	if m.filter.open {
		t.Error("enter should hand the keyboard to the table")
	}
	if m.suggestBox(80) != nil {
		t.Error("enter should dismiss the dropdown")
	}
	if !m.filter.f.active() || shown(m) != "C1,C2,R1,D1" {
		t.Errorf("enter must keep the filter applied, got %q", shown(m))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want the first row", m.cursor)
	}
	// the bar stays visible so a filter is never in force invisibly
	if !m.filterBarVisible() {
		t.Error("the filter bar should still show what's filtering")
	}

	// and from there the arrows walk the filtered rows
	mm, _ = m.updateTable(tea.KeyPressMsg{Code: tea.KeyDown})
	m = mm.(Model)
	if got := m.items[m.sel()].ID(); got != "C2" {
		t.Errorf("selected %s, want C2 — the second filtered row", got)
	}
}

// The bar says which half has the keyboard, since that's the only cue.
func TestFilterBarShowsWhereFocusIs(t *testing.T) {
	open := suggestModel(t, "net:GND")
	if got := stripANSI(open.filterBar(100)); !strings.Contains(got, "▸") {
		t.Errorf("a focused query should be marked: %q", got)
	}
	mm, _ := open.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := stripANSI(mm.(Model).filterBar(100)); strings.Contains(got, "▸") {
		t.Errorf("an unfocused query should drop the marker: %q", got)
	}
}

// Tab is the completion key, so a half-typed value finishes without leaving.
func TestTabCompletesWithoutLeaving(t *testing.T) {
	m := suggestModel(t, "net:sp")
	mm, _ := m.updateFilterKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m = mm.(Model)
	if !m.filter.open {
		t.Error("tab should keep the query focused")
	}
	if got := m.filter.field.Value(); got != "net:SPI_SCK " {
		t.Errorf("tab → %q", got)
	}
	if got, want := shown(m), "R1"; got != want {
		t.Errorf("the completed filter should apply: %s, want %s", got, want)
	}
}

func TestSuggestBoxOnlyWhenOpen(t *testing.T) {
	if box := filterModel(t).suggestBox(80); box != nil {
		t.Error("no dropdown when the bar is closed")
	}
	if box := suggestModel(t, "net:nope").suggestBox(80); box != nil {
		t.Error("no dropdown when nothing matches")
	}
}

func TestSuggestBoxFitsItsWidth(t *testing.T) {
	m := suggestModel(t, "net:")
	for _, w := range []int{30, 46, 80} {
		for i, ln := range m.suggestBox(w) {
			if got := lipgloss.Width(ln); got > w {
				t.Errorf("width %d: line %d is %d wide", w, i, got)
			}
		}
	}
}

// More candidates than fit must be capped, and the box has to say so rather
// than quietly hiding them.
func TestSuggestBoxCapsAndSaysSo(t *testing.T) {
	m := filterModel(t)
	m.items = nil
	for i := 0; i < sugMax+4; i++ {
		m.items = append(m.items, kicad.Item{
			Bases:       []string{fmt.Sprintf("R%d", i+1)},
			Designators: []string{fmt.Sprintf("R%d", i+1)},
			Value:       fmt.Sprintf("%dk", i+1),
			Footprint:   "R_0402_1005Metric",
			Quantity:    1,
		})
	}
	m.assigned = make([]*part.Part, len(m.items))
	m.excluded = make([]bool, len(m.items))
	mm, _ := m.reindex().openFilter()
	m = mm.(Model)
	m.filter.field.SetValue("val:")

	if got, want := len(m.suggestions()), sugMax+4; got != want {
		t.Fatalf("%d candidates, want %d", got, want)
	}
	box := m.suggestBox(80)
	if len(box) != sugMax+3 { // two borders plus the "+N more" line
		t.Errorf("dropdown has %d lines, want %d", len(box), sugMax+3)
	}
	if got := stripANSI(strings.Join(box, "\n")); !strings.Contains(got, "+4 more") {
		t.Errorf("want a +4 more line:\n%s", got)
	}

	// the capped rows are the clickable ones, so a click can't land past the list
	if _, n := m.suggestClickRows(); n != sugMax {
		t.Errorf("clickable rows = %d, want %d", n, sugMax)
	}
}

// A click has to land on the suggestion it looks like it's on.
func TestSuggestClickPicksTheRow(t *testing.T) {
	m := suggestModel(t, "net:")
	top, n := m.suggestClickRows()
	if n != len(m.suggestions()) {
		t.Fatalf("clickable rows = %d, want %d", n, len(m.suggestions()))
	}

	// row 1 of the dropdown is +5V
	mm, _ := m.mouseTable(tea.Mouse{X: 6, Y: top + 1, Button: tea.MouseLeft}, true, false)
	m = mm.(Model)
	if got := m.filter.field.Value(); got != "net:+5V " {
		t.Errorf("clicking row 1 gave %q, want net:+5V ", got)
	}
}

func TestFilteredTableStillFitsWithTheDropdown(t *testing.T) {
	m := suggestModel(t, "net:")
	h := m.contentH()
	block := m.tableBlock(layoutCols(m.tableW()), m.tableW(), h)
	if len(block) != h {
		t.Fatalf("table block has %d lines, want %d", len(block), h)
	}
	joined := stripANSI(strings.Join(block, "\n"))
	if !strings.Contains(joined, "GND") {
		t.Error("the dropdown should be drawn over the table")
	}
	// the horizontal scrollbar keeps the last line whatever overlays above it
	if strings.Contains(stripANSI(block[len(block)-1]), "GND") {
		t.Error("the dropdown must not overwrite the bottom scrollbar row")
	}
}
