package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
)

func diffModel(t *testing.T, findings ...kicad.Finding) Model {
	t.Helper()
	m := filterModel(t)
	m.w, m.h = 132, 30
	m.pcbPath = "/tmp/x.kicad_pcb"
	mm, _ := m.gotoTab(modeDiff)
	m = mm.(Model)
	m.diff.ran = true
	m.diff.res = kicad.SchDiff{
		SchPath: "/tmp/x.kicad_sch", BOMPath: "/tmp/theirs.csv",
		Findings: findings, SchCount: 10, BOMCount: 9, Matched: 4, SkippedDNP: 2,
	}
	return m
}

func sampleFindings() []kicad.Finding {
	return []kicad.Finding{
		{Kind: kicad.DiffMissing, Ref: "R2", Sch: "1k · R_0603_1608Metric", BOM: "—"},
		{Kind: kicad.DiffDNP, Ref: "C2", Sch: "dnp · 1uF", BOM: "1uF · C1592"},
		{Kind: kicad.DiffFootprint, Ref: "C1", Sch: "C_0603_1608Metric", BOM: "C_0805_2012Metric"},
		{Kind: kicad.DiffExcluded, Ref: "H1", Sch: "excluded · MountingHole", BOM: "MountingHole"},
	}
}

// s hides everything that wouldn't spoil an order, and says how many it hid rather
// than looking like a clean bill of health.
func TestDiffSevereOnlyFilterSaysWhatItHid(t *testing.T) {
	m := diffModel(t, sampleFindings()...)
	if got := len(m.diff.findings()); got != 4 {
		t.Fatalf("%d findings unfiltered, want 4", got)
	}
	mm, _ := m.updateDiffKey(key("s"))
	m = mm.(Model)
	if got := len(m.diff.findings()); got != 2 {
		t.Fatalf("%d findings with the filter on, want the 2 severe", got)
	}
	for _, f := range m.diff.findings() {
		if !f.Kind.Severe() {
			t.Errorf("%v survived the severe-only filter", f.Kind)
		}
	}

	// with nothing severe at all, the hidden count has to be on screen
	only := diffModel(t, kicad.Finding{Kind: kicad.DiffFootprint, Ref: "C1", Sch: "a", BOM: "b"})
	mm, _ = only.updateDiffKey(key("s"))
	out := stripANSI(mm.(Model).viewDiff(only.contentW(), only.contentH()))
	if !strings.Contains(out, "1 lesser findings hidden") {
		t.Errorf("the filter hid a finding without saying so:\n%s", out)
	}
}

// The report has to admit the DNP parts it deliberately skipped, or the schematic
// and BOM totals look like they disagree for no reason.
func TestDiffViewShowsSkippedDNP(t *testing.T) {
	m := diffModel(t, sampleFindings()...)
	out := stripANSI(m.viewDiff(m.contentW(), m.contentH()))
	if !strings.Contains(out, "2 dnp rightly absent") {
		t.Errorf("the skipped dnp count is not on screen:\n%s", out)
	}
}

// A sub-sheet that couldn't be read must be named, so half a comparison never
// passes for a whole one.
func TestDiffViewNamesUnreadSheets(t *testing.T) {
	m := diffModel(t)
	sc := &kicad.Schematic{Path: "/tmp/x.kicad_sch", Skipped: []string{"power.kicad_sch"}}
	m.diff.res = kicad.DiffSchematicBOM(sc, nil)
	m.diff.res.BOMPath = "/tmp/theirs.csv"
	out := stripANSI(m.viewDiff(m.contentW(), m.contentH()))
	if !strings.Contains(out, "unread sheet power.kicad_sch") {
		t.Errorf("an unread sheet went unmentioned:\n%s", out)
	}
}

// Editing the path invalidates the report on screen: it belonged to the old file.
func TestDiffTypingDropsTheOldReport(t *testing.T) {
	m := diffModel(t, sampleFindings()...)
	m.diff.field.Focus()
	mm, _ := m.updateDiffKey(key("x"))
	if mm.(Model).diff.ran {
		t.Error("the report should not outlive the path it came from")
	}
}

// Every line stays inside the panel at any size, or the border shifts.
func TestDiffWidthHolds(t *testing.T) {
	m := diffModel(t, sampleFindings()...)
	for _, size := range [][2]int{{80, 22}, {100, 26}, {132, 30}, {170, 44}} {
		m.w, m.h = size[0], size[1]
		w, h := m.contentW(), m.contentH()
		lines := strings.Split(m.viewDiff(w, h), "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), h)
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got > w {
				t.Errorf("%dx%d: line %d is %d columns, over the %d available",
					size[0], size[1], i, got, w)
			}
		}
	}
}

// With no design open the tab says so rather than failing on a search.
func TestDiffWithNoDesignExplainsItself(t *testing.T) {
	m := New(Options{})
	m.w, m.h = 120, 26
	mm, _ := m.gotoTab(modeDiff)
	m = mm.(Model)
	out := stripANSI(m.viewDiff(m.contentW(), m.contentH()))
	if !strings.Contains(out, "no design open") {
		t.Errorf("expected an explanation:\n%s", out)
	}
	m.diff.field.SetValue("/tmp/whatever.csv")
	_, cmd := m.startDiff()
	if cmd == nil {
		t.Fatal("want a command that reports the problem")
	}
	msg, ok := cmd().(diffDoneMsg)
	if !ok || msg.err == nil {
		t.Errorf("want an error message back, got %#v", msg)
	}
}
