package webjson

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c := New("test", "https://example.test/")
	c.SetCacheDir(t.TempDir())
	return c
}

func TestCacheRoundTrip(t *testing.T) {
	c := testClient(t)
	c.CachePut("C9", []byte(`{"code":"C9"}`))

	raw, fresh, ok := c.CacheGet("C9")
	if !ok || !fresh {
		t.Fatalf("expected a fresh hit, ok=%v fresh=%v", ok, fresh)
	}
	if string(raw) != `{"code":"C9"}` {
		t.Errorf("bad cached bytes: %s", raw)
	}
	if _, _, ok := c.CacheGet("nope"); ok {
		t.Error("expected a miss for an unknown key")
	}
}

func TestCacheReportsStale(t *testing.T) {
	c := testClient(t)
	c.CachePut("old", []byte(`{}`))
	p := c.cachePath("old")
	past := time.Now().Add(-2 * CacheTTL)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}
	// still readable, but the caller must know it's past its TTL
	if _, fresh, ok := c.CacheGet("old"); !ok || fresh {
		t.Errorf("want a stale hit, got ok=%v fresh=%v", ok, fresh)
	}
}

func TestCacheKeysCannotEscapeDir(t *testing.T) {
	c := testClient(t)
	c.CachePut("../../etc/passwd", []byte(`{}`))
	if _, _, ok := c.CacheGet("../../etc/passwd"); !ok {
		t.Fatal("sanitized key should still round-trip")
	}
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one file in the cache dir, got %d", len(entries))
	}
	if got := entries[0].Name(); got != "______etc_passwd.json" {
		t.Errorf("unsanitized cache filename: %q", got)
	}
	if filepath.Dir(c.cachePath("../../x")) != c.cacheDir {
		t.Error("cachePath escaped the cache dir")
	}
}

func TestCacheDisabled(t *testing.T) {
	c := New("test", "")
	c.SetCacheDir("")
	c.CachePut("C1", []byte(`{}`)) // must not panic
	if _, _, ok := c.CacheGet("C1"); ok {
		t.Error("a disabled cache should always miss")
	}
}

func TestFetchRetriesNonJSON(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			// a 5xx or a truncated read is worth another try; a 4xx is not
			http.Error(w, "<html>bad gateway</html>", http.StatusBadGateway)
			return
		}
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Referer") == "" {
			t.Error("browser headers were not sent")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("json body sent without a content type")
		}
		w.Write([]byte(`{"code":200,"ok":true}`))
	}))
	defer srv.Close()

	c := testClient(t)
	data, err := c.Fetch("POST", srv.URL, map[string]string{"keyword": "100nF"})
	if err != nil {
		t.Fatalf("Fetch after one bad response: %v", err)
	}
	if string(data) != `{"code":200,"ok":true}` {
		t.Errorf("unexpected body: %s", data)
	}
	if hits != 2 {
		t.Errorf("want 2 requests (one retry), got %d", hits)
	}
}

func TestFetchGivesUpAndReportsLastError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	if _, err := testClient(t).Fetch("GET", srv.URL, nil); err == nil {
		t.Fatal("want an error when every attempt returns non-json")
	}
}

// A 4xx is the server's decision. Retrying a rate limit is how a short block becomes
// a long one, and EasyEDA serves a plain 403 for it.
func TestFetchDoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusNotFound} {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			http.Error(w, "<html>no</html>", status)
		}))
		c := testClient(t)
		_, err := c.Fetch("GET", srv.URL, nil)
		srv.Close()

		if err == nil {
			t.Errorf("%d: no error", status)
			continue
		}
		if hits != 1 {
			t.Errorf("%d: asked %d times, want once", status, hits)
		}
		want := status == http.StatusForbidden || status == http.StatusTooManyRequests
		if got := RateLimited(err); got != want {
			t.Errorf("%d: RateLimited = %v, want %v (%v)", status, got, want, err)
		}
	}
}
