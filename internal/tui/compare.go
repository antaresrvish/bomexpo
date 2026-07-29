package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bomexpo/internal/part"
	"bomexpo/internal/render"
)

const (
	// cmpMinColW is the narrowest a part card can get before paging beats
	// squashing: a label, a value and the box borders.
	cmpMinColW = 24
	// minCardH is the shortest a card can be and still hold a footprint drawing
	// plus a field or two.
	minCardH = 7
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

// compareTitle names the panel, saying which cards are on screen when they don't
// all fit.
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
	perPage = max(1, w/cmpMinColW)
	if perPage > n {
		perPage = n
	}
	colW = w / perPage
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

// compareFieldRows is how many differing fields a card shows at once, which is
// what up and down scroll through.
func (m Model) compareFieldRows() int {
	botH := m.contentH() - m.contentH()*4/10 - 1
	return max((botH-2)/2, 1)
}

// compareDiffCount is how many fields the pinned parts actually differ on.
func (m Model) compareDiffCount() int {
	n := 0
	for _, r := range compareRows(m.parts.pinned) {
		if !r.rule && r.differ {
			n++
		}
	}
	return n
}

func (m Model) updateCompareKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	total := m.compareDiffCount()
	vis := m.compareFieldRows()
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
		m.compare.top = clampInt(m.compare.top+1, 0, max(0, total-vis))
	case "pgup":
		m.compare.top = max(0, m.compare.top-vis)
	case "pgdown":
		m.compare.top = clampInt(m.compare.top+vis, 0, max(0, total-vis))
	case "g", "home":
		m.compare.top = 0
	case "G", "end":
		m.compare.top = max(0, total-vis)
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
	if wheel {
		vis, total := m.compareFieldRows(), m.compareDiffCount()
		if ms.Button == tea.MouseWheelUp {
			m.compare.top = max(0, m.compare.top-1)
		} else if ms.Button == tea.MouseWheelDown {
			m.compare.top = clampInt(m.compare.top+1, 0, max(0, total-vis))
		}
		return m, nil
	}
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	// clicking a card focuses it; the cards span the whole content width
	colW, perPage, first := m.compareLayout(m.contentW())
	if colW == 0 {
		return m, nil
	}
	if x := ms.X - 2; x >= 0 {
		if col := first + x/colW; col < first+perPage && col < len(m.parts.pinned) {
			m.compare.sel = col
		}
	}
	return m, nil
}

// viewCompare splits the page: what the parts agree on up top, then one column
// per part below, each with its footprint and the fields that set it apart.
func (m Model) viewCompare(w, h int) string {
	if len(m.parts.pinned) < 2 {
		return dimStyle.Render("pin at least two parts in the Parts tab to compare them")
	}

	// 40% for the common ground, 60% for the per-part columns — the columns need
	// the room, since each carries a drawing.
	topH := h * 4 / 10
	botH := h - topH
	if botH < minCardH+1 {
		botH = min(h-2, minCardH+1)
		topH = h - botH
	}

	out := m.compareShared(w, topH)
	for len(out) < topH {
		out = append(out, "")
	}
	out = append(out, m.compareColumns(w, botH-1)...)
	for len(out) < h-1 {
		out = append(out, "")
	}
	out = append(out[:h-1], padRender(m.compareLegend(), w))
	return strings.Join(out, "\n")
}

func (m Model) compareLegend() string {
	return okStyle.Render("▴") + dimStyle.Render(" best of these   ") +
		dimStyle.Render("←→ column · ↑↓ more fields · x unpin · d datasheet · esc back")
}

// compareShared is the top pane: the fields every pinned part answers the same
// way. Clearing them out of the way is what makes the differences below read.
func (m Model) compareShared(w, h int) []string {
	var same []cmpRow
	for _, r := range compareRows(m.parts.pinned) {
		if !r.rule && !r.differ {
			same = append(same, r)
		}
	}

	out := []string{accentStyle.Render("In common") +
		dimStyle.Render(fmt.Sprintf("  all %d parts", len(m.parts.pinned)))}
	if len(same) == 0 {
		return append(out, dimStyle.Render("  nothing — these parts differ on every field"))
	}

	// Each entry is just a label and one value, so lay them out across the width
	// rather than as a thin list with the pane half empty.
	cols := min(3, len(same))
	if w/max(cols, 1) < 24 {
		cols = max(1, w/24)
	}
	perCol := (len(same) + cols - 1) / cols
	if maxRows := max(h-1, 1); perCol > maxRows {
		perCol = maxRows
	}
	colW := w / cols

	grid := make([]string, perCol)
	for i, r := range same {
		row := i / cols // row-major, so it reads left to right
		if row >= perCol {
			break
		}
		grid[row] += padRender(dimStyle.Render(pad(trunc(r.label, 13), 14))+
			subtleStyle.Render(trunc(r.vals[0], max(colW-15, 4))), colW)
	}
	for _, g := range grid {
		if strings.TrimSpace(ansi.Strip(g)) != "" {
			out = append(out, g)
		}
	}
	if shown := min(len(same), perCol*cols); shown < len(same) {
		out = append(out, dimStyle.Render(fmt.Sprintf("  +%d more in common", len(same)-shown)))
	}
	return out
}

// compareColumns is the bottom pane: one boxed card per part.
func (m Model) compareColumns(w, h int) []string {
	ps := m.parts.pinned
	colW, perPage, first := m.compareLayout(w)
	if colW == 0 || h < minCardH {
		return nil
	}
	last := min(len(ps), first+perPage)

	var diff []cmpRow
	for _, r := range compareRows(ps) {
		if !r.rule && r.differ {
			diff = append(diff, r)
		}
	}

	// the drawing takes what the fields leave; the fields scroll if there are
	// more than fit
	factH := max((h-2)/2, 1)
	drawH := h - 2 - factH
	if drawH < 2 {
		drawH, factH = 2, max(h-4, 1)
	}
	top := clampInt(m.compare.top, 0, max(0, len(diff)-factH))

	cards := make([][]string, 0, last-first)
	for i := first; i < last; i++ {
		cards = append(cards, m.compareCard(i, diff, top, colW, drawH, factH))
	}

	out := make([]string, h)
	for y := 0; y < h; y++ {
		line := ""
		for _, c := range cards {
			cell := ""
			if y < len(c) {
				cell = c[y]
			}
			line += padRender(cell, colW)
		}
		out[y] = padRender(line, w)
	}
	return out
}

// compareCard is one part's column: its code in the frame, its footprint, then
// the fields that differ — bright where this part is the best of the bunch.
func (m Model) compareCard(idx int, diff []cmpRow, top, w, drawH, factH int) []string {
	p := m.parts.pinned[idx]
	border, name := borderStyle, accentStyle
	if idx == m.compare.sel {
		border, name = accentStyle, cursorStyle
	}

	title := p.Code
	if title == "" {
		title = "—"
	}
	fill := max(w-4-lipgloss.Width(title), 0)
	out := []string{border.Render("╭ ") + name.Render(title) +
		border.Render(" "+strings.Repeat("─", fill)+"╮")}

	inner := w - 2
	body := m.compareFootprint(p, inner, drawH)
	for len(body) < drawH {
		body = append(body, "")
	}

	for i := top; i < len(diff) && len(body) < drawH+factH; i++ {
		r := diff[i]
		val, style, mark := r.vals[idx], subtleStyle, " "
		if idx == r.best {
			style, mark = okStyle, okStyle.Render("▴")
		}
		body = append(body, dimStyle.Render(pad(trunc(r.label, 10), 11))+
			style.Render(trunc(val, max(inner-13, 3)))+" "+mark)
	}
	for len(body) < drawH+factH {
		body = append(body, "")
	}

	for _, ln := range body {
		out = append(out, border.Render("│")+padRender(ln, inner)+border.Render("│"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

// compareFootprint draws the part's land pattern: from the board when the part
// is on it, otherwise from the copy downloaded for the comparison.
func (m Model) compareFootprint(p part.Part, w, h int) []string {
	if lands := m.boardLandsFor(p.Code); len(lands) > 0 {
		rot := 0.0
		if i := m.itemWithCode(p.Code); i >= 0 {
			rot = m.rotOf(i)
		}
		if img := render.Footprint(lands, render.FootprintOptions{W: w, H: h, Rotate: rot}); img != "" {
			return strings.Split(img, "\n")
		}
	}
	if fp, ok := m.edaLands[p.Code]; ok && len(fp.Lands) > 0 {
		if img := render.Footprint(fp.Lands, render.FootprintOptions{W: w, H: h}); img != "" {
			return strings.Split(img, "\n")
		}
	}

	pkg := p.Package
	if fp, ok := m.edaLands[p.Code]; ok && fp.Package != "" {
		pkg = fp.Package
	}
	if pkg == "" {
		pkg = "unknown package"
	}
	return []string{"", dimStyle.Render("  " + pkg), dimStyle.Render("  fetching footprint…")}
}
