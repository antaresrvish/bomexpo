package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

// stockModel is FPC1 off the 350-board order — 338 in stock against 350 needed — plus
// a capacitor with room to spare and one that only just covers.
func stockModel() Model {
	m := New(Options{})
	m.pcbPath = "/tmp/b.kicad_pcb"
	m.items = []kicad.Item{
		{Designators: []string{"FPC1"}, Bases: []string{"FPC1"}, Value: "conn", Footprint: "FPC-8", LCSC: "C1", Quantity: 1},
		{Designators: []string{"C1"}, Bases: []string{"C1"}, Value: "1uF", Footprint: "C_0402_1005Metric", LCSC: "C2", Quantity: 4},
		{Designators: []string{"Y2"}, Bases: []string{"Y2"}, Value: "12MHz", Footprint: "Crystal", LCSC: "C3", Quantity: 1},
	}
	price := []part.Price{{Ladder: 1, USD: 0.1}}
	m.assigned = []*part.Part{
		{Source: "lcsc", Code: "C1", Stock: 338, MinBuy: 1, Prices: price},
		{Source: "lcsc", Code: "C2", Stock: 4198450, MinBuy: 1, Prices: price},
		{Source: "lcsc", Code: "C3", Stock: 449, MinBuy: 1, Prices: price},
	}
	m.excluded = make([]bool, 3)
	m.w, m.h = 132, 46
	return m.reindex()
}

// Without a target every part covers one board, which is the old behaviour.
func TestNoTargetJudgesOneBoard(t *testing.T) {
	m := stockModel()
	for i := range m.items {
		if got := m.stateOf(i); got != stOK {
			t.Errorf("item %d = %v, want stOK against a single board", i, got)
		}
	}
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "press q") {
		t.Errorf("pre-flight let one board pass for a real run:\n%s", out)
	}
}

func TestShortStockAtTheRealOrderSize(t *testing.T) {
	m := stockModel()
	m.check.target = 350

	if got := m.stateOf(0); got != stShort {
		t.Fatalf("FPC1 = %v, want stShort: 338 in stock against 350 needed", got)
	}
	if got := m.stateOf(1); got != stOK {
		t.Errorf("the capacitor with 4.2M in stock = %v, want stOK", got)
	}
	if got := m.stateOf(2); got != stOK {
		t.Errorf("Y2 covers 350 with 449, so it is not short: got %v", got)
	}

	issues := m.issues()
	if len(issues) != 1 || issues[0].ref != "FPC1" {
		t.Fatalf("issues = %+v, want one for FPC1", issues)
	}
	if !strings.Contains(issues[0].label, "338") || !strings.Contains(issues[0].label, "350") {
		t.Errorf("the label should give both numbers, got %q", issues[0].label)
	}

	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "1 part cannot fill an order of 350 boards") {
		t.Errorf("pre-flight missed it:\n%s", out)
	}
	// Y2 covers, but at 1.28× it is worth naming
	if !strings.Contains(out, "only just covers it") || !strings.Contains(out, "Y2") {
		t.Errorf("pre-flight said nothing about the thin one:\n%s", out)
	}
	if !strings.Contains(stripANSI(strings.Join(m.confirmContent(60), "\n")), "stock cannot fill this order") {
		t.Error("the export confirmation stayed silent about short stock")
	}
}

// Four per board multiplies: a part with plenty for 350 boards can still fall short.
func TestQuantityPerBoardMultiplies(t *testing.T) {
	m := stockModel()
	m.check.target = 350
	m.assigned[1].Stock = 1000 // needs 4 × 350 = 1400
	if got := m.stateOf(1); got != stShort {
		t.Errorf("state = %v, want stShort: 1000 in stock against 1400 needed", got)
	}
}

// The minimum order is what has to be in stock, not what you need.
func TestMinimumOrderCounts(t *testing.T) {
	m := stockModel()
	m.check.target = 10
	m.assigned[0].Stock = 30
	m.assigned[0].MinBuy = 50
	if got := m.stateOf(0); got != stShort {
		t.Errorf("state = %v, want stShort: a 50-piece minimum against 30 in stock", got)
	}
}

func TestFilterSelectsShortStock(t *testing.T) {
	m := stockModel()
	m.check.target = 350
	for _, alias := range []string{"short", "stock"} {
		if !(filterTerm{key: "st", want: alias}).hitState(m, 0) {
			t.Errorf("st:%s missed FPC1", alias)
		}
		if (filterTerm{key: "st", want: alias}).hitState(m, 1) {
			t.Errorf("st:%s matched a part with stock to spare", alias)
		}
	}
}

// Out of stock and short of stock are different sentences.
func TestZeroStockStaysOutOfStock(t *testing.T) {
	m := stockModel()
	m.check.target = 350
	m.assigned[0].Stock = 0
	if got := m.stateOf(0); got != stOutOfStock {
		t.Errorf("state = %v, want stOutOfStock", got)
	}
}
