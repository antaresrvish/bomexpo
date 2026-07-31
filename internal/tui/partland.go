package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/render"
)

// selCode is the part code under the cursor, empty when there isn't one.
func (m Model) selCode() string {
	i := m.sel()
	if i < 0 || i >= len(m.items) {
		return ""
	}
	return m.items[i].LCSC
}

// askPadsCmd fetches a part's own pads, or returns nil when there is nothing to ask:
// no code, the pads are in hand, a request is already out, or we have asked as often
// as we will. In-flight is tracked separately from the attempt count, or two messages
// arriving while one request is out would spend the whole budget on it.
func (m Model) askPadsCmd(code string) tea.Cmd {
	if code == "" || m.edaFetching[code] || m.edaTried[code] >= maxFitAttempts {
		return nil
	}
	if m.waitingOnVendor() {
		return nil
	}
	if _, have := m.edaLands[code]; have {
		return nil
	}
	if m.edaTried != nil {
		m.edaTried[code]++
	}
	if m.edaFetching != nil {
		m.edaFetching[code] = true
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
		// only say fetching while something actually is
		if m.waitingOnVendor() {
			return []string{name, dimStyle.Render("vendor is turning us away — waiting it out")}
		}
		if m.edaTried[code] >= maxFitAttempts {
			return []string{name, dimStyle.Render("could not reach the vendor for its pads")}
		}
		return []string{name, dimStyle.Render("fetching pads…")}
	}
	if len(fp.Lands) == 0 {
		return []string{name, dimStyle.Render("no pads published for this part")}
	}
	if ok, note := m.landFit(i); !ok {
		return []string{name, badStyle.Render(trunc(note, w))}
	}
	sum := render.FootprintSummary(fp.Lands)
	if a := kicad.PadAlign(m.landsFor(i), fp.Lands); a != 0 {
		sum += fmt.Sprintf(" · turned %.0f° to match", math.Mod(a+360, 360))
	}
	return []string{name, dimStyle.Render(trunc(sum, w))}
}

// waitingOnVendor reports whether the client is sitting out a rate limit.
func (m Model) waitingOnVendor() bool {
	return !m.padWait.IsZero() && time.Now().Before(m.padWait)
}

// partFootprint draws the part turned into the land's frame, so the two drawings can
// be compared by eye. Vendors publish in their own orientation.
func (m Model) partFootprint(w, h int) []string {
	i := m.sel()
	if i < 0 || h < 1 {
		return nil
	}
	fp, have := m.edaLands[m.items[i].LCSC]
	if !have || len(fp.Lands) == 0 {
		return nil
	}
	img := render.Footprint(fp.Lands, render.FootprintOptions{
		W: w, H: h, Rotate: m.rotOf(i) + kicad.PadAlign(m.landsFor(i), fp.Lands),
	})
	if img == "" {
		return []string{dimStyle.Render("too small to draw")}
	}
	return strings.Split(img, "\n")
}
