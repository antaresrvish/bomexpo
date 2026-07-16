package lcsc

import (
	"fmt"
	"strings"
)

type Param struct {
	Name  string `json:"paramNameEn"`
	Value string `json:"paramValueEn"`
}

type Price struct {
	Ladder       int     `json:"ladder"`
	USD          float64 `json:"usdPrice"`
	ProductPrice string  `json:"productPrice"`
}

type stockVO struct {
	Total           int `json:"total"`
	ShipImmediately int `json:"shipImmediately"`
}

type Part struct {
	Code      string   `json:"productCode"`
	Model     string   `json:"productModel"`
	IntroEn   string   `json:"productIntroEn"`
	NameEn    string   `json:"productNameEn"`
	Brand     string   `json:"brandNameEn"`
	Category  string   `json:"catalogName"`
	ParentCat string   `json:"parentCatalogName"`
	Package   string   `json:"encapStandard"`
	Stock     int      `json:"stockNumber"`
	MinBuy    int      `json:"minBuyNumber"`
	ImageURL  string   `json:"productImageUrl"`
	Datasheet string   `json:"pdfUrl"`
	Prices    []Price  `json:"productPriceList"`
	Domestic  *stockVO `json:"domesticStockVO"`
	Overseas  *stockVO `json:"overseasStockVO"`
	Params    []Param  `json:"paramVOList"`
}

// primaryParam holds the params already shown as the value column, so Specs
// leaves them out.
var primaryParam = map[string]bool{
	"Capacitance": true, "Resistance": true, "Inductance": true, "Type": true,
}

// Specs is a compact string of the key electrical parameters (tolerance,
// voltage, dielectric, power…), excluding the primary value.
func (p Part) Specs() string {
	var out []string
	for _, pr := range p.Params {
		if pr.Value == "" || primaryParam[pr.Name] {
			continue
		}
		out = append(out, pr.Value)
	}
	return strings.Join(out, " ")
}

// SpecPairs returns the full labeled parameter list for a detail view.
func (p Part) SpecPairs() []Param { return p.Params }

// Param returns the value of the first matching parameter name, or "".
func (p Part) Param(names ...string) string {
	for _, want := range names {
		for _, pr := range p.Params {
			if pr.Name == want {
				return pr.Value
			}
		}
	}
	return ""
}

func (p Part) Description() string {
	switch {
	case p.IntroEn != "":
		return p.IntroEn
	case p.NameEn != "":
		return p.NameEn
	case p.Model != "":
		return p.Model
	default:
		return "—"
	}
}

func (p Part) InStock() bool { return p.Stock > 0 }

func (p Part) UnitPrice() (float64, bool) {
	if len(p.Prices) == 0 {
		return 0, false
	}
	best := p.Prices[0].USD
	for _, pr := range p.Prices[1:] {
		if pr.USD < best {
			best = pr.USD
		}
	}
	return best, true
}

// PriceAt returns the unit price for ordering qty units, using the ladder tier
// with the highest quantity break ≤ qty (or the smallest tier if qty is below
// the first break). Assumes Prices are ascending by Ladder, as LCSC returns.
func (p Part) PriceAt(qty int) (float64, bool) {
	if len(p.Prices) == 0 {
		return 0, false
	}
	price := p.Prices[0].USD
	for _, pr := range p.Prices {
		if pr.Ladder <= qty {
			price = pr.USD
		} else {
			break
		}
	}
	return price, true
}

func (p Part) PriceLabel() string {
	u, ok := p.UnitPrice()
	if !ok {
		return "—"
	}
	return fmt.Sprintf("$%.4g", u)
}

type Category struct {
	ID       int    `json:"categoryId"`
	ParentID *int   `json:"parentId"`
	NameEn   string `json:"categoryNameEn"`
}

type SearchResult struct {
	Items    []Part
	Total    int
	Page     int
	PageSize int
}
