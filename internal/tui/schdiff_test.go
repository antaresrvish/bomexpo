package tui

import (
	"os"
	"path/filepath"
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
		Findings: findings, Rows: sampleRows(), SchCount: 10, BOMCount: 9,
		Matched: 4, SkippedDNP: 2,
	}
	return m
}

// sampleRows is two designators that agree and three that don't, two of those
// seriously.
func sampleRows() []kicad.Row {
	return []kicad.Row{
		{Ref: "R2", InSch: true, SchValue: "1k", SchFootprint: "R_0603_1608Metric",
			Kinds: []kicad.DiffKind{kicad.DiffMissing}},
		{Ref: "C2", InSch: true, InBOM: true, SchValue: "1uF", BOMValue: "1uF", DNP: true,
			Kinds: []kicad.DiffKind{kicad.DiffDNP}},
		{Ref: "C1", InSch: true, InBOM: true, SchFootprint: "C_0603_1608Metric",
			BOMFootprint: "C_0805_2012Metric", Kinds: []kicad.DiffKind{kicad.DiffFootprint}},
		{Ref: "R1", InSch: true, InBOM: true, SchValue: "10k", BOMValue: "10k"},
		{Ref: "R3", InSch: true, InBOM: true, SchValue: "2k", BOMValue: "2k"},
	}
}

func sampleFindings() []kicad.Finding {
	return []kicad.Finding{
		{Kind: kicad.DiffMissing, Ref: "R2", Sch: "1k · R_0603_1608Metric", BOM: "—"},
		{Kind: kicad.DiffDNP, Ref: "C2", Sch: "dnp · 1uF", BOM: "1uF · C1592"},
		{Kind: kicad.DiffFootprint, Ref: "C1", Sch: "C_0603_1608Metric", BOM: "C_0805_2012Metric"},
		{Kind: kicad.DiffExcluded, Ref: "H1", Sch: "excluded · MountingHole", BOM: "MountingHole"},
	}
}

// s narrows from every designator, to the differences, to the serious ones, and
// back. The default has to be everything: a list of problems only is
// indistinguishable from a comparison that never ran.
func TestDiffFilterCyclesFromEverything(t *testing.T) {
	m := diffModel(t)
	if m.diff.show != showAll {
		t.Fatal("the tab should open showing every designator")
	}
	if got := len(m.diff.rows()); got != 5 {
		t.Fatalf("%d rows unfiltered, want all 5", got)
	}

	mm, _ := m.updateDiffKey(key("s"))
	m = mm.(Model)
	if got := len(m.diff.rows()); got != 3 {
		t.Fatalf("%d rows on the differences, want 3", got)
	}
	for _, r := range m.diff.rows() {
		if r.Agrees() {
			t.Errorf("%s agrees but survived the differences filter", r.Ref)
		}
	}

	mm, _ = m.updateDiffKey(key("s"))
	m = mm.(Model)
	for _, r := range m.diff.rows() {
		if !r.Severe() {
			t.Errorf("%s is not severe but survived", r.Ref)
		}
	}

	mm, _ = m.updateDiffKey(key("s"))
	if got := mm.(Model).diff.show; got != showAll {
		t.Errorf("s should cycle back to everything, got %v", got)
	}
}

// A filter that empties the list says so and names the way out.
func TestDiffEmptyFilterSaysHowManyItHid(t *testing.T) {
	m := diffModel(t)
	m.diff.show = showSerious
	m.diff.res.Rows = []kicad.Row{{Ref: "R1", InSch: true, InBOM: true}}
	out := stripANSI(m.viewDiff(m.contentW(), m.contentH()))
	if !strings.Contains(out, "1 designators hidden") {
		t.Errorf("the filter hid a row without saying so:\n%s", out)
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

// completeDir builds a directory to complete against.
func completeDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(strings.TrimSuffix(p, "/"), 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte("Designator\nR1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// diffField is the Diff tab with its path field focused and preloaded.
func diffField(t *testing.T, value string) Model {
	t.Helper()
	m := diffModel(t)
	m.diff.ran = false
	m.diff.field.Focus()
	m.diff.field.SetValue(value)
	return m
}

// The field only wants a csv, so completion must not land on files it would reject.
func TestDiffCompletionOffersOnlyCsvAndFolders(t *testing.T) {
	dir := completeDir(t, "board.kicad_pcb", "bom.csv", "sub/")
	_, names := diffCandidates(dir + "/")
	if strings.Join(names, ",") != "sub/,bom.csv" {
		t.Errorf("candidates = %v, want the folder and the csv only", names)
	}

	// a lone csv completes all the way
	m := diffField(t, filepath.Join(dir, "b"))
	mm, _ := m.updateDiffKey(key("tab"))
	if got := mm.(Model).diff.field.Value(); got != filepath.Join(dir, "bom.csv") {
		t.Errorf("tab gave %q, want the csv completed", got)
	}
}

// Tab extends as far as the shared prefix allows and no further.
func TestDiffCompletionStopsAtTheSharedPrefix(t *testing.T) {
	dir := completeDir(t, "bom-old.csv", "bom-new.csv")
	m := diffField(t, filepath.Join(dir, "b"))
	mm, _ := m.updateDiffKey(key("tab"))
	m = mm.(Model)
	if got := m.diff.field.Value(); got != filepath.Join(dir, "bom-") {
		t.Errorf("tab gave %q, want it to stop at bom-", got)
	}
	// pressing again can't add anything, and must not throw the field away
	mm, _ = m.updateDiffKey(key("tab"))
	if !mm.(Model).diff.field.Focused() {
		t.Error("an ambiguous tab lost the field — a shell would list and wait")
	}
}

// With nothing left to match, tab behaves like tab everywhere else and hands the
// page back.
func TestDiffCompletionWithNoCandidatesLeavesTheField(t *testing.T) {
	dir := completeDir(t, "board.kicad_pcb")
	for _, in := range []string{filepath.Join(dir, "zzz"), dir + "/"} {
		m := diffField(t, in)
		mm, _ := m.updateDiffKey(key("tab"))
		if mm.(Model).diff.field.Focused() {
			t.Errorf("%q: nothing to complete, so tab should hand the page back", in)
		}
	}
}

// Completing a path invalidates the report, which belonged to the old one.
func TestDiffCompletionDropsTheOldReport(t *testing.T) {
	dir := completeDir(t, "bom.csv")
	m := diffField(t, filepath.Join(dir, "b"))
	m.diff.ran = true
	mm, _ := m.updateDiffKey(key("tab"))
	if mm.(Model).diff.ran {
		t.Error("the report should not survive a path change")
	}
}

// The candidates are on screen, so tab isn't a guess.
func TestDiffShowsCompletionCandidates(t *testing.T) {
	dir := completeDir(t, "bom.csv", "positions.csv")
	m := diffField(t, dir+"/")
	out := stripANSI(m.viewDiff(m.contentW(), m.contentH()))
	if !strings.Contains(out, "tab completes") {
		t.Errorf("no hint about tab:\n%s", out)
	}
	for _, want := range []string{"bom.csv", "positions.csv"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is not offered on screen:\n%s", want, out)
		}
	}
}

// Load's completion must keep taking every file type — the csv filter is this
// field's, not everyone's.
func TestLoadCompletionStillTakesBoards(t *testing.T) {
	dir := completeDir(t, "board.kicad_pcb")
	got, ok := completePath(filepath.Join(dir, "bo"))
	if !ok || got != filepath.Join(dir, "board.kicad_pcb") {
		t.Errorf("completePath gave %q (%v), want the board", got, ok)
	}
}
