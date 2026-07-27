package export

import (
	"testing"

	"bomexpo/internal/kicad"
)

func TestCorrectRotationTop(t *testing.T) {
	cases := []struct {
		fp     string
		in     float64
		want   float64
		bottom bool
	}{
		{"SOT-23", 0, 180, false},
		{"SOT-23-6", 90, 270, false},
		{"SOT-223-3_TabPin2", 0, 180, false},
		{"SOIC-8_5.3x5.3mm_P1.27mm", 0, 270, false},
		{"SOIC-16_3.9x9.9mm_P1.27mm", 90, 0, false},
		{"SSOP-28_5.3x10.2mm_P0.65mm", 90, 0, false},
		{"VSSOP-8_2.3x2mm_P0.5mm", 0, 270, false},
		{"D_SOD-123", 0, 180, false},
		// untouched families keep the angle (normalised).
		{"R_0402_1005Metric", 90, 90, false},
		{"C_0603_1608Metric", -90, 270, false},
		{"QFN-56-1EP_7x7mm_P0.4mm_EP3.2x3.2mm", 0, 0, false},
		{"JST_SH_BM03B-SRSS-TB_1x03-1MP", 180, 180, false},
		// bottom mirrors the offset sign.
		{"SOT-23", 0, 180, true},
		{"SOIC-8_5.3x5.3mm_P1.27mm", 0, 90, true},
	}
	for _, c := range cases {
		if got := correctRotation(c.fp, c.in, c.bottom); got != c.want {
			t.Errorf("correctRotation(%q, %g, bottom=%v) = %g, want %g", c.fp, c.in, c.bottom, got, c.want)
		}
	}
}

func TestRotationFixesReportsOnlyChanged(t *testing.T) {
	pl := []kicad.Placement{
		{Designator: "U1", Package: "SOIC-8_5.3x5.3mm_P1.27mm", Rotation: 0, Layer: "top"},
		{Designator: "R1", Package: "R_0402_1005Metric", Rotation: 90, Layer: "top"},
		{Designator: "Q1", Package: "SOT-23", Rotation: 0, Layer: "top"},
		{Designator: "U2", Package: "QFN-56-1EP", Rotation: 0, Layer: "top"},
	}
	fixes := RotationFixes(pl, map[string]bool{"Q1": true})
	if len(fixes) != 1 {
		t.Fatalf("want 1 fix (U1 only; R1/U2 unchanged, Q1 excluded), got %d: %+v", len(fixes), fixes)
	}
	if fixes[0].Designator != "U1" || fixes[0].To != 270 {
		t.Errorf("got %+v, want U1 -> 270", fixes[0])
	}
}

func TestNormDeg(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{0, 0}, {360, 0}, {-90, 270}, {450, 90}, {-450, 270},
	} {
		if got := normDeg(c.in); got != c.want {
			t.Errorf("normDeg(%g) = %g, want %g", c.in, got, c.want)
		}
	}
}
