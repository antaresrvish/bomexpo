// Package easyeda reads a part's land pattern, so a part that isn't on your board
// can still be drawn. EasyEDA is the CAD side of the LCSC/JLCPCB family and serves
// this without a key. Only the pads are read; symbols and 3D models are left.
package easyeda

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"bomexpo/internal/kicad"
	"bomexpo/internal/webjson"
)

const (
	base = "https://easyeda.com"
	site = "https://easyeda.com/"

	// EasyEDA works in 10-mil units. Verified against parts with a known pitch:
	// a 2.54mm header comes out 10 units apart, and an 0.5mm-pitch LQFP 1.969.
	unitMM = 0.254

	// A part's land pattern doesn't change the way its stock and price do, so the
	// default day-long freshness would spend requests re-reading settled geometry —
	// and EasyEDA answers a burst with 403.
	cacheTTL = 30 * 24 * time.Hour
)

type Client struct {
	w *webjson.Client
}

func New() *Client {
	w := webjson.New("easyeda", site)
	w.SetCacheTTL(cacheTTL)
	return &Client{w: w}
}

type Footprint struct {
	Code    string
	Package string
	Lands   []kicad.Land
}

type envelope struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type result struct {
	Package struct {
		DataStr struct {
			Head struct {
				X     float64        `json:"x"`
				Y     float64        `json:"y"`
				CPara map[string]any `json:"c_para"`
			} `json:"head"`
			Shape []string `json:"shape"`
		} `json:"dataStr"`
	} `json:"packageDetail"`
}

// Fetch reads from the on-disk cache when it's fresh.
func (c *Client) Fetch(code string) (Footprint, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Footprint{}, fmt.Errorf("easyeda: no part code")
	}
	if raw, fresh, ok := c.w.CacheGet(code); ok && fresh {
		if fp, err := parse(code, raw); err == nil {
			return fp, nil
		}
	}

	data, err := c.w.Fetch("GET", base+"/api/products/"+code+"/components?version=6.4.19.5", nil)
	if err != nil {
		// offline: a stale copy beats nothing
		if raw, _, ok := c.w.CacheGet(code); ok {
			if fp, perr := parse(code, raw); perr == nil {
				return fp, nil
			}
		}
		return Footprint{}, err
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Footprint{}, fmt.Errorf("easyeda: decode: %w", err)
	}
	if !env.Success || len(env.Result) == 0 {
		msg := env.Message
		if msg == "" {
			msg = "no data for " + code
		}
		return Footprint{}, fmt.Errorf("easyeda: %s", msg)
	}

	fp, err := parse(code, env.Result)
	if err != nil {
		return Footprint{}, err
	}
	c.w.CachePut(code, env.Result)
	return fp, nil
}

func parse(code string, raw []byte) (Footprint, error) {
	var r result
	if err := json.Unmarshal(raw, &r); err != nil {
		return Footprint{}, fmt.Errorf("easyeda: decode package: %w", err)
	}
	ds := r.Package.DataStr
	if len(ds.Shape) == 0 {
		return Footprint{}, fmt.Errorf("easyeda: %s has no footprint", code)
	}

	fp := Footprint{Code: code}
	if v, ok := ds.Head.CPara["package"].(string); ok {
		fp.Package = v
	}
	// shapes are document-space; the head's x/y is the footprint origin
	ox, oy := ds.Head.X, ds.Head.Y
	for _, s := range ds.Shape {
		if l, ok := parsePad(s, ox, oy); ok {
			fp.Lands = append(fp.Lands, l)
		}
	}
	if len(fp.Lands) == 0 {
		return Footprint{}, fmt.Errorf("easyeda: %s has no pads", code)
	}
	return fp, nil
}

// parsePad reads one PAD shape:
//
//	PAD~shape~x~y~w~h~layer~net~number~holeR~points~rot~id~…
//
// Layer 1 is top copper, 2 bottom, 11 a through-hole. A hole radius means drilled.
func parsePad(s string, ox, oy float64) (kicad.Land, bool) {
	if !strings.HasPrefix(s, "PAD~") {
		return kicad.Land{}, false
	}
	f := strings.Split(s, "~")
	if len(f) < 10 {
		return kicad.Land{}, false
	}

	w, h := num(f[4])*unitMM, num(f[5])*unitMM
	if w <= 0 || h <= 0 {
		return kicad.Land{}, false
	}
	// a pad turned on its side swaps its extents, same as the pcb parser does
	if len(f) > 11 {
		if rot := math.Mod(math.Abs(num(f[11])), 180); rot > 45 && rot < 135 {
			w, h = h, w
		}
	}

	name := strings.TrimSpace(f[8])
	return kicad.Land{
		Name:  name,
		X:     (num(f[2]) - ox) * unitMM,
		Y:     (num(f[3]) - oy) * unitMM,
		W:     w,
		H:     h,
		Round: strings.EqualFold(f[1], "ELLIPSE") || strings.EqualFold(f[1], "OVAL"),
		Hole:  num(f[9]) > 0,
		First: name == "1" || name == "A1",
	}, true
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
