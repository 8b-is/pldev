package mem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

type Options struct {
	ReportOnly bool
	YesSafe    bool
}

func defaultRoots() []string {
	home := os.Getenv("HOME")
	return []string{filepath.Join(home, "workspace")}
}

// Run collects the report, prints it, and (unless ReportOnly) offers fixes.
func Run(ctx context.Context, c probe.Collector, opts Options) error {
	r := probe.Collect(ctx, c, defaultRoots())
	fmt.Println(RenderPlain(r))

	if opts.ReportOnly {
		return nil
	}

	fixes := buildFixes(r)
	if len(fixes) == 0 {
		fmt.Println("No fixes suggested. System looks OK.")
		return nil
	}

	if opts.YesSafe {
		for _, f := range fix.SafeOnly(fixes) {
			out, err := f.Apply(ctx)
			fmt.Printf("[auto] %s: %s %v\n", f.Title(), out, err)
		}
		return nil
	}

	return interactiveFixMenu(ctx, fixes)
}

func buildFixes(r probe.MemReport) []fix.Fix {
	var fixes []fix.Fix
	// reboot hint
	if r.Uptime > 7*24*time.Hour && probe.SwapSeverity(r.Swap.Used, r.Swap.Total) != probe.SevOK {
		fixes = append(fixes, fix.RebootHint{Reason: fmt.Sprintf(
			"uptime %s + high swap — reboot clears leaked daemons and swap", r.Uptime.Round(time.Hour))})
	}
	// node_modules purge (top dirs)
	if len(r.NodeModules) > 0 {
		var paths []string
		for _, d := range r.NodeModules {
			paths = append(paths, d.Path)
		}
		fixes = append(fixes, fix.PurgeNodeModules{Dirs: paths})
	}
	fixes = append(fixes, platformFixes(r)...)
	return fixes
}

func interactiveFixMenu(ctx context.Context, fixes []fix.Fix) error {
	items := make([]string, len(fixes))
	for i, f := range fixes {
		tag := "SAFE"
		if !f.Safe() {
			tag = "DESTRUCTIVE"
		}
		items[i] = fmt.Sprintf("[%s] %s — %s", tag, f.Title(), f.Describe())
	}
	items = append(items, "Quit")

	m := tui.NewSelect("doctor mem — choose a fix", items)
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	sm := res.(tui.SelectModel)
	if sm.Aborted || sm.Chosen < 0 || sm.Chosen >= len(fixes) {
		return nil
	}
	chosen := fixes[sm.Chosen]
	if !chosen.Safe() {
		if !tui.Confirm(fmt.Sprintf("Confirm DESTRUCTIVE: %s", chosen.Describe())) {
			fmt.Println("aborted.")
			return nil
		}
	}
	out, err := chosen.Apply(ctx)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return err
	}
	fmt.Printf("done: %s\n", out)
	return nil
}
