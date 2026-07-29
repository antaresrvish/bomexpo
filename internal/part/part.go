// Package part holds the distributor-neutral part record every source returns,
// so the rest of the app never depends on one vendor's JSON shape.
package part

import (
	"fmt"
	"strings"
)

// Param is a single labeled parametric attribute ("Voltage Rating" → "16V").
type Param struct {
	Name  string
	Value string
}

// Price is one quantity break: USD unit price when ordering at least Ladder
// units.
type Price struct {
	Ladder int
	USD    float64
}

// LibKind is a part's standing in the assembler's own library, which decides
// whether an order pays a per-part setup fee. Sources that don't model this
// report LibUnknown.
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

// Known reports whether the source actually told us the library standing.
func (k LibKind) Known() bool { return k != LibUnknown }

// Part is one orderable component as a source describes it. Fields a given
// source doesn't supply stay zero — check Lib.Known() and AsmStock > 0 rather
// than assuming.
type Part struct {
	Source    string // provider ID that produced this record
	Code      string // the source's own part code
	MPN       string
	Brand     string
	Package   string
	Desc      string
	Datasheet string
	Stock     int
	MinBuy    int // minimum order quantity
	Prices    []Price
	Params    []Param

	// Assembly-specific, only from sources that quote assembly (see Caps).
	Lib    LibKind
	AsmMin int // minimum units consumed per assembly job
	Loss   int // units the assembler adds for attrition
}

// AsmUnits is how many units an assembly job actually consumes for need units
// of this part, once the assembler's minimum and attrition are applied.
func (p Part) AsmUnits(need int) int {
	n := need + p.Loss
	if p.AsmMin > n {
		n = p.AsmMin
	}
	return n
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
	case p.Desc != "":
		return p.Desc
	case p.MPN != "":
		return p.MPN
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
// the first break). Assumes Prices are ascending by Ladder.
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
