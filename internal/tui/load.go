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

type loadState struct {
	field textfield
}

func newLoadState(project string) loadState {
	f := newField("› ", "a .kicad_pcb, a project folder, or a BOM .csv", 60)
	f.SetValue(project)
	f.Focus()
	return loadState{field: f}
}

func (ls *loadState) focusCmd() tea.Cmd {
	ls.field.Focus()
	return nil
}

func (m Model) updateLoad(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
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
	case "tab":
		if next, ok := completePath(m.load.field.Value()); ok {
			m.load.field.SetValue(next)
		}
		return m, nil
	}
	m.load.field.Update(msg)
	return m, nil
}

func (m Model) viewLoad(width, height int) string {
	header := lipgloss.JoinVertical(lipgloss.Center,
		logo(),
		"",
		subtleStyle.Render("KiCad Fabrication tool"),
		dimStyle.Render("◆ board   ▤ bom csv"),
	)
	form := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Project  ")+m.load.field.View(),
		"",
		m.renderListing(m.load.field.Value()),
		"",
		dimStyle.Render("tab complete · enter open · ^a select all · ^w delete word"),
	)
	body := lipgloss.JoinVertical(lipgloss.Center, header, "", form)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderListing(input string) string {
	dir, filter, entries := listDir(input, 9)
	head := dimStyle.Render(dir)
	if filter != "" {
		head += subtleStyle.Render("  ") + warnStyle.Render(filter)
	}
	if len(entries) == 0 {
		return head + "\n" + dimStyle.Render("  (no matches)")
	}
	lines := []string{head}
	for _, e := range entries {
		switch {
		case e.isDir:
			lines = append(lines, accentStyle.Render("  ▸ "+e.name+"/"))
		case e.kind == entryKicad:
			lines = append(lines, codeStyle.Render("  ◆ "+e.name))
		case e.kind == entryBOM:
			lines = append(lines, okStyle.Render("  ▤ "+e.name))
		default:
			lines = append(lines, dimStyle.Render("  · "+e.name))
		}
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
