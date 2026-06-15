# arp-corpus — validated agent-activity corpus + memory feed (design)

## Context

`vaked-base` PR #273 shipped a deterministic PostToolUse hook (`.claude/hooks/arp_log.py`) that records substantial shell commands as **validatable `arp_event` Vaked declarations** — replacing an advisory nag rule that left the log empty. Each entry is `vakedc check`-provable.

The goal here (user-chosen end-state): turn that into a **practical, swarm-wide, always-valid corpus that feeds agent recall and auditability**. Not a research notation — the emergent "LLM-Human language" is a side-benefit, explicitly out of scope for this spec (deferred to its own thread).

This builds on pldev #2: the corpus feeds the **MemPalace** memory lane (file/corpus recall) already wired by `setup ruflo`.

## Goal

A central git repo (`peterlodri-sec/arp-corpus`) where every machine's agent activity accumulates as host/date-partitioned `arp_event` files, kept provably valid by `vakedc` (local gate + CI), synced by `pldev`, and mined by MemPalace. Success: agents recall/act on prior activity, and the corpus is never invalid.

## Architecture

```
each machine (M1 / M3 / dev-cx53 / …):
  Claude Code PostToolUse hook (#273)
        │  appends arp_event
        ▼
  local clone:  ~/.local/state/pldev/arp-corpus/corpus/<host>/<YYYY-MM-DD>.md
        │
  pldev arp sync   (manual or timer)
        │  vakedc check (local gate)  →  git commit + push
        ▼
  GitHub: peterlodri-sec/arp-corpus
        │  CI: vakedc check on every push (hard gate)
        ▼
  MemPalace mines a clone  →  corpus/recall lane (pldev #2)
```

Host/date partitioning (`corpus/<host>/<date>.md`) makes concurrent multi-machine appends **conflict-free** — no two machines write the same file.

## Components

### 1. `arp-corpus` repo (new)
- `corpus/<host>/<date>.md` — append targets (fenced ```vaked `arp_event` blocks).
- `vaked/` — a copy of the `arp_event` schema (so CI can validate standalone).
- `.github/workflows/validate.yml` — on push: extract every ```vaked fence → `vakedc check`; red on any invalid entry. This is the load-bearing validity gate.
- `README.md` — partition convention, how MemPalace consumes it, how `pldev arp` writes it.

### 2. Hook (vaked-base, small tweak)
- Reuse the #273 hook via its portable `docs/arp-log.meta-hook.md` bootstrap (installs + registers standalone).
- **Retarget output** from the per-project `docs/arp-log.md` to the local clone path. Requires the hook to honor an `ARP_LOG_PATH` env (and default to `~/.local/state/pldev/arp-corpus/corpus/<host>/<date>.md`). If #273 hardcodes the path, this is a minimal vaked-base change: read `ARP_LOG_PATH`, fall back to current behavior.
- **Identity without schema change:** host + date come from the file path; session id is appended to the existing `notes` field. No `arp_event` schema or vaked grammar change — the richer-event-vocabulary idea stays in the deferred language thread.

### 3. `pldev setup arp` (recipe on the #1 engine)
Ordered steps:
- `InstallTools` — `git` (+ the vaked toolchain / `uv` for `vakedc` if needed).
- `RunCmd` — clone `arp-corpus` to `~/.local/state/pldev/arp-corpus` (probe: dir exists).
- install + register the #273 hook via the meta-hook bootstrap; set `ARP_LOG_PATH` to the clone's host/date target.
- point MemPalace at the clone (config the mempalace mine path).
- `Verify` — `vakedc check` passes on the clone; hook registered in `.claude/settings.json`.

### 4. `pldev arp` command
- `pldev arp sync` — `vakedc check` the local clone (abort on invalid), `git add/commit/push` the host/date file. Batched; optional timer.
- `pldev arp verify` — local `vakedc check` only (the gate, no push).
- Emits through `internal/report` (one line per action), like every pldev command.

### 5. MemPalace feed
Mine the arp-corpus clone as a corpus source (the #2 mempalace lane already exists). No new memory plane — arp-corpus is a source feeding the existing corpus/recall lane.

## Validity guarantee

Two gates ensure the corpus never goes invalid (the failure that killed the old advisory log):
- **Local:** `pldev arp sync` runs `vakedc check` and refuses to push invalid entries.
- **CI:** `validate.yml` re-checks on every push; main stays green-or-it-didn't-land.

## Safety / contract

- The hook always exits 0 (never blocks the tool pipeline) — carried from #273.
- Captured commands may contain sensitive args; the hook's existing noise filter applies, and arp-corpus is a **private** repo. (A redaction pass is a possible later addition, noted not built.)
- `pldev arp sync` pushes to a private remote — an outward action; it runs on explicit invocation or an opted-in timer, never silently.

## Out of scope (deferred, documented)

- **Subagent / swarm-internal capture** — only the main session's PostToolUse Bash is captured; subagent-internal tool calls may not fire the parent hook. Documented as a known gap, not worked around in v1.
- **Richer event vocabulary** (reasoning/decision/intent events beyond `arp_event`) — that's the "LLM-Human native language" research thread, deliberately separate.
- **Redaction / secret-scrubbing** of captured commands beyond the existing noise filter.

## Repos touched

- `peterlodri-sec/arp-corpus` — new (structure + CI + schema copy).
- `pldev` — `setup arp` recipe + `arp sync`/`verify` command + MemPalace mine config.
- `vaked-base` — `ARP_LOG_PATH` support in the #273 hook (minimal).

## Verification

1. `pldev setup arp` on a machine: clones the corpus, installs+registers the hook with `ARP_LOG_PATH` set, points MemPalace at the clone, `vakedc check` passes.
2. Run a substantial shell command → an `arp_event` appends to `corpus/<host>/<date>.md` in the clone; a trivial command is skipped (noise filter).
3. `pldev arp verify` → `vakedc check` exit 0. `pldev arp sync` → commits + pushes; an injected invalid entry makes `sync` abort before push.
4. CI `validate.yml` goes red on a pushed invalid fence, green otherwise.
5. MemPalace recall surfaces a recent `arp_event` from the corpus.
