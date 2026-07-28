package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func kicadCLI() string {
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

func renderBoard(pcbPath, side string) (string, error) {
	cli := kicadCLI()
	if cli == "" {
		return "", fmt.Errorf("kicad-cli not found")
	}
	fi, err := os.Stat(pcbPath)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "bomexpo-render")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s-%d-%s", strings.TrimSuffix(filepath.Base(pcbPath), ".kicad_pcb"), fi.ModTime().UnixNano(), side)
	out := filepath.Join(dir, key+".png")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	args := []string{"pcb", "render", "--quality", "high", "--width", "1600", "--height", "1200",
		"--background", "opaque", "-o", out}
	switch side {
	case "bottom":
		args = append(args, "--side", "bottom")
	case "iso":
		args = append(args, "--rotate", "-30,0,25", "--perspective")
	default:
		args = append(args, "--side", "top")
	}
	args = append(args, pcbPath)

	if o, err := exec.Command(cli, args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("render: %v %s", err, strings.TrimSpace(string(o)))
	}
	return out, nil
}

func openExternal(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
