// Package source wires the parts sources together. Separate package so the vendor
// clients can depend on the neutral part types with nothing depending back.
package source

import (
	"strings"

	"bomexpo/internal/config"
	"bomexpo/internal/jlcpcb"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/part"
)

// New returns every source in display order. Call it once: each owns an HTTP
// client and a cache directory.
func New() []part.Provider {
	return []part.Provider{
		lcsc.New().Provider(),
		jlcpcb.New(),
	}
}

// Index is -1 when there's no such source.
func Index(ps []part.Provider, id string) int {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return -1
	}
	for i, p := range ps {
		if p.ID() == id {
			return i
		}
	}
	return -1
}

// Start prefers the requested source, then the configured default, then the
// first. An unknown request is reported rather than ignored.
func Start(ps []part.Provider, cfg config.Config, want string) (idx int, unknown string) {
	if want != "" {
		if i := Index(ps, want); i >= 0 {
			return i, ""
		}
		unknown = want
	}
	if i := Index(ps, cfg.DefaultSource); i >= 0 {
		return i, unknown
	}
	return 0, unknown
}

func IDs(ps []part.Provider) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID())
	}
	return out
}
