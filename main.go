package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bomexpo/internal/source"
	"bomexpo/internal/tui"
)

var version = "dev"

func main() {
	project := flag.String("project", "", "KiCad project folder, .kicad_pro, or .kicad_pcb")
	src := flag.String("source", "",
		"parts source to open with: "+strings.Join(source.IDs(source.New()), ", "))
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("bomexpo", version)
		return
	}

	p := *project
	if p == "" && flag.NArg() > 0 {
		p = flag.Arg(0)
	}

	prog := tea.NewProgram(tui.New(p, *src))
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
