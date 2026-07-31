package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/easyeda"
	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

func pads(n int) []kicad.Land {
	out := make([]kicad.Land, n)
	for i := range out {
		out[i] = kicad.Land{W: 0.5, H: 0.5}
	}
	return out
}

// fitModel is R12 with the resistor array that reached a finished board, and R1
// with an ordinary 0402 that fits.
func fitModel() Model {
	m := New(Options{})
	m.pcbPath = "/tmp/board.kicad_pcb" // the land geometry comes from a board
	m.items = []kicad.Item{
		{Bases: []string{"R12"}, Value: "27R", Footprint: "R_0402_1005Metric", LCSC: "C2006", Quantity: 1},
		{Bases: []string{"R1"}, Value: "150k", Footprint: "R_0402_1005Metric", LCSC: "C93947", Quantity: 1},
	}
	m.assigned = []*part.Part{
		{Source: "lcsc", Code: "C2006", Stock: 5000, Prices: []part.Price{{Ladder: 1, USD: 0.01}}, Desc: "4 ±5% 27Ω resistor networks, arrays"},
		{Source: "lcsc", Code: "C93947", Stock: 5000, Prices: []part.Price{{Ladder: 1, USD: 0.01}}, Desc: "150kΩ 0402 thick film resistor"},
	}
	m.excluded = make([]bool, 2)
	m.designLands = map[string][]kicad.Land{"R_0402_1005Metric": pads(2)}
	m.edaLands = map[string]easyeda.Footprint{
		"C2006":  {Code: "C2006", Package: "RES-ARRAY-SMD_0402-8P", Lands: pads(8)},
		"C93947": {Code: "C93947", Package: "R0402", Lands: pads(2)},
	}
	m.edaTried = map[string]int{"C2006": 1, "C93947": 1}
	m = m.reindex()
	return m
}

func TestPartWithMorePadsThanItsLandIsAnIssue(t *testing.T) {
	m := fitModel()
	if got := m.stateOf(0); got != stFootprint {
		t.Fatalf("R12 state = %v, want stFootprint", got)
	}
	if got := m.stateOf(1); got != stOK {
		t.Fatalf("R1 state = %v, want stOK", got)
	}

	issues := m.issues()
	if len(issues) != 1 || issues[0].ref != "R12" {
		t.Fatalf("issues = %+v, want one for R12", issues)
	}
	if !strings.Contains(issues[0].label, "8 pads") || !strings.Contains(issues[0].label, "2") {
		t.Errorf("the label should give both counts, got %q", issues[0].label)
	}
}

// The land having more pads than the part is routine — thermal vias, paste-only
// pads and paired mounting pads — and must stay quiet.
func TestALandWithSparePadsIsNotAnIssue(t *testing.T) {
	m := fitModel()
	m.designLands["R_0402_1005Metric"] = pads(22)
	m.edaLands["C2006"] = easyeda.Footprint{Code: "C2006", Lands: pads(11)}
	if got := m.stateOf(0); got != stOK {
		t.Fatalf("state = %v, want stOK for a land with pads to spare", got)
	}
}

// Until the vendor answers there is no verdict, and the pre-flight has to say so
// rather than showing a pass it hasn't earned.
func TestUnknownGeometryIsNotAPass(t *testing.T) {
	m := fitModel()
	delete(m.edaLands, "C2006")
	if got := m.stateOf(0); got != stOK {
		t.Fatalf("state = %v, want stOK while the geometry is unknown", got)
	}
	bad, unknown, checked := m.fitCount()
	if bad != 0 || unknown != 1 || checked != 1 {
		t.Fatalf("fitCount = (%d, %d, %d), want (0, 1, 1)", bad, unknown, checked)
	}
	m.w, m.h = 130, 40
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "checking 1 parts against their lands") {
		t.Errorf("pre-flight hid the unfinished check:\n%s", out)
	}
}

func TestPreflightNamesTheFootprintFault(t *testing.T) {
	m := fitModel()
	m.w, m.h = 130, 40
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "1 parts have more pads than their land") {
		t.Errorf("pre-flight missed the fault:\n%s", out)
	}
}

// A part that cannot be soldered outranks one with the wrong value printed on it.
func TestFootprintOutranksValueMismatch(t *testing.T) {
	m := fitModel()
	m.items[0].Value = "999R" // also a value mismatch now
	if got := m.stateOf(0); got != stFootprint {
		t.Fatalf("state = %v, want stFootprint to win over stMismatch", got)
	}
}

func TestFilterSelectsFootprintFaults(t *testing.T) {
	m := fitModel()
	if !(filterTerm{key: "st", want: "footprint"}).hitState(m, 0) {
		t.Error("st:footprint missed R12")
	}
	for _, alias := range []string{"fp", "land"} {
		if !(filterTerm{key: "st", want: alias}).hitState(m, 0) {
			t.Errorf("st:%s missed R12", alias)
		}
	}
	if (filterTerm{key: "st", want: "footprint"}).hitState(m, 1) {
		t.Error("st:footprint matched the part that fits")
	}
}

// A board loaded from disk has codes but no fetched part details. The fault has to
// surface anyway, or the issue list and the export confirmation both stay silent on
// the one thing that ruins a board.
func TestFaultSurfacesWithoutFetchedPartDetail(t *testing.T) {
	m := fitModel()
	m.assigned = []*part.Part{nil, nil}
	if got := m.stateOf(0); got != stFootprint {
		t.Fatalf("state = %v, want stFootprint with no part detail fetched", got)
	}
	if len(m.issues()) != 1 {
		t.Fatalf("issues = %d, want 1", len(m.issues()))
	}
	out := stripANSI(strings.Join(m.confirmContent(60), "\n"))
	if !strings.Contains(out, "more pads than the land") {
		t.Errorf("the export confirmation stayed silent:\n%s", out)
	}
}

// A design with no land geometry at all must not read as a clean pass.
func TestNoGeometryIsNotAPass(t *testing.T) {
	m := fitModel()
	m.designLands = nil
	if bad, unknown, checked := m.fitCount(); bad+unknown+checked != 0 {
		t.Fatalf("fitCount = (%d, %d, %d), want all zero", bad, unknown, checked)
	}
	m.w, m.h = 130, 40
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "no land geometry") {
		t.Errorf("a design with nothing to measure claimed a verdict:\n%s", out)
	}
	if strings.Contains(out, "fit their footprints") {
		t.Error("claimed parts fit when nothing was compared")
	}
}

func TestConfirmNamesTheFootprintFault(t *testing.T) {
	m := fitModel()
	out := stripANSI(strings.Join(m.confirmContent(60), "\n"))
	if !strings.Contains(out, "more pads than the land") {
		t.Errorf("the export confirmation didn't mention it:\n%s", out)
	}
}

// Opening Export is what asks the vendor for the geometry, since that is when the
// pre-flight gets read.
func TestOpeningExportFetchesTheGeometry(t *testing.T) {
	m := fitModel()
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	if n := len(m.fitCmds()); n != 2 {
		t.Fatalf("fitCmds = %d, want one per assigned part", n)
	}
	// a request that never came back is retried, but only up to the limit
	for pass := 2; pass <= maxFitAttempts; pass++ {
		if n := len(m.fitCmds()); n != 2 {
			t.Fatalf("pass %d asked for %d, want 2", pass, n)
		}
	}
	if n := len(m.fitCmds()); n != 0 {
		t.Errorf("asked again for %d parts after %d attempts", n, maxFitAttempts)
	}
}

// A part EasyEDA has no record of must stop being retried and be reported as
// unchecked, not as a comparison still in flight.
func TestAPartWithNoVendorGeometryStopsBeingRetried(t *testing.T) {
	m := fitModel()
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}

	for pass := 1; pass <= maxFitAttempts+2; pass++ {
		cmds := m.fitCmds()
		if pass <= maxFitAttempts && len(cmds) != 2 {
			t.Fatalf("pass %d asked for %d parts, want 2", pass, len(cmds))
		}
		if pass > maxFitAttempts && len(cmds) != 0 {
			t.Fatalf("pass %d still asking after %d attempts", pass, maxFitAttempts)
		}
		for _, c := range cmds {
			mm, _ := m.Update(footprintDoneMsg{code: "C2006", err: errNoSource})
			m = mm.(Model)
			_ = c
		}
	}

	tl := m.fitTally()
	if tl.Pending != 0 {
		t.Errorf("still pending after giving up: %+v", tl)
	}
	if tl.Unchecked == 0 {
		t.Errorf("gave up without saying so: %+v", tl)
	}
	m.w, m.h = 130, 40
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))
	if !strings.Contains(out, "unchecked") || !strings.Contains(out, "no vendor geometry") {
		t.Errorf("pre-flight hid the unchecked parts:\n%s", out)
	}
	if strings.Contains(out, "checking") {
		t.Error("pre-flight still claims a check is in flight")
	}
}

// Opening the Export tab is the thing that triggers the fetch, so the wiring in
// gotoTab has to stay put.
func TestGotoExportAsksForGeometry(t *testing.T) {
	m := fitModel()
	m.edaLands = map[string]easyeda.Footprint{}
	m.edaTried = map[string]int{}
	mm, cmd := m.gotoTab(modeCheck)
	if cmd == nil {
		t.Fatal("opening Export asked for nothing")
	}
	if got := mm.(Model).mode; got != modeCheck {
		t.Errorf("mode = %v, want modeCheck", got)
	}
	if len(mm.(Model).edaTried) != 2 {
		t.Errorf("recorded %d requests, want 2", len(mm.(Model).edaTried))
	}
}

// The issue list shows the glyph without its note, so a footprint fault must not
// wear the same one as an out-of-stock part.
func TestFootprintGlyphIsItsOwn(t *testing.T) {
	seen := map[string]itemState{}
	for _, st := range []itemState{stOK, stUnassigned, stOutOfStock, stFootprint, stMismatch, stExcluded} {
		icon, _, _ := stateDecor(st)
		icon = stripANSI(icon)
		if prev, dup := seen[icon]; dup {
			t.Errorf("state %v reuses the glyph %q from state %v", st, icon, prev)
		}
		seen[icon] = st
		if lipgloss.Width(icon) != 1 {
			t.Errorf("state %v glyph %q is %d columns, the column allows 1", st, icon, lipgloss.Width(icon))
		}
	}
}

// The unchecked note used to ride on the end of the verdict and run off the panel.
func TestPreflightLinesFitThePanel(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 42
	m.edaLands["C93947"] = easyeda.Footprint{Code: "C93947"} // vendor has no geometry

	lines := m.fitCheck(func(ok bool, pass, fail string) string {
		if ok {
			return "✓ " + pass
		}
		return "✗ " + fail
	})
	if len(lines) != 2 {
		t.Fatalf("fitCheck gave %d lines, want the verdict and the unchecked note apart: %q", len(lines), lines)
	}
	for _, l := range lines {
		if w := lipgloss.Width(stripANSI(l)); w > 60 {
			t.Errorf("%q is %d columns, too wide for the checklist column", stripANSI(l), w)
		}
	}
	if !strings.Contains(stripANSI(lines[1]), "1 unchecked") {
		t.Errorf("the unchecked note went missing: %q", lines)
	}
}

// A board with a footprint fault must not read as clean on the Components overview.
func TestOverviewCountsFootprintFaults(t *testing.T) {
	m := fitModel()
	m.w, m.h = 132, 42
	out := stripANSI(strings.Join(m.compactOverview(50, 20), "\n"))
	if !strings.Contains(out, "footprint 1") {
		t.Errorf("the overview called a faulty board clean:\n%s", out)
	}
}
