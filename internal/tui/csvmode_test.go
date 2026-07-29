package tui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const testBOM = `Comment,Designator,Footprint,Quantity,LCSC Part #
100nF,"C1,C2,C3",C_0402_1005Metric,3,C1525
10k,"R1,R2",R_0402_1005Metric,2,C25744
STM32F103CBT6,U1,LQFP-48,1,C8304
`

const testCPL = `Designator,Mid X,Mid Y,Layer,Rotation
C1,10.0,20.0,top,0
C2,12.5,20.0,top,90
C3,15.0,20.0,bottom,180
R1,10.0,25.0,top,0
R2,12.5,25.0,top,0
U1,30.0,40.0,top,270
`

// csvModel loads a BOM (and optionally a placement file) the way the app does.
func csvModel(t *testing.T, withCPL bool) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	bom := filepath.Join(dir, "drone-bom.csv")
	if err := os.WriteFile(bom, []byte(testBOM), 0o644); err != nil {
		t.Fatal(err)
	}
	if withCPL {
		if err := os.WriteFile(filepath.Join(dir, "drone-cpl.csv"), []byte(testCPL), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := New(Options{Project: bom})
	m = step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = step(m, loadProjectCmd(bom, "")())
	if m.err != "" {
		t.Fatalf("load failed: %s", m.err)
	}
	return m, bom
}

func TestCSVModeLoads(t *testing.T) {
	m, bom := csvModel(t, true)

	if m.mode != modeTable {
		t.Fatalf("mode = %v, want the components table", m.mode)
	}
	if m.fromBoard() {
		t.Error("a CSV design must not claim a board")
	}
	if m.bomPath != bom || m.pcbPath != "" {
		t.Errorf("paths wrong: bom %q pcb %q", m.bomPath, m.pcbPath)
	}
	if len(m.items) != 3 {
		t.Fatalf("got %d line items, want 3", len(m.items))
	}
	if len(m.placements) != 6 {
		t.Errorf("got %d placements, want 6 from the sibling cpl", len(m.placements))
	}
	if m.sourcePath() != bom {
		t.Errorf("sourcePath = %q, want the bom path", m.sourcePath())
	}
	if !strings.Contains(m.status, "bom csv") {
		t.Errorf("status %q should say what kind of design this is", m.status)
	}
	if !strings.Contains(m.flash, "cpl drone-cpl.csv") {
		t.Errorf("flash %q should name the placement file it picked up", m.flash)
	}
}

// The bug this mode used to hit: no output path, so export died on "output path
// is empty".
func TestCSVModeSeedsOutputPath(t *testing.T) {
	m, bom := csvModel(t, true)

	mm, _ := m.gotoTab(modeCheck)
	m = mm.(Model)

	got := m.check.out.Value()
	want := filepath.Join(filepath.Dir(bom), "drone-bom-order.zip")
	if got != want {
		t.Errorf("output path = %q, want %q", got, want)
	}
}

func TestCSVModeExportsBomAndPositions(t *testing.T) {
	m, bom := csvModel(t, true)
	out := filepath.Join(filepath.Dir(bom), "order.zip")

	msg, ok := m.exportCmd(out)().(exportDoneMsg)
	if !ok {
		t.Fatal("unexpected message from exportCmd")
	}
	if msg.err != nil {
		t.Fatalf("export failed: %v", msg.err)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "bom.csv") {
		t.Errorf("zip %v is missing bom.csv", names)
	}
	if !strings.Contains(joined, "positions.csv") {
		t.Errorf("zip %v is missing positions.csv — the cpl should have produced one", names)
	}
	// no board means no gerbers, and that's not a failure
	if strings.Contains(joined, "gerber/") {
		t.Errorf("zip %v should not contain gerbers without a board", names)
	}
}

func TestCSVModeWithoutCPLStillExportsBom(t *testing.T) {
	m, bom := csvModel(t, false)
	if len(m.placements) != 0 {
		t.Fatalf("expected no placements, got %d", len(m.placements))
	}

	out := filepath.Join(filepath.Dir(bom), "order.zip")
	msg := m.exportCmd(out)().(exportDoneMsg)
	if msg.err != nil {
		t.Fatalf("export failed: %v", msg.err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "bom.csv" {
		var names []string
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		t.Errorf("zip = %v, want just bom.csv", names)
	}
}

func TestCSVModeBoardActionsExplainThemselves(t *testing.T) {
	m, _ := csvModel(t, true)

	// write-back has no file to write to
	mm, cmd := m.updateTable(tea.KeyPressMsg{Text: "w", Code: 'w'})
	m = mm.(Model)
	if cmd != nil {
		t.Error("w should not run a write command without a board")
	}
	if !strings.Contains(m.flash, "csv") {
		t.Errorf("flash %q should explain why w did nothing", m.flash)
	}

	// so does the 3D render
	mm, cmd = m.updateTable(tea.KeyPressMsg{Text: "t", Code: 't'})
	m = mm.(Model)
	if cmd != nil {
		t.Error("t should not start a render without a board")
	}
	if !strings.Contains(m.flash, "no board") {
		t.Errorf("flash %q should explain why the render was refused", m.flash)
	}
}

func TestCSVModeDrawsPlacementMap(t *testing.T) {
	withCPL, _ := csvModel(t, true)
	if got := strings.Join(withCPL.miniBoard(40, 12), "\n"); strings.Contains(got, "no placements") {
		t.Error("with a cpl there is a placement map to draw")
	}

	// the board buttons make no sense without a board
	if got := stripANSI(withCPL.boardHeader()); !strings.Contains(got, "Placements") {
		t.Errorf("board header = %q, want it to say Placements", got)
	}
	if strings.Contains(stripANSI(withCPL.boardHeader()), "[t]op") {
		t.Error("render buttons should not be offered without a board")
	}

	noCPL, _ := csvModel(t, false)
	if got := strings.Join(noCPL.miniBoard(40, 12), "\n"); !strings.Contains(got, "no placements") {
		t.Errorf("without a cpl the panel should say so, got %q", stripANSI(got))
	}
}

func TestCSVModePreflightIsNotAllFailures(t *testing.T) {
	m, _ := csvModel(t, true)
	out := stripANSI(strings.Join(m.preflightAndManifest(m.contentW()), "\n"))

	if !strings.Contains(out, "no board — bom csv") {
		t.Errorf("pre-flight should note the missing board neutrally:\n%s", out)
	}
	if !strings.Contains(out, "n/a — no board") {
		t.Errorf("the manifest should mark gerbers as not applicable:\n%s", out)
	}
	if !strings.Contains(out, "6 placements") {
		t.Errorf("pre-flight should count the placements:\n%s", out)
	}
}
