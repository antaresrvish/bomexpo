package lcsc

import (
	"sort"
	"strings"

	"bomexpo/internal/part"
)

// provider adapts the raw LCSC client to the neutral parts-source interface.
type provider struct{ c *Client }

// Provider exposes the client as a parts source.
func (c *Client) Provider() part.Provider { return provider{c: c} }

func (provider) ID() string            { return "lcsc" }
func (provider) Label() string         { return "LCSC" }
func (provider) Ready() (bool, string) { return true, "" }

// Caps is empty: LCSC is a shop, so it says nothing about assembly libraries.
func (provider) Caps() part.Caps { return part.Caps{} }

func (p provider) Search(q part.Query) (part.Result, error) {
	q = q.Norm()
	res, err := p.c.Search(q.Keyword, q.Page, q.Size)
	if err != nil {
		return part.Result{}, err
	}
	out := part.Result{Total: res.Total, Page: res.Page, PageSize: res.PageSize}
	for _, it := range res.Items {
		out.Items = append(out.Items, toPart(it))
	}
	return out, nil
}

func (p provider) Detail(code string) (part.Part, error) {
	it, err := p.c.Detail(code)
	if err != nil {
		return part.Part{}, err
	}
	return toPart(it), nil
}

func (p provider) Refresh(code string) (part.Part, error) {
	it, err := p.c.Refresh(code)
	if err != nil {
		return part.Part{}, err
	}
	return toPart(it), nil
}

func toPart(p Part) part.Part {
	// Leave the MPN fallback to part.Description() so Desc stays empty rather
	// than picking up a placeholder.
	desc := p.IntroEn
	if desc == "" {
		desc = p.NameEn
	}

	out := part.Part{
		Source:    "lcsc",
		Code:      p.Code,
		MPN:       p.Model,
		Brand:     p.Brand,
		Package:   strings.TrimSpace(p.Package),
		Desc:      desc,
		Datasheet: p.Datasheet,
		Stock:     p.Stock,
		MinBuy:    p.MinBuy,
	}
	for _, pr := range p.Prices {
		if pr.USD > 0 {
			out.Prices = append(out.Prices, part.Price{Ladder: pr.Ladder, USD: pr.USD})
		}
	}
	// PriceAt stops at the first break above the quantity, so keep it ascending.
	sort.SliceStable(out.Prices, func(i, j int) bool { return out.Prices[i].Ladder < out.Prices[j].Ladder })

	for _, pr := range p.Params {
		if pr.Name != "" && pr.Value != "" {
			out.Params = append(out.Params, part.Param{Name: pr.Name, Value: pr.Value})
		}
	}
	return out
}
