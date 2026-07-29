package render

import (
	"fmt"
	"math"

	"bomexpo/internal/kicad"
)

// A whole board squeezed into a sidebar is a smudge. One footprint at the same
// size is legible, and it answers the question that actually comes up while
// assigning parts: which way does this thing face, and how many pads does it
// have?

// maxSubpixelPerMM keeps the drawing to scale rather than stretched to fit.
const maxSubpixelPerMM = 14

type FootprintOptions struct {
	W, H int
	// Rotate is the angle the part is placed at, snapped to a quarter turn, so
	// the drawing matches how it sits on the board.
	Rotate float64
	// Highlight names the net whose pads to pick out, or "" for none.
	Highlight string
}

// Footprint draws a footprint's pads. Pad 1 is drawn in its own colour so the
// orientation is readable at a glance.
func Footprint(lands []kicad.Land, opt FootprintOptions) string {
	if len(lands) == 0 || opt.W < 6 || opt.H < 3 {
		return ""
	}

	quarter := int(math.Round(opt.Rotate/90)) % 4
	if quarter < 0 {
		quarter += 4
	}
	turned := make([]kicad.Land, len(lands))
	for i, l := range lands {
		turned[i] = turnLand(l, quarter)
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, l := range turned {
		minX, maxX = math.Min(minX, l.X-l.W/2), math.Max(maxX, l.X+l.W/2)
		minY, maxY = math.Min(minY, l.Y-l.H/2), math.Max(maxY, l.Y+l.H/2)
	}
	fw, fh := maxX-minX, maxY-minY
	if fw <= 0 {
		fw = 1
	}
	if fh <= 0 {
		fh = 1
	}

	pw, ph := opt.W, opt.H*2 // half-block rows give two subpixels of height
	// A subpixel is about square (a cell is twice as tall as wide, and a row
	// holds two), so one scale serves both axes.
	scale := math.Min(float64(pw-2)/fw, float64(ph-2)/fh)
	// Cap it, or a 0402 gets blown up into two slabs that fill the panel. With
	// a ceiling, small parts draw small and you can tell a chip from a connector
	// just by moving the cursor.
	if scale > maxSubpixelPerMM {
		scale = maxSubpixelPerMM
	}
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	at := func(x, y float64) (int, int) {
		return int(math.Round(float64(pw)/2 + (x-cx)*scale)),
			int(math.Round(float64(ph)/2 + (y-cy)*scale))
	}

	cv := newCanvas(pw, ph)
	for _, l := range turned {
		col := cTop
		switch {
		case l.First:
			col = cHighlight
		case opt.Highlight != "" && l.Net == opt.Highlight:
			col = cMatch
		case l.Hole:
			col = cVia
		}
		x, y := at(l.X, l.Y)
		hw := int(math.Round(l.W / 2 * scale))
		hh := int(math.Round(l.H / 2 * scale))
		cv.rect(x, y, max(hw, 0), max(hh, 0), col)
	}
	return compose(cv, opt.W, opt.H)
}

// turnLand rotates a pad about the footprint origin by whole quarter turns,
// which is all the CPL ever asks for.
func turnLand(l kicad.Land, quarter int) kicad.Land {
	for i := 0; i < quarter; i++ {
		l.X, l.Y = -l.Y, l.X
		l.W, l.H = l.H, l.W
	}
	return l
}

// FootprintSummary is a one-line caption for the drawing: how many pads, how
// big the land pattern is, and how many pads are drilled. The size matters
// because the drawing is capped in scale, not measured off the screen.
func FootprintSummary(lands []kicad.Land) string {
	if len(lands) == 0 {
		return ""
	}
	holes := 0
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, l := range lands {
		if l.Hole {
			holes++
		}
		minX, maxX = math.Min(minX, l.X-l.W/2), math.Max(maxX, l.X+l.W/2)
		minY, maxY = math.Min(minY, l.Y-l.H/2), math.Max(maxY, l.Y+l.H/2)
	}

	out := fmt.Sprintf("%d pads", len(lands))
	if len(lands) == 1 {
		out = "1 pad"
	}
	out += fmt.Sprintf(" · %.1f×%.1fmm", maxX-minX, maxY-minY)
	if holes > 0 {
		out += fmt.Sprintf(" · %d drilled", holes)
	}
	return out
}
