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

// The board lives on the Check page now, where the output path owns the
// keyboard, so its view is driven with modifiers.
func TestBoardZoomAndPan(t *testing.T) {
	m, _ := csvModel(t, true) // has placements, so there's a backdrop to move
	m.mode = modeCheck
	if m.boardv.zoom != 1 {
		t.Fatalf("zoom starts at %g, want 1", m.boardv.zoom)
	}

	// the output path is not focused on entry, so the page's own keys work
	if m.check.out.Focused() {
		t.Fatal("the output path should not grab the keyboard on entry")
	}
	text := func(m Model, s string, code rune) Model {
		mm, _ := m.updateCheck(tea.KeyPressMsg{Text: s, Code: code})
		return mm.(Model)
	}
	zoomIn := func(m Model) Model { return text(m, "+", '+') }
	zoomOut := func(m Model) Model { return text(m, "-", '-') }
	panRight := func(m Model) Model {
		mm, _ := m.updateCheck(tea.KeyPressMsg{Code: tea.KeyRight})
		return mm.(Model)
	}

	// panning does nothing while the whole board fits
	if got := panRight(m).boardv.panX; got != 0 {
		t.Errorf("panX = %g, want 0 at fit zoom", got)
	}

	m = zoomIn(m)
	if m.boardv.zoom <= 1 {
		t.Errorf("+ should zoom in, got %g", m.boardv.zoom)
	}
	m = panRight(m)
	if m.boardv.panX <= 0 {
		t.Errorf("panX = %g, want it to move right", m.boardv.panX)
	}

	// zoom is capped
	for i := 0; i < 40; i++ {
		m = zoomIn(m)
	}
	if m.boardv.zoom > zoomMax {
		t.Errorf("zoom = %g, want it capped at %g", m.boardv.zoom, zoomMax)
	}

	// zooming all the way back out is the way home, pan included
	m = panRight(m)
	for i := 0; i < 40; i++ {
		m = zoomOut(m)
	}
	if m.boardv.zoom != zoomMin || m.boardv.panX != 0 || m.boardv.panY != 0 {
		t.Errorf("zooming back out left %+v, want the fit view", m.boardv)
	}

	// the level is shown, so you know you aren't seeing the whole board
	m = zoomIn(m)
	if got := stripANSI(strings.Join(m.boardPane(40, 12), "\n")); !strings.Contains(got, "×") {
		t.Errorf("board pane = %q, want a zoom indicator", got)
	}

	// and the output path is untouched by all of that
	if got := m.check.out.Value(); strings.ContainsAny(got, "+-") {
		t.Errorf("the board keys leaked into the output path: %q", got)
	}
}
