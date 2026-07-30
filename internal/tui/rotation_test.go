package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/export"
)

// The rotation summary used to be one line of "SOT-23 +180° ×4" tallies, which told
// you how many but never which. A rotation you did not expect is looked up by
// designator, so it has to be a list.
func TestRotationListNamesEveryPart(t *testing.T) {
	m := priceModel(t)
	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)

	fixes := []export.RotationFix{
		{Designator: "D1", Footprint: "Diode_SMD:D_SOD-523", From: 90, To: 270},
		{Designator: "Q1", Footprint: "Package_TO_SOT_SMD:SOT-23", From: 270, To: 90, Manual: true},
	}
	out := strings.Join(m.rotationList(fixes, 90), "\n")
	plain := stripANSI(out)

	for _, want := range []string{
		"JLCPCB rotation", "2 parts turned for the pick-and-place", "1 yours",
		"REF", "FOOTPRINT", "BOARD", "CPL", "WHY",
		"D1", "D_SOD-523", "90°", "270°", "library rule",
		"Q1", "SOT-23", "your override",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the list never says %q:\n%s", want, plain)
		}
	}
	// the library prefix is dropped, the way footprints read everywhere else
	if strings.Contains(plain, "Diode_SMD:") {
		t.Errorf("the library prefix should go: %q", plain)
	}
}

// A negative or over-wound angle is written the way a CPL wants it.
func TestRotationAnglesAreNormalised(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{
		{0, 0}, {90, 90}, {-90, 270}, {-270, 90}, {360, 0}, {450, 90},
	} {
		if got := normDeg(tc.in); got != tc.want {
			t.Errorf("normDeg(%g) = %g, want %g", tc.in, got, tc.want)
		}
	}
}

// A long list is cut with a count rather than pushing the pricing off the page.
func TestRotationListSaysWhatItCut(t *testing.T) {
	m := priceModel(t)
	m.w, m.h = 130, 30
	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)

	var fixes []export.RotationFix
	for i := 0; i < 40; i++ {
		fixes = append(fixes, export.RotationFix{
			Designator: "R" + string(rune('a'+i%26)), Footprint: "R_0402", From: 0, To: 180})
	}
	out := strings.Join(m.rotationList(fixes, 90), "\n")
	if !strings.Contains(stripANSI(out), "more") {
		t.Errorf("40 fixes were cut without saying so:\n%s", stripANSI(out))
	}
	if n := len(m.rotationList(fixes, 90)); n > 14 {
		t.Errorf("the list took %d lines, which crowds out the pricing", n)
	}
}
