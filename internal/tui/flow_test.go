package tui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/lcsc"
	"bomexpo/internal/part"
)

func TestFullFlow(t *testing.T) {
	proj := os.Getenv("BOMEXPO_PROJ")
	if proj == "" {
		t.Skip("set BOMEXPO_PROJ")
	}

	m := New(proj, "")
	m = step(m, tea.WindowSizeMsg{Width: 130, Height: 44})
	m = step(m, loadProjectCmd(proj)())

	if m.mode != modeTable || len(m.items) == 0 {
		t.Fatalf("load failed: mode=%d items=%d", m.mode, len(m.items))
	}
	t.Logf("loaded %d items, %d placements, board=%v", len(m.items), len(m.placements), m.board != nil)
	mustRender(t, m, "table")

	mm, _ := m.openSearch(0)
	m = mm.(Model)
	if m.mode != modeSearch {
		t.Fatal("openSearch did not switch mode")
	}

	kw := searchKeyword(m.items[0])
	res, err := lcsc.New().Provider().Search(part.Query{Keyword: kw, Size: 30})
	if err != nil {
		t.Fatalf("live search %q: %v", kw, err)
	}
	m = step(m, searchDoneMsg{token: m.search.token, res: res})
	if len(m.search.results) == 0 {
		t.Fatalf("no results for %q", kw)
	}
	t.Logf("search %q -> %d results (%d after filters)", kw, len(m.search.results), len(m.search.filtered()))

	mm, _ = m.assignSelected()
	m = mm.(Model)
	if m.items[0].LCSC == "" || m.assigned[0] == nil {
		t.Fatal("assignment failed")
	}
	t.Logf("assigned %s -> %s (%s)", m.items[0].ID(), m.items[0].LCSC, m.assigned[0].MPN)
	mustRender(t, m, "table-after-assign")

	m.mode = modeCheck
	m.check.setDefault(m.pcbPath)
	mustRender(t, m, "check")

	out := filepath.Join(t.TempDir(), "order.zip")
	msg := m.exportCmd(out)().(exportDoneMsg)
	if msg.err != nil {
		t.Fatalf("export: %v", msg.err)
	}
	verifyZip(t, out)
}

func step(m Model, msg tea.Msg) Model {
	mm, _ := m.Update(msg)
	return mm.(Model)
}

func mustRender(t *testing.T, m Model, name string) {
	t.Helper()
	_ = m.View()
	_, s := m.titleBody()
	if strings.TrimSpace(stripANSI(s)) == "" {
		t.Fatalf("%s render empty", name)
	}
}

func verifyZip(t *testing.T, path string) {
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[string]bool{}
	gerbers := 0
	for _, f := range r.File {
		names[f.Name] = true
		if strings.HasPrefix(f.Name, "gerber/") {
			gerbers++
		}
	}
	if !names["bom.csv"] {
		t.Error("zip missing bom.csv")
	}
	t.Logf("zip ok: %d entries, %d gerbers", len(r.File), gerbers)
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
