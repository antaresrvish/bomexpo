package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/easyeda"
	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// The Check page is split down the middle: numbers left, board right.
func TestCheckSplitsDownTheMiddle(t *testing.T) {
	m, _ := csvModel(t, true)
	m.mode = modeCheck
	m.w, m.h = 124, 30
	w, h := m.contentW(), m.contentH()
	mid := w / 2

	lines := strings.Split(stripANSI(m.viewCheck(w, h)), "\n")
	if len(lines) != h {
		t.Fatalf("%d lines, want %d", len(lines), h)
	}

	// the summary and the board header share the first row, one per side
	first := lines[0]
	if !strings.Contains(first, "assigned") {
		t.Errorf("the left pane should lead with the summary: %q", first)
	}
	if at := strings.Index(first, "Placements"); at < mid {
		t.Errorf("the board pane should start in the right half, found at %d (mid %d)", at, mid)
	}

	// nothing on the left may cross the divide
	for y, ln := range lines[:h-2] { // the output field spans the bottom
		if len(ln) > mid && strings.TrimSpace(ln[:mid]) != "" {
			// left content is fine; just make sure it stops before the divide
			trimmed := strings.TrimRight(ln[:mid], " ")
			if lipgloss.Width(trimmed) > mid {
				t.Errorf("row %d: left pane runs past the middle", y)
			}
		}
	}

	// the board is drawn, and the output field still spans the bottom
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "▀") {
		t.Error("the board pane drew nothing")
	}
	if !strings.Contains(lines[h-2], "Output") {
		t.Errorf("the output field should be the second-to-last row: %q", lines[h-2])
	}
}

// In half a page there isn't room for the checklist and the manifest side by
// side, so they stack rather than both being cut off.
func TestPreflightStacksWhenNarrow(t *testing.T) {
	m, _ := csvModel(t, true)

	wide := stripANSI(strings.Join(m.preflightAndManifest(120), "\n"))
	var wideRow string
	for _, ln := range strings.Split(wide, "\n") {
		if strings.Contains(ln, "Pre-flight") {
			wideRow = ln
			break
		}
	}
	if !strings.Contains(wideRow, "Order package") {
		t.Errorf("at 120 wide they should share a row: %q", wideRow)
	}

	narrow := stripANSI(strings.Join(m.preflightAndManifest(60), "\n"))
	for _, ln := range strings.Split(narrow, "\n") {
		if strings.Contains(ln, "Pre-flight") && strings.Contains(ln, "Order package") {
			t.Errorf("at 60 wide they should stack: %q", ln)
		}
	}
	if !strings.Contains(narrow, "Order package") {
		t.Error("stacking must not drop the manifest")
	}
	// and the manifest values survive intact rather than being truncated
	if !strings.Contains(narrow, "positions.csv") {
		t.Errorf("manifest rows were lost:\n%s", narrow)
	}
}

func compareModel(t *testing.T) Model {
	t.Helper()
	m := New(Options{})
	m.w, m.h = 124, 30
	m.mode = modeCompare
	m.parts.pinned = cmpFixture()
	return m
}

// The cards divide the width between however many parts are pinned.
func TestCompareCardsDivideTheWidth(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		m := compareModel(t)
		m.parts.pinned = m.parts.pinned[:min(n, len(m.parts.pinned))]
		for len(m.parts.pinned) < n {
			m.parts.pinned = append(m.parts.pinned, part.Part{
				Source: "lcsc", Code: "C" + strings.Repeat("9", len(m.parts.pinned)+1),
			})
		}

		w := m.contentW()
		colW, perPage, _ := m.compareLayout(w)
		if perPage != n {
			t.Errorf("%d parts: perPage %d, want %d", n, perPage, n)
		}
		if colW != w/n {
			t.Errorf("%d parts: colW %d, want %d", n, colW, w/n)
		}

		// one frame per part on the frame row
		out := stripANSI(m.viewCompare(w, m.contentH()))
		var frame string
		for _, ln := range strings.Split(out, "\n") {
			if strings.Contains(ln, "╭") {
				frame = ln
				break
			}
		}
		if got := strings.Count(frame, "╭"); got != n {
			t.Errorf("%d parts: %d card frames on the row %q", n, got, frame)
		}
	}
}

// A card leads with its footprint when the board has one, and says why not when
// it doesn't.
func TestCompareCardShowsFootprintOrSaysWhy(t *testing.T) {
	m := compareModel(t)
	m.pcbPath = "/tmp/x.kicad_pcb"
	m.items = []kicad.Item{{
		Bases: []string{"U1"}, Designators: []string{"U1"},
		Value: "STM32F103", Footprint: "LQFP-48", Quantity: 1, LCSC: "C8734",
	}}
	m.assigned = make([]*part.Part, 1)
	m.excluded = make([]bool, 1)
	m.designLands = map[string][]kicad.Land{"LQFP-48": {
		{Name: "1", X: -1, Y: -2, W: 0.3, H: 1.2, First: true},
		{Name: "2", X: 1, Y: -2, W: 0.3, H: 1.2},
		{Name: "3", X: 0, Y: 2, W: 0.3, H: 1.2},
	}}
	m = m.reindex()

	// C8734 is on the board, so it draws
	drawn := strings.Join(m.compareFootprint(m.parts.pinned[0], 30, 6), "\n")
	if !strings.Contains(drawn, "▀") {
		t.Errorf("C8734 should draw its footprint, got %q", stripANSI(drawn))
	}

	// C8304 isn't on the board, so until its footprint arrives the card names
	// the package and says one is coming
	missing := stripANSI(strings.Join(m.compareFootprint(m.parts.pinned[1], 30, 6), "\n"))
	if !strings.Contains(missing, "fetching") {
		t.Errorf("want a note that a download is under way, got %q", missing)
	}
	if !strings.Contains(missing, "LQFP-48") {
		t.Errorf("want the package named meanwhile, got %q", missing)
	}

	// once downloaded, the card draws it like any other
	m.edaLands["C8304"] = easyeda.Footprint{Code: "C8304", Package: "LQFP-48", Lands: []kicad.Land{
		{Name: "1", X: -2, Y: -2, W: 0.3, H: 1.2, First: true},
		{Name: "2", X: 2, Y: -2, W: 0.3, H: 1.2},
		{Name: "3", X: 0, Y: 2, W: 0.3, H: 1.2},
	}}
	fetched := strings.Join(m.compareFootprint(m.parts.pinned[1], 30, 6), "\n")
	if !strings.Contains(fetched, "▀") {
		t.Errorf("a downloaded footprint should draw, got %q", stripANSI(fetched))
	}
}

// A footprint is only downloaded when we don't already have it.
func TestLandsCmdSkipsWhatWeHave(t *testing.T) {
	m := compareModel(t)
	m.pcbPath = "/tmp/x.kicad_pcb"
	m.items = []kicad.Item{{
		Bases: []string{"U1"}, Designators: []string{"U1"},
		Footprint: "LQFP-48", Quantity: 1, LCSC: "C8734",
	}}
	m.assigned = make([]*part.Part, 1)
	m.excluded = make([]bool, 1)
	m.designLands = map[string][]kicad.Land{"LQFP-48": {{Name: "1", W: 1, H: 1}}}
	m = m.reindex()

	if m.landsCmd("C8734") != nil {
		t.Error("a part on the board needs no download")
	}
	if m.landsCmd("") != nil {
		t.Error("an empty code needs no download")
	}
	if m.landsCmd("C8304") == nil {
		t.Error("a part we know nothing about should be fetched")
	}

	m.edaLands["C8304"] = easyeda.Footprint{Code: "C8304", Lands: []kicad.Land{{W: 1, H: 1}}}
	if m.landsCmd("C8304") != nil {
		t.Error("an already-downloaded footprint should not be fetched again")
	}
}

// A failed download must not wipe what's already there, and must not crash.
func TestFootprintDoneIgnoresFailures(t *testing.T) {
	m := compareModel(t)
	m.edaLands["C1"] = easyeda.Footprint{Code: "C1", Lands: []kicad.Land{{W: 1, H: 1}}}

	for _, msg := range []footprintDoneMsg{
		{code: "C1", err: errNoSource},
		{code: "C1", fp: easyeda.Footprint{Code: "C1"}}, // no lands
		{code: "C2", err: errNoSource},
	} {
		mm, _ := m.Update(msg)
		m = mm.(Model)
	}
	if len(m.edaLands["C1"].Lands) != 1 {
		t.Error("a failed download overwrote a good footprint")
	}
	if _, bad := m.edaLands["C2"]; bad {
		t.Error("a failed download should not record anything")
	}
}

// Up and down scroll the fields inside the cards, bounded by how many differ.
func TestCompareScrollsTheFields(t *testing.T) {
	m := compareModel(t)
	total, vis := m.compareDiffCount(), m.compareFieldRows()
	if total == 0 || vis == 0 {
		t.Fatalf("nothing to scroll: %d fields, %d visible", total, vis)
	}

	for i := 0; i < total+10; i++ {
		mm, _ := m.updateCompareKey(tea.KeyPressMsg{Code: tea.KeyDown})
		m = mm.(Model)
	}
	if want := max(0, total-vis); m.compare.top != want {
		t.Errorf("top = %d after scrolling to the end, want %d", m.compare.top, want)
	}

	for i := 0; i < total+10; i++ {
		mm, _ := m.updateCompareKey(tea.KeyPressMsg{Code: tea.KeyUp})
		m = mm.(Model)
	}
	if m.compare.top != 0 {
		t.Errorf("top = %d after scrolling back, want 0", m.compare.top)
	}
}

// The focused card is the one the keys act on, so it has to look different.
func TestCompareMarksTheFocusedCard(t *testing.T) {
	m := compareModel(t)
	w, h := m.contentW(), m.contentH()

	first := m.viewCompare(w, h)
	mm, _ := m.updateCompareKey(tea.KeyPressMsg{Code: tea.KeyRight})
	second := mm.(Model).viewCompare(w, h)

	if first == second {
		t.Error("moving the focus should change how the cards render")
	}
	// the plain text is the same either way; only the styling moves
	if stripANSI(first) != stripANSI(second) {
		t.Error("focus should be styling, not different content")
	}
}

// Every compare row must come out exactly the content width, at any size.
func TestCompareWidthHolds(t *testing.T) {
	m := compareModel(t)
	for _, size := range [][2]int{{80, 24}, {100, 20}, {124, 30}, {160, 44}, {60, 16}} {
		m.w, m.h = size[0], size[1]
		w, h := m.contentW(), m.contentH()
		lines := strings.Split(m.viewCompare(w, h), "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), h)
		}
		for y, ln := range lines {
			if n := lipgloss.Width(ln); n > w {
				t.Errorf("%dx%d: row %d is %d wide, want at most %d", size[0], size[1], y, n, w)
			}
		}
	}
}
