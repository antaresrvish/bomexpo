package lcsc

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheTTL is how long a cached part detail is served before a normal load
// re-fetches it. Refresh always bypasses this.
const cacheTTL = 24 * time.Hour

func (c *Client) cachePath(code string) string {
	if c.cacheDir == "" {
		return ""
	}
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, code)
	return filepath.Join(c.cacheDir, safe+".json")
}

func (c *Client) cacheGet(code string) (raw []byte, fresh, ok bool) {
	p := c.cachePath(code)
	if p == "" {
		return nil, false, false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, false, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false, false
	}
	return data, time.Since(fi.ModTime()) < cacheTTL, true
}

func (c *Client) cachePut(code string, raw []byte) {
	p := c.cachePath(code)
	if p == "" || len(raw) == 0 {
		return
	}
	os.WriteFile(p, raw, 0o644)
}
