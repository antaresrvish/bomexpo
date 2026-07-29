package kicad

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// a JLCPCB-shaped export: one row per line item, designators already collapsed
const groupedBOM = `Comment,Designator,Footprint,Quantity,LCSC Part #
100nF,"C1,C2,C3",C_0402_1005Metric,3,C1525
10k,"R1,R2",R_0402_1005Metric,2,C25744
STM32F103CBT6,U1,LQFP-48,1,C8304
`

// a per-component export: one row per part, which is worth merging
const flatBOM = `Reference,Value,Footprint,LCSC
C1,100nF,C_0402_1005Metric,C1525
C2,100nF,C_0402_1005Metric,
C3,100nF,C_0402_1005Metric,
R1,10k,R_0402_1005Metric,C25744
U1,STM32F103CBT6,LQFP-48,C8304
`

const cplCSV = `Designator,Mid X,Mid Y,Layer,Rotation
C1,10.0,20.0,top,0
C2,12.5,20.0,top,90
C3,15.0,20.0,bottom,180
R1,10.0,25.0,top,0
U1,30.0,40.0,top,270
`

func TestLoadGroupedBOM(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "drone-bom.csv", groupedBOM)

	d, err := Load(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.FromBoard() || d.PCBPath != "" {
		t.Error("a CSV design must not claim a board")
	}
	if d.Name != "drone-bom" {
		t.Errorf("name = %q, want drone-bom", d.Name)
	}
	if d.BOMPath != p {
		t.Errorf("BOMPath = %q, want %q", d.BOMPath, p)
	}
	if len(d.Items) != 3 {
		t.Fatalf("got %d line items, want 3: %+v", len(d.Items), d.Items)
	}
	// pre-grouped rows are sorted but not re-merged
	if got := d.Items[0].ID(); got != "C1" {
		t.Errorf("first item = %s, want C1 (sorted by reference)", got)
	}
	if q := d.Items[0].Quantity; q != 3 {
		t.Errorf("C1 quantity = %d, want 3", q)
	}
	if d.Items[0].LCSC != "C1525" {
		t.Errorf("LCSC = %q, want C1525", d.Items[0].LCSC)
	}
}

func TestLoadFlatBOMMergesRows(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "flat.csv", flatBOM)

	d, err := Load(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 3 {
		t.Fatalf("got %d line items, want 3 after merging: %+v", len(d.Items), d.Items)
	}
	caps := d.Items[0]
	if caps.ID() != "C1" || caps.Quantity != 3 {
		t.Errorf("caps = %s ×%d, want C1 ×3", caps.ID(), caps.Quantity)
	}
	if len(caps.Designators) != 3 {
		t.Errorf("designators = %v, want three", caps.Designators)
	}
	// the first non-empty code wins, as on the board path
	if caps.LCSC != "C1525" {
		t.Errorf("merged LCSC = %q, want C1525", caps.LCSC)
	}
}

func TestLoadBOMFindsSiblingCPL(t *testing.T) {
	dir := t.TempDir()
	bom := writeFile(t, dir, "drone-bom.csv", groupedBOM)
	writeFile(t, dir, "drone-cpl.csv", cplCSV)

	d, err := Load(bom, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.CPLPath == "" {
		t.Fatal("the sibling drone-cpl.csv should have been picked up")
	}
	if len(d.Placements) != 5 {
		t.Fatalf("got %d placements, want 5", len(d.Placements))
	}
	if d.Board == nil {
		t.Fatal("placements should give the board view a frame")
	}
	// framed on the placements, padded so edge parts aren't clipped
	if d.Board.Min.X != 8 || d.Board.Max.X != 32 {
		t.Errorf("x frame = %g..%g, want 8..32", d.Board.Min.X, d.Board.Max.X)
	}
	if d.Board.Min.Y != 18 || d.Board.Max.Y != 42 {
		t.Errorf("y frame = %g..%g, want 18..42", d.Board.Min.Y, d.Board.Max.Y)
	}
	// the placement extent is not the board size, so don't claim one
	if d.BoardW != 0 || d.BoardH != 0 {
		t.Errorf("board size = %gx%g, want 0x0 for a CSV design", d.BoardW, d.BoardH)
	}
}

func TestLoadBOMWithoutCPL(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(writeFile(t, dir, "solo.csv", groupedBOM), "")
	if err != nil {
		t.Fatal(err)
	}
	if d.CPLPath != "" || len(d.Placements) != 0 || d.Board != nil {
		t.Errorf("a lone BOM should have no placements: %+v", d)
	}
}

func TestExplicitCPLErrorsButFoundOneIsQuiet(t *testing.T) {
	dir := t.TempDir()
	bom := writeFile(t, dir, "b.csv", groupedBOM)
	bad := writeFile(t, dir, "junk.csv", "nothing,useful\n1,2\n")

	// named on the command line: the user needs to hear it failed
	if _, err := Load(bom, bad); err == nil {
		t.Error("an explicit placement file that can't be read should error")
	}

	// found on our own: a bonus, so stay quiet and carry on
	dir2 := t.TempDir()
	bom2 := writeFile(t, dir2, "x-bom.csv", groupedBOM)
	writeFile(t, dir2, "x-cpl.csv", "nothing,useful\n1,2\n")
	d, err := Load(bom2, "")
	if err != nil {
		t.Fatalf("an unreadable sibling should not fail the load: %v", err)
	}
	if len(d.Placements) != 0 {
		t.Error("no placements should have come from the junk sibling")
	}
}

func TestIsBOMFile(t *testing.T) {
	for path, want := range map[string]bool{
		"bom.csv": true, "BOM.CSV": true, "x.tsv": true,
		"board.kicad_pcb": false, "proj.kicad_pro": false, "dir": false,
	} {
		if got := IsBOMFile(path); got != want {
			t.Errorf("IsBOMFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestLoadStillOpensABoard(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "mini.kicad_pcb", miniPCB)

	d, err := Load(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if !d.FromBoard() || d.PCBPath == "" {
		t.Error("a .kicad_pcb design should report a board")
	}
	if len(d.Items) == 0 || len(d.Placements) == 0 {
		t.Errorf("board load produced %d items and %d placements", len(d.Items), len(d.Placements))
	}
	if d.BOMPath != "" || d.CPLPath != "" {
		t.Error("a board design should not claim CSV sources")
	}
}
