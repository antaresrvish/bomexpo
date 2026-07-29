package kicad

import (
	"path/filepath"
	"strings"
	"testing"
)

// netPCB exercises the shapes that show up in the wild: a net table, pads that
// name their net inline, a pad that carries only the net number (name lives in
// the table), the unconnected net 0, and a footprint with no nets at all.
const netPCB = `(kicad_pcb (version 20221018)
  (layers (0 "F.Cu" signal) (31 "B.Cu" signal))
  (net 0 "")
  (net 1 "GND")
  (net 2 "+3V3")
  (net 3 "SPI_SCK")
  (footprint "Capacitor_SMD:C_0402_1005Metric" (layer "F.Cu")
    (at 10 20)
    (property "Reference" "C1")
    (property "Value" "100nF")
    (pad "1" smd roundrect (at -0.5 0) (size 0.5 0.6) (net 2 "+3V3"))
    (pad "2" smd roundrect (at 0.5 0) (size 0.5 0.6) (net 1 "GND"))
  )
  (footprint "Capacitor_SMD:C_0402_1005Metric" (layer "F.Cu")
    (at 12 20)
    (property "Reference" "C2")
    (property "Value" "100nF")
    (pad "1" smd roundrect (at -0.5 0) (size 0.5 0.6) (net 2 "+3V3"))
    (pad "2" smd roundrect (at 0.5 0) (size 0.5 0.6) (net 1 "GND"))
  )
  (footprint "Resistor_SMD:R_0402_1005Metric" (layer "F.Cu")
    (at 14 20)
    (property "Reference" "R1")
    (property "Value" "10k")
    (pad "1" smd roundrect (at -0.5 0) (size 0.5 0.6) (net 3))
    (pad "2" smd roundrect (at 0.5 0) (size 0.5 0.6) (net 1 "GND"))
  )
  (footprint "TestPoint:TP" (layer "F.Cu")
    (at 20 20)
    (property "Reference" "TP1")
    (property "Value" "TestPoint")
    (pad "1" smd circle (at 0 0) (size 1 1) (net 0 ""))
  )
)
`

func loadNetPCB(t *testing.T) *Project {
	t.Helper()
	p, err := LoadProject(writeFile(t, t.TempDir(), "nets.kicad_pcb", netPCB))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParsePadNets(t *testing.T) {
	p := loadNetPCB(t)

	byRef := map[string][]string{}
	for _, c := range p.Components {
		byRef[c.Ref] = c.Nets
	}
	for ref, want := range map[string]string{
		"C1": "+3V3,GND",
		"C2": "+3V3,GND",
		// the bare "(net 3)" resolves through the net table
		"R1":  "SPI_SCK,GND",
		"TP1": "", // net 0 has no name, so nothing to record
	} {
		if got := strings.Join(byRef[ref], ","); got != want {
			t.Errorf("%s nets = %q, want %q", ref, got, want)
		}
	}
}

func TestProjectNetsAreBusiestFirst(t *testing.T) {
	nets := loadNetPCB(t).Nets()

	var got []string
	for _, n := range nets {
		got = append(got, n.Name)
	}
	// GND on three parts, +3V3 on two, SPI_SCK on one
	if want := "GND,+3V3,SPI_SCK"; strings.Join(got, ",") != want {
		t.Errorf("nets = %v, want %s", got, want)
	}
	if refs := strings.Join(nets[0].Refs, ","); refs != "C1,C2,R1" {
		t.Errorf("GND refs = %q, want C1,C2,R1", refs)
	}
	// the unnamed net 0 must not become a net of its own
	for _, n := range nets {
		if n.Name == "" {
			t.Error("an unnamed net leaked into the net list")
		}
	}
}

func TestBOMItemsCarryTheUnionOfNets(t *testing.T) {
	items := loadNetPCB(t).BOM()

	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID()] = it
	}
	caps, ok := byID["C1"]
	if !ok {
		t.Fatalf("no C1 line item in %+v", items)
	}
	if caps.Quantity != 2 {
		t.Errorf("the two 100nF caps should be one line item of 2, got %d", caps.Quantity)
	}
	// merged from both members, deduplicated and sorted
	if got := strings.Join(caps.Nets, ","); got != "+3V3,GND" {
		t.Errorf("merged nets = %q, want +3V3,GND", got)
	}
	if got := strings.Join(byID["R1"].Nets, ","); got != "GND,SPI_SCK" {
		t.Errorf("R1 nets = %q, want GND,SPI_SCK", got)
	}
	if len(byID["TP1"].Nets) != 0 {
		t.Errorf("TP1 should carry no nets, got %v", byID["TP1"].Nets)
	}
}

func TestDesignExposesNets(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(writeFile(t, dir, "nets.kicad_pcb", netPCB), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nets) != 3 {
		t.Errorf("design has %d nets, want 3", len(d.Nets))
	}

	// a CSV design has no connectivity to report
	csv, err := Load(writeFile(t, dir, "b.csv", groupedBOM), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(csv.Nets) != 0 {
		t.Errorf("a CSV design reported %d nets", len(csv.Nets))
	}
}

// A board with no net table at all must still parse — plenty of the fixtures in
// this repo look like that.
func TestBoardWithoutNetsStillLoads(t *testing.T) {
	p, err := LoadProject(writeFile(t, t.TempDir(), "mini.kicad_pcb", miniPCB))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Nets(); len(got) != 0 {
		t.Errorf("expected no nets, got %v", got)
	}
	for _, c := range p.Components {
		if len(c.Nets) != 0 {
			t.Errorf("%s picked up nets from nowhere: %v", c.Ref, c.Nets)
		}
	}
	if filepath.Base(p.PCBPath) != "mini.kicad_pcb" {
		t.Errorf("unexpected path %q", p.PCBPath)
	}
}
