package lcsc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	baseWMSC = "https://wmsc.lcsc.com"
	baseWWW  = "https://www.lcsc.com"

	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
)

type Client struct {
	http     *http.Client
	sem      chan struct{}
	cacheDir string
}

func New() *Client {
	c := &Client{
		http: &http.Client{Timeout: 15 * time.Second},
		sem:  make(chan struct{}, 6),
	}
	if dir, err := os.UserCacheDir(); err == nil {
		c.cacheDir = filepath.Join(dir, "bomexpo", "lcsc")
		os.MkdirAll(c.cacheDir, 0o755)
	}
	return c
}

type envelope struct {
	Code   int             `json:"code"`
	Msg    string          `json:"msg"`
	Result json.RawMessage `json:"result"`
	OK     bool            `json:"ok"`
}

func (c *Client) call(method, url string, body any, out any) error {
	raw, err := c.callRaw(method, url, body)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) callRaw(method, url string, body any) (json.RawMessage, error) {
	if c.sem != nil {
		c.sem <- struct{}{}
		defer func() { <-c.sem }()
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseWWW+"/")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	var env envelope
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
		if reader != nil && attempt > 0 {
			b, _ := json.Marshal(body)
			req.Body = io.NopCloser(bytes.NewReader(b))
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
		if err := json.Unmarshal(data, &env); err != nil {
			lastErr = fmt.Errorf("decode: %w", err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if env.Code != 200 {
		return nil, fmt.Errorf("lcsc: %s (code %d)", env.Msg, env.Code)
	}
	return env.Result, nil
}
