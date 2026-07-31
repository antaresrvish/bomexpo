package kicad

import (
	"fmt"
	"strconv"
	"strings"
)

type Placement struct {
	Designator string
	X, Y       float64
	Rotation   float64
	Layer      string
	Value      string
	Package    string
	// PackageLib is the footprint's library, which decides whose 0° it is drawn to.
	PackageLib string
	BodyW      float64
	BodyH      float64
}

func normLayer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "bottom", "bot", "b", "back":
		return "bottom"
	default:
		return "top"
	}
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func ImportCPL(path string) ([]Placement, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	start := -1
	for i, r := range rows {
		if !rowEmpty(r) {
			start = i
			break
		}
	}
	if start < 0 || len(rows)-start < 2 {
		return nil, fmt.Errorf("empty CPL")
	}

	header := rows[start]
	refCol := matchCol(header, "designator", "ref", "refdes", "reference")
	xCol := matchCol(header, "midx", "posx", "x", "centerx", "refx")
	yCol := matchCol(header, "midy", "posy", "y", "centery", "refy")
	rotCol := matchCol(header, "rotation", "rot", "angle")
	layerCol := matchCol(header, "layer", "side")
	valCol := matchCol(header, "val", "value", "comment")
	pkgCol := matchCol(header, "package", "footprint")

	if refCol < 0 || xCol < 0 || yCol < 0 {
		return nil, fmt.Errorf("could not detect CPL columns (need Designator/X/Y)")
	}

	var out []Placement
	for _, row := range rows[start+1:] {
		if rowEmpty(row) {
			continue
		}
		ref := field(row, refCol)
		if ref == "" {
			continue
		}
		out = append(out, Placement{
			Designator: ref,
			X:          parseFloat(field(row, xCol)),
			Y:          parseFloat(field(row, yCol)),
			Rotation:   parseFloat(field(row, rotCol)),
			Layer:      normLayer(field(row, layerCol)),
			Value:      field(row, valCol),
			Package:    field(row, pkgCol),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no placements in CPL")
	}
	return out, nil
}
