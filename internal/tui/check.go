package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/export"
	"bomexpo/internal/kicad"
	"bomexpo/internal/value"
)

// checkPane says which pane has the keyboard. The issue list and the board both
// want the up and down arrows.
type checkPane int

const (
	paneIssues checkPane = iota
	paneBoard
	paneOut
)

type checkState struct {
	out textfield
	top int
	// cur is the highlighted issue. The list is a list, so it gets a cursor and an
	// enter like every other one here.
	cur int
	// target is how many boards you actually plan to order, 0 until you say. The
	// pricing table marks it, since that is the number you're deciding on.
	target  int
	qty     textfield
	confirm confirmState
	pane    checkPane
}

func newCheckState() checkState {
	return checkState{
		out: newField("› ", "output .zip path", 56),
		qty: newField("boards › ", "how many boards?", 10),
	}
}

// setPane keeps the output field's own focus in step, so there's one answer to
// what has the keys.
func (cs *checkState) setPane(p checkPane) {
	cs.pane = p
	if p == paneOut {
		cs.out.Focus()
		return
	}
	cs.out.Blur()
}

// cyclePane walks issues → board → output path.
func (cs *checkState) cyclePane(dir int) {
	cs.setPane(checkPane((int(cs.pane) + dir + 3) % 3))
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
			m.check.setPane(paneIssues)
			m.check.cur = row
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
	key := msg.String()

	if m.check.confirm.open {
		return m.updateConfirmKey(msg)
	}

	// The board count owns the keyboard while it's being typed, and only takes
	// digits — anything else here would be a typo, not a quantity.
	if m.check.qty.Focused() {
		switch key {
		case "esc":
			m.check.qty.Blur()
			return m, nil
		case "enter", "tab":
			m.check.qty.Blur()
			if n, err := strconv.Atoi(strings.TrimSpace(m.check.qty.Value())); err == nil && n > 0 {
				m.check.target = n
				m.flash = fmt.Sprintf("pricing for %d boards", n)
			} else if strings.TrimSpace(m.check.qty.Value()) == "" {
				m.check.target = 0
			}
			return m, nil
		}
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			m.check.qty.Update(msg)
			return m, nil
		}
		switch key {
		case "backspace", "ctrl+u", "ctrl+w", "left", "right", "ctrl+a", "ctrl+e":
			m.check.qty.Update(msg)
		}
		return m, nil
	}

	// While the path is being edited it owns the keyboard.
	if m.check.pane == paneOut {
		switch key {
		case "esc":
			m.check.setPane(paneIssues)
			return m, nil
		case "tab":
			m.check.cyclePane(1)
			return m, nil
		case "shift+tab":
			m.check.cyclePane(-1)
			return m, nil
		case "enter":
			m.check.setPane(paneIssues)
			return m.requestExport()
		}
		m.check.out.Update(msg)
		return m, nil
	}

	if mm, cmd, done := m.tabSwitchKey(key); done {
		return mm, cmd
	}
	switch key {
	case "esc":
		m.mode = modeTable
		return m, nil
	case "tab":
		m.check.cyclePane(1)
		return m, nil
	case "shift+tab":
		m.check.cyclePane(-1)
		return m, nil
	case "e", "/":
		m.check.setPane(paneOut)
		return m, nil
	case "v":
		// verify before you commit, or after an order came back wrong
		return m.openVerify()
	case "q":
		m.check.qty.SetValue("")
		m.check.qty.Focus()
		return m, nil
	case "enter":
		// enter acts on the highlighted issue; x writes the zip, since the list is
		// what the arrows are driving.
		if is, ok := m.selectedIssue(); ok && m.check.pane == paneIssues {
			return m.jumpToComponent(is.ref)
		}
		return m.requestExport()
	case "x":
		return m.requestExport()

	// up and down belong to whichever pane has the keyboard
	case "up", "k":
		if m.check.pane == paneBoard {
			m.boardv = m.boardv.panBy(0, -1)
			return m, nil
		}
		m.check.cur--
		m.clampIssues()
	case "down", "j":
		if m.check.pane == paneBoard {
			m.boardv = m.boardv.panBy(0, 1)
			return m, nil
		}
		m.check.cur++
		m.clampIssues()
	case "g", "home":
		m.check.cur = 0
		m.clampIssues()
	case "G", "end":
		m.check.cur = len(m.issues()) - 1
		m.clampIssues()

	case "t":
		return m.openRender("top")
	case "b":
		return m.openRender("bottom")
	case "i":
		return m.openRender("iso")

	// the issue list has no horizontal axis and no zoom, so these need no ring
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

// selectedIssue is the highlighted issue, if there is one.
func (m Model) selectedIssue() (issue, bool) {
	is := m.issues()
	if m.check.cur < 0 || m.check.cur >= len(is) {
		return issue{}, false
	}
	return is[m.check.cur], true
}

// clampIssues keeps the cursor on a real issue and scrolls to hold it on screen.
func (m *Model) clampIssues() {
	n := len(m.issues())
	if n == 0 {
		m.check.cur, m.check.top = 0, 0
		return
	}
	m.check.cur = clampInt(m.check.cur, 0, n-1)
	vis := m.issueRows()
	if m.check.cur < m.check.top {
		m.check.top = m.check.cur
	}
	if m.check.cur >= m.check.top+vis {
		m.check.top = m.check.cur - vis + 1
	}
	m.check.top = clampInt(m.check.top, 0, max(0, n-1))
}

// issueRows is how many issues the left column shows at once.
func (m Model) issueRows() int {
	n := m.contentH() - 24
	if n < 1 {
		n = 1
	}
	return n
}

// rotationList shows what the CPL will say versus what the board says, one part per
// row. It used to be a single line of "SOT-23 +180° ×4" tallies, which told you a
// count but never which part — and a rotation you did not expect is exactly the thing
// you want to look up by designator.
func (m Model) rotationList(fixes []export.RotationFix, w int) []string {
	manual := 0
	for _, f := range fixes {
		if f.Manual {
			manual++
		}
	}
	head := accentStyle.Render("JLCPCB rotation") +
		dimStyle.Render(fmt.Sprintf("  %d parts turned for the pick-and-place", len(fixes)))
	if manual > 0 {
		head += dimStyle.Render(fmt.Sprintf(" · %d yours", manual))
	}

	const refW, fpW = 9, 22
	out := []string{head, colHeadStyle.Render(padRender(
		"  "+pad("REF", refW)+pad("FOOTPRINT", fpW)+pad("BOARD", 8)+pad("CPL", 8)+"WHY", w))}

	vis := m.rotationRows()
	for i, f := range fixes {
		if i >= vis {
			out = append(out, dimStyle.Render(fmt.Sprintf("  … %d more", len(fixes)-vis)))
			break
		}
		why, st := "library rule", dimStyle
		if f.Manual {
			why, st = "your override", warnStyle
		}
		out = append(out, padRender("  "+
			accentStyle.Render(pad(f.Designator, refW))+
			subtleStyle.Render(pad(trunc(shortFP(f.Footprint), fpW-1), fpW))+
			dimStyle.Render(pad(fmt.Sprintf("%g°", normDeg(f.From)), 8))+
			okStyle.Render(pad(fmt.Sprintf("%g°", normDeg(f.To)), 8))+
			st.Render(why), w))
	}
	return out
}

// shortFP drops the library prefix, the way the table elsewhere shows footprints.
func shortFP(fp string) string {
	if i := strings.LastIndex(fp, ":"); i >= 0 {
		return fp[i+1:]
	}
	return fp
}

// rotationRows keeps the list from crowding out the pricing beneath it.
func (m Model) rotationRows() int {
	n := (m.contentH() - 26) / 2
	return clampInt(n, 3, 10)
}

// normDeg brings an angle into 0–359, since a CPL never wants a negative one.
func normDeg(d float64) float64 {
	for d < 0 {
		d += 360
	}
	for d >= 360 {
		d -= 360
	}
	return d
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
		lines = append(lines, focusMark(m.check.pane == paneIssues)+
			okStyle.Render("✓ every line item is assigned, in stock, and value-matched"))
	} else {
		lines = append(lines, focusMark(m.check.pane == paneIssues)+
			subtleStyle.Render("issues to review"))
		vis := m.issueRows()
		m.check.top = clampInt(m.check.top, 0, max(0, len(issues)-1))
		end := min(len(issues), m.check.top+vis)
		for i := m.check.top; i < end; i++ {
			is := issues[i]
			icon, _, _ := stateDecor(is.kind)
			if i == m.check.cur && m.check.pane == paneIssues {
				lines = append(lines, selRowStyle.Render(padRender(
					"▶ "+pad(is.ref, 10)+"  "+is.label, leftW)))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s  %s",
				icon, accentStyle.Render(pad(is.ref, 10)), colorIssue(is.kind, is.label)))
		}
		if end < len(issues) {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  … %d more (↓)", len(issues)-end)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.preflightAndManifest(leftW)...)

	rows := m.pricingRows(m.check.target, 7)
	head := dimStyle.Render("  where the price per board actually changes")
	if m.check.target > 0 {
		head = dimStyle.Render("  ordering ") + accentStyle.Render(fmt.Sprintf("%d", m.check.target)) +
			dimStyle.Render(" boards · q changes it")
	}
	pricing := []string{
		accentStyle.Render("Volume pricing") + head,
		colHeadStyle.Render(pad("  BOARDS", 9) + pad("ORDER COST", 13) + pad("PER BOARD", 12) + "WHAT CHANGES"),
	}
	for _, r := range rows {
		mark := " "
		if !r.Complete {
			mark = dimStyle.Render("*")
		}
		boards := pad(fmt.Sprintf("%d", r.Boards), 7)
		perBoard := pad(fmt.Sprintf("$%.4f", r.PerBoard), 12)
		why := dimStyle.Render(trunc(r.Why, max(leftW-34, 4)))
		line := "  " + boards + okStyle.Render(pad(fmt.Sprintf("$%.2f", r.Total), 11)) + mark + " " +
			subtleStyle.Render(perBoard) + why
		if r.Why == "your order" {
			// the highlighted cell needs a gap of its own, or the background runs
			// straight into the label beside it
			line = "▸ " + boards + okStyle.Render(pad(fmt.Sprintf("$%.2f", r.Total), 11)) + mark + " " +
				cursorStyle.Render(pad(fmt.Sprintf("$%.4f", r.PerBoard), 11)) + " " +
				accentStyle.Render("your order")
		}
		pricing = append(pricing, line)
	}
	if len(rows) == 0 {
		pricing = append(pricing, dimStyle.Render("  assign some parts and the breaks appear"))
	}
	if n, extra := m.moqImpact(max(m.check.target, 1)); n > 0 {
		pricing = append(pricing, dimStyle.Render(fmt.Sprintf(
			"  %d parts over-bought to reach a supplier minimum — +$%.2f", n, extra)))
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
		lines = append(lines, "")
		lines = append(lines, m.rotationList(fixes, leftW)...)
	}

	// The numbers on the left, the board on the right at the full height it can
	// get. The output field spans the bottom because it acts on both.
	body := sideBySide(lines, m.boardPane(rightW, paneH), leftW, w, paneH)
	if m.check.qty.Focused() {
		body = append(body, focusMark(true)+m.check.qty.View())
	}
	label := focusMark(m.check.pane == paneOut) + dimStyle.Render("Output  ")
	hint := "enter opens the issue · x export · v verify against a bom · tab moves pane"
	if m.check.pane == paneOut {
		label = focusMark(true) + accentStyle.Render("Output  ")
		hint = "enter export · tab moves on · esc back to the issues"
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
	// both text rows carry the mark's width so they stay aligned; the drawing keeps
	// the full width, being a centred canvas
	mark := focusMark(m.check.pane == paneBoard)
	head := mark + m.boardHeader()
	if m.boardW > 0 {
		head += dimStyle.Render(boardSize(m.boardW, m.boardH))
	}
	if m.boardv.zoom > zoomMin {
		head += warnStyle.Render(fmt.Sprintf("  %.1f×", m.boardv.zoom))
	}

	out := []string{head, focusMark(false) + m.boardCaption()}
	drawH := h - len(out)
	if drawH < 2 {
		return out
	}
	return append(out, m.miniBoard(w, drawH)...)
}

// boardCaption names the keys, which differ by whether the board has the arrows.
func (m Model) boardCaption() string {
	keys := "+- zoom · ←→ pan · 0 reset · tab to pan ↑↓"
	if m.check.pane == paneBoard {
		keys = "+- zoom · ←→↑↓ pan · 0 reset"
	}
	if n := len(m.placements); n > 0 {
		return dimStyle.Render(fmt.Sprintf("%d placed · %s", n, keys))
	}
	return dimStyle.Render(keys)
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
