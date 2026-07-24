package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/lcsc"
	"bomexpo/internal/value"
)

const searchDataTop = 7 // tab(1)+border(1)+title,input,status,colhead,rule(5)

const (
	scCode  = 9
	scPkg   = 7
	scStock = 9
	scPrice = 8
	scDs    = 9
	scMpn   = 18
)

// searchDsRange is the x-span of the DATASHEET column (content x=2, prefix 2,
// then code|pkg|stock|price with " | "/"   " 3-wide separators).
func searchDsRange() (int, int) {
	start := 4 + scCode + scPkg + scStock + scPrice + 3*4
	return start, start + scDs
}

type searchDebounceMsg struct{ seq int }

type searchState struct {
	field       textfield
	results     []lcsc.Part
	cursor      int
	top         int
	token       int
	debounce    int
	loading     bool
	inStockOnly bool
	pkgOnly     bool
	typeOnly    bool
	pkg         string
	kind        value.Kind
	total       int
}

func newSearchState() searchState {
	return searchState{field: newField("› ", "search LCSC…", 46)}
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

func (s searchState) filtered() []lcsc.Part {
	var out []lcsc.Part
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
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
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
	s := &m.search
	switch msg.String() {
	case "esc":
		s.field.Blur()
		m.mode = modeTable
		return m, nil
	case "enter":
		return m.assignSelected()
	case "down", "ctrl+n":
		s.cursor = min(len(s.filtered())-1, s.cursor+1)
		s.clampSearch(m.searchRows())
		return m, nil
	case "up", "ctrl+p":
		s.cursor = max(0, s.cursor-1)
		s.clampSearch(m.searchRows())
		return m, nil
	case "ctrl+s":
		s.inStockOnly = !s.inStockOnly
		s.cursor, s.top = 0, 0
		return m, nil
	case "ctrl+f":
		s.pkgOnly = !s.pkgOnly
		s.cursor, s.top = 0, 0
		return m, nil
	case "ctrl+t":
		s.typeOnly = !s.typeOnly
		s.cursor, s.top = 0, 0
		return m, nil
	}
	before := s.field.Value()
	s.field.Update(msg)
	if s.field.Value() != before {
		s.debounce++
		return m, searchDebounceCmd(s.debounce)
	}
	return m, nil
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
			if lo, hi := searchDsRange(); ms.X >= lo && ms.X < hi && f[row].Datasheet != "" {
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
	if m.cursor >= 0 && m.cursor < len(m.items) {
		m.items[m.cursor].LCSC = p.Code
		m.assigned[m.cursor] = &p
		m.status = fmt.Sprintf("%s ← %s  %s", m.items[m.cursor].ID(), p.Code, p.Model)
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
	it := m.items[m.cursor]

	title := subtleStyle.Render("assign ") + accentStyle.Render(it.ID()) +
		subtleStyle.Render(fmt.Sprintf("  (%s · %s)", it.Value, it.Footprint))

	var status string
	switch {
	case s.loading:
		status = m.spin.View() + " searching…"
	case len(s.results) == 0:
		status = dimStyle.Render("type to search LCSC")
	default:
		f := s.filtered()
		status = subtleStyle.Render(fmt.Sprintf("%d of %d", len(f), s.total)) + "   " + m.filterChips()
	}

	code, pkg, stock, price, ds, mpn := scCode, scPkg, scStock, scPrice, scDs, scMpn
	desc := w - (2 + code + pkg + stock + price + ds + mpn) - 3*6
	if desc < 8 {
		desc = 8
	}
	head := colHeadStyle.Render(padRender(strings.Join([]string{
		pad("LCSC", code), pad("PKG", pkg), pad("STOCK", stock),
		pad("PRICE", price), pad("DATASHEET", ds), pad("MPN", mpn), pad("DESCRIPTION", desc),
	}, " | "), w))
	rule := borderStyle.Render(strings.Repeat("─", w))

	lines := []string{title, s.field.View(), status, head, rule}
	f := s.filtered()
	vis := m.searchRows()
	end := min(len(f), s.top+vis)
	for i := s.top; i < end; i++ {
		p := f[i]
		dsc := ""
		if p.Datasheet != "" {
			dsc = "datasheet"
		}
		plain := []string{
			pad(p.Code, code), pad(p.Package, pkg), pad(groupThousands(p.Stock), stock),
			pad(p.PriceLabel(), price), pad(dsc, ds), pad(trunc(p.Model, mpn), mpn), pad(p.Description(), desc),
		}
		if i == s.cursor {
			lines = append(lines, selRowStyle.Render(padRender("▶ "+strings.Join(plain, "   "), w)))
			continue
		}
		sc := okStyle.Render(plain[2])
		if !p.InStock() {
			sc = badStyle.Render(pad("out", stock))
		}
		row := strings.Join([]string{
			accentStyle.Render(plain[0]), subtleStyle.Render(plain[1]), sc,
			warnStyle.Render(plain[3]), dsCellStyle(dsc, plain[4]),
			plain[5], descStyle.Render(plain[6]),
		}, sepStyle.Render(" | "))
		lines = append(lines, padRender("  "+row, w))
	}
	if len(f) == 0 && !s.loading && len(s.results) > 0 {
		lines = append(lines, dimStyle.Render("  nothing matches the filters — toggle ctrl+f pkg · ctrl+t type · ctrl+s stock"))
	}
	return strings.Join(lines, "\n")
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
	return strings.Join(chips, " ") + dimStyle.Render("  ^f ^t ^s")
}

func refPrefix(id string) string {
	i := 0
	for i < len(id) && (id[i] < '0' || id[i] > '9') {
		i++
	}
	return id[:i]
}
