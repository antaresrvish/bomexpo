// Package source wires the concrete parts sources together. It exists as its
// own package so the vendor clients can depend on the neutral part types
// without anything depending back on them.
package source

import (
	"strings"

	"bomexpo/internal/config"
	"bomexpo/internal/jlcpcb"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/part"
)

// New returns every built-in source, in display order. Call it once — each
// source owns an HTTP client and a cache directory.
func New() []part.Provider {
	return []part.Provider{
		lcsc.New().Provider(),
		jlcpcb.New(),
	}
}

// Index finds a source by ID, or -1 when there's no such source.
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

// Start picks which source to open with: the requested one if it exists, then
// the configured default, then the first available. An unknown request is
// reported so the caller can say so rather than silently ignoring the flag.
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

// IDs lists the source IDs, for help text and error messages.
func IDs(ps []part.Provider) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID())
	}
	return out
}
