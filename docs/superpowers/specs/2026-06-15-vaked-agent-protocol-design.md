# vaked agent protocol — validatable swarm messages (design)

## Context

vaked-language thread, forge-converged framing: **vaked as a typed, validatable inter-agent wire format** over the live NATS bus (`Herald`/crabcc-nats). The defensible core of the whole "LLM-Human language" idea is not a new natural language — it is `vakedc`-**checkability**. This framing is where checkability pays: a malformed agent message is **rejected at the boundary**, never delivered, so the swarm can't be poisoned by a garbage payload.

Honesty constraint (drives scope): the agents that would speak this — ruflo — are not live-run yet, so there is **no real vaked traffic** on the bus today. The deliverable is therefore the **protocol design** (spec) + a thin **offline validator** (`pldev fed validate`); the live NATS `watch`/`pub` endpoints are deferred until agents emit traffic.

## Goal

Define a vaked message schema + NATS subject convention + a validate-at-boundary contract, and ship `pldev fed validate <file>` — the offline boundary check (a vaked message → `vakedc check` → valid/rejected) that the future live endpoints will reuse.

## Protocol design

### Message schema (lives in vaked-base, as a builtin)
```
schema agent_msg { ts, from, to, kind, corr_id, payload }
```
- `from` / `to` — agent or fleet-node identity (maps to a fleet nickname where it's a node).
- `kind` — message class (e.g. `task`, `result`, `status`, `query`). The closed set is the protocol's vocabulary; extending it is a deliberate schema change (mirrors how `arp_event` was added in vaked-base #273).
- `corr_id` — correlates a `result` to its `task`.
- `payload` — the kind-specific body.

### Subjects (NATS)
`fed.<from>.<kind>` for emission; agents subscribe to `fed.>` or `fed.*.<kind>`. (Subject layout is a convention here; the live endpoints will pin it.)

### Boundary contract
Every message crossing the boundary is `vakedc check`-validated against `agent_msg` **before** it is delivered/published. Invalid → rejected with the vakedc error, never forwarded. This is the single invariant the whole framing rests on: **nothing malformed reaches an agent.**

## Component (this spec)

`internal/fed/validate.go` + `pldev fed validate <file>`:
- Shell `vakedc check <file>` (same pattern as `pldev verify`/`arp`): injectable runner for tests; `vakedc` absent → actionable error, not a crash.
- On success, emit `valid` via `internal/report`; on failure, surface the `vakedc` error. (Routing preview — mapping `from`/`to` to fleet nicknames — needs a vaked *instance* parser whose syntax isn't pinned here; deferred to the live endpoints, noted not built.)

## Cross-repo dependency

`agent_msg` must be added to vaked-base's builtins (like `arp_event` in #273) for `vakedc check` to know it — until then `fed validate` reports invalid. Documented; the schema decl above is the proposal for that vaked-base change.

## Safety / contract

- `fed validate` is read-only (reads a file, runs a checker). No NATS, no mutation.
- The boundary invariant (validate-before-deliver) is the security property: a typed wire format that rejects malformed input is strictly safer than free-form JSON/prose messaging.

## Deferred (own specs / when agents exist)

- `pldev fed watch <subject>` / `fed pub` over live NATS (shell `nats` CLI or embed nats.go — decision deferred with the live work).
- Routing/nickname preview (needs a vaked instance-field parser).
- `kind` vocabulary expansion + payload sub-schemas per kind.
- ruflo agents actually speaking the protocol (depends on ruflo #2 live-run).

## Verification

1. `go build/vet/test ./...` green; `fed validate` tests pass (runner stubbed: valid file → ok; invalid → surfaced error; vakedc absent → actionable error).
2. `pldev fed validate <good.vaked>` with `vakedc` + the merged `agent_msg` schema → `valid`; a malformed message → rejected with the vakedc error.
3. Spec's `agent_msg` schema is the concrete proposal for the vaked-base change.
