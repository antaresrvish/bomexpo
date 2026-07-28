package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/export"
	"bomexpo/internal/kicad"
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
	if click && ms.Button == tea.MouseLeft && ms.Y == 2 {
		if k, ok := colSortKey(layoutCols(m.contentW()), ms.X); ok {
			if m.sort == k {
				m.sortAsc = !m.sortAsc
			} else {
				m.sort, m.sortAsc = k, true
			}
			return m.sorted(), nil
		}
		return m, nil
	}
	if click && ms.Button == tea.MouseLeft {
		row := m.top + (ms.Y - dataTop)
		if row >= 0 && row < len(m.items) {
			m.cursor = row
			m.clampScroll()
			if p := m.assigned[row]; p != nil && p.Datasheet != "" {
				if lo, hi := layoutCols(m.contentW()).dsRange(); ms.X >= lo && ms.X < hi {
					openExternal(p.Datasheet)
				}
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

func layoutCols(w int) cols {
	c := cols{ref: 9, val: 10, fp: 18, qty: 4, code: 9, stock: 9, price: 8, ds: 9, rot: 5}
	fixed := 2 + c.ref + c.val + c.fp + c.qty + c.code + c.stock + c.price + c.ds + c.rot
	c.note = w - fixed - 3*9
	if c.note < 6 {
		c.note = 6
	}
	return c
}

func (m Model) viewTable(w, h int) string {
	if len(m.items) == 0 {
		return subtleStyle.Render("no components")
	}
	c := layoutCols(w)
	sep := sepStyle.Render(" │ ")

	sh := func(label string, k sortKey) string {
		if m.sort == k {
			if m.sortAsc {
				return label + "▲"
			}
			return label + "▼"
		}
		return label
	}
	headCells := []string{
		pad("", 1), pad(sh("REF", sortRef), c.ref), pad(sh("VALUE", sortVal), c.val),
		pad(sh("FOOTPRINT", sortFp), c.fp), pad(sh("QTY", sortQty), c.qty),
		pad(sh("LCSC", sortCode), c.code), pad(sh("STOCK", sortStock), c.stock),
		pad(sh("PRICE", sortPrice), c.price), pad("DATASHEET", c.ds),
		pad(sh("ROT", sortRot), c.rot), pad("NOTE", c.note),
	}
	head := colHeadStyle.Render(padRender(headCells[0]+" "+strings.Join(headCells[1:], " | "), w))
	rule := borderStyle.Render(strings.Repeat("─", w))

	vis := m.visibleRows()
	end := min(len(m.items), m.top+vis)
	lines := []string{head, rule}
	for i := m.top; i < end; i++ {
		lines = append(lines, m.rowView(i, c, sep, w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) rowView(i int, c cols, sep string, w int) string {
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
		return selRowStyle.Render(padRender(line, w))
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
	return padRender(line, w)
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

func (c cols) dsRange() (int, int) {
	start := 4 + c.ref + c.val + c.fp + c.qty + c.code + c.stock + c.price + 3*7
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
