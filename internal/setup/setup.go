package setup

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

type Options struct {
	Tokens string // "" => interactive
	DryRun bool
	Yes    bool
	List   bool
}

func loadRegistry() (Registry, error) {
	user, err := LoadUserManifest(DefaultManifestPath())
	if err != nil {
		return Registry{}, fmt.Errorf("bad user manifest: %w", err)
	}
	return Merge(BuiltinRegistry(), user), nil
}

// Run executes the setup flow. rep is the action logger; pass nil to default
// to a stderr reporter scoped "setup".
func Run(ctx context.Context, opts Options, rep *report.Reporter) error {
	if rep == nil {
		rep = report.New(os.Stderr, "setup")
	}
	if opts.DryRun {
		rep.SetDryRun(true)
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	if opts.List {
		printList(reg)
		return nil
	}

	var tools []Tool
	if opts.Tokens == "" {
		tools, err = interactiveSelect(reg)
		if err != nil {
			return err
		}
		if len(tools) == 0 {
			fmt.Println("nothing selected.")
			return nil
		}
	} else {
		tools, err = Resolve(reg, opts.Tokens)
		if err != nil {
			return err
		}
	}

	plan, err := BuildPlan(ctx, reg, tools, NewDetector(), AvailableBackends())
	if err != nil {
		return err
	}
	printPlan(rep, plan)

	if opts.DryRun {
		return nil
	}
	if len(plan.ToInstall) == 0 {
		rep.Info("nothing to install")
		return nil
	}
	if !opts.Yes && !tui.Confirm(fmt.Sprintf("Install %d package(s)?", len(plan.ToInstall))) {
		rep.Info("aborted")
		return nil
	}
	return apply(ctx, plan, rep)
}

func apply(ctx context.Context, plan InstallPlan, rep *report.Reporter) error {
	var installed, failed int
	var fails []string
	for _, it := range plan.ToInstall {
		rep.Do("install", it.Tool.Name)
		if _, err := Install(ctx, it.Spec); err != nil {
			failed++
			fails = append(fails, it.Tool.Name+": "+err.Error())
			rep.Fail("install", it.Tool.Name, err)
			continue
		}
		installed++
		rep.OK("install", it.Tool.Name, report.KV{K: "via", V: string(it.Spec.Backend)})
	}
	rep.Info("setup-summary",
		report.KV{K: "installed", V: itoa(installed)},
		report.KV{K: "skipped", V: itoa(len(plan.Installed))},
		report.KV{K: "unsupported", V: itoa(len(plan.Unsupported))},
		report.KV{K: "failed", V: itoa(failed)},
	)
	if failed > 0 {
		return fmt.Errorf("failed: %s", strings.Join(fails, "; "))
	}
	return nil
}

// printPlan previews the resolved plan via the reporter. Already-installed
// tools log as have; unsupported as skip; in dry-run, pending installs log as
// dry (the preview). In a live run, pending installs are logged by apply.
func printPlan(rep *report.Reporter, p InstallPlan) {
	for _, t := range p.Installed {
		rep.Have("install", t.Name)
	}
	for _, t := range p.Unsupported {
		rep.Skip("install", t.Name, "no backend on this OS")
	}
	// In dry-run, preview each pending install as status=dry. In a live run,
	// apply logs do/ok/fail per item, so don't pre-announce here (avoids a
	// duplicate status=do line per tool).
	if rep.DryRun() {
		for _, it := range p.ToInstall {
			rep.Do("install", it.Tool.Name)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func printList(r Registry) {
	byCat := map[string][]string{}
	for n, t := range r.Tools {
		byCat[t.Category] = append(byCat[t.Category], n)
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	fmt.Println("TOOLS")
	for _, c := range cats {
		sort.Strings(byCat[c])
		fmt.Printf("  %-11s %s\n", c, strings.Join(byCat[c], ", "))
	}
	fmt.Println("PROFILES")
	pnames := make([]string, 0, len(r.Profiles))
	for n := range r.Profiles {
		pnames = append(pnames, n)
	}
	sort.Strings(pnames)
	for _, n := range pnames {
		fmt.Printf("  @%-9s %s\n", n, strings.Join(r.Profiles[n].Tools, ", "))
	}
}

func interactiveSelect(r Registry) ([]Tool, error) {
	var items []tui.MultiSelectItem
	// profiles first
	items = append(items, tui.MultiSelectItem{Label: "PROFILES", Header: true})
	pnames := make([]string, 0, len(r.Profiles))
	for n := range r.Profiles {
		pnames = append(pnames, n)
	}
	sort.Strings(pnames)
	for _, n := range pnames {
		items = append(items, tui.MultiSelectItem{Label: "@" + n + " — " + r.Profiles[n].Desc, Key: "@" + n})
	}
	// tools grouped by category
	byCat := map[string][]string{}
	for n, t := range r.Tools {
		byCat[t.Category] = append(byCat[t.Category], n)
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		items = append(items, tui.MultiSelectItem{Label: strings.ToUpper(c), Header: true})
		sort.Strings(byCat[c])
		for _, n := range byCat[c] {
			items = append(items, tui.MultiSelectItem{Label: n + " — " + r.Tools[n].Desc, Key: n})
		}
	}

	res, err := tea.NewProgram(tui.NewMultiSelect("pldev setup — pick tools/profiles", items)).Run()
	if err != nil {
		return nil, err
	}
	ms := res.(tui.MultiSelect)
	if ms.Aborted || len(ms.Chosen) == 0 {
		return nil, nil
	}
	return Resolve(r, strings.Join(ms.Chosen, "+"))
}
