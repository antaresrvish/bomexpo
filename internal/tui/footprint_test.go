package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

// fpModel is filterModel with pad geometry for two of its footprints.
func fpModel(t *testing.T) Model {
	t.Helper()
	m := filterModel(t)
	m.pcbPath = "/tmp/x.kicad_pcb"
	m.designLands = map[string][]kicad.Land{
		"C_0402_1005Metric": {
			{Name: "1", X: -0.48, W: 0.55, H: 0.65, First: true, Net: "GND"},
			{Name: "2", X: 0.48, W: 0.55, H: 0.65, Net: "+3V3"},
		},
		"LED_0603": {
			{Name: "1", X: -0.75, W: 0.8, H: 0.9, First: true, Net: "GND"},
			{Name: "2", X: 0.75, W: 0.8, H: 0.9, Net: "GND"},
		},
	}
	return m
}

func TestMiniFootprintFollowsTheSelection(t *testing.T) {
	m := fpModel(t)

	m.cursor = 0 // C1, which has geometry
	if got := strings.Join(m.miniFootprint(44, 8), "\n"); !strings.Contains(got, "▀") {
		t.Errorf("C1 should draw its pads, got %q", stripANSI(got))
	}

	m.cursor = 2 // R1, which has none
	if got := stripANSI(strings.Join(m.miniFootprint(44, 8), "\n")); !strings.Contains(got, "no pads") {
		t.Errorf("a footprint with no geometry should say so, got %q", got)
	}
}

func TestMiniFootprintExplainsCSVDesigns(t *testing.T) {
	m := filterModel(t) // no pcb, no lands
	if got := stripANSI(strings.Join(m.miniFootprint(44, 8), "\n")); !strings.Contains(got, "bom csv") {
		t.Errorf("a CSV design should say why there's nothing to draw, got %q", got)
	}
}

func TestFootprintHeaderIsTwoLines(t *testing.T) {
	m := fpModel(t)
	m.cursor = 0

	head := m.footprintHeader(46)
	if len(head) != 2 {
		t.Fatalf("header has %d lines, want 2", len(head))
	}
	if !strings.Contains(stripANSI(head[0]), "C_0402_1005Metric") {
		t.Errorf("first line should name the footprint: %q", stripANSI(head[0]))
	}
	// the second line carries the facts that wouldn't fit beside the name
	second := stripANSI(head[1])
	for _, want := range []string{"2 pads", "mm", "°"} {
		if !strings.Contains(second, want) {
			t.Errorf("%q missing from %q", want, second)
		}
	}
	// nothing selected still gives two lines, so the layout can't shift
	empty := Model{}
	if got := len(empty.footprintHeader(46)); got != 2 {
		t.Errorf("header with no selection has %d lines, want 2", got)
	}
}

// The sidebar must return exactly the height it was asked for. Getting this
// wrong panics viewTable, which is how the two-line header first broke.
func TestSidebarBlockIsExactlyTheRequestedHeight(t *testing.T) {
	m := fpModel(t)
	for _, h := range []int{8, 12, 20, 26, 40} {
		for _, sideW := range []int{30, 40, 48} {
			if got := len(m.sidebarBlock(sideW, h)); got != h {
				t.Errorf("sidebarBlock(%d, %d) returned %d lines", sideW, h, got)
			}
		}
	}
}

// And so must the whole view, at any terminal size the panel will hand it.
func TestViewTableHeightHolds(t *testing.T) {
	m := fpModel(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {124, 28}, {160, 50}, {90, 12}} {
		m.w, m.h = size[0], size[1]
		m.clampScroll()
		h := m.contentH()
		if got := len(strings.Split(m.viewTable(m.contentW(), h), "\n")); got != h {
			t.Errorf("%dx%d: viewTable gave %d lines, want %d", size[0], size[1], got, h)
		}
	}
}

// The whole-board drawing moved to the Check page, where there's room for it.
func TestCheckPageCarriesTheBoard(t *testing.T) {
	m, _ := csvModel(t, true) // a CSV design with placements to draw
	m.mode = modeCheck
	out := stripANSI(m.viewCheck(m.contentW(), m.contentH()))

	if !strings.Contains(out, "Volume pricing") {
		t.Error("the pricing table should still be there")
	}
	if !strings.Contains(out, "▀") {
		t.Error("the board pane should have drawn something")
	}

	// the two panes share every row: the pricing header on the left, the board
	// pane's caption on the right
	var row string
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "Volume pricing") {
			row = ln
			break
		}
	}
	if row == "" {
		t.Fatal("no pricing row found")
	}
	// the left half must not run past the middle
	if mid := m.contentW() / 2; strings.Index(row, "Volume pricing") >= mid {
		t.Errorf("the pricing block should be in the left half: %q", row)
	}

	// the board pane is titled for what it's actually drawing
	pane := stripANSI(strings.Join(m.boardPane(40, 12), "\n"))
	if !strings.Contains(pane, "Placements") {
		t.Errorf("a CSV design's pane = %q, want it to say Placements", pane)
	}
	if !strings.Contains(pane, "zoom") {
		t.Errorf("the pane should say how to move the view: %q", pane)
	}
}

// The Components sidebar shows the footprint now, so it must not still offer
// board render buttons there.
func TestComponentsSidebarHasNoBoardButtons(t *testing.T) {
	m := fpModel(t)
	m.w, m.h = 140, 30
	out := stripANSI(m.viewTable(m.contentW(), m.contentH()))
	if strings.Contains(out, "[t]op") {
		t.Error("the render buttons belong to the Check board, not the Components sidebar")
	}
	if !strings.Contains(out, "Footprint") {
		t.Error("the sidebar should be showing the footprint")
	}
}
