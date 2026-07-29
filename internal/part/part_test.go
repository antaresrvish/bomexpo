package part

import "testing"

var ladder = []Price{{Ladder: 1, USD: 0.005}, {Ladder: 1000, USD: 0.004}, {Ladder: 3000, USD: 0.003}}

func TestPriceAtPicksTier(t *testing.T) {
	p := Part{Prices: ladder}
	for _, c := range []struct {
		qty  int
		want float64
	}{
		{0, 0.005}, // below the first break falls back to the smallest tier
		{1, 0.005},
		{999, 0.005},
		{1000, 0.004},
		{2999, 0.004},
		{3000, 0.003},
		{50000, 0.003},
	} {
		got, ok := p.PriceAt(c.qty)
		if !ok {
			t.Fatalf("PriceAt(%d): no price", c.qty)
		}
		if got != c.want {
			t.Errorf("PriceAt(%d) = %g, want %g", c.qty, got, c.want)
		}
	}
}

func TestUnitPriceIsCheapestLadder(t *testing.T) {
	u, ok := Part{Prices: ladder}.UnitPrice()
	if !ok || u != 0.003 {
		t.Fatalf("UnitPrice() = %g, %v; want 0.003, true", u, ok)
	}
	if _, ok := (Part{}).UnitPrice(); ok {
		t.Error("UnitPrice() on a part with no prices should report !ok")
	}
	if lbl := (Part{}).PriceLabel(); lbl != "—" {
		t.Errorf("PriceLabel() with no prices = %q, want —", lbl)
	}
}

func TestPriceAtWithoutPrices(t *testing.T) {
	if _, ok := (Part{}).PriceAt(100); ok {
		t.Error("PriceAt on a part with no prices should report !ok")
	}
}

func TestDescriptionFallsBackToMPN(t *testing.T) {
	for _, c := range []struct {
		p    Part
		want string
	}{
		{Part{Desc: "100nF 16V X7R", MPN: "CL05B104KO5NNNC"}, "100nF 16V X7R"},
		{Part{MPN: "CL05B104KO5NNNC"}, "CL05B104KO5NNNC"},
		{Part{}, "—"},
	} {
		if got := c.p.Description(); got != c.want {
			t.Errorf("Description() = %q, want %q", got, c.want)
		}
	}
}

func TestSpecsExcludesPrimaryValue(t *testing.T) {
	p := Part{Params: []Param{
		{Name: "Capacitance", Value: "100nF"}, // primary, belongs to the value column
		{Name: "Tolerance", Value: "±10%"},
		{Name: "Voltage Rating", Value: "16V"},
		{Name: "Temperature Coefficient", Value: ""}, // empty, dropped
	}}
	if got, want := p.Specs(), "±10% 16V"; got != want {
		t.Errorf("Specs() = %q, want %q", got, want)
	}
}

func TestLibKind(t *testing.T) {
	if !LibBasic.Known() || LibUnknown.Known() {
		t.Error("Known() should be true for Basic and false for Unknown")
	}
	for k, want := range map[LibKind]string{
		LibBasic: "Basic", LibPreferred: "Preferred", LibExtended: "Extended", LibUnknown: "unknown",
	} {
		if got := k.String(); got != want {
			t.Errorf("LibKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestQueryNormClamps(t *testing.T) {
	q := Query{}.Norm()
	if q.Page != 1 || q.Size != 25 {
		t.Errorf("Norm() on a zero query = page %d size %d, want 1/25", q.Page, q.Size)
	}
	if q := (Query{Page: 3, Size: 100}).Norm(); q.Page != 3 || q.Size != 100 {
		t.Errorf("Norm() should leave valid values alone, got page %d size %d", q.Page, q.Size)
	}
}
