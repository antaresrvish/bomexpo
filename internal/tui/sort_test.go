package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

func sortModel() Model {
	m := Model{
		items: []kicad.Item{
			{Value: "10k", Footprint: "R_0402", Bases: []string{"R1"}, Quantity: 3},
			{Value: "1uF", Footprint: "C_0402", Bases: []string{"C1"}, Quantity: 1},
			{Value: "100nF", Footprint: "C_0402", Bases: []string{"C2"}, Quantity: 5},
		},
		assigned: []*part.Part{{Code: "A"}, {Code: "B"}, {Code: "C"}},
		excluded: []bool{false, true, false},
	}
	m.w, m.h = 140, 40
	return m.reindex()
}

// order reads the values off the display rows, which is what the user sees.
func order(m Model) string {
	var out []string
	for row := 0; row < m.rows(); row++ {
		out = append(out, m.items[m.at(row)].Value)
	}
	return strings.Join(out, ",")
}

func TestReindexKeepsLoadOrderWithoutASort(t *testing.T) {
	m := sortModel()
	if got, want := order(m), "10k,1uF,100nF"; got != want {
		t.Errorf("order = %s, want %s (whatever kicad.BOM produced)", got, want)
	}
}

func TestReindexSortsTheDisplayOrder(t *testing.T) {
	m := sortModel()

	m.sort, m.sortAsc = sortQty, true
	if got, want := order(m.reindex()), "1uF,10k,100nF"; got != want {
		t.Errorf("qty asc = %s, want %s", got, want)
	}

	m.sortAsc = false
	if got, want := order(m.reindex()), "100nF,10k,1uF"; got != want {
		t.Errorf("qty desc = %s, want %s", got, want)
	}
}

// Sorting must not touch items/assigned/excluded. The old implementation
// permuted all three in place, which only worked as long as they stayed exactly
// in step.
func TestReindexLeavesTheDataAlone(t *testing.T) {
	m := sortModel()
	m.sort, m.sortAsc = sortQty, true
	got := m.reindex()

	if got.items[0].Value != "10k" || got.items[1].Value != "1uF" || got.items[2].Value != "100nF" {
		t.Errorf("items were reordered: %+v", got.items)
	}
	if got.assigned[0].Code != "A" || got.assigned[1].Code != "B" {
		t.Error("assigned was reordered")
	}
	if got.excluded[0] || !got.excluded[1] {
		t.Error("excluded was reordered")
	}
}

// A display row and its line item have to stay tied together through a sort:
// the cursor selects a row, but every action works on the line item.
func TestSelFollowsTheSortedRow(t *testing.T) {
	m := sortModel()
	m.sort, m.sortAsc = sortQty, true
	m = m.reindex()

	m.cursor = 0 // the smallest quantity, which is the 1uF at index 1
	if got := m.sel(); got != 1 {
		t.Errorf("sel = %d, want line item 1 (1uF)", got)
	}
	if !m.excluded[m.sel()] {
		t.Error("the 1uF item's excluded flag should have travelled with it")
	}
	if code := m.assigned[m.sel()].Code; code != "B" {
		t.Errorf("assigned = %s, want B", code)
	}

	// and the reverse mapping agrees
	if row := m.rowOf(1); row != 0 {
		t.Errorf("rowOf(1) = %d, want 0", row)
	}
	if row := m.rowOf(2); row != 2 { // 100nF has the largest quantity
		t.Errorf("rowOf(2) = %d, want 2", row)
	}
}

func TestAtAndSelOutOfRange(t *testing.T) {
	m := sortModel()
	for _, row := range []int{-1, 3, 99} {
		if got := m.at(row); got != -1 {
			t.Errorf("at(%d) = %d, want -1", row, got)
		}
	}
	if got := m.rowOf(42); got != -1 {
		t.Errorf("rowOf(42) = %d, want -1", got)
	}

	// an empty model must not select anything
	empty := Model{}
	if got := empty.sel(); got != -1 {
		t.Errorf("sel on an empty model = %d, want -1", got)
	}
	if got := empty.rows(); got != 0 {
		t.Errorf("rows on an empty model = %d, want 0", got)
	}
}
