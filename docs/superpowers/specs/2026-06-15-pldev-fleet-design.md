# pldev fleet — federation substrate (design)

## Context

The user runs ~11 tailnet nodes (macOS control boxes M1/M3/MBP, Hetzner linux compute `dev-cx53`/`crabcc-ccx33`, a `crabcc-nats` message bus, agent/bench/nix linux nodes, rpi4 + an offline rpi3b) with **no unified view or control plane**. The eventual goal — federation (ruflo-federation over the existing NATS bus) + ruvllm→Ollama routing — needs a substrate first: you can't federate or route to nodes you can't enumerate, reach, or describe.

This spec is that substrate: **`pldev fleet`** — a config-annotated, tailscale-discovered node registry with list / health / ssh / run. Federation coordination and ruvllm routing are deferred to their own specs (this is the first of the federation thread, mirroring how #0→#3 staged).

## Decisions (locked)

- **Hybrid registry:** `tailscale status --json` is ground truth for which nodes exist, their IPs, OS, online state, tags; `~/.config/pldev/fleet.toml` annotates `role`, `caps`, `ssh_user`, `exclude` per node, merged by short name. No hand-maintained IPs.
- **v1 commands:** `list`, `health`, `ssh`, `run` (fan-out exec).
- **`run` safety:** `--dry-run` (print resolved nodes + command, run nothing); confirm before a live run unless `--yes`; ssh `BatchMode` key-auth only (no password prompt — can't hang or capture); per-node `ssh_user`; parallel exec, collect, report ok/fail + output per node.

## Architecture

`internal/fleet/`:

- **discover.go** — run `tailscale status --json`, parse into `[]TSNode{Name, IP, OS, Online, Tags}` (short name = first DNS label).
- **config.go** — `fleet.toml`: `[node.<name>]` tables with `role`, `caps[]`, `ssh_user`, `exclude`; plus `default_ssh_user`. `LoadFleetConfig(path)`.
- **fleet.go** — `Node{Name, IP, OS, Online, Tags, Role, Caps, SSHUser, Excluded}`; `Merge(tsNodes, cfg) []Node` (annotate by name, drop excluded, fill ssh_user from default); `Select(nodes, selector)` where selector ∈ `all` / `@<role>` / `<name>`.
- **commands** (driven from `main.go` `pldev fleet …`, scope `pldev>fleet`):
  - `list` — table (name / role / os / online / caps) → **stdout** (data).
  - `health` — parallel per node: online (from tailscale) + ssh reachability (`ssh -o BatchMode=yes -o ConnectTimeout=6 <user>@<ip> true`); one **report** line each (ok/fail) + summary.
  - `ssh <node>` — interactive `ssh <user>@<ip>` (RunInteractive).
  - `run <selector> -- <cmd>` — resolve selector → nodes; `--dry-run` prints the plan; else confirm (unless `--yes`); parallel `ssh -o BatchMode=yes <user>@<ip> <cmd>`, collect stdout per node; print a per-node labeled result + ok/fail summary. Bounded concurrency (pool, like the recipe runner).

Reuses: `internal/report` (action/status lines; data tables stay on stdout), `internal/sys` (Run/RunInteractive), the bounded-parallel pattern.

## Capability model

`caps` is a free-form string list per node in config (e.g. `["ollama","docker","nixos"]`). v1 only stores + displays it. The ruvllm spec will consume `caps` to decide where Ollama can run (Hetzner/linux yes; pis/phones no). Roles seed sensible defaults (e.g. `role="hetzner"` → `caps` hints), but caps stay explicit.

## Safety / contract

- `list`/`health` are read-only.
- `ssh`/`run` are outward. `run` is dry-run-previewable + confirm-gated; ssh always `BatchMode` (key auth only — never prompts for or captures a password). pldev never logs command output as secrets, though `run` does print captured stdout (its purpose) — the user selects the command.
- Excluded nodes (`exclude=true`) never appear in selectors or health.
- Offline nodes (tailscale `Online=false`) are shown in `list`, marked, and skipped by `run`/`ssh` with a clear "offline" status rather than a hang.

## Verifiable now vs deferred

- **Now:** tailscale-JSON parse (fixture), config merge, selector resolution, list render — unit-tested. `list` + `health` smoke live (tailnet is up). `run --dry-run` smoke live.
- **Smoke live (safe):** `pldev fleet list`, `pldev fleet health`, `pldev fleet run @role --dry-run`.
- **Deferred (own specs):** ruflo-federation coordination over the `crabcc-nats` bus; ruvllm→Ollama routing keyed on `caps`. `fleet run` live mutating exec is gated, exercised by the user.

## Verification

1. `go build/vet/test ./...` green; fleet unit tests pass (parse/merge/select/render).
2. `pldev fleet list` shows the real tailnet nodes with roles/caps from `fleet.toml` (and defaults when unconfigured).
3. `pldev fleet health` parallel-probes; offline rpi3b shows `offline`, reachable nodes show `ok`.
4. `pldev fleet run @hetzner -- uname -a --dry-run` prints the resolved node set + command, runs nothing; without `--dry-run` it confirms first.
5. Excluded nodes are absent from selectors.
