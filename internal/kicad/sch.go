package kicad

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Symbol is one schematic symbol as it would appear on a BOM.
type Symbol struct {
	Ref       string
	Value     string
	Footprint string
	LibID     string
	LCSC      string
	Unit      int
	InBOM     bool
	DNP       bool
}

// Bommable reports whether this symbol should reach a BOM at all: power rails and
// anything the designer excluded do not.
func (s Symbol) Bommable() bool {
	if !s.InBOM || s.Ref == "" || strings.HasPrefix(s.Ref, "#") {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(s.LibID), "power:")
}

// Schematic is the symbols read out of one or more .kicad_sch files.
type Schematic struct {
	Path    string
	Symbols []Symbol
	// Sheets are hierarchical sub-sheets that were followed.
	Sheets []string
	// Skipped are sub-sheets named by the design that could not be read, so a
	// short count never passes for a complete one.
	Skipped []string
}

// LoadSchematic reads a schematic and every sub-sheet it names. path may be the
// .kicad_sch itself, a .kicad_pro, or the project folder.
func LoadSchematic(path string) (*Schematic, error) {
	root, err := resolveSch(path)
	if err != nil {
		return nil, err
	}
	sc := &Schematic{Path: root}
	seen := map[string]bool{}
	if err := sc.read(root, seen); err != nil {
		return nil, err
	}
	sc.dedupeUnits()
	return sc, nil
}

// resolveSch finds the schematic to open from a file or folder.
func resolveSch(path string) (string, error) {
	full, err := ExpandPath(path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		switch strings.ToLower(filepath.Ext(full)) {
		case ".kicad_sch":
			return full, nil
		case ".kicad_pro", ".kicad_pcb":
			cand := strings.TrimSuffix(full, filepath.Ext(full)) + ".kicad_sch"
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
			return "", fmt.Errorf("no .kicad_sch beside %s", filepath.Base(full))
		}
		return "", fmt.Errorf("%s is not a kicad schematic", filepath.Base(full))
	}

	hits, _ := filepath.Glob(filepath.Join(full, "*.kicad_sch"))
	switch len(hits) {
	case 0:
		return "", fmt.Errorf("no .kicad_sch in %s", filepath.Base(full))
	case 1:
		return hits[0], nil
	}
	// Several: prefer the one named after the project, which is the root sheet.
	if pro, _ := filepath.Glob(filepath.Join(full, "*.kicad_pro")); len(pro) == 1 {
		want := strings.TrimSuffix(pro[0], filepath.Ext(pro[0])) + ".kicad_sch"
		for _, h := range hits {
			if h == want {
				return h, nil
			}
		}
	}
	sort.Strings(hits)
	return hits[0], nil
}

// read parses one sheet file and recurses into the sub-sheets it names.
func (sc *Schematic) read(path string, seen map[string]bool) error {
	if seen[path] {
		return nil // a sheet used twice is still one file
	}
	seen[path] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseSexp(string(data))
	if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	top := root
	if len(root.kids) == 1 && root.kids[0].head() == "kicad_sch" {
		top = root.kids[0]
	}
	if top.head() != "kicad_sch" {
		return fmt.Errorf("%s is not a schematic", filepath.Base(path))
	}

	for _, n := range top.kids {
		switch n.head() {
		case "symbol":
			if s, ok := parseSymbol(n); ok {
				sc.Symbols = append(sc.Symbols, s)
			}
		case "sheet":
			sub := sheetFile(n)
			if sub == "" {
				continue
			}
			p := filepath.Join(filepath.Dir(path), sub)
			if _, err := os.Stat(p); err != nil {
				sc.Skipped = append(sc.Skipped, sub)
				continue
			}
			sc.Sheets = append(sc.Sheets, sub)
			if err := sc.read(p, seen); err != nil {
				sc.Skipped = append(sc.Skipped, sub)
			}
		}
	}
	return nil
}

// sheetFile is the file a hierarchical sheet points at.
func sheetFile(n *node) string {
	for _, k := range n.kids {
		if k.head() != "property" {
			continue
		}
		if strings.EqualFold(atom(k, 1), "Sheetfile") || strings.EqualFold(atom(k, 1), "Sheet file") {
			return atom(k, 2)
		}
	}
	return ""
}

// parseSymbol reads one schematic symbol. The reference comes from the instances
// block rather than the Reference property: that is what KiCad annotates, and in a
// hierarchical design it is the only place the per-instance name lives.
func parseSymbol(n *node) (Symbol, bool) {
	s := Symbol{InBOM: true, Unit: 1}
	for _, k := range n.kids {
		switch k.head() {
		case "lib_id":
			s.LibID = atom(k, 1)
		case "unit":
			s.Unit = int(num(atom(k, 1)))
		case "in_bom":
			s.InBOM = atom(k, 1) == "yes"
		case "dnp":
			s.DNP = atom(k, 1) == "yes"
		case "property":
			name := strings.ToLower(atom(k, 1))
			val := atom(k, 2)
			switch {
			case name == "reference":
				s.Ref = val
			case name == "value":
				s.Value = val
			case name == "footprint":
				s.Footprint = shortFootprint(val)
			case strings.Contains(name, "lcsc") || strings.Contains(name, "jlc"):
				if val != "" {
					s.LCSC = val
				}
			}
		case "instances":
			if ref, unit, ok := instanceRef(k); ok {
				s.Ref = ref
				if unit > 0 {
					s.Unit = unit
				}
			}
		}
	}
	if s.Ref == "" {
		return Symbol{}, false
	}
	return s, true
}

// instanceRef digs the annotated reference out of an instances block.
func instanceRef(n *node) (ref string, unit int, ok bool) {
	for _, proj := range n.kids {
		if proj.head() != "project" {
			continue
		}
		for _, p := range proj.kids {
			if p.head() != "path" {
				continue
			}
			r := atom(child(p, "reference"), 1)
			if r == "" {
				continue
			}
			return r, int(num(atom(child(p, "unit"), 1))), true
		}
	}
	return "", 0, false
}

// dedupeUnits collapses a multi-unit symbol — an opamp drawn as four gates is one
// line on a BOM, not four. The unit carrying the footprint wins.
func (sc *Schematic) dedupeUnits() {
	byRef := map[string]int{}
	var out []Symbol
	for _, s := range sc.Symbols {
		if i, dup := byRef[s.Ref]; dup {
			if out[i].Footprint == "" && s.Footprint != "" {
				out[i].Footprint = s.Footprint
			}
			if out[i].Value == "" && s.Value != "" {
				out[i].Value = s.Value
			}
			continue
		}
		byRef[s.Ref] = len(out)
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return refLess(out[i].Ref, out[j].Ref) })
	sc.Symbols = out
}

// BOMSymbols are the symbols that belong on a BOM, DNP included — whether a DNP
// part should be there is the caller's call, and the diff reports on it.
func (sc *Schematic) BOMSymbols() []Symbol {
	var out []Symbol
	for _, s := range sc.Symbols {
		if s.Bommable() {
			out = append(out, s)
		}
	}
	return out
}
