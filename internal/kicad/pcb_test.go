package kicad

import (
	"os"
	"testing"
)

func TestLoadProject(t *testing.T) {
	path := os.Getenv("BOMEXPO_PROJ")
	if path == "" {
		t.Skip("set BOMEXPO_PROJ")
	}
	p, err := LoadProject(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("project %q: %d components, %d outline segs, bbox %.1fx%.1f mm",
		p.Name, len(p.Components), len(p.Outline), p.Max.X-p.Min.X, p.Max.Y-p.Min.Y)

	items := p.BOM()
	t.Logf("BOM: %d line items", len(items))
	for _, it := range items[:min(8, len(items))] {
		t.Logf("  %-6s %-10s %-22s qty=%d lcsc=%q", it.ID(), it.Value, it.Footprint, it.Quantity, it.LCSC)
	}
	pl := p.Placements()
	if len(pl) != len(p.Components) {
		t.Fatalf("placements %d != components %d", len(pl), len(p.Components))
	}
	if len(p.Outline) == 0 {
		t.Error("no outline extracted")
	}
	if p.Board().Empty() {
		t.Error("board reports empty")
	}
}
