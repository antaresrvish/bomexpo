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
	project := flag.String("project", "",
		"a .kicad_pcb, a project folder or .kicad_pro, or a BOM .csv")
	cpl := flag.String("cpl", "",
		"placement .csv to pair with a BOM .csv (found automatically when it sits beside it)")
	src := flag.String("source", "",
		"parts source to open with: "+strings.Join(source.IDs(source.New()), ", "))
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("bomexpo", version)
		return
	}

	// bomexpo board.kicad_pcb  ·  bomexpo bom.csv cpl.csv
	p, c := *project, *cpl
	if p == "" && flag.NArg() > 0 {
		p = flag.Arg(0)
	}
	if c == "" && flag.NArg() > 1 {
		c = flag.Arg(1)
	}

	prog := tea.NewProgram(tui.New(tui.Options{Project: p, CPL: c, Source: *src}))
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
