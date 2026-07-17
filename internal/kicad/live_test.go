package kicad

import (
	"os"
	"testing"
)

func TestParseRealBOM(t *testing.T) {
	path := os.Getenv("BOMEXPO_BOM")
	if path == "" {
		t.Skip("set BOMEXPO_BOM")
	}
	items, err := ImportBOM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("parsed %d line items", len(items))
	for _, it := range items[:min(6, len(items))] {
		t.Logf("%-6s val=%-10s fp=%-12s qty=%-4d perBoard=%d lcsc=%q", it.ID(), it.Value, it.Footprint, it.Quantity, it.PerBoard(), it.LCSC)
	}
}

func TestParseRealCPL(t *testing.T) {
	path := os.Getenv("BOMEXPO_CPL")
	if path == "" {
		t.Skip("set BOMEXPO_CPL")
	}
	ps, err := ImportCPL(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("parsed %d placements", len(ps))
	for _, p := range ps[:min(4, len(ps))] {
		t.Logf("%-8s x=%.3f y=%.3f rot=%g layer=%s val=%q pkg=%q", p.Designator, p.X, p.Y, p.Rotation, p.Layer, p.Value, p.Package)
	}
}
