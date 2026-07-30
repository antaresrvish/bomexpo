package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Exporting with issues open is sometimes right — parts you'll hand-solder or source
// yourself — so this asks rather than refuses, and lists what is wrong.

type confirmState struct {
	open bool
	yes  bool
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

func (m Model) confirmContent(w int) []string {
	issues := m.issues()
	var un, oos, mism, fit int
	for _, is := range issues {
		switch is.kind {
		case stUnassigned:
			un++
		case stOutOfStock:
			oos++
		case stFootprint:
			fit++
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
	add(fit, "whose part has more pads than the land it sits on", badStyle)
	add(mism, "whose value does not match the schematic", warnStyle)

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

// confirmButtons marks the chosen answer as well as colouring it: a terminal not
// showing the colour would leave the answer ambiguous.
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

// viewConfirm floats the question over Export, so what you are agreeing to stays visible.
func (m Model) viewConfirm(w, h int) string {
	bg := strings.Split(m.viewCheck(w, h), "\n")
	body := m.confirmContent(popupW(w) - 5)
	x, y, pw, ph := popupBox(w, h, len(body)+4)
	box := popupFrame("Export with issues open?", body, pw, ph)
	return strings.Join(overlay(bg, box, x, y, w), "\n")
}
