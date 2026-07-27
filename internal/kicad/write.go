package kicad

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteLCSC writes LCSC part codes back into the .kicad_pcb, keyed by
// reference designator. A footprint that already carries an LCSC property has
// its value replaced in place; one without gets a minimal property line after
// its Value. Everything else in the file is preserved byte-for-byte, and the
// result is re-parsed before it replaces the original.
func WriteLCSC(pcbPath string, codes map[string]string) (updated, inserted int, err error) {
	data, err := os.ReadFile(pcbPath)
	if err != nil {
		return 0, 0, err
	}
	src := string(data)
	root, err := parseSexp(src)
	if err != nil {
		return 0, 0, fmt.Errorf("parse pcb: %w", err)
	}

	type edit struct {
		at, end int
		text    string
	}
	var edits []edit
	for _, n := range root.kids {
		if h := n.head(); h != "footprint" && h != "module" {
			continue
		}
		code := codes[propValue(n, "reference")]
		if code == "" {
			continue
		}
		if lc := lcscProp(n); lc != nil {
			v := lc.kids[2]
			if v.atom == code {
				continue
			}
			edits = append(edits, edit{v.start, v.end, quoteAtom(code)})
			updated++
		} else if a := anchorProp(n); a != nil {
			edits = append(edits, edit{a.end, a.end, "\n\t\t(property \"LCSC\" " + quoteAtom(code) + ")"})
			inserted++
		}
	}
	if len(edits) == 0 {
		return 0, 0, nil
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].at > edits[j].at })
	out := []byte(src)
	for _, e := range edits {
		nb := make([]byte, 0, len(out)+len(e.text))
		nb = append(nb, out[:e.at]...)
		nb = append(nb, e.text...)
		nb = append(nb, out[e.end:]...)
		out = nb
	}
	if _, err := parseSexp(string(out)); err != nil {
		return 0, 0, fmt.Errorf("edit would corrupt the pcb, aborting: %w", err)
	}
	if err := writeFileAtomic(pcbPath, out); err != nil {
		return 0, 0, err
	}
	return updated, inserted, nil
}

func propValue(fp *node, name string) string {
	for _, k := range fp.kids {
		if k.head() == "property" && len(k.kids) >= 3 && strings.EqualFold(k.kids[1].atom, name) {
			return k.kids[2].atom
		}
	}
	return ""
}

func lcscProp(fp *node) *node {
	for _, k := range fp.kids {
		if k.head() != "property" || len(k.kids) < 3 {
			continue
		}
		n := strings.ToLower(k.kids[1].atom)
		if strings.Contains(n, "lcsc") || strings.Contains(n, "jlc") {
			return k
		}
	}
	return nil
}

func anchorProp(fp *node) *node {
	var ref *node
	for _, k := range fp.kids {
		if k.head() != "property" || len(k.kids) < 2 {
			continue
		}
		switch strings.ToLower(k.kids[1].atom) {
		case "value":
			return k
		case "reference":
			ref = k
		}
	}
	return ref
}

func quoteAtom(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bomexpo-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		os.Chmod(name, fi.Mode())
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
