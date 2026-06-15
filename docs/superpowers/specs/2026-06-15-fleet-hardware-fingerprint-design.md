# Fleet Hardware Fingerprint — Design

**Date:** 2026-06-15
**Subsystem:** `internal/fleet`
**Status:** approved (phase A)

## Motivation

The fleet keys every node on its tailnet `Name`. A re-auth/reinstall can leave a
machine registered twice (e.g. `dev-main` at `100.92.65.95` and
`lodris-macbook-pro` at `100.117.70.42` are the **same** MacBookPro18,3, serial
`RJ6W340X7F`). The fleet counted them as two ssh-ok nodes — a silent
double-count, invisible until someone compared serials by hand.

Make hardware identity a first-class, visible property: capture each node's
serial/model/cpu/cores/mem/arch, surface it in `fleet list`, and flag nodes that
share a serial as `⚠dup`. This is **display + detection only** (phase A); the
tailnet `Name` remains pldev's internal key.

## Scope

**In (phase A):**
- Gather hardware info per node via the existing `probe-user` ssh round-trip.
- Persist it to a cache file.
- Render a wide `fleet list` with hardware columns + `⚠dup` marker.
- Detect dups by shared serial (pure function, unit-tested).

**Out (phase B, near-term follow-up — NOT this spec):**
- Serial becomes pldev's canonical node identity.
- Collapse multiple tailnet nodes that share a serial into one logical machine.
- UI showing a machine with its tailnet aliases nested under it.
- Touches `Merge`/`Select`/`Render`/health/sync — everything keyed on `Name` today.

## Architecture

### `HWInfo` (internal/fleet/hwid.go — new)

```go
type HWInfo struct {
    Serial string // dup key; falls back to /etc/machine-id on linux when DMI is root-only
    Model  string // "MacBookPro18,3", "Mac15,11", linux product_name
    CPU    string // chip marketing/model name, stored full, displayed short
    Cores  int    // physical cores
    MemGB  int    // total RAM, rounded to GB
    Arch   string // normalized: arm64 | amd64 | <raw>
}
```

Helpers (all pure, table-tested):
- `gatherScript() string` — one POSIX `sh` script that branches on `uname -s`
  (Darwin vs other) and emits `key=val` lines.
- `parseHW(out string) HWInfo` — parse `key=val` lines into the struct; missing
  keys leave zero values.
- `normArch(raw string) string` — `aarch64|arm64→arm64`, `x86_64|amd64→amd64`,
  else raw.
- `normMemKB`/`normMemBytes → MemGB` — darwin emits bytes (`hw.memsize`), linux
  emits kB (`MemTotal`); both round to nearest GB.
- `shortCPU(full string) string` — trim vendor noise for display
  ("Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz" → "Xeon E5-2680",
  "Apple M3 Max" → unchanged).

### Gather script (emitted by `gatherScript`)

```sh
os=$(uname -s)
if [ "$os" = "Darwin" ]; then
  echo "model=$(sysctl -n hw.model 2>/dev/null)"
  echo "cpu=$(sysctl -n machdep.cpu.brand_string 2>/dev/null)"
  echo "cores=$(sysctl -n hw.physicalcpu 2>/dev/null)"
  echo "membytes=$(sysctl -n hw.memsize 2>/dev/null)"
  echo "arch=$(uname -m)"
  echo "serial=$(ioreg -rd1 -c IOPlatformExpertDevice | awk -F'\"' '/IOPlatformSerialNumber/{print $4}')"
else
  echo "model=$(cat /sys/class/dmi/id/product_name 2>/dev/null)"
  echo "cpu=$(awk -F: '/model name/{print $2; exit}' /proc/cpuinfo | sed 's/^ *//')"
  echo "cores=$(nproc 2>/dev/null)"
  echo "memkb=$(awk '/MemTotal/{print $2}' /proc/meminfo)"
  echo "arch=$(uname -m)"
  echo "serial=$(cat /sys/class/dmi/id/product_uuid 2>/dev/null || cat /etc/machine-id 2>/dev/null)"
fi
```

`parseHW` reads either `membytes` (darwin) or `memkb` (linux) into `MemGB`.

### Cache (internal/fleet/cache.go — new)

```go
type NodeCache struct {
    SSHUser string `json:"ssh_user,omitempty"`
    HW      HWInfo `json:"hw,omitempty"`
}
type FleetCache map[string]NodeCache // keyed by tailnet Name

func LoadCache(path string) (FleetCache, error) // missing file → empty cache, no error
func (c FleetCache) Save(path string) error     // 0o644, mkdir -p dir
func DefaultCachePath() string                  // ~/.config/pldev/fleet-cache.json
```

`probe-user` currently only prints a fleet.toml snippet to stdout. It now also
writes/updates this cache. The snippet behavior is unchanged.

### Node + dup detection (internal/fleet/fleet.go — edit)

`Node` gains:
```go
HW    HWInfo
Dup   bool
DupOf string // label of the first node seen with the same serial
```

`Merge` fills `Node.HW` from the cache (blank `HWInfo` if the node was never
probed). After building the slice, it calls `detectDups`.

```go
// detectDups marks every node that shares a non-empty Serial with another node.
// The first node (sorted order) keeps Dup=false; later ones get Dup=true and
// DupOf=<first node's Label()>.
func detectDups(nodes []Node) // mutates in place
```

Empty serials never match each other (a node that failed to probe is not a dup).

### Render (internal/fleet/fleet.go — edit)

Wide table replacing the current one:

```
NICK         HOST          ROLE     ST   CPU           CR  MEM  ARCH   CAPS
Forgemaster  dev-main      command  on   Apple M3 Max  16  64G  arm64  control,nix
Squire       lodris-mac..  command  on   Apple M3 Max  16  64G  arm64  -  ⚠dup
Warforge     crabcc-ccx33  hetzner  on   Xeon E5-2680   4  32G  amd64  docker
```

- `ST` = `on`/`off` (online/offline — unchanged semantics; list does not probe health).
- `CPU` = `shortCPU(HW.CPU)`; blank → `-`.
- `CR` = cores; `MEM` = `<n>G`; `ARCH` = normalized; all blank → `-`.
- `⚠dup` appended to the row when `Node.Dup`.
- HOST truncated to the column width with `..` as today's columns do.

### Dispatch (main.go — edit)

No new subcommand (wide table chosen over a separate `fleet hw` view). `fleet
list` reads the cache through `Merge`. `probe-user` writes the cache.

## Data flow

```
pldev fleet probe-user
  → Discover (tailscale LocalClient)        # names, IPs, online
  → for each reachable node: ssh probe
       ssh_user candidates  → SSHUser
       gatherScript output  → parseHW → HWInfo
  → FleetCache{name: {SSHUser, HW}}.Save(cache path)

pldev fleet list
  → Discover
  → LoadCache
  → Merge(ts, cfg, cache)  → []Node{… HW …}
  → detectDups(nodes)
  → Render → stdout
```

## Error handling

- Gather failures (ssh error, missing tool) → that node keeps a blank `HWInfo`;
  never aborts the probe of other nodes. Logged via the reporter as today.
- Missing cache file → empty cache, `list` renders `-` for hw columns.
- Unreadable linux serial (DMI root-only) → `/etc/machine-id` fallback; still a
  stable per-machine token for dup detection.
- A node probed on one OS, parsed for the other → `parseHW` simply leaves the
  absent keys zero; no panic.

## Testing

Pure functions, no live ssh in unit tests (gather is injectable like
`trySSHUser`):
- `parseHW`: darwin sample, linux sample, missing-field sample, both mem units.
- `detectDups`: no dups, one dup pair, three sharing a serial, empty serials
  (must NOT match), preserves first-seen as canonical.
- `normArch`, `normMemKB`/`normMemBytes`, `shortCPU`: table tests incl. edge inputs.
- Cache: `Save`→`LoadCache` round-trip, missing file → empty, partial entries.
- `Render`: column alignment with/without hw, `⚠dup` marker present only on dups.

## Verification

1. `go test ./internal/fleet/...` green.
2. `pldev fleet probe-user` writes `~/.config/pldev/fleet-cache.json` with hw per
   reachable node.
3. `pldev fleet list` shows CPU/CR/MEM/ARCH columns; Forgemaster + Squire both
   render the MacBookPro18,3 hardware and Squire carries `⚠dup`.
4. `go vet ./...` clean; cross-compiles darwin+linux.
