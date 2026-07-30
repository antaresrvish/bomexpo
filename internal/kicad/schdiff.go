package kicad

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"bomexpo/internal/value"
)

// Side names one of the three descriptions of a board that get compared: the
// schematic, the board file, and an external BOM.
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

func (s Side) short() string {
	if s == SideSch {
		return "sch"
	}
	return s.String()
}

// DiffKind is what went wrong for one designator on one side.
type DiffKind int

const (
	DiffMissing DiffKind = iota
	DiffExtra
	DiffDNP
	DiffExcluded
	DiffValue
	DiffFootprint
	DiffCode
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
	case DiffCode:
		return "part code"
	}
	return "?"
}

// Severe reports whether a kind would spoil an order rather than merely warrant a look.
func (k DiffKind) Severe() bool {
	switch k {
	case DiffMissing, DiffExtra, DiffValue, DiffDNP, DiffCode:
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

// Text writes every side the same way, so a difference shows up across the row.
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

// Row is one designator across all three sides, agreeing or not: a report that
// lists nothing looks the same as one that never ran.
type Row struct {
	Designator    string
	Sch, PCB, BOM Cell
	Kinds         []DiffKind
	Sides         []Side // the side each kind belongs to, in step with Kinds
	Ref           Side
}

// Cell is one side's reading.
func (r Row) Cell(s Side) Cell {
	switch s {
	case SidePCB:
		return r.PCB
	case SideBOM:
		return r.BOM
	}
	return r.Sch
}

// Agrees reports whether every side carrying this designator describes it the same.
func (r Row) Agrees() bool { return len(r.Kinds) == 0 }

// Severe reports whether anything here would spoil an order.
func (r Row) Severe() bool {
	for _, k := range r.Kinds {
		if k.Severe() {
			return true
		}
	}
	return false
}

// SideOK reports whether a side agrees with the reference.
func (r Row) SideOK(s Side) bool {
	if s == r.Ref {
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
		switch ref := r.Cell(r.Ref); {
		case ref.DNP:
			return "dnp, left out"
		case ref.OffBOM:
			return "off the bom"
		}
		return "agrees"
	}
	// Both sides differing the same way means the reference is the odd one out;
	// naming them both would blame the two that match.
	others := r.others()
	onBoth := map[DiffKind]bool{}
	if len(others) == 2 {
		for _, k := range []DiffKind{DiffValue, DiffFootprint, DiffCode} {
			if r.has(k, others[0]) && r.has(k, others[1]) &&
				strings.EqualFold(r.field(k, others[0]), r.field(k, others[1])) {
				onBoth[k] = true
			}
		}
	}

	seen := map[string]bool{}
	var out []string
	for i, k := range r.Kinds {
		label := r.Sides[i].String() + " " + k.String()
		if onBoth[k] {
			label = r.Ref.short() + " " + k.String() + " stale"
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return strings.Join(out, " + ")
}

func (r Row) has(k DiffKind, s Side) bool {
	for i := range r.Kinds {
		if r.Kinds[i] == k && r.Sides[i] == s {
			return true
		}
	}
	return false
}

func (r Row) others() []Side {
	var out []Side
	for _, s := range []Side{SideSch, SidePCB, SideBOM} {
		if s != r.Ref {
			out = append(out, s)
		}
	}
	return out
}

// RefStale reports whether the other two agree and differ from the reference, which
// points the fix at the reference.
func (r Row) RefStale() bool {
	o := r.others()
	if len(o) != 2 {
		return false
	}
	for _, k := range []DiffKind{DiffValue, DiffFootprint, DiffCode} {
		if r.has(k, o[0]) && r.has(k, o[1]) &&
			strings.EqualFold(r.field(k, o[0]), r.field(k, o[1])) {
			return true
		}
	}
	return false
}

// SchDiff is the whole comparison.
type SchDiff struct {
	SchPath, PCBPath, BOMPath string
	Rows                      []Row
	Findings                  []Finding
	SchCount                  int // bom-eligible symbols
	PCBCount                  int
	BOMCount                  int
	Matched                   int // designators every side agrees on
	// SkippedDNP are do-not-populate symbols a BOM rightly omits: not findings, but
	// without them the totals look wrong.
	SkippedDNP int
	unread     []string
	ref        Side
	codeRef    Side
	// notCompared are columns a side never supplied; comparing against one that
	// isn't there reports every row as a mismatch.
	notCompared []string
}

// Skipped names the sub-sheets that went unread.
func (d SchDiff) Skipped() []string { return d.unread }

// NotCompared names the fields a side carried no data for.
func (d SchDiff) NotCompared() []string { return d.notCompared }

// CodeRef is the side part codes were compared against.
func (d SchDiff) CodeRef() Side { return d.codeRef }

// Ref is the side everything else was measured against.
func (d SchDiff) Ref() Side { return d.ref }

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
	hasCodes                bool
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
		if strings.TrimSpace(it.LCSC) != "" {
			d.hasCodes = true
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

// Compare lines a schematic up against the board file and an external BOM, with ref
// naming the side the other two are measured against. A side given as nil stays
// blank and nothing is reported for it.
func Compare(sc *Schematic, pcb, bom []Item, ref Side) SchDiff {
	d := SchDiff{SchPath: sc.Path, unread: sc.Skipped, ref: ref}

	schData := sideData{byRef: map[string]Cell{}}
	for _, sym := range sc.Symbols {
		if sym.Ref == "" {
			continue
		}
		schData.byRef[upRef(sym.Ref)] = Cell{
			Present: true, Value: sym.Value, Footprint: sym.Footprint, Code: sym.LCSC,
			DNP: sym.DNP, OffBOM: !sym.Bommable(),
		}
		if sym.Value != "" {
			schData.hasValues = true
		}
		if sym.Footprint != "" {
			schData.hasFootprint = true
		}
		if sym.LCSC != "" {
			schData.hasCodes = true
		}
		if sym.Bommable() {
			d.SchCount++
		}
	}
	schData.count = len(schData.byRef)

	pcbData, bomData := gather(pcb), gather(bom)
	d.PCBCount, d.BOMCount = pcbData.count, bomData.count

	byside := map[Side]sideData{SideSch: schData, SidePCB: pcbData, SideBOM: bomData}
	on := map[Side]bool{SideSch: true, SidePCB: len(pcb) > 0, SideBOM: len(bom) > 0}
	if !on[ref] {
		ref = SideSch // nothing to measure against, so fall back to the schematic
		d.ref = ref
	}
	refData := byside[ref]

	// bomexpo writes part codes to the board, so a schematic often carries none; the
	// board stands in rather than leaving every code unchecked.
	codeRef := ref
	if !refData.hasCodes && pcbData.hasCodes {
		codeRef = SidePCB
	}
	d.codeRef = codeRef

	for _, s := range []Side{SideSch, SidePCB, SideBOM} {
		if !on[s] || s == ref {
			continue
		}
		if !byside[s].hasValues {
			d.notCompared = append(d.notCompared, s.String()+" value")
		}
		if !byside[s].hasFootprint {
			d.notCompared = append(d.notCompared, s.String()+" footprint")
		}
	}

	refs := map[string]bool{}
	for _, s := range []Side{SideSch, SidePCB, SideBOM} {
		for r := range byside[s].byRef {
			refs[r] = true
		}
	}

	for name := range refs {
		want := refData.byRef[name]
		row := Row{
			Designator: name, Ref: ref,
			Sch: schData.byRef[name], PCB: pcbData.byRef[name], BOM: bomData.byRef[name],
		}

		for _, side := range []Side{SideSch, SidePCB, SideBOM} {
			if !on[side] || side == ref {
				continue
			}
			data := byside[side]
			other := data.byRef[name]

			switch {
			case !want.Present && other.Present:
				row.add(DiffExtra, side)
				continue
			case want.Present && !other.Present:
				switch {
				case want.OffBOM:
					// kept off the bom on purpose, so its absence is correct
				case want.DNP && side == SideBOM:
					d.SkippedDNP++
				default:
					row.add(DiffMissing, side)
				}
				continue
			case !want.Present && !other.Present:
				continue
			}

			// authored, not derived: a BOM has no such column
			if side == SideBOM {
				switch {
				case want.OffBOM:
					row.add(DiffExcluded, side)
				case want.DNP:
					row.add(DiffDNP, side)
				}
			}
			if data.hasValues && refData.hasValues && !sameValue(want.Value, other.Value) {
				row.add(DiffValue, side)
			}
			if data.hasFootprint && refData.hasFootprint && !sameFootprint(want.Footprint, other.Footprint) {
				row.add(DiffFootprint, side)
			}
			if side != codeRef && data.hasCodes {
				code := row.Cell(codeRef).Code
				if code != "" && other.Code != "" && !strings.EqualFold(code, other.Code) {
					row.add(DiffCode, side)
				}
			}
		}

		if row.Agrees() {
			d.Matched++
		}
		d.Rows = append(d.Rows, row)
	}

	sort.SliceStable(d.Rows, func(i, j int) bool {
		a, b := d.Rows[i], d.Rows[j]
		if a.Agrees() != b.Agrees() {
			return !a.Agrees()
		}
		if a.Severe() != b.Severe() {
			return a.Severe()
		}
		return refLess(a.Designator, b.Designator)
	})

	for _, r := range d.Rows {
		for i, k := range r.Kinds {
			side := r.Sides[i]
			d.Findings = append(d.Findings, Finding{
				Kind: k, Side: side, Ref: r.Designator,
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

// field narrows a cell to what the finding is about.
func (r Row) field(k DiffKind, s Side) string {
	c := r.Cell(s)
	switch k {
	case DiffValue:
		return dash(c.Value)
	case DiffFootprint:
		return dash(c.Footprint)
	case DiffCode:
		return dash(c.Code)
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

// sameValue compares electrically first, so 0.1uF matches 100nF, then textually.
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

// sameFootprint ignores the library prefix, and accepts a bare chip size against a
// full name — C_0603_1608Metric is the 0603 land. Only one side may be bare, or
// C_0402 and R_0402 would compare equal and hide a real swap.
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

// bareSize reports whether a footprint is just the chip size: "0402", "C0402",
// "0402 (1005)".
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
	for _, s := range []Side{SideSch, SidePCB, SideBOM} {
		if n := by[s]; n > 0 {
			where = append(where, fmt.Sprintf("%d in the %s", n, s.short()))
		}
	}
	return fmt.Sprintf("%d of %d agree · %s%s",
		d.Matched, len(d.Rows), strings.Join(where, " · "), dnp)
}
