package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

func fgSwatch(c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render("■")
}

var (
	cFg     = lipgloss.Color("#d5e4cc")
	cMuted  = lipgloss.Color("#7c8a73")
	cDim    = lipgloss.Color("#5a6650")
	cAccent = lipgloss.Color("#a7c957")
	cGreen  = lipgloss.Color("#7fb069")
	cTeal   = lipgloss.Color("#4cc9f0")
	cMag    = lipgloss.Color("#c77dff")
	cOk     = lipgloss.Color("#80ed99")
	cWarn   = lipgloss.Color("#f6bd60")
	cBad    = lipgloss.Color("#e07a5f")
	cInk    = lipgloss.Color("#12180f")
	cSelBg  = lipgloss.Color("#2d3a2e")
	cPanel  = lipgloss.Color("#3a4a38")
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent).Padding(0, 1)
	subtleStyle = lipgloss.NewStyle().Foreground(cMuted)
	dimStyle    = lipgloss.NewStyle().Foreground(cDim)
	accentStyle = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(cOk)
	warnStyle   = lipgloss.NewStyle().Foreground(cWarn)
	badStyle    = lipgloss.NewStyle().Foreground(cBad)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cPanel).
			Padding(0, 1)

	selRowStyle = lipgloss.NewStyle().Background(cSelBg).Foreground(cFg).Bold(true)

	badgeOk   = lipgloss.NewStyle().Foreground(cInk).Background(cOk).Padding(0, 1).Bold(true)
	badgeWarn = lipgloss.NewStyle().Foreground(cInk).Background(cWarn).Padding(0, 1).Bold(true)
	badgeBad  = lipgloss.NewStyle().Foreground(cFg).Background(cBad).Padding(0, 1).Bold(true)

	keyStyle  = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	descStyle = lipgloss.NewStyle().Foreground(cMuted)
	sepStyle  = lipgloss.NewStyle().Foreground(cDim)

	labelStyle = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	codeStyle  = lipgloss.NewStyle().Foreground(cTeal).Bold(true)

	cursorStyle    = lipgloss.NewStyle().Background(cAccent).Foreground(cInk)
	selectionStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3d5a3f")).Foreground(cFg)
	linkStyle      = lipgloss.NewStyle().Foreground(cTeal).Underline(true)

	tabActive   = lipgloss.NewStyle().Foreground(cInk).Background(cAccent).Bold(true).Padding(0, 2)
	tabInactive = lipgloss.NewStyle().Foreground(cMuted).Padding(0, 2)
	tabBrand    = lipgloss.NewStyle().Foreground(cAccent).Bold(true).Padding(0, 2)

	colHeadStyle = lipgloss.NewStyle().Foreground(cGreen).Background(lipgloss.Color("#1c2419")).Bold(true)
	borderStyle  = lipgloss.NewStyle().Foreground(cPanel)
)

func panelBox(title, body string, w, h int) string {
	if w < 6 {
		w = 6
	}
	innerW := w - 4
	t := " " + title + " "
	fill := w - 3 - lipgloss.Width(t)
	if fill < 0 {
		fill = 0
	}
	top := borderStyle.Render("╭─") + accentStyle.Render(t) + borderStyle.Render(strings.Repeat("─", fill)+"╮")
	bottom := borderStyle.Render("╰" + strings.Repeat("─", w-2) + "╯")

	lines := strings.Split(body, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	lines = lines[:h]

	bar := borderStyle.Render("│")
	var b strings.Builder
	b.WriteString(top + "\n")
	for i, ln := range lines {
		b.WriteString(bar + " " + padRender(ln, innerW) + " " + bar)
		if i < len(lines)-1 || true {
			b.WriteByte('\n')
		}
	}
	b.WriteString(bottom)
	return b.String()
}

func padRender(s string, n int) string {
	w := lipgloss.Width(s)
	if w > n {
		return truncANSI(s, n)
	}
	return s + spaces(n-w)
}

func truncANSI(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return trunc(s, n)
	}
	return s + spaces(n-w)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// plural renders a count with the right word for it.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
