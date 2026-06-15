# Fleet Machine Identity — Design (phase B)

**Date:** 2026-06-15
**Subsystem:** `internal/fleet`
**Status:** approved
**Depends on:** phase A (PR #11, fleet hardware fingerprint) — `HWInfo.Serial`, the probe cache, `Node.HW`.

## Motivation

Phase A made hardware visible and flagged serial dups with `⚠dup`, but still keys every operation on the tailnet `Name`. The same physical machine registered twice in tailscale (`dev-main` + `lodris-macbook-pro`, serial `RJ6W340X7F`) therefore still counts twice in `health` (inflated `ssh-ok`) and gets hit twice by `run`/`sync-signers`.

Phase B introduces a **Machine** — one physical box keyed on serial — as the unit of operation. Multiple tailnet nodes sharing a serial collapse into one Machine; operations act once on its canonical reachable alias. Because the tailnet ACL is admin-console-only, pldev cannot delete a ghost registration, so it emits a **prune hint** instead.

## Scope

**In:**
- Group nodes by serial into Machines; serial-less nodes stay singleton Machines.
- Canonical alias selection (online → `primary=true` pin → lexical).
- `Render`/`Select`/`Health`/`Run`/`SyncSigners` operate on Machines.
- Prune hint for stale (unreachable) aliases.

**Out:**
- Deleting tailnet nodes (ACL is admin-console-only — advise only).
- Any change to `Discover`, `Merge`'s tailscale mapping, or the cache format.
- Cross-machine identity beyond serial (no fuzzy/host matching).

## Architecture

### `Machine` (internal/fleet/machine.go — new)

```go
// Machine is one physical box (keyed on hardware serial) and the tailnet nodes
// that resolve to it. A serial-less node forms its own singleton Machine.
type Machine struct {
	Serial    string // "" for singletons (never-probed / no serial)
	Nodes     []Node // all aliases, sorted by Name; len>=1
	canonical int    // index into Nodes of the chosen operational alias
}

func (m Machine) Canonical() Node   // Nodes[canonical]
func (m Machine) Label() string     // Canonical().Label()
func (m Machine) Aliases() []Node   // Nodes minus the canonical (may be empty)
func (m Machine) HasDup() bool      // len(Nodes) > 1 && Serial != ""
```

### `GroupByMachine` (machine.go)

```go
// GroupByMachine collapses nodes sharing a non-empty serial into one Machine and
// leaves serial-less nodes as singletons. Machines are sorted by canonical label.
func GroupByMachine(nodes []Node) []Machine
```

- Group by `HW.Serial`; empty serial → singleton (each its own Machine, `Serial=""`).
- Within a group, `Nodes` sorted by `Name`; `canonical` chosen by `pickCanonical`.

```go
// pickCanonical returns the index of the operational alias:
//   1. prefer Online nodes;
//   2. among the preferred set, one with Primary==true;
//   3. else the lexically-first Name (deterministic).
func pickCanonical(nodes []Node) int
```

### Config + Node (additive, phase-A-safe)

- `config.go`: `NodeConfig` gains `Primary bool \`toml:"primary"\``.
- `fleet.go`: `Node` gains `Primary bool`; `Merge` fills it from `nc.Primary`.

### Render (fleet.go — signature change)

`Render([]Machine, full bool)`:
- One row per machine, using `Canonical()`'s hw, same columns as phase A
  (`NICK HOST ROLE ST CPU CR MEM ARCH`).
- A machine with aliases shows a trailing `+N alias` marker (replaces phase A's
  per-node `⚠dup`).
- `--full`: after the machine row, list each alias indented as
  `    alias <name> <ip> <on|off>`, plus the `CAPS` column on the machine row.

### Select (fleet.go — return type change)

`Select([]Machine, selector) []Machine`:
- `all`/`@all` → every machine.
- `@<role>` → machines whose canonical role matches.
- `<name>` → the machine containing a node whose `Name` or `Nickname` matches the
  selector (case-insensitive) — **including a non-canonical alias name** (so
  selecting the stale name still resolves to the live machine).

### Health / Run / SyncSigners (run.go, sync.go — loop over machines)

- `Health([]Machine)`: probe the **canonical once**; the per-state summary counts
  machines, not registrations. After probing, for each machine with an alias that
  is offline or TCP-unreachable, emit the prune hint (below).
- `Run([]Machine, cmd)`: one exec per machine, on the canonical.
- `SyncSigners([]Machine, ...)`: push the anchor to the canonical once per machine
  (+ local, unchanged).

### Prune hint

Emitted via the reporter to stderr during `health`:
```
stale alias <name> (<ip>) — delete in tailnet admin
```
Triggered for any alias of a multi-node machine that is `!Online` or fails the TCP
:22 probe. Advisory only (pldev cannot edit the ACL).

## Data flow

```
Discover → Merge(ts, cfg, cache) → []Node
         → GroupByMachine          → []Machine
         → Render / Select / Health / Run / SyncSigners  (all act on Canonical)
```

## Error handling

- Serial-less or never-probed nodes → singleton Machines; behavior identical to
  phase A (no regression for an un-probed fleet).
- A machine whose every alias is offline/unreachable → `health` reports it
  unreachable; no canonical operation is attempted.
- `primary=true` set on a node that is offline → rule (1) still prefers an online
  alias over it; the pin only breaks ties among online aliases. (Documented so a
  stale pin can't strand operations on a dead alias.)

## Testing

Pure functions, no live ssh in unit tests:
- `pickCanonical`: single node; all offline (lexical wins); one online (wins);
  two online + one `primary` (pin wins); two online no pin (lexical); offline
  `primary` + online non-primary (online wins — rule 1 beats rule 2).
- `GroupByMachine`: two nodes one serial → one machine; serial-less → singletons;
  mixed; machine sort order by label.
- `Select`: alias name resolves to the machine; nickname; `@role`; `all`.
- prune-hint detection: multi-node machine with an offline alias → hint; all
  reachable → no hint; singleton → no hint.
- `Render`: one row per machine, `+N alias` marker, `--full` alias lines + CAPS.

## Verification

1. `go test ./internal/fleet/...` green.
2. `pldev fleet list` shows ONE row for the MacBook (canonical) with `+1 alias`;
   `--full` lists the stale alias + IP.
3. `pldev fleet health` counts the MacBook once and prints the stale-alias prune
   hint for the offline registration.
4. `pldev fleet run @command -- true` execs once on the MacBook, not twice.
5. `go vet ./...` clean; cross-compiles darwin+linux.
