package tui

import (
	tea "charm.land/bubbletea/v2"
)

type boardState struct {
	side      string
	img       string
	rendering bool
	rerr      string

	// zoom and pan drive the inline board view. zoom 1 fits the board.
	zoom       float64
	panX, panY float64
}

func newBoardState() boardState { return boardState{zoom: 1} }

const (
	zoomStep float64 = 1.25
	zoomMin  float64 = 1
	zoomMax  float64 = 12
	panStep  float64 = 4 // subpixels per keypress, scaled by the zoom level
)

// zoomBy multiplies the zoom, keeping it in range. Zooming back out to the fit
// level drops the pan too, so there's always a way back to the whole board.
func (b boardState) zoomBy(f float64) boardState {
	b.zoom = clampFloat(b.zoom*f, zoomMin, zoomMax)
	if b.zoom == zoomMin {
		b.panX, b.panY = 0, 0
	}
	return b
}

// panBy moves the view, not the board: dx=1 means "show me what's to the right",
// so it walks the board the other way. The renderer adds panX to every drawn
// point, hence the sign flip here — the one place it belongs, so callers can just
// say which way the arrow pointed.
func (b boardState) panBy(dx, dy float64) boardState {
	if b.zoom <= zoomMin {
		return b // nothing to pan when the whole board already fits
	}
	b.panX -= dx * panStep * b.zoom
	b.panY -= dy * panStep * b.zoom
	return b
}

// resetView returns to the fit-the-whole-board view. Zooming all the way out
// does this too, so there's no separate key for it.
func (b boardState) resetView() boardState {
	b.zoom, b.panX, b.panY = 1, 0, 0
	return b
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
	if m.boardv.rendering {
		return m, nil
	}
	if m.pcbPath == "" {
		m.flash = "no board to render — this design came from a csv"
		return m, nil
	}
	m.boardv.side = side
	m.boardv.rendering = true
	m.loading = true
	m.status = "rendering 3D board (" + side + ")…"
	return m, m.renderCmd(side)
}
