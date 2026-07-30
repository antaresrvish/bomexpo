package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
	"bomexpo/internal/value"
)

const searchDataTop = 7 // tab(1)+border(1)+title,input,status,colhead,rule(5)

// keystrokeSettle is how long typing pauses before a query goes out.
const keystrokeSettle = 300 * time.Millisecond

type searchDebounceMsg struct{ seq int }

type searchState struct {
	field       textfield
	results     []part.Part
	cursor      int
	top         int
	token       int
	debounce    int
	loading     bool
	inStockOnly bool
	pkgOnly     bool
	typeOnly    bool
	// basicOnly asks the source for its basic library only. Unlike the other
	// filters this one is applied server-side, so toggling it re-searches.
	basicOnly bool
	pkg       string
	kind      value.Kind
	total     int
}

func newSearchState() searchState {
	return searchState{field: newField("› ", "search parts…", 46)}
}

func (s *searchState) begin(keyword, footprint, val, prefix string) {
	s.field.SetValue(keyword)
	s.field.Focus()
	s.results = nil
	s.cursor, s.top = 0, 0
	s.loading = true
	s.pkg = sizeCode.FindString(footprint)
	s.pkgOnly = s.pkg != ""
	s.kind = deriveKind(val, prefix)
	s.typeOnly = s.kind != value.Unknown
}

func deriveKind(val, prefix string) value.Kind {
	if v, ok := value.ExtractValue(val); ok {
		return v.Kind
	}
	switch prefix {
	case "R":
		return value.Resistance
	case "C":
		return value.Capacitance
	case "L", "FB":
		return value.Inductance
	}
	return value.Unknown
}

func (s searchState) filtered() []part.Part {
	var out []part.Part
	for _, p := range s.results {
		if s.inStockOnly && !p.InStock() {
			continue
		}
		if s.pkgOnly && s.pkg != "" && !strings.EqualFold(strings.TrimSpace(p.Package), s.pkg) {
			continue
		}
		if s.typeOnly && s.kind != value.Unknown {
			if v, ok := value.ExtractValue(p.Description()); ok && v.Kind != s.kind {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func searchDebounceCmd(seq int) tea.Cmd {
	return tea.Tick(keystrokeSettle, func(time.Time) tea.Msg {
		return searchDebounceMsg{seq: seq}
	})
}

func (m Model) updateSearchDebounce(msg searchDebounceMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.search.debounce {
		return m, nil
	}
	kw := strings.TrimSpace(m.search.field.Value())
	if kw == "" {
		m.search.results = nil
		return m, nil
	}
	m.search.loading = true
	m.search.token++
	return m, m.searchCmd(m.search.token, kw)
}

func (m Model) updateSearch(msg searchDoneMsg) (tea.Model, tea.Cmd) {
	if msg.token != m.search.token {
		return m, nil
	}
	m.search.loading = false
	if msg.err != nil {
		m.err = "search: " + msg.err.Error()
		return m, nil
	}
	m.err = ""
	m.search.results = msg.res.Items
	m.search.total = msg.res.Total
	m.search.cursor, m.search.top = 0, 0
	return m, nil
}

func (m Model) updateSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	typing := m.search.field.Focused()

	// The arrows and the ctrl forms work whichever pane has focus.
	switch key {
	case "esc":
		if typing {
			m.mode = modeTable // in the query, esc leaves the search
			return m, nil
		}
		// Out of it, the first esc goes back to what you typed and the second leaves,
		// so a wrong keyword is one key from being fixed.
		m.search.field.Focus()
		return m, nil
	case "tab", "shift+tab":
		return m.toggleSearchFocus()
	case "enter":
		return m.assignSelected()
	case "down", "ctrl+n":
		// Reaching into the results means you are done typing, so the field lets go
		// and the letters become commands. esc puts you back in it.
		m.search.field.Blur()
		m.search.cursor = min(len(m.search.filtered())-1, m.search.cursor+1)
		m.search.clampSearch(m.searchRows())
		return m, nil
	case "up", "ctrl+p":
		m.search.field.Blur()
		m.search.cursor = max(0, m.search.cursor-1)
		m.search.clampSearch(m.searchRows())
		return m, nil
	case "ctrl+s":
		return m.toggleSearchFilter("stock")
	case "ctrl+f":
		return m.toggleSearchFilter("pkg")
	case "ctrl+t":
		return m.toggleSearchFilter("type")
	case "ctrl+o":
		return m.switchSource()
	case "ctrl+b":
		return m.toggleSearchFilter("basic")
	}

	if typing {
		before := m.search.field.Value()
		m.search.field.Update(msg)
		if m.search.field.Value() != before {
			m.search.debounce++
			return m, searchDebounceCmd(m.search.debounce)
		}
		return m, nil
	}

	// With the results focused the letters are commands.
	if mm, cmd, done := m.tabSwitchKey(key); done {
		return mm, cmd
	}
	switch key {
	case "/", "i":
		m.search.field.Focus()
	case "s":
		return m.toggleSearchFilter("stock")
	case "f":
		return m.toggleSearchFilter("pkg")
	case "t":
		return m.toggleSearchFilter("type")
	case "b":
		return m.toggleSearchFilter("basic")
	case "o":
		return m.switchSource()
	case "d":
		f := m.search.filtered()
		if m.search.cursor >= 0 && m.search.cursor < len(f) && f[m.search.cursor].Datasheet != "" {
			openExternal(f[m.search.cursor].Datasheet)
		}
	case "g", "home":
		m.search.cursor = 0
		m.search.clampSearch(m.searchRows())
	case "G", "end":
		m.search.cursor = max(0, len(m.search.filtered())-1)
		m.search.clampSearch(m.searchRows())
	}
	return m, nil
}

// toggleSearchFocus hands the keyboard between the query and the results.
func (m Model) toggleSearchFocus() (tea.Model, tea.Cmd) {
	if m.search.field.Focused() {
		m.search.field.Blur()
	} else {
		m.search.field.Focus()
	}
	return m, nil
}

// toggleSearchFilter flips one result filter. basic is server-side, so it
// re-searches; the rest are local.
func (m Model) toggleSearchFilter(which string) (tea.Model, tea.Cmd) {
	switch which {
	case "stock":
		m.search.inStockOnly = !m.search.inStockOnly
	case "pkg":
		m.search.pkgOnly = !m.search.pkgOnly
	case "type":
		m.search.typeOnly = !m.search.typeOnly
	case "basic":
		src := m.src()
		if src == nil || !src.Caps().BasicFilter {
			m.flash = fmt.Sprintf("%s has no basic library — switch source with o", m.srcLabel())
			return m, nil
		}
		m.search.basicOnly = !m.search.basicOnly
		return m.research()
	}
	m.search.cursor, m.search.top = 0, 0
	return m, nil
}

// switchSource moves to the next parts source and re-runs the query there.
// Filters the new source can't honour are dropped rather than left showing.
func (m Model) switchSource() (tea.Model, tea.Cmd) {
	if len(m.srcs) < 2 {
		return m, nil
	}
	m = m.nextSrc()
	if src := m.src(); src != nil && !src.Caps().BasicFilter {
		m.search.basicOnly = false
	}
	m.search.results = nil
	m.search.total = 0
	m.flash = "source → " + m.srcLabel()
	return m.research()
}

// research re-runs the current query. Needed whenever something the source
// itself decides changes — the source, or a server-side filter.
func (m Model) research() (tea.Model, tea.Cmd) {
	m.search.cursor, m.search.top = 0, 0
	kw := strings.TrimSpace(m.search.field.Value())
	if kw == "" {
		m.search.results = nil
		return m, nil
	}
	m.search.loading = true
	m.search.token++
	return m, m.searchCmd(m.search.token, kw)
}

func (m Model) srcLabel() string {
	if s := m.src(); s != nil {
		return s.Label()
	}
	return "no source"
}

func (m Model) mouseSearch(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	s := &m.search
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			s.cursor = max(0, s.cursor-1)
		} else if ms.Button == tea.MouseWheelDown {
			s.cursor = min(len(s.filtered())-1, s.cursor+1)
		}
		s.clampSearch(m.searchRows())
		return m, nil
	}
	if click && ms.Button == tea.MouseLeft {
		row := s.top + (ms.Y - searchDataTop)
		f := s.filtered()
		if row >= 0 && row < len(f) {
			if lo, hi := m.resultCols(m.contentW()).dsRange(); ms.X >= lo && ms.X < hi && f[row].Datasheet != "" {
				openExternal(f[row].Datasheet)
				s.cursor = row
				s.clampSearch(m.searchRows())
				return m, nil
			}
			if row == s.cursor {
				return m.assignSelected()
			}
			s.cursor = row
			s.clampSearch(m.searchRows())
		}
	}
	return m, nil
}

func (s *searchState) clampSearch(vis int) {
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

func (m Model) assignSelected() (tea.Model, tea.Cmd) {
	f := m.search.filtered()
	if m.search.cursor < 0 || m.search.cursor >= len(f) {
		return m, nil
	}
	p := f[m.search.cursor]
	if i := m.sel(); i >= 0 {
		m.items[i].LCSC = p.Code
		m.assigned[i] = &p
		m.status = fmt.Sprintf("%s ← %s  %s", m.items[i].ID(), p.Code, p.MPN)
	}
	m.search.field.Blur()
	m.mode = modeTable
	return m, nil
}

func (m Model) searchRows() int {
	n := m.contentH() - 5
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) viewSearch(w, h int) string {
	s := m.search
	i := m.sel()
	if i < 0 {
		return dimStyle.Render("nothing selected to assign")
	}
	it := m.items[i]

	title := focusMark(!s.field.Focused()) + subtleStyle.Render("assign ") +
		accentStyle.Render(it.ID()) +
		subtleStyle.Render(fmt.Sprintf("  (%s · %s)", it.Value, it.Footprint))
	// The source picker rides on the title row so adding it shifts no rows.
	if chips := m.sourceChips(); chips != "" {
		gap := w - lipgloss.Width(title) - lipgloss.Width(chips)
		if gap < 1 {
			gap = 1
		}
		title += spaces(gap) + chips
	}

	var status string
	switch {
	case s.loading:
		status = m.spin.View() + " searching…"
	case len(s.results) == 0:
		status = dimStyle.Render("type to search " + m.srcLabel())
	default:
		f := s.filtered()
		status = subtleStyle.Render(fmt.Sprintf("%d of %d", len(f), s.total)) + "   " + m.filterChips()
	}

	c := m.resultCols(w)
	query := focusMark(s.field.Focused()) + s.field.View()
	lines := []string{title, query, status, partHead(c, w), borderStyle.Render(strings.Repeat("─", w))}
	f := s.filtered()
	vis := m.searchRows()
	end := min(len(f), s.top+vis)
	for i := s.top; i < end; i++ {
		marker := "  "
		if i == s.cursor {
			marker = "▶ "
		}
		lines = append(lines, partRow(f[i], c, w, marker, i == s.cursor))
	}
	if len(f) == 0 && !s.loading && len(s.results) > 0 {
		lines = append(lines, dimStyle.Render("  nothing matches the filters — tab out and toggle f pkg · t type · s stock"))
	}
	return strings.Join(lines, "\n")
}

// sourceChips shows which parts source is active. It's an indicator, not a
// control — ^o switches.
func (m Model) sourceChips() string {
	if len(m.srcs) < 2 {
		return ""
	}
	out := make([]string, 0, len(m.srcs))
	for i, p := range m.srcs {
		if i == m.srcIdx {
			out = append(out, badgeOk.Render(p.ID()))
			continue
		}
		out = append(out, dimStyle.Render(p.ID()))
	}
	key := "o"
	if m.search.field.Focused() || m.parts.field.Focused() {
		key = "^o"
	}
	return strings.Join(out, " ") + dimStyle.Render("  "+key)
}

func (m Model) filterChips() string {
	s := m.search
	chip := func(on bool, label string) string {
		if on {
			return badgeOk.Render(label)
		}
		return dimStyle.Render("[" + label + "]")
	}
	var chips []string
	if s.pkg != "" {
		chips = append(chips, chip(s.pkgOnly, s.pkg))
	}
	if s.kind != value.Unknown {
		chips = append(chips, chip(s.typeOnly, s.kind.String()))
	}
	chips = append(chips, chip(s.inStockOnly, "in-stock"))
	keys := []string{"f", "t", "s"}
	if src := m.src(); src != nil && src.Caps().BasicFilter {
		chips = append(chips, chip(s.basicOnly, "basic"))
		keys = append(keys, "b")
	}
	hint := "  " + strings.Join(keys, " ")
	if s.field.Focused() {
		hint = "  ^" + strings.Join(keys, " ^") // the letters are text right now
	}
	return strings.Join(chips, " ") + dimStyle.Render(hint)
}

func refPrefix(id string) string {
	i := 0
	for i < len(id) && (id[i] < '0' || id[i] > '9') {
		i++
	}
	return id[:i]
}
