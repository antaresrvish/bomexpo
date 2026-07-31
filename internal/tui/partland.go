package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/render"
)

// selPartLandsCmd fetches the highlighted part's own pads. Moving through the table
// asks for one code at a time, which the 24h disk cache absorbs; Export asks for
// all of them at once, for the pre-flight.
func (m Model) selPartLandsCmd() tea.Cmd {
	i := m.sel()
	if i < 0 || i >= len(m.items) {
		return nil
	}
	code := m.items[i].LCSC
	if code == "" || m.edaTried[code] >= maxFitAttempts {
		return nil
	}
	if _, have := m.edaLands[code]; have {
		return nil
	}
	if m.edaTried != nil {
		m.edaTried[code]++
	}
	return m.footprintCmd(code)
}

// partFootprintHeader labels the lower drawing with the part's own package name, and
// says outright when its pads outnumber the land's.
func (m Model) partFootprintHeader(w int) []string {
	i := m.sel()
	if i < 0 {
		return []string{accentStyle.Render("Part"), ""}
	}
	code := m.items[i].LCSC
	if code == "" {
		return []string{accentStyle.Render("Part") + " " + dimStyle.Render("none assigned"), ""}
	}
	fp, have := m.edaLands[code]
	name := accentStyle.Render("Part") + " " + codeStyle.Render(code)
	if have && fp.Package != "" {
		name += " " + subtleStyle.Render(trunc(fp.Package, max(w-13-len(code), 8)))
	}
	if !have {
		return []string{name, dimStyle.Render("fetching pads…")}
	}
	if len(fp.Lands) == 0 {
		return []string{name, dimStyle.Render("no pads published for this part")}
	}
	sum := render.FootprintSummary(fp.Lands)
	if ok, note := m.landFit(i); !ok {
		return []string{name, badStyle.Render(trunc(note, w))}
	}
	return []string{name, dimStyle.Render(sum)}
}

// partFootprint draws the part's pads unrotated: this is the part as the vendor
// publishes it, not as the board places it.
func (m Model) partFootprint(w, h int) []string {
	i := m.sel()
	if i < 0 || h < 1 {
		return nil
	}
	fp, have := m.edaLands[m.items[i].LCSC]
	if !have || len(fp.Lands) == 0 {
		return nil
	}
	img := render.Footprint(fp.Lands, render.FootprintOptions{W: w, H: h})
	if img == "" {
		return []string{dimStyle.Render("too small to draw")}
	}
	return strings.Split(img, "\n")
}
