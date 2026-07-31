package tui

import (
	"fmt"
	"testing"

	"bomexpo/internal/easyeda"
	"bomexpo/internal/kicad"
)

func mkItem(i int) kicad.Item {
	ref := fmt.Sprintf("R%d", i+1)
	return kicad.Item{Designators: []string{ref}, Bases: []string{ref}, Value: "10k",
		Footprint: "R_0402_1005Metric", LCSC: fmt.Sprintf("C%d", 1000+i), Quantity: 1}
}

// Opening Export used to ask for every part at once — 47 requests, which is what
// earned the 403. It asks a few and tops up as answers land.
func TestExportAsksAFewAtATime(t *testing.T) {
	m := New(Options{})
	m.pcbPath = "/tmp/b.kicad_pcb"
	for i := 0; i < 40; i++ {
		m.items = append(m.items, mkItem(i))
	}
	m = m.reindex()

	if n := len(m.fitCmds()); n != padLanes {
		t.Fatalf("first ask = %d requests, want %d lanes", n, padLanes)
	}
	if n := len(m.fitCmds()); n != 0 {
		t.Errorf("asked %d more with every lane busy", n)
	}

	// one answer frees exactly one lane
	mm, _ := m.route(footprintDoneMsg{code: m.items[0].LCSC,
		fp: easyeda.Footprint{Code: m.items[0].LCSC, Lands: pads(2)}})
	m = mm.(Model)
	if n := len(m.fitCmds()); n != 1 {
		t.Errorf("one answer freed %d lanes, want 1", n)
	}
}

// and the whole board still gets covered, a few at a time, because each answer tops
// the lanes back up inside Update
func TestThePipelineCoversEveryPart(t *testing.T) {
	m := New(Options{})
	m.pcbPath = "/tmp/b.kicad_pcb"
	for i := 0; i < 40; i++ {
		m.items = append(m.items, mkItem(i))
	}
	m = m.reindex()
	m.padFill = true
	m.fitCmds() // opens the first lanes

	asked := map[string]bool{}
	peak := 0
	for guard := 0; len(m.edaFetching) > 0 && guard < 500; guard++ {
		if n := len(m.edaFetching); n > peak {
			peak = n
		}
		var code string
		for c := range m.edaFetching {
			code = c
			break
		}
		asked[code] = true
		mm, _ := m.Update(footprintDoneMsg{code: code,
			fp: easyeda.Footprint{Code: code, Lands: pads(2)}})
		m = mm.(Model)
	}
	if len(asked) != 40 {
		t.Errorf("covered %d of 40 parts", len(asked))
	}
	if peak > padLanes {
		t.Errorf("had %d requests out at once, the limit is %d", peak, padLanes)
	}
}
