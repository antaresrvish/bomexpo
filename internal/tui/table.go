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
	"bomexpo/internal/render"
	"bomexpo/internal/value"
)

const dataTop = 4 // tab(1) + border(1) + colhead(1) + rule(1)

func (m Model) visibleRows() int {
	n := m.contentH() - 2
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
	case "1":
		return m.gotoTab(modeLoad)
	case "2":
		return m.gotoTab(modeOverview)
	case "4":
		return m.gotoTab(modeBoard)
	case "5":
		return m.gotoTab(modeCheck)
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
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	bx := ms.X - 2 // strip the panel bar + space
	if bx < 0 || bx >= m.tableW() {
		return m, nil // click landed on the sidebar
	}
	c := layoutCols()
	lineX := bx + clampInt(m.hoff, 0, m.maxHoff())

	if ms.Y == 2 { // header row → sort by column
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
	if row >= 0 && row < len(m.items) {
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

// layoutCols is a fixed-width layout; the table renders at its natural width
// and the viewport scrolls horizontally over it.
func layoutCols() cols {
	return cols{ref: 9, val: 10, fp: 18, qty: 4, code: 9, stock: 9, price: 8, ds: 9, rot: 5, note: 24}
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
		return w - sw - 1
	}
	return w
}

func (m Model) maxHoff() int {
	if over := layoutCols().fullWidth() - m.tableW(); over > 0 {
		return over
	}
	return 0
}

func (m Model) viewTable(w, h int) string {
	if len(m.items) == 0 {
		return subtleStyle.Render("no components")
	}
	c := layoutCols()
	sideW := sidebarW(w)
	tableW := w
	if sideW > 0 {
		tableW = w - sideW - 1
	}

	tbl := m.tableBlock(c, tableW, h)
	if sideW == 0 {
		return strings.Join(tbl, "\n")
	}
	side := m.sidebarBlock(sideW, h)
	bar := borderStyle.Render("│")
	out := make([]string, h)
	for i := 0; i < h; i++ {
		out[i] = tbl[i] + bar + side[i]
	}
	return strings.Join(out, "\n")
}

// tableBlock renders the header, rule and visible rows at full width, then
// horizontally crops each line to the viewport [hoff, hoff+tableW].
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
	end := min(len(m.items), m.top+(h-2))
	for i := m.top; i < end; i++ {
		lines = append(lines, crop(m.rowView(i, c, sep)))
	}
	for len(lines) < h {
		lines = append(lines, spaces(tableW))
	}
	return lines[:h]
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

// sidebarBlock is the right panel: a compact overview on top and a fixed,
// shrunk board render below, split in half.
func (m Model) sidebarBlock(sideW, h int) []string {
	topH := (h - 1) / 2
	botH := h - 1 - topH
	lines := make([]string, 0, h)
	ov := m.compactOverview()
	for i := 0; i < topH; i++ {
		if i < len(ov) {
			lines = append(lines, padRender(ov[i], sideW))
		} else {
			lines = append(lines, spaces(sideW))
		}
	}
	lines = append(lines, borderStyle.Render(strings.Repeat("─", sideW)))
	bd := m.miniBoard(sideW, botH)
	for i := 0; i < botH; i++ {
		if i < len(bd) {
			lines = append(lines, padRender(bd[i], sideW))
		} else {
			lines = append(lines, spaces(sideW))
		}
	}
	return lines
}

func (m Model) compactOverview() []string {
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
	frac := 0.0
	if active > 0 {
		frac = float64(assigned) / float64(active)
	}
	kv := func(k, v string) string { return dimStyle.Render(pad(k, 11)) + v }
	nRot := len(export.RotationFixes(m.placements, m.excludeSet(), m.rotOverrideMap()))
	cost, complete := m.costAt(1)
	costStr := fmt.Sprintf("$%.2f", cost)
	if !complete {
		costStr += "*"
	}
	return []string{
		accentStyle.Render("Overview"),
		progressBar(frac, 12) + subtleStyle.Render(fmt.Sprintf(" %d%%", int(frac*100+0.5))),
		kv("assigned", okStyle.Render(fmt.Sprintf("%d/%d", assigned, active))),
		kv("unassigned", hotStyle(un, warnStyle).Render(fmt.Sprintf("%d", un))),
		kv("no stock", hotStyle(oos, badStyle).Render(fmt.Sprintf("%d", oos))),
		kv("mismatch", hotStyle(mm, warnStyle).Render(fmt.Sprintf("%d", mm))),
		kv("excluded", fmt.Sprintf("%d", m.excludedCount())),
		kv("dnp", fmt.Sprintf("%d", m.dnpCount())),
		kv("board", boardSize(m.boardW, m.boardH)),
		kv("layers", dash(m.layers > 0, fmt.Sprintf("%d", m.layers))),
		kv("est cost", costStr+dimStyle.Render(" qty1")),
		kv("rotation", dash(nRot > 0, fmt.Sprintf("%d fixed", nRot))),
	}
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
	img := render.Render(m.board, m.placements, render.Options{W: w, H: h, Highlight: hl})
	if img == "" {
		return []string{dimStyle.Render("board too small")}
	}
	return strings.Split(img, "\n")
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
		codeCell(code, plain[4]),
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

func codeCell(code, s string) string {
	if code == "—" {
		return dimStyle.Render(s)
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
