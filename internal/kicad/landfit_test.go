package kicad

import "testing"

func TestFitsLand(t *testing.T) {
	cases := []struct {
		name        string
		board, part int
		want        bool
	}{
		// the two real mismatches found on the drone controller
		{"resistor array on a single 0402 land", 2, 8, false},
		{"xt60 male land, 4-pad female part", 2, 4, false},
		// the false alarms an equality test raised
		{"wson-10 with thermal vias", 22, 11, true},
		{"qfn-56 with thermal vias", 61, 57, true},
		{"usb-c with shield pads", 18, 16, true},
		{"jst connector, mounting pads paired", 5, 5, true},
		// no data on either side is not a verdict
		{"no board land", 0, 8, true},
		{"no part land", 2, 0, true},
	}
	for _, c := range cases {
		got, note := FitsLand(c.board, c.part)
		if got != c.want {
			t.Errorf("%s: FitsLand(%d, %d) = %v, want %v", c.name, c.board, c.part, got, c.want)
		}
		if !got && note == "" {
			t.Errorf("%s: a fault with no explanation", c.name)
		}
		if got && note != "" {
			t.Errorf("%s: a fit should say nothing, got %q", c.name, note)
		}
	}
}
