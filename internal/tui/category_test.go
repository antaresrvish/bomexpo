package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
	"bomexpo/internal/taxonomy"
)

// catPart is a result row with just a code and a category, which is all the
// picker reads.
func catPart(code, parent, leaf string) part.Part {
	return part.Part{Source: "lcsc", Code: code, ParentCat: parent, Category: leaf, Stock: 100,
		Prices: []part.Price{{Ladder: 1, USD: 0.01}}}
}

// catModel is the Parts tab holding a result set that spans several categories,
// the case the picker exists for.
func catModel(t *testing.T) Model {
	t.Helper()
	var rows []part.Part
	add := func(n int, parent, leaf string) {
		for i := 0; i < n; i++ {
			rows = append(rows, catPart(leaf+string(rune('a'+i)), parent, leaf))
		}
	}
	add(4, "Connectors", "USB Connectors")
	add(2, "Connectors", "Pin Headers")
	add(7, "Capacitors", "Multilayer Ceramic Capacitors MLCC - SMD/SMT")
	add(1, "Capacitors", "Tantalum Capacitors")
	m := New(Options{})
	m.w, m.h = 130, 34
	m.parts.results = rows
	m.parts.total = 5000
	mm, _ := m.gotoTab(modeParts)
	m = mm.(Model)
	// pretend the taxonomy is already cached, so no probe search runs in a test
	m.cat.cats = taxonomy.FromParts(rows)
	m.cat.loaded = m.srcID()
	cm, _ := m.openCategories()
	return cm.(Model)
}

// The busiest parent comes first and the busiest leaf within it, because that's
// the order you want to scan.
func TestCategoriesGroupByParentBusiestFirst(t *testing.T) {
	cells := catModel(t).catCells()
	var got []string
	for _, c := range cells {
		switch {
		case c.all:
			got = append(got, "ALL")
		case c.heading:
			got = append(got, "# "+c.label)
		default:
			got = append(got, c.label)
		}
	}
	want := []string{
		"ALL",
		"# Capacitors", // 8 rows beats Connectors' 6
		"Multilayer Ceramic Capacitors MLCC - SMD/SMT",
		"Tantalum Capacitors",
		"# Connectors",
		"USB Connectors",
		"Pin Headers",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("grouping =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if cells[0].count != 14 {
		t.Errorf("the all box counts %d rows, want 14", cells[0].count)
	}
}

// Picking a box narrows the table to that category and nothing else.
func TestPickingACategoryNarrowsTheResults(t *testing.T) {
	m := catModel(t)
	if got := len(m.parts.filtered()); got != 14 {
		t.Fatalf("unfiltered = %d rows, want 14", got)
	}
	cells := m.catCells()
	var usb catCell
	for _, c := range cells {
		if c.label == "USB Connectors" {
			usb = c
		}
	}
	mm, _ := m.applyCategory(usb)
	m = mm.(Model)
	if m.cat.open {
		t.Error("picking a category should close the panel")
	}
	f := m.parts.filtered()
	if len(f) != 4 {
		t.Fatalf("USB Connectors gave %d rows, want 4", len(f))
	}
	for _, p := range f {
		if p.Category != "USB Connectors" {
			t.Errorf("a %q row survived the USB filter", p.Category)
		}
	}

	// the all box puts everything back
	mm, _ = m.applyCategory(catCell{label: "all categories", all: true})
	if got := len(mm.(Model).parts.filtered()); got != 14 {
		t.Errorf("clearing the category gave %d rows, want 14", got)
	}
}

// The boxes are grouped over the unfiltered results, so picking one doesn't make
// the others vanish and leave you stuck.
func TestCategoryBoxesSurviveTheirOwnFilter(t *testing.T) {
	m := catModel(t)
	before := len(catPickable(m.catCells()))
	m.parts.cat = "USB Connectors"
	if after := len(catPickable(m.catCells())); after != before {
		t.Errorf("%d boxes with a category applied, %d without — you could not switch away", after, before)
	}
}

// The whole flow is pick a category, then search inside it, so a new query must
// keep the category rather than quietly dropping it.
func TestSearchingKeepsThePickedCategory(t *testing.T) {
	m := catModel(t)
	m.parts.cat = "USB Connectors"
	m.parts.field.SetValue("stm32")
	mm, _ := m.researchParts()
	if got := mm.(Model).parts.cat; got != "USB Connectors" {
		t.Errorf("a new query changed the category to %q", got)
	}
}

// Arrows walk the boxes and skip the parent headings, which are labels not
// choices.
func TestCategoryArrowsSkipHeadings(t *testing.T) {
	m := catModel(t)
	cells := m.catCells()
	mm, _ := m.updateCatKey(key("tab")) // hand the grid the keyboard
	m = mm.(Model)

	seen := map[int]bool{m.cat.cursor: true}
	for i := 0; i < 12; i++ {
		mm, _ = m.updateCatKey(key("right"))
		m = mm.(Model)
		if cells[m.cat.cursor].heading {
			t.Fatalf("the cursor landed on the heading %q", cells[m.cat.cursor].label)
		}
		seen[m.cat.cursor] = true
	}
	if len(seen) != len(catPickable(cells)) {
		t.Errorf("right reached %d boxes of %d", len(seen), len(catPickable(cells)))
	}

	// down walks rows and also never lands on a heading
	m = catModel(t)
	mm, _ = m.updateCatKey(key("tab"))
	m = mm.(Model)
	for i := 0; i < 6; i++ {
		mm, _ = m.updateCatKey(key("down"))
		m = mm.(Model)
		if cells[m.cat.cursor].heading {
			t.Fatalf("down landed on the heading %q", cells[m.cat.cursor].label)
		}
	}
}

// The popup's input is its own: it narrows the category list and never reaches the
// parts query. Two inputs taking the same keystroke was the bug.
func TestCategoryFilterIsSeparateFromTheSearch(t *testing.T) {
	m := catModel(t)
	before := len(catPickable(m.catCells()))
	for _, r := range "usb" {
		mm, cmd := m.updateCatKey(key(string(r)))
		m = mm.(Model)
		if cmd != nil {
			t.Error("narrowing the category list should not fire a parts search")
		}
	}
	if got := m.cat.field.Value(); got != "usb" {
		t.Errorf("the popup's input holds %q, want %q", got, "usb")
	}
	if got := m.parts.field.Value(); got != "" {
		t.Errorf("the keystrokes leaked into the parts query: %q", got)
	}
	after := catPickable(m.catCells())
	if len(after) >= before {
		t.Errorf("%d categories after narrowing, %d before", len(after), before)
	}
	// and the cursor lands on something that survived the filter
	cells := m.catCells()
	if m.cat.cursor >= len(cells) || cells[m.cat.cursor].heading {
		t.Errorf("cursor %d is not on a box after narrowing", m.cat.cursor)
	}
}

// Closing the popup, either way, leaves the keyboard on the parts query — that's
// what you reach for next.
func TestClosingTheCategoryPopupFocusesTheSearch(t *testing.T) {
	for _, k := range []string{"esc", "enter"} {
		m := catModel(t)
		if !m.cat.field.Focused() || m.parts.field.Focused() {
			t.Fatal("the popup should open with its own input focused and the query quiet")
		}
		mm, _ := m.updateCatKey(key(k))
		m = mm.(Model)
		if m.cat.open {
			t.Errorf("%s left the popup open", k)
		}
		if !m.parts.field.Focused() {
			t.Errorf("%s did not hand the keyboard to the parts query", k)
		}
		if m.cat.field.Focused() {
			t.Errorf("%s left the popup input focused", k)
		}
	}
}

// t reopens the popup from the results, so a category can be changed without
// clearing the search.
func TestTReopensTheCategoryPopup(t *testing.T) {
	m := catModel(t)
	mm, _ := m.updateCatKey(key("esc"))
	m = mm.(Model)
	m.parts.field.Blur() // back on the results list
	mm, _ = m.updatePartsKey(key("t"))
	if !mm.(Model).cat.open {
		t.Error("t should open the category popup")
	}
}

// Every grid row must come out exactly the panel width, at any size, or the
// boxes stop lining up.
func TestCategoryGridWidthHolds(t *testing.T) {
	m := catModel(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {130, 34}, {170, 44}, {60, 16}} {
		m.w, m.h = size[0], size[1]
		w, h := m.contentW(), m.contentH()
		lines := strings.Split(m.viewCategories(w, h), "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), h)
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got > w {
				t.Errorf("%dx%d: line %d is %d columns, want at most %d", size[0], size[1], i, got, w)
			}
		}
	}
}

// A long category name gets cut, but never at the cost of the count or the box's
// own right edge.
func TestCategoryBoxKeepsTheCountAndItsBorder(t *testing.T) {
	long := catCell{label: "Ethernet Connectors / Modular Connectors (RJ45 RJ11) and friends", count: 1234}
	for _, w := range []int{26, 30, 44, 60} {
		box := catBox(long, w, true)
		for i, ln := range box {
			if got := lipgloss.Width(ln); got != w {
				t.Errorf("width %d line %d came out %d columns", w, i, got)
			}
		}
		mid := stripANSI(box[1])
		if !strings.Contains(mid, "1,234") {
			t.Errorf("width %d dropped the count: %q", w, mid)
		}
		if !strings.HasSuffix(mid, "│") {
			t.Errorf("width %d lost its right border: %q", w, mid)
		}
	}
}

// The popup has to say what it's for, since it opens on the way to the search
// rather than being asked for.
func TestCategoryPopupSaysWhatItIsFor(t *testing.T) {
	m := catModel(t)
	out := stripANSI(m.viewCategories(m.contentW(), m.contentH()))
	for _, want := range []string{"What kind of part?", "pick one, then search inside it"} {
		if !strings.Contains(out, want) {
			t.Errorf("the popup never says %q:\n%s", want, out)
		}
	}
}

// The panel is a popup: it floats over the results rather than replacing them, so
// you can still see what you're narrowing.
func TestCategoryPanelFloatsOverTheResults(t *testing.T) {
	m := catModel(t)
	w, h := m.contentW(), m.contentH()

	closed := stripANSI(m.viewParts(w, h))
	open := stripANSI(m.viewCategories(w, h))
	if closed == open {
		t.Fatal("opening the popup changed nothing")
	}
	// the popup's own frame is there
	if !strings.Contains(open, "What kind of part?") {
		t.Error("the popup has no title")
	}
	// and so is the table behind it: its column header survives on both sides
	if !strings.Contains(open, "CODE") {
		t.Errorf("the results are gone — this is a replacement, not a popup:\n%s", open)
	}
	var pinned int
	for _, ln := range strings.Split(open, "\n") {
		if strings.Contains(ln, "│") && strings.Contains(ln, "░") {
			pinned++
		}
	}
	if pinned == 0 {
		t.Error("no shadowed rows — the popup isn't lifting off the page")
	}
}

// A popup is only as tall as its categories need, and never taller than the page.
func TestCategoryPopupFitsItsContent(t *testing.T) {
	small := catModel(t)
	one := []part.Part{catPart("C1", "Capacitors", "Tantalum Capacitors")}
	small.parts.results = one
	small.cat.cats = taxonomy.FromParts(one)
	small.w, small.h = 130, 40
	_, _, _, shortH := small.catGeom()

	big := catModel(t)
	big.w, big.h = 130, 40
	_, _, _, tallH := big.catGeom()
	if shortH >= tallH {
		t.Errorf("one category needs %d rows and four need %d — the popup isn't sizing to its content", shortH, tallH)
	}
	if _, y, _, ph := big.catGeom(); y+ph > big.contentH() {
		t.Errorf("the popup runs off the page: y=%d h=%d in %d", y, ph, big.contentH())
	}
}

// No line may exceed the panel width, and every line the popup lands on has to be
// exactly it — a row one column short or long moves the box's right edge and its
// shadow out of line with the rest.
func TestCategoryPopupWidthHolds(t *testing.T) {
	m := catModel(t)
	for _, size := range [][2]int{{80, 22}, {96, 26}, {130, 34}, {170, 44}, {60, 14}} {
		m.w, m.h = size[0], size[1]
		w, h := m.contentW(), m.contentH()
		_, y, _, ph := m.catGeom()
		lines := strings.Split(m.viewCategories(w, h), "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), h)
		}
		touched := 0
		for i, ln := range lines {
			got := lipgloss.Width(ln)
			if got > w {
				t.Errorf("%dx%d: line %d is %d columns, over the %d available", size[0], size[1], i, got, w)
			}
			if i >= y && i < y+ph {
				touched++
				if got != w {
					t.Errorf("%dx%d: popup line %d is %d columns, want exactly %d", size[0], size[1], i, got, w)
				}
			}
		}
		if touched == 0 {
			t.Errorf("%dx%d: the popup covered no rows", size[0], size[1])
		}
	}
}

// Grid rows that scroll out of the popup have to be counted on screen, or a short
// popup silently claims to show every category.
func TestCategoryFooterCountsWhatScrolledAway(t *testing.T) {
	if got := catFooter(5, 0, 5, 80); strings.Contains(got, "more") {
		t.Errorf("nothing is hidden but the footer says %q", got)
	}
	if got := catFooter(5, 0, 3, 80); !strings.Contains(got, "2↓ more") {
		t.Errorf("two rows below, footer = %q", got)
	}
	if got := catFooter(5, 1, 3, 80); !strings.Contains(got, "1↑") || !strings.Contains(got, "1↓") {
		t.Errorf("a row each way, footer = %q", got)
	}
	// the count outlives the key list when the popup is narrow
	for _, w := range []int{60, 48, 34, 20} {
		got := catFooter(9, 2, 3, w)
		if lipgloss.Width(got) > w {
			t.Errorf("width %d: footer is %d columns: %q", w, lipgloss.Width(got), got)
		}
		if !strings.Contains(got, "2↑") || !strings.Contains(got, "4↓") {
			t.Errorf("width %d dropped the offscreen count: %q", w, got)
		}
	}
}

// overlay has to leave the background showing on both sides of the box and keep
// the line width exact.
func TestOverlayKeepsWhatItDoesNotCover(t *testing.T) {
	bg := []string{
		strings.Repeat("a", 40),
		strings.Repeat("b", 40),
		strings.Repeat("c", 40),
	}
	out := overlay(bg, []string{"XXXX", "YYYY"}, 10, 1, 40)
	if len(out) != 3 {
		t.Fatalf("%d lines back, want 3", len(out))
	}
	if out[0] != bg[0] {
		t.Error("a row above the box should be untouched")
	}
	for i, ln := range out {
		if got := lipgloss.Width(ln); got != 40 {
			t.Errorf("line %d is %d columns, want 40", i, got)
		}
	}
	mid := stripANSI(out[1])
	if want := strings.Repeat("b", 10) + "XXXX" + strings.Repeat("b", 26); mid != want {
		t.Errorf("row 1 =\n  %q\nwant\n  %q", mid, want)
	}
	// a box wider than what's left gets cut rather than pushing the line out
	wide := overlay(bg, []string{strings.Repeat("Z", 50)}, 30, 0, 40)
	if got := lipgloss.Width(wide[0]); got != 40 {
		t.Errorf("an over-wide box gave a %d column line", got)
	}
}

// Narrowing the list and pressing enter must give you what you typed, not the box
// that clears the filter.
func TestNarrowingThenEnterPicksTheMatch(t *testing.T) {
	m := catModel(t)
	for _, r := range "usb" {
		mm, _ := m.updateCatKey(key(string(r)))
		m = mm.(Model)
	}
	mm, _ := m.updateCatKey(key("enter"))
	if got := mm.(Model).parts.cat; got != "USB Connectors" {
		t.Errorf("category = %q, want USB Connectors", got)
	}
}

// With nothing typed the cursor sits on the everything box, so enter is "search
// the lot" rather than whichever category happens to be first.
func TestEnterWithNoFilterSearchesEverything(t *testing.T) {
	m := catModel(t)
	m.parts.cat = "USB Connectors"
	mm, _ := m.updateCatKey(key("enter"))
	if got := mm.(Model).parts.cat; got != "" {
		t.Errorf("category = %q, want it cleared", got)
	}
}

// A category with no matching results has to say so, or the table just looks empty
// and broken.
func TestPartsSaysWhenTheCategoryMatchedNothing(t *testing.T) {
	m := catModel(t)
	mm, _ := m.updateCatKey(key("esc"))
	m = mm.(Model)
	m.parts.cat = "Ethernet Connectors"
	if got := len(m.parts.filtered()); got != 0 {
		t.Fatalf("expected the category to match nothing, got %d rows", got)
	}
	out := stripANSI(m.viewParts(m.contentW(), m.contentH()))
	if !strings.Contains(out, "none of the 14 results are in Ethernet Connectors") {
		t.Errorf("the empty result is unexplained:\n%s", out)
	}
	if !strings.Contains(out, "t to change the category") {
		t.Error("it should say how to get out of it")
	}
}

// Picking a category with nothing typed browses it, rather than leaving an empty
// table: the category's own name becomes the keyword.
func TestPickingACategoryWithNoQuerySearchesIt(t *testing.T) {
	m := catModel(t)
	m.parts.field.SetValue("")
	cells := m.catCells()
	var usb catCell
	for _, c := range cells {
		if c.label == "USB Connectors" {
			usb = c
		}
	}
	mm, cmd := m.applyCategory(usb)
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("picking a category with nothing typed should start a search")
	}
	if !m.parts.loading {
		t.Error("it should show as loading")
	}
	if m.parts.cat != "USB Connectors" {
		t.Errorf("category = %q", m.parts.cat)
	}
}

// Clearing the category with nothing typed has nothing to search, so what's on
// screen stays instead of being wiped.
func TestClearingACategoryWithNoQueryKeepsTheResults(t *testing.T) {
	m := catModel(t)
	m.parts.field.SetValue("")
	m.parts.cat = "USB Connectors"
	before := len(m.parts.results)
	mm, cmd := m.applyCategory(catCell{label: "all categories", all: true})
	m = mm.(Model)
	if cmd != nil {
		t.Error("there is nothing to search, so no command should go out")
	}
	if len(m.parts.results) != before {
		t.Errorf("%d results left of %d — clearing the category wiped them", len(m.parts.results), before)
	}
}
