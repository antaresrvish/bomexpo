package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	// Drive it the way the runtime does: run what a command returns, including the
	// batches Update hands back to top the lanes up. Discarding those left every
	// lane marked busy with nothing running, and the board half unchecked.
	m.padFill = true
	var drain func(tea.Cmd)
	drain = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		out := cmd()
		if batch, ok := out.(tea.BatchMsg); ok {
			for _, c := range batch {
				drain(c)
			}
			return
		}
		if out == nil {
			return
		}
		var next tea.Cmd
		mm, next = m.Update(out)
		m = mm.(Model)
		drain(next)
	}
	for guard := 0; guard < 400; guard++ {
		cmds := m.fitCmds()
		if len(cmds) == 0 {
			break
		}
		for _, c := range cmds {
			drain(c)
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
