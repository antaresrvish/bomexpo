package lcsc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetailUsesCache(t *testing.T) {
	c := New()
	c.cacheDir = t.TempDir()
	raw := `{"productCode":"C123","productModel":"RC0402","stockNumber":4200,` +
		`"productPriceList":[{"ladder":1,"usdPrice":0.002}]}`
	if err := os.WriteFile(filepath.Join(c.cacheDir, "C123.json"), []byte(raw), 0644); err != nil {
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

func TestCachePutGet(t *testing.T) {
	c := New()
	c.cacheDir = t.TempDir()
	c.cachePut("C9", []byte(`{"productCode":"C9"}`))
	raw, fresh, ok := c.cacheGet("C9")
	if !ok || !fresh {
		t.Fatalf("expected a fresh hit, ok=%v fresh=%v", ok, fresh)
	}
	if string(raw) != `{"productCode":"C9"}` {
		t.Errorf("bad cached bytes: %s", raw)
	}
	if _, _, ok := c.cacheGet("nope"); ok {
		t.Error("expected a miss for an unknown code")
	}
}
