package setup

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/tui"
)

type Options struct {
	Tokens string // "" => interactive
	DryRun bool
	Yes    bool
	List   bool
}

func loadRegistry() Registry {
	user, err := LoadUserManifest(DefaultManifestPath())
	if err != nil {
		fmt.Printf("warning: bad user manifest (%v) — using builtin only\n", err)
		user = Registry{Tools: map[string]Tool{}, Profiles: map[string]Profile{}}
	}
	return Merge(BuiltinRegistry(), user)
}

// Run executes the setup flow.
func Run(ctx context.Context, opts Options) error {
	reg := loadRegistry()

	if opts.List {
		printList(reg)
		return nil
	}

	var tools []Tool
	var err error
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
	printPlan(plan)

	if opts.DryRun {
		return nil
	}
	if len(plan.ToInstall) == 0 {
		fmt.Println("nothing to install.")
		return nil
	}
	if !opts.Yes && !tui.Confirm(fmt.Sprintf("Install %d package(s)?", len(plan.ToInstall))) {
		fmt.Println("aborted.")
		return nil
	}
	return apply(ctx, plan)
}

func apply(ctx context.Context, plan InstallPlan) error {
	var installed, failed int
	var fails []string
	for _, it := range plan.ToInstall {
		fmt.Printf("==> %s (%s)\n", it.Tool.Name, strings.Join(BuildCmd(it.Spec), " "))
		if _, err := Install(ctx, it.Spec); err != nil {
			failed++
			fails = append(fails, it.Tool.Name+": "+err.Error())
			fmt.Printf("    FAILED: %v\n", err)
			continue
		}
		installed++
		fmt.Println("    ok")
	}
	fmt.Printf("\nSummary: %d installed, %d skipped, %d unsupported, %d failed\n",
		installed, len(plan.Installed), len(plan.Unsupported), failed)
	if failed > 0 {
		return fmt.Errorf("failed: %s", strings.Join(fails, "; "))
	}
	return nil
}

func printPlan(p InstallPlan) {
	fmt.Println("== setup plan ==")
	for _, it := range p.ToInstall {
		sudo := ""
		if it.NeedsSudo {
			sudo = " (sudo)"
		}
		fmt.Printf("  install  %-12s via %s%s\n", it.Tool.Name, it.Spec.Backend, sudo)
	}
	for _, t := range p.Installed {
		fmt.Printf("  have     %-12s\n", t.Name)
	}
	for _, t := range p.Unsupported {
		fmt.Printf("  UNSUPPORTED %-9s (no backend on this OS)\n", t.Name)
	}
	fmt.Println()
}

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
