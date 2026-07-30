package kicad

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"bomexpo/internal/value"
)

// DiffKind is what went wrong for one designator.
type DiffKind int

const (
	// DiffMissing is a part the schematic wants that the BOM never lists — the one
	// that gets a board assembled with a hole in it.
	DiffMissing DiffKind = iota
	// DiffExtra is a BOM line with no symbol behind it.
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
		return "missing from bom"
	case DiffExtra:
		return "not in schematic"
	case DiffDNP:
		return "dnp but in bom"
	case DiffExcluded:
		return "excluded but in bom"
	case DiffValue:
		return "value differs"
	case DiffFootprint:
		return "footprint differs"
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

// Finding is one disagreement, with both sides so the fix is obvious.
type Finding struct {
	Kind DiffKind
	Ref  string
	Sch  string // what the schematic says
	BOM  string // what the BOM says
}

// Row is one designator with both sides lined up, whether or not they agree. The
// report is built from these so a clean comparison still shows its work — a screen
// that says "all good" and lists nothing is indistinguishable from one that did
// nothing.
type Row struct {
	Ref          string
	SchValue     string
	SchFootprint string
	BOMValue     string
	BOMFootprint string
	BOMCode      string
	InSch        bool
	InBOM        bool
	DNP          bool // the schematic marks it do-not-populate
	OffBOM       bool // the schematic keeps it off the BOM
	Kinds        []DiffKind
}

// Agrees reports whether this designator came out clean.
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

// What names the disagreements, or says the designator is fine.
func (r Row) What() string {
	if len(r.Kinds) == 0 {
		switch {
		case r.DNP:
			return "dnp, left out"
		case r.OffBOM:
			return "off the bom"
		}
		return "agrees"
	}
	var out []string
	for _, k := range r.Kinds {
		out = append(out, k.String())
	}
	return strings.Join(out, " + ")
}

// SchDiff is the whole comparison.
type SchDiff struct {
	SchPath, BOMPath string
	// Rows is every designator either side knows about, lined up.
	Rows     []Row
	Findings []Finding
	SchCount int // bom-eligible symbols
	BOMCount int // designators in the external bom
	Matched  int // designators both agree on completely
	// SkippedDNP counts do-not-populate symbols the BOM rightly leaves out. They
	// are not findings, but without them the schematic and BOM totals look like
	// they disagree for no reason.
	SkippedDNP int
	// unread carries the sub-sheets the schematic named but could not be read, so a
	// comparison over half a design can't pass for a whole one.
	unread []string
	// notCompared names the columns the BOM never supplied. Comparing against a
	// column that isn't there would report every single row as a mismatch.
	notCompared []string
}

// NotCompared names the fields the BOM carried no data for.
func (d SchDiff) NotCompared() []string { return d.notCompared }

// Skipped names the sub-sheets that went unread.
func (d SchDiff) Skipped() []string { return d.unread }

// Counts totals the findings by kind.
func (d SchDiff) Counts() map[DiffKind]int {
	out := map[DiffKind]int{}
	for _, f := range d.Findings {
		out[f.Kind]++
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

// DiffSchematicBOM compares a schematic against an externally produced BOM, one
// designator at a time.
//
// Values are compared through internal/value, so "0.1uF" and "100nF" agree —
// a plain string compare would drown the report in false alarms.
func DiffSchematicBOM(sc *Schematic, items []Item) SchDiff {
	d := SchDiff{SchPath: sc.Path, unread: sc.Skipped}

	bySch := map[string]Symbol{}
	offBOM := map[string]Symbol{}
	for _, s := range sc.Symbols {
		if s.Ref == "" {
			continue
		}
		if s.Bommable() {
			bySch[upRef(s.Ref)] = s
			d.SchCount++
			continue
		}
		offBOM[upRef(s.Ref)] = s
	}

	byBOM := map[string]Item{}
	hasValues, hasFootprints := false, false
	for _, it := range items {
		if strings.TrimSpace(it.Value) != "" {
			hasValues = true
		}
		if strings.TrimSpace(it.Footprint) != "" {
			hasFootprints = true
		}
		for _, ref := range it.Designators {
			r := upRef(ref)
			if r == "" {
				continue
			}
			byBOM[r] = it
			d.BOMCount++
		}
	}
	if len(items) > 0 && !hasValues {
		d.notCompared = append(d.notCompared, "value")
	}
	if len(items) > 0 && !hasFootprints {
		d.notCompared = append(d.notCompared, "footprint")
	}

	// Every designator either side knows about, so the report can show its work.
	refs := map[string]bool{}
	for r := range bySch {
		refs[r] = true
	}
	for r := range offBOM {
		refs[r] = true
	}
	for r := range byBOM {
		refs[r] = true
	}

	for ref := range refs {
		sym, inSch := bySch[ref]
		off, isOff := offBOM[ref]
		if !inSch && isOff {
			sym = off
		}
		it, inBOM := byBOM[ref]

		row := Row{
			Ref: sym.Ref, InSch: inSch, InBOM: inBOM,
			SchValue: sym.Value, SchFootprint: sym.Footprint,
			BOMValue: it.Value, BOMFootprint: it.Footprint, BOMCode: it.LCSC,
			DNP: sym.DNP, OffBOM: isOff,
		}
		if row.Ref == "" {
			row.Ref = ref
		}

		switch {
		case !inSch && !isOff:
			row.Kinds = append(row.Kinds, DiffExtra)
		case isOff && inBOM:
			row.Kinds = append(row.Kinds, DiffExcluded)
		case inSch && !inBOM:
			if sym.DNP {
				d.SkippedDNP++ // a dnp part absent from the bom is the point of dnp
			} else {
				row.Kinds = append(row.Kinds, DiffMissing)
			}
		case inSch && inBOM:
			if sym.DNP {
				row.Kinds = append(row.Kinds, DiffDNP)
			}
			if hasValues && !sameValue(sym.Value, it.Value) {
				row.Kinds = append(row.Kinds, DiffValue)
			}
			if hasFootprints && !sameFootprint(sym.Footprint, it.Footprint) {
				row.Kinds = append(row.Kinds, DiffFootprint)
			}
			if row.Agrees() {
				d.Matched++
			}
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
		for _, k := range r.Kinds {
			d.Findings = append(d.Findings, Finding{
				Kind: k, Ref: r.Ref, Sch: r.schSide(k), BOM: r.bomSide(k),
			})
		}
	}
	return d
}

// schSide and bomSide describe just the field a finding is about, so a value
// mismatch shows values rather than a whole line of unrelated detail.
func (r Row) schSide(k DiffKind) string {
	switch k {
	case DiffValue:
		return dash(r.SchValue)
	case DiffFootprint:
		return dash(r.SchFootprint)
	case DiffExtra:
		return "—"
	case DiffDNP:
		return "dnp · " + r.SchBoth()
	case DiffExcluded:
		return "off the bom · " + r.SchBoth()
	}
	return r.SchBoth()
}

func (r Row) bomSide(k DiffKind) string {
	switch k {
	case DiffValue:
		return dash(r.BOMValue)
	case DiffFootprint:
		return dash(r.BOMFootprint)
	case DiffMissing:
		return "—"
	}
	return r.BOMBoth()
}

// SchBoth and BOMBoth are the two sides written the same way, for a side-by-side
// listing.
func (r Row) SchBoth() string {
	if r.SchFootprint == "" {
		return dash(r.SchValue)
	}
	return dash(r.SchValue) + " · " + r.SchFootprint
}

func (r Row) BOMBoth() string {
	if !r.InBOM {
		return "—"
	}
	out := dash(r.BOMValue)
	if r.BOMFootprint != "" {
		out += " · " + r.BOMFootprint
	}
	if r.BOMCode != "" {
		out += " · " + r.BOMCode
	}
	return out
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
	return fmt.Sprintf("%d of %d agree · %d to review · %d serious%s",
		d.Matched, d.SchCount, len(d.Findings), d.Severe(), dnp)
}
