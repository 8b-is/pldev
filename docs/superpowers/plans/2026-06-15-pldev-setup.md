# pldev `setup` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `pldev setup` — declarative, idempotent, confirm-first installer of dev tooling from a curated built-in registry (+ user overrides), via brew/apt/dnf/pacman/mise, with named profiles.

**Architecture:** A merged `Registry` (embedded Go data + user TOML) resolves tool names/profiles to `[]Tool`. A per-OS backend selector + `Detector` produce an `InstallPlan` (install/skip/unsupported). An `Installer` runs commands (sudo-prefixed for system managers; sudo prompts in the TTY, never captured). Mirrors pldev's existing interface+build-tag patterns.

**Tech Stack:** Go 1.26, `github.com/BurntSushi/toml`, existing `internal/sys` + `internal/tui` (Bubble Tea/Lipgloss).

Working dir: `~/workspace/pldev` (git, remote `origin`). Spec: `docs/superpowers/specs/2026-06-15-pldev-setup-design.md`. Commit author: `peter.lodri <peter.lodri@instructure.com>`.

---

## File Structure

```
internal/setup/registry.go        types (Backend/PkgSpec/Tool/Profile/Registry) + BuiltinRegistry()
internal/setup/registry_test.go
internal/setup/manifest.go         LoadUserManifest + Merge
internal/setup/manifest_test.go
internal/setup/resolve.go          Resolve(reg, tokens) ([]Tool, error)  PURE
internal/setup/resolve_test.go
internal/setup/detect.go           Detector iface + parsers (brew/dpkg/mise) PURE
internal/setup/detect_test.go
internal/setup/testdata/           brew-list.txt, mise-ls.txt
internal/setup/plan.go             BuildPlan(reg, names, det, os) InstallPlan
internal/setup/plan_test.go
internal/setup/installer.go        Installer iface, BackendAvailable, BuildCmd  (BuildCmd PURE)
internal/setup/installer_test.go
internal/setup/installer_darwin.go //go:build darwin  (brew availability/detect)
internal/setup/installer_linux.go  //go:build linux   (apt/dnf/pacman availability/detect)
internal/setup/setup.go            Run(ctx, opts) orchestrator
internal/tui/multiselect.go        MultiSelect model
main.go                            wire `pldev setup` + flags + root menu entry (MODIFY)
SKILL.md / README.md               docs (MODIFY)
```

---

## Task 0: Add TOML dep

**Files:** Modify `go.mod`

- [ ] **Step 1: Add dependency**

Run:
```bash
cd ~/workspace/pldev && go get github.com/BurntSushi/toml@latest
```
Expected: `go.mod` gains `github.com/BurntSushi/toml`.

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "chore: add BurntSushi/toml for setup manifest"
```

---

## Task 1: Registry types + built-in registry

**Files:** Create `internal/setup/registry.go`, `internal/setup/registry_test.go`

- [ ] **Step 1: Write failing test**

```go
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
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/setup/` — Expected: FAIL (undefined BuiltinRegistry).

- [ ] **Step 3: Implement `registry.go`**

```go
package setup

type Backend string

const (
	BrewFormula Backend = "brew"
	BrewCask    Backend = "brew-cask"
	Apt         Backend = "apt"
	Dnf         Backend = "dnf"
	Pacman      Backend = "pacman"
	Mise        Backend = "mise"
	Script      Backend = "script"
)

// PkgSpec is one install method. For Script, Name is the shell command.
type PkgSpec struct {
	Backend     Backend  `toml:"backend"`
	Name        string   `toml:"name"`
	Tap         string   `toml:"tap,omitempty"`
	PostInstall []string `toml:"post_install,omitempty"`
}

// Tool: Darwin/Linux are ordered candidates; first available backend wins.
type Tool struct {
	Name     string    `toml:"-"`
	Desc     string    `toml:"desc"`
	Category string    `toml:"category"`
	Darwin   []PkgSpec `toml:"darwin"`
	Linux    []PkgSpec `toml:"linux"`
}

type Profile struct {
	Name  string   `toml:"-"`
	Desc  string   `toml:"desc"`
	Tools []string `toml:"tools"`
}

type Registry struct {
	Tools    map[string]Tool
	Profiles map[string]Profile
}

func d(specs ...PkgSpec) []PkgSpec { return specs }

func brew(name string) PkgSpec   { return PkgSpec{Backend: BrewFormula, Name: name} }
func cask(name string) PkgSpec   { return PkgSpec{Backend: BrewCask, Name: name} }
func apt(name string) PkgSpec    { return PkgSpec{Backend: Apt, Name: name} }
func mise(name string) PkgSpec   { return PkgSpec{Backend: Mise, Name: name} }

// BuiltinRegistry returns the curated, embedded registry.
func BuiltinRegistry() Registry {
	tools := map[string]Tool{
		"ghostty":  {Desc: "GPU terminal", Category: "terminal", Darwin: d(cask("ghostty")), Linux: d(apt("ghostty"))},
		"zsh":      {Desc: "Z shell", Category: "shell", Darwin: d(brew("zsh")), Linux: d(apt("zsh"))},
		"fish":     {Desc: "fish shell", Category: "shell", Darwin: d(brew("fish")), Linux: d(apt("fish"))},
		"jq":       {Desc: "JSON processor", Category: "cli", Darwin: d(brew("jq")), Linux: d(apt("jq"))},
		"ripgrep":  {Desc: "fast grep", Category: "cli", Darwin: d(brew("ripgrep")), Linux: d(apt("ripgrep"))},
		"fd":       {Desc: "fast find", Category: "cli", Darwin: d(brew("fd")), Linux: d(apt("fd-find"))},
		"bat":      {Desc: "cat w/ wings", Category: "cli", Darwin: d(brew("bat")), Linux: d(apt("bat"))},
		"eza":      {Desc: "modern ls", Category: "cli", Darwin: d(brew("eza")), Linux: d(apt("eza"))},
		"fzf":      {Desc: "fuzzy finder", Category: "cli", Darwin: d(brew("fzf")), Linux: d(apt("fzf"))},
		"tmux":     {Desc: "terminal multiplexer", Category: "cli", Darwin: d(brew("tmux")), Linux: d(apt("tmux"))},
		"neovim":   {Desc: "editor", Category: "editor", Darwin: d(brew("neovim")), Linux: d(apt("neovim"))},
		"starship": {Desc: "prompt", Category: "shell", Darwin: d(brew("starship")), Linux: d(brew("starship"))},
		"gh":       {Desc: "GitHub CLI", Category: "cli", Darwin: d(brew("gh")), Linux: d(apt("gh"))},
		"git":      {Desc: "git", Category: "cli", Darwin: d(brew("git")), Linux: d(apt("git"))},
		"tailscale": {Desc: "mesh VPN", Category: "net", Darwin: d(cask("tailscale")), Linux: d(apt("tailscale"))},
		"docker":   {Desc: "containers", Category: "containers", Darwin: d(cask("docker")), Linux: d(apt("docker.io"))},
		"bun":      {Desc: "JS runtime", Category: "cli", Darwin: d(mise("bun")), Linux: d(mise("bun"))},
		"node":     {Desc: "Node.js", Category: "cli", Darwin: d(mise("node")), Linux: d(mise("node"))},
		"go":       {Desc: "Go toolchain", Category: "cli", Darwin: d(mise("go")), Linux: d(mise("go"))},
		"mold":     {Desc: "fast linker", Category: "build", Darwin: d(brew("mold")), Linux: d(apt("mold"))},
		"mimalloc": {Desc: "allocator", Category: "build", Darwin: d(brew("mimalloc")), Linux: d(apt("libmimalloc-dev"))},
		"wild":     {Desc: "wild linker", Category: "build", Darwin: d(PkgSpec{Backend: Script, Name: "cargo install wild-linker"}), Linux: d(PkgSpec{Backend: Script, Name: "cargo install wild-linker"})},
	}
	for name, t := range tools { // backfill Name
		t.Name = name
		tools[name] = t
	}
	profiles := map[string]Profile{
		"minimal": {Desc: "bare essentials", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim"}},
		"dev":     {Desc: "full dev box", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "ghostty", "jq", "fd", "bat", "eza", "bun", "node", "gh", "docker", "tmux", "starship"}},
		"server":  {Desc: "headless server", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "tmux", "docker", "tailscale"}},
		"work":    {Desc: "work machine", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "ghostty", "jq", "fd", "bat", "eza", "bun", "node", "gh", "docker", "tmux", "starship", "tailscale"}},
	}
	for name, p := range profiles {
		p.Name = name
		profiles[name] = p
	}
	return Registry{Tools: tools, Profiles: profiles}
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/setup/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/registry.go internal/setup/registry_test.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): registry types + curated builtin registry"
```

---

## Task 2: User manifest load + merge

**Files:** Create `internal/setup/manifest.go`, `internal/setup/manifest_test.go`

- [ ] **Step 1: Write failing test**

```go
package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOverridesAndAdds(t *testing.T) {
	base := BuiltinRegistry()
	toml := `
[tools.ghostty]
desc = "custom"
category = "terminal"
darwin = [{ backend = "brew-cask", name = "ghostty-tip" }]

[tools.mytool]
category = "cli"
darwin = [{ backend = "brew", name = "mytool" }]

[profiles.dev]
desc = "slim dev"
tools = ["ghostty", "mytool"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "setup.toml")
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	user, err := LoadUserManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := Merge(base, user)

	if got := merged.Tools["ghostty"].Darwin[0].Name; got != "ghostty-tip" {
		t.Errorf("override failed: ghostty darwin name = %q", got)
	}
	if merged.Tools["ghostty"].Name != "ghostty" {
		t.Error("Name backfill lost on override")
	}
	if _, ok := merged.Tools["mytool"]; !ok {
		t.Error("new tool not added")
	}
	if got := merged.Profiles["dev"].Tools; len(got) != 2 {
		t.Errorf("profile override failed: %v", got)
	}
	// untouched builtin remains
	if _, ok := merged.Tools["jq"]; !ok {
		t.Error("builtin jq lost after merge")
	}
}

func TestLoadMissingFileIsEmptyNoError(t *testing.T) {
	user, err := LoadUserManifest(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(user.Tools) != 0 || len(user.Profiles) != 0 {
		t.Error("missing file should yield empty registry")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/setup/ -run Merge` — Expected: FAIL (undefined LoadUserManifest/Merge).

- [ ] **Step 3: Implement `manifest.go`**

```go
package setup

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// userManifest mirrors the TOML shape.
type userManifest struct {
	Tools    map[string]Tool    `toml:"tools"`
	Profiles map[string]Profile `toml:"profiles"`
}

// LoadUserManifest reads ~/.config/pldev/setup.toml (or given path). Missing file
// returns an empty Registry and no error. Malformed file returns an error.
func LoadUserManifest(path string) (Registry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Registry{Tools: map[string]Tool{}, Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var m userManifest
	if err := toml.Unmarshal(b, &m); err != nil {
		return Registry{}, err
	}
	if m.Tools == nil {
		m.Tools = map[string]Tool{}
	}
	if m.Profiles == nil {
		m.Profiles = map[string]Profile{}
	}
	return Registry{Tools: m.Tools, Profiles: m.Profiles}, nil
}

// DefaultManifestPath returns ~/.config/pldev/setup.toml.
func DefaultManifestPath() string {
	return os.Getenv("HOME") + "/.config/pldev/setup.toml"
}

// Merge overlays user onto base: same-key entries fully replace; new keys add.
// Name fields are (re)backfilled from the map key.
func Merge(base, user Registry) Registry {
	out := Registry{
		Tools:    map[string]Tool{},
		Profiles: map[string]Profile{},
	}
	for k, v := range base.Tools {
		out.Tools[k] = v
	}
	for k, v := range base.Profiles {
		out.Profiles[k] = v
	}
	for k, v := range user.Tools {
		v.Name = k
		out.Tools[k] = v
	}
	for k, v := range user.Profiles {
		v.Name = k
		out.Profiles[k] = v
	}
	return out
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/setup/ -run "Merge|Load"` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/manifest.go internal/setup/manifest_test.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): user manifest load + merge"
```

---

## Task 3: Resolve tokens/profiles → []Tool

**Files:** Create `internal/setup/resolve.go`, `internal/setup/resolve_test.go`

- [ ] **Step 1: Write failing test**

```go
package setup

import "testing"

func TestResolveNamesProfilesDedup(t *testing.T) {
	r := BuiltinRegistry()
	tools, err := Resolve(r, "@minimal+jq+git")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		if names[tl.Name] {
			t.Errorf("duplicate %q in result", tl.Name)
		}
		names[tl.Name] = true
	}
	// minimal includes git already; +git must not duplicate
	if !names["git"] || !names["jq"] || !names["zsh"] {
		t.Errorf("missing expected tools: %v", names)
	}
}

func TestResolveUnknownErrors(t *testing.T) {
	r := BuiltinRegistry()
	_, err := Resolve(r, "jq+nonsuch+@nope")
	if err == nil {
		t.Fatal("expected error for unknown tokens")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/setup/ -run Resolve` — Expected: FAIL.

- [ ] **Step 3: Implement `resolve.go`**

```go
package setup

import (
	"fmt"
	"sort"
	"strings"
)

// Resolve parses a "+"-joined token string. Tokens prefixed "@" are profiles;
// others are tool names. Profiles expand to their tool lists. Result is deduped,
// order-preserving. Unknown tokens produce an error listing them.
func Resolve(r Registry, input string) ([]Tool, error) {
	var unknown []string
	var order []string
	seen := map[string]bool{}

	add := func(name string) {
		if _, ok := r.Tools[name]; !ok {
			unknown = append(unknown, name)
			return
		}
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}

	for _, tok := range strings.Split(input, "+") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "@") {
			pname := strings.TrimPrefix(tok, "@")
			p, ok := r.Profiles[pname]
			if !ok {
				unknown = append(unknown, "@"+pname)
				continue
			}
			for _, tn := range p.Tools {
				add(tn)
			}
			continue
		}
		add(tok)
	}

	if len(unknown) > 0 {
		valid := make([]string, 0, len(r.Tools))
		for n := range r.Tools {
			valid = append(valid, n)
		}
		sort.Strings(valid)
		return nil, fmt.Errorf("unknown: %s\nvalid tools: %s", strings.Join(unknown, ", "), strings.Join(valid, ", "))
	}

	out := make([]Tool, 0, len(order))
	for _, n := range order {
		out = append(out, r.Tools[n])
	}
	return out, nil
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/setup/ -run Resolve` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/resolve.go internal/setup/resolve_test.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): token/profile resolution with dedup"
```

---

## Task 4: Detection parsers + Detector interface

**Files:** Create `internal/setup/detect.go`, `internal/setup/detect_test.go`, `internal/setup/testdata/brew-list.txt`, `internal/setup/testdata/mise-ls.txt`

- [ ] **Step 1: Write fixtures**

`testdata/brew-list.txt`:
```
jq 1.7.1
ripgrep 14.1.0
git 2.45.0
```
`testdata/mise-ls.txt`:
```
node    20.11.0  ~/.config/mise/config.toml
bun     1.1.0
```

- [ ] **Step 2: Write failing test**

```go
package setup

import (
	"os"
	"testing"
)

func TestBrewListHas(t *testing.T) {
	b, _ := os.ReadFile("testdata/brew-list.txt")
	if !brewListHas(string(b), "jq") {
		t.Error("jq should be present")
	}
	if brewListHas(string(b), "tmux") {
		t.Error("tmux should be absent")
	}
}

func TestMiseLsHas(t *testing.T) {
	b, _ := os.ReadFile("testdata/mise-ls.txt")
	if !miseLsHas(string(b), "bun") {
		t.Error("bun should be present")
	}
	if miseLsHas(string(b), "go") {
		t.Error("go should be absent")
	}
}
```

- [ ] **Step 3: Run, expect FAIL**

Run: `go test ./internal/setup/ -run "BrewList|MiseLs"` — Expected: FAIL.

- [ ] **Step 4: Implement `detect.go`**

```go
package setup

import (
	"context"
	"strings"
)

// Detector reports whether the package described by a spec is already installed.
type Detector interface {
	Installed(ctx context.Context, spec PkgSpec) (bool, error)
}

// brewListHas reports whether `brew list --versions` output contains name as the
// first field of any line.
func brewListHas(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == name {
			return true
		}
	}
	return false
}

// miseLsHas reports whether `mise ls` output lists the tool (first field).
func miseLsHas(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == name {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run, expect PASS**

Run: `go test ./internal/setup/ -run "BrewList|MiseLs"` — Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/setup/detect.go internal/setup/detect_test.go internal/setup/testdata/
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): Detector interface + brew/mise list parsers"
```

---

## Task 5: BuildPlan

**Files:** Create `internal/setup/plan.go`, `internal/setup/plan_test.go`

- [ ] **Step 1: Write failing test**

```go
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
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/setup/ -run BuildPlan` — Expected: FAIL.

- [ ] **Step 3: Implement `plan.go`**

```go
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
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/setup/ -run BuildPlan` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/plan.go internal/setup/plan_test.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): BuildPlan backend-select + classify"
```

---

## Task 6: Installer command construction

**Files:** Create `internal/setup/installer.go`, `internal/setup/installer_test.go`

- [ ] **Step 1: Write failing test**

```go
package setup

import (
	"reflect"
	"testing"
)

func TestBuildCmd(t *testing.T) {
	cases := []struct {
		spec PkgSpec
		want []string
	}{
		{PkgSpec{Backend: BrewFormula, Name: "jq"}, []string{"brew", "install", "jq"}},
		{PkgSpec{Backend: BrewCask, Name: "ghostty"}, []string{"brew", "install", "--cask", "ghostty"}},
		{PkgSpec{Backend: Apt, Name: "zsh"}, []string{"sudo", "apt-get", "install", "-y", "zsh"}},
		{PkgSpec{Backend: Dnf, Name: "zsh"}, []string{"sudo", "dnf", "install", "-y", "zsh"}},
		{PkgSpec{Backend: Pacman, Name: "zsh"}, []string{"sudo", "pacman", "-S", "--noconfirm", "zsh"}},
		{PkgSpec{Backend: Mise, Name: "bun"}, []string{"mise", "use", "-g", "bun@latest"}},
		{PkgSpec{Backend: Script, Name: "cargo install wild-linker"}, []string{"sh", "-c", "cargo install wild-linker"}},
	}
	for _, c := range cases {
		got := BuildCmd(c.spec)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("BuildCmd(%v) = %v, want %v", c.spec, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/setup/ -run BuildCmd` — Expected: FAIL.

- [ ] **Step 3: Implement `installer.go`**

```go
package setup

import (
	"context"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// BuildCmd returns the argv for installing the given spec. System managers are
// sudo-prefixed (sudo prompts in the TTY; pldev never reads the password).
func BuildCmd(s PkgSpec) []string {
	switch s.Backend {
	case BrewFormula:
		return []string{"brew", "install", s.Name}
	case BrewCask:
		return []string{"brew", "install", "--cask", s.Name}
	case Apt:
		return []string{"sudo", "apt-get", "install", "-y", s.Name}
	case Dnf:
		return []string{"sudo", "dnf", "install", "-y", s.Name}
	case Pacman:
		return []string{"sudo", "pacman", "-S", "--noconfirm", s.Name}
	case Mise:
		return []string{"mise", "use", "-g", s.Name + "@latest"}
	case Script:
		return []string{"sh", "-c", s.Name}
	default:
		return nil
	}
}

// Install runs the spec's command and any post-install hooks.
func Install(ctx context.Context, s PkgSpec) (string, error) {
	argv := BuildCmd(s)
	out, err := sys.Run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return out, err
	}
	for _, hook := range s.PostInstall {
		if _, herr := sys.Run(ctx, "sh", "-c", hook); herr != nil {
			return out, herr
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/setup/ -run BuildCmd` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/setup/installer.go internal/setup/installer_test.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): installer command construction + Install"
```

---

## Task 7: Platform backend availability + real detector

**Files:** Create `internal/setup/installer_darwin.go`, `internal/setup/installer_linux.go`

These provide `AvailableBackends()` and a real `Detector` (`sysDetector`) using `sys.Run`. Thin shell-outs; pure parsing already tested in Task 4.

- [ ] **Step 1: Implement `installer_darwin.go`**

```go
//go:build darwin

package setup

import (
	"context"
	"os/exec"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

func has(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// AvailableBackends reports which backends exist on this machine.
func AvailableBackends() map[Backend]bool {
	return map[Backend]bool{
		BrewFormula: has("brew"),
		BrewCask:    has("brew"),
		Mise:        has("mise"),
		Script:      true,
	}
}

type sysDetector struct{}

// NewDetector returns the live detector for this OS.
func NewDetector() Detector { return sysDetector{} }

func (sysDetector) Installed(ctx context.Context, s PkgSpec) (bool, error) {
	switch s.Backend {
	case BrewFormula:
		out, err := sys.Run(ctx, "brew", "list", "--versions", s.Name)
		if err != nil {
			return false, nil // non-zero exit = not installed
		}
		return brewListHas(out, s.Name), nil
	case BrewCask:
		out, err := sys.Run(ctx, "brew", "list", "--cask", "--versions", s.Name)
		if err != nil {
			return false, nil
		}
		return brewListHas(out, s.Name), nil
	case Mise:
		out, err := sys.Run(ctx, "mise", "ls", "--installed")
		if err != nil {
			return false, nil
		}
		return miseLsHas(out, s.Name), nil
	default: // Script and others
		return has(s.Name), nil
	}
}
```

- [ ] **Step 2: Implement `installer_linux.go`**

```go
//go:build linux

package setup

import (
	"context"
	"os/exec"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

func has(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

func AvailableBackends() map[Backend]bool {
	return map[Backend]bool{
		Apt:    has("apt-get"),
		Dnf:    has("dnf"),
		Pacman: has("pacman"),
		Mise:   has("mise"),
		Script: true,
	}
}

type sysDetector struct{}

func NewDetector() Detector { return sysDetector{} }

func (sysDetector) Installed(ctx context.Context, s PkgSpec) (bool, error) {
	switch s.Backend {
	case Apt:
		_, err := sys.Run(ctx, "dpkg", "-s", s.Name)
		return err == nil, nil
	case Dnf:
		_, err := sys.Run(ctx, "rpm", "-q", s.Name)
		return err == nil, nil
	case Pacman:
		_, err := sys.Run(ctx, "pacman", "-Qi", s.Name)
		return err == nil, nil
	case Mise:
		out, err := sys.Run(ctx, "mise", "ls", "--installed")
		if err != nil {
			return false, nil
		}
		return miseLsHas(out, s.Name), nil
	default:
		return has(s.Name), nil
	}
}
```

- [ ] **Step 3: Build both platforms**

Run: `go build ./... && GOOS=linux go build ./...` — Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/setup/installer_darwin.go internal/setup/installer_linux.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): platform backend availability + live detector"
```

---

## Task 8: TUI MultiSelect

**Files:** Create `internal/tui/multiselect.go`

- [ ] **Step 1: Implement `multiselect.go`**

```go
package tui

import tea "github.com/charmbracelet/bubbletea"

// MultiSelectItem is one toggleable row. Header rows (Header=true) are non-selectable labels.
type MultiSelectItem struct {
	Label  string
	Key    string // identity returned in Chosen (empty for headers)
	Header bool
}

// MultiSelect is a space-toggle list returning chosen Keys on enter.
type MultiSelect struct {
	Title   string
	Items   []MultiSelectItem
	cursor  int
	checked map[int]bool
	Chosen  []string
	Aborted bool
}

func NewMultiSelect(title string, items []MultiSelectItem) MultiSelect {
	m := MultiSelect{Title: title, Items: items, checked: map[int]bool{}}
	// start cursor on first selectable
	for i, it := range items {
		if !it.Header {
			m.cursor = i
			break
		}
	}
	return m
}

func (m MultiSelect) Init() tea.Cmd { return nil }

func (m MultiSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "ctrl+c", "q", "esc":
		m.Aborted = true
		return m, tea.Quit
	case "up", "k":
		for i := m.cursor - 1; i >= 0; i-- {
			if !m.Items[i].Header {
				m.cursor = i
				break
			}
		}
	case "down", "j":
		for i := m.cursor + 1; i < len(m.Items); i++ {
			if !m.Items[i].Header {
				m.cursor = i
				break
			}
		}
	case " ":
		if !m.Items[m.cursor].Header {
			m.checked[m.cursor] = !m.checked[m.cursor]
		}
	case "enter":
		for i, it := range m.Items {
			if m.checked[i] && !it.Header {
				m.Chosen = append(m.Chosen, it.Key)
			}
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m MultiSelect) View() string {
	s := Title.Render(m.Title) + "\n\n"
	for i, it := range m.Items {
		if it.Header {
			s += "\n" + Dim.Render(it.Label) + "\n"
			continue
		}
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		box := "[ ]"
		if m.checked[i] {
			box = "[x]"
		}
		s += cur + box + " " + it.Label + "\n"
	}
	s += "\n" + Dim.Render("↑/↓ move · space toggle · enter confirm · q quit")
	return s
}
```

- [ ] **Step 2: Build**

Run: `go build ./...` — Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/multiselect.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(tui): MultiSelect space-toggle model"
```

---

## Task 9: setup orchestrator

**Files:** Create `internal/setup/setup.go`

- [ ] **Step 1: Implement `setup.go`**

```go
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
```

- [ ] **Step 2: Build + test**

Run: `go build ./... && GOOS=linux go build ./... && go test ./...` — Expected: success, all green.

- [ ] **Step 3: Commit**

```bash
git add internal/setup/setup.go
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): orchestrator (resolve/plan/confirm/apply/list/tui)"
```

---

## Task 10: Wire `pldev setup` into main + docs

**Files:** Modify `main.go`, `SKILL.md`, `README.md`

- [ ] **Step 1: Add setup handling to `main.go`**

In the flag loop, add `--dry-run`/`--list` recognition; add the subcommand dispatch. Insert these cases alongside the existing `doctor mem` / `ps` handling.

Add to the flag-parsing switch (next to `--report`/`--yes`/`--no-color`):
```go
		case "--dry-run":
			setupDryRun = true
		case "--list":
			setupList = true
```
Declare near the other opts in `main()`:
```go
	var setupDryRun, setupList bool
```
Add subcommand dispatch (before the `len(positional)==0` root-menu branch):
```go
	if len(positional) >= 1 && positional[0] == "setup" {
		toks := strings.Join(positional[1:], "+")
		err := setup.Run(ctx, setup.Options{
			Tokens: toks,
			DryRun: setupDryRun,
			Yes:    opts.YesSafe,
			List:   setupList,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
```
Add imports `"strings"` and `"github.com/peterlodri-sec/pldev/internal/setup"`. Add a `"setup"` entry to the root menu `tui.NewSelect` items and, when chosen, call `setup.Run(ctx, setup.Options{})` (interactive).

- [ ] **Step 2: Build + smoke test**

Run:
```bash
go build -o pldev . && ./pldev setup --list && ./pldev setup ripgrep+jq --dry-run
```
Expected: `--list` prints tools by category + profiles; `--dry-run` prints a plan for ripgrep+jq and exits without installing.

- [ ] **Step 3: Update `SKILL.md`** — append a section:

```markdown
## setup

`pldev setup` installs dev tooling from a curated registry (+ ~/.config/pldev/setup.toml overrides).

- `pldev setup` — interactive multiselect (profiles + tools).
- `pldev setup ghostty+zsh+bun` — install named tools.
- `pldev setup @dev` — install a profile (minimal/dev/server/work).
- `--dry-run` plan only · `--yes` skip confirm · `--list` show registry · `--no-color`.

Idempotent (skips installed). System managers (apt/dnf/pacman) run via sudo — sudo prompts you directly; pldev never sees the password. No uninstall in v1.
```

- [ ] **Step 4: Update `README.md`** — add under Use:

```markdown
    pldev setup              # interactive tool installer
    pldev setup @dev         # install the 'dev' profile
    pldev setup jq+bun --dry-run
```

- [ ] **Step 5: Final gate + commit + push**

Run: `go test ./... && go vet ./... && go build ./... && GOOS=linux go build ./...` — Expected: all green.
```bash
git add main.go SKILL.md README.md
git -c user.email="peter.lodri@instructure.com" -c user.name="peter.lodri" commit -m "feat(setup): wire pldev setup command + docs"
git push
```

---

## Self-Review (plan author)

- **Spec coverage:** registry+overrides ✓(T1,T2), native+mise backends ✓(T6,T7), profiles ✓(T1,T3), install+idempotent+dry-run+confirm ✓(T5,T9), sudo-no-capture ✓(T6 BuildCmd + T9 plan flag), TOML shape ✓(T2), CLI surface+flags ✓(T10), resolution ✓(T3), detection ✓(T4,T7), errors (unknown/unsupported/bad-toml/install-fail) ✓(T3,T5,T9), testing ✓(T1-6), seed registry ✓(T1), docs/dep ✓(T0,T10), TUI multiselect ✓(T8,T9).
- **Type consistency:** `Backend`/`PkgSpec`/`Tool`/`Profile`/`Registry` (T1) used everywhere; `Detector` (T4) consumed by `BuildPlan` (T5) + implemented in T7; `BuildCmd`/`Install` (T6) used in T9; `Item`/`InstallPlan` (T5) used in T9; `tui.MultiSelect`/`NewMultiSelect`/`MultiSelectItem`/`.Chosen`/`.Aborted` (T8) used in T9; `NewDetector`/`AvailableBackends` (T7) used in T9. Consistent.
- **Placeholders:** none — real code/commands throughout.
- Note: `mimalloc`/`wild` registry specs are best-effort (wild uses Script/cargo); honest per spec, easy to refine.
```
