package value

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		kind Kind
		base float64
		ok   bool
	}{
		{"0.1uF", Capacitance, 1e-7, true},
		{"100nF", Capacitance, 1e-7, true},
		{"1uF", Capacitance, 1e-6, true},
		{"10uF", Capacitance, 1e-5, true},
		{"4.7uF", Capacitance, 4.7e-6, true},
		{"10k", Resistance, 1e4, true},
		{"4.7k", Resistance, 4700, true},
		{"27R", Resistance, 27, true},
		{"10kΩ", Resistance, 1e4, true},
		{"100kΩ", Resistance, 1e5, true},
		{"220Ω", Resistance, 220, true},
		{"4.7kΩ", Resistance, 4700, true},
		{"10m", Resistance, 0.01, true},
		{"60.4k", Resistance, 60400, true},
		{"220R", Resistance, 220, true},
		{"4k7", Resistance, 4700, true},
		{"R47", Resistance, 0.47, true},
		{"3.3uH", Inductance, 3.3e-6, true},
		{"RP2040", Unknown, 0, false},
		{"AP2112K-3.3", Unknown, 0, false},
		{"SG-5032CAN", Unknown, 0, false},
	}
	for _, c := range cases {
		v, ok := Parse(c.in)
		if ok != c.ok {
			t.Errorf("%q ok=%v want %v", c.in, ok, c.ok)
			continue
		}
		if ok && (v.Kind != c.kind || !approx(v.Base, c.base)) {
			t.Errorf("%q => {%v %g} want {%v %g}", c.in, v.Kind, v.Base, c.kind, c.base)
		}
	}
}

func TestCheck(t *testing.T) {
	if r := Check("0.1uF", "100nF ±10% 50V Ceramic Capacitor X7R 0603"); !r.Match {
		t.Errorf("0.1uF vs 100nF should match: %+v", r)
	}
	if r := Check("10k", "10kΩ ±1% 100mW Chip Resistor"); !r.Match {
		t.Errorf("10k should match: %+v", r)
	}
	if r := Check("10uF", "100nF ±10% 50V Ceramic Capacitor X7R"); r.Match {
		t.Errorf("10uF vs 100nF should NOT match: %+v", r)
	}
	if r := Check("RP2040", "Dual ARM Cortex-M0+ MCU"); !r.Match {
		t.Errorf("non-value should pass: %+v", r)
	}
}

func approx(a, b float64) bool {
	if b == 0 {
		return a == 0
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d/b < 1e-6
}
