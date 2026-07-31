package kicad

import (
	"fmt"
	"math"
)

// minSpanRatio is how much smaller than its land a part may be. A part far smaller
// cannot bridge the pads: an 0402 capacitor on an 0603 land came out at 0.55 and a
// WLCSP-9 on a WQFN-14 land at 0.32. Measured over six boards, the smallest correct
// assignment was 0.82 — a SOT-23-5_HandSoldering land, whose pads are deliberately
// long — so this sits between the two with room on both sides.
const minSpanRatio = 0.7

// LandFit compares a part's own pads with the land it was assigned to.
func LandFit(land, part []Land) (bool, string) {
	if len(land) == 0 || len(part) == 0 {
		return true, ""
	}
	if ok, note := FitsLand(len(land), len(part)); !ok {
		return ok, note
	}
	ls, ps := Span(land), Span(part)
	if ls > 0 && ps > 0 && ps < minSpanRatio*ls {
		return false, fmt.Sprintf("part is %.1fmm across, the land is %.1fmm", ps, ls)
	}
	return true, ""
}

// FitsLand reports whether a part offering partPads can sit on a land offering
// boardPads. Only the direction that ruins a board counts: KiCad footprints carry
// thermal vias, paste-only pads and paired mounting pads no vendor land pattern
// does, so a land with pads to spare is routine. Measured over 198 assigned parts
// on six boards, this raised two real faults; testing for equality raised three
// false alarms on one board alone.
func FitsLand(boardPads, partPads int) (bool, string) {
	if boardPads <= 0 || partPads <= 0 || partPads <= boardPads {
		return true, ""
	}
	return false, fmt.Sprintf("part has %d pads, the land has %d", partPads, boardPads)
}

// Span is the diagonal across a footprint's pads. One number, so it survives a
// vendor publishing the part turned 90° — comparing widths and heights separately
// put correct assignments anywhere from 1.0 to 2.4 and told us nothing.
func Span(ls []Land) float64 {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, l := range ls {
		minX, maxX = math.Min(minX, l.X-l.W/2), math.Max(maxX, l.X+l.W/2)
		minY, maxY = math.Min(minY, l.Y-l.H/2), math.Max(maxY, l.Y+l.H/2)
	}
	if math.IsInf(minX, 1) {
		return 0
	}
	return math.Hypot(maxX-minX, maxY-minY)
}
