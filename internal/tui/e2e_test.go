package tui

import (
	"os"
	"strings"
	"testing"
)

// TestRealBoardFit drives the real thing: load a board, open Export, let the
// footprint fetches land, and read what the pre-flight says.
func TestRealBoardFit(t *testing.T) {
	if testing.Short() {
		t.Skip("live")
	}
	path := os.Getenv("BOMEXPO_PROJ")
	if path == "" {
		t.Skip("set BOMEXPO_PROJ")
	}
	m := New(Options{})
	m.w, m.h = 140, 44

	msg := loadProjectCmd(path, "")()
	mm, _ := m.Update(msg)
	m = mm.(Model)
	if m.err != "" {
		t.Fatalf("load: %s", m.err)
	}

	// ask for every part's geometry the way opening Export does, and give the
	// retries the passes they are allowed
	for pass := 0; pass < maxFitAttempts; pass++ {
		for _, cmd := range m.fitCmds() {
			if out := cmd(); out != nil {
				mm, _ = m.Update(out)
				m = mm.(Model)
			}
		}
	}

	bad, unknown, checked := m.fitCount()
	t.Logf("%d items · bad=%d unknown=%d checked=%d", len(m.items), bad, unknown, checked)
	for _, is := range m.issues() {
		if is.kind == stFootprint {
			t.Logf("  ISSUE %-8s %s", is.ref, is.label)
		}
	}
	for _, l := range m.preflightAndManifest(m.contentW()) {
		if s := stripANSI(l); strings.Contains(s, "pads") || strings.Contains(s, "footprint") || strings.Contains(s, "land") {
			t.Logf("  PREFLIGHT %s", strings.TrimSpace(s))
		}
	}
}
