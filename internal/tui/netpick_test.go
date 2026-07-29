package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
)

func netModel(t *testing.T) Model {
	t.Helper()
	m := filterModel(t)
	m.pcbPath = "/tmp/x.kicad_pcb" // pretend a board, so the picker is offered
	m.designNets = []kicad.Net{
		{Name: "GND", Refs: []string{"C1", "C2", "R1", "D1"}},
		{Name: "+5V", Refs: []string{"C2", "L1"}},
		{Name: "+3V3", Refs: []string{"C1"}},
		{Name: "SPI_SCK", Refs: []string{"R1"}},
		{Name: "VBUS", Refs: []string{"L1"}},
	}
	return m
}

func TestNetPickerOpensAndFilters(t *testing.T) {
	m := netModel(t)

	mm, _ := m.updateTable(tea.KeyPressMsg{Text: "n", Code: 'n'})
	m = mm.(Model)
	if m.mode != modeNets || !m.nets.open {
		t.Fatal("n should open the net picker")
	}

	// the busiest net is first, so enter on it filters by GND
	mm, _ = m.updateNetKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mm.(Model)
	if m.mode != modeTable {
		t.Error("picking a net should return to the table")
	}
	if got := m.filter.field.Value(); got != "net:GND" {
		t.Errorf("query = %q, want net:GND", got)
	}
	if got, want := shown(m), "C1,C2,R1,D1"; got != want {
		t.Errorf("filtered to %s, want %s", got, want)
	}
	if !m.filterBarVisible() {
		t.Error("the filter bar should show what the picker set")
	}
}

func TestNetPickerNarrowsByTyping(t *testing.T) {
	m := netModel(t)
	mm, _ := m.openNetPicker()
	m = mm.(Model)

	for _, r := range "spi" {
		mm, _ = m.updateNetKey(tea.KeyPressMsg{Text: string(r), Code: r})
		m = mm.(Model)
	}
	nets := m.netsMatching()
	if len(nets) != 1 || nets[0].Name != "SPI_SCK" {
		t.Fatalf("narrowed to %+v, want just SPI_SCK", nets)
	}

	mm, _ = m.updateNetKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mm.(Model)
	if got, want := shown(m), "R1"; got != want {
		t.Errorf("filtered to %s, want %s", got, want)
	}
}

// Picking a second net must replace the first, not stack up net: terms that can
// never both match.
func TestPickNetReplacesTheExistingTerm(t *testing.T) {
	m := netModel(t)
	mm, _ := m.pickNet("GND")
	m = mm.(Model)
	mm, _ = m.pickNet("+5V")
	m = mm.(Model)

	if got := m.filter.field.Value(); got != "net:+5V" {
		t.Errorf("query = %q, want just net:+5V", got)
	}
	if got, want := shown(m), "C2,L1"; got != want {
		t.Errorf("filtered to %s, want %s", got, want)
	}
}

// Other terms in the query survive a net pick.
func TestPickNetKeepsOtherTerms(t *testing.T) {
	m := netModel(t)
	m.filter.field.SetValue("fp:0402 net:VBUS")
	m.filter.f = parseFilter(m.filter.field.Value())
	m = m.reindex()

	mm, _ := m.pickNet("GND")
	m = mm.(Model)
	if got := m.filter.field.Value(); got != "fp:0402 net:GND" {
		t.Errorf("query = %q, want fp:0402 net:GND", got)
	}
	if got, want := shown(m), "C1,R1"; got != want {
		t.Errorf("filtered to %s, want %s", got, want)
	}
}

func TestNetPickerRefusedWithoutNets(t *testing.T) {
	m := filterModel(t) // no designNets, no pcb
	mm, _ := m.updateTable(tea.KeyPressMsg{Text: "n", Code: 'n'})
	m = mm.(Model)
	if m.mode == modeNets {
		t.Error("the picker should not open with no nets")
	}
	if !strings.Contains(m.flash, "no nets") {
		t.Errorf("flash %q should explain there are no nets", m.flash)
	}

	// with a board but genuinely no nets, say that instead
	m = filterModel(t)
	m.pcbPath = "/tmp/x.kicad_pcb"
	mm, _ = m.openNetPicker()
	if got := mm.(Model).flash; !strings.Contains(got, "no nets on any pad") {
		t.Errorf("flash %q should blame the board", got)
	}
}

func TestNetViewRendersCountsAndFitsHeight(t *testing.T) {
	m := netModel(t)
	mm, _ := m.openNetPicker()
	m = mm.(Model)

	h := m.contentH()
	out := stripANSI(m.viewNets(m.contentW(), h))
	if n := len(strings.Split(out, "\n")); n != h {
		t.Errorf("net view has %d lines, want %d", n, h)
	}
	for _, want := range []string{"GND", "SPI_SCK", "C1 C2 R1 D1", "5 nets"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from the net view:\n%s", want, out)
		}
	}
}

func TestBoardZoomAndPan(t *testing.T) {
	m := netModel(t)
	if m.boardv.zoom != 1 {
		t.Fatalf("zoom starts at %g, want 1", m.boardv.zoom)
	}

	press := func(m Model, s string, code rune) Model {
		mm, _ := m.updateTable(tea.KeyPressMsg{Text: s, Code: code})
		return mm.(Model)
	}

	// panning does nothing while the whole board fits
	m = press(m, "", 0)
	mm, _ := m.updateTable(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if got := mm.(Model).boardv.panX; got != 0 {
		t.Errorf("panX = %g, want 0 at fit zoom", got)
	}

	m = press(m, "+", '+')
	if m.boardv.zoom <= 1 {
		t.Errorf("+ should zoom in, got %g", m.boardv.zoom)
	}
	// now panning bites
	mm, _ = m.updateTable(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	m = mm.(Model)
	if m.boardv.panX <= 0 {
		t.Errorf("panX = %g, want it to move right", m.boardv.panX)
	}

	// 0 puts everything back
	m = press(m, "0", '0')
	if m.boardv.zoom != 1 || m.boardv.panX != 0 || m.boardv.panY != 0 {
		t.Errorf("reset left %+v", m.boardv)
	}

	// zoom is bounded, and coming back to the fit level drops the pan
	for i := 0; i < 40; i++ {
		m = press(m, "+", '+')
	}
	if m.boardv.zoom > zoomMax {
		t.Errorf("zoom = %g, want it capped at %g", m.boardv.zoom, zoomMax)
	}
	mm, _ = m.updateTable(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	m = mm.(Model)
	for i := 0; i < 40; i++ {
		m = press(m, "-", '-')
	}
	if m.boardv.zoom != zoomMin || m.boardv.panX != 0 {
		t.Errorf("zooming back out left %+v, want the fit view", m.boardv)
	}

	// the zoom level is shown so you know you're not looking at the whole board
	m = press(m, "+", '+')
	if got := stripANSI(m.boardHeader()); !strings.Contains(got, "×") {
		t.Errorf("board header = %q, want a zoom indicator", got)
	}
}
