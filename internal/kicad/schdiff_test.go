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

	d := DiffSchematicBOM(sc, items)
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
	if d.Matched != 1 {
		t.Errorf("Matched = %d, want 1 (R1)", d.Matched)
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
	d := DiffSchematicBOM(sc, []Item{
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
	d := DiffSchematicBOM(sc, []Item{
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
	d := DiffSchematicBOM(sc, []Item{
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
	d := DiffSchematicBOM(sc, []Item{bomItem("r1", "10k", "R_0603_1608Metric", "")})
	if len(d.Findings) != 0 {
		t.Errorf("case difference produced %+v", d.Findings)
	}
}
