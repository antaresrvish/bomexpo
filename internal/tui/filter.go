package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
)

// The filter is a handful of space-separated terms, all of which must match:
//
//	net:GND          on that net
//	ref:C1  val:100nF  fp:0402  lcsc:C1525
//	lib:basic        assembly library standing
//	st:unassigned    line-item state
//	0402             bare text: reference, value or footprint
//	-st:excluded     leading minus inverts the term
//
// Matching is case-insensitive, and per-field — see fieldMatch. Values and
// references match a token's start (val:1k does not find 5.1k), while footprints
// and nets match anywhere (net:3v3 finds +3V3).
var filterKeys = []string{"net", "ref", "val", "fp", "lcsc", "lib", "st"}

type filterTerm struct {
	key    string // one of filterKeys, or "" for bare text
	want   string
	negate bool
}

type filter struct {
	raw   string
	terms []filterTerm
	// unknown holds key-looking prefixes we didn't recognise, so a typo says so
	// instead of silently matching nothing.
	unknown []string
}

func (f filter) active() bool { return len(f.terms) > 0 }

func parseFilter(s string) filter {
	f := filter{raw: strings.TrimSpace(s)}
	for _, tok := range strings.Fields(f.raw) {
		t := filterTerm{}
		if strings.HasPrefix(tok, "-") {
			t.negate, tok = true, tok[1:]
		}
		if tok == "" {
			continue
		}
		if k, v, ok := strings.Cut(tok, ":"); ok {
			lk := strings.ToLower(k)
			switch {
			case knownKey(lk):
				t.key, t.want = lk, strings.ToLower(v)
			case isWord(lk):
				// looks like a key but isn't one — say so rather than
				// searching for the literal text and finding nothing
				f.unknown = append(f.unknown, lk)
				continue
			default:
				t.want = strings.ToLower(tok)
			}
		} else {
			t.want = strings.ToLower(tok)
		}
		if t.want == "" {
			continue
		}
		f.terms = append(f.terms, t)
	}
	return f
}

func knownKey(k string) bool {
	for _, want := range filterKeys {
		if k == want {
			return true
		}
	}
	return false
}

func isWord(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// match reports whether line item i survives the filter.
func (f filter) match(m Model, i int) bool {
	for _, t := range f.terms {
		if t.hit(m, i) == t.negate {
			return false
		}
	}
	return true
}

func (t filterTerm) hit(m Model, i int) bool {
	it := m.items[i]
	switch t.key {
	case "net":
		return anyMatch("net", it.Nets, t.want)
	case "ref":
		return anyMatch("ref", it.Designators, t.want) || fieldMatch("ref", it.ID(), t.want)
	case "val":
		return fieldMatch("val", it.Value, t.want)
	case "fp":
		return fieldMatch("fp", it.Footprint, t.want)
	case "lcsc":
		return fieldMatch("lcsc", it.LCSC, t.want)
	case "lib":
		return t.hitLib(m.libOf(i))
	case "st":
		return t.hitState(m, i)
	}
	// bare text: the columns you'd scan by eye
	return fieldMatch("ref", it.ID(), t.want) || anyMatch("ref", it.Designators, t.want) ||
		fieldMatch("val", it.Value, t.want) || fieldMatch("fp", it.Footprint, t.want)
}

// fieldMatch is the one place that decides what "matches" means per field, so
// the dropdown offers exactly what the filter will keep.
//
// Values, references and part codes are short whole tokens, so they match on a
// token's start: otherwise val:1k finds 5.1k, which is never what you meant.
// Footprints and net names are compound ("C_0402_1005Metric", "+3V3"), so a
// substring anywhere is what's useful there.
func fieldMatch(key, hay, needle string) bool {
	if needle == "" {
		return true
	}
	switch key {
	case "val", "ref", "lcsc":
		return tokenPrefix(hay, needle)
	default:
		return contains(hay, needle)
	}
}

func anyMatch(key string, hay []string, needle string) bool {
	for _, h := range hay {
		if fieldMatch(key, h, needle) {
			return true
		}
	}
	return false
}

// tokenPrefix reports whether any word in hay starts with needle.
func tokenPrefix(hay, needle string) bool {
	for _, tok := range strings.FieldsFunc(strings.ToLower(hay), isTokenSep) {
		if strings.HasPrefix(tok, needle) {
			return true
		}
	}
	return false
}

// isTokenSep splits a value into the words a person would read: "4.7uF ±10% 16V"
// is three, and "5.1k" is one.
func isTokenSep(r rune) bool {
	switch r {
	case ' ', '\t', ',', ';', '/', '(', ')', '[', ']', '±', '%', '~':
		return true
	}
	return false
}

func (t filterTerm) hitLib(k part.LibKind) bool {
	switch t.want {
	case "basic":
		return k == part.LibBasic
	case "preferred", "pref":
		return k == part.LibPreferred
	case "extended", "ext":
		return k == part.LibExtended
	case "none", "unknown":
		return !k.Known()
	}
	return contains(k.String(), t.want)
}

func (t filterTerm) hitState(m Model, i int) bool {
	st := m.stateOf(i)
	switch t.want {
	case "ok":
		return st == stOK
	case "unassigned", "todo":
		return st == stUnassigned
	case "oos", "nostock", "out":
		return st == stOutOfStock
	case "mismatch":
		return st == stMismatch
	case "excluded":
		return st == stExcluded
	case "dnp":
		return m.items[i].DNP
	case "rot":
		return m.items[i].HasRotOverride
	}
	return false
}

func contains(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), needle)
}

// filterState is the filter bar: the query being typed, the one in force, and
// where the completion dropdown is pointing.
type filterState struct {
	field textfield
	open  bool // the bar has the keyboard
	f     filter
	sug   int // highlighted suggestion
}

func newFilterState() filterState {
	// no long placeholder: the dropdown below shows what's on offer
	return filterState{field: newField("", "filter…", 60)}
}

// visible reports whether the bar takes a row: while typing, and afterwards so a
// filter is never in force invisibly.
func (m Model) filterBarVisible() bool { return m.filter.open || m.filter.f.active() }

// matchedRefs are the designators the filter selected, for the board view. Nil
// when no filter is on, which means "don't dim anything".
func (m Model) matchedRefs() map[string]bool {
	if !m.filter.f.active() {
		return nil
	}
	out := map[string]bool{}
	for _, i := range m.view {
		for _, d := range m.items[i].Designators {
			out[d] = true
		}
	}
	return out
}

func (m Model) openFilter() (tea.Model, tea.Cmd) {
	m.filter.open = true
	m.filter.sug = 0
	m.filter.field.Focus()
	return m, nil
}

// closeFilter puts the keyboard back on the table. clear also drops the query.
func (m Model) closeFilter(clear bool) (tea.Model, tea.Cmd) {
	m.filter.open = false
	m.filter.field.Blur()
	if clear {
		m.filter.field.SetValue("")
		m.filter.f = filter{}
		return m.reindex(), nil
	}
	return m, nil
}

func (m Model) updateFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sug := m.suggestions()

	switch msg.String() {
	case "esc":
		return m.closeFilter(true)
	case "tab":
		// tab completes the word being typed and leaves the query focused
		if len(sug) > 0 {
			return m.acceptSuggestion(), nil
		}
		return m, nil
	case "enter":
		// enter is done: the dropdown closes and the table takes the keyboard
		mm, cmd := m.closeFilter(false)
		m = mm.(Model)
		m.cursor, m.top = 0, 0
		m.clampScroll()
		return m, cmd
	// the arrows walk the dropdown; enter is how you get down into the table
	case "down", "ctrl+n":
		if n := shownSuggestions(sug); n > 0 {
			m.filter.sug = (m.filter.sug + 1) % n
		}
		return m, nil
	case "up", "ctrl+p":
		if n := shownSuggestions(sug); n > 0 {
			m.filter.sug = (m.filter.sug - 1 + n) % n
		}
		return m, nil
	}

	before := m.filter.field.Value()
	m.filter.field.Update(msg)
	if m.filter.field.Value() == before {
		return m, nil
	}
	// filtering is local, so there's nothing to wait for — apply as you type
	m.filter.f = parseFilter(m.filter.field.Value())
	m.filter.sug = 0
	m.cursor, m.top = 0, 0
	return m.reindex(), nil
}

func shownSuggestions(sug []suggestion) int { return min(len(sug), sugMax) }

func (m Model) filterBar(w int) string {
	// The bar is bright while it has the keyboard and dim once the table does, so
	// there's always something on screen saying where you're typing.
	var left string
	if m.filter.open {
		left = accentStyle.Render("▸ /") + " " + m.filter.field.View()
	} else {
		left = dimStyle.Render("  /") + " " + subtleStyle.Render(m.filter.f.raw) +
			dimStyle.Render("   / edit · esc clear")
	}

	right := subtleStyle.Render(fmt.Sprintf("%d of %d", m.rows(), len(m.items)))
	switch {
	case len(m.filter.f.unknown) > 0:
		right = badStyle.Render("unknown: " + strings.Join(m.filter.f.unknown, " ") +
			dimStyle.Render("  try "+strings.Join(filterKeys, " ")))
	case m.filter.f.active() && m.rows() == 0:
		right = badStyle.Render("nothing matches")
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + spaces(gap) + right
}
