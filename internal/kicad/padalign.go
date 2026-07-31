package kicad

import "math"

// PadAlign is the angle the part has to be turned by for its pad 1 to sit where the
// land's does, snapped to a right angle. Vendors publish a footprint in their own
// frame — measured over one board, EasyEDA's differs from KiCad's by 90° on eight
// packages and 180° on four, and the rotation table the CPL uses predicts only some
// of them. Drawn in raw frames, a correct assignment reads as the wrong part.
//
// This aligns the drawings for comparison; it says nothing about placement angle,
// which is the CPL's business and is corrected separately.
func PadAlign(land, part []Land) float64 {
	la, ok1 := pad1Angle(land)
	pa, ok2 := pad1Angle(part)
	if !ok1 || !ok2 {
		return 0
	}
	return math.Round((la-pa)/90) * 90
}

// pad1Angle is where pad 1 sits as seen from the footprint's centre.
func pad1Angle(ls []Land) (float64, bool) {
	for _, l := range ls {
		if l.First {
			if l.X == 0 && l.Y == 0 {
				return 0, false
			}
			return math.Atan2(l.Y, l.X) * 180 / math.Pi, true
		}
	}
	return 0, false
}
