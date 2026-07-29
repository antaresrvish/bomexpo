package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bomexpo/internal/render"
)

// The Check page is the last look before money is spent, so it puts the board
// behind the numbers: full size, muted, showing through wherever the text
// leaves a gap.

// boardBackdrop is the whole board drawn to fill w×h in background tones, as a
// grid of single-cell strings. Nil when there's nothing to draw.
func (m Model) boardBackdrop(w, h int) [][]string {
	if w < 8 || h < 4 {
		return nil
	}
	if m.board == nil || (m.board.Empty() && len(m.placements) == 0) {
		return nil
	}
	hl := map[string]bool{}
	if i := m.sel(); i >= 0 {
		for _, d := range m.items[i].Designators {
			hl[d] = true
		}
	}
	return render.Cells(m.board, m.placements, render.Options{
		// No copper: traces cover nearly every cell and turn the page into
		// noise. The outline and the parts read as the board on their own.
		W: w, H: h, ShowCopper: false, Dim: true,
		Highlight: hl,
		Match:     m.matchedRefs(),
		Zoom:      m.boardv.zoom,
		PanX:      m.boardv.panX,
		PanY:      m.boardv.panY,
	})
}

// overlay draws content over a backdrop: every run of blanks in a content line
// is replaced by the cells behind it.
//
// Columns are counted in runes. Everything either layer draws — box characters,
// ✓/✗, ▀, dashes — is one cell wide, so a rune is a column.
func overlay(content []string, bg [][]string, w int) []string {
	if len(bg) == 0 {
		return content
	}
	out := make([]string, len(content))
	for y, line := range content {
		if y >= len(bg) {
			out[y] = line
			continue
		}
		out[y] = overlayLine(line, bg[y], w)
	}
	return out
}

const (
	// minBackdropGap is how many blank cells it takes before the backdrop shows
	// through. Without it the single spaces between words fill in too, and the
	// text stops being readable.
	minBackdropGap = 3
	// textMargin keeps a clear cell either side of every glyph, so letters never
	// touch the drawing behind them.
	textMargin = 1
)

func overlayLine(line string, bg []string, w int) string {
	plain := []rune(ansi.Strip(line))

	// claim the glyphs and their margins
	taken := make([]bool, w)
	for i := 0; i < w && i < len(plain); i++ {
		if plain[i] == ' ' {
			continue
		}
		for d := -textMargin; d <= textMargin; d++ {
			if j := i + d; j >= 0 && j < w {
				taken[j] = true
			}
		}
	}

	// what's left shows the backdrop, but only where the gap is wide enough
	show := make([]bool, w)
	for x := 0; x < w; {
		if taken[x] {
			x++
			continue
		}
		start := x
		for x < w && !taken[x] {
			x++
		}
		if x-start >= minBackdropGap {
			for i := start; i < x; i++ {
				show[i] = true
			}
		}
	}

	var b strings.Builder
	for x := 0; x < w; {
		start := x
		if show[x] {
			for x < w && show[x] {
				x++
			}
			for i := start; i < x; i++ {
				if i < len(bg) {
					b.WriteString(bg[i])
					continue
				}
				b.WriteByte(' ')
			}
			continue
		}
		for x < w && !show[x] {
			x++
		}
		// keep the text's own styling by cutting it straight out of the line
		seg := ansi.Cut(line, start, x)
		b.WriteString(seg)
		if gap := (x - start) - lipgloss.Width(seg); gap > 0 {
			b.WriteString(spaces(gap))
		}
	}
	return b.String()
}
