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

const loadHintW = 70 // fixed, or the centred block shifts when focus moves

// loadState has three keyboard states, told apart by field.Focused() and cursor:
// typing the path, a listing row picked, or neither — page focused, tab keys live.
type loadState struct {
	field  textfield
	cursor int // highlighted listing row, or -1
}

func newLoadState(project string) loadState {
	f := newField("› ", "a .kicad_pcb, a project folder, or a BOM .csv", 60)
	f.SetValue(project)
	f.Focus()
	return loadState{field: f, cursor: -1}
}

// focusPath is called by Init, where there's nothing else to do, but not by
// gotoTab.
func (ls *loadState) focusPath() tea.Cmd {
	ls.field.Focus()
	ls.cursor = -1
	return nil
}

// loadEntries keeps key handling and rendering agreeing on what row 3 is.
func (m Model) loadEntries() []fsEntry {
	_, _, entries := listDir(m.load.field.Value(), loadListing)
	return entries
}

func (m Model) focusLoadField() (tea.Model, tea.Cmd) {
	m.load.cursor = -1
	m.load.field.Focus()
	return m, nil
}

// openLoadEntry browses a directory or opens a file.
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
			// completes, then hands the page back with nothing left to complete
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
		case e.isDir: // ▸ means focus, so a directory can't wear it
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
	return complete(input, nil)
}

// complete extends input as far as the directory allows. keep, when given, drops
// entries the field has no use for, so a completion never lands on a file the
// caller would reject.
func complete(input string, keep func(fsEntry) bool) (string, bool) {
	dir, filter, all := listDir(input, 500)
	entries := all
	if keep != nil {
		entries = nil
		for _, e := range all {
			if e.isDir || keep(e) {
				entries = append(entries, e)
			}
		}
	}
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
