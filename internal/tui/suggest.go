package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/part"
)

// Nobody remembers their own net names, so the query completes itself: the keys
// when empty, and after "net:" the board's actual nets with their line counts.

const (
	sugMax   = 8 // rows in the dropdown
	sugMinW  = 26
	sugMaxW  = 46
	sugCount = 5 // right-hand column for the count
)

// padRight right-aligns in a fixed width, so a column of numbers lines up on
// its digits.
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return spaces(n-w) + s
	}
	return trunc(s, n)
}

type suggestion struct {
	value string // what gets inserted
	label string // what's shown, same as value unless it needs decorating
	count int    // line items this would show; -1 when counting is meaningless
}

// suggestKeys are offered when no key has been typed yet, in the order most
// people want them.
var suggestKeys = []struct{ key, what string }{
	{"net", "what a part connects to"},
	{"val", "component value"},
	{"fp", "footprint"},
	{"st", "assigned, out of stock, dnp…"},
	{"lib", "basic / extended"},
	{"ref", "designator"},
	{"lcsc", "part code"},
}

// lastToken is the word being typed. Completion works on the end of the query,
// which is where you always are while typing one.
func lastToken(s string) string {
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// replaceLastToken swaps the word being typed for a finished one.
func replaceLastToken(s, with string) string {
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[:i+1] + with
	}
	return with
}

// suggestions offers completions for the word being typed: the filter keys, or
// the real values behind a key.
func (m Model) suggestions() []suggestion {
	tok := lastToken(m.filter.field.Value())
	neg := strings.HasPrefix(tok, "-")
	tok = strings.TrimPrefix(tok, "-")

	key, val, hasColon := strings.Cut(tok, ":")
	if !hasColon || !knownKey(strings.ToLower(key)) {
		return keySuggestions(strings.ToLower(tok), neg)
	}
	return m.valueSuggestions(strings.ToLower(key), strings.ToLower(val), neg)
}

func keySuggestions(partial string, neg bool) []suggestion {
	var out []suggestion
	for _, k := range suggestKeys {
		if partial != "" && !strings.HasPrefix(k.key, partial) {
			continue
		}
		v := k.key + ":"
		if neg {
			v = "-" + v
		}
		out = append(out, suggestion{value: v, label: k.key + ":  " + k.what, count: -1})
	}
	return out
}

// baseFilter is everything already typed except the word being completed. The
// counts are taken over what that leaves, so a suggestion's number is what
// adding it would actually show — not a board-wide total that might combine to
// nothing.
func (m Model) baseFilter() filter {
	q := m.filter.field.Value()
	if i := strings.LastIndexAny(q, " \t"); i >= 0 {
		return parseFilter(q[:i])
	}
	return filter{}
}

func (m Model) valueSuggestions(key, partial string, neg bool) []suggestion {
	base := m.baseFilter()
	// each visits the line items the rest of the query already allows
	each := func(f func(i int)) {
		for i := range m.items {
			if base.match(m, i) {
				f(i)
			}
		}
	}

	counts := map[string]int{}
	order := []string{} // first-seen, so fixed vocabularies keep their order
	add := func(v string) {
		if v == "" {
			return
		}
		if _, seen := counts[v]; !seen {
			order = append(order, v)
		}
		counts[v]++
	}

	fixed := false
	switch key {
	case "net":
		each(func(i int) {
			for _, n := range m.items[i].Nets {
				add(n)
			}
		})
	case "val":
		each(func(i int) { add(m.items[i].Value) })
	case "fp":
		each(func(i int) { add(m.items[i].Footprint) })
	case "ref":
		each(func(i int) {
			for _, d := range m.items[i].Designators {
				add(d)
			}
		})
	case "lcsc":
		each(func(i int) { add(m.items[i].LCSC) })
	case "lib":
		fixed = true
		for _, v := range []string{"basic", "preferred", "extended", "none"} {
			order = append(order, v)
			counts[v] = 0
		}
		each(func(i int) {
			switch m.libOf(i) {
			case part.LibBasic:
				counts["basic"]++
			case part.LibPreferred:
				counts["preferred"]++
			case part.LibExtended:
				counts["extended"]++
			default:
				counts["none"]++
			}
		})
	case "st":
		fixed = true
		for _, v := range []string{"unassigned", "oos", "mismatch", "ok", "excluded", "dnp", "rot"} {
			order = append(order, v)
			counts[v] = 0
		}
		each(func(i int) {
			counts[stateWord(m.stateOf(i))]++
			if m.items[i].DNP {
				counts["dnp"]++
			}
			if m.items[i].HasRotOverride {
				counts["rot"]++
			}
		})
	}

	out := make([]suggestion, 0, len(order))
	for _, v := range order {
		// narrow the same way the filter will, so the dropdown never offers a
		// value that the query would then throw away
		if !fieldMatch(key, v, partial) {
			continue
		}
		full := key + ":" + v
		if neg {
			full = "-" + full
		}
		out = append(out, suggestion{value: full, label: v, count: counts[v]})
	}
	if !fixed {
		// busiest first: the rails and the parts you have most of
		sort.SliceStable(out, func(i, j int) bool {
			if a, b := out[i].count, out[j].count; a != b {
				return a > b
			}
			return strings.ToLower(out[i].label) < strings.ToLower(out[j].label)
		})
	}
	return out
}

func stateWord(st itemState) string {
	switch st {
	case stUnassigned:
		return "unassigned"
	case stOutOfStock:
		return "oos"
	case stShort:
		return "short"
	case stFootprint:
		return "footprint"
	case stMismatch:
		return "mismatch"
	case stExcluded:
		return "excluded"
	}
	return "ok"
}

// accept completes the word being typed with the highlighted suggestion, and
// leaves a trailing space so the next term can be typed straight away.
func (m Model) acceptSuggestion() Model {
	sug := m.suggestions()
	if len(sug) == 0 {
		return m
	}
	s := sug[clampInt(m.filter.sug, 0, len(sug)-1)]

	q := replaceLastToken(m.filter.field.Value(), s.value)
	if !strings.HasSuffix(s.value, ":") {
		q += " " // a finished term; a bare key still needs its value
	}
	m.filter.field.SetValue(q)
	m.filter.f = parseFilter(q)
	m.filter.sug = 0
	m.cursor, m.top = 0, 0
	return m.reindex()
}

// suggestBox renders the dropdown. It returns nil when there's nothing to offer.
func (m Model) suggestBox(maxW int) []string {
	if !m.filter.open {
		return nil
	}
	sug := m.suggestions()
	if len(sug) == 0 {
		return nil
	}

	w := sugMinW
	for _, s := range sug {
		if n := lipgloss.Width(s.label) + sugCount + 4; n > w {
			w = n
		}
	}
	w = min(min(w, sugMaxW), maxW)
	inner := w - 2
	if inner < 8 {
		return nil
	}

	shown := sug
	more := 0
	if len(shown) > sugMax {
		more = len(shown) - sugMax
		shown = shown[:sugMax]
	}
	cur := clampInt(m.filter.sug, 0, len(shown)-1)

	title := "filter by…"
	if tok := lastToken(m.filter.field.Value()); strings.Contains(tok, ":") {
		title = strings.TrimPrefix(strings.SplitN(tok, ":", 2)[0], "-") +
			dimStyle.Render("  parts")
	}
	fill := inner - 2 - lipgloss.Width(title)
	if fill < 0 {
		fill = 0
	}
	out := []string{borderStyle.Render("╭ ") + accentStyle.Render(title) +
		borderStyle.Render(" "+strings.Repeat("─", fill)+"╮")}

	for i, s := range shown {
		count := ""
		if s.count >= 0 {
			count = fmt.Sprintf("%d", s.count)
		}
		labelW := inner - 2 - sugCount
		body := " " + pad(trunc(s.label, labelW), labelW) + padRight(count, sugCount) + " "
		if i == cur {
			out = append(out, borderStyle.Render("│")+selRowStyle.Render(padRender(body, inner))+borderStyle.Render("│"))
			continue
		}
		style := subtleStyle
		if s.count == 0 {
			style = dimStyle // nothing would show; still listed so you know it exists
		}
		out = append(out, borderStyle.Render("│")+padRender(style.Render(body), inner)+borderStyle.Render("│"))
	}
	if more > 0 {
		out = append(out, borderStyle.Render("│")+
			padRender(dimStyle.Render(fmt.Sprintf("  +%d more — keep typing", more)), inner)+
			borderStyle.Render("│"))
	}
	return append(out, borderStyle.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

// suggestClickRows is the screen-y of the first clickable suggestion and how
// many there are, so a click can be mapped back to a row.
func (m Model) suggestClickRows() (top, n int) {
	box := m.suggestBox(m.tableW())
	if len(box) == 0 {
		return 0, 0
	}
	rows := len(box) - 2 // drop the two border lines
	if sug := m.suggestions(); len(sug) > sugMax {
		rows-- // the "+N more" line isn't selectable
	}
	// panel body starts at y=2; the bar takes line 0, the box border line 1
	return 2 + 2, rows
}
