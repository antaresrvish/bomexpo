package tui

import (
	"os"
	"testing"
)

func TestRenderBoard(t *testing.T) {
	proj := os.Getenv("BOMEXPO_PROJ")
	if proj == "" || testing.Short() {
		t.Skip("set BOMEXPO_PROJ")
	}
	if kicadCLI() == "" {
		t.Skip("kicad-cli not found")
	}
	p1, err := renderBoard(proj, "top")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p1); err != nil || fi.Size() == 0 {
		t.Fatalf("no render output: %v", err)
	}
	p2, err := renderBoard(proj, "top")
	if err != nil || p2 != p1 {
		t.Fatalf("cache miss: %q vs %q (%v)", p1, p2, err)
	}
	t.Logf("rendered + cached: %s", p1)
}
