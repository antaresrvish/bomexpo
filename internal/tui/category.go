package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/taxonomy"
)

// The category popup opens on the way into the Parts search: pick what kind of
// part, then search inside it. Its input narrows the category list and is not the
// parts query; t reopens it. The pick filters the results we fetch, because
// neither vendor will search by category.

const (
	catMinBoxW = 26 // below this the grid drops a column instead of squeezing
	catBoxGap  = 1  // widened by catCols to spread the boxes evenly
	catChromeH = 3 + 5
)

type catsLoadedMsg struct {
	source string
	cats   []taxonomy.Cat
	err    error
}

type catState struct {
	open    bool
	field   textfield
	cats    []taxonomy.Cat
	loading bool
	loaded  string // source the list belongs to, so a source switch re-harvests
	cursor  int    // index into the flattened cell list
	top     int    // first visible grid row
}

func newCatState() catState {
	return catState{field: newField("› ", "type to narrow the categories…", 40)}
}

// catCell is a category box or a group heading. Headings are laid out, never
// focused.
type catCell struct {
	label   string
	count   int // matching results, 0 before anything is searched
	parent  string
	heading bool
	all     bool // the box that clears the filter
}

// catGroups builds the cells: an everything box, then each parent as a heading
// with its leaves. Counts come from the current results, so what your search hit
// sorts first.
func catGroups(cats []taxonomy.Cat, counts map[string]int, query string) []catCell {
	q := strings.ToLower(strings.TrimSpace(query))
	byParent := map[string][]catCell{}
	parentTotal := map[string]int{}
	for _, c := range cats {
		if q != "" && !strings.Contains(strings.ToLower(c.Leaf), q) &&
			!strings.Contains(strings.ToLower(c.Parent), q) {
			continue
		}
		n := counts[strings.ToLower(c.Leaf)]
		byParent[c.Parent] = append(byParent[c.Parent], catCell{label: c.Leaf, count: n, parent: c.Parent})
		parentTotal[c.Parent] += n
	}
	if len(byParent) == 0 {
		return nil
	}

	parents := make([]string, 0, len(byParent))
	for p := range byParent {
		parents = append(parents, p)
	}
	sort.Slice(parents, func(i, j int) bool {
		if a, b := parentTotal[parents[i]], parentTotal[parents[j]]; a != b {
			return a > b
		}
		return parents[i] < parents[j]
	})

	total := 0
	for _, n := range counts {
		total += n
	}
	cells := []catCell{{label: "all categories", count: total, all: true}}
	for _, parent := range parents {
		leaves := byParent[parent]
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

// catCounts keys by lowercased leaf name.
func (m Model) catCounts() map[string]int {
	out := map[string]int{}
	for _, p := range m.parts.preCat() {
		if leaf := strings.TrimSpace(p.Category); leaf != "" {
			out[strings.ToLower(leaf)]++
		}
	}
	return out
}

func (m Model) catCells() []catCell {
	return catGroups(m.cat.cats, m.catCounts(), m.cat.field.Value())
}

// catCols keeps a uniform box width and lets the gap take up the slack, so the row
// fills the width and a click still maps to a column by division.
func catCols(w int) (cols, boxW, gap int) {
	cols = max(1, (w+catBoxGap)/(catMinBoxW+catBoxGap))
	boxW = (w - (cols-1)*catBoxGap) / cols
	gap = catBoxGap
	if cols > 1 {
		gap = (w - cols*boxW) / (cols - 1)
	}
	return cols, boxW, gap
}

// catLayout gives a heading its own row and wraps the boxes beneath it.
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

func catPickable(cells []catCell) []int {
	var out []int
	for i, c := range cells {
		if !c.heading {
			out = append(out, i)
		}
	}
	return out
}

// catFirstMatch skips the clear-the-filter box: typing "usb" then enter has to
// give you USB, not everything.
func catFirstMatch(cells []catCell) int {
	for i, c := range cells {
		if !c.heading && !c.all {
			return i
		}
	}
	if pick := catPickable(cells); len(pick) > 0 {
		return pick[0]
	}
	return 0
}

// catGridW depends only on the terminal width, so catPopupH can build on it
// without the two chasing each other.
func (m Model) catGridW() int { return popupW(m.contentW()) - 5 }

func (m Model) catGeom() (x, y, pw, ph int) {
	cols, _, _ := catCols(m.catGridW())
	rows := catLayout(m.catCells(), cols)
	return popupBox(m.contentW(), m.contentH(), catChromeH+3*max(len(rows), 1))
}

func (m Model) catVisibleRows() int {
	_, _, _, ph := m.catGeom()
	return max((ph-catChromeH)/3, 1)
}

func (m Model) catsCmd() tea.Cmd {
	src := m.src()
	if src == nil {
		return nil
	}
	id := src.ID()
	return func() tea.Msg {
		cats, err := taxonomy.Load(src)
		return catsLoadedMsg{source: id, cats: cats, err: err}
	}
}

func (m Model) updateCatsLoaded(msg catsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.source != m.srcID() {
		return m, nil // a source switch beat the harvest home
	}
	m.cat.loading = false
	m.cat.loaded = msg.source
	if msg.err != nil && len(msg.cats) == 0 {
		m.flash = "could not load the category list: " + msg.err.Error()
		return m, nil
	}
	m.cat.cats = taxonomy.Merge(msg.cats, taxonomy.FromParts(m.parts.results))
	return m, nil
}

// openCategories focuses the popup's own filter and blurs the parts query: two
// inputs taking the same keystroke is what made this confusing before.
func (m Model) openCategories() (tea.Model, tea.Cmd) {
	m.cat.open = true
	m.cat.cursor, m.cat.top = 0, 0
	m.cat.field.SetValue("")
	m.cat.field.Focus()
	m.parts.field.Blur()

	m.cat.cats = taxonomy.Merge(m.cat.cats, taxonomy.FromParts(m.parts.results))
	if m.cat.loaded == m.srcID() || m.cat.loading {
		return m, nil
	}
	m.cat.loading = true
	return m, m.catsCmd()
}

// closeCategories hands the keyboard to the parts query either way.
func (m Model) closeCategories() (tea.Model, tea.Cmd) {
	m.cat.open = false
	m.cat.field.Blur()
	m.parts.field.Focus()
	return m, nil
}

func (m Model) applyCategory(c catCell) (tea.Model, tea.Cmd) {
	if c.all {
		m.parts.cat = ""
		m.flash = "searching every category"
	} else {
		m.parts.cat = c.label
		if c.count > 0 {
			m.flash = fmt.Sprintf("%s · %d of the results so far", c.label, c.count)
		} else {
			m.flash = c.label + " · now search inside it"
		}
	}
	m.parts.cursor, m.parts.top = 0, 0
	mm, _ := m.closeCategories()
	m = mm.(Model)
	// Picking is itself a request to see what's in it. Clearing with nothing typed
	// has nothing to search, so what's on screen stays.
	if strings.TrimSpace(m.parts.field.Value()) == "" && m.parts.cat == "" {
		m.parts.clamp(m.partsRows())
		return m, nil
	}
	return m.researchParts()
}

// moveCat steps the boxes in order horizontally and a row at a time vertically,
// holding roughly the same column.
func (m Model) moveCat(dx, dy int) (tea.Model, tea.Cmd) {
	cells := m.catCells()
	pick := catPickable(cells)
	if len(pick) == 0 {
		return m, nil
	}
	cols, _, _ := catCols(m.catGridW())
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
	cells := m.catCells()
	cols, _, _ := catCols(m.catGridW())
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

func (m Model) updateCatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cells := m.catCells()
	key := msg.String()

	switch key {
	case "esc":
		return m.closeCategories()
	case "enter":
		if m.cat.cursor >= 0 && m.cat.cursor < len(cells) {
			return m.applyCategory(cells[m.cat.cursor])
		}
		return m.closeCategories()
	case "tab", "shift+tab":
		if m.cat.field.Focused() {
			m.cat.field.Blur()
		} else {
			m.cat.field.Focus()
		}
		return m, nil
	case "left", "ctrl+p":
		return m.moveCat(-1, 0)
	case "right", "ctrl+n":
		return m.moveCat(1, 0)
	case "up":
		return m.moveCat(0, -1)
	case "down":
		return m.moveCat(0, 1)
	}

	if m.cat.field.Focused() {
		before := m.cat.field.Value()
		m.cat.field.Update(msg)
		if m.cat.field.Value() != before {
			m.cat.top = 0
			m.cat.cursor = catFirstMatch(m.catCells())
		}
		return m, nil
	}

	switch key {
	case "/", "i":
		m.cat.field.Focus()
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
	cells := m.catCells()
	cols, boxW, gap := catCols(m.catGridW())
	rows := catLayout(cells, cols)

	// screen to grid: tab bar and panel border, the popup, its border and padding,
	// then the four rows above the boxes
	px, py, _, _ := m.catGeom()
	gridX := 2 + px + 2
	gridY := 2 + py + 1 + 4
	r := m.cat.top + (ms.Y-gridY)/3
	if ms.Y < gridY || r < 0 || r >= len(rows) {
		return m, nil
	}
	row := rows[r]
	if cells[row[0]].heading {
		return m, nil
	}
	col := (ms.X - gridX) / (boxW + gap)
	if ms.X < gridX || col < 0 || col >= len(row) {
		return m, nil
	}
	return m.applyCategory(cells[row[col]])
}

// viewCategories floats the popup over the Parts tab, so the results you're
// narrowing stay visible around it.
func (m Model) viewCategories(w, h int) string {
	bg := strings.Split(m.viewParts(w, h), "\n")
	x, y, pw, ph := m.catGeom()
	box := popupFrame(m.catTitle(), m.catContent(pw-5, ph-3), pw, ph)
	return strings.Join(overlay(bg, box, x, y, w), "\n")
}

func (m Model) catTitle() string {
	if n := len(m.parts.results); n > 0 {
		return fmt.Sprintf("What kind of part? · %d results to narrow", n)
	}
	return "What kind of part? · " + m.srcLabel()
}

func (m Model) catContent(w, h int) []string {
	cells := m.catCells()
	cols, boxW, gap := catCols(w)
	rows := catLayout(cells, cols)

	title := focusMark(!m.cat.field.Focused()) +
		subtleStyle.Render("pick one, then search inside it")
	filter := focusMark(m.cat.field.Focused()) + m.cat.field.View()

	var status string
	switch {
	case m.cat.loading && len(cells) == 0:
		status = m.spin.View() + " reading the category list from " + m.srcLabel() + "…"
	case len(cells) == 0 && m.cat.field.Value() != "":
		status = dimStyle.Render("nothing here matches — esc, then search for it and it gets added")
	case len(cells) == 0:
		status = dimStyle.Render("no category list yet — search first and it fills in")
	default:
		status = subtleStyle.Render(plural(len(catPickable(cells))-1, "category", "categories"))
		switch {
		case m.cat.loading:
			status += dimStyle.Render("   " + m.spin.View() + " loading more")
		case m.cat.field.Value() == "":
			// the list is crawled, so a gap must not look like an absence
			status += dimStyle.Render("   not in here? search for it and it gets added")
		}
	}

	out := []string{title, filter, status, borderStyle.Render(strings.Repeat("─", w))}

	vis := m.catVisibleRows()
	for ri := m.cat.top; ri < min(len(rows), m.cat.top+vis); ri++ {
		row := rows[ri]
		if cells[row[0]].heading {
			c := cells[row[0]]
			head := accentStyle.Render(strings.ToUpper(c.label))
			if c.count > 0 {
				head += dimStyle.Render(fmt.Sprintf("  %d", c.count))
			}
			out = append(out, "", head, "")
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

	for len(out) < h-1 {
		out = append(out, "")
	}
	out = out[:h-1]
	out = append(out, dimStyle.Render(catFooter(len(rows), m.cat.top, vis, w)))
	// a box row that overran would push the popup border out
	for i := range out {
		out[i] = padRender(out[i], w)
	}
	return out
}

// catFooter names the keys and counts the rows scrolled off, which otherwise read
// as "that's all of them".
func catFooter(rows, top, vis, w int) string {
	var off []string
	if top > 0 {
		off = append(off, fmt.Sprintf("%d↑", top))
	}
	if n := rows - top - vis; n > 0 {
		off = append(off, fmt.Sprintf("%d↓", n))
	}
	tail := ""
	if len(off) > 0 {
		tail = "   " + strings.Join(off, " ") + " more"
	}
	// the offscreen count outlives the key list when it's tight
	for _, keys := range []string{
		"↑↓←→ pick · enter search inside it · type to narrow · esc back",
		"↑↓←→ pick · enter pick it · type to narrow · esc back",
		"↑↓←→ pick · enter pick it · esc back",
		"↑↓←→ · enter · esc",
	} {
		if lipgloss.Width(keys)+lipgloss.Width(tail) <= w {
			return keys + tail
		}
	}
	return strings.TrimSpace(tail)
}

// catBox draws a category as a three-line box. The count only appears once there
// are results to count.
func catBox(c catCell, w int, focused bool) []string {
	border, name := borderStyle, subtleStyle
	if focused {
		border, name = accentStyle, cursorStyle
	}
	if c.all && !focused {
		name = okStyle
	}
	inner := max(w-2, 1)
	top := border.Render("╭" + strings.Repeat("─", inner) + "╮")
	bot := border.Render("╰" + strings.Repeat("─", inner) + "╯")

	count := ""
	if c.count > 0 {
		count = groupThousands(c.count)
	}
	label := trunc(c.label, max(inner-lipgloss.Width(count)-3, 1))
	gap := max(inner-2-lipgloss.Width(label)-lipgloss.Width(count), 1)
	mid := border.Render("│") + " " + name.Render(label) + spaces(gap) +
		dimStyle.Render(count) + " " + border.Render("│")
	return []string{top, padRender(mid, w), bot}
}
