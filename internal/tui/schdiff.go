package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
)

// The Diff tab answers one question: does the schematic agree with a BOM some
// other tool produced? The schematic is found beside the open design, the BOM is
// typed in, and the report leads with what would spoil an order.

const diffDataTop = 8 // tab, border, title, path, summary, counts, colhead, rule

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
	return diffState{field: newField("› ", "path to the bom csv to compare against…", 56)}
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
	sch, pcb := m.schPath(), m.pcbPath
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
		bomItems, err := kicad.ImportBOM(bomPath)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		res := kicad.Compare(sc, pcbItems, bomItems)
		res.PCBPath, res.BOMPath = pcb, bomPath
		return diffDoneMsg{res: res}
	}
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
		m.mode = modeTable
		return m, nil
	case "tab", "shift+tab", "e", "/", "i":
		m.diff.field.Focus()
		return m, nil
	case "enter", "r":
		return m.startDiff()
	case "s":
		m.diff.show = (m.diff.show + 1) % 3
		m.diff.cursor, m.diff.top = 0, 0
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

// diffKindStyle colours a finding by whether it would spoil an order.
func diffKindStyle(k kicad.DiffKind) lipgloss.Style {
	switch k {
	case kicad.DiffMissing, kicad.DiffExtra:
		return badStyle
	case kicad.DiffDNP, kicad.DiffValue:
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

// diffMark is the gutter: the row's verdict at a glance.
func diffMark(r kicad.Row) string {
	switch {
	case r.Severe():
		return badStyle.Render("! ")
	case !r.Agrees():
		return warnStyle.Render("~ ")
	case r.Sch.DNP || r.Sch.OffBOM:
		return dimStyle.Render("∅ ")
	}
	return okStyle.Render("✓ ")
}

func (m Model) viewDiff(w, h int) string {
	s := m.diff

	title := focusMark(!s.field.Focused()) + subtleStyle.Render("compare the schematic against a bom from somewhere else")
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

	// mark(2) + ref + what + three side columns + four 3-wide separators
	const refW, kindW = 8, 24
	sideW := (w - 2 - refW - kindW - 4*3) / 3
	if sideW < 10 {
		sideW = 10
	}
	lines = append(lines,
		colHeadStyle.Render(padRender(strings.Join([]string{
			pad("REF", refW), pad("WHAT", kindW),
			pad("SCHEMATIC", sideW), pad("PCB", sideW), pad("BOM", sideW),
		}, " | "), w)),
		borderStyle.Render(strings.Repeat("─", w)))

	rows := s.rows()
	vis := m.diffRows()
	for i := s.top; i < min(len(rows), s.top+vis); i++ {
		r := rows[i]
		// The two sides carry the same fields in the same order, so a difference
		// jumps out by sitting directly across from its counterpart.
		plain := []string{
			pad(trunc(r.Ref, refW), refW),
			pad(trunc(r.What(), kindW), kindW),
			pad(trunc(r.Cell(kicad.SideSch).Text(), sideW), sideW),
			pad(trunc(r.Cell(kicad.SidePCB).Text(), sideW), sideW),
			pad(trunc(r.Cell(kicad.SideBOM).Text(), sideW), sideW),
		}
		if i == s.cursor {
			lines = append(lines, selRowStyle.Render(padRender(diffMark(r)+strings.Join(plain, "   "), w)))
			continue
		}
		ref := accentStyle
		if r.Agrees() {
			ref = subtleStyle
		}
		lines = append(lines, padRender(diffMark(r)+strings.Join([]string{
			ref.Render(plain[0]),
			diffRowStyle(r).Render(plain[1]),
			diffCell(r, kicad.SideSch, plain[2]),
			diffCell(r, kicad.SidePCB, plain[3]),
			diffCell(r, kicad.SideBOM, plain[4]),
		}, sepStyle.Render(" | ")), w))
	}
	if s.ran && len(rows) == 0 {
		hidden := len(s.res.Rows)
		lines = append(lines, dimStyle.Render(fmt.Sprintf(
			"  nothing at this filter — %d designators hidden, press s for %s",
			hidden, (s.show+1)%3)))
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = lines[:h-1]
	return strings.Join(lines, "\n") + "\n" + m.diffFooter(w)
}

// diffCell colours one side's cell: the schematic reads as the reference, and only
// the sides that deviate from it light up, so the eye lands on the cell to read.
func diffCell(r kicad.Row, side kicad.Side, cell string) string {
	if r.Agrees() {
		return dimStyle.Render(cell)
	}
	if side == kicad.SideSch {
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
			kicad.DiffValue, kicad.DiffFootprint, kicad.DiffExcluded,
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
		parts = append(parts, warnStyle.Render("no "+strings.Join(nc, "/")+" column in that bom"))
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
	left := dimStyle.Render("  tab path · enter compare · s showing ") +
		accentStyle.Render(m.diff.show.String())
	right := dimStyle.Render(fmt.Sprintf("%d of %d · ↑↓ move · esc back",
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
