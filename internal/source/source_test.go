package source

import (
	"strings"
	"testing"

	"bomexpo/internal/config"
	"bomexpo/internal/part"
)

func TestNewExposesBothSources(t *testing.T) {
	ps := New()
	if got, want := strings.Join(IDs(ps), ","), "lcsc,jlcpcb"; got != want {
		t.Fatalf("sources = %s, want %s", got, want)
	}
	for _, p := range ps {
		if ok, hint := p.Ready(); !ok {
			t.Errorf("%s is not ready out of the box: %s", p.ID(), hint)
		}
		if p.Label() == "" {
			t.Errorf("%s has no display label", p.ID())
		}
	}
	// only the assembler knows about basic/extended libraries
	if ps[0].Caps().Library || !ps[1].Caps().Library {
		t.Error("Caps().Library should be false for lcsc and true for jlcpcb")
	}
	if !ps[1].Caps().BasicFilter {
		t.Error("jlcpcb should advertise the basic-only filter")
	}
}

func TestIndex(t *testing.T) {
	ps := New()
	for _, c := range []struct {
		id   string
		want int
	}{
		{"lcsc", 0},
		{"jlcpcb", 1},
		{"JLCPCB", 1}, // case and padding shouldn't matter
		{"  lcsc ", 0},
		{"nope", -1},
		{"", -1},
	} {
		if got := Index(ps, c.id); got != c.want {
			t.Errorf("Index(%q) = %d, want %d", c.id, got, c.want)
		}
	}
}

func TestStart(t *testing.T) {
	ps := New()

	if i, unknown := Start(ps, config.Default(), ""); i != 0 || unknown != "" {
		t.Errorf("default config = %d/%q, want 0/empty", i, unknown)
	}
	if i, _ := Start(ps, config.Config{DefaultSource: "jlcpcb"}, ""); i != 1 {
		t.Errorf("configured default ignored, got %d", i)
	}
	// an explicit request beats the configured default
	if i, _ := Start(ps, config.Config{DefaultSource: "jlcpcb"}, "lcsc"); i != 0 {
		t.Errorf("explicit request ignored, got %d", i)
	}
	// an unknown request is reported, and we fall back rather than failing
	i, unknown := Start(ps, config.Config{DefaultSource: "jlcpcb"}, "mouser")
	if i != 1 || unknown != "mouser" {
		t.Errorf("unknown request = %d/%q, want 1/mouser", i, unknown)
	}
	// a config naming a source that no longer exists still starts up
	if i, _ := Start(ps, config.Config{DefaultSource: "gone"}, ""); i != 0 {
		t.Errorf("stale config default = %d, want 0", i)
	}
}

func TestProvidersSatisfyInterface(t *testing.T) {
	// a compile-time-ish guard that the adapters keep implementing everything
	var _ []part.Provider = New()
}
