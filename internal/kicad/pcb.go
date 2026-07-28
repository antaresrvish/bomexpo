package kicad

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Component struct {
	Ref            string
	Value          string
	Footprint      string
	X, Y, Rot      float64
	Layer          string
	LCSC           string
	DNP            bool
	ExcludeBOM     bool
	RotOverride    int
	HasRotOverride bool
	BodyW, BodyH   float64
}

type Project struct {
	Name       string
	PCBPath    string
	Components []Component
	Outline    []Segment
	tracks     map[string][]Segment
	vias       []Point
	Layers     int
	BoardW     float64
	BoardH     float64
	Min, Max   Point
}

func LoadProject(path string) (*Project, error) {
	pcb, err := resolvePCB(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(pcb)
	if err != nil {
		return nil, err
	}
	root, err := parseSexp(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse pcb: %w", err)
	}

	p := &Project{
		Name:    strings.TrimSuffix(filepath.Base(pcb), ".kicad_pcb"),
		PCBPath: pcb,
		tracks:  map[string][]Segment{},
		Min:     Point{math.Inf(1), math.Inf(1)},
		Max:     Point{math.Inf(-1), math.Inf(-1)},
	}
	for _, n := range root.kids {
		switch n.head() {
		case "footprint", "module":
			if c, ok := parseFootprint(n); ok {
				p.Components = append(p.Components, c)
			}
		case "gr_line", "gr_arc", "gr_rect", "gr_circle":
			if onEdge(n) {
				p.Outline = append(p.Outline, graphicSegments(n)...)
			}
		case "segment":
			layer := atom(child(n, "layer"), 1)
			st, en := child(n, "start"), child(n, "end")
			if st != nil && en != nil {
				p.tracks[layer] = append(p.tracks[layer], Segment{
					A: Point{num(atom(st, 1)), num(atom(st, 2))},
					B: Point{num(atom(en, 1)), num(atom(en, 2))},
				})
			}
		case "via":
			if at := child(n, "at"); at != nil {
				p.vias = append(p.vias, Point{num(atom(at, 1)), num(atom(at, 2))})
			}
		case "layers":
			for _, k := range n.kids {
				if strings.HasSuffix(atom(k, 1), ".Cu") {
					p.Layers++
				}
			}
		}
	}
	if len(p.Components) == 0 {
		return nil, fmt.Errorf("no components found in %s", filepath.Base(pcb))
	}

	for _, s := range p.Outline {
		p.touch(s.A)
		p.touch(s.B)
	}
	for _, c := range p.Components {
		p.touch(Point{c.X, c.Y})
	}
	for _, segs := range p.tracks {
		for _, s := range segs {
			p.touch(s.A)
			p.touch(s.B)
		}
	}
	if len(p.Outline) > 0 {
		ox0, oy0 := math.Inf(1), math.Inf(1)
		ox1, oy1 := math.Inf(-1), math.Inf(-1)
		for _, s := range p.Outline {
			for _, pt := range []Point{s.A, s.B} {
				ox0, oy0 = math.Min(ox0, pt.X), math.Min(oy0, pt.Y)
				ox1, oy1 = math.Max(ox1, pt.X), math.Max(oy1, pt.Y)
			}
		}
		p.BoardW, p.BoardH = ox1-ox0, oy1-oy0
	}
	if math.IsInf(p.Min.X, 1) {
		p.Min, p.Max = Point{}, Point{}
	}
	sort.SliceStable(p.Components, func(i, j int) bool {
		return refLess(p.Components[i].Ref, p.Components[j].Ref)
	})
	return p, nil
}

func (p *Project) touch(pt Point) {
	p.Min.X, p.Min.Y = math.Min(p.Min.X, pt.X), math.Min(p.Min.Y, pt.Y)
	p.Max.X, p.Max.Y = math.Max(p.Max.X, pt.X), math.Max(p.Max.Y, pt.Y)
}

func resolvePCB(path string) (string, error) {
	full, err := ExpandPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		matches, _ := filepath.Glob(filepath.Join(full, "*.kicad_pcb"))
		if len(matches) == 0 {
			return "", fmt.Errorf("no .kicad_pcb in %s", filepath.Base(full))
		}
		return matches[0], nil
	}
	switch filepath.Ext(full) {
	case ".kicad_pcb":
		return full, nil
	case ".kicad_pro", ".kicad_sch":
		cand := strings.TrimSuffix(full, filepath.Ext(full)) + ".kicad_pcb"
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
		return "", fmt.Errorf("no matching .kicad_pcb next to %s", filepath.Base(full))
	default:
		return full, nil
	}
}

func parseFootprint(n *node) (Component, bool) {
	c := Component{Layer: "top", Rot: 0}
	if len(n.kids) > 1 {
		c.Footprint = shortFootprint(n.kids[1].atom)
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, k := range n.kids {
		switch k.head() {
		case "layer":
			if strings.HasPrefix(atom(k, 1), "B.") {
				c.Layer = "bottom"
			}
		case "attr":
			for _, a := range k.kids[1:] {
				switch a.atom {
				case "dnp":
					c.DNP = true
				case "exclude_from_bom":
					c.ExcludeBOM = true
				}
			}
		case "at":
			c.X = num(atom(k, 1))
			c.Y = num(atom(k, 2))
			c.Rot = num(atom(k, 3))
		case "property":
			name := strings.ToLower(atom(k, 1))
			val := atom(k, 2)
			switch {
			case name == "reference":
				c.Ref = val
			case name == "value":
				c.Value = val
			case strings.Contains(name, "lcsc") || strings.Contains(name, "jlc"):
				if val != "" {
					c.LCSC = val
				}
			case name == "bomexpo_rot":
				if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
					c.RotOverride = n
					c.HasRotOverride = true
				}
			}
		case "pad":
			at, sz := child(k, "at"), child(k, "size")
			px, py := num(atom(at, 1)), num(atom(at, 2))
			sw, sh := num(atom(sz, 1)), num(atom(sz, 2))
			minX, maxX = math.Min(minX, px-sw/2), math.Max(maxX, px+sw/2)
			minY, maxY = math.Min(minY, py-sh/2), math.Max(maxY, py+sh/2)
		}
	}
	if c.Ref == "" {
		return c, false
	}
	if !math.IsInf(minX, 1) {
		c.BodyW, c.BodyH = maxX-minX, maxY-minY
	}
	return c, true
}

func shortFootprint(lib string) string {
	if i := strings.LastIndex(lib, ":"); i >= 0 {
		return lib[i+1:]
	}
	return lib
}

func onEdge(n *node) bool {
	for _, k := range n.kids {
		if k.head() == "layer" && atom(k, 1) == "Edge.Cuts" {
			return true
		}
	}
	return false
}

func graphicSegments(n *node) []Segment {
	pt := func(head string) (Point, bool) {
		if c := child(n, head); c != nil {
			return Point{num(atom(c, 1)), num(atom(c, 2))}, true
		}
		return Point{}, false
	}
	switch n.head() {
	case "gr_line":
		a, ok1 := pt("start")
		b, ok2 := pt("end")
		if ok1 && ok2 {
			return []Segment{{A: a, B: b}}
		}
	case "gr_rect":
		a, ok1 := pt("start")
		b, ok2 := pt("end")
		if ok1 && ok2 {
			c1, c2 := Point{b.X, a.Y}, Point{a.X, b.Y}
			return []Segment{{A: a, B: c1}, {A: c1, B: b}, {A: b, B: c2}, {A: c2, B: a}}
		}
	case "gr_arc":
		a, ok1 := pt("start")
		mid, ok2 := pt("mid")
		b, ok3 := pt("end")
		if ok1 && ok2 && ok3 {
			return arc3(a, mid, b)
		}
	case "gr_circle":
		c, ok1 := pt("center")
		e, ok2 := pt("end")
		if ok1 && ok2 {
			r := math.Hypot(e.X-c.X, e.Y-c.Y)
			return circleSegments(c, r)
		}
	}
	return nil
}

func arc3(a, mid, b Point) []Segment {
	cx, cy, ok := circumcenter(a, mid, b)
	if !ok {
		return []Segment{{A: a, B: mid}, {A: mid, B: b}}
	}
	r := math.Hypot(a.X-cx, a.Y-cy)
	a0 := math.Atan2(a.Y-cy, a.X-cx)
	am := math.Atan2(mid.Y-cy, mid.X-cx)
	a1 := math.Atan2(b.Y-cy, b.X-cx)
	ccw := angNorm(am-a0) > 0 && angNorm(a1-a0) >= angNorm(am-a0) || (angNorm(a1-a0) > 0 && angNorm(am-a0) <= angNorm(a1-a0))
	if !ccw {
		if a1 >= a0 {
			a1 -= 2 * math.Pi
		}
	} else {
		if a1 <= a0 {
			a1 += 2 * math.Pi
		}
	}
	steps := int(math.Abs(a1-a0)/(math.Pi/24)) + 2
	var segs []Segment
	prev := a
	for k := 1; k <= steps; k++ {
		t := a0 + (a1-a0)*float64(k)/float64(steps)
		cur := Point{cx + r*math.Cos(t), cy + r*math.Sin(t)}
		segs = append(segs, Segment{A: prev, B: cur})
		prev = cur
	}
	return segs
}

func angNorm(a float64) float64 {
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

func circumcenter(a, b, c Point) (float64, float64, bool) {
	d := 2 * (a.X*(b.Y-c.Y) + b.X*(c.Y-a.Y) + c.X*(a.Y-b.Y))
	if math.Abs(d) < 1e-9 {
		return 0, 0, false
	}
	ux := ((a.X*a.X+a.Y*a.Y)*(b.Y-c.Y) + (b.X*b.X+b.Y*b.Y)*(c.Y-a.Y) + (c.X*c.X+c.Y*c.Y)*(a.Y-b.Y)) / d
	uy := ((a.X*a.X+a.Y*a.Y)*(c.X-b.X) + (b.X*b.X+b.Y*b.Y)*(a.X-c.X) + (c.X*c.X+c.Y*c.Y)*(b.X-a.X)) / d
	return ux, uy, true
}

func circleSegments(c Point, r float64) []Segment {
	const n = 48
	var segs []Segment
	prev := Point{c.X + r, c.Y}
	for i := 1; i <= n; i++ {
		t := 2 * math.Pi * float64(i) / n
		cur := Point{c.X + r*math.Cos(t), c.Y + r*math.Sin(t)}
		segs = append(segs, Segment{A: prev, B: cur})
		prev = cur
	}
	return segs
}

func (p *Project) BOM() []Item {
	type key struct {
		v, f string
		dnp  bool
	}
	order := []key{}
	groups := map[key]*Item{}
	for _, c := range p.Components {
		k := key{c.Value, c.Footprint, c.DNP}
		it, ok := groups[k]
		if !ok {
			it = &Item{Value: c.Value, Footprint: c.Footprint, DNP: c.DNP, ExcludeBOM: c.ExcludeBOM}
			groups[k] = it
			order = append(order, k)
		} else {
			it.ExcludeBOM = it.ExcludeBOM && c.ExcludeBOM
		}
		it.Bases = append(it.Bases, c.Ref)
		it.Designators = append(it.Designators, c.Ref)
		it.Quantity++
		if it.LCSC == "" && c.LCSC != "" {
			it.LCSC = c.LCSC
		}
		if !it.HasRotOverride && c.HasRotOverride {
			it.HasRotOverride = true
			it.RotOverride = c.RotOverride
		}
	}
	items := make([]Item, 0, len(order))
	for _, k := range order {
		it := groups[k]
		sort.SliceStable(it.Bases, func(i, j int) bool { return refLess(it.Bases[i], it.Bases[j]) })
		sort.SliceStable(it.Designators, func(i, j int) bool { return refLess(it.Designators[i], it.Designators[j]) })
		items = append(items, *it)
	}
	sort.SliceStable(items, func(i, j int) bool { return refLess(items[i].ID(), items[j].ID()) })
	return items
}

func (p *Project) Placements() []Placement {
	out := make([]Placement, 0, len(p.Components))
	for _, c := range p.Components {
		out = append(out, Placement{
			Designator: c.Ref, X: c.X, Y: c.Y, Rotation: c.Rot,
			Layer: c.Layer, Value: c.Value, Package: c.Footprint,
			BodyW: c.BodyW, BodyH: c.BodyH,
		})
	}
	return out
}

func (p *Project) Board() *Board {
	b := &Board{Outline: p.Outline, Min: p.Min, Max: p.Max}
	for layer, segs := range p.tracks {
		b.Layers = append(b.Layers, GerberLayer{
			File: layer, Role: "copper", Function: layerFunc(layer), Segments: segs,
		})
	}
	if len(p.vias) > 0 {
		pads := make([]Pad, len(p.vias))
		for i, v := range p.vias {
			pads[i] = Pad{At: v, W: 0.4, H: 0.4, Round: true}
		}
		b.Layers = append(b.Layers, GerberLayer{File: "vias", Role: "via", Pads: pads})
	}
	return b
}

func layerFunc(layer string) string {
	switch {
	case strings.HasPrefix(layer, "F."):
		return "Top"
	case strings.HasPrefix(layer, "B."):
		return "Bottom"
	default:
		return "Inner"
	}
}

func refLess(a, b string) bool {
	pa, na := splitRef(a)
	pb, nb := splitRef(b)
	if pa != pb {
		return pa < pb
	}
	if na != nb {
		return na < nb
	}
	return a < b
}

func splitRef(s string) (string, int) {
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') {
		i++
	}
	n, _ := strconv.Atoi(s[i:])
	return s[:i], n
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
