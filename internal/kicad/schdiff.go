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

// SchDiff is the whole comparison.
type SchDiff struct {
	SchPath, BOMPath string
	Findings         []Finding
	SchCount         int // bom-eligible symbols
	BOMCount         int // designators in the external bom
	Matched          int // designators both agree on completely
	// SkippedDNP counts do-not-populate symbols the BOM rightly leaves out. They
	// are not findings, but without them the schematic and BOM totals look like
	// they disagree for no reason.
	SkippedDNP int
	// unread carries the sub-sheets the schematic named but could not be read, so a
	// comparison over half a design can't pass for a whole one.
	unread []string
}

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
	for _, s := range sc.Symbols {
		if s.Bommable() {
			bySch[upRef(s.Ref)] = s
			d.SchCount++
		}
	}
	// Symbols the designer took off the BOM, kept aside so their presence in the
	// external BOM can still be reported.
	offBOM := map[string]Symbol{}
	for _, s := range sc.Symbols {
		if !s.Bommable() && s.Ref != "" {
			offBOM[upRef(s.Ref)] = s
		}
	}

	byBOM := map[string]Item{}
	for _, it := range items {
		for _, ref := range it.Designators {
			r := upRef(ref)
			if r == "" {
				continue
			}
			byBOM[r] = it
			d.BOMCount++
		}
	}

	for ref, s := range bySch {
		it, inBOM := byBOM[ref]
		if !inBOM {
			if s.DNP {
				d.SkippedDNP++ // a dnp part absent from the bom is the point of dnp
				continue
			}
			d.Findings = append(d.Findings, Finding{
				Kind: DiffMissing, Ref: s.Ref,
				Sch: describe(s), BOM: "—",
			})
			continue
		}
		clean := true
		if s.DNP {
			d.Findings = append(d.Findings, Finding{
				Kind: DiffDNP, Ref: s.Ref,
				Sch: "dnp · " + describe(s), BOM: describeItem(it),
			})
			clean = false
		}
		if !sameValue(s.Value, it.Value) {
			d.Findings = append(d.Findings, Finding{
				Kind: DiffValue, Ref: s.Ref, Sch: dash(s.Value), BOM: dash(it.Value),
			})
			clean = false
		}
		if !sameFootprint(s.Footprint, it.Footprint) {
			d.Findings = append(d.Findings, Finding{
				Kind: DiffFootprint, Ref: s.Ref, Sch: dash(s.Footprint), BOM: dash(it.Footprint),
			})
			clean = false
		}
		if clean {
			d.Matched++
		}
	}

	for ref, it := range byBOM {
		if _, ok := bySch[ref]; ok {
			continue
		}
		if s, off := offBOM[ref]; off {
			d.Findings = append(d.Findings, Finding{
				Kind: DiffExcluded, Ref: s.Ref,
				Sch: "excluded from bom · " + describe(s), BOM: describeItem(it),
			})
			continue
		}
		d.Findings = append(d.Findings, Finding{
			Kind: DiffExtra, Ref: ref, Sch: "—", BOM: describeItem(it),
		})
	}

	// Severe first, then by designator, so the report opens on what matters.
	sort.SliceStable(d.Findings, func(i, j int) bool {
		a, b := d.Findings[i], d.Findings[j]
		if a.Kind.Severe() != b.Kind.Severe() {
			return a.Kind.Severe()
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return refLess(a.Ref, b.Ref)
	})
	return d
}

func upRef(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func describe(s Symbol) string {
	if s.Footprint == "" {
		return dash(s.Value)
	}
	return dash(s.Value) + " · " + s.Footprint
}

func describeItem(it Item) string {
	out := dash(it.Value)
	if it.Footprint != "" {
		out += " · " + it.Footprint
	}
	if it.LCSC != "" {
		out += " · " + it.LCSC
	}
	return out
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
