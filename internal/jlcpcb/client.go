// Package jlcpcb reads JLCPCB's assembly parts catalogue. Unlike a plain
// distributor it also says whether a part sits in JLCPCB's basic library —
// extended parts carry a per-part setup fee on an assembly order — and what
// minimum quantity an assembly job consumes.
package jlcpcb

import (
	"encoding/json"
	"fmt"

	"bomexpo/internal/part"
	"bomexpo/internal/webjson"
)

const (
	baseAPI  = "https://jlcpcb.com/api/overseas-pcb-order/v1"
	baseSite = "https://jlcpcb.com/"

	// listPath is what the assembly parts picker itself calls.
	listPath = "/shoppingCart/smtGood/selectSmtComponentList"
)

type Client struct {
	w *webjson.Client
}

func New() *Client {
	return &Client{w: webjson.New("jlcpcb", baseSite)}
}

func (c *Client) ID() string    { return "jlcpcb" }
func (c *Client) Label() string { return "JLCPCB" }

// Ready is always true: the catalogue needs no credentials.
func (c *Client) Ready() (bool, string) { return true, "" }

func (c *Client) Caps() part.Caps {
	return part.Caps{BasicFilter: true, Library: true, Assembly: true}
}

// envelope wraps every response; the payload only counts when Code is 200.
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) call(path string, body any) (json.RawMessage, error) {
	data, err := c.w.Fetch("POST", baseAPI+path, body)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if env.Code != 200 {
		msg := env.Message
		if msg == "" {
			msg = "request rejected"
		}
		return nil, fmt.Errorf("jlcpcb: %s (code %d)", msg, env.Code)
	}
	return env.Data, nil
}
