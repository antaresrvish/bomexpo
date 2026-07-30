package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Exporting with issues open is sometimes right — a board you know you'll hand-solder
// two parts on, a part you'll source yourself. So this asks rather than refuses, and
// it lists what is wrong, because a confirmation that only says "are you sure" makes
// people stop reading.

type confirmState struct {
	open bool
	// yes is where the cursor sits. It starts on no: the safe answer is the default
	// when the thing being confirmed is an order.
	yes bool
}

// requestExport exports, or asks first when the board has issues.
func (m Model) requestExport() (tea.Model, tea.Cmd) {
	if len(m.issues()) == 0 {
		return m.startExport()
	}
	if strings.TrimSpace(m.check.out.Value()) == "" {
		m.err = "output path is empty — press e to type one"
		return m, nil
	}
	m.check.confirm = confirmState{open: true}
	return m, nil
}

func (m Model) updateConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n", "N", "q":
		m.check.confirm.open = false
		return m, nil
	case "y", "Y":
		m.check.confirm.open = false
		return m.startExport()
	case "left", "h", "right", "l", "tab", "shift+tab":
		m.check.confirm.yes = !m.check.confirm.yes
		return m, nil
	case "enter", " ":
		yes := m.check.confirm.yes
		m.check.confirm.open = false
		if yes {
			return m.startExport()
		}
		m.flash = "export cancelled"
		return m, nil
	}
	return m, nil
}

// confirmContent is the body of the popup: what is wrong, then the two answers.
func (m Model) confirmContent(w int) []string {
	issues := m.issues()
	var un, oos, mism int
	for _, is := range issues {
		switch is.kind {
		case stUnassigned:
			un++
		case stOutOfStock:
			oos++
		case stMismatch:
			mism++
		}
	}

	out := []string{
		warnStyle.Render(fmt.Sprintf("%d line items are not ready to order:", len(issues))),
		"",
	}
	add := func(n int, what string, st lipgloss.Style) {
		if n > 0 {
			out = append(out, "  "+st.Render(fmt.Sprintf("%d", n))+" "+subtleStyle.Render(what))
		}
	}
	add(un, "with no part assigned — they will be missing from the bom", badStyle)
	add(oos, "out of stock — the assembler cannot buy them", badStyle)
	add(mism, "whose value does not match the schematic", warnStyle)

	// Name a few, so the answer is informed rather than a shrug.
	out = append(out, "")
	for i, is := range issues {
		if i >= 4 {
			out = append(out, dimStyle.Render(fmt.Sprintf("  … and %d more", len(issues)-4)))
			break
		}
		out = append(out, "  "+accentStyle.Render(pad(is.ref, 10))+dimStyle.Render(trunc(is.label, max(w-14, 8))))
	}

	out = append(out, "", dimStyle.Render("The zip will still be written, without them."), "")
	out = append(out, m.confirmButtons(w))
	return out
}

// confirmButtons draws the two answers with the chosen one filled in and marked. The
// mark matters: colour alone leaves the answer ambiguous on a terminal that isn't
// showing it.
func (m Model) confirmButtons(w int) string {
	if m.check.confirm.yes {
		return centre(dimStyle.Render("  no, go back  ")+"   "+
			badgeBad.Render("▸ yes, export"), w)
	}
	return centre(badgeOk.Render("▸ no, go back")+"   "+
		dimStyle.Render("  yes, export  "), w)
}

func centre(s string, w int) string {
	pad := (w - lipgloss.Width(s)) / 2
	if pad < 0 {
		pad = 0
	}
	return spaces(pad) + s
}

// viewConfirm floats the question over the Export page, so what you are agreeing to
// is still on screen behind it.
func (m Model) viewConfirm(w, h int) string {
	bg := strings.Split(m.viewCheck(w, h), "\n")
	body := m.confirmContent(popupW(w) - 5)
	x, y, pw, ph := popupBox(w, h, len(body)+4)
	box := popupFrame("Export with issues open?", body, pw, ph)
	return strings.Join(overlay(bg, box, x, y, w), "\n")
}
