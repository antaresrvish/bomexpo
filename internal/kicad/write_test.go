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
		(layer "F.Cu")
		(attr smd)
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
		(layer "F.Cu")
		(attr smd)
		(property "Reference" "C7"
			(at 0 0 0)
		)
		(property "Value" "100nF"
			(at 0 0 0)
		)
	)
)
`

func TestWriteBackCodes(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	if err := os.WriteFile(pcb, []byte(miniPCB), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := WriteBack(pcb, map[string]string{"R1": "C111", "C7": "C222", "Q9": "C999"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.CodesUpdated != 1 || res.CodesInserted != 1 {
		t.Fatalf("updated=%d inserted=%d, want 1/1", res.CodesUpdated, res.CodesInserted)
	}
	s := readFile(t, pcb)
	if !strings.Contains(s, `(property "LCSC" "C111"`) {
		t.Error("R1 LCSC not updated to C111")
	}
	if !strings.Contains(s, "\n\t\t(property \"LCSC\" \"C222\")") {
		t.Error("C7 LCSC not inserted")
	}
	if _, err := parseSexp(s); err != nil {
		t.Fatalf("result no longer parses: %v", err)
	}
}

func TestWriteBackNoChange(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	os.WriteFile(pcb, []byte(miniPCB), 0644)
	before := readFile(t, pcb)
	res, err := WriteBack(pcb, map[string]string{"R1": "C1"}, nil, nil)
	if err != nil || res != (WriteResult{}) {
		t.Fatalf("want no-op, got %+v err=%v", res, err)
	}
	if readFile(t, pcb) != before {
		t.Error("file changed on a no-op save")
	}
}

func TestWriteBackExclude(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	os.WriteFile(pcb, []byte(miniPCB), 0644)

	res, err := WriteBack(pcb, nil, map[string]bool{"R1": true}, nil)
	if err != nil || res.Excluded != 1 {
		t.Fatalf("exclude: excluded=%d err=%v", res.Excluded, err)
	}
	if !strings.Contains(readFile(t, pcb), "(attr smd exclude_from_bom exclude_from_pos_files)") {
		t.Fatalf("attr not rebuilt with exclude tokens:\n%s", readFile(t, pcb))
	}
	// read-back through the loader
	p, err := LoadProject(pcb)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Components {
		if c.Ref == "R1" && !c.ExcludeBOM {
			t.Error("R1 should read back as ExcludeBOM")
		}
		if c.Ref == "C7" && c.ExcludeBOM {
			t.Error("C7 should not be excluded")
		}
	}

	// idempotent
	if res, _ := WriteBack(pcb, nil, map[string]bool{"R1": true}, nil); res != (WriteResult{}) {
		t.Errorf("second exclude should be a no-op, got %+v", res)
	}

	// re-include drops the tokens
	res, err = WriteBack(pcb, nil, map[string]bool{"R1": false}, nil)
	if err != nil || res.Included != 1 {
		t.Fatalf("re-include: included=%d err=%v", res.Included, err)
	}
	s := readFile(t, pcb)
	if strings.Contains(s, "exclude_from_bom") {
		t.Errorf("exclude tokens not removed:\n%s", s)
	}
	if !strings.Contains(s, "(attr smd)") {
		t.Errorf("attr not restored to (attr smd):\n%s", s)
	}
}

func TestWriteBackRotOverride(t *testing.T) {
	pcb := filepath.Join(t.TempDir(), "b.kicad_pcb")
	os.WriteFile(pcb, []byte(miniPCB), 0644)
	set := func(v int) *int { return &v }

	res, err := WriteBack(pcb, nil, nil, map[string]*int{"C7": set(90)})
	if err != nil || res.RotSet != 1 {
		t.Fatalf("set: RotSet=%d err=%v", res.RotSet, err)
	}
	if !strings.Contains(readFile(t, pcb), `(property "bomexpo_rot" "90")`) {
		t.Fatalf("override not written:\n%s", readFile(t, pcb))
	}
	p, _ := LoadProject(pcb)
	for _, c := range p.Components {
		if c.Ref == "C7" && (!c.HasRotOverride || c.RotOverride != 90) {
			t.Errorf("C7 override not read back: has=%v val=%d", c.HasRotOverride, c.RotOverride)
		}
	}
	if res, _ := WriteBack(pcb, nil, nil, map[string]*int{"C7": set(180)}); res.RotSet != 1 {
		t.Errorf("update: RotSet=%d", res.RotSet)
	}
	if !strings.Contains(readFile(t, pcb), `(property "bomexpo_rot" "180")`) {
		t.Error("override not updated to 180")
	}
	res, _ = WriteBack(pcb, nil, nil, map[string]*int{"C7": nil})
	if res.RotCleared != 1 {
		t.Errorf("clear: RotCleared=%d", res.RotCleared)
	}
	s := readFile(t, pcb)
	if strings.Contains(s, "bomexpo_rot") {
		t.Errorf("override not removed:\n%s", s)
	}
	if _, err := parseSexp(s); err != nil {
		t.Fatalf("no longer parses after removal: %v", err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
