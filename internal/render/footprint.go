package render

import (
	"fmt"
	"math"

	"bomexpo/internal/kicad"
)

// maxSubpixelPerMM keeps the drawing to scale rather than stretched to fit.
const maxSubpixelPerMM = 14

type FootprintOptions struct {
	W, H      int
	Rotate    float64
	Highlight string
}

// Footprint draws the pads, with pad 1 in its own colour for orientation.
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
	// a subpixel is about square, so one scale serves both axes
	scale := math.Min(float64(pw-2)/fw, float64(ph-2)/fh)
	// capped, or an 0402 fills the panel as two slabs
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
		x0, y0 := at(l.X-l.W/2, l.Y-l.H/2)
		x1, y1 := at(l.X+l.W/2, l.Y+l.H/2)
		cv.box(x0, y0, x1, y1, col)
	}
	return compose(cv, opt.W, opt.H)
}

// turnLand does whole quarter turns, all the CPL asks for.
func turnLand(l kicad.Land, quarter int) kicad.Land {
	for i := 0; i < quarter; i++ {
		l.X, l.Y = -l.Y, l.X
		l.W, l.H = l.H, l.W
	}
	return l
}

// FootprintSummary captions the drawing. Size matters because the drawing is
// capped in scale, not measured off the screen.
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
