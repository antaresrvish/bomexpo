package kicad

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"bomexpo/internal/value"
)

// Comparing three descriptions of the same board: the schematic, the board file,
// and a BOM some other tool produced. The schematic is taken as the truth — it is
// where a designer changes a value — so the board and the BOM are each measured
// against it.

// Side names one of the three descriptions.
type Side int

const (
	SideSch Side = iota
	SidePCB
	SideBOM
)

func (s Side) String() string {
	switch s {
	case SidePCB:
		return "pcb"
	case SideBOM:
		return "bom"
	}
	return "schematic"
}

// DiffKind is what went wrong for one designator on one side.
type DiffKind int

const (
	// DiffMissing is a part the schematic wants that a side never lists — the one
	// that gets a board assembled with a hole in it.
	DiffMissing DiffKind = iota
	// DiffExtra is a line with no symbol behind it.
	DiffExtra
	// DiffDNP is a part the schematic marks do-not-populate that the BOM buys and
	// places anyway.
	DiffDNP
	// DiffExcluded is a symbol flagged out of the BOM that the BOM includes.
	DiffExcluded
	// DiffValue and DiffFootprint are the same designator described two ways.
	DiffValue
	DiffFootprint
)

func (k DiffKind) String() string {
	switch k {
	case DiffMissing:
		return "missing"
	case DiffExtra:
		return "not in schematic"
	case DiffDNP:
		return "dnp but listed"
	case DiffExcluded:
		return "excluded but listed"
	case DiffValue:
		return "value"
	case DiffFootprint:
		return "footprint"
	}
	return "?"
}

// Severe reports whether a kind would actually spoil an order, as opposed to being
// worth a look.
func (k DiffKind) Severe() bool {
	switch k {
	case DiffMissing, DiffExtra, DiffValue, DiffDNP:
		return true
	}
	return false
}

// Finding is one disagreement: a kind, the side that deviates, and both readings.
type Finding struct {
	Kind  DiffKind
	Side  Side
	Ref   string
	Sch   string
	Other string
}

// Cell is one side's description of a designator.
type Cell struct {
	Present   bool
	Value     string
	Footprint string
	Code      string
	DNP       bool
	OffBOM    bool
}

// Text writes a cell the same way for every side, so a difference is visible by
// sitting directly across from its counterpart.
func (c Cell) Text() string {
	if !c.Present {
		return "—"
	}
	out := dash(c.Value)
	if c.Footprint != "" {
		out += " · " + c.Footprint
	}
	if c.Code != "" {
		out += " · " + c.Code
	}
	return out
}

// Row is one designator across all three sides, whether or not they agree. The
// report is built from these so a clean comparison still shows its work — a screen
// that says "all good" and lists nothing is indistinguishable from one that ran
// nothing.
type Row struct {
	Ref           string
	Sch, PCB, BOM Cell
	Kinds         []DiffKind
	Sides         []Side // the side each kind belongs to, in step with Kinds
}

// Cell returns one side's reading.
func (r Row) Cell(s Side) Cell {
	switch s {
	case SidePCB:
		return r.PCB
	case SideBOM:
		return r.BOM
	}
	return r.Sch
}

// Agrees reports whether every side that carries this designator describes it the
// same way.
func (r Row) Agrees() bool { return len(r.Kinds) == 0 }

// Severe reports whether anything wrong here would spoil an order.
func (r Row) Severe() bool {
	for _, k := range r.Kinds {
		if k.Severe() {
			return true
		}
	}
	return false
}

// SideOK reports whether a side agrees with the schematic, for colouring its cell.
func (r Row) SideOK(s Side) bool {
	if s == SideSch {
		return true
	}
	for _, dev := range r.Sides {
		if dev == s {
			return false
		}
	}
	return true
}

// What names the disagreements, or says the designator is fine.
func (r Row) What() string {
	if len(r.Kinds) == 0 {
		switch {
		case r.Sch.DNP:
			return "dnp, left out"
		case r.Sch.OffBOM:
			return "off the bom"
		}
		return "agrees"
	}
	seen := map[string]bool{}
	var out []string
	for i, k := range r.Kinds {
		label := r.Sides[i].String() + " " + k.String()
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return strings.Join(out, " + ")
}

// SchDiff is the whole comparison.
type SchDiff struct {
	SchPath, PCBPath, BOMPath string
	// Rows is every designator any side knows about, lined up.
	Rows     []Row
	Findings []Finding
	SchCount int // bom-eligible symbols
	PCBCount int
	BOMCount int
	Matched  int // designators every side agrees on
	// SkippedDNP counts do-not-populate symbols a BOM rightly leaves out. They are
	// not findings, but without them the totals look like they disagree for no
	// reason.
	SkippedDNP int
	unread     []string
	// notCompared names the columns a side never supplied. Comparing against a
	// column that isn't there would report every single row as a mismatch.
	notCompared []string
}

// Skipped names the sub-sheets that went unread.
func (d SchDiff) Skipped() []string { return d.unread }

// NotCompared names the fields a side carried no data for.
func (d SchDiff) NotCompared() []string { return d.notCompared }

// Counts totals the findings by kind.
func (d SchDiff) Counts() map[DiffKind]int {
	out := map[DiffKind]int{}
	for _, f := range d.Findings {
		out[f.Kind]++
	}
	return out
}

// SideCounts totals the findings by which side deviates.
func (d SchDiff) SideCounts() map[Side]int {
	out := map[Side]int{}
	for _, f := range d.Findings {
		out[f.Side]++
	}
	return out
}

// Severe is how many findings would spoil an order.
func (d SchDiff) Severe() int {
	n := 0
	for _, f := range d.Findings {
		if f.Kind.Severe() {
			n++
		}
	}
	return n
}

// sideData is one side's designators, plus whether it supplied each column at all.
type sideData struct {
	byRef                   map[string]Cell
	hasValues, hasFootprint bool
	count                   int
}

func gather(items []Item) sideData {
	d := sideData{byRef: map[string]Cell{}}
	for _, it := range items {
		if strings.TrimSpace(it.Value) != "" {
			d.hasValues = true
		}
		if strings.TrimSpace(it.Footprint) != "" {
			d.hasFootprint = true
		}
		for _, ref := range it.Designators {
			r := upRef(ref)
			if r == "" {
				continue
			}
			d.byRef[r] = Cell{
				Present: true, Value: it.Value, Footprint: it.Footprint,
				Code: it.LCSC, DNP: it.DNP, OffBOM: it.ExcludeBOM,
			}
			d.count++
		}
	}
	return d
}

// Compare lines a schematic up against the board file and an external BOM. Either
// of the latter may be empty, in which case that column stays blank and nothing is
// reported against it.
func Compare(sc *Schematic, pcb, bom []Item) SchDiff {
	d := SchDiff{SchPath: sc.Path, unread: sc.Skipped}

	schByRef := map[string]Cell{}
	for _, s := range sc.Symbols {
		if s.Ref == "" {
			continue
		}
		schByRef[upRef(s.Ref)] = Cell{
			Present: true, Value: s.Value, Footprint: s.Footprint, Code: s.LCSC,
			DNP: s.DNP, OffBOM: !s.Bommable(),
		}
		if s.Bommable() {
			d.SchCount++
		}
	}

	pcbData, bomData := gather(pcb), gather(bom)
	d.PCBCount, d.BOMCount = pcbData.count, bomData.count
	if len(bom) > 0 && !bomData.hasValues {
		d.notCompared = append(d.notCompared, "bom value")
	}
	if len(bom) > 0 && !bomData.hasFootprint {
		d.notCompared = append(d.notCompared, "bom footprint")
	}

	refs := map[string]bool{}
	for _, m := range []map[string]Cell{schByRef, pcbData.byRef, bomData.byRef} {
		for r := range m {
			refs[r] = true
		}
	}

	sides := []struct {
		side Side
		data sideData
		on   bool
	}{
		{SidePCB, pcbData, len(pcb) > 0},
		{SideBOM, bomData, len(bom) > 0},
	}

	for ref := range refs {
		sch := schByRef[ref]
		row := Row{Ref: ref, Sch: sch, PCB: pcbData.byRef[ref], BOM: bomData.byRef[ref]}

		for _, s := range sides {
			if !s.on {
				continue
			}
			other := s.data.byRef[ref]
			switch {
			case !sch.Present && other.Present:
				row.add(DiffExtra, s.side)
				continue
			case sch.Present && !other.Present:
				switch {
				case sch.OffBOM:
					// kept off the bom on purpose, so its absence is correct
				case sch.DNP && s.side == SideBOM:
					d.SkippedDNP++
				default:
					row.add(DiffMissing, s.side)
				}
				continue
			case !sch.Present && !other.Present:
				continue
			}

			if s.side == SideBOM {
				switch {
				case sch.OffBOM:
					row.add(DiffExcluded, s.side)
				case sch.DNP:
					row.add(DiffDNP, s.side)
				}
			}
			if s.data.hasValues && !sameValue(sch.Value, other.Value) {
				row.add(DiffValue, s.side)
			}
			if s.data.hasFootprint && !sameFootprint(sch.Footprint, other.Footprint) {
				row.add(DiffFootprint, s.side)
			}
		}

		if row.Agrees() {
			d.Matched++
		}
		d.Rows = append(d.Rows, row)
	}

	// Disagreements first, serious ones above the rest, then by designator.
	sort.SliceStable(d.Rows, func(i, j int) bool {
		a, b := d.Rows[i], d.Rows[j]
		if a.Agrees() != b.Agrees() {
			return !a.Agrees()
		}
		if a.Severe() != b.Severe() {
			return a.Severe()
		}
		return refLess(a.Ref, b.Ref)
	})

	for _, r := range d.Rows {
		for i, k := range r.Kinds {
			side := r.Sides[i]
			d.Findings = append(d.Findings, Finding{
				Kind: k, Side: side, Ref: r.Ref,
				Sch: r.field(k, SideSch), Other: r.field(k, side),
			})
		}
	}
	return d
}

func (r *Row) add(k DiffKind, s Side) {
	r.Kinds = append(r.Kinds, k)
	r.Sides = append(r.Sides, s)
}

// field is just the part of a cell a finding is about, so a value mismatch shows
// values rather than a whole line of unrelated detail.
func (r Row) field(k DiffKind, s Side) string {
	c := r.Cell(s)
	switch k {
	case DiffValue:
		return dash(c.Value)
	case DiffFootprint:
		return dash(c.Footprint)
	}
	return c.Text()
}

func upRef(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// sameValue compares electrically first and textually second, so 0.1uF matches
// 100nF but "TestPoint" still has to match "TestPoint".
func sameValue(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	va, oka := value.Parse(a)
	vb, okb := value.Parse(b)
	if oka && okb {
		return value.Equal(va, vb)
	}
	return false
}

// chipSizeRe finds an imperial chip size. The digit guards matter more than \b
// does, since \b treats the underscore in C_0402_1005Metric as a word character
// and so never fires there.
var chipSizeRe = regexp.MustCompile(`(?:^|[^0-9])(01005|0201|0402|0603|0805|1206|1210|1806|1812|2010|2512)(?:[^0-9]|$)`)

func chipSize(s string) string {
	if m := chipSizeRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// sameFootprint is lenient about the library prefix, and about a BOM that recorded
// nothing but the chip size — a schematic's C_0603_1608Metric against a BOM's 0603
// is the same land, and calling that a mismatch would bury the real ones.
//
// The size shortcut needs one side to be only the size. Accepting it both ways
// would call C_0402 and R_0402 equal and hide a genuine footprint swap.
func sameFootprint(a, b string) bool {
	a, b = shortFootprint(strings.TrimSpace(a)), shortFootprint(strings.TrimSpace(b))
	if strings.EqualFold(a, b) {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	if sa, sb := chipSize(a), chipSize(b); sa != "" && sa == sb {
		if bareSize(a) || bareSize(b) {
			return true
		}
	}
	// one extending the other covers "SOT-23" against "SOT-23-3"
	la, lb := strings.ToLower(a), strings.ToLower(b)
	return strings.HasPrefix(la, lb) || strings.HasPrefix(lb, la)
}

// bareSize reports whether a footprint field is just the chip size, give or take a
// letter — "0402", "C0402", "0402 (1005)".
func bareSize(s string) bool {
	size := chipSize(s)
	return size != "" && len(s) <= len(size)+2
}

// Summary is a one-line verdict for a status bar.
func (d SchDiff) Summary() string {
	dnp := ""
	if d.SkippedDNP > 0 {
		dnp = fmt.Sprintf(" · %d dnp not expected in the bom", d.SkippedDNP)
	}
	if len(d.Findings) == 0 {
		return fmt.Sprintf("%d designators agree%s", d.Matched, dnp)
	}
	by := d.SideCounts()
	var where []string
	if n := by[SidePCB]; n > 0 {
		where = append(where, fmt.Sprintf("%d on the pcb", n))
	}
	if n := by[SideBOM]; n > 0 {
		where = append(where, fmt.Sprintf("%d in the bom", n))
	}
	return fmt.Sprintf("%d of %d agree · %s%s",
		d.Matched, len(d.Rows), strings.Join(where, " · "), dnp)
}
