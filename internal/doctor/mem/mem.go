package mem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/report"
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

// Run collects the report, prints it (stdout), and (unless ReportOnly) offers
// fixes via the action logger rep. Pass nil to auto-create a stderr reporter.
func Run(ctx context.Context, c probe.Collector, opts Options, rep *report.Reporter) error {
	if rep == nil {
		rep = report.New(os.Stderr, "doctor")
	}
	r := probe.Collect(ctx, c, defaultRoots())
	fmt.Println(RenderPlain(r)) // data output stays on stdout

	if opts.ReportOnly {
		return nil
	}

	fixes := buildFixes(r)
	if len(fixes) == 0 {
		rep.Info("no fixes suggested")
		return nil
	}

	if opts.YesSafe {
		for _, f := range fix.SafeOnly(fixes) {
			rep.Do("fix", f.Title())
			out, err := f.Apply(ctx)
			if err != nil {
				rep.Fail("fix", f.Title(), err, report.KV{K: "out", V: out})
			} else {
				rep.OK("fix", f.Title(), report.KV{K: "out", V: out})
			}
		}
		return nil
	}

	return interactiveFixMenu(ctx, fixes, rep)
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

func interactiveFixMenu(ctx context.Context, fixes []fix.Fix, rep *report.Reporter) error {
	var applied, skipped, failed int
	for _, f := range fixes {
		safety := "safe"
		if !f.Safe() {
			safety = "destructive"
		}
		// Print fix description to stdout so user can read it before confirming.
		fmt.Printf("\n[%s] %s\n  %s\n", safety, f.Title(), f.Describe())

		prompt := "Apply this fix?"
		if !f.Safe() {
			prompt = "Apply this DESTRUCTIVE fix?"
		}
		if !tui.Confirm(prompt) {
			rep.Skip("fix", f.Title(), "user skipped")
			skipped++
			continue
		}
		rep.Do("fix", f.Title())
		out, err := f.Apply(ctx)
		if err != nil {
			rep.Fail("fix", f.Title(), err)
			failed++
			continue
		}
		rep.OK("fix", f.Title(), report.KV{K: "out", V: out})
		applied++
	}
	rep.Info("doctor-summary",
		report.KV{K: "applied", V: strconv.Itoa(applied)},
		report.KV{K: "skipped", V: strconv.Itoa(skipped)},
		report.KV{K: "failed", V: strconv.Itoa(failed)},
	)
	return nil
}
