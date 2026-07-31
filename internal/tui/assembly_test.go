package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/part"
)

// asmModel is D1 with the diode that came back unmatched off a real order, and a
// resistor the assembler does stock.
func asmModel() Model {
	m := stockModel()
	m.items[0].Designators = []string{"D1", "D2"}
	m.items[0].Bases = []string{"D1", "D2"}
	m.items[0].Quantity = 2
	m.asm = map[string]asmRecord{
		"C1": {},                                                // no record: C42387326 on the real order
		"C2": {Found: true, Stock: 3655182, Lib: part.LibBasic}, // C23162: 0 at the shop, millions for assembly
	}
	m.asmTried = map[string]int{"C1": 1, "C2": 1}
	return m.reindex()
}

func TestAPartTheAssemblerCannotPlace(t *testing.T) {
	m := asmModel()
	if got := m.stateOf(0); got != stUnplaceable {
		t.Fatalf("D1 = %v, want stUnplaceable", got)
	}
	issues := m.issues()
	if len(issues) == 0 || issues[0].ref != "D1" {
		t.Fatalf("issues = %+v, want one for D1", issues)
	}
	if !strings.Contains(issues[0].label, "no record") {
		t.Errorf("label %q should say the assembler has no record", issues[0].label)
	}
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "1 part the assembler cannot place") {
		t.Errorf("pre-flight missed it:\n%s", out)
	}
	if !strings.Contains(stripANSI(strings.Join(m.confirmContent(70), "\n")), "no record of") {
		t.Error("the export confirmation stayed silent")
	}
}

// The shop's stock and the assembler's disagree in both directions, and the order
// draws from the assembler's.
func TestStockComesFromTheAssemblyLibrary(t *testing.T) {
	m := asmModel()
	m.check.target = 350
	m.assigned[1].Stock = 0 // nothing at the shop, millions for assembly

	if got := m.stateOf(1); got != stOK {
		t.Errorf("state = %v, want stOK: the assembler has 3.6M of it", got)
	}

	// and the other way: plenty at the shop, nothing on the assembler's shelf
	m.asm["C2"] = asmRecord{Found: true, Stock: 100}
	m.assigned[1].Stock = 4198450
	if got := m.stateOf(1); got != stShort {
		t.Fatalf("state = %v, want stShort: 100 for assembly against 1,400 needed", got)
	}
	if bad, note := m.stockShort(1); !bad || !strings.Contains(note, "assembly stock") {
		t.Errorf("note %q should name whose stock it is", note)
	}
}

// Until the library answers, the shop's number stands in and the check says so.
func TestFallsBackToShopStock(t *testing.T) {
	m := asmModel()
	m.check.target = 350
	delete(m.asm, "C2")
	m.assigned[1].Stock = 500 // needs 4 × 350 = 1400
	if got := m.stateOf(1); got != stShort {
		t.Fatalf("state = %v, want stShort from the shop's number", got)
	}
	if _, note := m.stockShort(1); !strings.Contains(note, "shop stock") {
		t.Errorf("note %q should admit it is the shop's number", note)
	}
}

func TestAssemblyLookupsRunInLanes(t *testing.T) {
	m := stockModel()
	m.asm = map[string]asmRecord{}
	m.asmTried = map[string]int{}
	m.asmFetching = map[string]bool{}
	if m.asmProvider() == nil {
		t.Skip("no source quotes assembly")
	}
	n := len(m.asmCmds())
	if n == 0 || n > asmLanes {
		t.Fatalf("asked for %d, want between 1 and %d", n, asmLanes)
	}
	if again := len(m.asmCmds()); again != 0 {
		t.Errorf("asked for %d more with the lanes busy", again)
	}
}

func TestFilterSelectsUnplaceable(t *testing.T) {
	m := asmModel()
	for _, alias := range []string{"unplaceable", "noasm"} {
		if !(filterTerm{key: "st", want: alias}).hitState(m, 0) {
			t.Errorf("st:%s missed D1", alias)
		}
	}
}
