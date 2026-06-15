# pldev `setup` — Design Spec

**Date:** 2026-06-15
**Status:** Approved (design), pending implementation plan
**Author:** peter.lodri
**Depends on:** existing pldev v1 (`doctor mem`, `ps`) — reuses `internal/sys`, `internal/tui`, `probe.Proc`.

## Purpose

`pldev setup` installs developer tooling across the author's machines (work/personal
MacBooks, Linux dev boxes, servers) from a curated, declarative registry. Goal: type
`pldev setup ghostty+zsh+bun` or `pldev setup @dev` on any machine and get the right
tools via the native package manager, idempotently, with a confirm-first plan.

## Decisions (locked during brainstorming)

1. **Registry model:** built-in curated registry baked into the binary + user overrides
   in `~/.config/pldev/setup.toml`. Logical names "just work".
2. **Backends (v1):** native managers (brew incl. casks on macOS; apt/dnf/pacman on
   Linux) + **mise** for version-pinned dev runtimes. **No Nix in v1** (addable later as
   a new `Installer` impl — the interface makes it cheap).
3. **Profiles:** flat tool selection + named profiles (`minimal`/`dev`/`server`/`work`),
   defined in registry and overridable. A profile = alias to a tool-name list.
4. **Operations:** install + idempotent skip + plan/confirm + `--dry-run`. **No uninstall**
   in v1 (`sync --prune` deferred).

## Non-Goals (v1)

- No uninstall / prune / machine convergence.
- No Nix/home-manager backend.
- No external/pullable registry repo (registry is embedded Go data).
- No password capture (sudo prompts in the user's TTY; pldev never reads it).

## Layout (new `internal/setup/`)

```
internal/setup/
  setup.go        orchestrate: resolve → plan → confirm/dry-run → apply → summary
  registry.go     Tool / Profile / PkgSpec / Backend types + embedded built-in registry
  manifest.go     load + merge user overrides from ~/.config/pldev/setup.toml
  resolve.go      input tokens / @profiles / categories → deduped []Tool (PURE)
  plan.go         build InstallPlan via injected Detector
  detect.go       per-backend "is installed" checks + output parsers (PURE parsers)
  installer/
    installer.go  Installer interface, Backend availability, command construction
    brew.go       //go:build darwin   (formulae + --cask, brew tap)
    apt.go        //go:build linux
    dnf.go        //go:build linux
    pacman.go     //go:build linux
    mise.go       (no build tag) version-pinned dev runtimes
  testdata/       fixtures: brew-list.txt, dpkg-status.txt, mise-ls.txt
internal/tui/
  multiselect.go  new MultiSelect model (space-toggle, grouped) — reused by setup TUI
```

## Types

```go
type Backend string

const (
    BrewFormula Backend = "brew"
    BrewCask    Backend = "brew-cask"
    Apt         Backend = "apt"
    Dnf         Backend = "dnf"
    Pacman      Backend = "pacman"
    Mise        Backend = "mise"
    Script      Backend = "script" // custom shell install (last resort)
)

// PkgSpec is one way to install a tool on a platform. For Script backend,
// Name holds the shell command to run.
type PkgSpec struct {
    Backend     Backend
    Name        string   // package id for the backend, or shell cmd for Script
    Tap         string   // optional brew tap
    PostInstall []string // optional shell commands run after install
}

// Tool is a logical entry. Darwin/Linux hold ordered candidate specs;
// the first whose backend is available on the machine wins.
type Tool struct {
    Name     string
    Desc     string
    Category string // terminal, shell, cli, build, containers, net, editor
    Darwin   []PkgSpec
    Linux    []PkgSpec
}

type Profile struct {
    Name  string
    Desc  string
    Tools []string // logical tool names (NOT other profiles — kept flat)
}

type Registry struct {
    Tools    map[string]Tool
    Profiles map[string]Profile
}
```

## User Override (`~/.config/pldev/setup.toml`)

```toml
[tools.ghostty]
desc     = "GPU terminal"
category = "terminal"
darwin   = [{ backend = "brew-cask", name = "ghostty" }]
linux    = [{ backend = "apt", name = "ghostty" }]

[profiles.dev]
desc  = "full dev box"
tools = ["ghostty","zsh","jq","ripgrep","bun","mold","docker","gh"]
```

Merge rule: a user `[tools.X]` or `[profiles.X]` fully **replaces** the built-in entry of
the same key; new keys are added. Parsed with Go stdlib-friendly TOML
(`github.com/BurntSushi/toml`). Missing file → registry = built-in only.

## CLI Surface

- `pldev setup` → interactive TUI (profiles listed first, then categories→tools,
  space toggles, enter confirms selection) → plan → confirm → apply.
- `pldev setup ghostty+zsh+bun` → explicit names, split on `+`.
- `pldev setup @dev` → expand profile `dev`. Tokens may mix: `@minimal+docker`.
- Flags: `--dry-run` (print plan, exit 0, no mutation), `--yes` (skip confirm),
  `--list` (print registry tools by category + profiles, exit), `--no-color`.
- Add "setup" entry to the root `pldev` menu.

## Resolution (`resolve.go`, pure)

1. Split input on `+`. Each token: `@name` → profile; `name` → tool; (categories
   reachable in TUI only, not required on CLI for v1).
2. Expand profiles to their tool lists.
3. Look up each tool in the merged registry. Unknown name → return an error naming the
   unknown token(s) and listing valid names. Dedup by tool name preserving order.

## Backend Selection & sudo

For each resolved tool, choose the first `PkgSpec` (in the platform's slice) whose
`Backend` is **available** on this machine (`brew`/`apt`/`dnf`/`pacman`/`mise` found on
PATH; or dpkg/rpm presence). If none available → mark tool **Unsupported** for this OS.

- brew, mise, Script(no sudo): run directly via `sys.Run`.
- apt/dnf/pacman: command is prefixed with `sudo` (e.g. `sudo apt-get install -y X`).
  sudo prompts in the inherited TTY; **pldev never captures the password.** The plan
  flags which steps will invoke sudo.

## Plan (`plan.go`)

```go
type Detector interface { Installed(ctx, PkgSpec) (bool, error) }

type Item struct { Tool Tool; Spec PkgSpec; NeedsSudo bool }
type InstallPlan struct {
    ToInstall   []Item
    Installed   []Tool // already present → skip
    Unsupported []Tool // no available backend on this OS
}
```

`BuildPlan(reg, names, detector, os)` resolves, selects backend, runs detection, and
classifies. Detector is an interface → unit-testable with a fake (no real shell-outs).

## Detection (`detect.go`)

Per backend, "is installed":
- brew: `brew list --versions <name>` (non-empty) / `brew list --cask --versions` for casks.
- apt: `dpkg -s <name>` exit 0. dnf: `rpm -q <name>`. pacman: `pacman -Qi <name>`.
- mise: parse `mise ls --installed` for the tool.
- Script/fallback: `command -v <name>`.

Output parsers (brew-list, dpkg-status, mise-ls) are PURE functions, fixture-tested.

## Apply & Summary

For each `ToInstall` item: print intended command, run it (sudo-prefixed where flagged),
run any `PostInstall` hooks on success. Continue on failure. Final summary:
`N installed, M skipped (already present), K unsupported, J failed` with the failed
tool names + error tails.

Confirm gate: after printing the plan, require `[y/N]` (via `tui.Confirm`) unless
`--yes`. `--dry-run` prints the plan and exits before any mutation.

## Error Handling

- Unknown tool/profile token → error listing unknowns + valid names; non-zero exit.
- No available backend for a tool → Unsupported (continue others).
- Install/PostInstall failure → recorded, continue, surfaced in summary; exit non-zero
  if any failed.
- Malformed user TOML → error with file path + parse message; fall back to nothing
  (do not silently ignore — the user asked for overrides).

## Testing

- Registry + user-override merge: table tests (built-in only; override replaces;
  new entry added; malformed file errors).
- `resolve`: names / `@profiles` / mixed / dedup / unknown-token error — pure unit.
- Detection parsers: fixtures (`brew list --versions`, `dpkg -s`, `mise ls`).
- `BuildPlan`: injected fake `Detector` + fake backend-availability → asserts
  ToInstall/Installed/Unsupported and NeedsSudo flags.
- Installer command construction: assert the exact argv built per backend (incl. sudo
  prefix and brew --cask), WITHOUT executing.
- TUI MultiSelect: manual verification.

## Seed Registry (v1)

Tools (category): ghostty(terminal), zsh/fish(shell), jq/ripgrep/fd/bat/eza/fzf(cli),
tmux(cli), neovim(editor), starship(shell), gh/git(cli), tailscale(net), docker(containers),
bun/node/go(cli, via mise), mold/mimalloc/wild(build).

Profiles:
- `minimal`: zsh, git, ripgrep, fzf, neovim
- `dev`: minimal set + ghostty, jq, fd, bat, eza, bun, node, gh, docker, tmux, starship
- `server`: zsh, git, ripgrep, fzf, neovim, tmux, docker, tailscale
- `work`: dev set + tailscale

Registry accuracy is iterative: build libs without a clean brew/apt formula
(e.g. `wild` linker) use the `Script` backend or are marked honestly; the embedded
registry is data and easy to extend.

## Distribution / Docs

- Update `SKILL.md` and `README.md` with `pldev setup` usage.
- New dep: `github.com/BurntSushi/toml` (user-override parsing).

## Future (out of scope)

`sync --prune` (uninstall tools not in profile), Nix backend, external registry repo,
`doctor mcp` (orphaned MCP/npx process detection — separate doctor module).
