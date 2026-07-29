package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bomexpo/internal/export"
	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
	"bomexpo/internal/render"
	"bomexpo/internal/value"
)

const dataTop = 4 // tab(1) + border(1) + colhead(1) + rule(1)

func (m Model) visibleRows() int {
	n := m.contentH() - 3 // header, rule, horizontal scrollbar
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) updateTable(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab":
		return m.cycleTab(1)
	case "shift+tab":
		return m.cycleTab(-1)
	case "1", "3", "4", "5":
		// tab by number; 2 is Components itself, so it stays unbound
		if md, ok := m.tabMode(int(msg.String()[0] - '0')); ok {
			return m.gotoTab(md)
		}
		return m, nil
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(m.items)-1, m.cursor+1)
	case "left", "h":
		m.hoff = clampInt(m.hoff-8, 0, m.maxHoff())
	case "right", "l":
		m.hoff = clampInt(m.hoff+8, 0, m.maxHoff())
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.items) - 1
	case "pgup":
		m.cursor = max(0, m.cursor-m.visibleRows())
	case "pgdown":
		m.cursor = min(len(m.items)-1, m.cursor+m.visibleRows())
	case "enter", " ":
		return m.openSearch(m.cursor)
	case "x":
		if m.cursor >= 0 && m.cursor < len(m.excluded) {
			m.excluded[m.cursor] = !m.excluded[m.cursor]
		}
	case "a":
		return m.startAutoAssign()
	case "t":
		return m.openRender("top")
	case "b":
		return m.openRender("bottom")
	case "i":
		return m.openRender("iso")
	case "r":
		return m.refreshCmd()
	case "o":
		return m.cycleRotOverride()
	case "O":
		return m.resetRotOverride()
	case "w":
		return m, m.saveCmd()
	case "d":
		m.openDatasheet(m.cursor)
	}
	m.clampScroll()
	return m, nil
}

func (m Model) startAutoAssign() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.items {
		st := m.stateOf(i)
		if st != stUnassigned && st != stOutOfStock {
			continue
		}
		if searchKeyword(m.items[i]) == "" {
			continue
		}
		cmds = append(cmds, m.autoAssignCmd(i))
	}
	if len(cmds) == 0 {
		m.status = "nothing to auto-assign"
		return m, nil
	}
	m.autoRemaining = len(cmds)
	m.autoOK = 0
	m.loading = true
	m.status = fmt.Sprintf("Auto-assigning %d parts…", len(cmds))
	return m, tea.Batch(cmds...)
}

func (m Model) mouseTable(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	h := m.contentH()
	tableW := m.tableW()
	visRows := h - 3
	vbarX := 2 + tableW
	hbarY := 2 + h - 1
	left := ms.Button == tea.MouseLeft

	if wheel {
		switch ms.Button {
		case tea.MouseWheelUp:
			m.cursor = max(0, m.cursor-1)
		case tea.MouseWheelDown:
			m.cursor = min(len(m.items)-1, m.cursor+1)
		}
		m.clampScroll()
		return m, nil
	}

	if !click { // motion: continue an in-progress scrollbar drag, or end it
		if !left {
			m.drag = dragNone
			return m, nil
		}
		switch m.drag {
		case dragVert:
			return m.vScrollTo(ms.Y), nil
		case dragHorz:
			return m.hScrollTo(ms.X), nil
		}
		return m, nil
	}
	if !left {
		return m, nil
	}

	// grab a scrollbar?
	if ms.X == vbarX && ms.Y >= dataTop && ms.Y < dataTop+visRows {
		m.drag = dragVert
		return m.vScrollTo(ms.Y), nil
	}
	if ms.Y == hbarY {
		if bx := ms.X - 2; bx >= 0 && bx < tableW {
			m.drag = dragHorz
			return m.hScrollTo(ms.X), nil
		}
	}
	m.drag = dragNone

	// board side buttons live in the sidebar's board-header row
	if sidebarW(m.contentW()) > 0 {
		topH := (h - 1) / 2
		if ms.Y == 2+topH+1 {
			for _, b := range boardButtonSpans(2 + tableW + 2) {
				if ms.X >= b.lo && ms.X < b.hi {
					return m.openRender(b.side)
				}
			}
		}
	}

	bx := ms.X - 2
	if bx < 0 || bx >= tableW {
		return m, nil // sidebar
	}
	c := layoutCols(tableW)
	lineX := bx + clampInt(m.hoff, 0, m.maxHoff())

	if ms.Y == 2 { // header → sort by column
		if k, ok := colSortKey(c, lineX); ok {
			if m.sort == k {
				m.sortAsc = !m.sortAsc
			} else {
				m.sort, m.sortAsc = k, true
			}
			return m.sorted(), nil
		}
		return m, nil
	}
	row := m.top + (ms.Y - dataTop)
	if ms.Y >= dataTop && ms.Y < dataTop+visRows && row >= 0 && row < len(m.items) {
		m.cursor = row
		m.clampScroll()
		if p := m.assigned[row]; p != nil && p.Datasheet != "" {
			if lo, hi := c.dsRange(); lineX >= lo && lineX < hi {
				openExternal(p.Datasheet)
			}
		}
	}
	return m, nil
}

func (m *Model) clampScroll() {
	if len(m.items) == 0 {
		m.cursor, m.top = 0, 0
		return
	}
	m.cursor = clampInt(m.cursor, 0, len(m.items)-1)
	vis := m.visibleRows()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+vis {
		m.top = m.cursor - vis + 1
	}
	m.top = clampInt(m.top, 0, max(0, len(m.items)-1))
}

type cols struct{ ref, val, fp, qty, code, stock, price, ds, rot, note int }

// layoutCols flexes the NOTE column to fill the table viewport, with a minimum
// width; when the fixed columns already overflow, note stays at the minimum and
// the viewport scrolls horizontally.
func layoutCols(tableW int) cols {
	// stock holds a grouped 9-digit number: an assembler's stock runs into the
	// tens of millions and eliding it hides the useful digits.
	c := cols{ref: 9, val: 10, fp: 18, qty: 4, code: 9, stock: 11, price: 8, ds: 9, rot: 5}
	c.note = tableW - 2 - c.ref - c.val - c.fp - c.qty - c.code - c.stock - c.price - c.ds - c.rot - 3*9
	if c.note < 24 {
		c.note = 24
	}
	return c
}

// fullWidth is the natural width of a rendered row: icon + space, every column,
// and a 3-col separator between the ten cells.
func (c cols) fullWidth() int {
	return 2 + c.ref + c.val + c.fp + c.qty + c.code + c.stock + c.price + c.ds + c.rot + c.note + 3*9
}

func sidebarW(w int) int {
	if w < 80 {
		return 0
	}
	sw := w * 2 / 5
	if sw > 48 {
		sw = 48
	}
	return sw
}

func (m Model) tableW() int {
	w := m.contentW()
	if sw := sidebarW(w); sw > 0 {
		return w - sw - 2 // vertical scrollbar + a gap before the sidebar
	}
	return w - 1 // reserve 1 column for the vertical scrollbar
}

func (m Model) maxHoff() int {
	tw := m.tableW()
	if over := layoutCols(tw).fullWidth() - tw; over > 0 {
		return over
	}
	return 0
}

func (m Model) viewTable(w, h int) string {
	if len(m.items) == 0 {
		return subtleStyle.Render("no components")
	}
	sideW := sidebarW(w)
	tableW := w - 1
	if sideW > 0 {
		tableW = w - sideW - 2
	}
	c := layoutCols(tableW)
	tbl := m.tableBlock(c, tableW, h)
	vbar := m.vScrollCol(h)
	var side []string
	if sideW > 0 {
		side = m.sidebarBlock(sideW, h)
	}
	out := make([]string, h)
	for i := 0; i < h; i++ {
		line := tbl[i] + vbar[i]
		if sideW > 0 {
			line += " " + side[i]
		}
		out[i] = line
	}
	return strings.Join(out, "\n")
}

// tableBlock renders the header, rule and visible rows at full width, crops
// each to the viewport [hoff, hoff+tableW], and caps it with a horizontal
// scrollbar on the last line.
func (m Model) tableBlock(c cols, tableW, h int) []string {
	full := c.fullWidth()
	hoff := clampInt(m.hoff, 0, max(0, full-tableW))
	crop := func(s string) string {
		s = ansi.Cut(s, hoff, hoff+tableW)
		if p := tableW - lipgloss.Width(s); p > 0 {
			s += spaces(p)
		}
		return s
	}
	sep := sepStyle.Render(" │ ")
	head := colHeadStyle.Render(padRender(m.headRow(c), full))
	rule := borderStyle.Render(strings.Repeat("─", full))
	lines := []string{crop(head), crop(rule)}
	visRows := h - 3
	end := min(len(m.items), m.top+visRows)
	for i := m.top; i < end; i++ {
		lines = append(lines, crop(m.rowView(i, c, sep)))
	}
	for len(lines) < h-1 {
		lines = append(lines, spaces(tableW))
	}
	lines = append(lines, hScrollRow(tableW, full, tableW, hoff))
	return lines[:h]
}

func hScrollRow(tableW, total, vis, hoff int) string {
	if total <= vis {
		return borderStyle.Render(strings.Repeat("─", tableW))
	}
	thumb := max(1, vis*tableW/total)
	if thumb > tableW {
		thumb = tableW
	}
	pos := hoff * (tableW - thumb) / max(1, total-vis)
	var b strings.Builder
	for i := 0; i < tableW; i++ {
		if i >= pos && i < pos+thumb {
			b.WriteString(accentStyle.Render("━"))
		} else {
			b.WriteString(borderStyle.Render("─"))
		}
	}
	return b.String()
}

// vScrollCol is the vertical scrollbar drawn between the table and the sidebar.
func (m Model) vScrollCol(h int) []string {
	total := len(m.items)
	vis := h - 3
	top := clampInt(m.top, 0, max(0, total-vis))
	trackTop, trackLen := 2, h-3
	thumb, pos := trackLen, 0
	if total > vis && vis > 0 {
		thumb = max(1, vis*trackLen/total)
		if thumb > trackLen {
			thumb = trackLen
		}
		pos = top * (trackLen - thumb) / max(1, total-vis)
	}
	col := make([]string, h)
	for i := 0; i < h; i++ {
		if r := i - trackTop; i >= trackTop && i < trackTop+trackLen && r >= pos && r < pos+thumb {
			col[i] = accentStyle.Render("█")
		} else {
			col[i] = borderStyle.Render("│")
		}
	}
	return col
}

func (m Model) vScrollTo(screenY int) Model {
	total := len(m.items)
	vis := m.visibleRows()
	if total <= vis {
		return m
	}
	rel := clampInt(screenY-dataTop, 0, vis-1)
	m.top = clampInt(rel*(total-vis)/max(1, vis-1), 0, total-vis)
	if m.cursor < m.top {
		m.cursor = m.top
	}
	if m.cursor >= m.top+vis {
		m.cursor = m.top + vis - 1
	}
	return m
}

func (m Model) hScrollTo(screenX int) Model {
	tableW := m.tableW()
	bx := clampInt(screenX-2, 0, tableW-1)
	m.hoff = clampInt(bx*m.maxHoff()/max(1, tableW-1), 0, m.maxHoff())
	return m
}

func (m Model) headRow(c cols) string {
	sh := func(label string, k sortKey) string {
		if m.sort == k {
			if m.sortAsc {
				return label + "▲"
			}
			return label + "▼"
		}
		return label
	}
	cells := []string{
		pad("", 1), pad(sh("REF", sortRef), c.ref), pad(sh("VALUE", sortVal), c.val),
		pad(sh("FOOTPRINT", sortFp), c.fp), pad(sh("QTY", sortQty), c.qty),
		pad(sh("LCSC", sortCode), c.code), pad(sh("STOCK", sortStock), c.stock),
		pad(sh("PRICE", sortPrice), c.price), pad("DATASHEET", c.ds),
		pad(sh("ROT", sortRot), c.rot), pad("NOTE", c.note),
	}
	return cells[0] + " " + strings.Join(cells[1:], " | ")
}

// sidebarBlock is the right panel: boxed overview stats on top and a fixed,
// shrunk board render (traces on) below, split in half.
func (m Model) sidebarBlock(sideW, h int) []string {
	topH := (h - 1) / 2
	botH := h - 1 - topH
	lines := make([]string, 0, h)
	ov := m.compactOverview(sideW, topH)
	for i := 0; i < topH; i++ {
		if i < len(ov) {
			lines = append(lines, padRender(ov[i], sideW))
		} else {
			lines = append(lines, spaces(sideW))
		}
	}
	lines = append(lines, borderStyle.Render(strings.Repeat("─", sideW)))
	lines = append(lines, padRender(m.boardHeader(), sideW))
	bd := m.miniBoard(sideW, botH-1)
	for i := 0; i < botH-1; i++ {
		if i < len(bd) {
			lines = append(lines, padRender(bd[i], sideW))
		} else {
			lines = append(lines, spaces(sideW))
		}
	}
	return lines
}

func (m Model) compactOverview(sideW, avail int) []string {
	var un, oos, mm int
	for i := range m.items {
		switch m.stateOf(i) {
		case stUnassigned:
			un++
		case stOutOfStock:
			oos++
		case stMismatch:
			mm++
		}
	}
	assigned, _ := m.counts()
	active := m.activeCount()
	tw := (sideW - 1) / 2
	tile := func(label, value string, vs lipgloss.Style) string { return sideTile(label, value, vs, tw) }
	row := func(a, b string) string { return lipgloss.JoinHorizontal(lipgloss.Top, a, " ", b) }
	grid := lipgloss.JoinVertical(lipgloss.Left,
		row(tile("assigned", fmt.Sprintf("%d/%d", assigned, active), okStyle),
			tile("unassigned", fmt.Sprintf("%d", un), hotStyle(un, warnStyle))),
		row(tile("no stock", fmt.Sprintf("%d", oos), hotStyle(oos, badStyle)),
			tile("mismatch", fmt.Sprintf("%d", mm), hotStyle(mm, warnStyle))),
	)
	nRot := len(export.RotationFixes(m.placements, m.excludeSet(), m.rotOverrideMap()))
	sum := fmt.Sprintf("excluded %d · dnp %d · rot %d", m.excludedCount(), m.dnpCount(), nRot)
	if n := m.extCount(); n > 0 {
		sum += fmt.Sprintf(" · ext %d", n)
	}
	summary := dimStyle.Render(sum)

	lines := append([]string{accentStyle.Render("Overview")}, strings.Split(grid, "\n")...)
	lines = append(lines, summary)
	if budget := avail - len(lines); budget >= 3 {
		lines = append(lines, m.selectedInspector(sideW, budget)...)
	}
	return lines
}

// selectedInspector renders a labeled card for the highlighted row so the side
// panel doubles as a live inspector.
func (m Model) selectedInspector(sideW, budget int) []string {
	i := m.cursor
	if i < 0 || i >= len(m.items) || budget < 3 {
		return nil
	}
	it := m.items[i]
	ref := it.ID()
	if it.PerBoard() > 1 {
		ref = fmt.Sprintf("%s ×%d", it.ID(), it.PerBoard())
	}
	icon, note, ns := stateDecor(m.stateOf(i))
	if it.DNP {
		note, ns = "do not populate", dimStyle
	} else if note == "" {
		note, ns = "ready", okStyle
	}
	kv := func(label, val string) string { return dimStyle.Render(pad(label, 10)) + val }

	rows := []string{
		kv("status", icon+" "+ns.Render(note)),
		kv("value", subtleStyle.Render(it.Value)),
	}
	if p := m.assigned[i]; p != nil {
		code := codeStyle.Render(it.LCSC)
		if p.Brand != "" {
			code += dimStyle.Render(" · ") + subtleStyle.Render(p.Brand)
		}
		// Order matters: a short sidebar truncates from the bottom, so the
		// numbers you check every time come before the extras.
		rows = append(rows,
			kv("lcsc", code),
			kv("desc", dimStyle.Render(p.Description())),
			kv("stock", okStyle.Render(groupThousands(p.Stock))),
			kv("price", warnStyle.Render(p.PriceLabel())),
		)
		if p.Lib.Known() {
			rows = append(rows, kv("library", libCell(p.Lib, p.Lib.String())))
		}
		if p.MinBuy > 1 {
			rows = append(rows, kv("moq", subtleStyle.Render(fmt.Sprintf("%d", p.MinBuy))))
		}
		if p.AsmMin > 1 || p.Loss > 0 {
			rows = append(rows, kv("assembly",
				subtleStyle.Render(fmt.Sprintf("min %d · loss %d", p.AsmMin, p.Loss))))
		}
		if s := p.Specs(); s != "" {
			rows = append(rows, kv("specs", dimStyle.Render(s)))
		}
	} else if it.LCSC != "" {
		rows = append(rows, kv("lcsc", codeStyle.Render(it.LCSC)+dimStyle.Render(" loading…")))
	} else {
		rows = append(rows, kv("lcsc", dimStyle.Render("— unassigned")))
	}
	rt, rs := rotCell(it)
	rows = append(rows, kv("footprint", dimStyle.Render(it.Footprint)), kv("rotation", rs.Render(rt)))

	if n := budget - 2; len(rows) > n {
		rows = rows[:n]
	}
	return sideCard(sideW, "◆ "+ref, rows)
}

// sideCard frames labeled rows in a titled rounded box sized to the sidebar.
func sideCard(sideW int, title string, rows []string) []string {
	inner := sideW - 2
	fill := sideW - 4 - lipgloss.Width(title)
	if fill < 0 {
		fill = 0
	}
	out := []string{borderStyle.Render("╭ ") + accentStyle.Render(title) + borderStyle.Render(" "+strings.Repeat("─", fill)+"╮")}
	for _, r := range rows {
		out = append(out, borderStyle.Render("│")+padRender(r, inner)+borderStyle.Render("│"))
	}
	return append(out, borderStyle.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

func (m Model) miniBoard(w, h int) []string {
	if m.board == nil || m.board.Empty() || h < 2 {
		return []string{dimStyle.Render("no board outline")}
	}
	hl := map[string]bool{}
	if m.cursor >= 0 && m.cursor < len(m.items) {
		for _, d := range m.items[m.cursor].Designators {
			hl[d] = true
		}
	}
	img := render.Render(m.board, m.placements, render.Options{W: w, H: h, ShowCopper: true, Highlight: hl})
	if img == "" {
		return []string{dimStyle.Render("board too small")}
	}
	return strings.Split(img, "\n")
}

// boardSides pairs each button's display text (the bracketed mnemonic doubles
// as the keyboard hint) with the render side it selects.
var boardSides = []struct{ text, side string }{
	{"[t]op", "top"}, {"[b]ot", "bottom"}, {"[i]so", "iso"},
}

// boardHeader draws the "Board [t]op [b]ot [i]so" row; the selected side is
// highlighted. boardButtonSpans must stay in sync with its layout.
func (m Model) boardHeader() string {
	sel := m.boardv.sideOr()
	b := accentStyle.Render("Board") + " "
	for _, s := range boardSides {
		if s.side == sel {
			b += cursorStyle.Render(s.text)
		} else {
			b += dimStyle.Render(s.text)
		}
		b += " "
	}
	return b
}

// boardButtonSpans returns the screen-x ranges of the side buttons, given where
// the sidebar content starts on screen.
func boardButtonSpans(sideStartX int) []struct {
	lo, hi int
	side   string
} {
	x := sideStartX + 6 // "Board "
	var out []struct {
		lo, hi int
		side   string
	}
	for _, s := range boardSides {
		w := lipgloss.Width(s.text)
		out = append(out, struct {
			lo, hi int
			side   string
		}{x, x + w, s.side})
		x += w + 1
	}
	return out
}

func (m Model) rowView(i int, c cols, sep string) string {
	full := c.fullWidth()
	it := m.items[i]
	st := m.stateOf(i)
	icon, note, noteStyle := stateDecor(st)
	if it.DNP {
		note, noteStyle = "do not populate", dimStyle
	}
	rotText, rotStyle := rotCell(it)

	code := it.LCSC
	if code == "" {
		code = "—"
	}
	stock, price, ds := "—", "—", ""
	if p := m.assigned[i]; p != nil {
		stock = groupThousands(p.Stock)
		price = p.PriceLabel()
		if p.Datasheet != "" {
			ds = "datasheet"
		}
		if note == "" {
			note, noteStyle = p.Description(), descStyle
		}
	}
	ref := it.ID()
	if it.PerBoard() > 1 {
		ref = fmt.Sprintf("%s ×%d", it.ID(), it.PerBoard())
	}

	plain := []string{
		pad(ref, c.ref), pad(it.Value, c.val), pad(it.Footprint, c.fp),
		pad(fmt.Sprintf("%d", it.Quantity), c.qty), pad(code, c.code),
		pad(stock, c.stock), pad(price, c.price), pad(ds, c.ds),
		pad(rotText, c.rot), pad(note, c.note),
	}

	if i == m.cursor {
		line := "▶ " + strings.Join(plain, "   ")
		return selRowStyle.Render(padRender(line, full))
	}

	colored := []string{
		codeStyle.Render(plain[0]),
		plain[1],
		subtleStyle.Render(plain[2]),
		dimStyle.Render(plain[3]),
		codeCell(code, m.libOf(i), plain[4]),
		stockCell(stock, plain[5]),
		warnStyle.Render(plain[6]),
		dsCellStyle(ds, plain[7]),
		rotStyle.Render(plain[8]),
		noteStyle.Render(plain[9]),
	}
	line := icon + " " + strings.Join(colored, sep)
	return padRender(line, full)
}

func dsCellStyle(ds, s string) string {
	if ds == "" {
		return s
	}
	return linkStyle.Render(s)
}

// colSortKey maps a click x on the header row to the column's sort key. The ds
// and note columns are not sortable.
func colSortKey(c cols, x int) (sortKey, bool) {
	spans := []struct {
		w        int
		k        sortKey
		sortable bool
	}{
		{c.ref, sortRef, true}, {c.val, sortVal, true}, {c.fp, sortFp, true},
		{c.qty, sortQty, true}, {c.code, sortCode, true}, {c.stock, sortStock, true},
		{c.price, sortPrice, true}, {c.ds, sortNone, false}, {c.rot, sortRot, true},
		{c.note, sortNone, false},
	}
	pos := 2 // icon (1) + space (1)
	for _, s := range spans {
		if x >= pos && x < pos+s.w {
			return s.k, s.sortable
		}
		pos += s.w + 3 // + column separator
	}
	return sortNone, false
}

// dsRange is the datasheet column's [start,end) in row-line coordinates (icon
// at 0). Callers convert screen x to this space (subtract the panel offset, add
// the horizontal scroll).
func (c cols) dsRange() (int, int) {
	start := 2 + c.ref + c.val + c.fp + c.qty + c.code + c.stock + c.price + 3*7
	return start, start + c.ds
}

func (m Model) openDatasheet(idx int) {
	if idx >= 0 && idx < len(m.assigned) {
		if p := m.assigned[idx]; p != nil && p.Datasheet != "" {
			openExternal(p.Datasheet)
		}
	}
}

func rotCell(it kicad.Item) (string, lipgloss.Style) {
	if it.HasRotOverride {
		return fmt.Sprintf("%d°", it.RotOverride), warnStyle
	}
	return fmt.Sprintf("%.0f°", export.FamilyOffset(it.Footprint)), okStyle
}

// codeCell colours the part code by what it costs to assemble when the source
// told us, so an order full of extended parts is visible without a new column.
func codeCell(code string, k part.LibKind, s string) string {
	if code == "—" {
		return dimStyle.Render(s)
	}
	if k.Known() {
		return libCell(k, s)
	}
	return accentStyle.Render(s)
}

func stockCell(stock, s string) string {
	if stock == "—" {
		return dimStyle.Render(s)
	}
	return okStyle.Render(s)
}

func stateDecor(st itemState) (icon, note string, style lipgloss.Style) {
	switch st {
	case stOK:
		return okStyle.Render("●"), "", descStyle
	case stUnassigned:
		return dimStyle.Render("○"), "needs LCSC part", dimStyle
	case stOutOfStock:
		return badStyle.Render("✗"), "OUT OF STOCK", badStyle
	case stMismatch:
		return warnStyle.Render("⚠"), "value mismatch", warnStyle
	case stExcluded:
		return dimStyle.Render("∅"), "excluded from BOM", dimStyle
	}
	return " ", "", descStyle
}

func (m Model) openSearch(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.items) {
		return m, nil
	}
	m.cursor = i
	m.mode = modeSearch
	it := m.items[i]
	kw := searchKeyword(it)
	m.search.begin(kw, it.Footprint, it.Value, refPrefix(it.ID()))
	m.search.token++
	return m, m.searchCmd(m.search.token, kw)
}

var sizeCode = regexp.MustCompile(`(01005|0201|0402|0603|0805|1206|1210|2010|2512)`)

// searchKeyword builds the LCSC query from a component's value. LCSC only
// value-searches when the keyword is the normalized value alone (e.g. "2kΩ",
// "100nF"); appending the package pollutes it with popular parts, so the
// package stays a client-side filter.
func searchKeyword(it kicad.Item) string {
	if v, ok := value.ExtractValue(it.Value); ok {
		return value.Format(v)
	}
	return strings.TrimSpace(it.Value)
}

func groupThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		b.WriteByte(',')
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
