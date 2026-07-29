package tui

import (
	"testing"

	"bomexpo/internal/kicad"
	"bomexpo/internal/value"
)

func TestAutoAssignAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	lines := []kicad.Item{
		{Bases: []string{"C1"}, Value: "1uF", Footprint: "C_0402_1005Metric", Quantity: 4},
		{Bases: []string{"C3"}, Value: "0.1uF", Footprint: "C_0402_1005Metric", Quantity: 15},
		{Bases: []string{"C4"}, Value: "4.7uF", Footprint: "C_0402_1005Metric", Quantity: 1},
		{Bases: []string{"C8"}, Value: "4.7uF", Footprint: "CP_Elec_4x5.3", Quantity: 1},
		{Bases: []string{"C11"}, Value: "10uF", Footprint: "C_0603_1608Metric", Quantity: 1},
		{Bases: []string{"C14"}, Value: "22uF", Footprint: "C_0603_1608Metric", Quantity: 2},
		{Bases: []string{"C18"}, Value: "47uF", Footprint: "C_0603_1608Metric", Quantity: 1},
		{Bases: []string{"C7"}, Value: "100uF", Footprint: "CP_Elec_6.3x7.7", Quantity: 1},
		{Bases: []string{"R1"}, Value: "10k", Footprint: "R_0402_1005Metric", Quantity: 6},
		{Bases: []string{"R9"}, Value: "10m", Footprint: "R_2512_6332Metric", Quantity: 1},
		{Bases: []string{"L1"}, Value: "3.3uH", Footprint: "IND_IHLP-2525CZ_VIS", Quantity: 1},
	}
	m := New(Options{})
	m.items = lines
	for i, it := range lines {
		msg := m.autoAssignCmd(i)().(autoAssignedMsg)
		if !msg.ok {
			t.Logf("%-4s %-6s %-22s -> (no pick)", it.ID(), it.Value, it.Footprint)
			continue
		}
		p := msg.part
		flag := ""
		want, w := value.Parse(it.Value)
		got, g := value.ExtractValue(p.Description())
		if w && g && !value.Equal(want, got) {
			flag = "  <<< VALUE MISMATCH"
		}
		pkg := sizeCode.FindString(it.Footprint)
		if pkg != "" && p.Package != pkg {
			flag += "  <<< PKG MISMATCH"
		}
		t.Logf("%-4s %-6s %-22s -> %-9s %-6s %q%s", it.ID(), it.Value, it.Footprint, p.Code, p.Package, p.Description(), flag)
	}
}
