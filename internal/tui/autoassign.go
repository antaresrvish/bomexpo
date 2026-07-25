package tui

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/value"
)

var (
	reVolt = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*V\b`)
	reTol  = regexp.MustCompile(`±?\s*(\d+(?:\.\d+)?)\s*%`)
	reDiel = regexp.MustCompile(`\b(C0G|NP0|X7R|X5R|X6S|X7S|X7T|Y5V|Z5U|Y5U)\b`)
)

type specs struct {
	volt, tol  float64
	diel       string
	hasV, hasT bool
	hasD       bool
}

func parseSpecs(s string) specs {
	var sp specs
	up := strings.ToUpper(s)
	if m := reVolt.FindStringSubmatch(up); m != nil {
		sp.volt, _ = strconv.ParseFloat(m[1], 64)
		sp.hasV = true
	}
	if m := reTol.FindStringSubmatch(up); m != nil {
		sp.tol, _ = strconv.ParseFloat(m[1], 64)
		sp.hasT = true
	}
	if m := reDiel.FindStringSubmatch(up); m != nil {
		sp.diel, sp.hasD = m[1], true
	}
	return sp
}

func dielRank(d string) int {
	switch d {
	case "C0G", "NP0":
		return 4
	case "X7R", "X7S", "X7T":
		return 3
	case "X5R", "X6S":
		return 2
	default:
		return 0
	}
}

func unstableDiel(d string) bool {
	switch d {
	case "Y5V", "Z5U", "Y5U", "Z5V":
		return true
	}
	return false
}

// pickBest chooses the best in-stock LCSC part for a line: it must match the
// package, the exact value+type (passives) or MPN (ICs), and any explicit
// voltage/tolerance/dielectric spelled out in the value; unstable ceramics are
// dropped unless asked for. Candidates rank by enough-stock, price, dielectric
// quality, then stock.
func pickBest(it kicad.Item, kind value.Kind, pkg string, results []lcsc.Part) (lcsc.Part, bool) {
	// A passive whose footprint has no chip size code (electrolytic, tantalum,
	// molded inductor, special cans) can't have its physical package matched
	// reliably — don't guess, leave it for manual selection.
	if kind != value.Unknown && pkg == "" {
		return lcsc.Part{}, false
	}

	target, hasTarget := value.ExtractValue(it.Value)
	want := parseSpecs(it.Value)
	isCap := kind == value.Capacitance

	var cands []lcsc.Part
	for _, p := range results {
		if p.Stock <= 0 {
			continue
		}
		if pkg != "" && !strings.EqualFold(strings.TrimSpace(p.Package), pkg) {
			continue
		}
		desc := p.Description()
		if kind != value.Unknown {
			pv, ok := value.ExtractValue(desc)
			if !ok || pv.Kind != kind {
				continue
			}
			if hasTarget && !value.Equal(pv, target) {
				continue
			}
		} else if !matchesMPN(it.Value, p) {
			continue
		}

		ps := parseSpecs(desc)
		if isCap && want.hasV && ps.hasV && ps.volt < want.volt {
			continue
		}
		if want.hasT && ps.hasT && ps.tol > want.tol+1e-9 {
			continue
		}
		if want.hasD {
			if ps.hasD && ps.diel != want.diel {
				continue
			}
		} else if isCap && ps.hasD && unstableDiel(ps.diel) {
			continue
		}
		cands = append(cands, p)
	}
	if len(cands) == 0 {
		return lcsc.Part{}, false
	}

	need := it.Quantity
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if ea, eb := a.Stock >= need, b.Stock >= need; ea != eb {
			return ea
		}
		pa, oka := a.UnitPrice()
		pb, okb := b.UnitPrice()
		if oka && okb && pa != pb {
			return pa < pb
		}
		if da, db := dielRank(parseSpecs(a.Description()).diel), dielRank(parseSpecs(b.Description()).diel); da != db {
			return da > db
		}
		return a.Stock > b.Stock
	})
	return cands[0], true
}

func matchesMPN(val string, p lcsc.Part) bool {
	needle := normMPN(val)
	if needle == "" {
		return false
	}
	return strings.Contains(normMPN(p.Model), needle) ||
		strings.Contains(normMPN(p.Description()), needle)
}

func normMPN(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
