# arp-corpus Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Steps use `- [ ]`.

**Goal:** A validated, swarm-wide agent-activity corpus (`peterlodri-sec/arp-corpus`) fed by the #273 hook, synced + verified by pldev, mined by MemPalace.

**Architecture:** Central git repo, host/date-partitioned `arp_event` files, `vakedc`-gated (local via `pldev arp verify` + CI). `pldev setup arp` (recipe on the #1 engine) installs the pipeline; `pldev arp sync` pushes batches.

**Tech:** Go (pldev), the #1 recipe engine, `internal/report`; GitHub Actions + `vakedc` (python) for CI.

---

## Task 1: arp-corpus repo skeleton

**Files (in the new repo):** `README.md`, `corpus/.gitkeep`, `vaked/arp_event.vaked` (schema copy), `tools/verify.py` (extract ```vaked fences → `vakedc check`), `.github/workflows/validate.yml`.

- [ ] Create private repo `peterlodri-sec/arp-corpus` via `gh repo create`.
- [ ] Add the skeleton: README (partition convention `corpus/<host>/<date>.md`, MemPalace consumption, pldev write path), `corpus/.gitkeep`, schema copy, `tools/verify.py` (stdlib: walk `corpus/**/*.md`, extract fences, write temp `.vaked`, `python3 -m vakedc check`, exit nonzero on any failure), `validate.yml` (checkout → install vakedc → `python3 tools/verify.py`).
- [ ] Commit + push. Confirm Actions runs green on the empty corpus.

## Task 2: `internal/arp` — verify + sync

**Files:** Create `internal/arp/arp.go`, `internal/arp/arp_test.go`.

- [ ] `DefaultClonePath()` → `~/.local/state/pldev/arp-corpus`; `Host()` → `os.Hostname()` short name. Unit-test path + host-file (`corpus/<host>/<date>.md`) builders.
- [ ] `Verify(ctx, rep, clone)` — run `python3 tools/verify.py` in the clone via `sys.RunInteractive`; report ok/fail.
- [ ] `Sync(ctx, rep, clone)` — `Verify` first (abort on fail); `git -C <clone> add -A`; if `git status --porcelain` is empty → `rep.Have("arp-sync","clean")`, return; else commit (`arp: sync <host> <date>`) + `git push`. Emit each via `internal/report`.
- [ ] Test the pure helpers (paths, commit-message builder, "nothing to commit" decision); orchestration of git/python is integration, exercised live.

## Task 3: `pldev arp` dispatch

**Files:** Modify `main.go`.

- [ ] Add `pldev arp sync` / `pldev arp verify` dispatch (scope `pldev>arp`), before the `setup` branch. Unknown `arp` subcommand → usage.

## Task 4: `setup arp` recipe

**Files:** Create `internal/setup/recipe/arp.go`, `internal/setup/recipe/arp_test.go`; modify `internal/setup/recipe/registry.go` (`Named` adds `"arp"`).

- [ ] `ArpConfig` + `BuiltinArpConfig` (clone path, remote `git@github.com:peterlodri-sec/arp-corpus.git`, `ARP_LOG_PATH` target). Overlay from `recipes.toml` `[arp]`.
- [ ] `BuildArpRecipe(cfg)` steps: `InstallTools{"git+uv"}`; `RunCmd` clone arp-corpus (probe: clone dir exists); install+register the #273 hook via the meta-hook bootstrap with `ARP_LOG_PATH` set; `AppendRC` `export ARP_LOG_PATH=<target>`; point MemPalace mine at the clone; `Verify` (`vakedc check` clone + hook registered).
- [ ] `Named("arp")` returns `BuildArpRecipe(...)`. Test recipe shape/order + config overlay (mirror ruflo tests).

## Task 5: vaked-base `ARP_LOG_PATH` (deferred / cross-repo)

- [ ] In vaked-base, make `.claude/hooks/arp_log.py` honor `ARP_LOG_PATH` (default = current `docs/arp-log.md`). Minimal; its own PR. **Not in this plan's repo** — documented dependency; pldev's `setup arp` sets the env, the hook must read it.

## Verification

- `go build/vet/test ./...` green; recipe + arp helper tests pass.
- arp-corpus Actions green on empty corpus, red on an injected invalid fence.
- `pldev setup arp --dry-run` previews every step, mutates nothing.
- Live (after vakedc local + the vaked-base tweak): a substantial command appends an `arp_event`; `pldev arp verify` exit 0; `pldev arp sync` pushes; MemPalace recalls it.
