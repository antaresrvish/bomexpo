package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws box over bg at (x, y), leaving the rest showing. Slicing goes
// through ansi.Cut so the background keeps its colours either side of the hole.
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
			right = "\x1b[0m" + ansi.Cut(under, x+boxW, w) // reset, or the box bleeds
		}
		out[row] = left + padRender(truncANSI(ln, boxW), boxW) + right
	}
	return out
}

// popupFrame wraps content in a titled box with a shadow down its right side and
// along the bottom.
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

	for i := 1; i < len(out); i++ {
		out[i] += dimStyle.Render("░")
	}
	return append(out, spaces(1)+dimStyle.Render(strings.Repeat("░", bw-1)))
}

func popupW(w int) int {
	pw := w - 8
	if pw > 108 {
		pw = 108
	}
	return max(min(pw, w), 8)
}

// popupBox centres a popup, only as tall as want rows need.
func popupBox(w, h, want int) (x, y, pw, ph int) {
	pw = popupW(w)
	ph = clampInt(want, 6, max(h-3, 6))
	x = max((w-pw)/2, 0)
	y = max((h-ph)/2-1, 0)
	return x, y, pw, ph
}
