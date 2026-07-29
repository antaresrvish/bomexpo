package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/config"
	"bomexpo/internal/export"
	"bomexpo/internal/kicad"
	"bomexpo/internal/part"
	"bomexpo/internal/source"
	"bomexpo/internal/value"
)

type mode int

const (
	modeLoad mode = iota
	modeTable
	modeSearch
	modeParts
	modeCompare
	modeCheck
)

type tabDef struct {
	mode  mode
	label string
}

// tabs is dynamic: Compare only earns a tab once there's something to compare,
// which keeps it discoverable without stealing a key from the search field.
func (m Model) tabs() []tabDef {
	t := []tabDef{
		{modeLoad, "Load"},
		{modeTable, "Components"},
		{modeParts, "Parts"},
		{modeCheck, "Check"},
	}
	if n := len(m.parts.pinned); n >= 2 {
		t = append(t, tabDef{modeCompare, fmt.Sprintf("Compare %d", n)})
	}
	return t
}

// tabMode maps a 1-based tab number to its mode, for the digit shortcuts.
func (m Model) tabMode(n int) (mode, bool) {
	t := m.tabs()
	if n < 1 || n > len(t) {
		return modeLoad, false
	}
	return t[n-1].mode, true
}

const (
	dragNone = iota
	dragVert
	dragHorz
)

type Model struct {
	srcs   []part.Provider
	srcIdx int
	mode   mode
	w, h   int
	err    string
	status string
	flash  string

	spin    spinner.Model
	loading bool

	load    loadState
	search  searchState
	parts   partsState
	compare compareState
	boardv  boardState
	check   checkState

	name       string
	pcbPath    string // empty for a CSV design
	bomPath    string // set when the line items came from a CSV
	cplPath    string // set when the placements came from a CSV
	items      []kicad.Item
	placements []kicad.Placement
	board      *kicad.Board
	assigned   []*part.Part
	excluded   []bool
	layers     int
	boardW     float64
	boardH     float64

	// cplArg is the placement csv named on the command line, used when the
	// design being opened is a BOM csv.
	cplArg string

	cursor int
	top    int
	hoff   int
	drag   int

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

// Options are the startup inputs, all optional.
type Options struct {
	Project string // path to open on start: a board, a folder, or a BOM csv
	CPL     string // placement csv to pair with a BOM csv
	Source  string // parts source to open with; empty uses the configured default
}

// New builds the initial model.
func New(opt Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot

	srcs := source.New()
	idx, unknown := source.Start(srcs, config.Load(), opt.Source)

	m := Model{srcs: srcs, srcIdx: idx, mode: modeLoad, spin: sp, cplArg: opt.CPL}
	if unknown != "" {
		m.err = fmt.Sprintf("unknown source %q — using %s (have: %s)",
			unknown, srcs[idx].ID(), strings.Join(source.IDs(srcs), ", "))
	}
	m.load = newLoadState(opt.Project)
	m.search = newSearchState()
	m.parts = newPartsState()
	m.check = newCheckState()
	return m
}

// src is the parts source in use. It is nil only for a hand-built Model in a
// test, so every command that needs it checks.
func (m Model) src() part.Provider {
	if m.srcIdx < 0 || m.srcIdx >= len(m.srcs) {
		return nil
	}
	return m.srcs[m.srcIdx]
}

// srcID names the active source, for status text and cache keys.
func (m Model) srcID() string {
	if s := m.src(); s != nil {
		return s.ID()
	}
	return ""
}

// nextSrc moves to the following source, wrapping around.
func (m Model) nextSrc() Model {
	if len(m.srcs) > 1 {
		m.srcIdx = (m.srcIdx + 1) % len(m.srcs)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.load.focusCmd())
}

type projectLoadedMsg struct {
	design *kicad.Design
	err    error
}

type searchDoneMsg struct {
	token int
	res   part.Result
	err   error
}

type detailDoneMsg struct {
	idx  int
	part part.Part
	err  error
}

type exportDoneMsg struct {
	path string
	err  error
}

type autoAssignedMsg struct {
	idx  int
	part part.Part
	ok   bool
	err  error
}

// errNoSource can only happen for a Model built by hand in a test; the real one
// always has a source. Commands report it instead of returning a nil Cmd so
// in-flight counters still settle.
var errNoSource = errors.New("no parts source configured")

func (m Model) autoAssignCmd(idx int) tea.Cmd {
	it := m.items[idx]
	kw := searchKeyword(it)
	kind := deriveKind(it.Value, refPrefix(it.ID()))
	pkg := sizeCode.FindString(it.Footprint)
	src := m.src()
	return func() tea.Msg {
		if src == nil {
			return autoAssignedMsg{idx: idx, err: errNoSource}
		}
		res, err := src.Search(part.Query{Keyword: kw, Size: 100})
		if err != nil {
			return autoAssignedMsg{idx: idx, err: err}
		}
		p, ok := pickBest(it, kind, pkg, res.Items)
		return autoAssignedMsg{idx: idx, part: p, ok: ok}
	}
}

func loadProjectCmd(path, cplPath string) tea.Cmd {
	return func() tea.Msg {
		d, err := kicad.Load(path, cplPath)
		if err != nil {
			return projectLoadedMsg{err: err}
		}
		return projectLoadedMsg{design: d}
	}
}

func (m Model) searchCmd(token int, keyword string) tea.Cmd {
	src := m.src()
	basicOnly := m.search.basicOnly
	return func() tea.Msg {
		if src == nil {
			return searchDoneMsg{token: token, err: errNoSource}
		}
		res, err := src.Search(part.Query{Keyword: keyword, Size: 100, BasicOnly: basicOnly})
		return searchDoneMsg{token: token, res: res, err: err}
	}
}

func (m Model) detailCmd(idx int, code string) tea.Cmd {
	src := m.src()
	return func() tea.Msg {
		if src == nil {
			return detailDoneMsg{idx: idx, err: errNoSource}
		}
		p, err := src.Detail(code)
		return detailDoneMsg{idx: idx, part: p, err: err}
	}
}

type refreshDoneMsg struct {
	idx  int
	part part.Part
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
	src := m.src()
	return func() tea.Msg {
		if src == nil {
			return refreshDoneMsg{idx: idx, err: errNoSource}
		}
		p, err := src.Refresh(code)
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

// fromBoard reports whether a .kicad_pcb backs the loaded design; gerbers, the
// 3D render and write-back all need one.
func (m Model) fromBoard() bool { return m.pcbPath != "" }

// sourcePath is the file the design was opened from, whichever kind it is.
func (m Model) sourcePath() string {
	if m.pcbPath != "" {
		return m.pcbPath
	}
	return m.bomPath
}

// libOf is the assembly library standing of the part assigned to line i, or
// LibUnknown when nothing is assigned or the source doesn't report it.
func (m Model) libOf(i int) part.LibKind {
	if i >= 0 && i < len(m.assigned) {
		if p := m.assigned[i]; p != nil {
			return p.Lib
		}
	}
	return part.LibUnknown
}

// extCount is how many active line items use an extended-library part, each of
// which adds a per-part setup fee to an assembly order.
func (m Model) extCount() int {
	n := 0
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		if m.libOf(i) == part.LibExtended {
			n++
		}
	}
	return n
}

// libBreakdown counts active line items by assembly library standing. known is
// how many reported one at all, so callers can tell "no extended parts" apart
// from "no library data".
func (m Model) libBreakdown() (basic, preferred, extended, known int) {
	for i := range m.items {
		if i < len(m.excluded) && m.excluded[i] {
			continue
		}
		switch m.libOf(i) {
		case part.LibBasic:
			basic++
		case part.LibPreferred:
			preferred++
		case part.LibExtended:
			extended++
		default:
			continue
		}
		known++
	}
	return
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
	na := make([]*part.Part, len(m.assigned))
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
