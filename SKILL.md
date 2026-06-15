---
name: pldev
description: Use pldev to diagnose and fix machine memory/swap/disk problems, or show a neofetch-style system overview. Triggers on "check my memory", "what's eating my RAM", "diagnose this machine", "free up space", "system info", "pldev ps", "pldev".
---

# pldev

`pldev` is a cross-platform (macOS/Linux) machine doctor. Modules: `doctor mem`, `ps`.

## Run

- Interactive: `pldev doctor mem` — shows a severity-coded report, then an
  arrow-key menu of fixes.
- Non-interactive (capture/ssh): `pldev doctor mem --report` — prints the report
  only, no prompts. Use this when reading output programmatically.
- `pldev ps` — neofetch-style system overview panel (host/OS/kernel/model/arch/
  uptime, CPU & memory, disks, network interfaces). Always non-interactive; prints
  and exits. Honors `--no-color`.
- `--yes` applies SAFE fixes automatically (never destructive).
- `--no-color` for pipes/CI.

## Reading the report

Severity tags: `OK` / `WARN` / `CRIT`.
- Swap >50% WARN, >75% CRIT — high swap = paging = slowdowns; reboot is the fix.
- Free RAM <10% CRIT.
- fseventsd (macOS) >500MB WARN, >1GB CRIT — Spotlight indexing churn.
- node_modules listed largest-first.

## Fixes

- **SAFE** (reversible, ok to apply): Spotlight-exclude dirs, reboot hint.
- **DESTRUCTIVE** (explicit confirm required): purge node_modules (recoverable via
  reinstall), remove zombie LaunchAgent / disable failed systemd unit.

## Important

pldev never runs `sudo` or captures passwords. If a fix needs root, it prints the
exact command for the human to run.

## setup

`pldev setup` installs dev tooling from a curated registry (+ ~/.config/pldev/setup.toml overrides).

- `pldev setup` — interactive multiselect (profiles + tools).
- `pldev setup ghostty+zsh+bun` — install named tools.
- `pldev setup @dev` — install a profile (minimal/dev/server/work).
- `--dry-run` plan only · `--yes` skip confirm · `--list` show registry · `--no-color`.

Idempotent (skips installed). System managers (apt/dnf/pacman) run via sudo — sudo prompts you directly; pldev never sees the password. No uninstall in v1.

## Action log & version

- `pldev --version` (or `pldev version`) prints embedded build info: `pldev <ver> (<sha>, <commit-date>, go1.x)`.
- Every command logs one line per action to **stderr** (data stays on stdout). logfmt fields: `ts lvl scope action target status [k=v…] [msg]`; `status` ∈ ok/fail/skip/have/do/dry. In `--dry-run`, pending actions log as `status=dry`.
- Env: `PLDEV_LOG=plain|pretty|json` forces render mode; `NO_COLOR=1` forces plain. Parse the log with standard logfmt tooling (`grep 'status=fail'`, etc.).
