package easyeda

import (
	"math"
	"testing"
)

// Parts with a dimension anyone can check: a 40-pin 2.54mm header must come out
// 39*2.54 + one pad long, and an 0.5mm-pitch LQFP-48 must land on a 10mm span.
func TestLiveFetchKnownParts(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	c.w.SetCacheDir(t.TempDir())
	for _, code := range []string{"C1525", "C8734", "C2337", "C25744", "C14663"} {
		fp, err := c.Fetch(code)
		if err != nil {
			t.Errorf("%-8s error: %v", code, err)
			continue
		}
		minX, minY, maxX, maxY := 1e9, 1e9, -1e9, -1e9
		holes := 0
		for _, l := range fp.Lands {
			minX, minY = min(minX, l.X-l.W/2), min(minY, l.Y-l.H/2)
			maxX, maxY = max(maxX, l.X+l.W/2), max(maxY, l.Y+l.H/2)
			if l.Hole {
				holes++
			}
		}
		w, h := maxX-minX, maxY-minY
		t.Logf("%-8s %-34s %2d pads  %5.2f x %5.2f mm  pad1 %.2fx%.2f  drilled=%d",
			code, fp.Package, len(fp.Lands), w, h, fp.Lands[0].W, fp.Lands[0].H, holes)

		switch code {
		case "C2337": // 40 pins at 2.54mm, plus one pad's width
			if len(fp.Lands) != 40 || holes != 40 {
				t.Errorf("%s: %d pads, %d drilled; want 40 and 40", code, len(fp.Lands), holes)
			}
			if want := 39*2.54 + fp.Lands[0].W; math.Abs(w-want) > 0.05 {
				t.Errorf("%s: %.2fmm long, want %.2f — the unit scale is off", code, w, want)
			}
		case "C8734": // LQFP-48, 0.5mm pitch, 9mm nominal land span
			if len(fp.Lands) != 48 {
				t.Errorf("%s: %d pads, want 48", code, len(fp.Lands))
			}
			if math.Abs(w-h) > 0.2 {
				t.Errorf("%s: %.2f x %.2f, want square", code, w, h)
			}
			if w < 8 || w > 11 {
				t.Errorf("%s: %.2fmm across, want about 10", code, w)
			}
		case "C1525", "C25744": // 0402 chips
			if len(fp.Lands) != 2 {
				t.Errorf("%s: %d pads, want 2", code, len(fp.Lands))
			}
			if w < 1 || w > 1.8 || h < 0.4 || h > 0.9 {
				t.Errorf("%s: %.2f x %.2f mm is not an 0402 land", code, w, h)
			}
		}
		if fp.Package == "" {
			t.Errorf("%s: no package name", code)
		}
	}
}
