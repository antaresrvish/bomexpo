package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// bgRow is a backdrop row of distinguishable cells.
func bgRow(w int) []string {
	row := make([]string, w)
	for i := range row {
		row[i] = "▀"
	}
	return row
}

func TestOverlayKeepsTextAndFillsRealGaps(t *testing.T) {
	const w = 40
	bg := [][]string{bgRow(w)}

	// two words a single space apart: the gap is too small to open up, or the
	// words stop being readable
	got := stripANSI(overlayLine("hello world", bg[0], w))
	if !strings.HasPrefix(got, "hello world") {
		t.Errorf("text mangled: %q", got)
	}
	if strings.Contains(got[:11], "▀") {
		t.Errorf("the space between words should stay blank: %q", got[:11])
	}
	// past the text there's room, so the backdrop comes through
	if !strings.Contains(got, "▀") {
		t.Errorf("the backdrop should fill the rest: %q", got)
	}
}

func TestOverlayKeepsAMarginAroundText(t *testing.T) {
	const w = 20
	got := stripANSI(overlayLine("ab", bgRow(w), w))
	// a clear cell after the last glyph, then backdrop
	if !strings.HasPrefix(got, "ab ▀") {
		t.Errorf("want a clear cell between text and backdrop, got %q", got)
	}

	// and before it too
	got = stripANSI(overlayLine("        ab", bgRow(w), w))
	if !strings.Contains(got, "▀ ab ▀") {
		t.Errorf("want clear cells either side, got %q", got)
	}
}

func TestOverlayRespectsTheMinimumGap(t *testing.T) {
	const w = 30
	// "a" then exactly minBackdropGap+2 spaces then "b": after the margins take
	// one cell each side there are minBackdropGap left, so it opens
	wide := "a" + strings.Repeat(" ", minBackdropGap+2) + "b"
	if got := stripANSI(overlayLine(wide, bgRow(w), w)); !strings.Contains(got[:len(wide)], "▀") {
		t.Errorf("a gap of %d should open, got %q", minBackdropGap+2, got[:len(wide)])
	}

	// one narrower stays closed
	narrow := "a" + strings.Repeat(" ", minBackdropGap+1) + "b"
	if got := stripANSI(overlayLine(narrow, bgRow(w), w)); strings.Contains(got[:len(narrow)], "▀") {
		t.Errorf("a gap of %d should stay closed, got %q", minBackdropGap+1, got[:len(narrow)])
	}
}

// The panel pads every body line to the content width, so a composited line has
// to come out exactly as wide as it was asked for.
func TestOverlayPreservesWidth(t *testing.T) {
	for _, w := range []int{10, 40, 80, 132} {
		bg := bgRow(w)
		for _, line := range []string{
			"",
			"short",
			strings.Repeat("x", w),
			strings.Repeat("x", w+20), // longer than the width
			accentStyle.Render("styled") + "   " + okStyle.Render("bits"),
			"a  b   c    d",
		} {
			got := overlayLine(line, bg, w)
			if n := lipgloss.Width(got); n != w {
				t.Errorf("w=%d line=%q: composited width %d", w, stripANSI(line), n)
			}
		}
	}
}

func TestOverlayKeepsStyling(t *testing.T) {
	const w = 30
	line := accentStyle.Render("keep") + "        " + okStyle.Render("me")
	got := overlayLine(line, bgRow(w), w)
	if !strings.Contains(got, "\x1b[") {
		t.Error("the text's own escapes were dropped")
	}
	if plain := stripANSI(got); !strings.Contains(plain, "keep") || !strings.Contains(plain, "me") {
		t.Errorf("text lost: %q", plain)
	}
}

func TestOverlayWithoutABackdropIsANoOp(t *testing.T) {
	content := []string{"a", "b"}
	got := overlay(content, nil, 20)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("overlay with no backdrop changed the content: %v", got)
	}

	// more content rows than backdrop rows: the extras pass through untouched
	got = overlay([]string{"a", "b", "c"}, [][]string{bgRow(20)}, 20)
	if got[1] != "b" || got[2] != "c" {
		t.Errorf("rows past the backdrop should be untouched: %v", got[1:])
	}
}

func TestBoardBackdropNeedsSomethingToDraw(t *testing.T) {
	m := filterModel(t) // no board, no placements
	if m.boardBackdrop(100, 20) != nil {
		t.Error("nothing to draw should give no backdrop")
	}

	withBoard, _ := csvModel(t, true) // placements from a cpl
	if withBoard.boardBackdrop(100, 20) == nil {
		t.Error("placements should give a backdrop")
	}
	// and it refuses sizes too small to say anything
	if withBoard.boardBackdrop(6, 20) != nil || withBoard.boardBackdrop(100, 2) != nil {
		t.Error("a tiny panel should get no backdrop")
	}
}

func TestBoardBackdropGridMatchesTheRequestedSize(t *testing.T) {
	m, _ := csvModel(t, true)
	for _, size := range [][2]int{{60, 10}, {100, 20}, {132, 30}} {
		grid := m.boardBackdrop(size[0], size[1])
		if len(grid) != size[1] {
			t.Fatalf("%dx%d: %d rows", size[0], size[1], len(grid))
		}
		for y, row := range grid {
			if len(row) != size[0] {
				t.Errorf("%dx%d: row %d has %d cells", size[0], size[1], y, len(row))
			}
		}
	}
}

// The Check page is the whole point of the backdrop, and its lines still have to
// measure up or the panel shifts.
func TestCheckPageLinesStayExactlyContentWide(t *testing.T) {
	m, _ := csvModel(t, true)
	m.mode = modeCheck
	for _, size := range [][2]int{{90, 24}, {118, 30}, {160, 44}} {
		m.w, m.h = size[0], size[1]
		w, h := m.contentW(), m.contentH()
		lines := strings.Split(m.viewCheck(w, h), "\n")
		if len(lines) > h {
			t.Errorf("%dx%d: %d lines, want at most %d", size[0], size[1], len(lines), h)
		}
		for y, ln := range lines {
			if n := lipgloss.Width(ln); n > w {
				t.Errorf("%dx%d: line %d is %d wide, want at most %d", size[0], size[1], y, n, w)
			}
		}
	}
}
