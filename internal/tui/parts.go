package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
	"bomexpo/internal/taxonomy"
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
	// cat keeps only one leaf category, chosen from the category panel. Empty
	// means every category.
	cat string

	pinned []part.Part
}

func newPartsState() partsState {
	return partsState{field: newField("› ", "search any part by value, mpn or code…", 46)}
}

// preCat is the results with every filter but the category applied. The category
// panel groups over this, so picking a category doesn't make the other boxes
// disappear.
func (s partsState) preCat() []part.Part {
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

func (s partsState) filtered() []part.Part {
	rows := s.preCat()
	if s.cat == "" {
		return rows
	}
	var out []part.Part
	for _, p := range rows {
		if strings.EqualFold(strings.TrimSpace(p.Category), s.cat) {
			out = append(out, p)
		}
	}
	return out
}

// rows is what the table shows: the pinned parts first, then the search results
// with the pinned ones left out so nothing appears twice. Pins ignore the filters
// and outlive the query — they're the shortlist you're working with, and you have
// to be able to reach one to unpin it even when the search that found it is gone.
func (s partsState) rows() []part.Part {
	out := append([]part.Part(nil), s.pinned...)
	for _, p := range s.filtered() {
		if s.pinAt(p.Source, p.Code) < 0 {
			out = append(out, p)
		}
	}
	return out
}

// resultsFrom is the row index the search results start at, which is where the
// view draws a rule between the shortlist and everything else.
func (s partsState) resultsFrom() int { return len(s.pinned) }

// pinAt finds a pinned part by source and code, or -1.
func (s partsState) pinAt(source, code string) int {
	for i, p := range s.pinned {
		if p.Code == code && p.Source == source {
			return i
		}
	}
	return -1
}

// partsCount says what the row count is out of. A category filter only sees the
// rows we fetched, so saying "of 5000" there would claim more than it does.
func (m Model) partsCount(shown int) string {
	s := m.parts
	if s.cat != "" {
		return fmt.Sprintf("%d of the %d fetched", shown, len(s.results))
	}
	return fmt.Sprintf("%d of %d", shown, s.total)
}

func (m Model) partsRows() int {
	n := m.contentH() - 6 // header block + the pinned footer line
	if len(m.parts.pinned) > 0 {
		n-- // the rule between the shortlist and the results takes a line
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (s *partsState) clamp(vis int) {
	f := s.rows()
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

// catPageLimit and catRowsWanted bound the paging done for a category filter:
// enough pages to fill a screen, few enough not to hammer the vendor.
const (
	catPageLimit  = 4
	catRowsWanted = 40
)

func (m Model) partsSearchCmd(token int, keyword string) tea.Cmd {
	src := m.src()
	basicOnly := m.parts.basicOnly
	cat := m.parts.cat
	return func() tea.Msg {
		if src == nil {
			return partsDoneMsg{token: token, err: errNoSource}
		}
		q := part.Query{Keyword: keyword, Size: 100, BasicOnly: basicOnly}
		res, err := src.Search(q)
		if err != nil || cat == "" {
			return partsDoneMsg{token: token, res: res, err: err}
		}
		// A category is a filter over what comes back, so a keyword whose first
		// page holds few of that category needs more pages before the table looks
		// like anything.
		for page := 2; page <= catPageLimit && countIn(res.Items, cat) < catRowsWanted; page++ {
			q.Page = page
			more, err := src.Search(q)
			if err != nil || len(more.Items) == 0 {
				break
			}
			res.Items = append(res.Items, more.Items...)
		}
		return partsDoneMsg{token: token, res: res}
	}
}

// countIn is how many parts sit in a category.
func countIn(ps []part.Part, cat string) int {
	n := 0
	for _, p := range ps {
		if strings.EqualFold(strings.TrimSpace(p.Category), cat) {
			n++
		}
	}
	return n
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
//
// With a category picked and nothing typed it browses the category instead of
// showing an empty table: neither vendor takes a category id, so the category's
// own name becomes the keyword and the filter does the rest.
func (m Model) researchParts() (tea.Model, tea.Cmd) {
	m.parts.cursor, m.parts.top = 0, 0
	kw := strings.TrimSpace(m.parts.field.Value())
	if kw == "" && m.parts.cat != "" {
		kw = taxonomy.Keyword(m.parts.cat)
	}
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
	// Every search teaches the category list something: the crawl that builds it
	// can't reach every corner, but a search that lands in one can.
	if grown := taxonomy.Add(m.srcID(), msg.res.Items); len(grown) > len(m.cat.cats) {
		m.cat.cats = grown
	} else {
		m.cat.cats = taxonomy.Merge(m.cat.cats, taxonomy.FromParts(msg.res.Items))
	}
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
	f := m.parts.rows()
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
	key := msg.String()
	typing := m.parts.field.Focused()

	// Keys that work whichever pane has focus: the arrows always drive the list,
	// and the ctrl forms of every command stay available so nothing has to be
	// relearned.
	switch key {
	case "esc":
		if typing {
			m.parts.field.Blur() // one esc leaves the field, the next leaves the tab
			return m, nil
		}
		m.mode = modeTable
		return m, nil
	case "tab":
		return m.togglePartsFocus()
	case "shift+tab":
		return m.togglePartsFocus()
	case "enter":
		return m.togglePin()
	case "down", "ctrl+n":
		m.parts.cursor = min(len(m.parts.rows())-1, m.parts.cursor+1)
		m.parts.clamp(m.partsRows())
		return m, nil
	case "up", "ctrl+p":
		m.parts.cursor = max(0, m.parts.cursor-1)
		m.parts.clamp(m.partsRows())
		return m, nil
	case "ctrl+s":
		return m.togglePartsStock()
	case "ctrl+d":
		return m.openPartsDatasheet()
	case "ctrl+o":
		return m.switchPartsSource()
	case "ctrl+b":
		return m.togglePartsBasic()
	}

	if typing {
		before := m.parts.field.Value()
		m.parts.field.Update(msg)
		if m.parts.field.Value() != before {
			m.parts.debounce++
			return m, partsDebounceCmd(m.parts.debounce)
		}
		return m, nil
	}

	// With the list focused the letters are commands, not text.
	if mm, cmd, done := m.tabSwitchKey(key); done {
		return mm, cmd
	}
	switch key {
	case "/", "i":
		// Reaching for the search opens the category popup first: pick what kind
		// of part you want, then type inside it.
		return m.openCategories()
	case "t":
		return m.openCategories() // and t is how you change your mind later
	case "p", " ":
		return m.togglePin()
	case "c":
		return m.gotoTab(modeCompare)
	case "d":
		return m.openPartsDatasheet()
	case "s":
		return m.togglePartsStock()
	case "b":
		return m.togglePartsBasic()
	case "o":
		return m.switchPartsSource()
	case "g", "home":
		m.parts.cursor = 0
		m.parts.clamp(m.partsRows())
	case "G", "end":
		m.parts.cursor = max(0, len(m.parts.rows())-1)
		m.parts.clamp(m.partsRows())
	case "pgup":
		m.parts.cursor = max(0, m.parts.cursor-m.partsRows())
		m.parts.clamp(m.partsRows())
	case "pgdown":
		m.parts.cursor = min(len(m.parts.rows())-1, m.parts.cursor+m.partsRows())
		m.parts.clamp(m.partsRows())
	}
	return m, nil
}

// togglePartsFocus hands the keyboard between the query and the results.
func (m Model) togglePartsFocus() (tea.Model, tea.Cmd) {
	if m.parts.field.Focused() {
		m.parts.field.Blur()
	} else {
		m.parts.field.Focus()
	}
	return m, nil
}

func (m Model) togglePartsStock() (tea.Model, tea.Cmd) {
	m.parts.inStockOnly = !m.parts.inStockOnly
	m.parts.cursor, m.parts.top = 0, 0
	return m, nil
}

func (m Model) openPartsDatasheet() (tea.Model, tea.Cmd) {
	f := m.parts.rows()
	if m.parts.cursor >= 0 && m.parts.cursor < len(f) && f[m.parts.cursor].Datasheet != "" {
		openExternal(f[m.parts.cursor].Datasheet)
	}
	return m, nil
}

func (m Model) switchPartsSource() (tea.Model, tea.Cmd) {
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
}

func (m Model) togglePartsBasic() (tea.Model, tea.Cmd) {
	src := m.src()
	if src == nil || !src.Caps().BasicFilter {
		m.flash = fmt.Sprintf("%s has no basic library — switch source with o", m.srcLabel())
		return m, nil
	}
	m.parts.basicOnly = !m.parts.basicOnly
	return m.researchParts()
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

	title := focusMark(!s.field.Focused()) + subtleStyle.Render("browse ") +
		accentStyle.Render(m.srcLabel()) + subtleStyle.Render("  pin parts to compare them")
	if chips := m.sourceChips(); chips != "" {
		gap := w - lipgloss.Width(title) - lipgloss.Width(chips)
		if gap < 1 {
			gap = 1
		}
		title += spaces(gap) + chips
	}

	rows := s.rows()
	f := s.filtered()
	var status string
	switch {
	case s.loading:
		status = m.spin.View() + " searching…"
	case len(s.results) == 0 && !s.field.Focused():
		status = dimStyle.Render("press ") + accentStyle.Render("/") +
			dimStyle.Render(" to search "+m.srcLabel()+" — no board needed")
	case len(s.results) == 0:
		status = dimStyle.Render("type to search " + m.srcLabel() + " — no board needed")
	case len(f) == 0 && s.cat != "":
		status = warnStyle.Render(fmt.Sprintf("none of the %d results are in %s", len(s.results), s.cat)) +
			dimStyle.Render("   t to change the category")
	default:
		status = subtleStyle.Render(m.partsCount(len(f))) + "   " + m.partsChips()
	}

	c := m.resultCols(w)
	query := focusMark(s.field.Focused()) + s.field.View()
	lines := []string{title, query, status, partHead(c, w), borderStyle.Render(strings.Repeat("─", w))}

	vis := m.partsRows()
	end := min(len(rows), s.top+vis)
	for i := s.top; i < end; i++ {
		// a rule between the shortlist and the search results, so the pinned block
		// reads as its own thing rather than as the first few hits
		if i == s.resultsFrom() && i > 0 {
			lines = append(lines, borderStyle.Render(strings.Repeat("╌", w)))
		}
		p := rows[i]
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
	if len(rows) == 0 && !s.loading && len(s.results) > 0 {
		lines = append(lines, dimStyle.Render("  nothing in stock matches — toggle ^s"))
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + m.pinnedFooter(w)
}

func (m Model) pinnedFooter(w int) string {
	if len(m.parts.pinned) == 0 {
		tabs := "tab focuses the query"
		if m.parts.field.Focused() {
			tabs = "tab leaves the query"
		}
		return dimStyle.Render("  enter pin · " + tabs + " · nothing pinned yet")
	}
	codes := make([]string, 0, len(m.parts.pinned))
	for _, p := range m.parts.pinned {
		codes = append(codes, libCell(p.Lib, p.Code))
	}
	left := accentStyle.Render("  ◆ pinned ") + strings.Join(codes, dimStyle.Render(" · "))
	right := dimStyle.Render("enter pin/unpin · d datasheet")
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
	if s.cat != "" {
		chips = append(chips, accentStyle.Render("["+trunc(s.cat, 30)+"]"))
	}
	keys := []string{"s"}
	if src := m.src(); src != nil && src.Caps().BasicFilter {
		chips = append(chips, chip(s.basicOnly, "basic"))
		keys = append(keys, "b")
	}
	if len(m.srcs) > 1 {
		keys = append(keys, "o")
	}
	hint := "  " + strings.Join(keys, " ") + " · t category"
	if s.field.Focused() {
		// the letters are text right now, so name the forms that still work
		hint = "  ^" + strings.Join(keys, " ^")
	}
	return strings.Join(chips, " ") + dimStyle.Render(hint)
}
