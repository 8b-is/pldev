package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildArpRecipeShape(t *testing.T) {
	r := BuildArpRecipe(BuiltinArpConfig())
	if r.Name != "arp" {
		t.Fatalf("name = %q", r.Name)
	}
	ls := labels(r)
	for _, want := range []string{"install-tools git+uv", "clone arp-corpus", "mempalace-mine arp-corpus"} {
		if indexOf(ls, want) < 0 {
			t.Errorf("missing step %q in %v", want, ls)
		}
	}
	// ends on a non-mutating verify
	if r.Steps[len(r.Steps)-1].Mutating() {
		t.Error("recipe should end on a non-mutating verify")
	}
	// ARP_LOG_PATH export present, pointed at the clone's corpus dir
	var hasLogPath bool
	for _, s := range r.Steps {
		if ap, ok := s.(AppendRC); ok && strings.HasPrefix(ap.Line, "export ARP_LOG_PATH=") && strings.HasSuffix(ap.Line, "/corpus") {
			hasLogPath = true
		}
	}
	if !hasLogPath {
		t.Error("missing ARP_LOG_PATH export pointed at <clone>/corpus")
	}
}

func TestArpHookInstallStepConditional(t *testing.T) {
	noHook := labels(BuildArpRecipe(BuiltinArpConfig())) // HookInstall == ""
	if indexOf(noHook, "install arp-hook") >= 0 {
		t.Error("no hook-install step when HookInstall is empty")
	}
	cfg := BuiltinArpConfig()
	cfg.HookInstall = "bash install-hook.sh"
	withHook := labels(BuildArpRecipe(cfg))
	if indexOf(withHook, "install arp-hook") < 0 {
		t.Error("hook-install step expected when HookInstall is set")
	}
}

func TestLoadArpConfigOverlay(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "recipes.toml")
	os.WriteFile(p, []byte("[arp]\nremote = \"git@example:me/x.git\"\n"), 0o644)
	cfg, err := LoadArpConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Remote != "git@example:me/x.git" {
		t.Errorf("remote not overridden: %q", cfg.Remote)
	}
	if cfg.ClonePath == "" { // default preserved
		t.Error("clone_path default lost under overlay")
	}
}
