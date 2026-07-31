package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Load a real board, fetch every part's stock, set the target, read the pre-flight.
func TestRealBoardStock(t *testing.T) {
	if testing.Short() {
		t.Skip("live")
	}
	path := os.Getenv("BOMEXPO_PROJ")
	boards, _ := strconv.Atoi(os.Getenv("BOARDS"))
	if path == "" || boards == 0 {
		t.Skip("set BOMEXPO_PROJ and BOARDS")
	}
	m := New(Options{})
	m.w, m.h = 140, 46
	mm, _ := m.Update(loadProjectCmd(path, "")())
	m = mm.(Model)
	if m.err != "" {
		t.Fatalf("load: %s", m.err)
	}

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
	drain(m.prefillCmd())
	m.padFill = true
	for guard := 0; guard < 400; guard++ {
		cmds := m.asmCmds()
		if len(cmds) == 0 {
			break
		}
		for _, c := range cmds {
			drain(c)
		}
	}

	m.check.target = boards
	missing, checked, pending := m.asmTally()
	t.Logf("assembly library: %d missing, %d found, %d pending", missing, checked, pending)
	for _, is := range m.issues() {
		if is.kind == stUnplaceable {
			t.Logf("  NO-ASM %-10s %s", is.ref, is.label)
		}
	}
	t.Logf("%d boards · %d short · assigned %d", boards, m.shortCount(), len(m.assigned))
	for _, is := range m.issues() {
		if is.kind == stShort {
			t.Logf("  SHORT %-10s %s", is.ref, is.label)
		}
	}
	if n, tightest := m.thinStock(); n > 0 {
		t.Logf("  thin: %d, tightest %s", n, tightest)
	}
	for _, l := range m.preflightAndManifest(m.contentW()) {
		if s := stripANSI(l); strings.Contains(s, "stock") || strings.Contains(s, "cover") || strings.Contains(s, "fill") || strings.Contains(s, "assemb") || strings.Contains(s, "place") {
			t.Logf("  PREFLIGHT %s", strings.TrimSpace(s))
		}
	}
}
