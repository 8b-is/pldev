package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/buildinfo"
	"github.com/peterlodri-sec/pldev/internal/doctor/mem"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/setup"
	"github.com/peterlodri-sec/pldev/internal/setup/recipe"
	"github.com/peterlodri-sec/pldev/internal/sysinfo"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

func main() {
	args := os.Args[1:]
	opts := mem.Options{}
	var setupDryRun, setupList, showVersion bool
	var positional []string
	for _, a := range args {
		switch a {
		case "--report":
			opts.ReportOnly = true
		case "--yes":
			opts.YesSafe = true
		case "--no-color":
			os.Setenv("NO_COLOR", "1")
		case "--dry-run":
			setupDryRun = true
		case "--list":
			setupList = true
		case "--version", "-v":
			showVersion = true
		default:
			positional = append(positional, a)
		}
	}

	// Build info: `pldev --version` / `pldev version` prints and exits.
	if showVersion || (len(positional) >= 1 && positional[0] == "version") {
		fmt.Println(buildinfo.String())
		return
	}

	ctx := context.Background()

	// Root action logger. One build-info line per invocation (stderr; keeps
	// stdout data pipe-clean).
	rep := report.New(os.Stderr, "pldev")
	rep.Info(buildinfo.String())

	// direct subcommand: pldev doctor mem
	if len(positional) >= 2 && positional[0] == "doctor" && positional[1] == "mem" {
		runMem(ctx, opts, rep.Sub("doctor.mem"))
		return
	}
	// direct subcommand: pldev ps
	if len(positional) >= 1 && positional[0] == "ps" {
		runPS(ctx)
		return
	}
	// direct subcommand: pldev setup <recipe> (composite flows like ruflo)
	if len(positional) >= 2 && positional[0] == "setup" {
		if r, ok := recipe.Named(positional[1]); ok {
			err := recipe.Run(ctx, r, recipe.Options{
				DryRun: setupDryRun,
				Yes:    opts.YesSafe,
				Rep:    rep.Sub("setup"),
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			return
		}
	}
	// direct subcommand: pldev setup (package install)
	if len(positional) >= 1 && positional[0] == "setup" {
		toks := strings.Join(positional[1:], "+")
		err := setup.Run(ctx, setup.Options{
			Tokens: toks,
			DryRun: setupDryRun,
			Yes:    opts.YesSafe,
			List:   setupList,
		}, rep.Sub("setup"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if len(positional) == 0 {
		rootMenu(ctx, opts, rep)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: pldev [doctor mem | ps | setup | version] [--report] [--yes] [--no-color] [--dry-run] [--list] [--version]")
	os.Exit(2)
}

func runPS(ctx context.Context) {
	info := sysinfo.Collect(ctx, sysinfo.New(), probe.New())
	color := os.Getenv("NO_COLOR") == ""
	fmt.Print(sysinfo.RenderPanel(info, color))
}

func runMem(ctx context.Context, opts mem.Options, rep *report.Reporter) {
	if err := mem.Run(ctx, probe.New(), opts, rep); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootMenu(ctx context.Context, opts mem.Options, rep *report.Reporter) {
	m := tui.NewSelect("pldev", []string{"doctor mem", "system info (ps)", "setup", "Quit"})
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sm := res.(tui.SelectModel)
	switch sm.Chosen {
	case 0:
		runMem(ctx, opts, rep.Sub("doctor.mem"))
	case 1:
		runPS(ctx)
	case 2:
		if err := setup.Run(ctx, setup.Options{}, rep.Sub("setup")); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
}
