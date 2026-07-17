package kicad

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Item struct {
	Designators []string
	Bases       []string
	Value       string
	Footprint   string
	Quantity    int
	LCSC        string
}

func (it Item) ID() string {
	if len(it.Bases) > 0 {
		return it.Bases[0]
	}
	if len(it.Designators) > 0 {
		return it.Designators[0]
	}
	return it.Value
}

func (it Item) PerBoard() int { return len(it.Bases) }

var panelSuffix = regexp.MustCompile(`_\d+$`)

func baseDesignator(d string) string { return panelSuffix.ReplaceAllString(d, "") }

func splitDesignators(s string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\t' || r == ' '
	}) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func collapsePanel(designators []string) []string {
	seen := map[string]bool{}
	var bases []string
	for _, d := range designators {
		b := baseDesignator(d)
		if !seen[b] {
			seen[b] = true
			bases = append(bases, b)
		}
	}
	sort.Strings(bases)
	return bases
}

func ImportBOM(path string) ([]Item, error) {
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
		return nil, fmt.Errorf("empty BOM")
	}

	header := rows[start]
	desigCol := matchCol(header, "designator", "designators", "refdes", "reference", "references", "ref")
	valCol := matchCol(header, "value", "comment", "val")
	fpCol := matchCol(header, "footprint", "package", "pattern")
	qtyCol := matchCol(header, "quantity", "qty", "count")
	lcscCol := matchCol(header, "lcscpart#", "lcscpart", "lcsc", "jlcpcbpart#", "jlcpart", "supplierpart")

	if desigCol < 0 && valCol < 0 {
		return nil, fmt.Errorf("could not detect BOM columns (need Designator/Value)")
	}

	var items []Item
	for _, row := range rows[start+1:] {
		if rowEmpty(row) {
			continue
		}
		it := Item{
			Value:     field(row, valCol),
			Footprint: field(row, fpCol),
			LCSC:      field(row, lcscCol),
			Quantity:  atoi(field(row, qtyCol)),
		}
		if d := field(row, desigCol); d != "" {
			it.Designators = splitDesignators(d)
			it.Bases = collapsePanel(it.Designators)
		}
		if it.Quantity == 0 {
			it.Quantity = len(it.Designators)
		}
		if len(it.Designators) == 0 && it.Value == "" {
			continue
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no rows in BOM")
	}
	return items, nil
}
