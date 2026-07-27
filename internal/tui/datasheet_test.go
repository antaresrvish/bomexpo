package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
)

func TestDatasheetColumnAlignment(t *testing.T) {
	m := New("")
	m.w, m.h = 140, 40
	m.items = []kicad.Item{
		{Bases: []string{"C1"}, Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 1, LCSC: "C1525"},
		{Bases: []string{"C2"}, Value: "1uF", Footprint: "C_0402_1005Metric", Quantity: 1, LCSC: "C2"},
	}
	m.assigned = []*lcsc.Part{
		{Code: "C1525", Datasheet: "https://x/y.pdf", IntroEn: "100nF X7R"},
		{Code: "C2", Datasheet: "https://x/z.pdf", IntroEn: "1uF X5R"},
	}
	m.excluded = []bool{false, false}
	m.cursor = 0

	c := layoutCols(m.contentW())
	lo, hi := c.dsRange()

	// row 1 is non-selected; "datasheet" must sit inside dsRange (minus the
	// panel's 2-col bar+space offset that dsRange includes).
	line := stripANSI(m.rowView(1, c, sepStyle.Render(" │ "), m.contentW()))
	b := strings.Index(line, "datasheet")
	if b < 0 {
		t.Fatalf("no datasheet text in row: %q", line)
	}
	col := lipgloss.Width(line[:b]) // column (cell) position, not byte offset
	// dsRange includes the panel's 2-col bar+space; rowView output has no panel.
	if col != lo-2 {
		t.Errorf("datasheet starts at column %d, want %d (dsRange %d-%d)", col, lo-2, lo, hi)
	}
	if col+lipgloss.Width("datasheet") > hi-2 {
		t.Errorf("datasheet text overflows dsRange")
	}
}
