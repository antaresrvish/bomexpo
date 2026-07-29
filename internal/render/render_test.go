package render

import (
	"os"
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

func TestRenderReal(t *testing.T) {
	gbr := os.Getenv("BOMEXPO_GBR")
	cpl := os.Getenv("BOMEXPO_CPL")
	if gbr == "" {
		t.Skip("set BOMEXPO_GBR")
	}
	b, err := kicad.LoadGerbers(gbr)
	if err != nil {
		t.Fatal(err)
	}
	var ps []kicad.Placement
	if cpl != "" {
		ps, _ = kicad.ImportCPL(cpl)
	}
	out := Render(b, ps, Options{W: 72, H: 22, ShowCopper: false, Zoom: 1})
	if out == "" {
		t.Fatal("empty render")
	}
	plain := strings.ReplaceAll(shape(out), "▀", "#")
	for _, line := range strings.Split(plain, "\n") {
		t.Log(line)
	}
}
