package jlcpcb

import (
	"encoding/json"
	"testing"

	"bomexpo/internal/part"
)

// listFixture mirrors a real selectSmtComponentList payload: one basic part
// with deliberately out-of-order price breaks, one extended, one preferred, and
// one malformed row that must not lose the page.
const listFixture = `{"componentPageInfo":{"total":15939,"pageNum":1,"list":[
{"componentCode":"C1525","componentModelEn":"CL05B104KO5NNNC","componentBrandEn":"Samsung Electro-Mechanics",
 "componentSpecificationEn":"0402","describe":"100nF 16V X7R ±10% 0402 MLCC","dataManualUrl":"https://x/ds.pdf",
 "stockCount":45594270,"minPurchaseNum":1,"componentLibraryType":"base","preferredComponentFlag":false,
 "leastPatchNumber":20,"lossNumber":10,
 "componentPrices":[{"startNumber":3000,"endNumber":9999,"productPrice":0.0031},
                    {"startNumber":1,"endNumber":999,"productPrice":0.0053},
                    {"startNumber":1000,"endNumber":2999,"productPrice":0.0044}],
 "attributes":[{"attribute_name_en":"Voltage Rating","attribute_value_name":"16V"},
               {"attribute_name_en":"Tolerance","attribute_value_name":"±10%"},
               {"attribute_name_en":"Empty","attribute_value_name":""}]},
{"componentCode":"C8304","componentModelEn":"STM32F103CBT6","componentLibraryType":"expand",
 "stockCount":1657,"minPurchaseNum":1,"leastPatchNumber":1,"lossNumber":0,
 "componentPrices":[],"buyComponentPrices":[{"startNumber":1,"endNumber":9,"productPrice":2.1}]},
{"componentCode":"C2040","componentLibraryType":"expand","preferredComponentFlag":true,"stockCount":900},
"not-an-object"
]}}`

func decodeFixture(t *testing.T) []component {
	t.Helper()
	items, raws, total, err := decodeList(json.RawMessage(listFixture))
	if err != nil {
		t.Fatal(err)
	}
	if total != 15939 {
		t.Errorf("total = %d, want 15939", total)
	}
	if len(items) != 3 || len(raws) != 3 {
		t.Fatalf("want 3 usable rows (malformed row skipped), got %d items %d raws", len(items), len(raws))
	}
	return items
}

func TestConvertBasicPart(t *testing.T) {
	p := decodeFixture(t)[0].toPart()

	if p.Source != "jlcpcb" || p.Code != "C1525" || p.MPN != "CL05B104KO5NNNC" {
		t.Errorf("identity fields wrong: %+v", p)
	}
	if p.Brand != "Samsung Electro-Mechanics" || p.Package != "0402" {
		t.Errorf("brand/package wrong: %q %q", p.Brand, p.Package)
	}
	if p.Desc != "100nF 16V X7R ±10% 0402 MLCC" || p.Datasheet != "https://x/ds.pdf" {
		t.Errorf("desc/datasheet wrong: %q %q", p.Desc, p.Datasheet)
	}
	if p.Stock != 45594270 || p.MinBuy != 1 {
		t.Errorf("stock/minbuy wrong: %d %d", p.Stock, p.MinBuy)
	}
	if p.Lib != part.LibBasic {
		t.Errorf("Lib = %v, want Basic", p.Lib)
	}
	if p.AsmMin != 20 || p.Loss != 10 {
		t.Errorf("assembly numbers wrong: min %d loss %d", p.AsmMin, p.Loss)
	}

	// ladders must come out ascending, whatever order the wire used
	want := []part.Price{{Ladder: 1, USD: 0.0053}, {Ladder: 1000, USD: 0.0044}, {Ladder: 3000, USD: 0.0031}}
	if len(p.Prices) != len(want) {
		t.Fatalf("prices = %+v, want %d entries", p.Prices, len(want))
	}
	for i := range want {
		if p.Prices[i] != want[i] {
			t.Errorf("price[%d] = %+v, want %+v", i, p.Prices[i], want[i])
		}
	}
	if u, ok := p.PriceAt(1500); !ok || u != 0.0044 {
		t.Errorf("PriceAt(1500) = %g %v, want 0.0044 true", u, ok)
	}

	// empty attribute values are dropped
	if len(p.Params) != 2 || p.Params[0].Name != "Voltage Rating" || p.Params[1].Value != "±10%" {
		t.Errorf("params = %+v", p.Params)
	}
}

func TestConvertLibKinds(t *testing.T) {
	items := decodeFixture(t)
	if got := items[1].toPart().Lib; got != part.LibExtended {
		t.Errorf("expand mapped to %v, want Extended", got)
	}
	// the preferred flag wins over the raw "expand" type
	if got := items[2].toPart().Lib; got != part.LibPreferred {
		t.Errorf("preferred flag mapped to %v, want Preferred", got)
	}
	if got := (component{LibraryType: "mystery"}).lib(); got != part.LibUnknown {
		t.Errorf("unknown wording mapped to %v, want Unknown", got)
	}
}

func TestConvertFallsBackToBuyPrices(t *testing.T) {
	p := decodeFixture(t)[1].toPart()
	if len(p.Prices) != 1 || p.Prices[0].USD != 2.1 {
		t.Errorf("want the buy-only ladder as fallback, got %+v", p.Prices)
	}
}

func TestAsmUnitsAppliesMinimumAndLoss(t *testing.T) {
	p := decodeFixture(t)[0].toPart() // AsmMin 20, Loss 10
	for _, c := range []struct{ need, want int }{
		{1, 20},  // minimum dominates
		{5, 20},  // 5+10 still under the minimum
		{15, 25}, // need + loss clears it
		{100, 110},
	} {
		if got := p.AsmUnits(c.need); got != c.want {
			t.Errorf("AsmUnits(%d) = %d, want %d", c.need, got, c.want)
		}
	}
	// a part with no assembly numbers consumes exactly what's needed
	if got := (part.Part{}).AsmUnits(7); got != 7 {
		t.Errorf("AsmUnits with no assembly data = %d, want 7", got)
	}
}
