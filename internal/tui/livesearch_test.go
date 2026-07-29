package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/part"
	"bomexpo/internal/value"
)

func TestLiveResistorSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	it := kicad.Item{Value: "4.7k", Footprint: "R_0603_1608Metric", Bases: []string{"R1"}}
	kw := searchKeyword(it)
	if kw != "4.7kΩ" {
		t.Fatalf("keyword = %q, want 4.7kΩ", kw)
	}
	res, err := lcsc.New().Provider().Search(part.Query{Keyword: kw, Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	s := searchState{results: res.Items, kind: value.Resistance, typeOnly: true, pkg: "0603", pkgOnly: true}
	f := s.filtered()
	t.Logf("%q → %d raw, %d after 0603+resistor filter", kw, len(res.Items), len(f))
	if len(f) < 3 {
		t.Fatalf("expected several 4.7k 0603 resistors, got %d", len(f))
	}
	target, _ := value.Parse("4.7k")
	wrong := 0
	for _, p := range f {
		if !strings.EqualFold(p.Package, "0603") {
			t.Errorf("wrong package leaked: %s (%s)", p.Package, p.Code)
		}
		if v, ok := value.ExtractValue(p.Description()); ok && !value.Equal(v, target) {
			wrong++
			t.Logf("  non-4.7k value: %s", p.Description())
		}
	}
	if wrong > len(f)/2 {
		t.Fatalf("%d/%d results are not 4.7k", wrong, len(f))
	}
}
