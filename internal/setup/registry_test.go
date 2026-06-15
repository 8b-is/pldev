package setup

import "testing"

func TestBuiltinRegistryHasCoreToolsAndProfiles(t *testing.T) {
	r := BuiltinRegistry()
	for _, name := range []string{"ghostty", "zsh", "jq", "bun", "docker", "ripgrep"} {
		if _, ok := r.Tools[name]; !ok {
			t.Errorf("missing builtin tool %q", name)
		}
	}
	for _, p := range []string{"minimal", "dev", "server", "work"} {
		if _, ok := r.Profiles[p]; !ok {
			t.Errorf("missing builtin profile %q", p)
		}
	}
	// every profile references only known tools
	for pname, p := range r.Profiles {
		for _, tn := range p.Tools {
			if _, ok := r.Tools[tn]; !ok {
				t.Errorf("profile %q references unknown tool %q", pname, tn)
			}
		}
	}
	// every tool has at least one spec for at least one platform
	for tn, tool := range r.Tools {
		if len(tool.Darwin) == 0 && len(tool.Linux) == 0 {
			t.Errorf("tool %q has no specs", tn)
		}
	}
}
