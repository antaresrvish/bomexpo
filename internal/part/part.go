// Package part holds the distributor-neutral part record every source returns.
package part

import (
	"fmt"
	"strings"
)

// Param is one attribute: "Voltage Rating" → "16V".
type Param struct {
	Name  string
	Value string
}

// Price is the unit price when ordering at least Ladder units.
type Price struct {
	Ladder int
	USD    float64
}

// LibKind is the part's standing in the assembler's library, which decides
// whether an order pays a per-part setup fee.
type LibKind int

const (
	LibUnknown LibKind = iota
	LibBasic
	LibPreferred
	LibExtended
)

func (k LibKind) String() string {
	switch k {
	case LibBasic:
		return "Basic"
	case LibPreferred:
		return "Preferred"
	case LibExtended:
		return "Extended"
	default:
		return "unknown"
	}
}

func (k LibKind) Known() bool { return k != LibUnknown }

// Part is one orderable component. Fields a source doesn't supply stay zero.
type Part struct {
	Source    string // provider ID that produced this record
	Code      string // the source's own part code
	MPN       string
	Brand     string
	Package   string
	Desc      string
	Datasheet string
	// "Multilayer Ceramic Capacitors MLCC - SMD/SMT" under "Capacitors"
	Category  string
	ParentCat string
	Stock     int
	MinBuy    int // minimum order quantity
	Prices    []Price
	Params    []Param

	// only from sources that quote assembly, see Caps
	Lib    LibKind
	AsmMin int // minimum units consumed per assembly job
	Loss   int // units the assembler adds for attrition
}

// AsmUnits applies the assembler's minimum and attrition to need units.
func (p Part) AsmUnits(need int) int {
	n := need + p.Loss
	if p.AsmMin > n {
		n = p.AsmMin
	}
	return n
}

// already shown in the value column, so Specs leaves them out
var primaryParam = map[string]bool{
	"Capacitance": true, "Resistance": true, "Inductance": true, "Type": true,
}

// Specs is tolerance, voltage, dielectric and the like, without the value.
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

func (p Part) SpecPairs() []Param { return p.Params }

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
	case p.Desc != "":
		return p.Desc
	case p.MPN != "":
		return p.MPN
	default:
		return "—"
	}
}

func (p Part) InStock() bool { return p.Stock > 0 }

// BuyQty is how many pieces an order of n actually buys: the vendor's minimum when
// that is more than you need.
func (p Part) BuyQty(n int) int {
	if p.MinBuy > n {
		return p.MinBuy
	}
	return n
}

// Covers reports whether stock can fill an order of n. InStock only asks whether any
// exist, which passes a part with ten of them against a run of three hundred boards.
func (p Part) Covers(n int) bool {
	if n <= 0 {
		return true
	}
	return p.Stock >= p.BuyQty(n)
}

// Headroom is stock over what an order of n needs. Under 1 the order cannot be
// filled; a little over 1 is worth a look, since stock moves between quote and order.
func (p Part) Headroom(n int) float64 {
	need := p.BuyQty(n)
	if need <= 0 {
		return 0
	}
	return float64(p.Stock) / float64(need)
}

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

// PriceAt takes the highest break ≤ qty, or the smallest tier below the first
// break. Prices must be ascending by Ladder.
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
