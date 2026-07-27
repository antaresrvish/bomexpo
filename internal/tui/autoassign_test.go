package tui

import (
	"strings"
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/value"
)

func cap0402(code, desc string, stock int, usd float64) lcsc.Part {
	return lcsc.Part{Code: code, Package: "0402", IntroEn: desc, Stock: stock,
		Prices: []lcsc.Price{{USD: usd}}}
}

func TestSpecValueHandling(t *testing.T) {
	it := kicad.Item{Value: "4.7uF 50V", Footprint: "C_0402_1005Metric"}
	if kw := searchKeyword(it); kw != "4.7uF" {
		t.Errorf("searchKeyword(%q)=%q want 4.7uF", it.Value, kw)
	}
	if deriveKind("4.7uF 50V", "C") != value.Capacitance {
		t.Error("deriveKind should be capacitance")
	}
	if r := value.Check("4.7uF 50V", "4.7uF ±10% 50V Ceramic Capacitor X7R 0402"); !r.Match {
		t.Errorf("4.7uF 50V vs 4.7uF part should match: %+v", r)
	}
	if r := value.Check("4.7uF 50V", "100nF ±10% 50V Ceramic Capacitor X7R 0402"); r.Match {
		t.Errorf("4.7uF 50V vs 100nF part should NOT match: %+v", r)
	}
	// pickBest must reject 100nF for a 4.7uF 50V line
	res := []lcsc.Part{
		cap0402("wrong", "100nF ±10% 50V Ceramic Capacitor X7R 0402", 9999, 0.001),
		cap0402("low", "4.7uF ±20% 16V Ceramic Capacitor X5R 0402", 9999, 0.002), // underrated
		cap0402("right", "4.7uF ±10% 50V Ceramic Capacitor X5R 0402", 9999, 0.02),
	}
	p, ok := pickBest(it, value.Capacitance, "0402", res)
	if !ok || p.Code != "right" {
		t.Fatalf("want 'right' (4.7uF 50V), got %q ok=%v", p.Code, ok)
	}
}

func TestPickBestParametric(t *testing.T) {
	it := kicad.Item{Value: "100nF 50V X7R", Footprint: "C_0402_1005Metric", Quantity: 10}
	res := []lcsc.Part{
		cap0402("A", "100nF ±10% 16V Ceramic Capacitor X7R 0402", 9999, 0.001),  // underrated voltage
		cap0402("B", "100nF ±10% 50V Ceramic Capacitor Y5V 0402", 9999, 0.0005), // wrong dielectric
		cap0402("C", "100nF ±10% 50V Ceramic Capacitor X7R 0402", 9999, 0.005),  // correct
		{Code: "D", Package: "0603", IntroEn: "100nF ±10% 50V Ceramic Capacitor X7R 0603", Stock: 9999, Prices: []lcsc.Price{{USD: 0.0001}}},
	}
	p, ok := pickBest(it, value.Capacitance, "0402", res)
	if !ok || p.Code != "C" {
		t.Fatalf("parametric: want C, got %q ok=%v", p.Code, ok)
	}
}

func TestPickBestExcludesUnstableAndOOS(t *testing.T) {
	it := kicad.Item{Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 1}
	res := []lcsc.Part{
		cap0402("Y", "100nF ±20% 25V Ceramic Capacitor Y5V 0402", 9999, 0.0005), // unstable, excluded by default
		cap0402("Z", "100nF ±10% 25V Ceramic Capacitor X7R 0402", 0, 0.001),     // out of stock
		cap0402("X", "100nF ±10% 25V Ceramic Capacitor X7R 0402", 5000, 0.003),  // the pick
	}
	p, ok := pickBest(it, value.Capacitance, "0402", res)
	if !ok || p.Code != "X" {
		t.Fatalf("want X (Y5V excluded, Z oos), got %q ok=%v", p.Code, ok)
	}

	// nothing in stock -> no pick (out-of-stock lines stay as-is)
	if _, ok := pickBest(it, value.Capacitance, "0402", []lcsc.Part{cap0402("Z", "100nF X7R 0402", 0, 0.001)}); ok {
		t.Fatal("should not pick an out-of-stock part")
	}
}

func TestLiveAutoAssign(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	lines := []kicad.Item{
		{Bases: []string{"R1"}, Value: "4.7k", Footprint: "R_0603_1608Metric", Quantity: 6},
		{Bases: []string{"C1"}, Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 10},
		{Bases: []string{"C2"}, Value: "10uF", Footprint: "C_0805_2012Metric", Quantity: 2},
	}
	m := New("")
	m.items = lines

	for i, it := range lines {
		msg := m.autoAssignCmd(i)().(autoAssignedMsg)
		if msg.err != nil {
			t.Fatalf("%s: %v", it.ID(), msg.err)
		}
		if !msg.ok {
			t.Errorf("%s (%s %s): no auto-pick", it.ID(), it.Value, it.Footprint)
			continue
		}
		p := msg.part
		wantPkg := sizeCode.FindString(it.Footprint)
		if !strings.EqualFold(p.Package, wantPkg) {
			t.Errorf("%s: picked package %s, want %s", it.ID(), p.Package, wantPkg)
		}
		target, _ := value.Parse(it.Value)
		got, ok := value.ExtractValue(p.Description())
		if !ok || !value.Equal(got, target) {
			t.Errorf("%s: picked value %q does not match %s", it.ID(), p.Description(), it.Value)
		}
		if p.Stock <= 0 {
			t.Errorf("%s: picked out-of-stock %s", it.ID(), p.Code)
		}
		t.Logf("%-3s %-6s %-22s -> %-9s %s  stock=%d %s", it.ID(), it.Value, it.Footprint, p.Code, p.Package, p.Stock, p.PriceLabel())
	}
}
