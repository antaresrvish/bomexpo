package lcsc

import (
	"encoding/json"
	"fmt"

	"bomexpo/internal/webjson"
)

const (
	baseWMSC = "https://wmsc.lcsc.com"
	baseWWW  = "https://www.lcsc.com"
)

type Client struct {
	w *webjson.Client
}

func New() *Client {
	return &Client{w: webjson.New("lcsc", baseWWW+"/")}
}

// envelope wraps every LCSC response; the payload only counts when Code is 200.
type envelope struct {
	Code   int             `json:"code"`
	Msg    string          `json:"msg"`
	Result json.RawMessage `json:"result"`
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
	data, err := c.w.Fetch(method, url, body)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if env.Code != 200 {
		return nil, fmt.Errorf("lcsc: %s (code %d)", env.Msg, env.Code)
	}
	return env.Result, nil
}
