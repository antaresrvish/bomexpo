package render

import (
	"regexp"
	"strings"
	"testing"

	"bomexpo/internal/kicad"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

// shape is the drawing without colour, for comparing silhouettes.
func shape(s string) string { return ansiRe.ReplaceAllString(s, "") }

// chip0402 is a two-pad chip: pad 1 left, pad 2 right.
var chip0402 = []kicad.Land{
	{Name: "1", X: -0.48, Y: 0, W: 0.55, H: 0.65, First: true, Net: "GND"},
	{Name: "2", X: 0.48, Y: 0, W: 0.55, H: 0.65, Net: "+3V3"},
}

func TestFootprintDrawsSomething(t *testing.T) {
	out := Footprint(chip0402, FootprintOptions{W: 40, H: 10})
	if out == "" {
		t.Fatal("nothing drawn")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Errorf("drew %d lines, want 10", len(lines))
	}
	if !strings.Contains(out, "▀") {
		t.Error("no pad blocks in the drawing")
	}
}

func TestFootprintNeedsPadsAndRoom(t *testing.T) {
	if got := Footprint(nil, FootprintOptions{W: 40, H: 10}); got != "" {
		t.Error("no pads should draw nothing")
	}
	if got := Footprint(chip0402, FootprintOptions{W: 4, H: 10}); got != "" {
		t.Error("too narrow should draw nothing")
	}
	if got := Footprint(chip0402, FootprintOptions{W: 40, H: 1}); got != "" {
		t.Error("too short should draw nothing")
	}
}

// The drawing is capped in scale, so a tiny part stays tiny — that's what lets
// you tell a chip from a connector by eye.
func TestFootprintScaleIsCapped(t *testing.T) {
	filled := func(s string) int {
		return strings.Count(s, "▀")
	}
	small := filled(Footprint(chip0402, FootprintOptions{W: 44, H: 11}))

	big := make([]kicad.Land, 0, 4)
	for i := 0; i < 4; i++ {
		big = append(big, kicad.Land{X: float64(i) * 2.54, Y: 0, W: 1.7, H: 1.7, Hole: true})
	}
	large := filled(Footprint(big, FootprintOptions{W: 44, H: 11}))

	if small == 0 || large == 0 {
		t.Fatalf("nothing drawn: small %d large %d", small, large)
	}
	// a 1.5mm chip must not cover as much canvas as a 9mm header
	if small >= large {
		t.Errorf("0402 filled %d cells and a 1x4 header %d — the scale isn't capped", small, large)
	}
}

// A quarter turn moves the pads, which is the point: the drawing shows how the
// part will actually sit.
func TestFootprintRotationTurnsThePads(t *testing.T) {
	flat := Footprint(chip0402, FootprintOptions{W: 40, H: 10})
	turned := Footprint(chip0402, FootprintOptions{W: 40, H: 10, Rotate: 90})
	if flat == turned {
		t.Error("90° should change the drawing")
	}
	// A half turn leaves a symmetric chip's silhouette alone — only pad 1 swaps
	// sides, and that's a colour change, so compare the shape not the escapes.
	if half := Footprint(chip0402, FootprintOptions{W: 40, H: 10, Rotate: 180}); shape(half) != shape(flat) {
		t.Errorf("180° changed the silhouette:\n%s\nvs\n%s", shape(half), shape(flat))
	}
	// and 360 comes home
	if full := Footprint(chip0402, FootprintOptions{W: 40, H: 10, Rotate: 360}); full != flat {
		t.Error("360° should match 0°")
	}
	// a negative angle is the same as its positive complement
	if neg := Footprint(chip0402, FootprintOptions{W: 40, H: 10, Rotate: -90}); neg != Footprint(chip0402, FootprintOptions{W: 40, H: 10, Rotate: 270}) {
		t.Error("-90° should equal 270°")
	}
}

func TestTurnLand(t *testing.T) {
	l := kicad.Land{X: 2, Y: 0, W: 1, H: 3}
	q1 := turnLand(l, 1)
	if q1.X != 0 || q1.Y != 2 {
		t.Errorf("a quarter turn put it at %g,%g; want 0,2", q1.X, q1.Y)
	}
	if q1.W != 3 || q1.H != 1 {
		t.Errorf("a quarter turn should swap the extents, got %gx%g", q1.W, q1.H)
	}
	if q4 := turnLand(l, 4); q4 != l {
		t.Errorf("four quarter turns changed it: %+v", q4)
	}
}

func TestFootprintSummary(t *testing.T) {
	if got, want := FootprintSummary(chip0402), "2 pads · 1.5×0.7mm"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	one := []kicad.Land{{W: 1, H: 1}}
	if got := FootprintSummary(one); !strings.HasPrefix(got, "1 pad ") {
		t.Errorf("summary = %q, want the singular", got)
	}
	drilled := []kicad.Land{
		{X: 0, W: 1.7, H: 1.7, Hole: true},
		{X: 2.54, W: 1.7, H: 1.7, Hole: true},
		{X: 5.08, W: 1.7, H: 1.7},
	}
	if got := FootprintSummary(drilled); !strings.HasSuffix(got, "2 drilled") {
		t.Errorf("summary = %q, want it to count the drilled pads", got)
	}
	if got := FootprintSummary(nil); got != "" {
		t.Errorf("summary with no pads = %q, want empty", got)
	}
}
