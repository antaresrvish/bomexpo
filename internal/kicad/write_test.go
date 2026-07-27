package kicad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const miniPCB = `(kicad_pcb
	(version 20241229)
	(footprint "R_0402"
		(property "Reference" "R1"
			(at 0 0 0)
		)
		(property "Value" "10k"
			(at 0 0 0)
		)
		(property "LCSC" "C1"
			(at 0 0 0)
		)
	)
	(footprint "C_0402"
		(property "Reference" "C7"
			(at 0 0 0)
		)
		(property "Value" "100nF"
			(at 0 0 0)
		)
	)
)
`

func TestWriteLCSC(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	if err := os.WriteFile(pcb, []byte(miniPCB), 0644); err != nil {
		t.Fatal(err)
	}
	upd, ins, err := WriteLCSC(pcb, map[string]string{"R1": "C111", "C7": "C222", "Q9": "C999"})
	if err != nil {
		t.Fatal(err)
	}
	if upd != 1 || ins != 1 {
		t.Fatalf("updated=%d inserted=%d, want 1/1", upd, ins)
	}

	out, _ := os.ReadFile(pcb)
	s := string(out)
	if !strings.Contains(s, `(property "LCSC" "C111"`) {
		t.Error("R1 LCSC not updated to C111")
	}
	if strings.Contains(s, `"C1"`) {
		t.Error(`old value "C1" still present`)
	}
	if !strings.Contains(s, "\n\t\t(property \"LCSC\" \"C222\")") {
		t.Error("C7 LCSC not inserted as a minimal property line")
	}

	root, err := parseSexp(s)
	if err != nil {
		t.Fatalf("result no longer parses: %v", err)
	}
	got := map[string]string{}
	for _, n := range root.kids {
		if n.head() != "footprint" {
			continue
		}
		if lc := lcscProp(n); lc != nil {
			got[propValue(n, "reference")] = lc.kids[2].atom
		}
	}
	if got["R1"] != "C111" || got["C7"] != "C222" {
		t.Errorf("read-back codes = %v, want R1=C111 C7=C222", got)
	}
}

func TestWriteLCSCNoChange(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	os.WriteFile(pcb, []byte(miniPCB), 0644)
	before, _ := os.ReadFile(pcb)
	upd, ins, err := WriteLCSC(pcb, map[string]string{"R1": "C1"})
	if err != nil || upd != 0 || ins != 0 {
		t.Fatalf("want no-op, got upd=%d ins=%d err=%v", upd, ins, err)
	}
	after, _ := os.ReadFile(pcb)
	if string(before) != string(after) {
		t.Error("file changed on a no-op save")
	}
}
