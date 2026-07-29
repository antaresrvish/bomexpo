package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bomexpo/internal/kicad"
)

// loadListing is how many directory entries the listing shows at once.
const loadListing = 9

// loadHintW keeps the footer a fixed width so the centred block doesn't shift
// when focus moves between the path and the listing.
const loadHintW = 70

// loadState has three keyboard states, and field.Focused() plus cursor say which:
// typing in the path, a listing row picked, or neither — in which case the page
// has the keyboard and the tab keys work.
type loadState struct {
	field textfield
	// cursor is the highlighted listing row, or -1 when no row is picked.
	cursor int
}

func newLoadState(project string) loadState {
	f := newField("› ", "a .kicad_pcb, a project folder, or a BOM .csv", 60)
	f.SetValue(project)
	f.Focus()
	return loadState{field: f, cursor: -1}
}

// focusPath puts the keyboard on the path field. On first launch there's nothing
// else to do, so Init calls it; coming back to the tab later does not.
func (ls *loadState) focusPath() tea.Cmd {
	ls.field.Focus()
	ls.cursor = -1
	return nil
}

// loadEntries is the listing the user sees, so key handling and rendering can
// never disagree about what row 3 is.
func (m Model) loadEntries() []fsEntry {
	_, _, entries := listDir(m.load.field.Value(), loadListing)
	return entries
}

// focusLoadField hands the keyboard back to the path field.
func (m Model) focusLoadField() (tea.Model, tea.Cmd) {
	m.load.cursor = -1
	m.load.field.Focus()
	return m, nil
}

// openLoadEntry acts on the highlighted listing row: a directory becomes the new
// path to browse, a file gets opened.
func (m Model) openLoadEntry(e fsEntry) (tea.Model, tea.Cmd) {
	dir, _, _ := listDir(m.load.field.Value(), loadListing)
	full := filepath.Join(dir, e.name)
	if e.isDir {
		m.load.field.SetValue(full + string(filepath.Separator))
		return m.focusLoadField()
	}
	m.load.field.SetValue(full)
	return m.startLoad()
}

func (m Model) updateLoad(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	entries := m.loadEntries()
	key := msg.String()
	typing := m.load.field.Focused()

	switch key {
	case "down":
		// down walks into the listing and then through it
		if m.load.cursor+1 < len(entries) {
			m.load.cursor++
			m.load.field.Blur()
		}
		return m, nil
	case "up":
		if m.load.cursor < 0 {
			return m, nil
		}
		if m.load.cursor == 0 {
			return m.focusLoadField() // back out the top of the listing
		}
		m.load.cursor--
		return m, nil
	case "enter":
		if m.load.cursor >= 0 && m.load.cursor < len(entries) {
			return m.openLoadEntry(entries[m.load.cursor])
		}
		return m.startLoad()
	case "tab", "shift+tab":
		if typing {
			// tab completes the path, and hands the page back when there's nothing
			// left to complete — the same deal as the filter query
			if next, ok := completePath(m.load.field.Value()); ok {
				m.load.field.SetValue(next)
				return m, nil
			}
			m.load.field.Blur()
			return m, nil
		}
		return m.focusLoadField()
	case "esc":
		if typing {
			m.load.field.Blur()
			return m, nil
		}
		m.load.cursor = -1
		return m, nil
	}

	if typing {
		before := m.load.field.Value()
		m.load.field.Update(msg)
		if m.load.field.Value() != before {
			m.load.cursor = -1 // a new path means a new listing
		}
		return m, nil
	}

	// The path isn't focused, so the keys are commands — including tab switching,
	// which is the whole reason arriving here doesn't grab the keyboard.
	if mm, cmd, done := m.tabSwitchKey(key); done {
		return mm, cmd
	}
	switch key {
	case "/", "i":
		return m.focusLoadField()
	case "g", "home":
		if len(entries) > 0 {
			m.load.cursor = 0
		}
	case "G", "end":
		m.load.cursor = len(entries) - 1
	}
	return m, nil
}

// startLoad opens whatever the path field holds.
func (m Model) startLoad() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.load.field.Value())
	if path == "" {
		m.err = "enter a board or BOM path"
		return m, nil
	}
	m.err = ""
	m.loading = true
	m.status = "Reading KiCad project…"
	if kicad.IsBOMFile(path) {
		m.status = "Reading BOM…"
	}
	return m, loadProjectCmd(path, m.cplArg)
}

func (m Model) viewLoad(width, height int) string {
	header := lipgloss.JoinVertical(lipgloss.Center,
		logo(),
		"",
		subtleStyle.Render("KiCad Fabrication tool"),
		dimStyle.Render("▪ folder   ◆ board   ▤ bom csv"),
	)
	// Every hint is padded to the same width: the block is centre-placed, so a
	// shorter line would slide the whole page sideways when focus moves.
	var hint string
	switch {
	case m.load.cursor >= 0:
		hint = "↑↓ pick · enter open · tab types the path · esc clears the pick"
	case m.load.field.Focused():
		hint = "↓ browse · tab complete · enter open · ^a select all · ^w delete word"
	default:
		hint = "/ type a path · ↓ browse · enter open · 1-4 or [ ] switch tabs"
	}
	form := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Project  ")+focusMark(m.load.field.Focused())+m.load.field.View(),
		"",
		m.renderListing(m.load.field.Value()),
		"",
		dimStyle.Render(pad(hint, loadHintW)),
	)
	body := lipgloss.JoinVertical(lipgloss.Center, header, "", form)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderListing(input string) string {
	dir, filter, entries := listDir(input, loadListing)
	head := dimStyle.Render(dir)
	if filter != "" {
		head += subtleStyle.Render("  ") + warnStyle.Render(filter)
	}
	if len(entries) == 0 {
		return head + "\n" + dimStyle.Render("  (no matches)")
	}
	lines := []string{head}
	for i, e := range entries {
		glyph, style := "· ", dimStyle
		switch {
		// ▸ means focus everywhere else, so a directory can't wear it too
		case e.isDir:
			glyph, style = "▪ ", accentStyle
		case e.kind == entryKicad:
			glyph, style = "◆ ", codeStyle
		case e.kind == entryBOM:
			glyph, style = "▤ ", okStyle
		}
		name := e.name
		if e.isDir {
			name += "/"
		}
		if i == m.load.cursor {
			lines = append(lines, selRowStyle.Render("▶ "+glyph+name))
			continue
		}
		lines = append(lines, style.Render("  "+glyph+name))
	}
	return strings.Join(lines, "\n")
}

// fsEntry ranks directory entries so the files you're likely to open float to
// the top of the listing.
type fsEntry struct {
	name  string
	isDir bool
	kind  entryKind
}

type entryKind int

const (
	entryOther entryKind = iota
	entryKicad
	entryBOM
)

func listDir(input string, maxN int) (dir, filter string, entries []fsEntry) {
	if strings.TrimSpace(input) == "" {
		input = "."
	}
	full, err := kicad.ExpandPath(input)
	if err != nil {
		return input, "", nil
	}
	if strings.HasSuffix(input, "/") {
		dir, filter = full, ""
	} else {
		dir, filter = filepath.Dir(full), filepath.Base(full)
	}
	raws, err := os.ReadDir(dir)
	if err != nil {
		return dir, filter, nil
	}
	fl := strings.ToLower(filter)
	for _, e := range raws {
		name := e.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(fl, ".") {
			continue
		}
		if fl != "" && !strings.HasPrefix(strings.ToLower(name), fl) {
			continue
		}
		low := strings.ToLower(name)
		kind := entryOther
		switch {
		case strings.HasSuffix(low, ".kicad_pcb"), strings.HasSuffix(low, ".kicad_pro"):
			kind = entryKicad
		case kicad.IsBOMFile(low):
			kind = entryBOM
		}
		entries = append(entries, fsEntry{name: name, isDir: e.IsDir(), kind: kind})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		rank := func(e fsEntry) int {
			switch {
			case e.isDir:
				return 0
			case e.kind == entryKicad:
				return 1
			case e.kind == entryBOM:
				return 2
			default:
				return 3
			}
		}
		if a, b := rank(entries[i]), rank(entries[j]); a != b {
			return a < b
		}
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
	if len(entries) > maxN {
		entries = entries[:maxN]
	}
	return dir, filter, entries
}

func completePath(input string) (string, bool) {
	dir, filter, entries := listDir(input, 500)
	if len(entries) == 0 {
		return input, false
	}
	var seg string
	if len(entries) == 1 {
		seg = entries[0].name
		if entries[0].isDir {
			seg += "/"
		}
	} else {
		seg = commonPrefix(entries)
		if seg == "" || strings.EqualFold(seg, filter) {
			return input, false
		}
	}
	head := dir
	if !strings.HasSuffix(head, "/") {
		head += "/"
	}
	return head + seg, true
}

func commonPrefix(entries []fsEntry) string {
	if len(entries) == 0 {
		return ""
	}
	prefix := entries[0].name
	for _, e := range entries[1:] {
		for !strings.HasPrefix(strings.ToLower(e.name), strings.ToLower(prefix)) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}
