package tui

import (
	"math"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

func TestCostAtRespectsMOQ(t *testing.T) {
	m := Model{
		items: []kicad.Item{
			{Bases: []string{"R1"}, Value: "10k", Quantity: 5},   // need 5, MOQ 100 → buy 100
			{Bases: []string{"C1"}, Value: "1uF", Quantity: 200}, // need 200, MOQ 50 → buy 200
		},
		excluded: []bool{false, false},
		assigned: []*part.Part{
			{MinBuy: 100, Prices: []part.Price{{Ladder: 1, USD: 0.01}}},
			{MinBuy: 50, Prices: []part.Price{{Ladder: 1, USD: 0.02}}},
		},
	}
	// 100*0.01 + 200*0.02 = 1.00 + 4.00
	tot, complete := m.costAt(1)
	if !complete || math.Abs(tot-5.0) > 1e-6 {
		t.Errorf("costAt(1) = %.4f complete=%v, want 5.00 true", tot, complete)
	}

	// only R1 over-buys: (100-5)*0.01 = 0.95
	n, extra := m.moqImpact(1)
	if n != 1 || math.Abs(extra-0.95) > 1e-6 {
		t.Errorf("moqImpact(1) = %d parts, $%.4f extra; want 1, $0.95", n, extra)
	}

	// at 20 boards R1 needs 100 (== MOQ) → no over-buy
	if n, _ := m.moqImpact(20); n != 0 {
		t.Errorf("moqImpact(20) parts = %d, want 0", n)
	}
}
