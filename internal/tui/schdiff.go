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
	// severeOnly hides everything that wouldn't spoil an order.
	severeOnly bool
}

func newDiffState() diffState {
	return diffState{field: newField("› ", "path to the bom csv to compare against…", 56)}
}

// findings is the report as filtered.
func (s diffState) findings() []kicad.Finding {
	if !s.severeOnly {
		return s.res.Findings
	}
	var out []kicad.Finding
	for _, f := range s.res.Findings {
		if f.Kind.Severe() {
			out = append(out, f)
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
	sch := m.schPath()
	return func() tea.Msg {
		if sch == "" {
			return diffDoneMsg{err: fmt.Errorf("open a design first — the schematic is found beside it")}
		}
		sc, err := kicad.LoadSchematic(sch)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		items, err := kicad.ImportBOM(bomPath)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		res := kicad.DiffSchematicBOM(sc, items)
		res.BOMPath = bomPath
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
		m.diff.severeOnly = !m.diff.severeOnly
		m.diff.cursor, m.diff.top = 0, 0
	case "up", "k":
		m.diff.cursor = max(0, m.diff.cursor-1)
	case "down", "j":
		m.diff.cursor = min(len(m.diff.findings())-1, m.diff.cursor+1)
	case "g", "home":
		m.diff.cursor = 0
	case "G", "end":
		m.diff.cursor = max(0, len(m.diff.findings())-1)
	case "pgup":
		m.diff.cursor = max(0, m.diff.cursor-m.diffRows())
	case "pgdown":
		m.diff.cursor = min(len(m.diff.findings())-1, m.diff.cursor+m.diffRows())
	}
	m.clampDiff()
	return m, nil
}

func (m *Model) clampDiff() {
	n := len(m.diff.findings())
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
		where = dimStyle.Render("schematic ") + accentStyle.Render(filepath.Base(s.res.SchPath)) +
			dimStyle.Render("  vs  ") + accentStyle.Render(filepath.Base(s.res.BOMPath))
	default:
		where = dimStyle.Render("schematic will be found beside ") + subtleStyle.Render(filepath.Base(sch))
	}

	var summary string
	switch {
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

	// mark(2) + ref + kind + two side columns + three 3-wide separators
	const refW, kindW = 9, 21
	sideW := (w - 2 - refW - kindW - 3*3) / 2
	if sideW < 12 {
		sideW = 12
	}
	lines = append(lines,
		colHeadStyle.Render(padRender(strings.Join([]string{
			pad("REF", refW), pad("WHAT", kindW), pad("SCHEMATIC", sideW), pad("BOM", sideW),
		}, " | "), w)),
		borderStyle.Render(strings.Repeat("─", w)))

	f := s.findings()
	vis := m.diffRows()
	for i := s.top; i < min(len(f), s.top+vis); i++ {
		fd := f[i]
		mark := "  "
		if fd.Kind.Severe() {
			mark = "! "
		}
		plain := []string{
			pad(trunc(fd.Ref, refW), refW),
			pad(trunc(fd.Kind.String(), kindW), kindW),
			pad(trunc(fd.Sch, sideW), sideW),
			pad(trunc(fd.BOM, sideW), sideW),
		}
		if i == s.cursor {
			lines = append(lines, selRowStyle.Render(padRender(mark+strings.Join(plain, "   "), w)))
			continue
		}
		lines = append(lines, padRender(diffKindStyle(fd.Kind).Render(mark)+strings.Join([]string{
			accentStyle.Render(plain[0]),
			diffKindStyle(fd.Kind).Render(plain[1]),
			subtleStyle.Render(plain[2]),
			dimStyle.Render(plain[3]),
		}, sepStyle.Render(" | ")), w))
	}
	if s.ran && len(f) == 0 {
		if s.severeOnly && len(s.res.Findings) > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf(
				"  nothing serious — %d lesser findings hidden, press s", len(s.res.Findings))))
		} else {
			lines = append(lines, okStyle.Render("  the schematic and that bom agree"))
		}
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = lines[:h-1]
	return strings.Join(lines, "\n") + "\n" + m.diffFooter(w)
}

// diffCounts is the tally by kind, plus the skipped DNP count so the totals add up
// on screen.
func (m Model) diffCounts(w int) string {
	d := m.diff.res
	var parts []string
	for _, k := range []kicad.DiffKind{
		kicad.DiffMissing, kicad.DiffExtra, kicad.DiffDNP,
		kicad.DiffValue, kicad.DiffFootprint, kicad.DiffExcluded,
	} {
		if n := d.Counts()[k]; n > 0 {
			parts = append(parts, diffKindStyle(k).Render(fmt.Sprintf("%d %s", n, k)))
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
		return dimStyle.Render(fmt.Sprintf("%d symbols · %d bom designators", d.SchCount, d.BOMCount))
	}
	return padRender(strings.Join(parts, dimStyle.Render(" · ")), w)
}

func (m Model) diffFooter(w int) string {
	if m.diff.field.Focused() {
		return dimStyle.Render("  enter compare · tab leaves the path · esc back")
	}
	left := dimStyle.Render("  tab edit the path · enter compare · s ")
	if m.diff.severeOnly {
		left += okStyle.Render("serious only")
	} else {
		left += subtleStyle.Render("everything")
	}
	right := dimStyle.Render("↑↓ findings · esc back")
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + spaces(gap) + right
}
