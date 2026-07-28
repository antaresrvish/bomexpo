package tui

import (
	tea "charm.land/bubbletea/v2"
)

type boardState struct {
	side      string
	img       string
	rendering bool
	rerr      string
}

type renderDoneMsg struct {
	side string
	path string
	err  error
}

func (m Model) renderCmd(side string) tea.Cmd {
	pcb := m.pcbPath
	return func() tea.Msg {
		p, err := renderBoard(pcb, side)
		return renderDoneMsg{side: side, path: p, err: err}
	}
}

func (b boardState) sideOr() string {
	if b.side == "" {
		return "top"
	}
	return b.side
}

// openRender renders the chosen board side (top/bottom/iso) via kicad-cli and
// opens the photorealistic result in the external viewer.
func (m Model) openRender(side string) (tea.Model, tea.Cmd) {
	if m.pcbPath == "" || m.boardv.rendering {
		return m, nil
	}
	m.boardv.side = side
	m.boardv.rendering = true
	m.loading = true
	m.status = "rendering 3D board (" + side + ")…"
	return m, m.renderCmd(side)
}
