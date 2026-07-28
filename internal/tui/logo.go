package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const logoArt = `██████╗  ██████╗ ███╗   ███╗███████╗██╗  ██╗██████╗  ██████╗
██╔══██╗██╔═══██╗████╗ ████║██╔════╝╚██╗██╔╝██╔══██╗██╔═══██╗
██████╔╝██║   ██║██╔████╔██║█████╗   ╚███╔╝ ██████╔╝██║   ██║
██╔══██╗██║   ██║██║╚██╔╝██║██╔══╝   ██╔██╗ ██╔═══╝ ██║   ██║
██████╔╝╚██████╔╝██║ ╚═╝ ██║███████╗██╔╝ ██╗██║     ╚██████╔╝
╚═════╝  ╚═════╝ ╚═╝     ╚═╝╚══════╝╚═╝  ╚═╝╚═╝      ╚═════╝ `

// logo renders the banner with the solid blocks in the accent colour and the
// shadow strokes dimmed, for a subtle 3D look. Every line is padded to the
// widest so the block stays rectangular when it gets centred.
func logo() string {
	block := lipgloss.NewStyle().Foreground(cAccent)
	shade := lipgloss.NewStyle().Foreground(cPanel)
	lines := strings.Split(logoArt, "\n")
	width := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > width {
			width = w
		}
	}
	styled := make([]string, len(lines))
	for i, line := range lines {
		var b strings.Builder
		var run []rune
		var runBlock bool
		flush := func() {
			if len(run) == 0 {
				return
			}
			if runBlock {
				b.WriteString(block.Render(string(run)))
			} else {
				b.WriteString(shade.Render(string(run)))
			}
			run = run[:0]
		}
		for _, r := range line {
			if r == ' ' {
				flush()
				b.WriteByte(' ')
				continue
			}
			if isBlock := r == '█'; len(run) > 0 && isBlock != runBlock {
				flush()
				runBlock = isBlock
			} else {
				runBlock = isBlock
			}
			run = append(run, r)
		}
		flush()
		if pad := width - lipgloss.Width(line); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		styled[i] = b.String()
	}
	return strings.Join(styled, "\n")
}
