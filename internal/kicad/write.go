package kicad

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type WriteResult struct {
	CodesUpdated  int
	CodesInserted int
	Excluded      int
	Included      int
}

// WriteBack writes bomexpo's work back into the .kicad_pcb, keyed by reference
// designator: LCSC codes from `codes`, and the exclude-from-BOM flag from
// `exclude` (a ref present with value true gets excluded, false gets
// re-included; refs absent from the map are left untouched). Existing LCSC
// properties are updated in place, missing ones get a minimal property line,
// and the (attr …) list is rebuilt to add or drop the exclude tokens.
// Everything else is preserved byte-for-byte and the result is re-parsed
// before it replaces the original.
func WriteBack(pcbPath string, codes map[string]string, exclude map[string]bool) (WriteResult, error) {
	var res WriteResult
	data, err := os.ReadFile(pcbPath)
	if err != nil {
		return res, err
	}
	src := string(data)
	root, err := parseSexp(src)
	if err != nil {
		return res, fmt.Errorf("parse pcb: %w", err)
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
		ref := propValue(n, "reference")
		if ref == "" {
			continue
		}
		if code := codes[ref]; code != "" {
			if lc := lcscProp(n); lc != nil {
				if v := lc.kids[2]; v.atom != code {
					edits = append(edits, edit{v.start, v.end, quoteAtom(code)})
					res.CodesUpdated++
				}
			} else if a := anchorProp(n); a != nil {
				edits = append(edits, edit{a.end, a.end, "\n\t\t(property \"LCSC\" " + quoteAtom(code) + ")"})
				res.CodesInserted++
			}
		}
		if want, ok := exclude[ref]; ok {
			if at, end, text, changed := attrEdit(n, want); changed {
				edits = append(edits, edit{at, end, text})
				if want {
					res.Excluded++
				} else {
					res.Included++
				}
			}
		}
	}
	if len(edits) == 0 {
		return res, nil
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
		return WriteResult{}, fmt.Errorf("edit would corrupt the pcb, aborting: %w", err)
	}
	if err := writeFileAtomic(pcbPath, out); err != nil {
		return WriteResult{}, err
	}
	return res, nil
}

// attrEdit produces the edit that adds or drops the exclude-from-BOM tokens on
// a footprint's (attr …) list, or changed=false when it already matches.
func attrEdit(fp *node, want bool) (at, end int, text string, changed bool) {
	const bom, pos = "exclude_from_bom", "exclude_from_pos_files"
	attr := child(fp, "attr")
	if attr == nil {
		if !want {
			return 0, 0, "", false
		}
		anchor := child(fp, "layer")
		if anchor == nil {
			return 0, 0, "", false
		}
		return anchor.end, anchor.end, "\n\t\t(attr " + bom + " " + pos + ")", true
	}
	has := map[string]bool{}
	var toks []string
	for _, a := range attr.kids[1:] {
		has[a.atom] = true
		toks = append(toks, a.atom)
	}
	var target []string
	if want {
		if has[bom] && has[pos] {
			return 0, 0, "", false
		}
		target = append(target, toks...)
		if !has[bom] {
			target = append(target, bom)
		}
		if !has[pos] {
			target = append(target, pos)
		}
	} else {
		if !has[bom] && !has[pos] {
			return 0, 0, "", false
		}
		for _, t := range toks {
			if t != bom && t != pos {
				target = append(target, t)
			}
		}
	}
	rebuilt := "(attr"
	if len(target) > 0 {
		rebuilt += " " + strings.Join(target, " ")
	}
	rebuilt += ")"
	return attr.start, attr.end, rebuilt, true
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
