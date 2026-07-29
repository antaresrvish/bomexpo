package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
)

func TestDatasheetColumnAlignment(t *testing.T) {
	m := New("", "")
	m.w, m.h = 140, 40
	m.items = []kicad.Item{
		{Bases: []string{"C1"}, Value: "100nF", Footprint: "C_0402_1005Metric", Quantity: 1, LCSC: "C1525"},
		{Bases: []string{"C2"}, Value: "1uF", Footprint: "C_0402_1005Metric", Quantity: 1, LCSC: "C2"},
	}
	m.assigned = []*part.Part{
		{Code: "C1525", Datasheet: "https://x/y.pdf", Desc: "100nF X7R"},
		{Code: "C2", Datasheet: "https://x/z.pdf", Desc: "1uF X5R"},
	}
	m.excluded = []bool{false, false}
	m.cursor = 0

	c := layoutCols(m.tableW())
	lo, hi := c.dsRange()

	// dsRange is in row-line coordinates (icon at 0), matching rowView output.
	line := stripANSI(m.rowView(1, c, sepStyle.Render(" │ ")))
	b := strings.Index(line, "datasheet")
	if b < 0 {
		t.Fatalf("no datasheet text in row: %q", line)
	}
	col := lipgloss.Width(line[:b]) // column (cell) position, not byte offset
	if col != lo {
		t.Errorf("datasheet starts at column %d, want %d (dsRange %d-%d)", col, lo, lo, hi)
	}
	if col+lipgloss.Width("datasheet") > hi {
		t.Errorf("datasheet text overflows dsRange")
	}
}
