package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The category picker is a panel over the Parts tab: the same query on top, and
// below it every category the current results fall into, boxed and grouped by
// the parent category the source reports. Pick one and the result table keeps
// only that category.
//
// It is built from the results rather than from a catalogue on purpose. Neither
// LCSC nor JLCPCB will search inside a category — both ignore the catalog id in
// a query, LCSC's published tree uses a different id space than its search
// index, and its facet endpoint now answers empty. What both sources *do* give
// is a category on every row, so the categories you see are the ones your query
// actually hits, with real counts.

const (
	// catMinBoxW is the narrowest a category box gets before the grid drops a
	// column instead of squeezing the names into nothing.
	catMinBoxW = 26
	// catBoxGap is the least space between boxes in a row; the grid widens it to
	// spread the columns evenly across the panel.
	catBoxGap = 1
)

// catState is the picker's own state. The query lives in partsState, since it's
// the same query the results came from.
type catState struct {
	open   bool
	cursor int // index into the flattened cell list
	top    int // first visible grid row
}

// catCell is one thing you can pick: a category box, or the group heading above
// a run of them. Headings are laid out but never focused.
type catCell struct {
	label   string
	count   int
	parent  string
	heading bool
	// all marks the box that clears the filter rather than setting one.
	all bool
}

// catGroups turns a result set into the picker's cells: an "everything" box
// first, then each parent category as a heading with its leaf categories under
// it, busiest first.
func catGroups(ps []catRow) []catCell {
	type key struct{ parent, leaf string }
	count := map[key]int{}
	parentTotal := map[string]int{}
	for _, p := range ps {
		leaf := strings.TrimSpace(p.Category)
		if leaf == "" {
			leaf = "uncategorised"
		}
		parent := strings.TrimSpace(p.ParentCat)
		if parent == "" {
			parent = "other"
		}
		count[key{parent, leaf}]++
		parentTotal[parent]++
	}
	if len(count) == 0 {
		return nil
	}

	parents := make([]string, 0, len(parentTotal))
	for p := range parentTotal {
		parents = append(parents, p)
	}
	sort.Slice(parents, func(i, j int) bool {
		if a, b := parentTotal[parents[i]], parentTotal[parents[j]]; a != b {
			return a > b
		}
		return parents[i] < parents[j]
	})

	cells := []catCell{{label: "all categories", count: len(ps), all: true}}
	for _, parent := range parents {
		var leaves []catCell
		for k, n := range count {
			if k.parent == parent {
				leaves = append(leaves, catCell{label: k.leaf, count: n, parent: parent})
			}
		}
		sort.Slice(leaves, func(i, j int) bool {
			if leaves[i].count != leaves[j].count {
				return leaves[i].count > leaves[j].count
			}
			return leaves[i].label < leaves[j].label
		})
		cells = append(cells, catCell{label: parent, count: parentTotal[parent], parent: parent, heading: true})
		cells = append(cells, leaves...)
	}
	return cells
}

// catRow is the little of a part the picker needs, so the grouping can be tested
// without building whole records.
type catRow struct{ Category, ParentCat string }

func (m Model) catRows() []catRow {
	// Group over the results before the category filter, so the boxes don't
	// vanish the moment you pick one.
	out := make([]catRow, 0, len(m.parts.results))
	for _, p := range m.parts.preCat() {
		out = append(out, catRow{Category: p.Category, ParentCat: p.ParentCat})
	}
	return out
}

// catCols is how many boxes fit across, how wide each one is, and the gap between
// them. The boxes stay a uniform width and the gap takes up the slack, so the row
// reaches the right edge and a click still maps to a column with plain division.
func catCols(w int) (cols, boxW, gap int) {
	cols = max(1, (w+catBoxGap)/(catMinBoxW+catBoxGap))
	boxW = (w - (cols-1)*catBoxGap) / cols
	gap = catBoxGap
	if cols > 1 {
		gap = (w - cols*boxW) / (cols - 1)
	}
	return cols, boxW, gap
}

// catLayout places the cells into grid rows. A heading takes a whole row, and a
// run of boxes wraps across the columns beneath it.
func catLayout(cells []catCell, cols int) [][]int {
	var rows [][]int
	var cur []int
	flush := func() {
		if len(cur) > 0 {
			rows = append(rows, cur)
			cur = nil
		}
	}
	for i, c := range cells {
		if c.heading {
			flush()
			rows = append(rows, []int{i})
			continue
		}
		cur = append(cur, i)
		if len(cur) == cols {
			flush()
		}
	}
	flush()
	return rows
}

// catPickable is every cell index the cursor may land on, in grid order.
func catPickable(cells []catCell) []int {
	var out []int
	for i, c := range cells {
		if !c.heading {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) openCategories() (tea.Model, tea.Cmd) {
	m.cat.open = true
	m.cat.cursor, m.cat.top = 0, 0
	m.parts.field.Focus() // the query is the point of the panel; type straight into it
	return m, nil
}

func (m Model) closeCategories() (tea.Model, tea.Cmd) {
	m.cat.open = false
	return m, nil
}

// applyCategory sets or clears the category filter and closes the panel, leaving
// the keyboard on the results — you picked a category to look at them.
func (m Model) applyCategory(c catCell) (tea.Model, tea.Cmd) {
	if c.all {
		m.parts.cat = ""
		m.flash = "showing every category"
	} else {
		m.parts.cat = c.label
		m.flash = fmt.Sprintf("%s · %d of %d results", c.label, c.count, len(m.parts.results))
	}
	m.parts.cursor, m.parts.top = 0, 0
	m.parts.field.Blur()
	m.cat.open = false
	return m, nil
}

// moveCat walks the grid. Left and right step through the boxes in order, up and
// down move a row at a time, keeping roughly the same column.
func (m Model) moveCat(dx, dy int) (tea.Model, tea.Cmd) {
	cells := catGroups(m.catRows())
	pick := catPickable(cells)
	if len(pick) == 0 {
		return m, nil
	}
	cols, _, _ := catCols(m.contentW())
	rows := catLayout(cells, cols)

	if dx != 0 {
		at := 0
		for i, idx := range pick {
			if idx == m.cat.cursor {
				at = i
			}
		}
		m.cat.cursor = pick[clampInt(at+dx, 0, len(pick)-1)]
		m.clampCat()
		return m, nil
	}

	// find the cursor's row and column, then step vertically to the nearest box
	r, col := 0, 0
	for ri, row := range rows {
		for ci, idx := range row {
			if idx == m.cat.cursor {
				r, col = ri, ci
			}
		}
	}
	for step := r + dy; step >= 0 && step < len(rows); step += dy {
		row := rows[step]
		if cells[row[0]].heading {
			continue // headings aren't landing spots, keep going
		}
		m.cat.cursor = row[min(col, len(row)-1)]
		break
	}
	m.clampCat()
	return m, nil
}

func (m *Model) clampCat() {
	cells := catGroups(m.catRows())
	cols, _, _ := catCols(m.contentW())
	rows := catLayout(cells, cols)
	vis := m.catVisibleRows()
	for ri, row := range rows {
		for _, idx := range row {
			if idx != m.cat.cursor {
				continue
			}
			if ri < m.cat.top {
				m.cat.top = ri
			}
			if ri >= m.cat.top+vis {
				m.cat.top = ri - vis + 1
			}
			return
		}
	}
}

// catVisibleRows is how many grid rows fit under the query and the status line.
func (m Model) catVisibleRows() int {
	// title, query, status, rule, footer, and 3 lines per box row
	n := (m.contentH() - 5) / 3
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) updateCatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cells := catGroups(m.catRows())
	key := msg.String()

	switch key {
	case "esc":
		if m.parts.field.Focused() {
			m.parts.field.Blur()
			return m, nil
		}
		return m.closeCategories()
	case "tab":
		// tab hands the grid the keyboard, then hands it on to the results
		if m.parts.field.Focused() {
			m.parts.field.Blur()
			return m, nil
		}
		return m.closeCategories()
	case "shift+tab":
		if !m.parts.field.Focused() {
			m.parts.field.Focus()
		}
		return m, nil
	case "enter":
		if m.cat.cursor >= 0 && m.cat.cursor < len(cells) {
			return m.applyCategory(cells[m.cat.cursor])
		}
		return m.closeCategories()
	case "left":
		return m.moveCat(-1, 0)
	case "right":
		return m.moveCat(1, 0)
	case "up":
		return m.moveCat(0, -1)
	case "down":
		return m.moveCat(0, 1)
	}

	if m.parts.field.Focused() {
		before := m.parts.field.Value()
		m.parts.field.Update(msg)
		if m.parts.field.Value() != before {
			m.cat.cursor, m.cat.top = 0, 0
			m.parts.debounce++
			return m, partsDebounceCmd(m.parts.debounce)
		}
		return m, nil
	}

	switch key {
	case "/", "i":
		m.parts.field.Focus()
	case "h":
		return m.moveCat(-1, 0)
	case "l":
		return m.moveCat(1, 0)
	case "k":
		return m.moveCat(0, -1)
	case "j":
		return m.moveCat(0, 1)
	}
	return m, nil
}

func (m Model) mouseCat(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			return m.moveCat(0, -1)
		}
		if ms.Button == tea.MouseWheelDown {
			return m.moveCat(0, 1)
		}
		return m, nil
	}
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	cells := catGroups(m.catRows())
	cols, boxW, gap := catCols(m.contentW())
	rows := catLayout(cells, cols)
	// tab(1)+border(1)+title,query,status,rule(4)
	const gridTop = 6
	r := m.cat.top + (ms.Y-gridTop)/3
	if r < 0 || r >= len(rows) {
		return m, nil
	}
	row := rows[r]
	if cells[row[0]].heading {
		return m, nil
	}
	col := (ms.X - 2) / (boxW + gap)
	if col < 0 || col >= len(row) {
		return m, nil
	}
	return m.applyCategory(cells[row[col]])
}

func (m Model) viewCategories(w, h int) string {
	cells := catGroups(m.catRows())
	cols, boxW, gap := catCols(w)
	rows := catLayout(cells, cols)

	title := focusMark(!m.parts.field.Focused()) +
		subtleStyle.Render("pick a category to narrow the results  ") +
		dimStyle.Render(fmt.Sprintf("%d in %s", len(m.parts.results), m.srcLabel()))
	query := focusMark(m.parts.field.Focused()) + m.parts.field.View()

	var status string
	switch {
	case m.parts.loading:
		status = m.spin.View() + " searching…"
	case len(m.parts.results) == 0:
		status = dimStyle.Render("type a search above — the categories come from what it finds")
	default:
		status = subtleStyle.Render(fmt.Sprintf("%d categories", len(catPickable(cells))-1))
		// The caveat matters, so it only goes when there is genuinely no room.
		for _, tail := range []string{
			"   these are the categories this search hits, not the whole catalogue",
			"   from this search, not the whole catalogue",
			"   from this search",
		} {
			if lipgloss.Width(status)+lipgloss.Width(tail) <= w {
				status += dimStyle.Render(tail)
				break
			}
		}
	}

	out := []string{title, query, status, borderStyle.Render(strings.Repeat("─", w))}

	vis := m.catVisibleRows()
	for ri := m.cat.top; ri < min(len(rows), m.cat.top+vis); ri++ {
		row := rows[ri]
		if cells[row[0]].heading {
			c := cells[row[0]]
			out = append(out, "", accentStyle.Render(strings.ToUpper(c.label))+
				dimStyle.Render(fmt.Sprintf("  %d", c.count)), "")
			continue
		}
		var boxes [][]string
		for _, idx := range row {
			boxes = append(boxes, catBox(cells[idx], boxW, idx == m.cat.cursor))
		}
		for line := 0; line < 3; line++ {
			var parts []string
			for _, b := range boxes {
				parts = append(parts, b[line])
			}
			out = append(out, strings.Join(parts, spaces(gap)))
		}
	}
	if len(rows) == 0 {
		out = append(out, "", dimStyle.Render("  nothing to group yet"))
	}

	for len(out) < h-1 {
		out = append(out, "")
	}
	out = out[:h-1]
	// Every line goes through padRender: a box row that overran would push the
	// panel border out and skew the whole grid.
	for i := range out {
		out[i] = padRender(out[i], w)
	}
	return strings.Join(out, "\n") + "\n" +
		padRender(dimStyle.Render("  ↑↓←→ pick · enter narrow to it · tab the results · esc back"), w)
}

// catBox draws one category as a three-line box: top rule, name, count.
func catBox(c catCell, w int, focused bool) []string {
	border, name := borderStyle, subtleStyle
	if focused {
		border, name = accentStyle, cursorStyle
	}
	if c.all {
		name = okStyle
		if focused {
			name = cursorStyle
		}
	}
	inner := max(w-2, 1)
	top := border.Render("╭" + strings.Repeat("─", inner) + "╮")
	bot := border.Render("╰" + strings.Repeat("─", inner) + "╯")

	// The count is the reason to look at a box, so it keeps its room and the name
	// is what gets cut: " name … count ".
	count := groupThousands(c.count)
	label := trunc(c.label, max(inner-lipgloss.Width(count)-3, 1))
	gap := max(inner-2-lipgloss.Width(label)-lipgloss.Width(count), 1)
	mid := border.Render("│") + " " + name.Render(label) + spaces(gap) +
		dimStyle.Render(count) + " " + border.Render("│")
	return []string{top, padRender(mid, w), bot}
}
