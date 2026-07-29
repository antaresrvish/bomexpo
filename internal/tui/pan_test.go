package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/render"
)

// panBoard gives the renderer a coordinate range and nothing to draw. The only
// ink on screen is then the one placement, so where it lands is a clean read of
// which way the view moved — an outline would be clipped asymmetrically at zoom
// and drag the measurement around with it.
func panBoard() *kicad.Board {
	return &kicad.Board{
		Min: kicad.Point{X: 0, Y: 0},
		Max: kicad.Point{X: 100, Y: 100},
	}
}

// markAt finds the drawn mark's centre of mass, in columns and rows.
func markAt(t *testing.T, b boardState) (col, row float64) {
	t.Helper()
	img := render.Render(panBoard(), []kicad.Placement{{Designator: "U1", X: 50, Y: 50}},
		render.Options{W: 60, H: 20, Zoom: b.zoom, PanX: b.panX, PanY: b.panY})
	var sx, sy, n float64
	for y, line := range strings.Split(img, "\n") {
		plain := stripANSI(line)
		for x, r := range []rune(plain) {
			if r != ' ' {
				sx, sy, n = sx+float64(x), sy+float64(y), n+1
			}
		}
	}
	if n == 0 {
		t.Fatal("nothing drawn")
	}
	return sx / n, sy / n
}

// The arrows move the view, not the board: pressing right shows what's to the
// right, which walks the drawing left. Getting this backwards is easy — the
// renderer adds the pan to every point it draws.
func TestPanFollowsTheArrowDirection(t *testing.T) {
	zoomed := newBoardState().zoomBy(zoomStep).zoomBy(zoomStep)
	col0, row0 := markAt(t, zoomed)

	right := zoomed.panBy(1, 0)
	if col, _ := markAt(t, right); col >= col0 {
		t.Errorf("pressing right moved the drawing to column %.1f from %.1f — "+
			"it should slide left so you see what's further right", col, col0)
	}
	if left := zoomed.panBy(-1, 0); func() bool { c, _ := markAt(t, left); return c <= col0 }() {
		t.Error("pressing left should slide the drawing right")
	}

	down := zoomed.panBy(0, 1)
	if _, row := markAt(t, down); row >= row0 {
		t.Errorf("pressing down moved the drawing to row %.1f from %.1f — "+
			"it should slide up so you see what's further down", row, row0)
	}
	if up := zoomed.panBy(0, -1); func() bool { _, r := markAt(t, up); return r <= row0 }() {
		t.Error("pressing up should slide the drawing down")
	}
}

// Panning at the fit-everything zoom would only push the board off screen, so it
// does nothing until you've zoomed in.
func TestPanDoesNothingUntilZoomed(t *testing.T) {
	flat := newBoardState()
	if got := flat.panBy(1, 1); got != flat {
		t.Errorf("panning at zoom %.1f changed the view", flat.zoom)
	}
}

// Whichever way you panned, zooming back out has to give the whole board back.
func TestZoomingOutClearsThePan(t *testing.T) {
	b := newBoardState().zoomBy(zoomStep).panBy(3, -2)
	if b.panX == 0 && b.panY == 0 {
		t.Fatal("the pan did not take")
	}
	if out := b.zoomBy(1 / zoomStep); out.panX != 0 || out.panY != 0 {
		t.Errorf("zooming out left a pan of %.1f,%.1f", out.panX, out.panY)
	}
}

// The direction tests are only worth anything if the mark is really on screen and
// really moves, so check that before trusting them.
func TestPanFixtureDrawsAndMoves(t *testing.T) {
	zoomed := newBoardState().zoomBy(zoomStep).zoomBy(zoomStep)
	col, row := markAt(t, zoomed)
	if col <= 0 || row <= 0 {
		t.Fatalf("mark sits at %.1f,%.1f — the fixture draws nothing usable", col, row)
	}
	moved, _ := markAt(t, zoomed.panBy(1, 0))
	if d := col - moved; d < 2 {
		t.Fatalf("one pan step moved the mark %.1f columns — too little to measure", d)
	}
	if w := lipgloss.Width(stripANSI(strings.Split(
		render.Render(panBoard(), nil, render.Options{W: 40, H: 12, Zoom: 1}), "\n")[0])); w != 40 {
		t.Errorf("a render row came out %d columns wide, want 40", w)
	}
}
