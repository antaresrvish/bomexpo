package lcsc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetailUsesCache(t *testing.T) {
	c := New()
	dir := t.TempDir()
	c.w.SetCacheDir(dir)
	raw := `{"productCode":"C123","productModel":"RC0402","stockNumber":4200,` +
		`"productPriceList":[{"ladder":1,"usdPrice":0.002}]}`
	if err := os.WriteFile(filepath.Join(dir, "C123.json"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	// a fresh cache entry must be served without touching the network
	p, err := c.Detail("C123")
	if err != nil {
		t.Fatal(err)
	}
	if p.Code != "C123" || p.Model != "RC0402" || p.Stock != 4200 {
		t.Errorf("bad decode from cache: %+v", p)
	}
	if u, ok := p.UnitPrice(); !ok || u != 0.002 {
		t.Errorf("price not decoded from cache: %v %v", u, ok)
	}
}
