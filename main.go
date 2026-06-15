package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/doctor/mem"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

func main() {
	args := os.Args[1:]
	opts := mem.Options{}
	var positional []string
	for _, a := range args {
		switch a {
		case "--report":
			opts.ReportOnly = true
		case "--yes":
			opts.YesSafe = true
		case "--no-color":
			os.Setenv("NO_COLOR", "1")
		default:
			positional = append(positional, a)
		}
	}

	ctx := context.Background()

	// direct subcommand: pldev doctor mem
	if len(positional) >= 2 && positional[0] == "doctor" && positional[1] == "mem" {
		runMem(ctx, opts)
		return
	}
	if len(positional) == 0 {
		rootMenu(ctx, opts)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: pldev [doctor mem] [--report] [--yes] [--no-color]")
	os.Exit(2)
}

func runMem(ctx context.Context, opts mem.Options) {
	if err := mem.Run(ctx, probe.New(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootMenu(ctx context.Context, opts mem.Options) {
	m := tui.NewSelect("pldev", []string{"doctor mem", "Quit"})
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sm := res.(tui.SelectModel)
	if sm.Chosen == 0 {
		runMem(ctx, opts)
	}
}
