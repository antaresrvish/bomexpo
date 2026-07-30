package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bomexpo/internal/kicad"
)

// The Diff tab answers one question: does the schematic agree with a BOM some
// other tool produced? The schematic is found beside the open design, the BOM is
// typed in, and the report leads with what would spoil an order.

const diffDataTop = 8 // tab, border, title, path, summary, counts, colhead, rule

// diffCols are the column widths. The three side columns hold a value, a footprint
// and a part code each, so they start at a width that fits one and flex to fill
// whatever the terminal has spare. On a narrow terminal they stay at the minimum
// and the table scrolls sideways, the way Components does.
type diffCols struct{ ref, what, side int }

const (
	// sideMin is the narrowest a side column gets: enough for
	// "22uF · C_0603_1608Metric · C2762594" to read before the ellipsis.
	sideMin = 36
	// whatMax fits the verdicts that actually turn up — "schematic part code +
	// schematic footprint" is 41 columns. A row disagreeing three ways still
	// truncates, which is fine: the cells beside it say what happened.
	whatMax = 44
)

func layoutDiffCols(tableW int) diffCols {
	c := diffCols{ref: 9, what: 22, side: sideMin}
	spare := tableW - c.fullWidth()
	if spare <= 0 {
		return c
	}
	// The verdict gets room first, up to its longest wording, then the cells split
	// what's left three ways.
	if grow := min(spare/3, whatMax-c.what); grow > 0 {
		c.what += grow
		spare -= grow
	}
	c.side += spare / 3
	return c
}

// fullWidth is the row's natural width: the marker, every column, and a 3-column
// separator between the five cells.
func (c diffCols) fullWidth() int { return 2 + c.ref + c.what + 3*c.side + 4*3 }

// diffTableW is the width left for the table once the vertical scrollbar has its
// column.
func (m Model) diffTableW() int { return m.contentW() - 1 }

func (m Model) diffMaxHoff() int {
	w := m.diffTableW()
	if over := layoutDiffCols(w).fullWidth() - w; over > 0 {
		return over
	}
	return 0
}

type diffDoneMsg struct {
	res kicad.SchDiff
	err error
}

type diffState struct {
	field  textfield
	res    kicad.SchDiff
	ran    bool
	load   bool
	err    string
	cursor int
	top    int
	hoff   int // horizontal scroll, since the row is wider than most terminals
	// ref is the side the other two are measured against. The board is the default:
	// it is what gets manufactured and what bomexpo exports from.
	ref kicad.Side
	// show is how much of the comparison is on screen. It starts on everything:
	// a report that lists only problems looks identical to one that ran nothing.
	show diffShow
}

type diffShow int

const (
	// showAll lines every designator up, agreeing or not.
	showAll diffShow = iota
	showProblems
	showSerious
)

func (v diffShow) String() string {
	switch v {
	case showProblems:
		return "differences"
	case showSerious:
		return "serious only"
	}
	return "every part"
}

func newDiffState() diffState {
	return diffState{
		field: newField("› ", "path to the bom csv to compare against…", 56),
		ref:   kicad.SidePCB,
	}
}

// rows is the comparison as filtered.
func (s diffState) rows() []kicad.Row {
	if s.show == showAll {
		return s.res.Rows
	}
	var out []kicad.Row
	for _, r := range s.res.Rows {
		switch s.show {
		case showProblems:
			if !r.Agrees() {
				out = append(out, r)
			}
		case showSerious:
			if r.Severe() {
				out = append(out, r)
			}
		}
	}
	return out
}

func (m Model) diffRows() int {
	n := m.contentH() - 7
	if n < 1 {
		n = 1
	}
	return n
}

// schPath is the schematic to compare, taken from whatever design is open.
func (m Model) schPath() string {
	if m.pcbPath != "" {
		return m.pcbPath
	}
	if m.bomPath != "" {
		return filepath.Dir(m.bomPath)
	}
	return ""
}

func (m Model) diffCmd(bomPath string) tea.Cmd {
	sch, pcb, ref := m.schPath(), m.pcbPath, m.diff.ref
	return func() tea.Msg {
		if sch == "" {
			return diffDoneMsg{err: fmt.Errorf("open a design first — the schematic is found beside it")}
		}
		sc, err := kicad.LoadSchematic(sch)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		// Read the board from disk rather than using the open design: this compares
		// what the three files say, not what the session has in memory.
		var pcbItems []kicad.Item
		if pcb != "" {
			d, err := kicad.Load(pcb, "")
			if err != nil {
				return diffDoneMsg{err: err}
			}
			pcbItems = d.Items
		}
		bomItems, err := importOrderBOM(bomPath)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		res := kicad.Compare(sc, pcbItems, bomItems, ref)
		res.PCBPath, res.BOMPath = pcb, bomPath
		return diffDoneMsg{res: res}
	}
}

// openVerify enters the comparison from Export, which is where the question comes
// up. esc goes back there.
func (m Model) openVerify() (tea.Model, tea.Cmd) {
	m.mode = modeDiff
	if m.diff.field.Value() == "" {
		// Point it at the last order this project sent, since "what did I actually
		// send" is the question that brings anyone here.
		if p, _ := lastOrder(m.sourcePath()); p != "" {
			m.diff.field.SetValue(p)
		}
	}
	// Arriving here leaves the keys to the page, like everywhere else: a focused
	// field would swallow s, m and the digits. The footer says tab to type.
	m.diff.field.Blur()
	return m, nil
}

func (m Model) startDiff() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.diff.field.Value())
	if path == "" {
		m.diff.err = "type the path to a bom csv"
		return m, nil
	}
	m.diff.err = ""
	m.diff.load = true
	m.diff.cursor, m.diff.top = 0, 0
	return m, m.diffCmd(path)
}

func (m Model) updateDiffDone(msg diffDoneMsg) (tea.Model, tea.Cmd) {
	m.diff.load = false
	if msg.err != nil {
		m.diff.err = msg.err.Error()
		m.diff.ran = false
		return m, nil
	}
	m.diff.err = ""
	m.diff.ran = true
	m.diff.res = msg.res
	m.flash = msg.res.Summary()
	return m, nil
}

func (m Model) updateDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.diff.field.Focused() {
		switch key {
		case "esc":
			m.diff.field.Blur()
			return m, nil
		case "tab", "shift+tab":
			if next, ok := completeCSV(m.diff.field.Value()); ok {
				m.diff.field.SetValue(next)
				m.diff.ran = false
				return m, nil
			}
			// Several candidates with no shared prefix left to add is not "done" —
			// a shell would list them and keep the line. Only an empty directory
			// hands the page back.
			if _, names := diffCandidates(m.diff.field.Value()); len(names) > 0 {
				return m, nil
			}
			m.diff.field.Blur()
			return m, nil
		case "enter":
			m.diff.field.Blur()
			return m.startDiff()
		}
		before := m.diff.field.Value()
		m.diff.field.Update(msg)
		if m.diff.field.Value() != before {
			m.diff.ran = false // the report belongs to the old path
		}
		return m, nil
	}

	if mm, cmd, done := m.tabSwitchKey(key); done {
		return mm, cmd
	}
	switch key {
	case "esc":
		m.mode = modeCheck
		return m, nil
	case "tab", "shift+tab", "e", "/", "i":
		m.diff.field.Focus()
		return m, nil
	case "o":
		return m.useLastOrder()
	case "O":
		return m.usePrevOrder()
	case "enter", "r":
		return m.startDiff()
	case "s":
		m.diff.show = (m.diff.show + 1) % 3
		m.diff.cursor, m.diff.top = 0, 0
	case "m":
		// which file is right is the user's call, so the reference column moves
		m.diff.ref = (m.diff.ref + 1) % 3
		if m.diff.ran {
			return m.startDiff()
		}
		return m, nil
	case "left", "h":
		m.diff.hoff = clampInt(m.diff.hoff-8, 0, m.diffMaxHoff())
	case "right", "l":
		m.diff.hoff = clampInt(m.diff.hoff+8, 0, m.diffMaxHoff())
	case "up", "k":
		m.diff.cursor = max(0, m.diff.cursor-1)
	case "down", "j":
		m.diff.cursor = min(len(m.diff.rows())-1, m.diff.cursor+1)
	case "g", "home":
		m.diff.cursor = 0
	case "G", "end":
		m.diff.cursor = max(0, len(m.diff.rows())-1)
	case "pgup":
		m.diff.cursor = max(0, m.diff.cursor-m.diffRows())
	case "pgdown":
		m.diff.cursor = min(len(m.diff.rows())-1, m.diff.cursor+m.diffRows())
	}
	m.clampDiff()
	return m, nil
}

func (m *Model) clampDiff() {
	n := len(m.diff.rows())
	if n == 0 {
		m.diff.cursor, m.diff.top = 0, 0
		return
	}
	vis := m.diffRows()
	m.diff.cursor = clampInt(m.diff.cursor, 0, n-1)
	if m.diff.cursor < m.diff.top {
		m.diff.top = m.diff.cursor
	}
	if m.diff.cursor >= m.diff.top+vis {
		m.diff.top = m.diff.cursor - vis + 1
	}
	m.diff.top = clampInt(m.diff.top, 0, max(0, n-1))
}

// completeCSV completes the path, offering only directories and the csv files this
// field can actually use — a completion that lands on a .kicad_pcb would only have
// to be deleted again.
func completeCSV(input string) (string, bool) {
	return complete(input, func(e fsEntry) bool {
		return strings.EqualFold(filepath.Ext(e.name), ".csv")
	})
}

// diffCandidates is what a completion would choose between, for the hint line.
func diffCandidates(input string) (dir string, names []string) {
	d, _, all := listDir(input, 60)
	for _, e := range all {
		if e.isDir {
			names = append(names, e.name+"/")
			continue
		}
		if strings.EqualFold(filepath.Ext(e.name), ".csv") {
			names = append(names, e.name)
		}
	}
	return d, names
}

// useLastOrder fills the path in with the newest order this project sent.
func (m Model) useLastOrder() (tea.Model, tea.Cmd) {
	return m.pickOrder(0)
}

// usePrevOrder steps back through older orders, for the run before the bad one.
func (m Model) usePrevOrder() (tea.Model, tea.Cmd) {
	zips := orderZips(m.sourcePath())
	at := 0
	for i, z := range zips {
		if z.Path == strings.TrimSpace(m.diff.field.Value()) {
			at = i + 1
		}
	}
	return m.pickOrder(at)
}

func (m Model) pickOrder(n int) (tea.Model, tea.Cmd) {
	zips := orderZips(m.sourcePath())
	if len(zips) == 0 {
		m.diff.err = "no exported order beside this design to compare against"
		return m, nil
	}
	z := zips[clampInt(n, 0, len(zips)-1)]
	m.diff.field.SetValue(z.Path)
	m.diff.field.Blur()
	m.flash = "comparing against " + z.Name() + " · " + z.When.Format("2 Jan 15:04")
	return m.startDiff()
}

// diffKindStyle colours a finding by whether it would spoil an order.
func diffKindStyle(k kicad.DiffKind) lipgloss.Style {
	switch k {
	case kicad.DiffMissing, kicad.DiffExtra:
		return badStyle
	case kicad.DiffDNP, kicad.DiffValue, kicad.DiffCode:
		return warnStyle
	}
	return subtleStyle
}

// diffRowStyle colours a whole row: red for what breaks an order, amber for what
// merits a look, dim for the ones that agree.
func diffRowStyle(r kicad.Row) lipgloss.Style {
	switch {
	case r.Agrees():
		return dimStyle
	case r.Severe():
		return badStyle
	}
	return warnStyle
}

// diffGlyph is the row's verdict as one plain character.
func diffGlyph(r kicad.Row) string {
	ref := r.Cell(r.Ref)
	switch {
	case r.Severe():
		return "!"
	case !r.Agrees():
		return "~"
	case ref.DNP || ref.OffBOM:
		return "∅"
	}
	return "✓"
}

// diffMark is the gutter for an unselected row, coloured by verdict.
func diffMark(r kicad.Row) string {
	g := diffGlyph(r) + " "
	switch {
	case r.Severe():
		return badStyle.Render(g)
	case !r.Agrees():
		return warnStyle.Render(g)
	case g[0] == '\xe2': // the ∅ for a part left out on purpose
		return dimStyle.Render(g)
	}
	return okStyle.Render(g)
}

func (m Model) viewDiff(w, h int) string {
	s := m.diff

	title := focusMark(!s.field.Focused()) +
		subtleStyle.Render("does a bom match this design? point it at one, or press ") +
		accentStyle.Render("o") + subtleStyle.Render(" for the last order you sent")
	path := focusMark(s.field.Focused()) + s.field.View()

	sch := m.schPath()
	var where string
	switch {
	case sch == "":
		where = badStyle.Render("no design open — load a board or bom first")
	case s.ran:
		where = dimStyle.Render("sch ") + accentStyle.Render(filepath.Base(s.res.SchPath))
		if s.res.PCBPath != "" {
			where += dimStyle.Render("  ·  pcb ") + accentStyle.Render(filepath.Base(s.res.PCBPath))
		}
		where += dimStyle.Render("  ·  bom ") + accentStyle.Render(filepath.Base(s.res.BOMPath))
	default:
		where = dimStyle.Render("schematic will be found beside ") + subtleStyle.Render(filepath.Base(sch))
	}

	var summary string
	switch {
	case s.field.Focused() && !s.load:
		summary = diffCandidateLine(s.field.Value(), w)
	case s.load:
		summary = m.spin.View() + " reading the schematic…"
	case s.err != "":
		summary = badStyle.Render("✗ " + s.err)
	case !s.ran:
		summary = dimStyle.Render("enter a bom path and press enter")
	case len(s.res.Findings) == 0:
		summary = okStyle.Render("✓ " + s.res.Summary())
	default:
		summary = warnStyle.Render(s.res.Summary())
	}

	lines := []string{title, path, where, summary}
	if s.ran {
		lines = append(lines, m.diffCounts(w))
	} else {
		lines = append(lines, "")
	}

	// The header lines are prose, so a narrow terminal has to cut them rather than
	// letting one push the panel border out.
	for i := range lines {
		lines[i] = padRender(lines[i], w)
	}
	head, body := m.diffTable(h - len(lines) - 1)
	lines = append(lines, head...)
	lines = append(lines, body...)
	lines = lines[:h-1]
	return strings.Join(lines, "\n") + "\n" + m.diffFooter(w)
}

// diffTable draws the header and rows at their natural width and crops them to the
// viewport, with a vertical scrollbar down the side and a horizontal one along the
// bottom — the same treatment the Components table gets.
func (m Model) diffTable(h int) (head, body []string) {
	s := m.diff
	tableW := m.diffTableW()
	c := layoutDiffCols(tableW)
	full := c.fullWidth()
	hoff := clampInt(s.hoff, 0, max(0, full-tableW))
	crop := func(line string) string {
		line = ansi.Cut(line, hoff, hoff+tableW)
		if p := tableW - lipgloss.Width(line); p > 0 {
			line += spaces(p)
		}
		return line
	}
	sep := sepStyle.Render(" │ ")

	head = []string{
		crop(colHeadStyle.Render(padRender(spaces(2)+strings.Join([]string{
			pad("REF", c.ref), pad("WHAT", c.what),
			pad(diffHead("SCHEMATIC", kicad.SideSch, s.ref), c.side),
			pad(diffHead("PCB", kicad.SidePCB, s.ref), c.side),
			pad(diffHead("BOM", kicad.SideBOM, s.ref), c.side),
		}, " │ "), full))) + " ",
		crop(borderStyle.Render(strings.Repeat("─", full))) + " ",
	}

	rows := s.rows()
	// Every side writes the same fields in the same order, so a difference shows up
	// by sitting directly across from its counterpart.
	vis := h - 2 - 1 // the two header lines and the horizontal bar
	if vis < 1 {
		vis = 1
	}
	vbar := m.diffVScroll(len(rows), vis)
	for i := 0; i < vis; i++ {
		row := i + s.top
		if row >= len(rows) {
			body = append(body, spaces(tableW)+vbar[i])
			continue
		}
		body = append(body, crop(m.diffRowView(rows[row], c, sep, row == s.cursor))+vbar[i])
	}
	if s.ran && len(rows) == 0 && len(body) > 0 {
		body[0] = padRender(dimStyle.Render(fmt.Sprintf(
			"  nothing at this filter — %d designators hidden, press s for %s",
			len(s.res.Rows), (s.show+1)%3)), tableW) + vbar[0]
	}
	body = append(body, hScrollRow(tableW, full, tableW, hoff)+" ")
	return head, body
}

// diffRowView renders one row at its natural width.
func (m Model) diffRowView(r kicad.Row, c diffCols, sep string, cursor bool) string {
	plain := []string{
		pad(trunc(r.Designator, c.ref), c.ref),
		pad(trunc(r.What(), c.what), c.what),
		pad(trunc(r.Cell(kicad.SideSch).Text(), c.side), c.side),
		pad(trunc(r.Cell(kicad.SidePCB).Text(), c.side), c.side),
		pad(trunc(r.Cell(kicad.SideBOM).Text(), c.side), c.side),
	}
	if cursor {
		// The mark has to be unstyled here: an inner colour emits its own reset and
		// breaks the row highlight from that column on, which is why the Components
		// table draws a plain ▶ on its cursor row too.
		return selRowStyle.Render(padRender("▶"+diffGlyph(r)+strings.Join(plain, "   "), c.fullWidth()))
	}
	ref := accentStyle
	if r.Agrees() {
		ref = subtleStyle
	}
	return padRender(diffMark(r)+strings.Join([]string{
		ref.Render(plain[0]),
		diffRowStyle(r).Render(plain[1]),
		diffCell(r, kicad.SideSch, plain[2]),
		diffCell(r, kicad.SidePCB, plain[3]),
		diffCell(r, kicad.SideBOM, plain[4]),
	}, sep), c.fullWidth())
}

// diffVScroll is the thumb column beside the rows, one glyph per visible line.
func (m Model) diffVScroll(total, vis int) []string {
	out := make([]string, vis)
	if total <= vis {
		for i := range out {
			out[i] = " "
		}
		return out
	}
	thumb := max(1, vis*vis/total)
	pos := m.diff.top * (vis - thumb) / max(1, total-vis)
	for i := range out {
		if i >= pos && i < pos+thumb {
			out[i] = accentStyle.Render("█")
		} else {
			out[i] = borderStyle.Render("│")
		}
	}
	return out
}

// diffHead marks the reference column, so which side everything is measured
// against is on screen rather than assumed.
func diffHead(label string, side, ref kicad.Side) string {
	if side == ref {
		return label + " ◆"
	}
	return label
}

// diffCell colours one side's cell: the reference reads as the baseline, and only
// the sides that deviate from it light up, so the eye lands on the cell to read.
func diffCell(r kicad.Row, side kicad.Side, cell string) string {
	if r.Agrees() {
		return dimStyle.Render(cell)
	}
	if side == r.Ref {
		return okStyle.Render(cell)
	}
	if r.SideOK(side) {
		return dimStyle.Render(cell)
	}
	return diffRowStyle(r).Render(cell)
}

// diffCandidateLine shows what tab would complete to, so the key isn't a guess.
func diffCandidateLine(input string, w int) string {
	dir, names := diffCandidates(input)
	if len(names) == 0 {
		return dimStyle.Render("no csv in " + dir + " — tab completes folders too")
	}
	head := dimStyle.Render("tab completes  ")
	var shown []string
	for _, n := range names {
		if lipgloss.Width(head)+lipgloss.Width(strings.Join(shown, "  "))+lipgloss.Width(n)+12 > w {
			shown = append(shown, dimStyle.Render(fmt.Sprintf("+%d more", len(names)-len(shown))))
			break
		}
		st := okStyle
		if strings.HasSuffix(n, "/") {
			st = accentStyle
		}
		shown = append(shown, st.Render(n))
	}
	return head + strings.Join(shown, dimStyle.Render("  "))
}

// diffCounts is the tally by kind, plus the skipped DNP count so the totals add up
// on screen.
func (m Model) diffCounts(w int) string {
	d := m.diff.res
	var parts []string
	for _, side := range []kicad.Side{kicad.SidePCB, kicad.SideBOM} {
		byKind := map[kicad.DiffKind]int{}
		for _, f := range d.Findings {
			if f.Side == side {
				byKind[f.Kind]++
			}
		}
		var bits []string
		for _, k := range []kicad.DiffKind{
			kicad.DiffMissing, kicad.DiffExtra, kicad.DiffDNP,
			kicad.DiffValue, kicad.DiffFootprint, kicad.DiffCode, kicad.DiffExcluded,
		} {
			if n := byKind[k]; n > 0 {
				bits = append(bits, diffKindStyle(k).Render(fmt.Sprintf("%d %s", n, k)))
			}
		}
		if len(bits) > 0 {
			parts = append(parts, accentStyle.Render(side.String()+": ")+
				strings.Join(bits, dimStyle.Render(", ")))
		}
	}
	if d.SkippedDNP > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d dnp rightly absent", d.SkippedDNP)))
	}
	for _, sh := range d.Skipped() {
		parts = append(parts, badStyle.Render("unread sheet "+sh))
	}
	if nc := d.NotCompared(); len(nc) > 0 {
		parts = append(parts, warnStyle.Render("no "+strings.Join(nc, "/")+" column"))
	}
	if d.CodeRef() != kicad.SideSch {
		// The schematic carries no part codes, so say what the codes were judged
		// against rather than leaving the column looking authoritative.
		parts = append(parts, dimStyle.Render("codes vs the "+d.CodeRef().String()))
	}
	if len(parts) == 0 {
		return dimStyle.Render(fmt.Sprintf("%d symbols · %d on the pcb · %d bom designators",
			d.SchCount, d.PCBCount, d.BOMCount))
	}
	return padRender(strings.Join(parts, dimStyle.Render(" · ")), w)
}

func (m Model) diffFooter(w int) string {
	if m.diff.field.Focused() {
		return dimStyle.Render("  tab complete · enter compare · esc leaves the path")
	}
	if !m.diff.ran && m.diff.err == "" {
		if zips := orderZips(m.sourcePath()); len(zips) > 0 {
			return dimStyle.Render("  o ") + accentStyle.Render(zips[0].Name()) +
				dimStyle.Render("  ("+zips[0].When.Format("2 Jan 15:04")+")") +
				dimStyle.Render("   O older · tab type a path · esc back")
		}
	}
	left := dimStyle.Render("  o last order · s ") + accentStyle.Render(m.diff.show.String()) +
		dimStyle.Render(" · m against ") + accentStyle.Render(m.diff.ref.String())
	right := dimStyle.Render(fmt.Sprintf("%d of %d · ↑↓ rows · ←→ columns · esc back",
		len(m.diff.rows()), len(m.diff.res.Rows)))
	// The counts matter more than the key list when it's tight.
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > w {
		left = dimStyle.Render("  s ") + accentStyle.Render(m.diff.show.String())
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return padRender(left+spaces(gap)+right, w)
}
