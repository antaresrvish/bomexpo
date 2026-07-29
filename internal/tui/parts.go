package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
)

// partsDataTop is the first result row: tab(1)+border(1)+title,input,status,
// colhead,rule(5).
const partsDataTop = 7

// maxPinned is how many parts the compare matrix can show side by side and stay
// readable at a normal terminal width.
const maxPinned = 4

type partsDebounceMsg struct{ seq int }

type partsDoneMsg struct {
	token int
	res   part.Result
	err   error
}

// pinDetailMsg carries the full record for a freshly pinned part; search results
// come back with fewer parameters than a detail fetch, and the compare matrix
// wants all of them.
type pinDetailMsg struct {
	source string
	code   string
	part   part.Part
	err    error
}

// partsState is the Parts tab: a parts search that isn't tied to a board line
// item, plus the shortlist being compared.
type partsState struct {
	field    textfield
	results  []part.Part
	cursor   int
	top      int
	token    int
	debounce int
	loading  bool
	total    int

	inStockOnly bool
	basicOnly   bool

	pinned []part.Part
}

func newPartsState() partsState {
	return partsState{field: newField("› ", "search any part by value, mpn or code…", 46)}
}

func (s partsState) filtered() []part.Part {
	if !s.inStockOnly {
		return s.results
	}
	var out []part.Part
	for _, p := range s.results {
		if p.InStock() {
			out = append(out, p)
		}
	}
	return out
}

// pinAt finds a pinned part by source and code, or -1.
func (s partsState) pinAt(source, code string) int {
	for i, p := range s.pinned {
		if p.Code == code && p.Source == source {
			return i
		}
	}
	return -1
}

func (m Model) partsRows() int {
	n := m.contentH() - 6 // header block + the pinned footer line
	if n < 1 {
		n = 1
	}
	return n
}

func (s *partsState) clamp(vis int) {
	f := s.filtered()
	if len(f) == 0 {
		s.cursor, s.top = 0, 0
		return
	}
	s.cursor = clampInt(s.cursor, 0, len(f)-1)
	if s.cursor < s.top {
		s.top = s.cursor
	}
	if s.cursor >= s.top+vis {
		s.top = s.cursor - vis + 1
	}
	s.top = clampInt(s.top, 0, max(0, len(f)-1))
}

func partsDebounceCmd(seq int) tea.Cmd {
	return tea.Tick(keystrokeSettle, func(time.Time) tea.Msg {
		return partsDebounceMsg{seq: seq}
	})
}

func (m Model) partsSearchCmd(token int, keyword string) tea.Cmd {
	src := m.src()
	basicOnly := m.parts.basicOnly
	return func() tea.Msg {
		if src == nil {
			return partsDoneMsg{token: token, err: errNoSource}
		}
		res, err := src.Search(part.Query{Keyword: keyword, Size: 100, BasicOnly: basicOnly})
		return partsDoneMsg{token: token, res: res, err: err}
	}
}

func (m Model) pinDetailCmd(source, code string) tea.Cmd {
	src := m.src()
	return func() tea.Msg {
		if src == nil {
			return pinDetailMsg{source: source, code: code, err: errNoSource}
		}
		p, err := src.Detail(code)
		return pinDetailMsg{source: source, code: code, part: p, err: err}
	}
}

func (m Model) updatePartsDebounce(msg partsDebounceMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.parts.debounce {
		return m, nil
	}
	return m.researchParts()
}

// researchParts re-runs the Parts query, which a new keyword, a new source or a
// server-side filter all call for.
func (m Model) researchParts() (tea.Model, tea.Cmd) {
	m.parts.cursor, m.parts.top = 0, 0
	kw := strings.TrimSpace(m.parts.field.Value())
	if kw == "" {
		m.parts.results, m.parts.total = nil, 0
		return m, nil
	}
	m.parts.loading = true
	m.parts.token++
	return m, m.partsSearchCmd(m.parts.token, kw)
}

func (m Model) updatePartsDone(msg partsDoneMsg) (tea.Model, tea.Cmd) {
	if msg.token != m.parts.token {
		return m, nil
	}
	m.parts.loading = false
	if msg.err != nil {
		m.err = "parts: " + msg.err.Error()
		return m, nil
	}
	m.err = ""
	m.parts.results = msg.res.Items
	m.parts.total = msg.res.Total
	m.parts.cursor, m.parts.top = 0, 0
	return m, nil
}

// updatePinDetail swaps the fuller record in, as long as that part is still
// pinned — the user may have unpinned it while the fetch was in flight.
func (m Model) updatePinDetail(msg pinDetailMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.part.Code == "" {
		return m, nil // the search-result copy stands in fine
	}
	if i := m.parts.pinAt(msg.source, msg.code); i >= 0 {
		m.parts.pinned[i] = msg.part
	}
	return m, nil
}

// togglePin pins or unpins the row under the cursor, fetching the full record
// on a pin.
func (m Model) togglePin() (tea.Model, tea.Cmd) {
	f := m.parts.filtered()
	if m.parts.cursor < 0 || m.parts.cursor >= len(f) {
		return m, nil
	}
	p := f[m.parts.cursor]

	if i := m.parts.pinAt(p.Source, p.Code); i >= 0 {
		m.parts.pinned = append(m.parts.pinned[:i:i], m.parts.pinned[i+1:]...)
		m.flash = "unpinned " + p.Code
		return m, nil
	}
	if len(m.parts.pinned) >= maxPinned {
		m.flash = fmt.Sprintf("%d parts is the most you can compare — unpin one first", maxPinned)
		return m, nil
	}
	m.parts.pinned = append(m.parts.pinned, p)
	m.flash = fmt.Sprintf("pinned %s (%d/%d)", p.Code, len(m.parts.pinned), maxPinned)
	return m, tea.Batch(m.pinDetailCmd(p.Source, p.Code), m.landsCmd(p.Code))
}

// landsCmd downloads a part's footprint unless we already have it — from the
// board or from an earlier download.
func (m Model) landsCmd(code string) tea.Cmd {
	if code == "" || m.boardLandsFor(code) != nil {
		return nil
	}
	if _, have := m.edaLands[code]; have {
		return nil
	}
	return m.footprintCmd(code)
}

func (m Model) updatePartsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &m.parts
	switch msg.String() {
	case "esc":
		s.field.Blur()
		m.mode = modeTable
		return m, nil
	case "tab":
		s.field.Blur()
		return m.cycleTab(1)
	case "shift+tab":
		s.field.Blur()
		return m.cycleTab(-1)
	case "enter":
		return m.togglePin()
	case "down", "ctrl+n":
		s.cursor = min(len(s.filtered())-1, s.cursor+1)
		s.clamp(m.partsRows())
		return m, nil
	case "up", "ctrl+p":
		s.cursor = max(0, s.cursor-1)
		s.clamp(m.partsRows())
		return m, nil
	case "ctrl+s":
		s.inStockOnly = !s.inStockOnly
		s.cursor, s.top = 0, 0
		return m, nil
	case "ctrl+d":
		// shadows the field's forward-delete, which no one needs in a search box
		f := s.filtered()
		if s.cursor >= 0 && s.cursor < len(f) && f[s.cursor].Datasheet != "" {
			openExternal(f[s.cursor].Datasheet)
		}
		return m, nil
	case "ctrl+o":
		if len(m.srcs) < 2 {
			return m, nil
		}
		m = m.nextSrc()
		if src := m.src(); src != nil && !src.Caps().BasicFilter {
			m.parts.basicOnly = false
		}
		m.parts.results, m.parts.total = nil, 0
		m.flash = "source → " + m.srcLabel()
		return m.researchParts()
	case "ctrl+b":
		src := m.src()
		if src == nil || !src.Caps().BasicFilter {
			m.flash = fmt.Sprintf("%s has no basic library — switch source with ^o", m.srcLabel())
			return m, nil
		}
		s.basicOnly = !s.basicOnly
		return m.researchParts()
	}
	before := s.field.Value()
	s.field.Update(msg)
	if s.field.Value() != before {
		s.debounce++
		return m, partsDebounceCmd(s.debounce)
	}
	return m, nil
}

func (m Model) mouseParts(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	s := &m.parts
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			s.cursor = max(0, s.cursor-1)
		} else if ms.Button == tea.MouseWheelDown {
			s.cursor = min(len(s.filtered())-1, s.cursor+1)
		}
		s.clamp(m.partsRows())
		return m, nil
	}
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	row := s.top + (ms.Y - partsDataTop)
	f := s.filtered()
	if row < 0 || row >= len(f) {
		return m, nil
	}
	if lo, hi := m.resultCols(m.contentW()).dsRange(); ms.X >= lo && ms.X < hi && f[row].Datasheet != "" {
		openExternal(f[row].Datasheet)
		s.cursor = row
		s.clamp(m.partsRows())
		return m, nil
	}
	if row == s.cursor {
		return m.togglePin()
	}
	s.cursor = row
	s.clamp(m.partsRows())
	return m, nil
}

func (m Model) viewParts(w, h int) string {
	s := m.parts

	title := subtleStyle.Render("browse ") + accentStyle.Render(m.srcLabel()) +
		subtleStyle.Render("  pin parts to compare them")
	if chips := m.sourceChips(); chips != "" {
		gap := w - lipgloss.Width(title) - lipgloss.Width(chips)
		if gap < 1 {
			gap = 1
		}
		title += spaces(gap) + chips
	}

	f := s.filtered()
	var status string
	switch {
	case s.loading:
		status = m.spin.View() + " searching…"
	case len(s.results) == 0:
		status = dimStyle.Render("type to search " + m.srcLabel() + " — no board needed")
	default:
		status = subtleStyle.Render(fmt.Sprintf("%d of %d", len(f), s.total)) + "   " + m.partsChips()
	}

	c := m.resultCols(w)
	lines := []string{title, s.field.View(), status, partHead(c, w), borderStyle.Render(strings.Repeat("─", w))}

	vis := m.partsRows()
	end := min(len(f), s.top+vis)
	for i := s.top; i < end; i++ {
		p := f[i]
		cursor, pinned := i == s.cursor, s.pinAt(p.Source, p.Code) >= 0
		marker := "  "
		switch {
		case cursor && pinned:
			marker = "▶◆"
		case cursor:
			marker = "▶ "
		case pinned:
			marker = "◆ "
		}
		lines = append(lines, partRow(p, c, w, marker, cursor))
	}
	if len(f) == 0 && !s.loading && len(s.results) > 0 {
		lines = append(lines, dimStyle.Render("  nothing in stock matches — toggle ^s"))
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + m.pinnedFooter(w)
}

func (m Model) pinnedFooter(w int) string {
	if len(m.parts.pinned) == 0 {
		return dimStyle.Render("  enter pin · ^d datasheet · ^s stock · nothing pinned yet")
	}
	codes := make([]string, 0, len(m.parts.pinned))
	for _, p := range m.parts.pinned {
		codes = append(codes, libCell(p.Lib, p.Code))
	}
	left := accentStyle.Render("  ◆ pinned ") + strings.Join(codes, dimStyle.Render(" · "))
	right := dimStyle.Render("enter pin/unpin · ^d datasheet")
	if len(m.parts.pinned) >= 2 {
		right = okStyle.Render("Compare tab ready") + dimStyle.Render(" · tab to open")
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + spaces(gap) + right
}

func (m Model) partsChips() string {
	s := m.parts
	chip := func(on bool, label string) string {
		if on {
			return badgeOk.Render(label)
		}
		return dimStyle.Render("[" + label + "]")
	}
	chips := []string{chip(s.inStockOnly, "in-stock")}
	hint := "  ^s"
	if src := m.src(); src != nil && src.Caps().BasicFilter {
		chips = append(chips, chip(s.basicOnly, "basic"))
		hint += " ^b"
	}
	if len(m.srcs) > 1 {
		hint += " ^o"
	}
	return strings.Join(chips, " ") + dimStyle.Render(hint)
}
