package export

import (
	"testing"

	"bomexpo/internal/kicad"
)

func TestCorrectRotation(t *testing.T) {
	cases := []struct {
		fp, lib string
		in      float64
		want    float64
		bottom  bool
		why     string
	}{
		{fp: "SOT-23", in: 0, want: 180},
		{fp: "SOT-23-6", in: 90, want: 270},
		{fp: "SOT-223-3_TabPin2", in: 0, want: 180},
		{fp: "SOIC-8_5.3x5.3mm_P1.27mm", in: 0, want: 270},
		{fp: "SOIC-16_3.9x9.9mm_P1.27mm", in: 90, want: 0},
		{fp: "SSOP-28_5.3x10.2mm_P0.65mm", in: 90, want: 0},
		{fp: "VSSOP-8_2.3x2mm_P0.5mm", in: 0, want: 270},
		{fp: "D_SOD-123", in: 0, want: 180},
		// untouched families keep the angle, normalised
		{fp: "R_0402_1005Metric", in: 90, want: 90},
		{fp: "C_0603_1608Metric", in: -90, want: 270},
		{fp: "QFN-56-1EP_7x7mm_P0.4mm_EP3.2x3.2mm", in: 0, want: 0},
		{fp: "JST_SH_BM03B-SRSS-TB_1x03-1MP", in: 180, want: 180},

		// A footprint imported from the vendor is already drawn to their 0°, and the
		// library is the only thing that says so. JLCPCB reported this one 180° over.
		{fp: "SOT-23_L2.9-W1.3-P1.90-LS2.4-BR", lib: "easyeda2kicad", in: -90, want: 270,
			why: "Q1 on the qr-order board"},
		{fp: "SOT-23", lib: "Package_TO_SOT_SMD", in: 0, want: 180,
			why: "kicad's own library still gets the offset"},

		// Bottom is seen from the other side, so the angle mirrors before the offset.
		// JLCPCB reported this connector 180° over, which the mirror accounts for.
		{fp: "CONN-SMD_2P-P1.00_SM02B-SRSS-TB-LF-SN", lib: "easyeda2kicad", in: 180, want: 0,
			bottom: true, why: "AA1 on the qr-order board"},
		{fp: "SOT-23", in: 0, want: 0, bottom: true},
		{fp: "SOIC-8_5.3x5.3mm_P1.27mm", in: 0, want: 270, bottom: true},
	}
	for _, c := range cases {
		got := correctRotation(c.fp, c.lib, c.in, c.bottom)
		if got != c.want {
			t.Errorf("correctRotation(%q, lib=%q, %g, bottom=%v) = %g, want %g  %s",
				c.fp, c.lib, c.in, c.bottom, got, c.want, c.why)
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
	fixes := RotationFixes(pl, map[string]bool{"Q1": true}, nil)
	if len(fixes) != 1 {
		t.Fatalf("want 1 fix (U1 only; R1/U2 unchanged, Q1 excluded), got %d: %+v", len(fixes), fixes)
	}
	if fixes[0].Designator != "U1" || fixes[0].To != 270 {
		t.Errorf("got %+v, want U1 -> 270", fixes[0])
	}
}

func TestRotationFixesOverride(t *testing.T) {
	pl := []kicad.Placement{
		{Designator: "J1", Package: "USB_C_Receptacle", Rotation: 0, Layer: "top"}, // no auto correction
		{Designator: "U1", Package: "SOIC-8", Rotation: 0, Layer: "top"},           // auto 270
	}
	fixes := RotationFixes(pl, nil, map[string]int{"J1": 90})
	got := map[string]RotationFix{}
	for _, f := range fixes {
		got[f.Designator] = f
	}
	if f, ok := got["J1"]; !ok || !f.Manual || f.To != 90 {
		t.Errorf("J1 override: %+v ok=%v", f, ok)
	}
	if f, ok := got["U1"]; !ok || f.Manual || f.To != 270 {
		t.Errorf("U1 auto: %+v ok=%v", f, ok)
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
