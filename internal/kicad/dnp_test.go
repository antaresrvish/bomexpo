package kicad

import (
	"os"
	"path/filepath"
	"testing"
)

const dnpPCB = `(kicad_pcb
	(version 20241229)
	(footprint "R_0402"
		(layer "F.Cu")
		(attr smd)
		(property "Reference" "R1" (at 0 0 0))
		(property "Value" "10k" (at 0 0 0))
		(pad "1" smd roundrect (at 0 0) (size 0.5 0.5))
	)
	(footprint "R_0402"
		(layer "F.Cu")
		(attr smd dnp)
		(property "Reference" "R2" (at 0 0 0))
		(property "Value" "10k" (at 0 0 0))
		(pad "1" smd roundrect (at 0 0) (size 0.5 0.5))
	)
)
`

func TestParseDNPAndBOMSplit(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	if err := os.WriteFile(pcb, []byte(dnpPCB), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProject(pcb)
	if err != nil {
		t.Fatal(err)
	}

	dnp := map[string]bool{}
	for _, c := range p.Components {
		dnp[c.Ref] = c.DNP
	}
	if dnp["R1"] {
		t.Error("R1 should not be DNP")
	}
	if !dnp["R2"] {
		t.Error("R2 should be DNP (attr smd dnp)")
	}

	// same value+footprint but different DNP flag → two separate line items
	items := p.BOM()
	if len(items) != 2 {
		t.Fatalf("want 2 line items (populated + DNP), got %d", len(items))
	}
	var populated, doNotPop *Item
	for i := range items {
		if items[i].DNP {
			doNotPop = &items[i]
		} else {
			populated = &items[i]
		}
	}
	if populated == nil || doNotPop == nil {
		t.Fatalf("expected one populated and one DNP item, got %+v", items)
	}
	if populated.Designators[0] != "R1" || doNotPop.Designators[0] != "R2" {
		t.Errorf("grouping mixed up: populated=%v dnp=%v", populated.Designators, doNotPop.Designators)
	}
}
