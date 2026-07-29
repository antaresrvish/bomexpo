package render

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bomexpo/internal/kicad"
)

const (
	cEmpty byte = iota
	cCopperInner
	cCopperBot
	cCopperTop
	cVia
	cOutline
	cBottom
	cTop
	cMatch
	cHighlight
)

var palette = map[byte]color.Color{
	cCopperInner: lipgloss.Color("#3a3626"),
	cCopperBot:   lipgloss.Color("#2f4f6f"),
	cCopperTop:   lipgloss.Color("#7a4a1e"),
	cVia:         lipgloss.Color("#caa63c"),
	cOutline:     lipgloss.Color("#a7c957"),
	cBottom:      lipgloss.Color("#b06de0"),
	cTop:         lipgloss.Color("#4cc9f0"),
	cMatch:       lipgloss.Color("#f6bd60"),
	cHighlight:   lipgloss.Color("#ff477e"),
}

// dimPalette is the same board in near-background tones. Used when the drawing
// sits behind text, where anything brighter would fight with it.
var dimPalette = map[byte]color.Color{
	cCopperInner: lipgloss.Color("#1c1c16"),
	cCopperBot:   lipgloss.Color("#182530"),
	cCopperTop:   lipgloss.Color("#302014"),
	cVia:         lipgloss.Color("#4a3c18"),
	cOutline:     lipgloss.Color("#3f5133"),
	cBottom:      lipgloss.Color("#3a2547"),
	cTop:         lipgloss.Color("#1e4557"),
	cMatch:       lipgloss.Color("#5a4620"),
	cHighlight:   lipgloss.Color("#5c2137"),
}

type Options struct {
	W, H       int
	ShowCopper bool
	// Highlight is drawn brightest: the component the cursor is on.
	Highlight map[string]bool
	// Match is drawn a step below Highlight, for everything a filter selected.
	// Nil means no filter, so nothing is singled out.
	Match map[string]bool
	// Dim draws everything in muted tones, for a backdrop behind text.
	Dim        bool
	Zoom       float64
	PanX, PanY float64
}

type canvas struct {
	w, h int
	px   []byte
}

func newCanvas(w, h int) *canvas { return &canvas{w: w, h: h, px: make([]byte, w*h)} }

func (c *canvas) set(x, y int, v byte) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := y*c.w + x
	if v >= c.px[i] {
		c.px[i] = v
	}
}

func (c *canvas) line(x0, y0, x1, y1 int, v byte) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	err := dx + dy
	for {
		c.set(x0, y0, v)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *canvas) rect(cx, cy, hw, hh int, v byte) {
	for y := cy - hh; y <= cy+hh; y++ {
		for x := cx - hw; x <= cx+hw; x++ {
			c.set(x, y, v)
		}
	}
}

func (c *canvas) rectOutline(cx, cy, hw, hh int, v byte) {
	for x := cx - hw; x <= cx+hw; x++ {
		c.set(x, cy-hh, v)
		c.set(x, cy+hh, v)
	}
	for y := cy - hh; y <= cy+hh; y++ {
		c.set(cx-hw, y, v)
		c.set(cx+hw, y, v)
	}
}

type transform struct {
	scale      float64 // x, and y too unless scaleY is set
	scaleY     float64
	cx, cy     float64
	ox, oy     float64
	panX, panY float64
}

func (t transform) sy() float64 {
	if t.scaleY > 0 {
		return t.scaleY
	}
	return t.scale
}

func (t transform) apply(p kicad.Point) (int, int) {
	x := (p.X-t.cx)*t.scale + t.ox + t.panX
	y := (p.Y-t.cy)*t.sy() + t.oy + t.panY
	return int(math.Round(x)), int(math.Round(y))
}

func Render(b *kicad.Board, placements []kicad.Placement, opt Options) string {
	if opt.W < 4 || opt.H < 2 {
		return ""
	}
	pw, ph := opt.W, opt.H*2
	cv := newCanvas(pw, ph)

	bw, bh := b.Width(), b.Height()
	if bw <= 0 || bh <= 0 {
		bw, bh = 1, 1
	}
	zoom := opt.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	// A framed drawing gets a margin so edge traces aren't clipped and keeps the
	// board's true proportions.
	scale := math.Min((float64(pw)-4)/bw, (float64(ph)-4)/bh) * zoom
	scaleY := 0.0
	if opt.Dim {
		// A backdrop fills the page instead, stretching to reach both edges —
		// but only so far. Past maxStretch a long thin board would read as
		// square, and its shape is one of the things you're checking.
		const maxStretch = 1.5
		sx, sy := float64(pw)/bw*zoom, float64(ph)/bh*zoom
		if sx > sy*maxStretch {
			sx = sy * maxStretch
		}
		if sy > sx*maxStretch {
			sy = sx * maxStretch
		}
		scale, scaleY = sx, sy
	}
	t := transform{
		scale:  scale,
		scaleY: scaleY,
		cx:     (b.Min.X + b.Max.X) / 2,
		cy:     (b.Min.Y + b.Max.Y) / 2,
		ox:     float64(pw) / 2,
		oy:     float64(ph) / 2,
		panX:   opt.PanX,
		panY:   opt.PanY,
	}

	if opt.ShowCopper {
		for _, l := range b.Layers {
			switch l.Role {
			case "copper":
				col := cCopperInner
				switch l.Function {
				case "Top":
					col = cCopperTop
				case "Bottom":
					col = cCopperBot
				}
				for _, s := range l.Segments {
					x0, y0 := t.apply(s.A)
					x1, y1 := t.apply(s.B)
					cv.line(x0, y0, x1, y1, col)
				}
			case "via":
				for _, p := range l.Pads {
					x, y := t.apply(p.At)
					cv.set(x, y, cVia)
				}
			}
		}
	}

	for _, s := range b.Outline {
		x0, y0 := t.apply(s.A)
		x1, y1 := t.apply(s.B)
		cv.line(x0, y0, x1, y1, cOutline)
	}

	for _, p := range placements {
		x, y := t.apply(kicad.Point{X: p.X, Y: p.Y})
		col := cTop
		if p.Layer == "bottom" {
			col = cBottom
		}
		if opt.Match[p.Designator] {
			col = cMatch
		}
		if opt.Highlight[p.Designator] {
			col = cHighlight
		}
		// a stretched backdrop stretches the parts with it
		hw, hh := int(p.BodyW/2*t.scale), int(p.BodyH/2*t.sy())
		if p.BodyW <= 0 {
			hw = int(t.scale * 0.3)
		}
		if p.BodyH <= 0 {
			hh = int(t.sy() * 0.3)
		}
		if hw >= 2 && hh >= 2 {
			cv.rectOutline(x, y, hw, hh, col)
		} else {
			cv.rect(x, y, max(hw, 0), max(hh, 0), col)
		}
	}

	if opt.Dim {
		return composeDim(cv, opt.W, opt.H)
	}
	return compose(cv, opt.W, opt.H)
}

// Cells draws the board as one styled string per character cell, so a caller can
// composite something on top of it. Empty cells come back as a plain space, and
// the result is nil when nothing was drawn at all.
func Cells(b *kicad.Board, placements []kicad.Placement, opt Options) [][]string {
	img := Render(b, placements, opt)
	if strings.TrimSpace(ansi.Strip(img)) == "" {
		return nil
	}
	rows := strings.Split(img, "\n")
	out := make([][]string, len(rows))
	for i, row := range rows {
		out[i] = splitCells(row, opt.W)
	}
	return out
}

// splitCells breaks a styled row into per-cell strings, keeping each cell's own
// escapes so it can be dropped anywhere.
func splitCells(row string, w int) []string {
	cells := make([]string, w)
	for x := 0; x < w; x++ {
		c := ansi.Cut(row, x, x+1)
		if ansi.Strip(c) == "" {
			c = " "
		}
		cells[x] = c
	}
	return cells
}

// composeDim renders the same geometry in muted tones, for use as a backdrop
// that text has to stay readable over.
func composeDim(cv *canvas, cols, rows int) string {
	var b strings.Builder
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			top := cv.px[(2*r)*cv.w+c]
			bot := cv.px[(2*r+1)*cv.w+c]
			if top == cEmpty && bot == cEmpty {
				b.WriteByte(' ')
				continue
			}
			st := lipgloss.NewStyle()
			if top != cEmpty {
				st = st.Foreground(dimPalette[top])
			}
			if bot != cEmpty {
				st = st.Background(dimPalette[bot])
			}
			b.WriteString(st.Render("▀"))
		}
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func compose(cv *canvas, cols, rows int) string {
	var b strings.Builder
	for r := 0; r < rows; r++ {
		runStart := 0
		var curTop, curBot byte
		flush := func(end int) {
			if end <= runStart {
				return
			}
			n := end - runStart
			if curTop == cEmpty && curBot == cEmpty {
				b.WriteString(strings.Repeat(" ", n))
				return
			}
			st := lipgloss.NewStyle()
			if curTop != cEmpty {
				st = st.Foreground(palette[curTop])
			}
			if curBot != cEmpty {
				st = st.Background(palette[curBot])
			}
			b.WriteString(st.Render(strings.Repeat("▀", n)))
		}
		for c := 0; c < cols; c++ {
			top := cv.px[(2*r)*cv.w+c]
			bot := cv.px[(2*r+1)*cv.w+c]
			if c == 0 {
				curTop, curBot = top, bot
				continue
			}
			if top != curTop || bot != curBot {
				flush(c)
				runStart = c
				curTop, curBot = top, bot
			}
		}
		flush(cols)
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return 0
	}
}
