package tui

import (
	"encoding/base64"
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

// imageProtocol reports which inline-image escape protocol the terminal speaks,
// or "" (e.g. Warp, Terminal.app) which means fall back to opening externally.
func imageProtocol() string {
	if os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(os.Getenv("TERM"), "kitty") {
		return "kitty"
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm":
		return "iterm2"
	case "ghostty":
		return "kitty"
	}
	return ""
}

func inlineImage(path string, cols, rows int) (string, bool) {
	proto := imageProtocol()
	if proto == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	switch proto {
	case "iterm2":
		return fmt.Sprintf("\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a", cols, rows, b64), true
	case "kitty":
		return kittyImage(b64, cols, rows), true
	}
	return "", false
}

func kittyImage(b64 string, cols, rows int) string {
	var b strings.Builder
	const chunk = 4096
	first := true
	for len(b64) > 0 {
		n := len(b64)
		if n > chunk {
			n = chunk
		}
		part := b64[:n]
		b64 = b64[n:]
		more := "0"
		if len(b64) > 0 {
			more = "1"
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,c=%d,r=%d,m=%s;%s\x1b\\", cols, rows, more, part)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%s;%s\x1b\\", more, part)
		}
	}
	return b.String()
}
