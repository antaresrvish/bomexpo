package render

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

// squareBoard is a 40×40mm outline with a couple of parts on it.
func squareBoard(w, h float64) (*kicad.Board, []kicad.Placement) {
	b := &kicad.Board{
		Min: kicad.Point{X: 0, Y: 0}, Max: kicad.Point{X: w, Y: h},
		Outline: []kicad.Segment{
			{A: kicad.Point{X: 0, Y: 0}, B: kicad.Point{X: w, Y: 0}},
			{A: kicad.Point{X: w, Y: 0}, B: kicad.Point{X: w, Y: h}},
			{A: kicad.Point{X: w, Y: h}, B: kicad.Point{X: 0, Y: h}},
			{A: kicad.Point{X: 0, Y: h}, B: kicad.Point{X: 0, Y: 0}},
		},
	}
	pl := []kicad.Placement{
		{Designator: "C1", X: w / 4, Y: h / 2, Layer: "top", BodyW: 1, BodyH: 0.6},
		{Designator: "U1", X: w * 3 / 4, Y: h / 2, Layer: "top", BodyW: 7, BodyH: 7},
	}
	return b, pl
}

func TestCellsGridMatchesTheSize(t *testing.T) {
	b, pl := squareBoard(40, 30)
	for _, size := range [][2]int{{40, 10}, {100, 24}, {132, 30}} {
		grid := Cells(b, pl, Options{W: size[0], H: size[1], Dim: true})
		if len(grid) != size[1] {
			t.Fatalf("%dx%d gave %d rows", size[0], size[1], len(grid))
		}
		for y, row := range grid {
			if len(row) != size[0] {
				t.Errorf("%dx%d row %d has %d cells", size[0], size[1], y, len(row))
			}
			for x, c := range row {
				if shape(c) == "" {
					t.Errorf("row %d cell %d is empty; want at least a space", y, x)
				}
				if n := len([]rune(shape(c))); n != 1 {
					t.Errorf("row %d cell %d holds %d runes, want 1", y, x, n)
				}
			}
		}
	}
}

func TestCellsEmptyBoard(t *testing.T) {
	if got := Cells(&kicad.Board{}, nil, Options{W: 40, H: 10}); got != nil {
		t.Error("an empty board should give no cells")
	}
}

// A dim board is the same drawing in different tones — same silhouette, muted
// colours.
func TestDimIsSameShapeDifferentColour(t *testing.T) {
	b, pl := squareBoard(40, 30)
	bright := Render(b, pl, Options{W: 60, H: 16})
	dim := Render(b, pl, Options{W: 60, H: 16, Dim: true})

	if bright == dim {
		t.Fatal("Dim should change the colours")
	}
	if !strings.Contains(dim, "▀") {
		t.Error("the dim drawing lost its blocks")
	}
}

// A backdrop stretches to reach both edges, but only so far: a long thin board
// must not come out looking square.
func TestBackdropStretchIsCapped(t *testing.T) {
	// 100×10mm is 10:1; the canvas below is about 2:1
	b, pl := squareBoard(100, 10)
	const w, h = 60, 16
	grid := Cells(b, pl, Options{W: w, H: h, Dim: true})
	if grid == nil {
		t.Fatal("nothing drawn")
	}

	rowsWithInk := 0
	for _, row := range grid {
		for _, c := range row {
			if shape(c) != " " {
				rowsWithInk++
				break
			}
		}
	}
	// stretched to fill it would use every row; capped, it must not
	if rowsWithInk >= h {
		t.Errorf("a 10:1 board filled all %d rows — the stretch isn't capped", h)
	}
	if rowsWithInk < 2 {
		t.Errorf("only %d rows have ink; the board vanished", rowsWithInk)
	}
}

// A board close to the panel's own proportions should reach both edges.
func TestBackdropFillsWhenItCan(t *testing.T) {
	b, pl := squareBoard(40, 30)
	const w, h = 60, 16
	grid := Cells(b, pl, Options{W: w, H: h, Dim: true})
	if grid == nil {
		t.Fatal("nothing drawn")
	}

	inked := func(x int) bool {
		for _, row := range grid {
			if x < len(row) && shape(row[x]) != " " {
				return true
			}
		}
		return false
	}
	if !inked(0) || !inked(w-1) {
		t.Errorf("the backdrop should reach both edges: left %v right %v", inked(0), inked(w-1))
	}
}
