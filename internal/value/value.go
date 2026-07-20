package value

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Kind int

const (
	Unknown Kind = iota
	Resistance
	Capacitance
	Inductance
)

func (k Kind) String() string {
	switch k {
	case Resistance:
		return "resistance"
	case Capacitance:
		return "capacitance"
	case Inductance:
		return "inductance"
	default:
		return "unknown"
	}
}

type Value struct {
	Kind Kind
	Base float64
}

var (
	reRes    = regexp.MustCompile(`^(\d+(?:\.\d+)?)([kKMGmR]?)$`)
	reResRKM = regexp.MustCompile(`^(\d*)([RkKMmG])(\d+)$`)
	reCap    = regexp.MustCompile(`^(\d+(?:\.\d+)?)([pnum]?)$`)
	reCapRKM = regexp.MustCompile(`^(\d*)([pnum])(\d+)$`)
	reInd    = regexp.MustCompile(`^(\d+(?:\.\d+)?)([pnum]?)$`)
)

func resMult(p string) float64 {
	switch p {
	case "k", "K":
		return 1e3
	case "M":
		return 1e6
	case "G":
		return 1e9
	case "m":
		return 1e-3
	default:
		return 1
	}
}

func smallMult(p string) float64 {
	switch p {
	case "p":
		return 1e-12
	case "n":
		return 1e-9
	case "u":
		return 1e-6
	case "m":
		return 1e-3
	default:
		return 1
	}
}

func rkm(intPart, frac string, mult float64) float64 {
	n := 0.0
	if intPart != "" {
		n, _ = strconv.ParseFloat(intPart, 64)
	}
	if frac != "" {
		f, _ := strconv.ParseFloat("0."+frac, 64)
		n += f
	}
	return n * mult
}

func Parse(s string) (Value, bool) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "µ", "u")
	s = strings.ReplaceAll(s, "μ", "u")
	s = strings.ReplaceAll(s, "\u2126", "\u03a9")
	if s == "" {
		return Value{}, false
	}

	low := strings.ToLower(s)
	switch {
	case strings.HasSuffix(s, "\u03a9"):
		return resistanceFrom(strings.TrimSuffix(s, "\u03a9"))
	case strings.HasSuffix(low, "ohms"):
		return resistanceFrom(s[:len(s)-4])
	case strings.HasSuffix(low, "ohm"):
		return resistanceFrom(s[:len(s)-3])
	case strings.HasSuffix(s, "F"):
		if v, ok := parseMagnitude(strings.TrimSuffix(s, "F"), smallMult, reCap, reCapRKM); ok {
			return Value{Capacitance, v}, true
		}
	case strings.HasSuffix(s, "H"):
		if v, ok := parseMagnitude(strings.TrimSuffix(s, "H"), smallMult, reInd, reCapRKM); ok {
			return Value{Inductance, v}, true
		}
	default:
		if v, ok := parseResistanceBare(s); ok {
			return Value{Resistance, v}, true
		}
	}
	return Value{}, false
}

func resistanceFrom(body string) (Value, bool) {
	if v, ok := parseMagnitude(strings.TrimSpace(body), resMult, reRes, reResRKM); ok {
		return Value{Resistance, v}, true
	}
	return Value{}, false
}

func parseMagnitude(body string, mult func(string) float64, re, reR *regexp.Regexp) (float64, bool) {
	body = strings.TrimSpace(body)
	if m := re.FindStringSubmatch(body); m != nil {
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, false
		}
		return n * mult(m[2]), true
	}
	if m := reR.FindStringSubmatch(body); m != nil {
		return rkm(m[1], m[3], mult(m[2])), true
	}
	return 0, false
}

func parseResistanceBare(s string) (float64, bool) {
	if m := reResRKM.FindStringSubmatch(s); m != nil {
		return rkm(m[1], m[3], resMult(m[2])), true
	}
	if m := reRes.FindStringSubmatch(s); m != nil {
		if m[2] == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, false
		}
		return n * resMult(m[2]), true
	}
	return 0, false
}

func ExtractValue(desc string) (Value, bool) {
	fields := strings.FieldsFunc(desc, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '/' || r == '('
	})
	for _, f := range fields {
		f = strings.Trim(f, "±%()")
		if v, ok := Parse(f); ok && v.Kind != Unknown {
			return v, true
		}
	}
	return Value{}, false
}

func Equal(a, b Value) bool {
	if a.Kind != b.Kind || a.Kind == Unknown {
		return false
	}
	if a.Base == 0 || b.Base == 0 {
		return a.Base == b.Base
	}
	return math.Abs(a.Base-b.Base)/math.Max(a.Base, b.Base) < 0.01
}

type Result struct {
	Design    Value
	Part      Value
	DesignHas bool
	PartHas   bool
	Match     bool
	Note      string
}

func Check(designValue, partDesc string) Result {
	var r Result
	r.Design, r.DesignHas = ExtractValue(designValue)
	r.Part, r.PartHas = ExtractValue(partDesc)

	switch {
	case !r.DesignHas:
		r.Match = true
		r.Note = "value not comparable"
	case !r.PartHas:
		r.Match = true
		r.Note = "part value not detected"
	case r.Design.Kind != r.Part.Kind:
		r.Note = "type mismatch: " + r.Design.Kind.String() + " vs " + r.Part.Kind.String()
	case Equal(r.Design, r.Part):
		r.Match = true
	default:
		r.Note = "value mismatch: " + Format(r.Design) + " vs " + Format(r.Part)
	}
	return r
}

func Format(v Value) string {
	unit := map[Kind]string{Resistance: "\u03a9", Capacitance: "F", Inductance: "H"}[v.Kind]
	n, prefix := v.Base, ""
	switch {
	case n >= 1e9:
		n, prefix = n/1e9, "G"
	case n >= 1e6:
		n, prefix = n/1e6, "M"
	case n >= 1e3:
		n, prefix = n/1e3, "k"
	case n >= 1:
	case n >= 1e-3:
		n, prefix = n/1e-3, "m"
	case n >= 1e-6:
		n, prefix = n/1e-6, "u"
	case n >= 1e-9:
		n, prefix = n/1e-9, "n"
	case n > 0:
		n, prefix = n/1e-12, "p"
	}
	return strconv.FormatFloat(n, 'g', 4, 64) + prefix + unit
}
