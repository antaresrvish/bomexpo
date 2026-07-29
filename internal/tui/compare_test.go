package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
)

func cmpFixture() []part.Part {
	return []part.Part{
		{
			Source: "jlcpcb", Code: "C8734", MPN: "STM32F103C8T6", Brand: "STMicro",
			Package: "LQFP-48", Stock: 1240, MinBuy: 1, Lib: part.LibBasic, AsmMin: 20, Loss: 10,
			Prices: []part.Price{{Ladder: 1, USD: 1.82}},
			Params: []part.Param{
				{Name: "Core", Value: "Cortex-M3"},
				{Name: "Flash", Value: "64KB"},
				{Name: "RAM", Value: "20KB"},
			},
		},
		{
			Source: "jlcpcb", Code: "C8304", MPN: "STM32F103CBT6", Brand: "STMicro",
			Package: "LQFP-48", Stock: 320, MinBuy: 1, Lib: part.LibExtended,
			Prices: []part.Price{{Ladder: 1, USD: 2.10}},
			Params: []part.Param{
				{Name: "Core", Value: "Cortex-M3"},
				{Name: "Flash", Value: "128KB"},
				{Name: "RAM", Value: "20KB"},
				{Name: "Package Height", Value: "1.4mm"}, // only this part reports it
			},
		},
	}
}

func rowByLabel(rows []cmpRow, label string) (cmpRow, bool) {
	for _, r := range rows {
		if r.label == label {
			return r, true
		}
	}
	return cmpRow{}, false
}

func TestCompareRowsMarkDifferences(t *testing.T) {
	rows := compareRows(cmpFixture())

	for _, c := range []struct {
		label  string
		differ bool
	}{
		{"source", false},
		{"brand", false},
		{"package", false},
		{"core", false}, // both Cortex-M3
		{"ram", false},
		{"mpn", true},
		{"library", true},
		{"flash", true}, // 64KB vs 128KB — the whole reason to compare
	} {
		r, ok := rowByLabel(rows, c.label)
		if !ok {
			t.Errorf("no %q row", c.label)
			continue
		}
		if r.differ != c.differ {
			t.Errorf("%s: differ = %v, want %v (vals %v)", c.label, r.differ, c.differ, r.vals)
		}
	}
}

func TestCompareRowsPickBest(t *testing.T) {
	rows := compareRows(cmpFixture())

	stock, _ := rowByLabel(rows, "stock")
	if stock.best != 0 {
		t.Errorf("best stock = column %d, want 0 (1240 beats 320)", stock.best)
	}
	price, _ := rowByLabel(rows, "unit price")
	if price.best != 0 {
		t.Errorf("best price = column %d, want 0 ($1.82 beats $2.10)", price.best)
	}
	// rows with no ordering must not claim a winner
	for _, label := range []string{"mpn", "brand", "package", "library", "flash"} {
		r, _ := rowByLabel(rows, label)
		if r.best != -1 {
			t.Errorf("%s claims best = %d, want -1", label, r.best)
		}
	}
}

func TestCompareSharedParamsComeFirst(t *testing.T) {
	rows := compareRows(cmpFixture())

	var order []string
	for _, r := range rows {
		if r.rule {
			order = nil // params start after the separator
			continue
		}
		if order != nil || r.label == "core" {
			order = append(order, r.label)
		}
	}
	// core/flash/ram are reported by both; package height by only one
	want := "core,flash,ram,package height"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("param order = %q, want %q", got, want)
	}
}

func TestCompareMissingParamShowsDash(t *testing.T) {
	rows := compareRows(cmpFixture())
	r, ok := rowByLabel(rows, "package height")
	if !ok {
		t.Fatal("no package height row")
	}
	if r.vals[0] != "—" || r.vals[1] != "1.4mm" {
		t.Errorf("vals = %v, want [— 1.4mm]", r.vals)
	}
	if !r.differ {
		t.Error("a param only one part reports counts as a difference")
	}
}

func TestCompareAssemblyRowOnlyWhenReported(t *testing.T) {
	if _, ok := rowByLabel(compareRows(cmpFixture()), "asm min"); !ok {
		t.Error("jlcpcb parts report assembly numbers, so the row should exist")
	}
	shop := []part.Part{{Source: "lcsc", Code: "C1"}, {Source: "lcsc", Code: "C2"}}
	if _, ok := rowByLabel(compareRows(shop), "asm min"); ok {
		t.Error("no assembly data means no assembly row")
	}
}

func TestArgBestNeedsTwoValues(t *testing.T) {
	one := []part.Part{
		{Prices: []part.Price{{Ladder: 1, USD: 1}}},
		{}, // no price at all
	}
	got := argBest(one, func(p part.Part) (float64, bool) { return p.UnitPrice() }, false)
	if got != -1 {
		t.Errorf("argBest with one reported value = %d, want -1", got)
	}
}

func TestCompareLayoutPagesWhenNarrow(t *testing.T) {
	m := New(Options{})
	m.parts.pinned = []part.Part{{Code: "A"}, {Code: "B"}, {Code: "C"}, {Code: "D"}}

	// wide: everything on one page, and the cards share the full width
	colW, perPage, first := m.compareLayout(136)
	if perPage != 4 || first != 0 {
		t.Errorf("wide layout = perPage %d first %d, want 4/0", perPage, first)
	}
	if colW != 136/4 {
		t.Errorf("colW = %d, want %d", colW, 136/4)
	}

	// narrow: two per page, and the window follows the focused card
	m.compare.sel = 3
	_, perPage, first = m.compareLayout(2 * cmpMinColW)
	if perPage != 2 {
		t.Fatalf("narrow perPage = %d, want 2", perPage)
	}
	if first != 2 {
		t.Errorf("first = %d, want 2 so the focused card 3 is visible", first)
	}
	m.compare.sel = 0
	if _, _, first = m.compareLayout(2 * cmpMinColW); first != 0 {
		t.Errorf("first = %d, want 0 for a focused card 0", first)
	}
}

func TestColFirstSlidesMinimally(t *testing.T) {
	for _, c := range []struct{ first, sel, perPage, n, want int }{
		{0, 0, 2, 3, 0},
		{0, 1, 2, 3, 0}, // already visible, don't move
		{0, 2, 2, 3, 1}, // slide one to bring 2 in
		{1, 0, 2, 3, 0}, // slide back
		{1, 2, 2, 3, 1}, // still visible
		{5, 0, 2, 3, 0}, // a stale first is clamped
		{0, 3, 2, 4, 2},
		{0, 0, 4, 3, 0}, // everything fits, no window
		{2, 1, 4, 3, 0},
	} {
		if got := colFirst(c.first, c.sel, c.perPage, c.n); got != c.want {
			t.Errorf("colFirst(first=%d sel=%d perPage=%d n=%d) = %d, want %d",
				c.first, c.sel, c.perPage, c.n, got, c.want)
		}
	}
}

// A full page must never be left half-blank while there are columns off-screen.
func TestComparePageAlwaysFull(t *testing.T) {
	m := New(Options{})
	m.parts.pinned = []part.Part{{Code: "A"}, {Code: "B"}, {Code: "C"}}
	w := 2 * cmpMinColW
	for sel := 0; sel < 3; sel++ {
		m.compare.sel = sel
		_, perPage, first := m.compareLayout(w)
		m.compare.first = first
		if first+perPage > len(m.parts.pinned) {
			t.Errorf("sel %d: window %d..%d runs past %d columns", sel, first, first+perPage, len(m.parts.pinned))
		}
		if sel < first || sel >= first+perPage {
			t.Errorf("sel %d is outside the visible window %d..%d", sel, first, first+perPage)
		}
	}
}

func TestUnpinFromCompareLeavesWhenTooFew(t *testing.T) {
	m := New(Options{})
	m.w, m.h = 140, 40
	m.mode = modeCompare
	m.parts.pinned = cmpFixture()
	m.compare.sel = 1

	mm, _ := m.unpinSelected()
	m = mm.(Model)
	if len(m.parts.pinned) != 1 {
		t.Fatalf("pinned = %d, want 1", len(m.parts.pinned))
	}
	if m.mode != modeParts {
		t.Error("with one part left there is nothing to compare — should fall back to Parts")
	}
	if m.compare.sel != 0 {
		t.Errorf("sel = %d, want it clamped to 0", m.compare.sel)
	}
}

// TestCompareColumnGeometry is the guard that the clickable card spans line up
// with where each card's frame actually renders.
func TestCompareColumnGeometry(t *testing.T) {
	m := New(Options{})
	m.w, m.h = 140, 40
	m.mode = modeCompare
	m.parts.pinned = cmpFixture()

	w, h := m.contentW(), m.contentH()
	colW, _, first := m.compareLayout(w)
	lines := strings.Split(stripANSI(m.viewCompare(w, h)), "\n")

	// find the row holding the card frames
	var frame string
	for _, ln := range lines {
		if strings.Contains(ln, "╭") {
			frame = ln
			break
		}
	}
	if frame == "" {
		t.Fatalf("no card frame row rendered:\n%s", strings.Join(lines, "\n"))
	}
	for i, p := range m.parts.pinned {
		b := strings.Index(frame, p.Code)
		if b < 0 {
			t.Fatalf("%s missing from the frame row: %q", p.Code, frame)
		}
		// box characters are multi-byte, so measure the prefix in columns
		at := lipgloss.Width(frame[:b])
		// "╭ " precedes the code inside each card
		if want := (i-first)*colW + 2; at != want {
			t.Errorf("%s renders at column %d, but the card span says %d", p.Code, at, want)
		}
	}
}

func TestViewCompareSplitsCommonFromDifferent(t *testing.T) {
	m := New(Options{})
	m.w, m.h = 140, 40
	m.mode = modeCompare
	m.parts.pinned = cmpFixture()

	out := stripANSI(m.viewCompare(m.contentW(), m.contentH()))
	lines := strings.Split(out, "\n")

	// the top pane collects what they agree on
	if !strings.Contains(out, "In common") {
		t.Error("want a shared-facts pane")
	}
	commonAt, cardAt := -1, -1
	for i, ln := range lines {
		if commonAt < 0 && strings.Contains(ln, "In common") {
			commonAt = i
		}
		if cardAt < 0 && strings.Contains(ln, "╭") {
			cardAt = i
		}
	}
	if commonAt < 0 || cardAt < 0 || commonAt >= cardAt {
		t.Errorf("the shared pane (row %d) should sit above the cards (row %d)", commonAt, cardAt)
	}
	// package and brand match across the fixture, so they belong up top
	top := strings.Join(lines[:cardAt], "\n")
	if !strings.Contains(top, "brand") || !strings.Contains(top, "package") {
		t.Errorf("brand and package agree, so they belong in the shared pane:\n%s", top)
	}

	// the differing values appear in the cards, with a winner marked
	cards := strings.Join(lines[cardAt:], "\n")
	for _, want := range []string{"64KB", "128KB", "▴"} {
		if !strings.Contains(cards, want) {
			t.Errorf("%q missing from the cards:\n%s", want, cards)
		}
	}
	if !strings.Contains(out, "best of these") {
		t.Error("the legend should explain the ▴ marker")
	}
}

func TestViewCompareRefusesWithOnePart(t *testing.T) {
	m := New(Options{})
	m.w, m.h = 140, 40
	m.parts.pinned = cmpFixture()[:1]
	out := stripANSI(m.viewCompare(m.contentW(), m.contentH()))
	if !strings.Contains(out, "at least two") {
		t.Errorf("want an explanation, got %q", out)
	}
}
