package lcsc

import (
	"testing"

	"bomexpo/internal/part"
)

func TestToPartMapsFields(t *testing.T) {
	got := toPart(Part{
		Code:      "C1525",
		Model:     "CL05B104KO5NNNC",
		Brand:     "Samsung Electro-Mechanics",
		Package:   "  0402 ", // LCSC pads some of these
		IntroEn:   "100nF ±10% 16V Ceramic Capacitor X7R 0402",
		Datasheet: "https://x/ds.pdf",
		Stock:     2130400,
		MinBuy:    50,
		Prices: []Price{
			{Ladder: 1000, USD: 0.0044},
			{Ladder: 1, USD: 0.0053},
			{Ladder: 5000, USD: 0}, // no price, dropped
		},
		Params: []Param{
			{Name: "Tolerance", Value: "±10%"},
			{Name: "Nothing", Value: ""}, // dropped
		},
	})

	if got.Source != "lcsc" || got.Code != "C1525" || got.MPN != "CL05B104KO5NNNC" {
		t.Errorf("identity fields wrong: %+v", got)
	}
	if got.Package != "0402" {
		t.Errorf("package = %q, want it trimmed to 0402", got.Package)
	}
	if got.Desc != "100nF ±10% 16V Ceramic Capacitor X7R 0402" || got.Datasheet != "https://x/ds.pdf" {
		t.Errorf("desc/datasheet wrong: %q %q", got.Desc, got.Datasheet)
	}
	if got.Stock != 2130400 || got.MinBuy != 50 {
		t.Errorf("stock/minbuy wrong: %d %d", got.Stock, got.MinBuy)
	}

	want := []part.Price{{Ladder: 1, USD: 0.0053}, {Ladder: 1000, USD: 0.0044}}
	if len(got.Prices) != len(want) {
		t.Fatalf("prices = %+v, want %d entries", got.Prices, len(want))
	}
	for i := range want {
		if got.Prices[i] != want[i] {
			t.Errorf("price[%d] = %+v, want %+v", i, got.Prices[i], want[i])
		}
	}
	if len(got.Params) != 1 || got.Params[0].Name != "Tolerance" {
		t.Errorf("params = %+v", got.Params)
	}

	// a shop says nothing about assembly libraries
	if got.Lib.Known() || got.AsmMin != 0 || got.Loss != 0 {
		t.Errorf("assembly fields should be zero, got %v %d %d", got.Lib, got.AsmMin, got.Loss)
	}
}

func TestToPartDescriptionFallbacks(t *testing.T) {
	for _, c := range []struct {
		name     string
		in       Part
		wantDesc string
		wantShow string
	}{
		{"intro wins", Part{IntroEn: "intro", NameEn: "name", Model: "MPN"}, "intro", "intro"},
		{"name is next", Part{NameEn: "name", Model: "MPN"}, "name", "name"},
		// Desc stays empty so part.Description() does the MPN fallback itself
		{"mpn last", Part{Model: "MPN"}, "", "MPN"},
		{"nothing at all", Part{}, "", "—"},
	} {
		got := toPart(c.in)
		if got.Desc != c.wantDesc {
			t.Errorf("%s: Desc = %q, want %q", c.name, got.Desc, c.wantDesc)
		}
		if shown := got.Description(); shown != c.wantShow {
			t.Errorf("%s: Description() = %q, want %q", c.name, shown, c.wantShow)
		}
	}
}

func TestProviderSearchGoesThroughAdapter(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	p := New().Provider()
	res, err := p.Search(part.Query{Keyword: "100nF 0402 X7R", Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no results through the provider adapter")
	}
	for _, it := range res.Items {
		if it.Source != "lcsc" || it.Code == "" {
			t.Errorf("adapter produced a malformed part: %+v", it)
		}
		t.Logf("%-8s %-22s stock=%-9d %s", it.Code, it.MPN, it.Stock, it.PriceLabel())
	}
}
