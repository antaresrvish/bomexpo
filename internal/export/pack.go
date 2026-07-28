package export

import (
	"archive/zip"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"bomexpo/internal/kicad"
)

func WriteOrderZip(outPath string, items []kicad.Item, placements []kicad.Placement, pcbPath string, exclude map[string]bool, rotOverride map[string]int) error {
	full, err := kicad.ExpandPath(outPath)
	if err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	if err := writeBOM(zw, items); err != nil {
		return err
	}

	cpl := placements
	if pcbPath != "" {
		if pos, err := genPositions(pcbPath); err == nil && len(pos) > 0 {
			cpl = pos
		}
	}
	if len(cpl) > 0 {
		if err := writeCPL(zw, cpl, exclude, rotOverride); err != nil {
			return err
		}
	}
	if pcbPath != "" {
		if err := addGerbers(zw, pcbPath); err != nil {
			return err
		}
	}
	return nil
}

func genPositions(pcbPath string) ([]kicad.Placement, error) {
	cli := findKicadCLI()
	if cli == "" {
		return nil, nil
	}
	tmp, err := os.MkdirTemp("", "bomexpo-pos")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	out := filepath.Join(tmp, "pos.csv")
	args := []string{"pcb", "export", "pos", "--format", "csv", "--units", "mm", "--side", "both", "-o", out, pcbPath}
	if o, err := exec.Command(cli, args...).CombinedOutput(); err != nil {
		return nil, &cliError{args: args, out: string(o), err: err}
	}
	return kicad.ImportCPL(out)
}

func writeBOM(zw *zip.Writer, items []kicad.Item) error {
	w, err := zw.Create("bom.csv")
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Write([]string{"Comment", "Designator", "Footprint", "Quantity", "LCSC Part #"})
	for _, it := range items {
		cw.Write([]string{it.Value, strings.Join(it.Designators, ","), it.Footprint, strconv.Itoa(it.Quantity), it.LCSC})
	}
	cw.Flush()
	return cw.Error()
}

func writeCPL(zw *zip.Writer, placements []kicad.Placement, exclude map[string]bool, rotOverride map[string]int) error {
	w, err := zw.Create("positions.csv")
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Write([]string{"Designator", "Mid X", "Mid Y", "Layer", "Rotation"})
	for _, p := range placements {
		if exclude[p.Designator] {
			continue
		}
		rot, _ := appliedRot(p.Package, p.Rotation, p.Layer == "bottom", rotOverride, p.Designator)
		cw.Write([]string{
			p.Designator,
			strconv.FormatFloat(p.X, 'f', 4, 64),
			strconv.FormatFloat(p.Y, 'f', 4, 64),
			layerLabel(p.Layer),
			strconv.FormatFloat(rot, 'f', -1, 64),
		})
	}
	cw.Flush()
	return cw.Error()
}

func layerLabel(l string) string {
	if l == "bottom" {
		return "Bottom"
	}
	return "Top"
}

func addGerbers(zw *zip.Writer, pcbPath string) error {
	cli := findKicadCLI()
	if cli == "" {
		return nil
	}
	tmp, err := os.MkdirTemp("", "bomexpo-gbr")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	for _, args := range [][]string{
		{"pcb", "export", "gerbers", "--output", tmp + string(os.PathSeparator), pcbPath},
		{"pcb", "export", "drill", "--output", tmp + string(os.PathSeparator), pcbPath},
	} {
		if out, err := exec.Command(cli, args...).CombinedOutput(); err != nil {
			return &cliError{args: args, out: string(out), err: err}
		}
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			return err
		}
		w, err := zw.Create("gerber/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func findKicadCLI() string {
	if p, err := exec.LookPath("kicad-cli"); err == nil {
		return p
	}
	for _, p := range []string{
		"/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli",
		"/usr/bin/kicad-cli",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

type cliError struct {
	args []string
	out  string
	err  error
}

func (e *cliError) Error() string {
	return "kicad-cli " + strings.Join(e.args, " ") + ": " + e.err.Error() + " " + strings.TrimSpace(e.out)
}
