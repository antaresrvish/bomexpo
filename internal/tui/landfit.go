package tui

import (
	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
)

// The land comes from the board file and the part's pads from EasyEDA, so nothing
// compared them: a 4-resistor array sold as "27R" passes the value check, and all
// three files carry the same code, so verify calls it agreement.

// landFit compares a line item's land with its part's pads, and reports true
// whenever there is nothing to say — including when either side is unknown.
func (m Model) landFit(i int) (bool, string) {
	if i < 0 || i >= len(m.items) || m.items[i].LCSC == "" {
		return true, ""
	}
	fp, have := m.edaLands[m.items[i].LCSC]
	if !have {
		return true, ""
	}
	return kicad.LandFit(m.landsFor(i), fp.Lands)
}

// maxFitAttempts bounds the asking: EasyEDA has no record of some parts at all —
// three of 38 on one board — and retrying those leaves the check reading "still
// checking" for good.
const maxFitAttempts = 2

// fitCmds fetches the vendor's land pattern for each assigned part. Unlike landsCmd
// it does not stop at a part the board already has geometry for — that geometry is
// the thing being questioned.
func (m Model) fitCmds() []tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		code := m.items[i].LCSC
		if code == "" || m.edaTried[code] >= maxFitAttempts {
			continue
		}
		if _, have := m.edaLands[code]; have {
			continue
		}
		if m.edaTried != nil {
			m.edaTried[code]++
		}
		cmds = append(cmds, m.footprintCmd(code))
	}
	return cmds
}

// fitTally keeps a fault, a comparison in flight, and a part there will never be
// geometry for apart — folding the last two together read as a check that never ends.
type fitTally struct {
	Bad       int
	Pending   int
	Checked   int
	Unchecked int // no vendor geometry, or asked as often as we will
}

func (m Model) fitCount() (bad, pending, checked int) {
	t := m.fitTally()
	return t.Bad, t.Pending, t.Checked
}

func (m Model) fitTally() fitTally {
	var t fitTally
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		code := m.items[i].LCSC
		if code == "" || len(m.landsFor(i)) == 0 {
			continue
		}
		fp, have := m.edaLands[code]
		switch {
		case have && len(fp.Lands) > 0:
			t.Checked++
			if ok, _ := m.landFit(i); !ok {
				t.Bad++
			}
		case have, m.edaTried[code] >= maxFitAttempts:
			t.Unchecked++
		default:
			t.Pending++
		}
	}
	return t
}
