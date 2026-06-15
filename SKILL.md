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

### setup recipes (composite flows)

`pldev setup <recipe>` runs an ordered, idempotent, dry-run-aware recipe (engine in `internal/setup/recipe`). `pldev setup ruflo` installs ruflo + wires two memory MCPs globally — **honcho** (relational/peer memory, remote on dev-cx53) and **mempalace** (file/corpus recall, local pgvector) — provisions the honcho stack via `nixos-rebuild` (auth-gated, biometric), sets `CLI_CORE`/langfuse env (secrets via `$(bao kv get …)` shell indirection, never on disk), and drops a memory-lane directive in `~/.claude/CLAUDE.md`. Config defaults in `~/.config/pldev/recipes.toml` (canonical copy on M1). `--dry-run` previews every step; secret-bearing steps never log values.

## Action log & version

- `pldev --version` (or `pldev version`) prints embedded build info: `pldev <ver> (<sha>, <commit-date>, go1.x)`.
- Every command logs one line per action to **stderr** (data stays on stdout). logfmt fields: `ts lvl scope action target status [k=v…] [msg]`; `status` ∈ ok/fail/skip/have/do/dry. In `--dry-run`, pending actions log as `status=dry`.
- Env: `PLDEV_LOG=plain|pretty|json` forces render mode; `NO_COLOR=1` forces plain. Parse the log with standard logfmt tooling (`grep 'status=fail'`, etc.).

## fleet (federation substrate)

`pldev fleet` is a tailscale-discovered, config-annotated node registry. `~/.config/pldev/fleet.toml` annotates each node by short name: `[node.<name>]` with `nickname`, `role`, `caps`, `ssh_user`, `exclude` (+ a top-level `default_ssh_user`). Set real `ssh_user`s there or `health`/`ssh`/`run` can't reach nodes. A `nickname` becomes the node's primary display identity (lore name) and is selectable (case-insensitive) anywhere a node name is — `pldev fleet ssh Katedralis`.

- `pldev fleet list` — table (node/role/os/status/caps).
- `pldev fleet health` — parallel 3-tier probe → 4 states per node: **offline** (tailscale down) · **unreachable** (online but TCP :22 won't connect) · **reachable** (port open, ssh command failed — usually wrong `ssh_user`) · **ssh-ok** (fully usable). Per-state summary line.
- `pldev fleet ssh <node>` — interactive shell.
- `pldev fleet run <selector> -- <cmd>` — parallel fan-out exec; selector = `all` / `@<role>` / `<name>`. **Gated:** `--dry-run` previews, confirm unless `--yes`, ssh key-auth only. Offline nodes skipped.
- `pldev fleet sync-signers` — push the PUBLIC `allowed_signers` trust anchor (the provenance signers) to every reachable node's `~/.config/pldev/allowed_signers` + local, so all nodes verify the same identities. **Private keys never sync** (signing stays on origin). Gated (`--dry-run`/`--yes`). Run this after adding a signer to `internal/verify/allowed_signers`.
- `pldev fleet ssh-config` — generate `~/.ssh/config` Host aliases (nickname + tailnet name → HostName/User) in a managed block (backs up first), so `ssh Katedralis` / `ssh Herald` work. `--dry-run` prints the block.

`pldev fed status` — read-only swarm view: fetches the NATS bus monitoring endpoint (`:8222` `/varz`+`/connz`) on the `role="bus"` node (the Herald; override with `[federation] monitor_url`), and renders server health + connected clients mapped to fleet nicknames. Populates once ruflo-federation connects agents.

`pldev fed validate <file>` — the agent-protocol boundary check: runs a vaked `agent_msg` through `vakedc` (valid → ok; malformed → rejected, never crosses to the swarm). Needs `vakedc` + the `agent_msg` schema in vaked-base. Live `fed watch`/`pub` over NATS deferred until agents emit traffic. ruflo-federation *enablement* + ruvllm→Ollama routing layer on top — separate specs.

## Provenance

`pldev verify <path>` checks a candidate binary against its `<path>.sig` using the embedded `internal/verify/allowed_signers` trust anchor (**ssh-keygen** signatures — key-based, no transparency log, reuses your ssh keys). It resolves the signer (must be in `allowed_signers`) then verifies over the file. Verify a *target* with an already-trusted pldev — never self-verify; bootstrap binary #1 once with raw `ssh-keygen -Y verify`. `v*` tags are signed by `.github/workflows/release.yml` (key from the `SSH_SIGNING_KEY` secret); `task sign BIN=… [KEY=~/.ssh/id_ed25519]` signs locally (ssh-keygen prompts for the passphrase / uses ssh-agent). Adding a trusted signer = a line in `allowed_signers` (`<id> ssh-ed25519 <key>`) — also where the gmail/cabotage identities land later. Built-in `ssh-keygen`, no extra install.
