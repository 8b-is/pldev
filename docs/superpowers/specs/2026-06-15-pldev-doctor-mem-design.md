# pldev — Design Spec (v1: `doctor mem`)

**Date:** 2026-06-15
**Status:** Approved (design), pending implementation plan
**Author:** peter.lodri

## Purpose

`pldev` is a single-entrypoint personal CLI/TUI helper for managing the author's
machines (work/personal MacBooks, dev boxes, Linux servers). v1 ships one module:
`doctor mem` — diagnose memory/swap/filesystem-event/disk-bloat problems and offer
guided, safe fixes. The architecture is module-based so future doctors
(`disk`, `net`, `processes`) and an `install`/`sync` capability slot in later.

### Motivating incident

A 36 GB MacBook hit recurring memory pressure. Root causes found manually:
- **Swap thrash** — 42.9 GB / 44 GB swap used after 20 days uptime (never rebooted).
- **`fseventsd` leak** — 1.9 GB RSS, runaway CPU, fed by Spotlight indexing of huge
  trees (`~/OrbStack` 45 GB, `~/workspace`, 3108 `node_modules` dirs / 25 GB).
- **Zombie LaunchAgent** — uninstalled `openclaw-gateway` left a `KeepAlive` plist
  that launchd respawned every 1 s.

`doctor mem` codifies this triage so it is repeatable across machines.

## Non-Goals (v1)

- No `install` / dotfiles-sync (future module).
- No `doctor disk`/`net`/`processes` (future modules).
- No password capture / no auto-`sudo`. sudo-needing fixes are printed for the user to run.
- No swap resizing/disabling (blocked by SIP on macOS; not attempted).

## Stack

- **Language:** Go, single static binary per arch. Zero runtime deps.
- **TUI:** charmbracelet **Bubble Tea** (loop) + **Lipgloss** (color/layout) + **Bubbles** (list, spinner, confirm).
- **Platform split:** Go build tags (`//go:build darwin`, `//go:build linux`).
- **Task runner:** **Taskfile** (`Taskfile.yml`, go-task) — not Make.

## Repository

- New **private** git repo, remote on GitHub (`peterlodri-sec`).
- Local path: `~/workspace/pldev`.

## Layout

```
pldev/
  Taskfile.yml             build / install / cross-compile / test / lint tasks
  go.mod
  main.go                  entry: args → TUI root menu | direct subcommand
  SKILL.md                 Claude skill: how to invoke & read pldev (see below)
  internal/
    tui/
      menu.go              Bubble Tea root menu model
      styles.go            Lipgloss palette + severity colors
      confirm.go           reusable [y/N] + multiselect prompts
    doctor/
      mem/
        mem.go             orchestrate collect → render → fix
        report.go          MemReport struct + render model
      probe/
        probe.go           Collector interface, MemReport type, severity logic
        probe_darwin.go    vm_stat, sysctl, ps, launchctl, du, uptime, mdutil state
        probe_linux.go     /proc/meminfo, free, swapon, systemctl, du, uptime
      fix/
        fix.go             Fix interface (Describe/Safe/Apply), registry/selection
        fix_darwin.go      spotlight marker, LaunchAgent removal, node_modules purge
        fix_linux.go       systemd orphan removal, node_modules purge
    sys/
      exec.go              run command, capture stdout/err, context timeout
  docs/superpowers/specs/  this spec
```

## Command Surface

- `pldev` → root interactive menu (lists available doctors).
- `pldev doctor mem` → straight into mem doctor: render report, then fix menu.
- Flags:
  - `--report` — non-interactive plain/colored dump, no prompts (for ssh/scripts).
  - `--yes` — auto-apply **Safe** fixes only; never destructive.
  - `--no-color` — disable Lipgloss styling (CI/pipes).

## Data Model

`MemReport` (populated by the platform `Collector`):

| Field | macOS source | Linux source |
|---|---|---|
| RAM total/used/free | `sysctl hw.memsize`, `vm_stat` | `/proc/meminfo` / `free` |
| Swap used/total | `sysctl vm.swapusage` | `swapon --show` / `/proc/meminfo` |
| Compressor pages | `vm_stat` | n/a |
| Top-N processes by RSS | `ps aux` | `ps aux` |
| `fseventsd` RSS | `ps -p <pid>` | n/a |
| node_modules inventory | `find` + `du` | `find` + `du` |
| Zombie services | `launchctl list` + plist scan | `systemctl --user list-units` orphan scan |
| Uptime | `uptime` / `sysctl kern.boottime` | `/proc/uptime` |

`Collector` interface (platform-implemented):

```go
type Collector interface {
    Mem(ctx) (MemStat, error)
    Swap(ctx) (SwapStat, error)
    TopProcs(ctx, n int) ([]Proc, error)
    FSEvents(ctx) (*Proc, error)          // nil on linux
    NodeModules(ctx, roots []string) ([]DirSize, error)
    Zombies(ctx) ([]Service, error)
    Uptime(ctx) (time.Duration, error)
}
```

## Flow (`doctor mem`)

1. **Probe** — platform `Collector` fills `MemReport`. Each probe runs under a
   `context` timeout; failure → that section marked `unavailable`, flow continues.
2. **Render** — Lipgloss report, severity-coded.
3. **Fix menu** (skipped if `--report`) — list applicable `Fix`es, each `[y/N]`:
   - **Spotlight-exclude heavy dirs** (darwin, Safe) — write `.metadata_never_index`
     in selected trees (`~/workspace`, `~/.orbstack`, …). iCloud noted as
     GUI-only (SIP). Reversible: delete marker.
   - **Remove zombie service** (Safe-ish) — `launchctl bootout` + remove plist
     (darwin) / disable+remove unit (linux). Shows full path before acting.
   - **Purge node_modules** (Destructive) — multiselect picker showing dir + size,
     explicit confirm listing exactly what is deleted + total reclaim. Recoverable
     via reinstall. Never under `--yes`.
   - **Reboot hint** (Safe) — if uptime high AND swap high, print recommendation
     (does not reboot).
4. **Apply** — every `Fix` prints what it will do, then acts, then reports result.

## Severity Thresholds

- Swap used: `>50%` yellow, `>75%` red.
- Free RAM: `<10%` red.
- `fseventsd` RSS (darwin): `>500 MB` yellow, `>1 GB` red.
- node_modules: informational, sorted by size descending.

## Safety / Guardrails

- Each `Fix` declares `Safe() bool`. **Safe** = reversible (spotlight marker, reboot
  hint). **Destructive** = node_modules purge, service removal.
- Destructive fixes: never auto-applied; always show full paths + sizes + explicit
  confirm. `--yes` applies Safe only.
- No password capture. A fix needing root prints the exact `sudo …` command for the
  user to run; `pldev` does not execute it.
- Dry-run mindset: print intended action before performing it.

## Error Handling

- Missing tool / command error → section `unavailable`, continue (degrade gracefully).
- `du` / `find` on huge trees → per-probe `context` timeout, partial results returned.
- Unsupported OS → clear message, exit non-zero.

## Testing

- `Collector` and `Fix` are interfaces → mockable.
- Parse functions (`vm_stat`, `free`, `/proc/meminfo`, `swapon`, `ps`) unit-tested
  against captured sample-output fixtures committed to the repo.
- Report rendering tested with fixture `MemReport` (golden output, `--no-color`).
- Fix selection/confirm logic tested with an injected fake filesystem — no real
  deletions in tests.
- Taskfile `test` task runs `go test ./...`.

## SKILL.md (deliverable)

A Claude Agent Skill shipped in the repo so agents know how to drive `pldev`:
- **Frontmatter:** `name: pldev`, `description:` triggers on "check memory", "what's
  eating my RAM", "diagnose this machine", "pldev".
- **Body:** how to run (`pldev doctor mem`, `--report` for non-interactive capture),
  how to read the severity output, which fixes are safe vs destructive, and the
  reminder that sudo fixes are printed not executed.

## Distribution (installer seed)

- v1: `Taskfile.yml` tasks — `build`, `install` (→ `~/.local/bin/pldev`),
  `cross` (darwin/arm64, darwin/amd64, linux/amd64, linux/arm64), `test`, `lint`.
- v2 (future): `install.sh` curl-bootstrap + GitHub Releases; broader installer/sync modules.

## Future Modules (out of scope for v1)

`doctor disk`, `doctor net`, `doctor processes`, `pldev install` (dotfiles/tool
bootstrap across machines), `pldev sync`.
