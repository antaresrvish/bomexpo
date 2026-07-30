package kicad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// schFixture writes a schematic whose symbols are given as
// "ref|value|footprint|libid|flags", flags being any of dnp, nobom, unit2.
func schFixture(t *testing.T, name string, rows ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("(kicad_sch\n\t(version 20250114)\n\t(generator \"eeschema\")\n")
	// a library definition, which carries the same tags and must not be counted
	b.WriteString("\t(lib_symbols\n\t\t(symbol \"Device:C\"\n\t\t\t(in_bom no)\n\t\t\t(dnp yes)\n\t\t)\n\t)\n")
	for _, r := range rows {
		f := strings.Split(r, "|")
		for len(f) < 5 {
			f = append(f, "")
		}
		ref, val, fp, lib, flags := f[0], f[1], f[2], f[3], f[4]
		if lib == "" {
			lib = "Device:R"
		}
		unit := 1
		if strings.Contains(flags, "unit2") {
			unit = 2
		}
		b.WriteString("\t(symbol\n\t\t(lib_id \"" + lib + "\")\n")
		b.WriteString("\t\t(unit " + itoa(unit) + ")\n")
		b.WriteString("\t\t(in_bom " + yn(!strings.Contains(flags, "nobom")) + ")\n")
		b.WriteString("\t\t(dnp " + yn(strings.Contains(flags, "dnp")) + ")\n")
		b.WriteString("\t\t(property \"Reference\" \"" + ref + "\")\n")
		b.WriteString("\t\t(property \"Value\" \"" + val + "\")\n")
		b.WriteString("\t\t(property \"Footprint\" \"" + fp + "\")\n")
		b.WriteString("\t\t(instances\n\t\t\t(project \"fix\"\n\t\t\t\t(path \"/uuid\"\n")
		b.WriteString("\t\t\t\t\t(reference \"" + ref + "\")\n\t\t\t\t\t(unit " + itoa(unit) + ")\n")
		b.WriteString("\t\t\t\t)\n\t\t\t)\n\t\t)\n\t)\n")
	}
	b.WriteString(")\n")

	dir := t.TempDir()
	p := filepath.Join(dir, name+".kicad_sch")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func itoa(n int) string { return string(rune('0' + n)) }
func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func loadFixture(t *testing.T, path string) *Schematic {
	t.Helper()
	sc, err := LoadSchematic(path)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

// A symbol library definition carries the same in_bom and dnp tags as a real
// symbol. Counting those would inflate every total.
func TestSchematicIgnoresLibrarySymbols(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "one", "R1|10k|R_0603_1608Metric"))
	if len(sc.Symbols) != 1 {
		t.Fatalf("%d symbols, want 1 — lib_symbols leaked in", len(sc.Symbols))
	}
	if s := sc.Symbols[0]; !s.InBOM || s.DNP {
		t.Errorf("flags came from the library definition: %+v", s)
	}
}

func TestSchematicReadsFlagsAndReference(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "flags",
		"R1|10k|R_0603_1608Metric",
		"R2|1k|R_0603_1608Metric|Device:R|dnp",
		"H1||MountingHole|Mechanical:MountingHole|nobom",
		"#PWR01|GND||power:GND",
	))
	if len(sc.Symbols) != 4 {
		t.Fatalf("%d symbols, want 4", len(sc.Symbols))
	}
	bom := sc.BOMSymbols()
	var refs []string
	for _, s := range bom {
		refs = append(refs, s.Ref)
	}
	if strings.Join(refs, ",") != "R1,R2" {
		t.Errorf("bom-eligible = %v, want R1,R2 (power and nobom dropped)", refs)
	}
	for _, s := range bom {
		if s.Ref == "R2" && !s.DNP {
			t.Error("R2 should be dnp")
		}
	}
}

// An opamp drawn as several gates is one BOM line, not several.
func TestSchematicCollapsesMultiUnitSymbols(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "multi",
		"U1|TL074||Amplifier_Operational:TL074",
		"U1|TL074|SOIC-14|Amplifier_Operational:TL074|unit2",
	))
	if len(sc.Symbols) != 1 {
		t.Fatalf("%d symbols, want 1 for a two-unit part", len(sc.Symbols))
	}
	// the unit carrying the footprint wins, so the land isn't lost
	if got := sc.Symbols[0].Footprint; got != "SOIC-14" {
		t.Errorf("footprint = %q, want SOIC-14", got)
	}
}

// A sub-sheet the design names but that isn't on disk must be reported, or a short
// symbol count passes for a complete one.
func TestSchematicReportsUnreadableSheets(t *testing.T) {
	root := schFixture(t, "root", "R1|10k|R_0603_1608Metric")
	body, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	withSheet := strings.Replace(string(body), "\t(symbol\n",
		"\t(sheet\n\t\t(property \"Sheetfile\" \"gone.kicad_sch\")\n\t)\n\t(symbol\n", 1)
	if err := os.WriteFile(root, []byte(withSheet), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := loadFixture(t, root)
	if len(sc.Skipped) != 1 || sc.Skipped[0] != "gone.kicad_sch" {
		t.Errorf("Skipped = %v, want the missing sheet named", sc.Skipped)
	}
}

func bomItem(refs, val, fp, lcsc string) Item {
	d := strings.Split(refs, ",")
	return Item{Designators: d, Bases: d, Value: val, Footprint: fp, LCSC: lcsc, Quantity: len(d)}
}

func kindsOf(d SchDiff) map[DiffKind][]string {
	out := map[DiffKind][]string{}
	for _, f := range d.Findings {
		out[f.Kind] = append(out[f.Kind], f.Ref)
	}
	return out
}

func TestDiffFindsEachKind(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "d",
		"R1|10k|R_0603_1608Metric",                           // agrees
		"R2|1k|R_0603_1608Metric",                            // absent from the bom
		"R3|2k|R_0603_1608Metric",                            // value differs
		"C1|100nF|C_0603_1608Metric",                         // footprint differs
		"C2|1uF|C_0603_1608Metric|Device:C|dnp",              // dnp, in the bom anyway
		"C3|1uF|C_0603_1608Metric|Device:C|dnp",              // dnp, rightly absent
		"H1||MountingHole|Mechanical:MountingHole_Pad|nobom", // excluded, in the bom anyway
	))
	items := []Item{
		bomItem("R1", "10k", "R_0603_1608Metric", "C1"),
		bomItem("R3", "3k", "R_0603_1608Metric", "C2"),
		bomItem("C1", "100nF", "C_0805_2012Metric", "C3"),
		bomItem("C2", "1uF", "C_0603_1608Metric", "C4"),
		bomItem("H1", "", "MountingHole", ""),
		bomItem("X9", "LOGO", "", ""),
	}

	d := Compare(sc, nil, items)
	got := kindsOf(d)
	for _, tc := range []struct {
		kind DiffKind
		refs string
	}{
		{DiffMissing, "R2"},
		{DiffValue, "R3"},
		{DiffFootprint, "C1"},
		{DiffDNP, "C2"},
		{DiffExcluded, "H1"},
		{DiffExtra, "X9"},
	} {
		if strings.Join(got[tc.kind], ",") != tc.refs {
			t.Errorf("%v = %v, want %s", tc.kind, got[tc.kind], tc.refs)
		}
	}
	if d.SkippedDNP != 1 {
		t.Errorf("SkippedDNP = %d, want 1 for C3", d.SkippedDNP)
	}
	// R1 agrees outright and C3 is a dnp part correctly left out — both are
	// agreement, so both count.
	if d.Matched != 2 {
		t.Errorf("Matched = %d, want 2 (R1 and the correctly absent C3)", d.Matched)
	}
	// the summary has to admit the dnp part it left out
	if !strings.Contains(d.Summary(), "1 dnp") {
		t.Errorf("summary hides the skipped dnp: %q", d.Summary())
	}
}

// The serious findings sort first, so the report opens on what would spoil an order.
func TestDiffSortsSeriousFirst(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "s",
		"R1|10k|R_0402_1005Metric",
		"R2|1k|R_0603_1608Metric",
	))
	d := Compare(sc, nil, []Item{
		bomItem("R1", "10k", "R_0805_2012Metric", ""), // footprint: not severe
	})
	if len(d.Findings) < 2 {
		t.Fatalf("want at least two findings, got %v", d.Findings)
	}
	if !d.Findings[0].Kind.Severe() {
		t.Errorf("first finding is %v, want a severe one", d.Findings[0].Kind)
	}
}

// The comparison has to understand units, or a report is nothing but false alarms.
func TestDiffAcceptsEquivalentValuesAndFootprints(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "eq",
		"C1|0.1uF|C_0603_1608Metric|Device:C",
		"C2|100nF|Capacitor_SMD:C_0402_1005Metric|Device:C",
		"Q1|2N7002|Package_TO_SOT_SMD:SOT-23",
	))
	d := Compare(sc, nil, []Item{
		bomItem("C1", "100nF", "C_0603_1608Metric", ""), // 0.1uF == 100nF
		bomItem("C2", "100nF", "0402", ""),              // bom records only the size
		bomItem("Q1", "2N7002", "SOT-23-3", ""),         // one name extends the other
	})
	if len(d.Findings) != 0 {
		t.Errorf("false alarms: %+v", d.Findings)
	}
	if d.Matched != 3 {
		t.Errorf("Matched = %d, want 3", d.Matched)
	}
}

// A BOM line with no designator (a placeholder row) must not become a phantom
// finding — real exports contain them.
func TestDiffIgnoresBOMRowsWithNoDesignator(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "blank", "R1|10k|R_0603_1608Metric"))
	d := Compare(sc, nil, []Item{
		bomItem("R1", "10k", "R_0603_1608Metric", ""),
		{Value: "0.5R", Footprint: "R_0603_1608Metric", Quantity: 0},
	})
	if len(d.Findings) != 0 {
		t.Errorf("a designator-less row produced %+v", d.Findings)
	}
}

// Designators differing only in case are the same part.
func TestDiffMatchesRefsCaseInsensitively(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "case", "R1|10k|R_0603_1608Metric"))
	d := Compare(sc, nil, []Item{bomItem("r1", "10k", "R_0603_1608Metric", "")})
	if len(d.Findings) != 0 {
		t.Errorf("case difference produced %+v", d.Findings)
	}
}

// A BOM that carries designators but no value or footprint column must not report
// every single row as a mismatch — a missing column is nothing to compare against.
func TestDiffSkipsColumnsTheBOMNeverHad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "refs-only.csv")
	if err := os.WriteFile(p, []byte("Designator,Quantity\nR1,1\nR2,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := ImportBOM(p)
	if err != nil {
		t.Fatal(err)
	}
	sc := loadFixture(t, schFixture(t, "novalue",
		"R1|10k|R_0603_1608Metric",
		"R2|1k|R_0603_1608Metric",
	))
	d := Compare(sc, nil, items)
	if len(d.Findings) != 0 {
		t.Errorf("a missing column produced %d findings: %+v", len(d.Findings), d.Findings)
	}
	if d.Matched != 2 {
		t.Errorf("Matched = %d, want 2", d.Matched)
	}
	// but it has to say which columns it could not check
	if got := strings.Join(d.NotCompared(), ","); got != "bom value,bom footprint" {
		t.Errorf("NotCompared = %q, want it to name the side too", got)
	}
}

// A BOM that does carry values still gets them compared, and still understands the
// notations a real export uses.
func TestDiffComparesValuesWhenThePresent(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "vals",
		"R1|16.2k|R_0603_1608Metric",
		"R2|36.5k|R_0603_1608Metric",
		"C1|100nF|C_0603_1608Metric|Device:C",
	))
	d := Compare(sc, nil, []Item{
		bomItem("R1", "1M", "R_0603_1608Metric", ""),    // a real mismatch
		bomItem("R2", "36k5", "R_0603_1608Metric", ""),  // rkm notation, same value
		bomItem("C1", "0.1uF", "C_0603_1608Metric", ""), // same value, other unit
	})
	if len(d.NotCompared()) != 0 {
		t.Fatalf("values were present but skipped: %v", d.NotCompared())
	}
	got := kindsOf(d)
	if strings.Join(got[DiffValue], ",") != "R1" {
		t.Errorf("value findings = %v, want just R1", got[DiffValue])
	}
	if d.Matched != 2 {
		t.Errorf("Matched = %d, want 2", d.Matched)
	}
}

func pcbItem(refs, val, fp, lcsc string) Item { return bomItem(refs, val, fp, lcsc) }

// Three sides: the schematic is the reference, and the board and the BOM are each
// reported against it by name, so you know which file to fix.
func TestCompareNamesTheSideThatDeviates(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "three",
		"R1|10k|R_0603_1608Metric", // every side agrees
		"R2|1k|R_0603_1608Metric",  // the pcb is stale
		"R3|2k|R_0603_1608Metric",  // the bom is stale
		"R4|3k|R_0603_1608Metric",  // both are stale, differently
	))
	pcb := []Item{
		pcbItem("R1", "10k", "R_0603_1608Metric", ""),
		pcbItem("R2", "999k", "R_0603_1608Metric", ""),
		pcbItem("R3", "2k", "R_0603_1608Metric", ""),
		pcbItem("R4", "111k", "R_0603_1608Metric", ""),
	}
	bom := []Item{
		bomItem("R1", "10k", "R_0603_1608Metric", ""),
		bomItem("R2", "1k", "R_0603_1608Metric", ""),
		bomItem("R3", "888k", "R_0603_1608Metric", ""),
		bomItem("R4", "222k", "R_0603_1608Metric", ""),
	}
	d := Compare(sc, pcb, bom)

	want := map[string]string{
		"R1": "agrees",
		"R2": "pcb value",
		"R3": "bom value",
		"R4": "pcb value + bom value",
	}
	for _, r := range d.Rows {
		if got := r.What(); got != want[r.Ref] {
			t.Errorf("%s: %q, want %q", r.Ref, got, want[r.Ref])
		}
	}
	if by := d.SideCounts(); by[SidePCB] != 2 || by[SideBOM] != 2 {
		t.Errorf("side counts = %v, want 2 each", by)
	}
	if d.Matched != 1 {
		t.Errorf("Matched = %d, want 1", d.Matched)
	}
}

// A cell is only highlighted on the side that deviates, so the eye lands on the one
// to read rather than on the whole row.
func TestCompareMarksOnlyTheDeviatingSide(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "sides", "R1|10k|R_0603_1608Metric"))
	d := Compare(sc,
		[]Item{pcbItem("R1", "10k", "R_0603_1608Metric", "")},
		[]Item{bomItem("R1", "47k", "R_0603_1608Metric", "")})
	if len(d.Rows) != 1 {
		t.Fatalf("%d rows, want 1", len(d.Rows))
	}
	r := d.Rows[0]
	if !r.SideOK(SidePCB) {
		t.Error("the pcb agrees but was marked")
	}
	if r.SideOK(SideBOM) {
		t.Error("the bom deviates but was not marked")
	}
	if !r.SideOK(SideSch) {
		t.Error("the schematic is the reference and can never deviate")
	}
}

// With no board given, nothing is reported against the pcb column.
func TestCompareWithoutAPCBReportsOnlyTheBOM(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "nopcb", "R1|10k|R_0603_1608Metric"))
	d := Compare(sc, nil, []Item{bomItem("R1", "47k", "R_0603_1608Metric", "")})
	if by := d.SideCounts(); by[SidePCB] != 0 {
		t.Errorf("%d findings against a pcb that wasn't given", by[SidePCB])
	}
	if d.Rows[0].Cell(SidePCB).Present {
		t.Error("the pcb cell should be empty")
	}
}

// A part on the board with no symbol behind it belongs to the pcb, not the bom.
func TestComparePinsAnExtraOnTheRightSide(t *testing.T) {
	sc := loadFixture(t, schFixture(t, "extra", "R1|10k|R_0603_1608Metric"))
	d := Compare(sc,
		[]Item{pcbItem("R1", "10k", "R_0603_1608Metric", ""), pcbItem("G***", "LOGO", "LOGO", "")},
		[]Item{bomItem("R1", "10k", "R_0603_1608Metric", "")})
	for _, f := range d.Findings {
		if f.Ref == "G***" {
			if f.Kind != DiffExtra || f.Side != SidePCB {
				t.Errorf("G***: %v on the %v, want an extra on the pcb", f.Kind, f.Side)
			}
			return
		}
	}
	t.Error("the board-only part was never reported")
}
