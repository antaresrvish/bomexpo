package lcsc

import "testing"

func TestLiveSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	res, err := c.Search("100nF 0402 X7R", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total == 0 || len(res.Items) == 0 {
		t.Fatalf("no results: %+v", res)
	}
	for _, p := range res.Items {
		t.Logf("%-8s %-22s stock=%-8d %s | %s", p.Code, p.Model, p.Stock, p.PriceLabel(), p.Description())
	}
}

func TestLiveDetail(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	p, err := c.Detail("C1525")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("C1525 -> %s %s stock=%d %s", p.Model, p.Brand, p.Stock, p.PriceLabel())
	if p.Model == "" {
		t.Fatal("empty model")
	}
}

func TestLiveCategory(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	cats, err := c.CategoryTree()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("categories: %d (first: %+v)", len(cats), cats[0])
	if len(cats) < 10 {
		t.Fatalf("too few categories: %d", len(cats))
	}
}
