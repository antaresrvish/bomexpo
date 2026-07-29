package kicad

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Item struct {
	Designators    []string
	Bases          []string
	Value          string
	Footprint      string
	Quantity       int
	LCSC           string
	DNP            bool
	ExcludeBOM     bool
	RotOverride    int
	HasRotOverride bool
	// Nets are every net the grouped components touch. Empty for a BOM CSV,
	// which carries no connectivity.
	Nets []string
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

// GroupItems turns a flat list of rows into sorted line items.
//
// Rows that already list several designators are taken as pre-grouped by
// whatever exported them, and are only sorted — re-merging those could fold
// together rows the exporter split deliberately, say two part codes for the same
// value and footprint.
func GroupItems(items []Item) []Item {
	for _, it := range items {
		if len(it.Designators) > 1 {
			out := append([]Item(nil), items...)
			sortItems(out)
			return out
		}
	}
	return mergeItems(items)
}

// mergeItems merges rows describing the same part — same value, footprint and
// DNP flag — into one line item, then sorts everything by reference.
func mergeItems(items []Item) []Item {
	type key struct {
		v, f string
		dnp  bool
	}
	var order []key
	groups := map[key]*Item{}

	for _, in := range items {
		k := key{in.Value, in.Footprint, in.DNP}
		it, ok := groups[k]
		if !ok {
			cp := in
			cp.Designators, cp.Bases, cp.Quantity, cp.Nets = nil, nil, 0, nil
			groups[k] = &cp
			order = append(order, k)
			it = &cp
		} else {
			// a group is only excluded when every member is
			it.ExcludeBOM = it.ExcludeBOM && in.ExcludeBOM
		}
		it.Designators = append(it.Designators, in.Designators...)
		it.Bases = append(it.Bases, in.Bases...)
		it.Nets = append(it.Nets, in.Nets...)
		it.Quantity += max(in.Quantity, len(in.Designators))
		if it.LCSC == "" {
			it.LCSC = in.LCSC
		}
		if !it.HasRotOverride && in.HasRotOverride {
			it.HasRotOverride, it.RotOverride = true, in.RotOverride
		}
	}

	out := make([]Item, 0, len(order))
	for _, k := range order {
		it := groups[k]
		sort.SliceStable(it.Bases, func(i, j int) bool { return refLess(it.Bases[i], it.Bases[j]) })
		sort.SliceStable(it.Designators, func(i, j int) bool { return refLess(it.Designators[i], it.Designators[j]) })
		it.Nets = uniqueSorted(it.Nets)
		out = append(out, *it)
	}
	sortItems(out)
	return out
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool { return refLess(items[i].ID(), items[j].ID()) })
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
