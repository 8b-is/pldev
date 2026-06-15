package mem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	var applied, skipped, failed int
	for _, f := range fixes {
		tag := "SAFE"
		if !f.Safe() {
			tag = "DESTRUCTIVE"
		}
		fmt.Printf("\n[%s] %s\n  %s\n", tag, f.Title(), f.Describe())

		prompt := "Apply this fix?"
		if !f.Safe() {
			prompt = "Apply this DESTRUCTIVE fix?"
		}
		if !tui.Confirm(prompt) {
			fmt.Println("  skipped.")
			skipped++
			continue
		}
		out, err := f.Apply(ctx)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			failed++
			continue
		}
		fmt.Printf("  done: %s\n", out)
		applied++
	}
	fmt.Printf("\nSummary: %d applied, %d skipped, %d failed\n", applied, skipped, failed)
	return nil
}
