package tui

import (
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
)

func TestExclude(t *testing.T) {
	m := New("")
	m.items = []kicad.Item{
		{Bases: []string{"H1"}, Value: "MountingHole"},
		{Bases: []string{"R1"}, Value: "10k", LCSC: "C1"},
	}
	m.assigned = make([]*lcsc.Part, 2)
	m.excluded = make([]bool, 2)

	if _, wn := m.counts(); wn != 1 {
		t.Fatalf("expected 1 warning (unassigned H1), got %d", wn)
	}
	if len(m.issues()) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(m.issues()))
	}

	m.excluded[0] = true
	if m.stateOf(0) != stExcluded {
		t.Fatal("H1 not marked excluded")
	}
	if _, wn := m.counts(); wn != 0 {
		t.Fatalf("excluding H1 should clear the warning, got %d", wn)
	}
	if m.activeCount() != 1 || m.excludedCount() != 1 {
		t.Fatalf("active=%d excluded=%d", m.activeCount(), m.excludedCount())
	}
	if len(m.issues()) != 0 {
		t.Fatalf("excluded item should not appear in issues, got %d", len(m.issues()))
	}
}
