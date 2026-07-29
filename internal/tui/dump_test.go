package tui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/lcsc"
	"bomexpo/internal/part"
)

func TestDumpFrames(t *testing.T) {
	proj := os.Getenv("BOMEXPO_PROJ")
	outDir := os.Getenv("BOMEXPO_DUMP")
	if proj == "" || outDir == "" {
		t.Skip("set BOMEXPO_PROJ and BOMEXPO_DUMP")
	}

	m := New(Options{Project: proj})
	m = step(m, tea.WindowSizeMsg{Width: 132, Height: 40})
	m = step(m, loadProjectCmd(proj, "")())

	// leave a few unassigned to show ○, assign one live via search to show a swap
	mm, _ := m.openSearch(0)
	m = mm.(Model)
	if res, err := lcsc.New().Provider().Search(part.Query{Keyword: searchKeyword(m.items[0]), Size: 30}); err == nil {
		m = step(m, searchDoneMsg{token: m.search.token, res: res})
	}
	dump(t, outDir+"/search.txt", frame(m))

	mm, _ = m.assignSelected()
	m = mm.(Model)
	m.mode = modeTable
	dump(t, outDir+"/table.txt", frame(m))

	m.mode = modeCheck
	m.check.setDefault(m.sourcePath())
	dump(t, outDir+"/check.txt", frame(m))
}

func frame(m Model) string {
	title, body := m.titleBody()
	return stripANSI(m.tabBar() + "\n" + panelBox(title, body, m.w, m.contentH()) + "\n" + m.bottomBar())
}

func dump(t *testing.T, path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
