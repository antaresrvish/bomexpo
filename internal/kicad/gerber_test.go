package kicad

import (
	"os"
	"testing"
)

func TestLoadRealGerbers(t *testing.T) {
	path := os.Getenv("BOMEXPO_GBR")
	if path == "" {
		t.Skip("set BOMEXPO_GBR")
	}
	b, err := LoadGerbers(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("board bbox: (%.2f,%.2f)-(%.2f,%.2f)  %.1f x %.1f mm  outline segs=%d",
		b.Min.X, b.Min.Y, b.Max.X, b.Max.Y, b.Width(), b.Height(), len(b.Outline))
	for _, l := range b.Layers {
		t.Logf("  %-40s role=%-8s fn=%-20s segs=%-5d pads=%-5d regions=%d",
			l.File, l.Role, l.Function, len(l.Segments), len(l.Pads), len(l.Regions))
	}
	if b.Width() <= 0 || b.Height() <= 0 {
		t.Fatalf("bad bbox")
	}
	if len(b.Outline) == 0 {
		t.Fatalf("no outline")
	}
}
