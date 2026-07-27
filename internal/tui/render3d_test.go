package tui

import (
	"os"
	"testing"
)

func TestImageProtocol(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"warp":     {map[string]string{"TERM_PROGRAM": "WarpTerminal", "TERM": "xterm-256color"}, ""},
		"iterm":    {map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM": "xterm-256color"}, "iterm2"},
		"ghostty":  {map[string]string{"TERM_PROGRAM": "ghostty", "TERM": "xterm-256color"}, "kitty"},
		"terminal": {map[string]string{"TERM_PROGRAM": "Apple_Terminal", "TERM": "xterm-256color"}, ""},
	}
	for name, c := range cases {
		os.Unsetenv("KITTY_WINDOW_ID")
		for k, v := range c.env {
			os.Setenv(k, v)
		}
		if got := imageProtocol(); got != c.want {
			t.Errorf("%s: imageProtocol()=%q want %q", name, got, c.want)
		}
	}
}

func TestRenderBoard(t *testing.T) {
	proj := os.Getenv("BOMEXPO_PROJ")
	if proj == "" || testing.Short() {
		t.Skip("set BOMEXPO_PROJ")
	}
	if kicadCLI() == "" {
		t.Skip("kicad-cli not found")
	}
	p1, err := renderBoard(proj, "top")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p1); err != nil || fi.Size() == 0 {
		t.Fatalf("no render output: %v", err)
	}
	p2, err := renderBoard(proj, "top")
	if err != nil || p2 != p1 {
		t.Fatalf("cache miss: %q vs %q (%v)", p1, p2, err)
	}
	t.Logf("rendered + cached: %s", p1)
}
