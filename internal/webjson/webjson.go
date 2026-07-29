// Package webjson calls a vendor's JSON endpoints politely: bounded
// concurrency, a few retries on transient failures, and an on-disk cache so
// repeat lookups are free and the app still works offline.
//
// It deliberately knows nothing about response envelopes — each vendor unwraps
// its own.
package webjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Vendors serve their JSON to browsers, so we identify as one.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"

	timeout  = 15 * time.Second
	attempts = 3
	inflight = 6

	// CacheTTL is how long a cached entry is served before a normal load
	// re-fetches it.
	CacheTTL = 24 * time.Hour
)

type Client struct {
	http     *http.Client
	sem      chan struct{}
	cacheDir string
	referer  string
}

// New returns a client whose cache lives under the user cache dir in a
// subdirectory named after the vendor. Caching is silently disabled if the
// directory is unavailable.
func New(name, referer string) *Client {
	c := &Client{
		http:    &http.Client{Timeout: timeout},
		sem:     make(chan struct{}, inflight),
		referer: referer,
	}
	if dir, err := os.UserCacheDir(); err == nil {
		c.cacheDir = filepath.Join(dir, "bomexpo", name)
		os.MkdirAll(c.cacheDir, 0o755)
	}
	return c
}

// Fetch performs the request, marshalling body as JSON when non-nil, and
// returns the raw response. Transport errors, unreadable bodies and responses
// that aren't valid JSON are retried; the last error is reported if all
// attempts fail.
func (c *Client) Fetch(method, url string, body any) ([]byte, error) {
	if c.sem != nil {
		c.sem <- struct{}{}
		defer func() { <-c.sem }()
	}

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if c.referer != "" {
		req.Header.Set("Referer", c.referer)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
			if payload != nil {
				req.Body = io.NopCloser(bytes.NewReader(payload))
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		// A non-JSON body means an error page or a truncated read, both of
		// which are worth another try.
		if !json.Valid(data) {
			lastErr = fmt.Errorf("decode: response is not json (%d bytes, status %s)", len(data), resp.Status)
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// SetCacheDir redirects the cache, which tests use to stay out of the real
// user cache dir. An empty dir disables caching.
func (c *Client) SetCacheDir(dir string) { c.cacheDir = dir }

func (c *Client) cachePath(key string) string {
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
	}, key)
	return filepath.Join(c.cacheDir, safe+".json")
}

// CacheGet reports the cached bytes for key, whether they are within CacheTTL,
// and whether anything was cached at all. Callers decide whether a stale entry
// is better than nothing.
func (c *Client) CacheGet(key string) (raw []byte, fresh, ok bool) {
	p := c.cachePath(key)
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
	return data, time.Since(fi.ModTime()) < CacheTTL, true
}

// CachePut stores raw under key, ignoring failures — a cache miss is never
// worth failing a lookup over.
func (c *Client) CachePut(key string, raw []byte) {
	p := c.cachePath(key)
	if p == "" || len(raw) == 0 {
		return
	}
	os.WriteFile(p, raw, 0o644)
}
