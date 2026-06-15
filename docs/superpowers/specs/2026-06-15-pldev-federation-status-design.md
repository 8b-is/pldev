# pldev fed status — federation observability (design)

## Context

The fleet substrate (`pldev fleet`) and the `Herald`/crabcc-nats bus are live. The next federation step (user-chosen): a **read-only status/topology view** — who is connected to the swarm — *before* any enablement. ruflo-federation coordination + ruvllm routing layer on later.

Grounding: NATS exposes a stable **monitoring HTTP endpoint** on `:8222`. Confirmed live against the Herald: `GET /varz` → server info (name/version/uptime/connections/subscriptions); `GET /connz` → connected clients (ip/name/subscriptions). This is the data source — no dependence on the (unknown-to-me) ruflo-federation plugin internals. As of probe: Herald = crabcc-nats v2.14.2, 9d uptime, **0 client connections** (federation not running; the view populates once agents connect).

## Goal

`pldev fed status` — fetch the bus's NATS monitoring data over tailscale, map connected clients back to **fleet node nicknames**, and render the swarm's current state (server health + who's on the bus). Read-only, safe.

## Architecture

`internal/fed/`:

- **status.go**
  - `Varz` / `Connz` structs — the subset of NATS monitoring JSON used (varz: `server_name`, `version`, `connections`, `subscriptions`, `uptime`; connz: `num_connections`, `connections[]{ip, name, cid, subscriptions, uptime}`).
  - `FetchVarz(ctx, base)` / `FetchConnz(ctx, base)` — HTTP GET `base/varz`, `base/connz` (4s timeout).
  - `BusURL(nodes []fleet.Node, override string)` — `override` if set, else the fleet node with `role=="bus"` → `http://<ip>:8222`. Error if neither resolves.
  - `Status(ctx, rep, nodes, base)` — fetch varz+connz; render a server summary line + a connections table mapping each client `ip` → fleet nickname (via `nodes`), to **stdout** (data); emit a `report` status line (ok/fail + counts).
- **config:** optional `[federation] monitor_url` in `fleet.toml` overrides the derived bus URL; otherwise derived from `role="bus"` (the Herald). The bus node is already `role="bus"` in the themed fleet.toml.

`main.go`: `pldev fed status` → load fleet (Discover + config + Merge), resolve bus URL, `fed.Status`.

## Rendering

```
bus: crabcc-nats (Herald)  v2.14.2  up 9d22h  conns=0  subs=83   [reachable]

CLIENT             NAME                 SUBS  UPTIME
(no clients connected — federation not running)
```
When clients exist: one row per connection, `CLIENT` = fleet nickname for the IP (or the raw IP if unmapped), plus name/subs/uptime.

## Safety / contract

- Read-only: only HTTP GETs to the monitoring endpoint; no writes, no mutation.
- Bus unreachable → clear error (`fed status: Herald monitoring unreachable at <url>`), not a crash. Emits through `internal/report`.
- No secrets: the monitoring endpoint exposes connection metadata only; pldev prints client names/IPs (mapped to nicknames), nothing sensitive.

## Verifiable now vs deferred

- **Now:** varz/connz JSON parse (fixtures), `BusURL` resolution (role=bus + override), IP→nickname mapping, render — unit-tested. Live-smoke `pldev fed status` against the Herald (shows v2.14.2, 0 conns today).
- **Deferred (own specs):** ruflo-federation *enablement* (install plugin, wire agents to the bus) and ruvllm→Ollama routing. This spec only *observes*; once federation runs, the same view shows the agents with no code change.

## Verification

1. `go build/vet/test ./...` green; fed unit tests pass (parse/BusURL/mapping/render).
2. `pldev fed status` live → prints Herald server summary + `conns=0` (today), `[reachable]`.
3. Bus URL derives from the `role="bus"` fleet node (Herald) with no config; `[federation] monitor_url` override honored.
4. A connz fixture with 2 clients whose IPs match fleet nodes renders their nicknames.
