package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
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

	case partsDebounceMsg:
		return m.updatePartsDebounce(msg)

	case partsDoneMsg:
		return m.updatePartsDone(msg)

	case pinDetailMsg:
		return m.updatePinDetail(msg)

	case catsLoadedMsg:
		return m.updateCatsLoaded(msg)

	case diffDoneMsg:
		return m.updateDiffDone(msg)

	case footprintDoneMsg:
		// a failed download just leaves the card showing the package name
		if msg.err == nil && len(msg.fp.Lands) > 0 && m.edaLands != nil {
			m.edaLands[msg.code] = msg.fp
		}
		return m, nil

	case projectLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		d := msg.design
		m.name = d.Name
		m.pcbPath, m.bomPath, m.cplPath = d.PCBPath, d.BOMPath, d.CPLPath
		m.items = d.Items
		m.placements = d.Placements
		m.board = d.Board
		m.designNets = d.Nets
		m.designLands = d.Lands
		m.boardv = newBoardState()
		m.layers = d.Layers
		m.boardW, m.boardH = d.BoardW, d.BoardH
		m.assigned = make([]*part.Part, len(m.items))
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
		m.load.field.Blur() // the path did its job; Components has the keys now
		m.mode = modeTable
		m.cursor, m.top, m.hoff = 0, 0, 0
		m.sort, m.sortAsc = sortNone, false
		m = m.reindex()
		m.err = ""
		kind := "board"
		if !m.fromBoard() {
			kind = "bom csv"
		}
		m.status = fmt.Sprintf("%s · %s · %d components in %d line items",
			d.Name, kind, len(m.placements), len(m.items))
		m.flash = fmt.Sprintf("loaded %s — %d line items", d.Name, len(m.items))
		if d.CPLPath != "" {
			m.flash += " · cpl " + filepath.Base(d.CPLPath)
		}
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
			if r.CodesUpdated+r.CodesInserted+r.Excluded+r.Included+r.RotSet+r.RotCleared == 0 {
				m.flash = "✓ " + filepath.Base(msg.path) + " already up to date"
			} else {
				m.flash = fmt.Sprintf("✓ saved %s — %d code, %d exclude, %d rotation changes",
					filepath.Base(msg.path), r.CodesUpdated+r.CodesInserted, r.Excluded+r.Included, r.RotSet+r.RotCleared)
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
		m.loading = false
		if msg.err != nil {
			m.err = "render failed: " + msg.err.Error()
			return m, nil
		}
		m.boardv.img = msg.path
		openExternal(msg.path)
		m.flash = "opened 3D render in your viewer"
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
	case modeTable:
		return m.updateTable(msg)
	case modeSearch:
		return m.updateSearchKey(msg)
	case modeParts:
		if m.cat.open {
			return m.updateCatKey(msg)
		}
		return m.updatePartsKey(msg)
	case modeCompare:
		return m.updateCompareKey(msg)
	case modeNets:
		return m.updateNetKey(msg)
	case modeCheck:
		return m.updateCheck(msg)
	case modeDiff:
		return m.updateDiffKey(msg)
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
	case modeTable:
		return m.mouseTable(ms, click, wheel)
	case modeSearch:
		return m.mouseSearch(ms, click, wheel)
	case modeParts:
		if m.cat.open {
			return m.mouseCat(ms, click, wheel)
		}
		return m.mouseParts(ms, click, wheel)
	case modeCompare:
		return m.mouseCompare(ms, click, wheel)
	case modeNets:
		return m.mouseNets(ms, click, wheel)
	case modeCheck:
		return m.mouseCheck(ms, click, wheel)
	}
	return m, nil
}

func (m Model) gotoTab(md mode) (tea.Model, tea.Cmd) {
	// Arriving anywhere leaves the keys to the page: 1-5 and [ ] are printable, so
	// a focused field would swallow them and tab switching would stop working.
	// Init focuses the path field because on first launch there's nothing else to
	// do; every move after that goes through here.
	m.load.field.Blur()
	m.parts.field.Blur()
	m.cat.open = false
	m.cat.field.Blur()
	m.check.setPane(paneIssues)

	switch md {
	case modeCheck:
		m.check.setDefault(m.sourcePath())
	case modeCompare:
		if len(m.parts.pinned) < 2 {
			m.flash = "pin at least two parts to compare"
			return m, nil
		}
		m.compare.sel = clampInt(m.compare.sel, 0, len(m.parts.pinned)-1)
		m.mode = md
		// pick up any footprint we still don't have, e.g. after a restart
		var cmds []tea.Cmd
		for _, p := range m.parts.pinned {
			if c := m.landsCmd(p.Code); c != nil {
				cmds = append(cmds, c)
			}
		}
		return m, tea.Batch(cmds...)
	}
	m.mode = md
	return m, nil
}

func (m Model) cycleTab(dir int) (tea.Model, tea.Cmd) {
	tabs := m.tabs()
	cur := 1
	want := m.mode
	switch want {
	case modeSearch, modeNets:
		want = modeTable // detours off Components, not tabs of their own
	case modeCompare:
		want = modeParts
	case modeDiff:
		want = modeCheck
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
	v := tea.NewView(m.screen())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// screen is the whole frame as text: tab bar, panel, bottom bar. View wraps it,
// and tests measure it.
func (m Model) screen() string {
	if m.w == 0 {
		m.w, m.h = 110, 34
	}
	title, body := m.titleBody()
	return lipgloss.JoinVertical(lipgloss.Left,
		m.tabBar(), panelBox(title, body, m.w, m.contentH()), m.bottomBar())
}

// titleBody renders the current mode's panel. It's the single place that maps a
// mode to its view — View and the test harnesses all go through here so a new
// mode can't be half-wired.
func (m Model) titleBody() (title, body string) {
	cw, ch := m.contentW(), m.contentH()
	switch m.mode {
	case modeTable:
		return "Components" + projSuffix(m.name), m.viewTable(cw, ch)
	case modeSearch:
		return "Search " + m.srcLabel(), m.viewSearch(cw, ch)
	case modeParts:
		if m.cat.open {
			// the popup floats over the results, so the tab keeps its own title
			return "Parts · " + m.srcLabel(), m.viewCategories(cw, ch)
		}
		return "Parts · " + m.srcLabel(), m.viewParts(cw, ch)
	case modeCompare:
		return m.compareTitle(), m.viewCompare(cw, ch)
	case modeNets:
		return "Nets" + projSuffix(m.name), m.viewNets(cw, ch)
	case modeCheck:
		return "Export the order", m.viewCheck(cw, ch)
	case modeDiff:
		return "Verify the design against a bom", m.viewDiff(cw, ch)
	}
	return "Open project", m.viewLoad(cw, ch)
}

func projSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " · " + name
}

func (m Model) tabBar() string {
	brand := tabBrand.Render("bomexpo")
	var segs []string
	for _, t := range m.tabs() {
		// Search and Nets are detours off Components; Compare is one off Parts. None
		// of them are tabs, so the tab they came from stays lit.
		home := m.mode
		switch home {
		case modeSearch, modeNets:
			home = modeTable
		case modeCompare:
			home = modeParts
		case modeDiff:
			home = modeCheck
		}
		active := t.mode == home
		st := tabInactive
		if active {
			st = tabActive
		}
		segs = append(segs, st.Render(t.label))
	}
	tabsStr := strings.Join(segs, "")

	right := ""
	if len(m.items) > 0 {
		a, wn := m.counts()
		right = okStyle.Render(fmt.Sprintf(" %d/%d ", a, m.activeCount())) + warnBadge(wn)
	}

	bw, tw, rw := lipgloss.Width(brand), lipgloss.Width(tabsStr), lipgloss.Width(right)
	// Narrow terminals drop the brand and then the counts, in that order, before
	// the tabs give up a column — one column over and every line of the view pads
	// out to match, which slides the whole page.
	if bw+tw+rw+2 > m.w {
		brand, bw = "", 0
	}
	if bw+tw+rw+2 > m.w {
		right, rw = "", 0
	}
	leftPad := (m.w-tw)/2 - bw
	if leftPad < 1 {
		leftPad = 1
	}
	rightPad := m.w - bw - leftPad - tw - rw
	if rightPad < 1 {
		rightPad = 1
	}
	return padRender(brand+spaces(leftPad)+tabsStr+spaces(rightPad)+right, m.w)
}

// tabStart is the x column where the (centered) tab strip begins; tabBar and
// tabAtX must agree on it.
func (m Model) tabStart() int {
	bw := lipgloss.Width(tabBrand.Render("bomexpo"))
	var segs []string
	for _, t := range m.tabs() {
		segs = append(segs, tabInactive.Render(t.label))
	}
	leftPad := (m.w-lipgloss.Width(strings.Join(segs, "")))/2 - bw
	if leftPad < 1 {
		leftPad = 1
	}
	return bw + leftPad
}

func warnBadge(n int) string {
	if n == 0 {
		return okStyle.Render(" ✓ all clear ")
	}
	return badStyle.Render(fmt.Sprintf(" %d ⚠ ", n))
}

func (m Model) tabAtX(x int) (mode, bool) {
	cur := m.tabStart()
	for _, t := range m.tabs() {
		w := lipgloss.Width(tabInactive.Render(t.label))
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
	a, _ := m.counts()
	active := m.activeCount()
	frac := 0.0
	if active > 0 {
		frac = float64(a) / float64(active)
	}
	parts := []string{readinessBar(frac)}
	if m.boardW > 0 {
		parts = append(parts, seg("board", fmt.Sprintf("%.1f×%.1f mm", m.boardW, m.boardH)))
	}
	if m.layers > 0 {
		parts = append(parts, seg("layers", fmt.Sprintf("%d", m.layers)))
	}
	cost, complete := m.totalCost()
	costStr := fmt.Sprintf("$%.2f", cost)
	if !complete {
		costStr += "*"
	}
	parts = append(parts, dimStyle.Render("cost ")+okStyle.Render(costStr))
	if next := m.nextStep(); next != "" {
		parts = append(parts, next)
	}
	return strings.Join(parts, sepStyle.Render("  │  "))
}

func readinessBar(frac float64) string {
	const w = 16
	fill := int(frac*float64(w) + 0.5)
	if fill > w {
		fill = w
	}
	return dimStyle.Render("assembly ") +
		okStyle.Render(strings.Repeat("█", fill)) + dimStyle.Render(strings.Repeat("░", w-fill)) +
		subtleStyle.Render(fmt.Sprintf(" %d%%", int(frac*100+0.5)))
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
	// The bar has to fit the terminal exactly: one column over and every line of
	// the view gets padded to match, which shifts the whole page. Two columns are
	// held back for the gap so the status and the keys never read as one phrase.
	help := m.helpLine(m.w - lipgloss.Width(left) - 3)
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(help) - 1
	if gap < 2 {
		gap = 2
	}
	return " " + left + spaces(gap) + help
}

// helpLine lists the keys for whatever has focus, dropping hints off the end
// until they fit the room left over.
// nextStep names the stage that follows, once this one has nothing left to do. It
// only speaks when the work is actually finished — a nudge on a half-assigned board
// would just be noise in the way of the counts.
func (m Model) nextStep() string {
	arrow := func(n int, label string) string {
		return accentStyle.Render(fmt.Sprintf("→ %d %s", n, label))
	}
	switch m.mode {
	case modeTable, modeSearch, modeNets:
		if len(m.items) == 0 {
			return ""
		}
		if a, _ := m.counts(); a < m.activeCount() {
			return "" // still parts to assign
		}
		return arrow(4, "Export")
	}
	return ""
}

func (m Model) helpLine(budget int) string {
	var hints [][2]string
	switch m.mode {
	case modeLoad:
		switch {
		case m.load.cursor >= 0:
			hints = [][2]string{{"↑↓", "pick"}, {"enter", "open"}, {"tab", "type the path"}, tabHint(true)}
		case m.load.field.Focused():
			hints = [][2]string{{"↓", "browse"}, {"tab", "complete"}, {"enter", "open"},
				tabHint(false), {"ctrl+c", "quit"}}
		default:
			hints = [][2]string{{"/", "type a path"}, {"↓", "browse"}, {"enter", "open"},
				tabHint(true), {"ctrl+c", "quit"}}
		}
	case modeTable:
		if m.filter.open {
			// the dropdown shows the keys, so the hint is about driving it
			hints = [][2]string{{"type", "narrow"}, {"↑↓", "pick"}, {"tab", "complete"},
				{"enter", "go to the rows"}, {"esc", "clear"}}
			break
		}
		hints = [][2]string{{"enter", "assign"}, {"a", "auto"}, {"tab", "filter"}, {"n", "nets"},
			{"t/b/i", "3D"}, {"w", "save"}, tabHint(true)}
	case modeSearch:
		if m.search.field.Focused() {
			hints = [][2]string{{"↑↓", "results"}, {"enter", "pick"}, {"^f/^t/^s", "filters"},
				tabHint(false), {"esc", "back"}}
			break
		}
		// the filter and source letters are printed next to their own chips, so
		// repeating them here would just crowd out the navigation keys
		hints = [][2]string{{"↑↓", "results"}, {"enter", "pick"}, {"d", "datasheet"},
			{"/", "search"}, {"esc", "back"}}
	case modeParts:
		if m.cat.open {
			hints = [][2]string{{"↑↓←→", "pick"}, {"enter", "search inside it"},
				{"type", "narrow the list"}, {"esc", "back"}}
			break
		}
		if m.parts.field.Focused() {
			hints = [][2]string{{"↑↓", "results"}, {"enter", "pin"}, {"^d", "datasheet"},
				tabHint(false)}
			break
		}
		hints = [][2]string{{"↑↓", "results"}, {"p", "pin"}, {"c", "compare"}, {"d", "datasheet"},
			{"/", "search"}, {"t", "category"}, tabHint(true)}
	case modeCompare:
		hints = [][2]string{{"tab", "card"}, {"↑↓", "scroll"}, {"x", "unpin"}, {"d", "datasheet"},
			tabHint(true), {"esc", "back"}}
	case modeNets:
		if m.nets.field.Focused() {
			hints = [][2]string{{"↑↓", "nets"}, {"enter", "filter by it"}, tabHint(false),
				{"esc", "back"}}
			break
		}
		hints = [][2]string{{"↑↓", "nets"}, {"enter", "filter by it"}, {"/", "narrow"}, {"esc", "back"}}
	case modeCheck:
		switch m.check.pane {
		case paneOut:
			hints = [][2]string{{"type", "output path"}, {"enter", "export"}, {"tab", "issues"},
				{"esc", "done"}}
		case paneBoard:
			hints = [][2]string{{"↑↓←→", "pan"}, {"+-", "zoom"}, {"0", "reset"}, {"t/b/i", "3D"},
				{"tab", "output path"}, tabHint(true), {"esc", "back"}}
		default:
			hints = [][2]string{{"↑↓", "issues"}, {"enter", "open it"}, {"q", "board count"},
				{"v", "verify"}, {"x", "export"}, {"tab", "board"}, tabHint(true)}
		}
	case modeDiff:
		if m.diff.field.Focused() {
			hints = [][2]string{{"type", "bom path"}, {"enter", "compare"}, tabHint(false), {"esc", "back"}}
			break
		}
		hints = [][2]string{{"enter", "open in Components"}, {"↑↓", "move"}, {"o", "last order"},
			{"s", m.diff.show.String()}, {"m", "vs " + m.diff.ref.String()},
			{"r", "again"}, {"esc", "back to Export"}}
	}
	var parts []string
	width := 0
	for _, h := range hints {
		one := keyStyle.Render(h[0]) + " " + descStyle.Render(h[1])
		next := width + lipgloss.Width(one)
		if len(parts) > 0 {
			next += 2 // the separator
		}
		if len(parts) > 0 && next > budget {
			break // the earlier hints matter more than the later ones
		}
		parts = append(parts, one)
		width = next
	}
	return strings.Join(parts, sepStyle.Render("  "))
}
