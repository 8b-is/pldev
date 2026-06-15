package recipe

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/setup"
	"github.com/peterlodri-sec/pldev/internal/sys"
)

// --- InstallTools: install a set of tools/profiles via the setup engine ---

type InstallTools struct {
	Tokens string // "+"-joined tool/profile tokens, e.g. "node+@dev"
}

func (s InstallTools) Label() (string, string) { return "install-tools", s.Tokens }
func (s InstallTools) Mutating() bool           { return true }

func (s InstallTools) plan(c Context) (setup.InstallPlan, error) {
	reg := setup.BuiltinRegistry()
	if user, err := setup.LoadUserManifest(setup.DefaultManifestPath()); err == nil {
		reg = setup.Merge(reg, user)
	}
	tools, err := setup.Resolve(reg, s.Tokens)
	if err != nil {
		return setup.InstallPlan{}, err
	}
	return setup.BuildPlan(c.Ctx, reg, tools, setup.NewDetector(), setup.AvailableBackends())
}

func (s InstallTools) Check(c Context) (Status, error) {
	plan, err := s.plan(c)
	if err != nil {
		return StatusPending, err
	}
	if len(plan.ToInstall) == 0 {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

func (s InstallTools) Apply(c Context) error {
	plan, err := s.plan(c)
	if err != nil {
		return err
	}
	for _, it := range plan.ToInstall {
		if _, err := setup.Install(c.Ctx, it.Spec); err != nil {
			return fmt.Errorf("%s: %w", it.Tool.Name, err)
		}
		c.Rep.OK("install", it.Tool.Name, report.KV{K: "via", V: string(it.Spec.Backend)})
	}
	return nil
}

// --- WriteConfig: write a file, backing up an existing differing one ---

type WriteConfig struct {
	Path    string
	Content string
	Backup  bool
}

func (s WriteConfig) Label() (string, string) { return "write-config", s.Path }
func (s WriteConfig) Mutating() bool           { return true }

func (s WriteConfig) Check(c Context) (Status, error) {
	b, err := os.ReadFile(s.Path)
	if err == nil && string(b) == s.Content {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

func (s WriteConfig) Apply(c Context) error {
	if s.Backup {
		if b, err := os.ReadFile(s.Path); err == nil && string(b) != s.Content {
			bak := s.Path + ".bak-" + time.Now().Format("20060102-150405")
			if err := os.WriteFile(bak, b, 0o644); err != nil {
				return fmt.Errorf("backup: %w", err)
			}
		}
	}
	return os.WriteFile(s.Path, []byte(s.Content), 0o644)
}

// --- AppendRC: idempotently append a line to a shell rc file ---

type AppendRC struct {
	Shellrc string
	Line    string
}

func (s AppendRC) Label() (string, string) { return "append-rc", s.Shellrc }
func (s AppendRC) Mutating() bool           { return true }

func (s AppendRC) Check(c Context) (Status, error) {
	b, err := os.ReadFile(s.Shellrc)
	if err == nil && linevalue(string(b), s.Line) {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

func (s AppendRC) Apply(c Context) error {
	f, err := os.OpenFile(s.Shellrc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, s.Line)
	return err
}

func linevalue(content, line string) bool {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == strings.TrimSpace(line) {
			return true
		}
	}
	return false
}

// --- RunCmd: run a command; optional Probe makes it idempotent ---

type RunCmd struct {
	ActionName  string
	TargetName  string
	Name        string
	Args        []string
	Interactive bool
	Probe       func(c Context) bool // nil => always pending (always runs)
}

func (s RunCmd) Label() (string, string) { return s.ActionName, s.TargetName }
func (s RunCmd) Mutating() bool           { return true }

func (s RunCmd) Check(c Context) (Status, error) {
	if s.Probe != nil && s.Probe(c) {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

func (s RunCmd) Apply(c Context) error {
	if s.Interactive {
		return sys.RunInteractive(c.Ctx, s.Name, s.Args...)
	}
	_, err := sys.Run(c.Ctx, s.Name, s.Args...)
	return err
}

// --- RegisterMCP: register an MCP server with `claude mcp add` if absent ---

type RegisterMCP struct {
	MCPName   string
	Transport string   // "" or "stdio" => stdio; "http"/"sse" => --transport
	Target    string   // stdio: the command (+args); http/sse: the URL
	Scope     string   // e.g. "user" (global) or "project"
	Env       []string // optional "KEY=VALUE" pairs passed as `-e KEY=VALUE`
}

func (s RegisterMCP) Label() (string, string) { return "register-mcp", s.MCPName }
func (s RegisterMCP) Mutating() bool           { return true }

func (s RegisterMCP) Check(c Context) (Status, error) {
	out, err := sys.Run(c.Ctx, "claude", "mcp", "list")
	if err != nil {
		return StatusPending, nil // can't confirm => treat as pending
	}
	if strings.Contains(out, s.MCPName) {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

// addArgs builds the `claude mcp add ...` argv for this registration.
func (s RegisterMCP) addArgs() []string {
	scope := s.Scope
	if scope == "" {
		scope = "user"
	}
	args := []string{"mcp", "add"}
	if s.Transport == "http" || s.Transport == "sse" {
		args = append(args, "--transport", s.Transport)
	}
	args = append(args, s.MCPName, "-s", scope)
	for _, e := range s.Env {
		args = append(args, "-e", e)
	}
	switch s.Transport {
	case "http", "sse":
		return append(args, s.Target)
	default: // stdio
		return append(append(args, "--"), strings.Fields(s.Target)...)
	}
}

func (s RegisterMCP) Apply(c Context) error {
	return sys.RunInteractive(c.Ctx, "claude", s.addArgs()...)
}

// --- Verify: a read-only post-check; never mutates ---

type Verify struct {
	ActionName string
	TargetName string
	Probe      func(c Context) error
}

func (s Verify) Label() (string, string) { return s.ActionName, s.TargetName }
func (s Verify) Mutating() bool           { return false }

func (s Verify) Check(c Context) (Status, error) {
	if s.Probe == nil {
		return StatusSatisfied, nil
	}
	if err := s.Probe(c); err != nil {
		return StatusPending, err
	}
	return StatusSatisfied, nil
}

func (s Verify) Apply(c Context) error { return nil }
