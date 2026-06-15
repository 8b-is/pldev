package recipe

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

// Options configures a recipe run.
type Options struct {
	DryRun bool
	Yes    bool             // skip per-step confirms
	Rep    *report.Reporter // nil => a stderr reporter scoped to the recipe
}

// confirm is the gate used for mutating steps; overridable in tests.
var confirm = tui.Confirm

// Run executes a recipe step by step: satisfied steps are skipped, dry-run
// previews mutations without applying, mutating steps confirm unless Yes, and a
// failed step is logged and the run continues. Returns an error if any step
// failed.
func Run(ctx context.Context, r Recipe, opts Options) error {
	rep := opts.Rep
	if rep == nil {
		rep = report.New(os.Stderr, "recipe")
	}
	rep = rep.Sub(r.Name)
	rep.SetDryRun(opts.DryRun)

	c := Context{Ctx: ctx, DryRun: opts.DryRun, Yes: opts.Yes, Rep: rep}
	var applied, satisfied, skipped, failed int
	var fails []string

	for _, step := range r.Steps {
		action, target := step.Label()

		st, err := step.Check(c)
		if err != nil {
			failed++
			fails = append(fails, target+": "+err.Error())
			rep.Fail(action, target, err)
			continue
		}
		if st == StatusSatisfied {
			satisfied++
			rep.Have(action, target)
			continue
		}
		if opts.DryRun {
			rep.Do(action, target) // reporter in dry-run => status=dry
			continue
		}
		if step.Mutating() && !opts.Yes && !confirm(fmt.Sprintf("Apply: %s %s?", action, target)) {
			skipped++
			rep.Skip(action, target, "declined")
			continue
		}
		rep.Do(action, target)
		if err := step.Apply(c); err != nil {
			failed++
			fails = append(fails, target+": "+err.Error())
			rep.Fail(action, target, err)
			continue
		}
		applied++
		rep.OK(action, target)
	}

	rep.Info("recipe-summary",
		report.KV{K: "applied", V: strconv.Itoa(applied)},
		report.KV{K: "satisfied", V: strconv.Itoa(satisfied)},
		report.KV{K: "skipped", V: strconv.Itoa(skipped)},
		report.KV{K: "failed", V: strconv.Itoa(failed)},
	)
	if failed > 0 {
		return fmt.Errorf("recipe %s: %d step(s) failed: %v", r.Name, failed, fails)
	}
	return nil
}
