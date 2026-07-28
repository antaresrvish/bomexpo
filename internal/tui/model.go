package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/export"
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
	modeOverview
)

var tabs = []struct {
	mode  mode
	label string
}{
	{modeLoad, "Load"},
	{modeOverview, "Overview"},
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
	flash  string

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
	hoff   int

	sort    sortKey
	sortAsc bool

	autoRemaining int
	autoOK        int

	refreshRemaining int
	refreshOK        int
}

type sortKey int

const (
	sortNone sortKey = iota
	sortRef
	sortVal
	sortFp
	sortQty
	sortCode
	sortStock
	sortPrice
	sortRot
)

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

type refreshDoneMsg struct {
	idx  int
	part lcsc.Part
	err  error
}

// refreshCmd force-refetches stock and pricing for every assigned line item,
// bypassing the cache.
func (m Model) refreshCmd() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i, it := range m.items {
		if it.LCSC == "" {
			continue
		}
		cmds = append(cmds, m.refreshOne(i, it.LCSC))
	}
	if len(cmds) == 0 {
		m.flash = "nothing assigned to refresh"
		return m, nil
	}
	m.refreshRemaining = len(cmds)
	m.refreshOK = 0
	m.loading = true
	m.status = fmt.Sprintf("Refreshing stock & prices for %d parts…", len(cmds))
	return m, tea.Batch(cmds...)
}

func (m Model) refreshOne(idx int, code string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		p, err := client.Refresh(code)
		return refreshDoneMsg{idx: idx, part: p, err: err}
	}
}

func (m Model) excludeSet() map[string]bool {
	excl := map[string]bool{}
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			for _, d := range it.Designators {
				excl[d] = true
			}
		}
	}
	return excl
}

func (m Model) exportCmd(path string) tea.Cmd {
	excl := m.excludeSet()
	rot := m.rotOverrideMap()
	var items []kicad.Item
	for i, it := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		items = append(items, it)
	}
	placements := m.placements
	pcb := m.pcbPath
	return func() tea.Msg {
		err := exportZip(path, items, placements, pcb, excl, rot)
		return exportDoneMsg{path: path, err: err}
	}
}

func (m Model) rotOverrideMap() map[string]int {
	out := map[string]int{}
	for _, it := range m.items {
		if !it.HasRotOverride {
			continue
		}
		for _, d := range it.Designators {
			out[d] = it.RotOverride
		}
	}
	return out
}

// cycleRotOverride steps the selected line item's CPL rotation override
// through none → 0 → 90 → 180 → 270 → none.
func (m Model) cycleRotOverride() (tea.Model, tea.Cmd) {
	i := m.cursor
	if i < 0 || i >= len(m.items) {
		return m, nil
	}
	it := m.items[i]
	switch {
	case !it.HasRotOverride:
		m.items[i].HasRotOverride, m.items[i].RotOverride = true, 0
	case it.RotOverride == 0:
		m.items[i].RotOverride = 90
	case it.RotOverride == 90:
		m.items[i].RotOverride = 180
	case it.RotOverride == 180:
		m.items[i].RotOverride = 270
	default:
		m.items[i].RotOverride = 0 // wrap; O resets to auto
	}
	m.flash = fmt.Sprintf("%s CPL rotation → +%d° (o cycles · O resets · w saves)", it.ID(), m.items[i].RotOverride)
	return m, nil
}

func (m Model) resetRotOverride() (tea.Model, tea.Cmd) {
	i := m.cursor
	if i < 0 || i >= len(m.items) {
		return m, nil
	}
	if !m.items[i].HasRotOverride {
		m.flash = fmt.Sprintf("%s already on auto rotation", m.items[i].ID())
		return m, nil
	}
	m.items[i].HasRotOverride, m.items[i].RotOverride = false, 0
	m.flash = fmt.Sprintf("%s rotation reset to auto (save with w)", m.items[i].ID())
	return m, nil
}

func (m Model) dnpCount() int {
	n := 0
	for _, it := range m.items {
		if it.DNP {
			n++
		}
	}
	return n
}

func (m Model) stockOf(i int) int {
	if p := m.assigned[i]; p != nil {
		return p.Stock
	}
	return -1
}

func (m Model) priceOf(i int) float64 {
	if p := m.assigned[i]; p != nil {
		if u, ok := p.UnitPrice(); ok {
			return u
		}
	}
	return -1
}

func (m Model) rotOf(i int) float64 {
	if m.items[i].HasRotOverride {
		return float64(m.items[i].RotOverride)
	}
	return export.FamilyOffset(m.items[i].Footprint)
}

func (m Model) itemLess(i, j int) bool {
	a, b := m.items[i], m.items[j]
	switch m.sort {
	case sortVal:
		return strings.ToLower(a.Value) < strings.ToLower(b.Value)
	case sortFp:
		return strings.ToLower(a.Footprint) < strings.ToLower(b.Footprint)
	case sortQty:
		return a.Quantity < b.Quantity
	case sortCode:
		return a.LCSC < b.LCSC
	case sortStock:
		return m.stockOf(i) < m.stockOf(j)
	case sortPrice:
		return m.priceOf(i) < m.priceOf(j)
	case sortRot:
		return m.rotOf(i) < m.rotOf(j)
	}
	return kicad.RefLess(a.ID(), b.ID())
}

// sorted reorders the line items (and the parallel assigned/excluded slices) by
// the active column and direction, keeping them index-aligned.
func (m Model) sorted() Model {
	perm := make([]int, len(m.items))
	for i := range perm {
		perm[i] = i
	}
	sort.SliceStable(perm, func(a, b int) bool {
		if m.sortAsc {
			return m.itemLess(perm[a], perm[b])
		}
		return m.itemLess(perm[b], perm[a])
	})
	ni := make([]kicad.Item, len(m.items))
	na := make([]*lcsc.Part, len(m.assigned))
	ne := make([]bool, len(m.excluded))
	for n, o := range perm {
		ni[n], na[n], ne[n] = m.items[o], m.assigned[o], m.excluded[o]
	}
	m.items, m.assigned, m.excluded = ni, na, ne
	m.cursor, m.top = 0, 0
	return m
}

type savedMsg struct {
	path string
	res  kicad.WriteResult
	err  error
}

func (m Model) saveCmd() tea.Cmd {
	if m.pcbPath == "" {
		return nil
	}
	codes := map[string]string{}
	exclude := map[string]bool{}
	rot := map[string]*int{}
	for i, it := range m.items {
		if it.LCSC != "" {
			for _, d := range it.Designators {
				codes[d] = it.LCSC
			}
		}
		if it.DNP {
			continue // DNP exclusion is KiCad's own dnp flag, left untouched
		}
		on := i < len(m.excluded) && m.excluded[i]
		var ov *int
		if it.HasRotOverride {
			v := it.RotOverride
			ov = &v
		}
		for _, d := range it.Designators {
			exclude[d] = on
			rot[d] = ov
		}
	}
	pcb := m.pcbPath
	return func() tea.Msg {
		res, err := kicad.WriteBack(pcb, codes, exclude, rot)
		return savedMsg{path: pcb, res: res, err: err}
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
