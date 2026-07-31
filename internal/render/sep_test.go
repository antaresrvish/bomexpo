package render

import (
	"regexp"
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

// A pad drawn from a rounded half-width fattens by up to a subpixel a side, which
// closed the gap between neighbours at some panel heights and not others: the
// 8-pad array below came out as two slabs at h=8 and four pads at h=7 and h=9.
func TestPadsStaySeparateAtEveryHeight(t *testing.T) {
	var lands []kicad.Land
	for i, x := range []float64{-0.75, -0.25, 0.25, 0.75} {
		lands = append(lands, kicad.Land{X: x, Y: -0.427, W: 0.3, H: 0.505, First: i == 0})
		lands = append(lands, kicad.Land{X: x, Y: 0.427, W: 0.3, H: 0.505})
	}
	esc := regexp.MustCompile("\x1b\\[[0-9;]*m")
	run := regexp.MustCompile(`▀+`)
	for _, w := range []int{24, 36, 48} {
		for h := 6; h <= 16; h++ {
			img := esc.ReplaceAllString(Footprint(lands, FootprintOptions{W: w, H: h}), "")
			best := 0
			for _, line := range strings.Split(img, "\n") {
				if n := len(run.FindAllString(line, -1)); n > best {
					best = n
				}
			}
			if best < 4 {
				t.Errorf("w=%d h=%d: %d pad groups across, want 4 — neighbours merged", w, h, best)
			}
		}
	}
}
