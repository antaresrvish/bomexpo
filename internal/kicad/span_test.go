package kicad

import (
	"math"
	"testing"
)

// two pads facing each other, spanning w × h overall
func chip(w, h float64) []Land {
	pw := w * 0.2
	return []Land{
		{Name: "1", X: -(w - pw) / 2, W: pw, H: h, First: true},
		{Name: "2", X: (w - pw) / 2, W: pw, H: h},
	}
}

// grid is a pads-on-all-sides package n per side inside w × w.
func grid(n int, w float64) []Land {
	var out []Land
	step := w / float64(n)
	for i := 0; i < n; i++ {
		x := -w/2 + step/2 + float64(i)*step
		out = append(out, Land{Name: "a", X: x, Y: -w / 2, W: step / 2, H: step / 2, First: i == 0})
		out = append(out, Land{Name: "b", X: x, Y: w / 2, W: step / 2, H: step / 2})
	}
	return out
}

// Every case below is a measurement off a real board, with the span ratio it produced.
func TestLandFitOnMeasuredCases(t *testing.T) {
	cases := []struct {
		name       string
		land, part []Land
		want       bool
	}{
		// caught by pad count
		{"4-resistor array on a single 0402 land", chip(1.56, 0.64), grid(4, 1.8), false},
		// caught by span, invisible to pad count: both are two-pad parts
		{"0402 capacitor on an 0603 land", chip(2.45, 0.95), chip(1.34, 0.54), false},
		// caught by span: 9 pads under 19, so pad count passes it
		{"WLCSP-9 on a WQFN-14 land", grid(7, 3.2), grid(3, 1.0), false},
		// the tightest correct assignment measured: hand-solder pads are long
		{"SOT-23-5 on its hand-soldering land", chip(4.4, 2.5), chip(3.6, 2.5), true},
		{"0402 on its own land", chip(1.64, 0.62), chip(1.34, 0.54), true},
		{"0603 on its own land", chip(2.63, 0.95), chip(2.38, 0.90), true},
		{"vson-10 on a dfn-10 land", grid(5, 3.0), grid(5, 3.0), true},
		{"nothing to compare", nil, chip(1, 1), true},
	}
	for _, c := range cases {
		got, note := LandFit(c.land, c.part)
		if got != c.want {
			t.Errorf("%s: LandFit = %v (%q), want %v — spans %.2f and %.2f",
				c.name, got, note, c.want, Span(c.land), Span(c.part))
		}
		if !got && note == "" {
			t.Errorf("%s: a fault with no explanation", c.name)
		}
	}
}

// The vendor may publish a part turned 90°; the span must not care.
func TestSpanIgnoresOrientation(t *testing.T) {
	flat := chip(2.45, 0.95)
	var turned []Land
	for _, l := range flat {
		turned = append(turned, Land{Name: l.Name, X: l.Y, Y: l.X, W: l.H, H: l.W, First: l.First})
	}
	if a, b := Span(flat), Span(turned); math.Abs(a-b) > 0.001 {
		t.Errorf("span %.3f turned becomes %.3f", a, b)
	}
}
