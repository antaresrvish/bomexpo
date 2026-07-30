package tui

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bomexpo/internal/kicad"
)

// Every order bomexpo exports is a zip with bom.csv inside it, sitting next to the
// project. So "what did I actually send last time" is already on disk — the reason
// anyone opens Verify after a bad assembly run — and there is no need to make them
// go and find it.

// maxOrdersScanned bounds how many archives get opened looking for a BOM.
const maxOrdersScanned = 8

var errNoBOMInZip = errors.New("no bom csv inside that zip")

// orderFile is one exported order.
type orderFile struct {
	Path string
	When time.Time
}

// Name is what to show in a list.
func (o orderFile) Name() string { return filepath.Base(o.Path) }

// orderZips lists the order zips beside a design, newest first.
func orderZips(designPath string) []orderFile {
	if designPath == "" {
		return nil
	}
	// Only zips that look like an order, and only the newest few opened: globbing
	// *.zip and reading every archive in the folder would be slow and would poke at
	// files that are none of our business.
	dir := filepath.Dir(designPath)
	hits, err := filepath.Glob(filepath.Join(dir, "*order*.zip"))
	if err != nil {
		return nil
	}
	type cand struct {
		path string
		when time.Time
	}
	var cands []cand
	for _, p := range hits {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			cands = append(cands, cand{p, fi.ModTime()})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].when.After(cands[j].when) })
	if len(cands) > maxOrdersScanned {
		cands = cands[:maxOrdersScanned]
	}
	var out []orderFile
	for _, c := range cands {
		if !hasBOM(c.path) {
			continue
		}
		out = append(out, orderFile{Path: c.path, When: c.when})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	return out
}

// lastOrder is the most recent exported order beside a design.
func lastOrder(designPath string) (string, time.Time) {
	if z := orderZips(designPath); len(z) > 0 {
		return z[0].Path, z[0].When
	}
	return "", time.Time{}
}

// hasBOM reports whether a zip carries a BOM we could read.
func hasBOM(path string) bool {
	r, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer r.Close()
	return zipBOM(r) != nil
}

// zipBOM finds the BOM inside an order zip. bomexpo writes it as bom.csv at the
// root; another tool's zip might name it differently, so anything csv-looking with
// "bom" in the name counts.
func zipBOM(r *zip.ReadCloser) *zip.File {
	var fallback *zip.File
	for _, f := range r.File {
		name := strings.ToLower(filepath.Base(f.Name))
		if !strings.HasSuffix(name, ".csv") {
			continue
		}
		if name == "bom.csv" {
			return f
		}
		if strings.Contains(name, "bom") && fallback == nil {
			fallback = f
		}
	}
	return fallback
}

// importOrderBOM reads the BOM out of an order zip, or straight off disk when the
// path is a plain csv.
func importOrderBOM(path string) ([]kicad.Item, error) {
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return kicad.ImportBOM(path)
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	f := zipBOM(r)
	if f == nil {
		return nil, errNoBOMInZip
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// ImportBOM reads a file, so the entry is unpacked to a temporary one.
	tmp, err := os.CreateTemp("", "bomexpo-order-*.csv")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.ReadFrom(rc); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()
	return kicad.ImportBOM(tmp.Name())
}
