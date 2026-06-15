package setup

import (
	"context"
	"runtime"
)

type Item struct {
	Tool      Tool
	Spec      PkgSpec
	NeedsSudo bool
}

type InstallPlan struct {
	ToInstall   []Item
	Installed   []Tool
	Unsupported []Tool
}

func needsSudo(b Backend) bool {
	switch b {
	case Apt, Dnf, Pacman:
		return true
	default:
		return false
	}
}

// specsFor returns the platform-appropriate candidate specs for a tool.
func specsFor(t Tool) []PkgSpec {
	if runtime.GOOS == "darwin" {
		return t.Darwin
	}
	return t.Linux
}

// BuildPlan selects the first available backend per tool, detects installed state,
// and classifies each tool. `avail` marks which backends exist on this machine.
func BuildPlan(ctx context.Context, r Registry, tools []Tool, det Detector, avail map[Backend]bool) (InstallPlan, error) {
	var plan InstallPlan
	for _, t := range tools {
		var chosen *PkgSpec
		for i := range specsFor(t) {
			s := specsFor(t)[i]
			if avail[s.Backend] {
				chosen = &s
				break
			}
		}
		if chosen == nil {
			plan.Unsupported = append(plan.Unsupported, t)
			continue
		}
		ok, err := det.Installed(ctx, *chosen)
		if err != nil {
			return plan, err
		}
		if ok {
			plan.Installed = append(plan.Installed, t)
			continue
		}
		plan.ToInstall = append(plan.ToInstall, Item{Tool: t, Spec: *chosen, NeedsSudo: needsSudo(chosen.Backend)})
	}
	return plan, nil
}
