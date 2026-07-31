// Package webjson calls a vendor's JSON endpoints politely: bounded concurrency,
// a few retries, and an on-disk cache so the app still works offline. It knows
// nothing about response envelopes — each vendor unwraps its own.
package webjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// vendors serve this JSON to browsers, so identify as one
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"

	timeout  = 15 * time.Second
	attempts = 3
	inflight = 6

	CacheTTL = 24 * time.Hour
)

type Client struct {
	http     *http.Client
	sem      chan struct{}
	cacheDir string
	referer  string
}

// New caches under the user cache dir, per vendor. No directory, no caching.
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

// Fetch retries transport errors, unreadable bodies and non-JSON responses,
// reporting the last error if every attempt fails.
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
		// an error page or a truncated read, both worth another try
		if !json.Valid(data) {
			lastErr = &HTTPError{Status: resp.StatusCode, Bytes: len(data)}
			// A 4xx is the server's decision and will not change on retry. Hammering
			// a 403 is how a rate limit turns into a longer one.
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// HTTPError is a reply the server chose to send that wasn't the json we asked for.
type HTTPError struct {
	Status int
	Bytes  int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("server replied %d with %d bytes of non-json", e.Status, e.Bytes)
}

// RateLimited reports whether the server is turning us away rather than failing.
// Vendors serve a plain 403 for this as often as a 429.
func (e *HTTPError) RateLimited() bool {
	return e.Status == http.StatusForbidden || e.Status == http.StatusTooManyRequests
}

// RateLimited reports whether err is a vendor turning us away.
func RateLimited(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.RateLimited()
}

// SetCacheDir redirects the cache, for tests. Empty disables caching.
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

// CacheGet returns the bytes, whether they're fresh, and whether anything was
// cached. Callers decide if stale beats nothing.
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

// CachePut ignores failures: a cache miss shouldn't fail a lookup.
func (c *Client) CachePut(key string, raw []byte) {
	p := c.cachePath(key)
	if p == "" || len(raw) == 0 {
		return
	}
	os.WriteFile(p, raw, 0o644)
}
