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

// openRender opens the photorealistic 3D board render in the external viewer,
// rendering it first (via kicad-cli) if it isn't cached yet.
func (m Model) openRender() (tea.Model, tea.Cmd) {
	if m.pcbPath == "" {
		return m, nil
	}
	if m.boardv.img != "" {
		openExternal(m.boardv.img)
		m.flash = "opened 3D render in your viewer"
		return m, nil
	}
	if m.boardv.rendering {
		return m, nil
	}
	m.boardv.rendering = true
	m.loading = true
	m.status = "rendering 3D board…"
	return m, m.renderCmd("top")
}
