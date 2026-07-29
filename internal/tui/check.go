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

// setDefault seeds the output path from whatever file the design was opened
// from. It used to only know how to strip ".kicad_pcb", which left a CSV design
// with an empty path and an export that failed on "output path is empty".
func (cs *checkState) setDefault(srcPath string) {
	if cs.out.Value() != "" || srcPath == "" {
		return
	}
	base := filepath.Base(srcPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	cs.out.SetValue(filepath.Join(filepath.Dir(srcPath), name+"-order.zip"))
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
			// issues carry line-item indices; the table cursor is a display row
			if r := m.rowOf(issues[row].idx); r >= 0 {
				m.cursor = r
			}
			m.clampScroll()
		}
	}
	return m, nil
}

func (m Model) updateCheck(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While the path is being edited it owns the keyboard; the rest of the time
	// the page does, so the board and the issue list are actually reachable.
	if m.check.out.Focused() {
		switch msg.String() {
		case "esc":
			m.check.out.Blur()
			return m, nil
		case "enter":
			m.check.out.Blur()
			return m.startExport()
		}
		m.check.out.Update(msg)
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.mode = modeTable
		return m, nil
	case "tab":
		return m.cycleTab(1)
	case "shift+tab":
		return m.cycleTab(-1)
	case "e":
		// edit the output path; esc or enter hands the keyboard back
		m.check.out.Focus()
		return m, nil
	case "enter":
		return m.startExport()
	case "up", "k":
		m.check.top = max(0, m.check.top-1)
	case "down", "j":
		m.check.top++
	case "t":
		return m.openRender("top")
	case "b":
		return m.openRender("bottom")
	case "i":
		return m.openRender("iso")
	case "+", "=":
		m.boardv = m.boardv.zoomBy(zoomStep)
	case "-", "_":
		m.boardv = m.boardv.zoomBy(1 / zoomStep)
	case "0":
		m.boardv = m.boardv.resetView()
	case "left", "h":
		m.boardv = m.boardv.panBy(-1, 0)
	case "right", "l":
		m.boardv = m.boardv.panBy(1, 0)
	case "shift+up":
		m.boardv = m.boardv.panBy(0, -1)
	case "shift+down":
		m.boardv = m.boardv.panBy(0, 1)
	}
	return m, nil
}

func (m Model) startExport() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.check.out.Value())
	if path == "" {
		m.err = "output path is empty — press e to type one"
		return m, nil
	}
	m.err = ""
	m.loading = true
	m.status = "Exporting (generating Gerbers)…"
	return m, m.exportCmd(path)
}

// costAt totals the order for a given board count, buying at least each part's
// LCSC minimum order quantity.
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
		order := it.Quantity * boards
		if p.MinBuy > order {
			order = p.MinBuy
		}
		if u, ok := p.PriceAt(order); ok {
			total += u * float64(order)
		} else {
			complete = false
		}
	}
	return total, complete
}

// moqImpact reports how many parts must be over-bought to hit their LCSC
// minimum order at the given board count, and the extra spend it costs.
func (m Model) moqImpact(boards int) (parts int, extra float64) {
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		p := m.assigned[i]
		if p == nil {
			continue
		}
		need := it.Quantity * boards
		if p.MinBuy > need {
			if u, ok := p.PriceAt(p.MinBuy); ok {
				extra += u * float64(p.MinBuy-need)
				parts++
			}
		}
	}
	return
}

func (m Model) viewCheck(w, h int) string {
	// The page is split down the middle, so everything on the left is laid out
	// to half the width.
	const footer = 2
	leftW := w / 2
	rightW := w - leftW - 1
	paneH := h - footer

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
	lines = append(lines, m.preflightAndManifest(leftW)...)

	pricing := []string{
		accentStyle.Render("Volume pricing") + dimStyle.Render("  cost at supplier breaks"),
		colHeadStyle.Render(pad("  BOARDS", 10) + pad("ORDER COST", 14) + pad("PER BOARD", 12)),
	}
	for _, n := range []int{1, 100, 200, 300, 400, 500} {
		tot, complete := m.costAt(n)
		mark := ""
		if !complete {
			mark = dimStyle.Render("*")
		}
		pricing = append(pricing, "  "+pad(fmt.Sprintf("%d", n), 8)+
			okStyle.Render(pad(fmt.Sprintf("$%.2f", tot), 12))+mark+"  "+
			subtleStyle.Render(fmt.Sprintf("$%.4f", tot/float64(n))))
	}
	if n, extra := m.moqImpact(1); n > 0 {
		pricing = append(pricing, dimStyle.Render(fmt.Sprintf("  %d parts hit a supplier minimum — +$%.2f at 1 board", n, extra)))
	}

	lines = append(lines, "")
	lines = append(lines, pricing...)

	if b, pref, ext, known := m.libBreakdown(); known > 0 {
		lines = append(lines, "", accentStyle.Render("Assembly library")+
			dimStyle.Render("  extended parts pay a per-part setup fee"))
		lines = append(lines, "  "+strings.Join([]string{
			okStyle.Render(fmt.Sprintf("basic %d", b)),
			accentStyle.Render(fmt.Sprintf("preferred %d", pref)),
			hotStyle(ext, warnStyle).Render(fmt.Sprintf("extended %d", ext)),
		}, sepStyle.Render(" · ")))
		if ext > 0 {
			// Deliberately no dollar figure: JLCPCB's rate changes and a stale
			// number here would be worse than none.
			fee := "setup fees are"
			if ext == 1 {
				fee = "setup fee is"
			}
			lines = append(lines, dimStyle.Render(fmt.Sprintf(
				"  %d %s not in the totals above — check JLCPCB's current rate", ext, fee)))
		}
		if rest := m.activeCount() - known; rest > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf(
				"  %d parts have no library data — re-search them on jlcpcb (^o in search) to fill it in", rest)))
		}
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

	// The numbers on the left, the board on the right at the full height it can
	// get. The output field spans the bottom because it acts on both.
	body := sideBySide(lines, m.boardPane(rightW, paneH), leftW, w, paneH)
	label, hint := dimStyle.Render("  Output  "), "e edit · enter export · * = some parts unassigned"
	if m.check.out.Focused() {
		label, hint = accentStyle.Render("▸ Output  "), "enter export · esc done"
	}
	body = append(body,
		label+m.check.out.View(),
		dimStyle.Render("  "+hint))
	return strings.Join(body, "\n")
}

// sideBySide lays two columns out to a fixed height, separated by a single space.
func sideBySide(left, right []string, leftW, w, h int) []string {
	out := make([]string, h)
	for i := 0; i < h; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = padRender(l, leftW) + " " + padRender(r, w-leftW-1)
	}
	return out
}

// boardPane is the right-hand column of the Check page: the whole board, as big
// as the page allows.
func (m Model) boardPane(w, h int) []string {
	head := m.boardHeader()
	if m.boardW > 0 {
		head += dimStyle.Render(boardSize(m.boardW, m.boardH))
	}
	if m.boardv.zoom > zoomMin {
		head += warnStyle.Render(fmt.Sprintf("  %.1f×", m.boardv.zoom))
	}

	out := []string{head, m.boardCaption()}
	drawH := h - len(out)
	if drawH < 2 {
		return out
	}
	return append(out, m.miniBoard(w, drawH)...)
}

// boardCaption says what's being drawn and how to move it.
func (m Model) boardCaption() string {
	if n := len(m.placements); n > 0 {
		return dimStyle.Render(fmt.Sprintf("%d placed · +- zoom · ←→ pan · 0 reset", n))
	}
	return dimStyle.Render("+- zoom · ←→ pan · 0 reset")
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
	// A CSV design isn't failing when it has no board — it never had one.
	na := func(s string) string { return dimStyle.Render("– " + s) }

	checklist := []string{
		accentStyle.Render("Pre-flight"),
		chk(active > 0 && un == 0, fmt.Sprintf("all %d line items assigned", active), fmt.Sprintf("%d line items need an LCSC part", un)),
		chk(oos == 0, "all assigned parts in stock", fmt.Sprintf("%d parts out of stock", oos)),
		chk(mm == 0, "values match the schematic", fmt.Sprintf("%d value mismatches", mm)),
	}
	if m.fromBoard() {
		checklist = append(checklist,
			chk(hasBoard, "board outline "+boardSize(m.boardW, m.boardH), "no board outline"),
			chk(cli, "kicad-cli ready for gerbers", "kicad-cli not found — gerbers skipped"),
		)
	} else {
		placements := "no placements — pass a cpl csv for positions.csv"
		if len(m.placements) > 0 {
			placements = fmt.Sprintf("%d placements from %s", len(m.placements), filepath.Base(m.cplPath))
		}
		checklist = append(checklist,
			na("no board — bom csv, so no gerbers"),
			chk(len(m.placements) > 0, placements, placements),
		)
	}

	excl := m.excludeSet()
	placed := 0
	for _, p := range m.placements {
		if !excl[p.Designator] {
			placed++
		}
	}
	gerbers := badStyle.Render("needs kicad-cli")
	switch {
	case !m.fromBoard():
		gerbers = dimStyle.Render("n/a — no board")
	case cli:
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
	// Side by side needs room for both; in a narrow column they'd both be
	// truncated, so stack them instead.
	if w < 90 {
		return append(append(checklist, ""), manifest...)
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
