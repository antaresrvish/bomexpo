package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/kicad"
	"bomexpo/internal/lcsc"
	"bomexpo/internal/value"
)

type mode int

const (
	modeLoad mode = iota
	modeTable
	modeSearch
	modeBoard
	modeCheck
)

var tabs = []struct {
	mode  mode
	label string
}{
	{modeLoad, "Load"},
	{modeTable, "Components"},
	{modeBoard, "Board"},
	{modeCheck, "Check"},
}

type Model struct {
	client *lcsc.Client
	mode   mode
	w, h   int
	err    string
	status string

	spin    spinner.Model
	loading bool

	load   loadState
	search searchState
	boardv boardState
	check  checkState

	name       string
	pcbPath    string
	items      []kicad.Item
	placements []kicad.Placement
	board      *kicad.Board
	assigned   []*lcsc.Part
	excluded   []bool
	layers     int
	boardW     float64
	boardH     float64

	cursor int
	top    int

	autoRemaining int
	autoOK        int
}

func New(project string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	m := Model{client: lcsc.New(), mode: modeLoad, spin: sp}
	m.load = newLoadState(project)
	m.search = newSearchState()
	m.check = newCheckState()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.load.focusCmd())
}

type projectLoadedMsg struct {
	name       string
	pcbPath    string
	items      []kicad.Item
	placements []kicad.Placement
	board      *kicad.Board
	layers     int
	boardW     float64
	boardH     float64
	err        error
}

type searchDoneMsg struct {
	token int
	res   lcsc.SearchResult
	err   error
}

type detailDoneMsg struct {
	idx  int
	part lcsc.Part
	err  error
}

type exportDoneMsg struct {
	path string
	err  error
}

type autoAssignedMsg struct {
	idx  int
	part lcsc.Part
	ok   bool
	err  error
}

func (m Model) autoAssignCmd(idx int) tea.Cmd {
	it := m.items[idx]
	kw := searchKeyword(it)
	kind := deriveKind(it.Value, refPrefix(it.ID()))
	pkg := sizeCode.FindString(it.Footprint)
	client := m.client
	return func() tea.Msg {
		res, err := client.Search(kw, 1, 100)
		if err != nil {
			return autoAssignedMsg{idx: idx, err: err}
		}
		p, ok := pickBest(it, kind, pkg, res.Items)
		return autoAssignedMsg{idx: idx, part: p, ok: ok}
	}
}

func loadProjectCmd(path string) tea.Cmd {
	return func() tea.Msg {
		p, err := kicad.LoadProject(path)
		if err != nil {
			return projectLoadedMsg{err: err}
		}
		return projectLoadedMsg{
			name: p.Name, pcbPath: p.PCBPath,
			items: p.BOM(), placements: p.Placements(), board: p.Board(),
			layers: p.Layers, boardW: p.BoardW, boardH: p.BoardH,
		}
	}
}

func (m Model) searchCmd(token int, keyword string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		res, err := client.Search(keyword, 1, 100)
		return searchDoneMsg{token: token, res: res, err: err}
	}
}

func (m Model) detailCmd(idx int, code string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		p, err := client.Detail(code)
		return detailDoneMsg{idx: idx, part: p, err: err}
	}
}

func (m Model) exportCmd(path string) tea.Cmd {
	var items []kicad.Item
	excl := map[string]bool{}
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			for _, d := range it.Designators {
				excl[d] = true
			}
			continue
		}
		items = append(items, it)
	}
	placements := m.placements
	pcb := m.pcbPath
	return func() tea.Msg {
		err := exportZip(path, items, placements, pcb, excl)
		return exportDoneMsg{path: path, err: err}
	}
}

type itemState int

const (
	stUnassigned itemState = iota
	stOutOfStock
	stMismatch
	stOK
	stExcluded
)

func (m Model) stateOf(i int) itemState {
	if i < 0 || i >= len(m.items) {
		return stUnassigned
	}
	if i < len(m.excluded) && m.excluded[i] {
		return stExcluded
	}
	if m.items[i].LCSC == "" {
		return stUnassigned
	}
	p := m.assigned[i]
	if p == nil {
		return stOK
	}
	if !p.InStock() {
		return stOutOfStock
	}
	if r := value.Check(m.items[i].Value, p.Description()); !r.Match {
		return stMismatch
	}
	return stOK
}

func (m Model) counts() (assigned, warn int) {
	for i := range m.items {
		switch m.stateOf(i) {
		case stUnassigned:
			warn++
		case stOutOfStock, stMismatch:
			assigned++
			warn++
		case stOK:
			assigned++
		}
	}
	return
}

func (m Model) activeCount() int {
	n := 0
	for i := range m.items {
		if i >= len(m.excluded) || !m.excluded[i] {
			n++
		}
	}
	return n
}

func (m Model) excludedCount() int { return len(m.items) - m.activeCount() }
