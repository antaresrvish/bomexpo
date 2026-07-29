package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/part"
)

const (
	// Vendor parameter names are long ("Program Storage Size", "Number of I/O"),
	// and eliding them to a dozen columns makes two rows indistinguishable.
	// Budget: the differs mark, the label, and a guaranteed gap before the
	// first value.
	cmpLabelW = 22
	// cmpMinColW is the narrowest a part column can get before paging is better
	// than squashing.
	cmpMinColW = 18
)

// compareState is the scroll and selection position in the matrix; the parts
// themselves live in partsState.pinned.
type compareState struct {
	top   int // first visible row
	sel   int // focused column, an index into pinned
	first int // leftmost visible column
}

// cmpRow is one line of the matrix: a label and one value per pinned part.
type cmpRow struct {
	label  string
	vals   []string
	differ bool // values are not all the same — the whole point of the view
	best   int  // column with the best value, or -1 when there's no ordering
	rule   bool // a separator rather than data
}

// compareRows builds the matrix: the numbers you decide on first, then every
// parameter any of the parts reports, with the ones they all share first.
func compareRows(ps []part.Part) []cmpRow {
	if len(ps) == 0 {
		return nil
	}
	get := func(f func(part.Part) string) []string {
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i] = f(p)
		}
		return out
	}
	dash := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return s
	}

	// best is -1 unless the values have a meaningful ordering; 0 is a real
	// column index, so it can't double as "none".
	plain := func(label string, f func(part.Part) string) cmpRow {
		return cmpRow{label: label, vals: get(f), best: -1}
	}

	rows := []cmpRow{
		plain("source", func(p part.Part) string { return p.Source }),
		plain("mpn", func(p part.Part) string { return dash(p.MPN) }),
		plain("brand", func(p part.Part) string { return dash(p.Brand) }),
		plain("package", func(p part.Part) string { return dash(p.Package) }),
		plain("library", func(p part.Part) string { return libText(p.Lib) }),
		{
			label: "stock",
			vals:  get(func(p part.Part) string { return groupThousands(p.Stock) }),
			best:  argBest(ps, func(p part.Part) (float64, bool) { return float64(p.Stock), true }, true),
		},
		{
			label: "unit price",
			vals:  get(func(p part.Part) string { return p.PriceLabel() }),
			best:  argBest(ps, func(p part.Part) (float64, bool) { return p.UnitPrice() }, false),
		},
		plain("moq", func(p part.Part) string { return countText(p.MinBuy) }),
	}
	if anyAssembly(ps) {
		rows = append(rows, plain("asm min", func(p part.Part) string {
			if p.AsmMin == 0 && p.Loss == 0 {
				return "—"
			}
			return fmt.Sprintf("%d +%d loss", p.AsmMin, p.Loss)
		}))
	}

	if params := paramRows(ps); len(params) > 0 {
		rows = append(rows, cmpRow{rule: true, best: -1})
		rows = append(rows, params...)
	}

	for i := range rows {
		if !rows[i].rule {
			rows[i].differ = !allSame(rows[i].vals)
		}
	}
	return rows
}

// paramRows turns the union of the parts' parameters into rows, keeping each
// source's own ordering and putting parameters every part reports first — those
// are the ones you can actually compare.
func paramRows(ps []part.Part) []cmpRow {
	var order []string
	seen := map[string]bool{}
	byPart := make([]map[string]string, len(ps))
	for i, p := range ps {
		byPart[i] = map[string]string{}
		for _, pr := range p.Params {
			key := strings.TrimSpace(pr.Name)
			if key == "" {
				continue
			}
			if _, dup := byPart[i][key]; !dup {
				byPart[i][key] = pr.Value
			}
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
		}
	}

	var shared, partial []cmpRow
	for _, key := range order {
		vals := make([]string, len(ps))
		have := 0
		for i := range ps {
			if v, ok := byPart[i][key]; ok && strings.TrimSpace(v) != "" {
				vals[i], have = v, have+1
				continue
			}
			vals[i] = "—"
		}
		row := cmpRow{label: strings.ToLower(key), vals: vals, best: -1}
		if have == len(ps) {
			shared = append(shared, row)
			continue
		}
		partial = append(partial, row)
	}
	return append(shared, partial...)
}

// argBest returns the index of the highest (or lowest) value, or -1 when fewer
// than two parts report one.
func argBest(ps []part.Part, f func(part.Part) (float64, bool), high bool) int {
	best, bestI, n := 0.0, -1, 0
	for i, p := range ps {
		v, ok := f(p)
		if !ok || v == 0 {
			continue
		}
		n++
		if bestI < 0 || (high && v > best) || (!high && v < best) {
			best, bestI = v, i
		}
	}
	if n < 2 {
		return -1
	}
	return bestI
}

func anyAssembly(ps []part.Part) bool {
	for _, p := range ps {
		if p.AsmMin > 0 || p.Loss > 0 {
			return true
		}
	}
	return false
}

func allSame(vals []string) bool {
	for _, v := range vals[1:] {
		if v != vals[0] {
			return false
		}
	}
	return true
}

func countText(n int) string {
	if n <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d", n)
}

// compareTitle names the panel, saying which columns are on screen when they
// don't all fit — the matrix rows themselves have no room to spare for it.
func (m Model) compareTitle() string {
	n := len(m.parts.pinned)
	if _, perPage, first := m.compareLayout(m.contentW()); perPage > 0 && perPage < n {
		return fmt.Sprintf("Compare %d parts · showing %d-%d", n, first+1, first+perPage)
	}
	return fmt.Sprintf("Compare %d parts", n)
}

// compareLayout decides how many columns fit and which slice of them is shown,
// so a narrow terminal pages instead of squashing.
func (m Model) compareLayout(w int) (colW, perPage, first int) {
	n := len(m.parts.pinned)
	if n == 0 {
		return 0, 0, 0
	}
	perPage = max(1, (w-cmpLabelW)/cmpMinColW)
	if perPage > n {
		perPage = n
	}
	colW = (w - cmpLabelW) / perPage
	first = colFirst(m.compare.first, clampInt(m.compare.sel, 0, n-1), perPage, n)
	return colW, perPage, first
}

// colFirst slides the visible window the least it can to keep the focused column
// on screen, and never leaves part of the page blank when there's more to show.
func colFirst(first, sel, perPage, n int) int {
	if perPage >= n {
		return 0
	}
	if sel < first {
		first = sel
	}
	if sel >= first+perPage {
		first = sel - perPage + 1
	}
	return clampInt(first, 0, n-perPage)
}

func (m Model) compareRowsVisible() int {
	n := m.contentH() - 4 // two header rows, a rule, and the legend
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) updateCompareKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := compareRows(m.parts.pinned)
	vis := m.compareRowsVisible()
	n := len(m.parts.pinned)

	switch msg.String() {
	case "esc":
		m.mode = modeParts
		return m, nil
	case "tab":
		return m.cycleTab(1)
	case "shift+tab":
		return m.cycleTab(-1)
	case "up", "k":
		m.compare.top = max(0, m.compare.top-1)
	case "down", "j":
		m.compare.top = clampInt(m.compare.top+1, 0, max(0, len(rows)-vis))
	case "pgup":
		m.compare.top = max(0, m.compare.top-vis)
	case "pgdown":
		m.compare.top = clampInt(m.compare.top+vis, 0, max(0, len(rows)-vis))
	case "g", "home":
		m.compare.top = 0
	case "G", "end":
		m.compare.top = max(0, len(rows)-vis)
	case "left", "h":
		m.compare.sel = max(0, m.compare.sel-1)
		_, _, m.compare.first = m.compareLayout(m.contentW())
	case "right", "l":
		m.compare.sel = min(n-1, m.compare.sel+1)
		_, _, m.compare.first = m.compareLayout(m.contentW())
	case "x":
		return m.unpinSelected()
	case "d":
		if p, ok := m.comparePart(); ok && p.Datasheet != "" {
			openExternal(p.Datasheet)
		}
	}
	return m, nil
}

func (m Model) comparePart() (part.Part, bool) {
	if m.compare.sel < 0 || m.compare.sel >= len(m.parts.pinned) {
		return part.Part{}, false
	}
	return m.parts.pinned[m.compare.sel], true
}

func (m Model) unpinSelected() (tea.Model, tea.Cmd) {
	p, ok := m.comparePart()
	if !ok {
		return m, nil
	}
	i := m.compare.sel
	m.parts.pinned = append(m.parts.pinned[:i:i], m.parts.pinned[i+1:]...)
	m.compare.sel = clampInt(i, 0, max(0, len(m.parts.pinned)-1))
	m.flash = "unpinned " + p.Code
	if len(m.parts.pinned) < 2 {
		m.mode = modeParts // nothing left to compare
	}
	return m, nil
}

func (m Model) mouseCompare(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	rows := compareRows(m.parts.pinned)
	vis := m.compareRowsVisible()
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			m.compare.top = max(0, m.compare.top-1)
		} else if ms.Button == tea.MouseWheelDown {
			m.compare.top = clampInt(m.compare.top+1, 0, max(0, len(rows)-vis))
		}
		return m, nil
	}
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	// clicking a column focuses it
	colW, perPage, first := m.compareLayout(m.contentW())
	if colW == 0 {
		return m, nil
	}
	if x := ms.X - 2 - cmpLabelW; x >= 0 {
		if col := first + x/colW; col < first+perPage && col < len(m.parts.pinned) {
			m.compare.sel = col
		}
	}
	return m, nil
}

func (m Model) viewCompare(w, h int) string {
	ps := m.parts.pinned
	if len(ps) < 2 {
		return dimStyle.Render("pin at least two parts in the Parts tab to compare them")
	}
	colW, perPage, first := m.compareLayout(w)
	last := min(len(ps), first+perPage)

	// two header rows: the code, then where it came from and how it's stocked
	codes, meta := spaces(cmpLabelW), spaces(cmpLabelW)
	for i := first; i < last; i++ {
		p := ps[i]
		code, sub := pad(p.Code, colW), pad(libText(p.Lib), colW)
		if i == m.compare.sel {
			codes += selRowStyle.Render(code)
			meta += selRowStyle.Render(sub)
			continue
		}
		codes += accentStyle.Render(code)
		meta += libCell(p.Lib, sub)
	}
	rows := compareRows(ps)
	vis := m.compareRowsVisible()
	top := clampInt(m.compare.top, 0, max(0, len(rows)-vis))

	lines := []string{codes, meta, borderStyle.Render(strings.Repeat("─", w))}
	for i := top; i < min(len(rows), top+vis); i++ {
		r := rows[i]
		if r.rule {
			lines = append(lines, borderStyle.Render(strings.Repeat("─", w)))
			continue
		}
		mark, label := " ", pad(trunc(r.label, cmpLabelW-2), cmpLabelW-2)
		if r.differ {
			mark, label = warnStyle.Render("!"), warnStyle.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		line := mark + label + " "
		for j := first; j < last; j++ {
			if j == r.best {
				// The tick goes right after the value, not at the cell's right
				// edge where it would read as belonging to the next column. And
				// it's a glyph, not just colour, so a monochrome terminal still
				// shows the winner.
				line += okStyle.Render(pad(trunc(r.vals[j], colW-3)+" ▴", colW))
				continue
			}
			cell := pad(trunc(r.vals[j], colW-1), colW)
			if r.differ {
				line += subtleStyle.Render(cell)
				continue
			}
			line += dimStyle.Render(cell)
		}
		lines = append(lines, padRender(line, w))
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	legend := warnStyle.Render("!") + dimStyle.Render(" differs   ") +
		okStyle.Render("▴") + dimStyle.Render(" best   ←→ column · x unpin · d datasheet · esc back")
	return strings.Join(lines, "\n") + "\n" + padRender(legend, w)
}
