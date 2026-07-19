package kicad

import (
	"archive/zip"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Point struct{ X, Y float64 }

type Segment struct {
	A, B  Point
	Width float64
}

type Pad struct {
	At    Point
	W, H  float64
	Round bool
}

type GerberLayer struct {
	File     string
	Function string
	Role     string
	Segments []Segment
	Pads     []Pad
	Regions  [][]Point
}

type Board struct {
	Layers   []GerberLayer
	Outline  []Segment
	Min, Max Point
}

func (b Board) Width() float64  { return b.Max.X - b.Min.X }
func (b Board) Height() float64 { return b.Max.Y - b.Min.Y }
func (b Board) Empty() bool     { return len(b.Layers) == 0 && len(b.Outline) == 0 }

type aperture struct {
	shape byte
	w, h  float64
}

type gstate struct {
	decX, decY int
	scale      float64
	aps        map[int]aperture
	cur        aperture
	x, y       float64
	interp     int
	region     bool
	contour    []Point
	layer      *GerberLayer
}

func GerberFiles(path string) (map[string][]byte, error) {
	full, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	switch {
	case info.IsDir():
		return readGerberDir(full)
	case strings.EqualFold(filepath.Ext(full), ".zip"):
		return readGerberZip(full)
	default:
		b, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		return map[string][]byte{filepath.Base(full): b}, nil
	}
}

func LoadGerbers(path string) (*Board, error) {
	files, err := GerberFiles(path)
	if err != nil {
		return nil, err
	}

	board := &Board{Min: Point{math.Inf(1), math.Inf(1)}, Max: Point{math.Inf(-1), math.Inf(-1)}}
	for name, data := range files {
		l := parseGerber(string(data), name)
		board.Layers = append(board.Layers, l)
		if l.Role == "outline" {
			board.Outline = append(board.Outline, l.Segments...)
		}
		accumulate(board, l)
	}
	if math.IsInf(board.Min.X, 1) {
		board.Min, board.Max = Point{}, Point{}
	}
	return board, nil
}

func readGerberDir(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".gbr") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[e.Name()] = b
	}
	return out, nil
}

func readGerberZip(path string) (map[string][]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := map[string][]byte{}
	for _, f := range r.File {
		if !strings.EqualFold(filepath.Ext(f.Name), ".gbr") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[filepath.Base(f.Name)] = b
	}
	return out, nil
}

func accumulate(b *Board, l GerberLayer) {
	touch := func(p Point) {
		b.Min.X = math.Min(b.Min.X, p.X)
		b.Min.Y = math.Min(b.Min.Y, p.Y)
		b.Max.X = math.Max(b.Max.X, p.X)
		b.Max.Y = math.Max(b.Max.Y, p.Y)
	}
	for _, s := range l.Segments {
		touch(s.A)
		touch(s.B)
	}
	for _, p := range l.Pads {
		touch(p.At)
	}
	for _, r := range l.Regions {
		for _, p := range r {
			touch(p)
		}
	}
}

func parseGerber(src, file string) GerberLayer {
	st := &gstate{decX: 6, decY: 6, scale: 1, aps: map[int]aperture{}, interp: 1,
		layer: &GerberLayer{File: file}}
	st.layer.Role = roleFromName(file)

	i := 0
	for i < len(src) {
		switch c := src[i]; {
		case c == '%':
			j := strings.IndexByte(src[i+1:], '%')
			if j < 0 {
				i = len(src)
				continue
			}
			block := src[i+1 : i+1+j]
			st.extended(strings.TrimSuffix(block, "*"))
			i += j + 2
		case c == '*' || c == '\n' || c == '\r' || c == ' ' || c == '\t':
			i++
		default:
			j := strings.IndexByte(src[i:], '*')
			if j < 0 {
				j = len(src) - i
			}
			st.word(src[i : i+j])
			i += j + 1
		}
	}
	return *st.layer
}

func (st *gstate) extended(b string) {
	switch {
	case strings.HasPrefix(b, "FS"):
		if k := strings.IndexByte(b, 'X'); k >= 0 && k+2 < len(b) {
			st.decX = int(b[k+2] - '0')
		}
		if k := strings.IndexByte(b, 'Y'); k >= 0 && k+2 < len(b) {
			st.decY = int(b[k+2] - '0')
		}
	case strings.HasPrefix(b, "MOIN"):
		st.scale = 25.4
	case strings.HasPrefix(b, "MOMM"):
		st.scale = 1
	case strings.HasPrefix(b, "ADD"):
		st.defineAperture(b[3:])
	case strings.HasPrefix(b, "TF.FileFunction,"):
		fn := b[len("TF.FileFunction,"):]
		st.layer.Function = fn
		if r := roleFromFunction(fn); r != "" {
			st.layer.Role = r
		}
	}
}

func (st *gstate) defineAperture(s string) {
	comma := strings.IndexByte(s, ',')
	head := s
	var params []float64
	if comma >= 0 {
		head = s[:comma]
		for _, p := range strings.Split(s[comma+1:], "X") {
			if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
				params = append(params, f*st.scale)
			}
		}
	}
	if len(head) < 2 {
		return
	}
	code, err := strconv.Atoi(head[:len(head)-1])
	if err != nil {
		code, err = strconv.Atoi(strings.TrimRight(head, "CROP"))
		if err != nil {
			return
		}
	}
	shape := head[len(head)-1]
	ap := aperture{shape: shape}
	switch shape {
	case 'C':
		if len(params) > 0 {
			ap.w, ap.h = params[0], params[0]
		}
	case 'R', 'O':
		if len(params) > 1 {
			ap.w, ap.h = params[0], params[1]
		}
	default:
		if len(params) > 0 {
			ap.w, ap.h = params[0], params[0]
		}
	}
	st.aps[code] = ap
}

func (st *gstate) coord(num string, dec int) float64 {
	if num == "" {
		return 0
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0
	}
	return float64(n) / math.Pow10(dec) * st.scale
}

func (st *gstate) word(w string) {
	if strings.HasPrefix(w, "G04") || w == "" {
		return
	}
	var gs []int
	dcode := -1
	var xv, yv, iv, jv float64
	var xset, yset, iset, jset bool

	i := 0
	for i < len(w) {
		c := w[i]
		if c < 'A' || c > 'Z' {
			i++
			continue
		}
		i++
		start := i
		if i < len(w) && (w[i] == '+' || w[i] == '-') {
			i++
		}
		for i < len(w) && w[i] >= '0' && w[i] <= '9' {
			i++
		}
		num := w[start:i]
		switch c {
		case 'G':
			if g, err := strconv.Atoi(num); err == nil {
				gs = append(gs, g)
			}
		case 'D':
			dcode, _ = strconv.Atoi(num)
		case 'X':
			xv, xset = st.coord(num, st.decX), true
		case 'Y':
			yv, yset = st.coord(num, st.decY), true
		case 'I':
			iv, iset = st.coord(num, st.decX), true
		case 'J':
			jv, jset = st.coord(num, st.decY), true
		}
	}

	for _, g := range gs {
		switch g {
		case 1, 2, 3:
			st.interp = g
		case 36:
			st.region, st.contour = true, nil
		case 37:
			if len(st.contour) >= 3 {
				st.layer.Regions = append(st.layer.Regions, st.contour)
			}
			st.region, st.contour = false, nil
		case 4:
			return
		}
	}

	if dcode >= 10 {
		st.cur = st.aps[dcode]
		return
	}

	nx, ny := st.x, st.y
	if xset {
		nx = xv
	}
	if yset {
		ny = yv
	}

	switch dcode {
	case 1:
		if st.region {
			if len(st.contour) == 0 {
				st.contour = append(st.contour, Point{st.x, st.y})
			}
			st.appendArcOrLine(&st.contour, nx, ny, iv, jv, iset || jset)
		} else if st.interp == 1 {
			st.layer.Segments = append(st.layer.Segments, Segment{Point{st.x, st.y}, Point{nx, ny}, st.cur.w})
		} else {
			st.emitArc(nx, ny, iv, jv)
		}
	case 3:
		st.layer.Pads = append(st.layer.Pads, Pad{At: Point{nx, ny}, W: st.cur.w, H: st.cur.h, Round: st.cur.shape == 'C' || st.cur.shape == 'O'})
	case 2:
		if st.region && len(st.contour) >= 3 {
			st.layer.Regions = append(st.layer.Regions, st.contour)
			st.contour = nil
		}
	}
	st.x, st.y = nx, ny
}

func arcPoints(sx, sy, ex, ey, cx, cy float64, ccw bool) []Point {
	r := math.Hypot(sx-cx, sy-cy)
	a0 := math.Atan2(sy-cy, sx-cx)
	a1 := math.Atan2(ey-cy, ex-cx)
	if ccw {
		if a1 <= a0 {
			a1 += 2 * math.Pi
		}
	} else {
		if a1 >= a0 {
			a1 -= 2 * math.Pi
		}
	}
	steps := int(math.Abs(a1-a0)/(math.Pi/24)) + 2
	pts := make([]Point, 0, steps)
	for k := 1; k <= steps; k++ {
		a := a0 + (a1-a0)*float64(k)/float64(steps)
		pts = append(pts, Point{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	return pts
}

func (st *gstate) emitArc(ex, ey, iv, jv float64) {
	cx, cy := st.x+iv, st.y+jv
	prev := Point{st.x, st.y}
	for _, p := range arcPoints(st.x, st.y, ex, ey, cx, cy, st.interp == 3) {
		st.layer.Segments = append(st.layer.Segments, Segment{prev, p, st.cur.w})
		prev = p
	}
}

func (st *gstate) appendArcOrLine(dst *[]Point, ex, ey, iv, jv float64, arc bool) {
	if arc && st.interp != 1 {
		cx, cy := st.x+iv, st.y+jv
		*dst = append(*dst, arcPoints(st.x, st.y, ex, ey, cx, cy, st.interp == 3)...)
		return
	}
	*dst = append(*dst, Point{ex, ey})
}

func roleFromFunction(fn string) string {
	f := strings.ToLower(fn)
	switch {
	case strings.HasPrefix(f, "profile"):
		return "outline"
	case strings.HasPrefix(f, "copper"):
		return "copper"
	case strings.HasPrefix(f, "soldermask"):
		return "mask"
	case strings.HasPrefix(f, "solderpaste"):
		return "paste"
	case strings.HasPrefix(f, "legend"):
		return "silk"
	}
	return ""
}

func roleFromName(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "edge_cuts") || strings.Contains(n, "edge") || strings.Contains(n, "profile"):
		return "outline"
	case strings.Contains(n, "paste"):
		return "paste"
	case strings.Contains(n, "silk") || strings.Contains(n, "legend"):
		return "silk"
	case strings.Contains(n, "mask"):
		return "mask"
	case strings.Contains(n, "_cu") || strings.Contains(n, "copper") || strings.Contains(n, "gnd") || strings.Contains(n, "pwr"):
		return "copper"
	}
	return "other"
}
