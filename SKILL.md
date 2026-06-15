---
name: pldev
description: Use pldev to diagnose and fix machine memory/swap/disk problems. Triggers on "check my memory", "what's eating my RAM", "diagnose this machine", "free up space", "pldev".
---

# pldev

`pldev` is a cross-platform (macOS/Linux) machine doctor. v1 module: `doctor mem`.

## Run

- Interactive: `pldev doctor mem` — shows a severity-coded report, then an
  arrow-key menu of fixes.
- Non-interactive (capture/ssh): `pldev doctor mem --report` — prints the report
  only, no prompts. Use this when reading output programmatically.
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
