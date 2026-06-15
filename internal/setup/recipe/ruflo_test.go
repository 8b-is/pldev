package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// labels returns the ordered (action target) pairs of a recipe's steps.
func labels(r Recipe) []string {
	var out []string
	for _, s := range r.Steps {
		a, t := s.Label()
		out = append(out, a+" "+t)
	}
	return out
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func TestBuildRufloRecipeShapeAndOrder(t *testing.T) {
	r := BuildRufloRecipe(BuiltinRufloConfig(), nil, nil)
	if r.Name != "ruflo" {
		t.Fatalf("name = %q", r.Name)
	}
	ls := labels(r)

	// every memory/MCP/provision step is present
	for _, want := range []string{
		"install-tools node+uv",
		"provision honcho-stack",
		"ruflo-init wizard",
		"register-mcp ruflo",
		"fetch honcho-mcp.py",
		"register-mcp honcho",
		"install mempalace",
		"register-mcp mempalace",
	} {
		if indexOf(ls, want) < 0 {
			t.Errorf("missing step %q in %v", want, ls)
		}
	}

	// order: provision the stack before registering the honcho MCP that uses it;
	// fetch the shim before registering honcho.
	if indexOf(ls, "provision honcho-stack") > indexOf(ls, "register-mcp honcho") {
		t.Error("provision must precede honcho MCP registration")
	}
	if indexOf(ls, "fetch honcho-mcp.py") > indexOf(ls, "register-mcp honcho") {
		t.Error("shim fetch must precede honcho MCP registration")
	}

	// verify steps are non-mutating and run last
	last := r.Steps[len(r.Steps)-1]
	if last.Mutating() {
		t.Error("recipe should end on a non-mutating verify step")
	}
}

func TestRufloRecipeWiresLanesAndEnv(t *testing.T) {
	r := BuildRufloRecipe(BuiltinRufloConfig(), nil, nil)

	var cliCore, langfuseSecret, laneDirective bool
	for _, s := range r.Steps {
		if ap, ok := s.(AppendRC); ok {
			switch {
			case ap.Line == "export CLI_CORE=1":
				cliCore = true
			case strings.Contains(ap.Line, "LANGFUSE_SECRET_KEY") && strings.Contains(ap.Line, "$(bao"):
				langfuseSecret = true // secret via shell indirection, not a literal value
			case strings.Contains(ap.Line, "Memory lanes") && strings.HasSuffix(ap.Shellrc, "CLAUDE.md"):
				laneDirective = true
			}
		}
	}
	if !cliCore {
		t.Error("missing CLI_CORE=1 rc export")
	}
	if !langfuseSecret {
		t.Error("langfuse secret must be a $(bao ...) shell-indirection, never a literal")
	}
	if !laneDirective {
		t.Error("missing memory-lane directive in CLAUDE.md")
	}

	// honcho MCP carries its env as -e flags
	for _, s := range r.Steps {
		if m, ok := s.(RegisterMCP); ok && m.MCPName == "honcho" {
			argv := strings.Join(m.addArgs(), " ")
			if !strings.Contains(argv, "-e HONCHO_BASE_URL=") {
				t.Errorf("honcho MCP missing env: %s", argv)
			}
		}
	}
}

func TestLoadRufloConfigDefaultsWhenAbsent(t *testing.T) {
	cfg, err := LoadRufloConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Honcho.BaseURL == "" || len(cfg.Plugins) != 3 || cfg.MemBackend != "rvf" {
		t.Errorf("absent file should yield builtin defaults, got %+v", cfg)
	}
}

func TestLoadRufloConfigOverlay(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "recipes.toml")
	os.WriteFile(p, []byte(`
[ruflo]
shellrc = "/custom/.zshrc"
[ruflo.langfuse]
host = "https://lf.example"
`), 0o644)
	cfg, err := LoadRufloConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shellrc != "/custom/.zshrc" {
		t.Errorf("shellrc not overridden: %q", cfg.Shellrc)
	}
	if cfg.Langfuse.Host != "https://lf.example" {
		t.Errorf("langfuse host not overridden: %q", cfg.Langfuse.Host)
	}
	// untouched fields keep their defaults
	if len(cfg.Plugins) != 3 || cfg.Honcho.UserPeer != "lodripeter" {
		t.Errorf("defaults not preserved under overlay: %+v", cfg)
	}
}

func TestNamedRuflo(t *testing.T) {
	if _, ok := Named("ruflo"); !ok {
		t.Error("ruflo recipe should be registered")
	}
	if _, ok := Named("nope"); ok {
		t.Error("unknown recipe must return false")
	}
}
