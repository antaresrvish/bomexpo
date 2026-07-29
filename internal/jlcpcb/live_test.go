package jlcpcb

import (
	"testing"

	"bomexpo/internal/part"
)

func TestLiveSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	res, err := New().Search(part.Query{Keyword: "100nF 0402", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total == 0 || len(res.Items) == 0 {
		t.Fatalf("no results: %+v", res)
	}
	var withLib, withPrice int
	for _, p := range res.Items {
		t.Logf("%-10s %-10s %-8s stock=%-10d %s | %s", p.Code, p.Lib, p.Package, p.Stock, p.PriceLabel(), p.Description())
		if p.Lib.Known() {
			withLib++
		}
		if _, ok := p.UnitPrice(); ok {
			withPrice++
		}
	}
	if withLib == 0 {
		t.Error("no result reported a library type — the field mapping may have drifted")
	}
	if withPrice == 0 {
		t.Error("no result carried a price ladder")
	}
}

func TestLiveBasicOnlyFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	all, err := c.Search(part.Query{Keyword: "100nF", Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	basic, err := c.Search(part.Query{Keyword: "100nF", Size: 10, BasicOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("100nF: %d total, %d basic", all.Total, basic.Total)
	if basic.Total == 0 {
		t.Fatal("basic-only search returned nothing")
	}
	if basic.Total >= all.Total {
		t.Errorf("basic-only (%d) should be a subset of all (%d)", basic.Total, all.Total)
	}
	for _, p := range basic.Items {
		if p.Lib != part.LibBasic {
			t.Errorf("%s came back as %v under a basic-only search", p.Code, p.Lib)
		}
	}
}

func TestLiveDetail(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	c.w.SetCacheDir(t.TempDir()) // don't let a warm cache hide a broken fetch

	p, err := c.Detail("C1525")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("C1525 -> %s %s %s stock=%d asmMin=%d loss=%d %s",
		p.MPN, p.Brand, p.Lib, p.Stock, p.AsmMin, p.Loss, p.PriceLabel())
	if p.Code != "C1525" {
		t.Fatalf("code = %q, want C1525", p.Code)
	}
	if p.MPN == "" || len(p.Prices) == 0 {
		t.Error("detail is missing an MPN or a price ladder")
	}

	// second call must be served from the cache we just filled
	if _, err := c.Detail("C1525"); err != nil {
		t.Errorf("cached detail failed: %v", err)
	}
	if _, _, ok := c.w.CacheGet("C1525"); !ok {
		t.Error("detail did not populate the cache")
	}
}

func TestLiveDetailUnknownCode(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	c.w.SetCacheDir(t.TempDir())
	if _, err := c.Detail("C000000000"); err == nil {
		t.Error("want an error for a code that doesn't exist")
	}
}
