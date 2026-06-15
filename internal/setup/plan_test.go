package setup

import (
	"context"
	"testing"
)

// fakeDetector: tools whose chosen spec name is in installed=true.
type fakeDetector struct{ installed map[string]bool }

func (f fakeDetector) Installed(_ context.Context, s PkgSpec) (bool, error) {
	return f.installed[s.Name], nil
}

func TestBuildPlanClassifies(t *testing.T) {
	r := BuiltinRegistry()
	// availability: brew + mise available, NOT apt (simulate macOS-ish)
	avail := map[Backend]bool{BrewFormula: true, BrewCask: true, Mise: true, Script: true}
	det := fakeDetector{installed: map[string]bool{"git": true}}

	plan, err := BuildPlan(context.Background(), r, []Tool{
		r.Tools["git"], // installed -> skip
		r.Tools["jq"],  // brew -> install
		r.Tools["bun"], // mise -> install
	}, det, avail)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Installed) != 1 || plan.Installed[0].Name != "git" {
		t.Errorf("git should be Installed: %+v", plan.Installed)
	}
	gotInstall := map[string]bool{}
	for _, it := range plan.ToInstall {
		gotInstall[it.Tool.Name] = true
	}
	if !gotInstall["jq"] || !gotInstall["bun"] {
		t.Errorf("jq+bun should be ToInstall: %+v", plan.ToInstall)
	}
}

func TestBuildPlanUnsupported(t *testing.T) {
	r := BuiltinRegistry()
	avail := map[Backend]bool{} // nothing available
	det := fakeDetector{installed: map[string]bool{}}
	plan, _ := BuildPlan(context.Background(), r, []Tool{r.Tools["jq"]}, det, avail)
	if len(plan.Unsupported) != 1 {
		t.Errorf("jq should be Unsupported when no backend available: %+v", plan)
	}
}
