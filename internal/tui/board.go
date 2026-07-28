package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/render"
)

type boardState struct {
	side       string
	img        string
	rendering  bool
	rerr       string
	showCopper bool
	zoom       float64
	panX, panY float64
	dragging   bool
	lastX      int
	lastY      int
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

func (m Model) setSide(side string) (tea.Model, tea.Cmd) {
	m.boardv.side = side
	m.boardv.rendering = true
	m.boardv.rerr = ""
	return m, m.renderCmd(side)
}

func (m Model) updateBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := &m.boardv
	switch msg.String() {
	case "esc", "q":
		m.mode = modeTable
	case "tab":
		return m.cycleTab(1)
	case "shift+tab":
		return m.cycleTab(-1)
	case "t":
		return m.setSide("top")
	case "b":
		return m.setSide("bottom")
	case "i":
		return m.setSide("iso")
	case "o", "enter":
		if b.img != "" {
			if err := openExternal(b.img); err != nil {
				m.err = err.Error()
			}
		} else if !b.rendering {
			return m.setSide(b.sideOr())
		}
	case "r":
		return m.setSide(b.sideOr())
	case "c":
		b.showCopper = !b.showCopper
	case "+", "=":
		b.zoom = clampZoom(b.zoomOr1() * 1.2)
	case "-", "_":
		b.zoom = clampZoom(b.zoomOr1() / 1.2)
	case "left":
		b.panX += 4
	case "right":
		b.panX -= 4
	case "up":
		b.panY += 4
	case "down":
		b.panY -= 4
	}
	return m, nil
}

func (m Model) mouseBoard(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	b := &m.boardv
	switch {
	case wheel:
		if ms.Button == tea.MouseWheelUp {
			b.zoom = clampZoom(b.zoomOr1() * 1.15)
		} else if ms.Button == tea.MouseWheelDown {
			b.zoom = clampZoom(b.zoomOr1() / 1.15)
		}
	case click && ms.Button == tea.MouseLeft:
		b.dragging = true
		b.lastX, b.lastY = ms.X, ms.Y
	case ms.Button == tea.MouseLeft:
		if b.dragging {
			b.panX += float64(ms.X - b.lastX)
			b.panY += float64((ms.Y - b.lastY) * 2)
			b.lastX, b.lastY = ms.X, ms.Y
		}
	default:
		b.dragging = false
	}
	return m, nil
}

func (b boardState) sideOr() string {
	if b.side == "" {
		return "top"
	}
	return b.side
}

func (b boardState) zoomOr1() float64 {
	if b.zoom <= 0 {
		return 1
	}
	return b.zoom
}

func clampZoom(z float64) float64 {
	if z < 0.3 {
		return 0.3
	}
	if z > 14 {
		return 14
	}
	return z
}

func (m Model) viewBoard(w, h int) string {
	if m.board == nil || m.board.Empty() {
		return subtleStyle.Render("no board outline in this project")
	}
	b := m.boardv

	var status string
	switch {
	case b.rendering:
		status = m.spin.View() + " rendering 3D (" + b.sideOr() + ")…"
	case b.rerr != "":
		status = badStyle.Render("render failed: " + b.rerr)
	case b.img != "":
		status = okStyle.Render("3D "+b.sideOr()+" ready") + dimStyle.Render(" — press o to open in viewer")
	default:
		status = dimStyle.Render("press o for a photorealistic 3D render (opens in your image viewer)")
	}
	hint := fmt.Sprintf("%s   %s",
		accentStyle.Render("o")+dimStyle.Render(" render/open  ")+
			accentStyle.Render("t")+dimStyle.Render("/")+accentStyle.Render("b")+dimStyle.Render("/")+accentStyle.Render("i")+dimStyle.Render(" angle"),
		status)

	hl := map[string]bool{}
	sel := "—"
	if m.cursor >= 0 && m.cursor < len(m.items) {
		sel = m.items[m.cursor].ID()
		for _, d := range m.items[m.cursor].Designators {
			hl[d] = true
		}
	}
	legend := fmt.Sprintf("%s edge  %s/%s smd  %s %s",
		accentStyle.Render("─"), fgSwatch(cTeal), fgSwatch(cMag),
		badStyle.Render("■"), subtleStyle.Render(sel))

	img := render.Render(m.board, m.placements, render.Options{
		W: w, H: h - 2,
		ShowCopper: b.showCopper,
		Highlight:  hl,
		Zoom:       b.zoomOr1(),
		PanX:       b.panX,
		PanY:       b.panY,
	})
	return hint + "\n" + legend + "\n" + img
}
