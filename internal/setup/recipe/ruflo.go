package recipe

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// expandHome resolves a leading ~/ against the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// httpOK reports whether url answers without a server error (health probe).
func httpOK(url string) bool {
	c := http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func onPath(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// BuildRufloRecipe assembles the ruflo install + dual-memory recipe from config.
// Honcho (relational/peer memory) and MemPalace (file/corpus recall) are both
// registered as global MCPs — every spawned agent inherits both — and a lane
// directive in CLAUDE.md tells agents which plane to query. Provision triggers a
// nixos-rebuild of the honcho stack on M1 (auth-gated); pass nil auth/remote to
// skip live provisioning. The ruflo daemon is installed but not started.
func BuildRufloRecipe(cfg RufloConfig, auth Authenticator, remote RemoteRunner) Recipe {
	rc := expandHome(cfg.Shellrc)
	shim := expandHome(cfg.Honcho.ShimDest)
	claudeMD := expandHome("~/.claude/CLAUDE.md")
	p := cfg.Provision

	steps := []Step{
		// runtimes: node for ruflo (npx), uv for honcho shim + mempalace
		InstallTools{Tokens: "node+uv"},

		// honcho stack lives on dev-cx53; provision via nixos-rebuild from M1
		Provision{
			Name: "honcho-stack",
			Host: p.SSHHost,
			Argv: []string{"sh", "-c", "cd " + p.RepoDir +
				" && nixos-rebuild switch --flake .#" + p.Target +
				" --build-host " + p.TargetHost + " --target-host " + p.TargetHost + " --sudo"},
			HealthProbe: func(Context) bool { return httpOK(p.HealthURL) },
			Auth:        auth,
			Remote:      remote,
		},

		// ruflo: init (hooks/agents/.claude) — daemon NOT started
		RunCmd{ActionName: "ruflo-init", TargetName: "wizard",
			Name: "npx", Args: []string{"ruflo@latest", "init", "wizard"}, Interactive: true},
		RegisterMCP{MCPName: "ruflo", Target: "npx ruflo@latest mcp start", Scope: "user"},

		// honcho MCP: fetch the stdio shim, register it pointed at remote honcho
		RunCmd{ActionName: "fetch", TargetName: "honcho-mcp.py",
			Name: "sh", Args: []string{"-c", "mkdir -p " + filepath.Dir(shim) + " && scp " + cfg.Honcho.ShimSrc + " " + shim},
			Probe: func(Context) bool { _, err := os.Stat(shim); return err == nil }},
		RegisterMCP{MCPName: "honcho", Target: "uv run --script " + shim, Scope: "user",
			Env: []string{
				"HONCHO_BASE_URL=" + cfg.Honcho.BaseURL,
				"HONCHO_USER_PEER=" + cfg.Honcho.UserPeer,
				"HONCHO_AGENT_PEER=" + cfg.Honcho.AgentPeer,
			}},

		// mempalace MCP: local install, pgvector backend, register stdio
		RunCmd{ActionName: "install", TargetName: "mempalace",
			Name: "sh", Args: []string{"-c", cfg.Mempalace.Install},
			Probe: func(Context) bool { return onPath("mempalace") }},
		RegisterMCP{MCPName: "mempalace", Target: "mempalace-mcp", Scope: "user",
			Env: []string{"MEMPALACE_BACKEND=" + cfg.Mempalace.Backend}},

		// ruflo memory plugins
		RunCmd{ActionName: "ruflo-plugins", TargetName: strings.Join(cfg.Plugins, ","),
			Name: "sh", Args: []string{"-c", "npx ruflo@latest plugins add " + strings.Join(cfg.Plugins, " ")}},
	}

	// shell rc: CLI_CORE + memory/trace env (secret values via shell indirection)
	if cfg.CLICore {
		steps = append(steps, AppendRC{Shellrc: rc, Line: "export CLI_CORE=1"})
	}
	steps = append(steps,
		AppendRC{Shellrc: rc, Line: "export MEMPALACE_BACKEND=" + cfg.Mempalace.Backend},
		AppendRC{Shellrc: rc, Line: "export MEMPALACE_PGVECTOR_DSN=" + cfg.Mempalace.DSNRef},
		AppendRC{Shellrc: rc, Line: "export HONCHO_BASE_URL=" + cfg.Honcho.BaseURL},
		AppendRC{Shellrc: rc, Line: "export LANGFUSE_HOST=" + cfg.Langfuse.Host},
		AppendRC{Shellrc: rc, Line: "export LANGFUSE_PUBLIC_KEY=" + cfg.Langfuse.PublicKeyRef},
		AppendRC{Shellrc: rc, Line: "export LANGFUSE_SECRET_KEY=" + cfg.Langfuse.SecretKeyRef},

		// memory lanes: tell every agent which plane to query (standing context)
		AppendRC{Shellrc: claudeMD, Line: "Memory lanes: honcho MCP = peer/prefs/dialogue state across sessions; mempalace MCP = file/code/transcript recall. Query the plane that matches the need."},

		// verify (non-mutating)
		Verify{ActionName: "verify", TargetName: "mcp-registered", Probe: func(c Context) error {
			return mcpListHas(c, "ruflo", "honcho", "mempalace")
		}},
		Verify{ActionName: "verify", TargetName: "honcho-health", Probe: func(Context) error {
			if !httpOK(p.HealthURL) {
				return &probeError{"honcho health unreachable: " + p.HealthURL}
			}
			return nil
		}},
	)

	return Recipe{Name: "ruflo", Steps: steps}
}

type probeError struct{ msg string }

func (e *probeError) Error() string { return e.msg }

func runClaudeMCPList(c Context) (string, error) {
	return sys.Run(c.Ctx, "claude", "mcp", "list")
}

// mcpListHas verifies `claude mcp list` contains every named server.
func mcpListHas(c Context, names ...string) error {
	out, err := runClaudeMCPList(c)
	if err != nil {
		return err
	}
	var missing []string
	for _, n := range names {
		if !strings.Contains(out, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return &probeError{"missing MCP server(s): " + strings.Join(missing, ", ")}
	}
	return nil
}
