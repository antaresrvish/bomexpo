package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/export"
	"bomexpo/internal/kicad"
	"bomexpo/internal/value"
)

type checkState struct {
	out textfield
	top int
}

func newCheckState() checkState {
	return checkState{out: newField("› ", "output .zip path", 56)}
}

func (cs *checkState) setDefault(pcbPath string) {
	if cs.out.Value() != "" || pcbPath == "" {
		return
	}
	dir := filepath.Dir(pcbPath)
	name := strings.TrimSuffix(filepath.Base(pcbPath), ".kicad_pcb")
	cs.out.SetValue(filepath.Join(dir, name+"-order.zip"))
}

type issue struct {
	idx   int
	ref   string
	kind  itemState
	label string
}

func (m Model) issues() []issue {
	var out []issue
	for i := range m.items {
		st := m.stateOf(i)
		if st == stOK || st == stExcluded {
			continue
		}
		it := m.items[i]
		label := ""
		switch st {
		case stUnassigned:
			label = "no LCSC part assigned"
		case stOutOfStock:
			label = "assigned part is out of stock"
		case stMismatch:
			label = value.Check(it.Value, m.assigned[i].Description()).Note
		}
		out = append(out, issue{idx: i, ref: it.ID(), kind: st, label: label})
	}
	return out
}

const checkDataTop = 5 // tab(1)+border(1)+summary,blank,"issues to review"(3)

func (m Model) mouseCheck(ms tea.Mouse, click, wheel bool) (tea.Model, tea.Cmd) {
	issues := m.issues()
	if wheel {
		if ms.Button == tea.MouseWheelUp {
			m.check.top = max(0, m.check.top-1)
		} else if ms.Button == tea.MouseWheelDown {
			m.check.top = clampInt(m.check.top+1, 0, max(0, len(issues)-1))
		}
		return m, nil
	}
	if click && ms.Button == tea.MouseLeft && len(issues) > 0 {
		row := m.check.top + (ms.Y - checkDataTop)
		if row >= 0 && row < len(issues) {
			m.check.out.Blur()
			m.mode = modeTable
			m.cursor = issues[row].idx
			m.clampScroll()
		}
	}
	return m, nil
}

func (m Model) updateCheck(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.check.out.Blur()
		m.mode = modeTable
		return m, nil
	case "tab":
		m.check.out.Blur()
		return m.cycleTab(1)
	case "shift+tab":
		m.check.out.Blur()
		return m.cycleTab(-1)
	case "up":
		m.check.top = max(0, m.check.top-1)
		return m, nil
	case "down":
		m.check.top++
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.check.out.Value())
		if path == "" {
			m.err = "output path is empty"
			return m, nil
		}
		m.err = ""
		m.loading = true
		m.status = "Exporting (generating Gerbers)…"
		return m, m.exportCmd(path)
	}
	m.check.out.Update(msg)
	return m, nil
}

func (m Model) costAt(boards int) (float64, bool) {
	total, complete := 0.0, true
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		p := m.assigned[i]
		if p == nil {
			complete = false
			continue
		}
		qty := it.Quantity * boards
		if u, ok := p.PriceAt(qty); ok {
			total += u * float64(qty)
		} else {
			complete = false
		}
	}
	return total, complete
}

func (m Model) viewCheck(w, h int) string {
	assigned, warn := m.counts()
	issues := m.issues()

	summary := okStyle.Render(fmt.Sprintf("%d/%d assigned", assigned, m.activeCount())) + "   " +
		warnStyle.Render(fmt.Sprintf("%d warnings", warn)) + "   " +
		subtleStyle.Render(fmt.Sprintf("%d line items", len(m.items)))

	lines := []string{summary, ""}
	if len(issues) == 0 {
		lines = append(lines, okStyle.Render("✓ every line item is assigned, in stock, and value-matched"))
	} else {
		lines = append(lines, subtleStyle.Render("issues to review"))
		vis := h - 24
		if vis < 1 {
			vis = 1
		}
		m.check.top = clampInt(m.check.top, 0, max(0, len(issues)-1))
		end := min(len(issues), m.check.top+vis)
		for _, is := range issues[m.check.top:end] {
			icon, _, _ := stateDecor(is.kind)
			lines = append(lines, fmt.Sprintf("%s %s  %s", icon, accentStyle.Render(pad(is.ref, 10)), colorIssue(is.kind, is.label)))
		}
		if end < len(issues) {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  … %d more (↓)", len(issues)-end)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.preflightAndManifest(w)...)

	lines = append(lines, "", accentStyle.Render("Volume pricing")+dimStyle.Render("  order cost at LCSC quantity breaks"))
	lines = append(lines, colHeadStyle.Render(pad("  BOARDS", 12)+pad("ORDER COST", 16)+pad("PER BOARD", 14)))
	for _, n := range []int{1, 100, 200, 300, 400, 500} {
		tot, complete := m.costAt(n)
		mark := ""
		if !complete {
			mark = dimStyle.Render("*")
		}
		lines = append(lines, "  "+pad(fmt.Sprintf("%d", n), 10)+
			okStyle.Render(pad(fmt.Sprintf("$%.2f", tot), 14))+mark+"  "+
			subtleStyle.Render(fmt.Sprintf("$%.4f", tot/float64(n))))
	}

	if fixes := export.RotationFixes(m.placements, m.excludeSet(), m.rotOverrideMap()); len(fixes) > 0 {
		manual := 0
		for _, f := range fixes {
			if f.Manual {
				manual++
			}
		}
		hdr := fmt.Sprintf("  %d parts realigned in the CPL", len(fixes))
		if manual > 0 {
			hdr += fmt.Sprintf(" · %d manual override", manual)
		}
		lines = append(lines, "", accentStyle.Render("JLCPCB rotation")+dimStyle.Render(hdr))
		norm := func(d float64) float64 {
			for d < 0 {
				d += 360
			}
			return d
		}
		order := []string{}
		count := map[string]int{}
		for _, f := range fixes {
			key := fmt.Sprintf("%s +%g°", rotFamily(f.Footprint), norm(f.To-f.From))
			if _, ok := count[key]; !ok {
				order = append(order, key)
			}
			count[key]++
		}
		var parts []string
		for i, k := range order {
			if i == 6 {
				parts = append(parts, fmt.Sprintf("+%d more", len(order)-6))
				break
			}
			parts = append(parts, fmt.Sprintf("%s ×%d", k, count[k]))
		}
		lines = append(lines, dimStyle.Render("  "+strings.Join(parts, "   ")))
	}

	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	lines = append(lines,
		labelStyle.Render("Output  ")+m.check.out.View(),
		dimStyle.Render("enter → order-ready zip (BOM + CPL + Gerbers) · * = some parts unassigned"))
	return strings.Join(lines, "\n")
}

func rotFamily(fp string) string {
	if i := strings.IndexAny(fp, "_ "); i > 0 {
		return fp[:i]
	}
	return fp
}

// preflightAndManifest renders the pre-flight checklist and the order-package
// manifest side by side, so the Check page uses its width.
func (m Model) preflightAndManifest(w int) []string {
	var un, oos, mm int
	for i := range m.items {
		switch m.stateOf(i) {
		case stUnassigned:
			un++
		case stOutOfStock:
			oos++
		case stMismatch:
			mm++
		}
	}
	active := m.activeCount()
	hasBoard := m.board != nil && !m.board.Empty()
	cli := kicadCLI() != ""

	chk := func(ok bool, pass, fail string) string {
		if ok {
			return okStyle.Render("✓ ") + subtleStyle.Render(pass)
		}
		return badStyle.Render("✗ ") + warnStyle.Render(fail)
	}
	checklist := []string{
		accentStyle.Render("Pre-flight"),
		chk(active > 0 && un == 0, fmt.Sprintf("all %d line items assigned", active), fmt.Sprintf("%d line items need an LCSC part", un)),
		chk(oos == 0, "all assigned parts in stock", fmt.Sprintf("%d parts out of stock", oos)),
		chk(mm == 0, "values match the schematic", fmt.Sprintf("%d value mismatches", mm)),
		chk(hasBoard, "board outline "+boardSize(m.boardW, m.boardH), "no board outline"),
		chk(cli, "kicad-cli ready for gerbers", "kicad-cli not found — gerbers skipped"),
	}

	excl := m.excludeSet()
	placed := 0
	for _, p := range m.placements {
		if !excl[p.Designator] {
			placed++
		}
	}
	gerbers := badStyle.Render("needs kicad-cli")
	if cli {
		gerbers = subtleStyle.Render("top · bottom · drill")
	}
	man := func(k, v string) string { return dimStyle.Render(pad(k, 14)) + v }
	manifest := []string{
		accentStyle.Render("Order package"),
		man("bom.csv", subtleStyle.Render(fmt.Sprintf("%d line items", active))),
		man("positions.csv", subtleStyle.Render(fmt.Sprintf("%d placed · %d excluded", placed, len(m.placements)-placed))),
		man("gerbers", gerbers),
		man("components", subtleStyle.Render(fmt.Sprintf("%d total", len(m.placements)))),
	}
	return twoCol(checklist, manifest, w/2)
}

func twoCol(left, right []string, leftW int) []string {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = padRender(l, leftW) + "  " + r
	}
	return out
}

func colorIssue(st itemState, label string) string {
	switch st {
	case stUnassigned:
		return dimStyle.Render(label)
	case stOutOfStock:
		return badStyle.Render(label)
	case stMismatch:
		return warnStyle.Render(label)
	}
	return label
}

func exportZip(path string, items []kicad.Item, placements []kicad.Placement, pcbPath string, exclude map[string]bool, rotOverride map[string]int) error {
	return export.WriteOrderZip(path, items, placements, pcbPath, exclude, rotOverride)
}
