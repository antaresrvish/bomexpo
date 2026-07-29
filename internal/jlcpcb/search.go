package jlcpcb

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"bomexpo/internal/part"
)

type listReq struct {
	CurrentPage int    `json:"currentPage"`
	PageSize    int    `json:"pageSize"`
	Keyword     string `json:"keyword"`
	LibraryType string `json:"componentLibraryType,omitempty"`
}

type listResp struct {
	PageInfo pageInfo `json:"componentPageInfo"`
}

// pageInfo keeps entries raw so a cached detail survives us learning to read
// more fields later.
type pageInfo struct {
	Total int               `json:"total"`
	List  []json.RawMessage `json:"list"`
}

type wirePrice struct {
	Start int     `json:"startNumber"`
	Price float64 `json:"productPrice"`
}

type wireAttr struct {
	Name  string `json:"attribute_name_en"`
	Value string `json:"attribute_value_name"`
}

type component struct {
	Code      string `json:"componentCode"`
	MPN       string `json:"componentModelEn"`
	Brand     string `json:"componentBrandEn"`
	Package   string `json:"componentSpecificationEn"`
	Describe  string `json:"describe"`
	Datasheet string `json:"dataManualUrl"`
	Stock     int    `json:"stockCount"`
	MinBuy    int    `json:"minPurchaseNum"`

	LibraryType string `json:"componentLibraryType"`
	Preferred   bool   `json:"preferredComponentFlag"`
	LeastPatch  int    `json:"leastPatchNumber"`
	Loss        int    `json:"lossNumber"`

	Prices    []wirePrice `json:"componentPrices"`
	BuyPrices []wirePrice `json:"buyComponentPrices"`
	Attrs     []wireAttr  `json:"attributes"`
}

// lib maps JLCPCB's own library wording onto the neutral kinds. A preferred
// part is an extended part JLCPCB keeps on hand, so it wins over the raw type.
func (w component) lib() part.LibKind {
	if w.Preferred {
		return part.LibPreferred
	}
	switch strings.ToLower(strings.TrimSpace(w.LibraryType)) {
	case "base":
		return part.LibBasic
	case "expand", "extend", "extended":
		return part.LibExtended
	}
	return part.LibUnknown
}

func (w component) toPart() part.Part {
	p := part.Part{
		Source:    "jlcpcb",
		Code:      w.Code,
		MPN:       w.MPN,
		Brand:     w.Brand,
		Package:   w.Package,
		Desc:      w.Describe,
		Datasheet: w.Datasheet,
		Stock:     w.Stock,
		MinBuy:    w.MinBuy,
		Lib:       w.lib(),
		AsmMin:    w.LeastPatch,
		Loss:      w.Loss,
	}

	// componentPrices is what an assembly order pays; the buy-only ladder is
	// the fallback for parts JLCPCB quotes but doesn't assemble.
	prices := w.Prices
	if len(prices) == 0 {
		prices = w.BuyPrices
	}
	for _, wp := range prices {
		if wp.Price > 0 {
			p.Prices = append(p.Prices, part.Price{Ladder: wp.Start, USD: wp.Price})
		}
	}
	// PriceAt walks the ladder in order and stops at the first break above the
	// quantity, so it must be ascending.
	sort.SliceStable(p.Prices, func(i, j int) bool { return p.Prices[i].Ladder < p.Prices[j].Ladder })

	for _, a := range w.Attrs {
		if a.Name != "" && a.Value != "" {
			p.Params = append(p.Params, part.Param{Name: a.Name, Value: a.Value})
		}
	}
	return p
}

func (c *Client) Search(q part.Query) (part.Result, error) {
	q = q.Norm()
	req := listReq{CurrentPage: q.Page, PageSize: q.Size, Keyword: q.Keyword}
	if q.BasicOnly {
		req.LibraryType = "base"
	}
	raw, err := c.call(listPath, req)
	if err != nil {
		return part.Result{}, err
	}
	items, _, total, err := decodeList(raw)
	if err != nil {
		return part.Result{}, err
	}
	out := part.Result{Total: total, Page: q.Page, PageSize: q.Size}
	for _, w := range items {
		out.Items = append(out.Items, w.toPart())
	}
	return out, nil
}

func decodeList(raw json.RawMessage) (items []component, raws []json.RawMessage, total int, err error) {
	var r listResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, nil, 0, fmt.Errorf("decode list: %w", err)
	}
	for _, entry := range r.PageInfo.List {
		var w component
		if json.Unmarshal(entry, &w) != nil {
			continue // one malformed row shouldn't lose the page
		}
		items = append(items, w)
		raws = append(raws, entry)
	}
	return items, raws, r.PageInfo.Total, nil
}

// Detail returns a part, served from the on-disk cache when a fresh copy exists
// and falling back to a stale copy if the network is unavailable.
func (c *Client) Detail(code string) (part.Part, error) { return c.detail(code, false) }

// Refresh re-fetches a part, bypassing the cache, and stores the fresh stock
// and pricing back into it.
func (c *Client) Refresh(code string) (part.Part, error) { return c.detail(code, true) }

func (c *Client) detail(code string, force bool) (part.Part, error) {
	cached, fresh, ok := c.w.CacheGet(code)
	if ok && fresh && !force {
		if p, good := decodeCached(cached); good {
			return p, nil
		}
	}
	entry, w, err := c.lookup(code)
	if err != nil {
		if ok && !force { // offline: serve whatever we cached, even if stale
			if p, good := decodeCached(cached); good {
				return p, nil
			}
		}
		return part.Part{}, err
	}
	c.w.CachePut(code, entry)
	return w.toPart(), nil
}

func decodeCached(raw []byte) (part.Part, bool) {
	var w component
	if json.Unmarshal(raw, &w) != nil || w.Code == "" {
		return part.Part{}, false
	}
	return w.toPart(), true
}

// lookup finds one part by code. There is no per-part endpoint, but a keyword
// search for a code returns the full record, so we search and match exactly.
func (c *Client) lookup(code string) (json.RawMessage, component, error) {
	raw, err := c.call(listPath, listReq{CurrentPage: 1, PageSize: 20, Keyword: code})
	if err != nil {
		return nil, component{}, err
	}
	items, raws, _, err := decodeList(raw)
	if err != nil {
		return nil, component{}, err
	}
	for i, w := range items {
		if strings.EqualFold(strings.TrimSpace(w.Code), strings.TrimSpace(code)) {
			return raws[i], w, nil
		}
	}
	return nil, component{}, fmt.Errorf("jlcpcb: no part with code %s", code)
}
