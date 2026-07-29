package kicad

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Design is everything bomexpo needs to work with, whether it came from a KiCad
// board or from a plain BOM export. A CSV design has no PCBPath, which is what
// gerber export and the 3D render key off.
type Design struct {
	Name    string
	PCBPath string // empty when there's no board behind this design
	BOMPath string // set when the line items came from a CSV
	CPLPath string // set when the placements came from a CSV

	Items      []Item
	Placements []Placement
	Board      *Board
	Nets       []Net // empty for a CSV design: a BOM carries no connectivity
	// Lands maps a footprint name to its pads, so a line item can be drawn from
	// its Footprint field. Empty for a CSV design.
	Lands  map[string][]Land
	Layers int
	BoardW float64
	BoardH float64
}

// FromBoard reports whether a .kicad_pcb backs this design.
func (d *Design) FromBoard() bool { return d.PCBPath != "" }

// IsBOMFile reports whether a path looks like a spreadsheet export rather than a
// KiCad design.
func IsBOMFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv":
		return true
	}
	return false
}

// Load opens a design: a .kicad_pcb, a project folder or .kicad_pro, or a BOM
// CSV. cplPath is only used for a CSV BOM — when empty, a placement file sitting
// beside the BOM is picked up automatically.
func Load(path, cplPath string) (*Design, error) {
	full, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	if IsBOMFile(full) {
		return loadBOM(full, cplPath)
	}
	p, err := LoadProject(full)
	if err != nil {
		return nil, err
	}
	return &Design{
		Name:       p.Name,
		PCBPath:    p.PCBPath,
		Items:      p.BOM(),
		Placements: p.Placements(),
		Board:      p.Board(),
		Nets:       p.Nets(),
		Lands:      p.Lands(),
		Layers:     p.Layers,
		BoardW:     p.BoardW,
		BoardH:     p.BoardH,
	}, nil
}

func loadBOM(bomPath, cplPath string) (*Design, error) {
	items, err := ImportBOM(bomPath)
	if err != nil {
		return nil, err
	}
	d := &Design{
		Name:    strings.TrimSuffix(filepath.Base(bomPath), filepath.Ext(bomPath)),
		BOMPath: bomPath,
		Items:   GroupItems(items),
	}

	// An explicitly named placement file has to work or say why; one we found
	// ourselves is a bonus and stays quiet.
	explicit := strings.TrimSpace(cplPath) != ""
	if !explicit {
		cplPath = findCPL(bomPath)
	}
	if cplPath == "" {
		return d, nil
	}

	full, err := ExpandPath(cplPath)
	if err != nil {
		if explicit {
			return nil, err
		}
		return d, nil
	}
	pl, err := ImportCPL(full)
	if err != nil {
		if explicit {
			return nil, fmt.Errorf("placement file %s: %w", filepath.Base(full), err)
		}
		return d, nil
	}
	d.CPLPath = full
	d.Placements = pl
	// No outline to frame with, so frame on where the parts actually sit. Board
	// size stays zero: the placement extent isn't the board's size and claiming
	// otherwise would be a lie in the status bar.
	d.Board = placementFrame(pl)
	return d, nil
}

// findCPL looks for the placement file that usually ships alongside a BOM
// export. It returns "" rather than guessing wildly.
func findCPL(bomPath string) string {
	dir := filepath.Dir(bomPath)
	ext := filepath.Ext(bomPath)
	base := strings.TrimSuffix(filepath.Base(bomPath), ext)

	var names []string
	// the common shape is a matching pair, "…bom….csv" next to "…cpl….csv"
	lower := strings.ToLower(base)
	for _, to := range []string{"cpl", "pos", "positions", "placement"} {
		if i := strings.Index(lower, "bom"); i >= 0 {
			names = append(names, base[:i]+to+base[i+3:]+ext)
		}
	}
	names = append(names,
		base+"-cpl"+ext, base+"_cpl"+ext, base+"-pos"+ext, base+"_pos"+ext,
		"cpl"+ext, "positions"+ext, "pos"+ext,
	)

	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.EqualFold(p, bomPath) {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// placementFrame builds the bounding box of the placements so the board view has
// something to draw when there's no outline to go on.
func placementFrame(pl []Placement) *Board {
	if len(pl) == 0 {
		return nil
	}
	min, max := Point{pl[0].X, pl[0].Y}, Point{pl[0].X, pl[0].Y}
	for _, p := range pl[1:] {
		if p.X < min.X {
			min.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	// pad so parts sitting on the extremes aren't drawn half off-canvas
	const pad = 2
	min.X, min.Y = min.X-pad, min.Y-pad
	max.X, max.Y = max.X+pad, max.Y+pad
	return &Board{Min: min, Max: max}
}
