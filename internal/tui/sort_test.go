package tui

import (
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

func TestSortedKeepsArraysAligned(t *testing.T) {
	m := Model{
		items: []kicad.Item{
			{Value: "10k", Footprint: "R_0402", Bases: []string{"R1"}, Quantity: 3},
			{Value: "1uF", Footprint: "C_0402", Bases: []string{"C1"}, Quantity: 1},
			{Value: "100nF", Footprint: "C_0402", Bases: []string{"C2"}, Quantity: 5},
		},
		assigned: []*part.Part{{Code: "A"}, {Code: "B"}, {Code: "C"}},
		excluded: []bool{false, true, false},
	}

	m.sort, m.sortAsc = sortQty, true
	got := m.sorted()
	if got.items[0].Value != "1uF" || got.items[1].Value != "10k" || got.items[2].Value != "100nF" {
		t.Fatalf("qty asc wrong: %s %s %s", got.items[0].Value, got.items[1].Value, got.items[2].Value)
	}
	// the 1uF item carried assigned "B" and excluded=true; they must move with it
	if got.assigned[0].Code != "B" || !got.excluded[0] {
		t.Errorf("parallel arrays misaligned: code=%s excl=%v", got.assigned[0].Code, got.excluded[0])
	}

	m.sortAsc = false
	if got := m.sorted(); got.items[0].Value != "100nF" {
		t.Errorf("qty desc wrong, first=%s", got.items[0].Value)
	}
}
