package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/lcsc"
)

func (m Model) contentW() int {
	w := m.w - 4
	if w < 24 {
		w = 24
	}
	return w
}

func (m Model) contentH() int {
	h := m.h - 4
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case searchDebounceMsg:
		return m.updateSearchDebounce(msg)

	case projectLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.name = msg.name
		m.pcbPath = msg.pcbPath
		m.items = msg.items
		m.placements = msg.placements
		m.board = msg.board
		m.layers = msg.layers
		m.boardW, m.boardH = msg.boardW, msg.boardH
		m.assigned = make([]*lcsc.Part, len(m.items))
		m.excluded = make([]bool, len(m.items))
		dnp, exb := 0, 0
		for i := range m.items {
			switch {
			case m.items[i].DNP:
				m.excluded[i] = true
				dnp++
			case m.items[i].ExcludeBOM:
				m.excluded[i] = true
				exb++
			}
		}
		m.mode = modeOverview
		m.cursor, m.top = 0, 0
		m.err = ""
		m.status = fmt.Sprintf("%s · %d components in %d line items", msg.name, len(m.placements), len(m.items))
		m.flash = fmt.Sprintf("loaded %s — %d line items", msg.name, len(m.items))
		if dnp > 0 {
			m.flash += fmt.Sprintf(" · %d DNP", dnp)
		}
		if exb > 0 {
			m.flash += fmt.Sprintf(" · %d excluded restored", exb)
		}
		return m, m.prefillCmd()

	case detailDoneMsg:
		if msg.err == nil && msg.idx >= 0 && msg.idx < len(m.assigned) {
			p := msg.part
			m.assigned[msg.idx] = &p
		}
		return m, nil

	case searchDoneMsg:
		return m.updateSearch(msg)

	case exportDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = "export failed: " + msg.err.Error()
		} else {
			m.flash = "✓ exported → " + msg.path
		}
		return m, nil

	case savedMsg:
		if msg.err != nil {
			m.err = "write failed: " + msg.err.Error()
		} else {
			r := msg.res
			if r.CodesUpdated+r.CodesInserted+r.Excluded+r.Included == 0 {
				m.flash = "✓ " + filepath.Base(msg.path) + " already up to date"
			} else {
				m.flash = fmt.Sprintf("✓ saved %s — %d codes, %d excluded, %d re-included",
					filepath.Base(msg.path), r.CodesUpdated+r.CodesInserted, r.Excluded, r.Included)
			}
		}
		return m, nil

	case autoAssignedMsg:
		if m.autoRemaining > 0 {
			m.autoRemaining--
		}
		if msg.ok && msg.idx >= 0 && msg.idx < len(m.items) {
			p := msg.part
			m.items[msg.idx].LCSC = p.Code
			m.assigned[msg.idx] = &p
			m.autoOK++
		}
		if m.autoRemaining <= 0 {
			m.loading = false
			m.status = fmt.Sprintf("Auto-assigned %d parts — review the warnings", m.autoOK)
		} else {
			m.status = fmt.Sprintf("Auto-assigning… %d left", m.autoRemaining)
		}
		return m, nil

	case refreshDoneMsg:
		if m.refreshRemaining > 0 {
			m.refreshRemaining--
		}
		if msg.err == nil && msg.idx >= 0 && msg.idx < len(m.assigned) {
			p := msg.part
			m.assigned[msg.idx] = &p
			m.refreshOK++
		}
		if m.refreshRemaining <= 0 {
			m.loading = false
			m.flash = fmt.Sprintf("↻ refreshed %d parts — stock & prices updated", m.refreshOK)
		}
		return m, nil

	case renderDoneMsg:
		m.boardv.rendering = false
		if msg.err != nil {
			m.boardv.rerr = msg.err.Error()
			return m, nil
		}
		m.boardv.rerr = ""
		if msg.side == m.boardv.sideOr() {
			m.boardv.img = msg.path
		}
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.flash = ""
		return m.routeKey(msg)

	case tea.MouseClickMsg:
		return m.routeMouse(msg.Mouse(), true, false)
	case tea.MouseWheelMsg:
		return m.routeMouse(msg.Mouse(), false, true)
	case tea.MouseMotionMsg:
		return m.routeMouse(msg.Mouse(), false, false)
	}
	return m, nil
}

func (m Model) routeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeLoad:
		return m.updateLoad(msg)
	case modeOverview:
		return m.updateOverview(msg)
	case modeTable:
		return m.updateTable(msg)
	case modeSearch:
		return m.updateSearchKey(msg)
	case modeBoard:
		return m.updateBoard(msg)
	case modeCheck:
		return m.updateCheck(msg)
	}
	return m, nil
}

func (m Model) routeMouse(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	if click && ms.Y == 0 && len(m.items) > 0 {
		if md, ok := m.tabAtX(ms.X); ok {
			return m.gotoTab(md)
		}
	}
	switch m.mode {
	case modeOverview:
		return m.mouseOverview(ms, click, wheel)
	case modeTable:
		return m.mouseTable(ms, click, wheel)
	case modeSearch:
		return m.mouseSearch(ms, click, wheel)
	case modeBoard:
		return m.mouseBoard(ms, click, wheel)
	case modeCheck:
		return m.mouseCheck(ms, click, wheel)
	}
	return m, nil
}

func (m Model) gotoTab(md mode) (tea.Model, tea.Cmd) {
	switch md {
	case modeBoard:
		if m.board == nil {
			m.status = "no board outline"
			return m, nil
		}
		m.mode = md
		if m.boardv.img == "" && !m.boardv.rendering && m.pcbPath != "" {
			m.boardv.side = m.boardv.sideOr()
			m.boardv.rendering = true
			return m, m.renderCmd(m.boardv.side)
		}
		return m, nil
	case modeCheck:
		m.check.setDefault(m.pcbPath)
		m.check.out.Focus()
		m.mode = md
		return m, nil
	case modeLoad:
		m.mode = md
		return m, m.load.focusCmd()
	}
	m.mode = md
	return m, nil
}

func (m Model) cycleTab(dir int) (tea.Model, tea.Cmd) {
	cur := 1
	want := m.mode
	if want == modeSearch {
		want = modeTable
	}
	for i, t := range tabs {
		if t.mode == want {
			cur = i
		}
	}
	return m.gotoTab(tabs[(cur+dir+len(tabs))%len(tabs)].mode)
}

func (m Model) prefillCmd() tea.Cmd {
	var cmds []tea.Cmd
	for i, it := range m.items {
		if it.LCSC != "" {
			cmds = append(cmds, m.detailCmd(i, it.LCSC))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	if m.w == 0 {
		m.w, m.h = 110, 34
	}
	cw, ch := m.contentW(), m.contentH()

	var title, body string
	switch m.mode {
	case modeLoad:
		title, body = "Open project", m.viewLoad(cw, ch)
	case modeOverview:
		title, body = "Overview"+projSuffix(m.name), m.viewOverview(cw, ch)
	case modeTable:
		title, body = "Components"+projSuffix(m.name), m.viewTable(cw, ch)
	case modeSearch:
		title, body = "Search LCSC", m.viewSearch(cw, ch)
	case modeBoard:
		title, body = "Board"+projSuffix(m.name), m.viewBoard(cw, ch)
	case modeCheck:
		title, body = "Final check & export", m.viewCheck(cw, ch)
	}

	screen := lipgloss.JoinVertical(lipgloss.Left,
		m.tabBar(), panelBox(title, body, m.w, ch), m.bottomBar())
	v := tea.NewView(screen)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func projSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " · " + name
}

func (m Model) tabBar() string {
	segs := []string{tabBrand.Render("bomexpo")}
	for i, t := range tabs {
		active := t.mode == m.mode || (m.mode == modeSearch && t.mode == modeTable)
		st := tabInactive
		if active {
			st = tabActive
		}
		segs = append(segs, st.Render(fmt.Sprintf("%d %s", i+1, t.label)))
	}
	left := strings.Join(segs, "")

	right := ""
	if len(m.items) > 0 {
		a, wn := m.counts()
		right = okStyle.Render(fmt.Sprintf(" %d/%d ", a, m.activeCount())) +
			warnBadge(wn)
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return left + spaces(gap) + right
}

func warnBadge(n int) string {
	if n == 0 {
		return okStyle.Render(" ✓ all clear ")
	}
	return badStyle.Render(fmt.Sprintf(" %d ⚠ ", n))
}

func (m Model) tabAtX(x int) (mode, bool) {
	cur := lipgloss.Width(tabBrand.Render("bomexpo"))
	for i, t := range tabs {
		w := lipgloss.Width(tabInactive.Render(fmt.Sprintf("%d %s", i+1, t.label)))
		if x >= cur && x < cur+w {
			return t.mode, true
		}
		cur += w
	}
	return modeLoad, false
}

func (m Model) totalCost() (float64, bool) {
	total, complete := 0.0, true
	for i, it := range m.items {
		p := m.assigned[i]
		if p == nil {
			complete = false
			continue
		}
		if u, ok := p.UnitPrice(); ok {
			total += u * float64(it.Quantity)
		} else {
			complete = false
		}
	}
	return total, complete
}

func (m Model) infoStats() string {
	seg := func(label, val string) string {
		return dimStyle.Render(label+" ") + lipgloss.NewStyle().Foreground(cFg).Render(val)
	}
	var parts []string
	if m.boardW > 0 {
		parts = append(parts, seg("board", fmt.Sprintf("%.1f×%.1f mm", m.boardW, m.boardH)))
	}
	if m.layers > 0 {
		parts = append(parts, seg("layers", fmt.Sprintf("%d", m.layers)))
	}
	a, _ := m.counts()
	parts = append(parts, seg("assigned", fmt.Sprintf("%d/%d", a, m.activeCount())))
	if ex := m.excludedCount(); ex > 0 {
		parts = append(parts, seg("excluded", fmt.Sprintf("%d", ex)))
	}
	cost, complete := m.totalCost()
	costStr := fmt.Sprintf("$%.2f", cost)
	if !complete {
		costStr += "*"
	}
	parts = append(parts, dimStyle.Render("cost ")+okStyle.Render(costStr))
	return strings.Join(parts, sepStyle.Render("  │  "))
}

func (m Model) bottomBar() string {
	var left string
	switch {
	case m.err != "":
		left = badStyle.Render("✗ " + m.err)
	case m.loading:
		left = m.spin.View() + " " + subtleStyle.Render(m.status)
	case m.flash != "":
		left = okStyle.Render(m.flash)
	case len(m.items) > 0:
		left = m.infoStats()
	default:
		left = subtleStyle.Render(m.status)
	}
	help := m.helpLine()
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(help) - 1
	if gap < 1 {
		gap = 1
	}
	return " " + left + spaces(gap) + help
}

func (m Model) helpLine() string {
	var hints [][2]string
	switch m.mode {
	case modeLoad:
		hints = [][2]string{{"tab", "complete"}, {"enter", "open"}, {"ctrl+c", "quit"}}
	case modeOverview:
		hints = [][2]string{{"enter", "next"}, {"a", "auto-assign"}, {"r", "refresh"}, {"w", "save"}, {"tab", "switch"}}
	case modeTable:
		hints = [][2]string{{"enter", "assign"}, {"a", "auto-assign"}, {"r", "refresh"}, {"w", "save"}, {"x", "exclude"}, {"tab", "switch"}}
	case modeSearch:
		hints = [][2]string{{"type", "search"}, {"↑↓", "results"}, {"enter", "pick"}, {"^f/^t/^s", "filters"}, {"esc", "back"}}
	case modeBoard:
		hints = [][2]string{{"o", "3D render"}, {"t/b/i", "angle"}, {"c", "traces"}, {"tab", "switch"}, {"esc", "back"}}
	case modeCheck:
		hints = [][2]string{{"click", "go to part"}, {"enter", "export"}, {"tab", "switch"}, {"esc", "back"}}
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = keyStyle.Render(h[0]) + " " + descStyle.Render(h[1])
	}
	return strings.Join(parts, sepStyle.Render("  "))
}
