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
// shadow strokes dimmed, for a subtle 3D look.
func logo() string {
	block := lipgloss.NewStyle().Foreground(cAccent)
	shade := lipgloss.NewStyle().Foreground(cPanel)
	var b strings.Builder
	for _, line := range strings.Split(logoArt, "\n") {
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
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
