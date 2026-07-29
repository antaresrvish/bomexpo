package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
)

// netDataTop is the first net row: tab(1)+border(1)+title,colhead,rule(3).
const netDataTop = 5

// netState is the net picker: a list of the board's nets, busiest first, that
// sets a net: filter on the table.
type netState struct {
	open   bool
	cursor int
	top    int
	field  textfield
}

func newNetState() netState {
	return netState{field: newField("", "type to narrow the net list…", 40)}
}

// matching is the net list narrowed by whatever is typed.
func (m Model) netsMatching() []kicad.Net {
	q := strings.ToLower(strings.TrimSpace(m.nets.field.Value()))
	if q == "" {
		return m.designNets
	}
	var out []kicad.Net
	for _, n := range m.designNets {
		if strings.Contains(strings.ToLower(n.Name), q) {
			out = append(out, n)
		}
	}
	return out
}

func (m Model) netRows() int {
	n := m.contentH() - 4 // title, column header, rule, hint
	if n < 1 {
		n = 1
	}
	return n
}

func (m Model) openNetPicker() (tea.Model, tea.Cmd) {
	if len(m.designNets) == 0 {
		if m.fromBoard() {
			m.flash = "this board has no nets on any pad"
		} else {
			m.flash = "no nets — a bom csv carries no connectivity"
		}
		return m, nil
	}
	m.nets.open = true
	m.nets.cursor, m.nets.top = 0, 0
	m.nets.field.SetValue("")
	m.nets.field.Focus()
	m.mode = modeNets
	return m, nil
}

func (m Model) closeNetPicker() (tea.Model, tea.Cmd) {
	m.nets.open = false
	m.nets.field.Blur()
	m.mode = modeTable
	return m, nil
}

// pickNet replaces any existing net: term with the chosen one, leaving the rest
// of the query alone.
func (m Model) pickNet(name string) (tea.Model, tea.Cmd) {
	var keep []string
	for _, tok := range strings.Fields(m.filter.field.Value()) {
		if k, _, ok := strings.Cut(strings.TrimPrefix(tok, "-"), ":"); ok && strings.EqualFold(k, "net") {
			continue
		}
		keep = append(keep, tok)
	}
	keep = append(keep, "net:"+name)
	q := strings.Join(keep, " ")

	m.filter.field.SetValue(q)
	m.filter.f = parseFilter(q)
	m.cursor, m.top = 0, 0
	m.flash = "filtering on " + name
	mm, _ := m.closeNetPicker()
	return mm.(Model).reindex(), nil
}

func (m Model) updateNetKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	nets := m.netsMatching()
	switch msg.String() {
	case "esc":
		return m.closeNetPicker()
	case "enter":
		if m.nets.cursor >= 0 && m.nets.cursor < len(nets) {
			return m.pickNet(nets[m.nets.cursor].Name)
		}
		return m, nil
	case "down", "ctrl+n":
		m.nets.cursor = min(len(nets)-1, m.nets.cursor+1)
		m.clampNets()
		return m, nil
	case "up", "ctrl+p":
		m.nets.cursor = max(0, m.nets.cursor-1)
		m.clampNets()
		return m, nil
	case "pgdown":
		m.nets.cursor = min(len(nets)-1, m.nets.cursor+m.netRows())
		m.clampNets()
		return m, nil
	case "pgup":
		m.nets.cursor = max(0, m.nets.cursor-m.netRows())
		m.clampNets()
		return m, nil
	}
	before := m.nets.field.Value()
	m.nets.field.Update(msg)
	if m.nets.field.Value() != before {
		m.nets.cursor, m.nets.top = 0, 0
	}
	return m, nil
}

func (m *Model) clampNets() {
	n := len(m.netsMatching())
	if n == 0 {
		m.nets.cursor, m.nets.top = 0, 0
		return
	}
	vis := m.netRows()
	m.nets.cursor = clampInt(m.nets.cursor, 0, n-1)
	if m.nets.cursor < m.nets.top {
		m.nets.top = m.nets.cursor
	}
	if m.nets.cursor >= m.nets.top+vis {
		m.nets.top = m.nets.cursor - vis + 1
	}
	m.nets.top = clampInt(m.nets.top, 0, max(0, n-1))
}

func (m Model) mouseNets(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	nets := m.netsMatching()
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			m.nets.cursor = max(0, m.nets.cursor-1)
		} else if ms.Button == tea.MouseWheelDown {
			m.nets.cursor = min(len(nets)-1, m.nets.cursor+1)
		}
		m.clampNets()
		return m, nil
	}
	if !click || ms.Button != tea.MouseLeft {
		return m, nil
	}
	row := m.nets.top + (ms.Y - netDataTop)
	if row < 0 || row >= len(nets) {
		return m, nil
	}
	if row == m.nets.cursor {
		return m.pickNet(nets[row].Name)
	}
	m.nets.cursor = row
	m.clampNets()
	return m, nil
}

func (m Model) viewNets(w, h int) string {
	nets := m.netsMatching()

	title := subtleStyle.Render("pick a net to filter the table  ") + m.nets.field.View()
	const nameW, countW = 30, 8
	refsW := w - nameW - countW - 2 - 3*2
	if refsW < 10 {
		refsW = 10
	}
	head := colHeadStyle.Render(padRender(strings.Join([]string{
		pad("NET", nameW), pad("PARTS", countW), pad("COMPONENTS", refsW),
	}, " | "), w))

	lines := []string{title, head, borderStyle.Render(strings.Repeat("─", w))}
	vis := m.netRows()
	for i := m.nets.top; i < min(len(nets), m.nets.top+vis); i++ {
		n := nets[i]
		plain := []string{
			pad(trunc(n.Name, nameW), nameW),
			pad(fmt.Sprintf("%d", len(n.Refs)), countW),
			pad(trunc(strings.Join(n.Refs, " "), refsW), refsW),
		}
		if i == m.nets.cursor {
			lines = append(lines, selRowStyle.Render(padRender("▶ "+strings.Join(plain, "   "), w)))
			continue
		}
		lines = append(lines, padRender("  "+strings.Join([]string{
			accentStyle.Render(plain[0]), okStyle.Render(plain[1]), dimStyle.Render(plain[2]),
		}, sepStyle.Render(" | ")), w))
	}
	if len(nets) == 0 {
		lines = append(lines, dimStyle.Render("  no net matches that"))
	}

	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" +
		dimStyle.Render(fmt.Sprintf("  %d nets · enter filter · esc back", len(m.designNets)))
}
