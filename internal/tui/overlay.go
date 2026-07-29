package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws box on top of bg at (x, y), leaving whatever the box doesn't
// cover still showing. That's what makes a popup read as floating over the page
// rather than replacing it.
//
// The slicing goes through ansi.Cut so a styled background line keeps its colours
// on both sides of the hole, and every line comes back exactly w columns wide —
// one column of overflow would push the panel border out.
func overlay(bg []string, box []string, x, y, w int) []string {
	if len(box) == 0 {
		return bg
	}
	boxW := 0
	for _, ln := range box {
		if n := lipgloss.Width(ln); n > boxW {
			boxW = n
		}
	}
	if x < 0 {
		x = 0
	}
	if x+boxW > w {
		boxW = w - x
	}
	if boxW < 1 {
		return bg
	}

	out := make([]string, len(bg))
	copy(out, bg)
	for i, ln := range box {
		row := y + i
		if row < 0 || row >= len(out) {
			continue
		}
		under := padRender(out[row], w)
		left := ansi.Cut(under, 0, x)
		right := ""
		if x+boxW < w {
			// the reset stops the box's colour bleeding into what follows
			right = "\x1b[0m" + ansi.Cut(under, x+boxW, w)
		}
		out[row] = left + padRender(truncANSI(ln, boxW), boxW) + right
	}
	return out
}

// popupFrame wraps content in a titled box sized to (w, h), padded a column in
// from the border, with a dim shadow down its right side and along the bottom so
// it lifts off the page behind it.
func popupFrame(title string, content []string, w, h int) []string {
	const shadow = 1
	bw, bh := w-shadow, h-shadow
	if bw < 4 || bh < 3 {
		return content
	}
	inner := bw - 4 // border and a column of padding on each side

	head := trunc(ansi.Strip(title), max(inner-2, 1))
	rule := strings.Repeat("─", max(bw-4-lipgloss.Width(head), 0))
	out := []string{borderStyle.Render("╭─ ") + accentStyle.Render(head) +
		borderStyle.Render(" "+rule+"╮")}

	for i := 0; i < bh-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		out = append(out, borderStyle.Render("│")+" "+padRender(truncANSI(line, inner), inner)+
			" "+borderStyle.Render("│"))
	}
	out = append(out, borderStyle.Render("╰"+strings.Repeat("─", bw-2)+"╯"))

	// The shadow is a column on the right of every row but the first, and a row
	// under the box offset by one, which is how a dropped shadow falls.
	for i := 1; i < len(out); i++ {
		out[i] += dimStyle.Render("░")
	}
	return append(out, spaces(1)+dimStyle.Render(strings.Repeat("░", bw-1)))
}

// popupW is how wide a popup gets: most of the page, but never so wide that the
// table behind it is a row of one-character slivers.
func popupW(w int) int {
	pw := w - 8
	if pw > 108 {
		pw = 108
	}
	return max(min(pw, w), 8)
}

// popupBox is where a popup sits: centred across, a little above centre so it
// doesn't crowd the status line, and only as tall as want rows need — a popup
// padded out with blank rows looks broken rather than roomy.
func popupBox(w, h, want int) (x, y, pw, ph int) {
	pw = popupW(w)
	ph = clampInt(want, 6, max(h-3, 6))
	x = max((w-pw)/2, 0)
	y = max((h-ph)/2-1, 0)
	return x, y, pw, ph
}
