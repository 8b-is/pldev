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
	"github.com/peterlodri-sec/pldev/internal/arp"
	"github.com/peterlodri-sec/pldev/internal/fed"
	"github.com/peterlodri-sec/pldev/internal/fleet"
	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/setup"
	"github.com/peterlodri-sec/pldev/internal/setup/recipe"
	"github.com/peterlodri-sec/pldev/internal/sysinfo"
	"github.com/peterlodri-sec/pldev/internal/tui"
	"github.com/peterlodri-sec/pldev/internal/verify"
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
	// direct subcommand: pldev arp <sync|verify>
	if len(positional) >= 1 && positional[0] == "arp" {
		arep := rep.Sub("arp")
		clone := arp.DefaultClonePath()
		var err error
		switch {
		case len(positional) >= 2 && positional[1] == "sync":
			err = arp.Sync(ctx, arep, clone)
		case len(positional) >= 2 && positional[1] == "verify":
			var out string
			out, err = arp.Verify(ctx, clone)
			if err == nil {
				arep.OK("arp-verify", clone)
			} else {
				arep.Fail("arp-verify", clone, err, report.KV{K: "out", V: strings.TrimSpace(out)})
			}
		default:
			fmt.Fprintln(os.Stderr, "usage: pldev arp [sync | verify]")
			os.Exit(2)
		}
		if err != nil {
			os.Exit(1)
		}
		return
	}
	// direct subcommand: pldev fleet <list|health|ssh|run>
	if len(positional) >= 1 && positional[0] == "fleet" {
		runFleet(ctx, rep.Sub("fleet"), positional, setupDryRun, opts.YesSafe)
		return
	}
	// direct subcommand: pldev fed status (federation observability)
	if len(positional) >= 1 && positional[0] == "fed" {
		sub := ""
		if len(positional) >= 2 {
			sub = positional[1]
		}
		if sub != "" && sub != "status" {
			fmt.Fprintln(os.Stderr, "usage: pldev fed status")
			os.Exit(2)
		}
		ts, err := fleet.Discover(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: tailscale discovery failed:", err)
			os.Exit(1)
		}
		cfg, _ := fleet.LoadFleetConfig(fleet.DefaultFleetConfigPath())
		nodes := fleet.Merge(ts, cfg)
		base, err := fed.BusURL(nodes, cfg.Federation.MonitorURL)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := fed.Status(ctx, rep.Sub("fed"), nodes, base); err != nil {
			os.Exit(1)
		}
		return
	}
	// direct subcommand: pldev verify <path> (binary provenance — verify a target)
	if len(positional) >= 1 && positional[0] == "verify" {
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pldev verify <path-to-binary>")
			os.Exit(2)
		}
		if err := verify.Verify(ctx, rep.Sub("verify"), positional[1]); err != nil {
			os.Exit(1)
		}
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

func runFleet(ctx context.Context, rep *report.Reporter, pos []string, dryRun, yes bool) {
	ts, err := fleet.Discover(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: tailscale discovery failed:", err)
		os.Exit(1)
	}
	cfg, _ := fleet.LoadFleetConfig(fleet.DefaultFleetConfigPath())
	nodes := fleet.Merge(ts, cfg)

	sub := ""
	if len(pos) >= 2 {
		sub = pos[1]
	}
	switch sub {
	case "", "list":
		fmt.Print(fleet.Render(nodes)) // data → stdout
	case "health":
		fleet.Health(ctx, rep, nodes)
	case "ssh":
		if len(pos) < 3 {
			fmt.Fprintln(os.Stderr, "usage: pldev fleet ssh <node>")
			os.Exit(2)
		}
		sel := fleet.Select(nodes, pos[2])
		if len(sel) != 1 {
			fmt.Fprintf(os.Stderr, "fleet ssh: %q matched %d nodes (need exactly 1)\n", pos[2], len(sel))
			os.Exit(2)
		}
		if err := fleet.SSH(ctx, sel[0]); err != nil {
			os.Exit(1)
		}
	case "run":
		// pldev fleet run <selector> -- <cmd...>
		dd := -1
		for i, a := range pos {
			if a == "--" {
				dd = i
				break
			}
		}
		if len(pos) < 3 || dd < 0 || dd+1 >= len(pos) {
			fmt.Fprintln(os.Stderr, "usage: pldev fleet run <selector> -- <cmd> [--dry-run] [--yes]")
			os.Exit(2)
		}
		sel := fleet.Select(nodes, pos[2])
		if err := fleet.Run(ctx, rep, sel, pos[dd+1:], dryRun, yes); err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: pldev fleet [list | health | ssh <node> | run <selector> -- <cmd>]")
		os.Exit(2)
	}
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
