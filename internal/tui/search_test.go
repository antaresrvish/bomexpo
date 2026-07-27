package tui

import (
	"testing"

	"bomexpo/internal/lcsc"
	"bomexpo/internal/value"
)

func TestSearchTypeFilter(t *testing.T) {
	s := searchState{
		kind:     value.Resistance,
		typeOnly: true,
		results: []lcsc.Part{
			{Code: "C1", IntroEn: "4.7kΩ ±1% 100mW 0402 Chip Resistor"},
			{Code: "C2", IntroEn: "1uF ±10% 16V 0402 Ceramic Capacitor"},
			{Code: "C3", IntroEn: "4.7uF ±10% 6.3V Ceramic Capacitor X5R 0402"},
			{Code: "C4", IntroEn: "10kΩ ±1% 0402 Thick Film Resistor"},
		},
	}
	f := s.filtered()
	if len(f) != 2 {
		t.Fatalf("type filter should keep 2 resistors, got %d", len(f))
	}
	for _, p := range f {
		if v, ok := value.ExtractValue(p.Description()); ok && v.Kind == value.Capacitance {
			t.Errorf("capacitor leaked into resistor search: %q", p.Description())
		}
	}

	s.typeOnly = false
	if len(s.filtered()) != 4 {
		t.Fatalf("filter off should keep all 4")
	}

	// the reported bug: a resistor must NOT leak into a capacitor search
	cap := searchState{
		kind:     value.Capacitance,
		typeOnly: true,
		results: []lcsc.Part{
			{Code: "C1591", IntroEn: "100nF ±10% 50V Ceramic Capacitor X7R 0603"},
			{Code: "C60491", IntroEn: "100kΩ 62.5mW 50V Thick Film Resistor ±1% 0402"},
			{Code: "C60490", IntroEn: "10kΩ 62.5mW 50V Thick Film Resistor ±1% 0402"},
		},
	}
	f = cap.filtered()
	if len(f) != 1 || f[0].Code != "C1591" {
		t.Fatalf("resistor leaked into capacitor search: %d results", len(f))
	}
}

func TestDeriveKind(t *testing.T) {
	cases := []struct {
		val, prefix string
		want        value.Kind
	}{
		{"4.7k", "R", value.Resistance},
		{"1uF", "C", value.Capacitance},
		{"3.3uH", "L", value.Inductance},
		{"RP2040", "U", value.Unknown},
		{"", "R", value.Resistance},
	}
	for _, c := range cases {
		if got := deriveKind(c.val, c.prefix); got != c.want {
			t.Errorf("deriveKind(%q,%q)=%v want %v", c.val, c.prefix, got, c.want)
		}
	}
}
