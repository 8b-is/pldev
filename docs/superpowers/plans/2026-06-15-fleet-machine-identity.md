# Fleet Machine Identity Implementation Plan (phase B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse tailnet nodes that share a hardware serial into one logical Machine, and make `list`/`select`/`health`/`run`/`sync-signers` operate once per machine via its canonical reachable alias, with a prune hint for stale registrations.

**Architecture:** A new `internal/fleet/machine.go` adds a `Machine` type (serial + its alias nodes + a canonical index) and `GroupByMachine([]Node) []Machine`. Phase-A plumbing (`Discover`, `Merge`, cache, `HWInfo`) is untouched except an additive `Primary` flag. The render/select/health/run/sync functions switch their loop from `[]Node` to `[]Machine`, acting on `Canonical()`.

**Tech Stack:** Go 1.26, stdlib; builds on phase A (`Node.HW.Serial`, `cpuCode`, `trunc`, `dash`, `sshBase`, `sshOpts`, `classify`, `report`).

**Branch:** This plan targets a branch off `main` AFTER PR #11 (phase A) merges. If #11 is not yet merged, branch off `feat/fleet-hwid` instead so `HWInfo`/`Node.HW`/`cpuCode` exist.

---

## File Structure

- `internal/fleet/machine.go` (new) — `Machine` type + methods, `pickCanonical`, `GroupByMachine`, `staleAliases`.
- `internal/fleet/machine_test.go` (new) — pickCanonical/GroupByMachine/staleAliases tests.
- `internal/fleet/config.go` (modify) — `NodeConfig.Primary`.
- `internal/fleet/fleet.go` (modify) — `Node.Primary`; `Merge` fills it; `Render([]Machine,bool)`; new machine `Select([]Machine,string)`; keep node-level selection as `SelectNodes`.
- `internal/fleet/fleet_test.go` (modify) — update Render/Select calls; add machine-render + select tests.
- `internal/fleet/run.go` (modify) — `Health([]Machine)`, `Run([]Machine,...)` act on canonical; emit prune hints.
- `internal/fleet/run_test.go` (modify if present) — update calls.
- `internal/fleet/sync.go` (modify) — `SyncSigners([]Machine,...)` acts on canonical; `reachableMachines`.
- `internal/fleet/sync_test.go` (modify if present) — update calls.
- `main.go` (modify) — `GroupByMachine` after `Merge`; pass machines to list/health/run/sync/ssh; keep `nodes` for `ssh-config` + `probe-user` (via `SelectNodes`).

---

## Task 1: Add Primary flag to config + Node

**Files:**
- Modify: `internal/fleet/config.go` (NodeConfig)
- Modify: `internal/fleet/fleet.go` (Node struct, Merge)
- Test: `internal/fleet/fleet_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/fleet_test.go`:

```go
func TestMergeFillsPrimary(t *testing.T) {
	ts := []TSNode{{Name: "dev-main", IP: "100.0.0.1", Online: true}}
	cfg := FleetConfig{Node: map[string]NodeConfig{"dev-main": {Primary: true}}}
	nodes := Merge(ts, cfg, nil)
	if len(nodes) != 1 || !nodes[0].Primary {
		t.Fatalf("Merge did not propagate Primary: %+v", nodes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run MergeFillsPrimary -v`
Expected: FAIL — `unknown field Primary in struct literal of type NodeConfig`.

- [ ] **Step 3: Implement**

In `internal/fleet/config.go`, add a field to `NodeConfig` (after `SSHUser`):
```go
	Primary  bool     `toml:"primary"` // tie-breaker: canonical alias when a machine has multiple online registrations
```

In `internal/fleet/fleet.go`, add a field to `Node` (after `Excluded`, alongside the phase-A `HW`/`Dup`/`DupOf`):
```go
	Primary  bool
```

In `Merge`, set it when constructing each node — add `Primary: nc.Primary,` to the `Node{...}` literal (next to `SSHUser: user,`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run MergeFillsPrimary -v && go build ./internal/fleet/`
Expected: PASS, package compiles.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/config.go internal/fleet/fleet.go internal/fleet/fleet_test.go
git commit -m "feat(fleet): add Primary alias pin to config + Node"
```

---

## Task 2: Machine type, canonical pick, grouping

**Files:**
- Create: `internal/fleet/machine.go`
- Test: `internal/fleet/machine_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fleet/machine_test.go`:

```go
package fleet

import "testing"

func TestPickCanonical(t *testing.T) {
	// single node
	if got := pickCanonical([]Node{{Name: "a"}}); got != 0 {
		t.Fatalf("single=%d", got)
	}
	// all offline → lexical first
	off := []Node{{Name: "b", Online: false}, {Name: "a", Online: false}}
	if got := pickCanonical(off); off[got].Name != "a" {
		t.Fatalf("all-offline want a, got %q", off[got].Name)
	}
	// one online beats offline
	mix := []Node{{Name: "a", Online: false}, {Name: "b", Online: true}}
	if got := pickCanonical(mix); mix[got].Name != "b" {
		t.Fatalf("online want b, got %q", mix[got].Name)
	}
	// two online, one primary → primary wins
	pin := []Node{{Name: "a", Online: true}, {Name: "b", Online: true, Primary: true}}
	if got := pickCanonical(pin); pin[got].Name != "b" {
		t.Fatalf("primary want b, got %q", pin[got].Name)
	}
	// two online, no primary → lexical
	two := []Node{{Name: "b", Online: true}, {Name: "a", Online: true}}
	if got := pickCanonical(two); two[got].Name != "a" {
		t.Fatalf("lexical want a, got %q", two[got].Name)
	}
	// offline primary vs online non-primary → online wins (rule 1 beats rule 2)
	stale := []Node{{Name: "a", Online: false, Primary: true}, {Name: "b", Online: true}}
	if got := pickCanonical(stale); stale[got].Name != "b" {
		t.Fatalf("online-over-stale-pin want b, got %q", stale[got].Name)
	}
}

func TestGroupByMachine(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", Online: true, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "warforge", Nickname: "Warforge", Online: true, HW: HWInfo{Serial: "XEON"}},
		{Name: "ghost", Nickname: "Ghost", HW: HWInfo{Serial: ""}},
	}
	ms := GroupByMachine(nodes)
	if len(ms) != 3 {
		t.Fatalf("want 3 machines, got %d", len(ms))
	}
	// find the RJ6W machine
	var mac *Machine
	for i := range ms {
		if ms[i].Serial == "RJ6W" {
			mac = &ms[i]
		}
	}
	if mac == nil || len(mac.Nodes) != 2 || !mac.HasDup() {
		t.Fatalf("RJ6W machine wrong: %+v", mac)
	}
	if mac.Canonical().Name != "dev-main" { // both online, no pin → lexical "dev-main" < "lodris-..."
		t.Fatalf("canonical want dev-main, got %q", mac.Canonical().Name)
	}
	if len(mac.Aliases()) != 1 || mac.Aliases()[0].Name != "lodris-macbook-pro" {
		t.Fatalf("aliases wrong: %+v", mac.Aliases())
	}
}

func TestStaleAliases(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, IP: "100.0.0.1", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "old-reg", Nickname: "Forgemaster", Online: false, IP: "100.0.0.9", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "solo", Online: true, HW: HWInfo{Serial: "X"}},
	}
	ms := GroupByMachine(nodes)
	stale := staleAliases(ms)
	if len(stale) != 1 || stale[0].Name != "old-reg" || stale[0].IP != "100.0.0.9" {
		t.Fatalf("staleAliases wrong: %+v", stale)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'PickCanonical|GroupByMachine|StaleAliases' -v`
Expected: FAIL — undefined: pickCanonical / GroupByMachine / staleAliases / Machine.

- [ ] **Step 3: Implement**

Create `internal/fleet/machine.go`:

```go
package fleet

import "sort"

// Machine is one physical box (keyed on hardware serial) and the tailnet nodes
// that resolve to it. A serial-less node forms its own singleton Machine.
type Machine struct {
	Serial    string // "" for singletons (never-probed / no serial)
	Nodes     []Node // aliases, sorted by Name; len >= 1
	canonical int    // index into Nodes of the operational alias
}

// Canonical is the alias operations act on.
func (m Machine) Canonical() Node { return m.Nodes[m.canonical] }

// Label is the canonical node's display label.
func (m Machine) Label() string { return m.Canonical().Label() }

// Aliases are the non-canonical nodes (may be empty).
func (m Machine) Aliases() []Node {
	var out []Node
	for i, n := range m.Nodes {
		if i != m.canonical {
			out = append(out, n)
		}
	}
	return out
}

// HasDup reports whether this machine has more than one tailnet registration.
func (m Machine) HasDup() bool { return m.Serial != "" && len(m.Nodes) > 1 }

// preferred reports whether a should beat b as the canonical alias:
// online beats offline; then a primary pin wins; then lexically-first name.
func preferred(a, b Node) bool {
	if a.Online != b.Online {
		return a.Online
	}
	if a.Primary != b.Primary {
		return a.Primary
	}
	return a.Name < b.Name
}

// pickCanonical returns the index of the operational alias among nodes.
func pickCanonical(nodes []Node) int {
	pick := 0
	for i := 1; i < len(nodes); i++ {
		if preferred(nodes[i], nodes[pick]) {
			pick = i
		}
	}
	return pick
}

// GroupByMachine collapses nodes sharing a non-empty serial into one Machine and
// leaves serial-less nodes as singletons. Machines are sorted by canonical label.
func GroupByMachine(nodes []Node) []Machine {
	bySerial := map[string][]Node{}
	var order []string
	var singles []Node
	for _, n := range nodes {
		if n.HW.Serial == "" {
			singles = append(singles, n)
			continue
		}
		if _, ok := bySerial[n.HW.Serial]; !ok {
			order = append(order, n.HW.Serial)
		}
		bySerial[n.HW.Serial] = append(bySerial[n.HW.Serial], n)
	}
	var machines []Machine
	for _, s := range order {
		g := bySerial[s]
		sort.Slice(g, func(i, j int) bool { return g[i].Name < g[j].Name })
		machines = append(machines, Machine{Serial: s, Nodes: g, canonical: pickCanonical(g)})
	}
	for _, n := range singles {
		machines = append(machines, Machine{Serial: "", Nodes: []Node{n}, canonical: 0})
	}
	sort.Slice(machines, func(i, j int) bool { return machines[i].Label() < machines[j].Label() })
	return machines
}

// StaleAlias is a non-canonical registration that tailscale reports offline —
// a likely ghost the user should delete in the tailnet admin (pldev can't).
type StaleAlias struct {
	Machine string
	Name    string
	IP      string
}

// staleAliases lists offline aliases of multi-registration machines.
func staleAliases(machines []Machine) []StaleAlias {
	var out []StaleAlias
	for _, m := range machines {
		for _, a := range m.Aliases() {
			if !a.Online {
				out = append(out, StaleAlias{Machine: m.Label(), Name: a.Name, IP: a.IP})
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'PickCanonical|GroupByMachine|StaleAliases' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/machine.go internal/fleet/machine_test.go
git commit -m "feat(fleet): Machine grouping + canonical pick + stale-alias detection"
```

---

## Task 3: Render + Select over machines (keep node-level SelectNodes)

**Files:**
- Modify: `internal/fleet/fleet.go` (Render, Select → rename + new machine Select)
- Test: `internal/fleet/fleet_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/fleet/fleet_test.go`:

```go
func TestRenderMachines(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, IP: "100.0.0.1", Role: "command",
			Caps: []string{"control"}, HW: HWInfo{Serial: "RJ6W", CPU: "Apple M1 Pro", Cores: 8, MemGB: 16, MemFreeGB: 2, Arch: "arm64"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", Online: false, IP: "100.0.0.9", HW: HWInfo{Serial: "RJ6W", CPU: "Apple M1 Pro"}},
	}
	ms := GroupByMachine(nodes)
	out := Render(ms, false)
	if !strings.Contains(out, "M1 Pro") || !strings.Contains(out, "16G/2G") {
		t.Fatalf("missing canonical hw:\n%s", out)
	}
	if !strings.Contains(out, "+1 alias") {
		t.Fatalf("missing alias marker:\n%s", out)
	}
	// non-full must NOT list the alias name
	if strings.Contains(out, "lodris-macbook-pro") {
		t.Fatalf("non-full leaked alias name:\n%s", out)
	}
	full := Render(ms, true)
	if !strings.Contains(full, "CAPS") || !strings.Contains(full, "control") {
		t.Fatalf("full missing caps:\n%s", full)
	}
	if !strings.Contains(full, "alias lodris-macbook-pro") {
		t.Fatalf("full must list alias:\n%s", full)
	}
}

func TestSelectMachineByAlias(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", Online: false, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "warforge", Nickname: "Warforge", Online: true, Role: "hetzner", HW: HWInfo{Serial: "XEON"}},
	}
	ms := GroupByMachine(nodes)
	// selecting the (alias) nickname "Squire" resolves to the RJ6W machine
	got := Select(ms, "Squire")
	if len(got) != 1 || got[0].Serial != "RJ6W" {
		t.Fatalf("alias select failed: %+v", got)
	}
	// @role on canonical
	if r := Select(ms, "@hetzner"); len(r) != 1 || r[0].Canonical().Name != "warforge" {
		t.Fatalf("role select failed: %+v", r)
	}
	if a := Select(ms, "all"); len(a) != 2 {
		t.Fatalf("all should be 2 machines, got %d", len(a))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'RenderMachines|SelectMachineByAlias' -v`
Expected: FAIL — `cannot use ms (...[]Machine) as []Node` / Render signature mismatch.

- [ ] **Step 3: Implement**

In `internal/fleet/fleet.go`:

(a) Rename the EXISTING `Select(nodes []Node, selector string) []Node` to `SelectNodes` (keep its body identical — it stays for `probe-user`). Just change the function name on its declaration line.

(b) Add a new machine-level `Select`:
```go
// Select resolves a selector against machines:
//   - "all"/"@all" → every machine
//   - "@<role>"    → machines whose canonical role matches
//   - "<name>"     → the machine containing a node matching name or nickname
//     (case-insensitive), including a non-canonical alias name.
func Select(machines []Machine, selector string) []Machine {
	switch {
	case selector == "all" || selector == "@all":
		return machines
	case strings.HasPrefix(selector, "@"):
		role := selector[1:]
		var out []Machine
		for _, m := range machines {
			if m.Canonical().Role == role {
				out = append(out, m)
			}
		}
		return out
	default:
		for _, m := range machines {
			for _, n := range m.Nodes {
				if n.Name == selector || strings.EqualFold(n.Nickname, selector) {
					return []Machine{m}
				}
			}
		}
		return nil
	}
}
```

(c) Replace `Render` to take `[]Machine`:
```go
// Render returns the fleet as an aligned wide ASCII table (data → stdout), one
// row per physical machine (its canonical alias). A machine with extra tailnet
// registrations shows a trailing "+N alias"; --full adds CAPS and indented alias
// lines.
func Render(machines []Machine, full bool) string {
	const base = "%-12s %-14s %-8s %-3s %-14s %-3s %-8s %-6s"
	var b strings.Builder
	if full {
		fmt.Fprintf(&b, base+" %s\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH", "CAPS")
	} else {
		fmt.Fprintf(&b, base+"\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH")
	}
	for _, m := range machines {
		n := m.Canonical()
		st := "on"
		if !n.Online {
			st = "off"
		}
		cores := "-"
		if n.HW.Cores > 0 {
			cores = strconv.Itoa(n.HW.Cores)
		}
		mem := "-"
		if n.HW.MemGB > 0 {
			mem = fmt.Sprintf("%dG/%dG", n.HW.MemGB, n.HW.MemFreeGB)
		}
		row := fmt.Sprintf(base,
			trunc(n.Label(), 12), trunc(n.Name, 14), dash(n.Role), st,
			trunc(cpuCode(n.HW.CPU), 14), cores, mem, dash(n.HW.Arch))
		if full {
			row += " " + strings.Join(n.Caps, ",")
		}
		if al := m.Aliases(); len(al) > 0 {
			row += fmt.Sprintf(" +%d alias", len(al))
		}
		b.WriteString(strings.TrimRight(row, " ") + "\n")
		if full {
			for _, a := range m.Aliases() {
				ast := "on"
				if !a.Online {
					ast = "off"
				}
				fmt.Fprintf(&b, "    alias %-14s %-16s %s\n", trunc(a.Name, 14), a.IP, ast)
			}
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Update existing callers in the fleet test files**

Run `cd /Users/peter.lodri/workspace/pldev && grep -rn "Render(\|Select(" internal/fleet/*_test.go`. For each:
- A `Render(nodes, full)` call → wrap nodes: `Render(GroupByMachine(nodes), full)`. (Or, if the test was asserting per-node `⚠dup` from phase A — that test is `TestRenderWideHardware`; update it: change `Render(nodes, false)` → `Render(GroupByMachine(nodes), false)`, and change the `⚠dup` assertion to `+1 alias` since the two M3-Max test nodes share serial. If that test's nodes do NOT share a serial, they become two singleton machines and there is no marker — in that case just assert the hw columns render and drop the dup/alias assertion.)
- A `Select(nodes, sel)` call used for machine-style assertions → `Select(GroupByMachine(nodes), sel)`.
- Any `Select(...)` call inside `probe`-related tests that wants NODES → change to `SelectNodes(...)`.

Verify each compiles. Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -v` after — fix until green.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/fleet.go internal/fleet/fleet_test.go
git commit -m "feat(fleet): render + select operate on machines; SelectNodes kept for probe"
```

---

## Task 4: Health + Run over machines, with prune hint

**Files:**
- Modify: `internal/fleet/run.go` (Health, Run)
- Test: `internal/fleet/run_test.go` (or fleet_test.go if no run_test.go)

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/run_test.go` (create the file with `package fleet` + imports if it doesn't exist):

```go
func TestHealthCountsMachinesOnce(t *testing.T) {
	// two online aliases of one machine must not be probed/counted twice.
	// We assert via staleAliases + GroupByMachine shape here; the live ssh path
	// is covered by manual verification (it needs a real network).
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, IP: "100.0.0.1", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "old-reg", Nickname: "Forgemaster", Online: false, IP: "100.0.0.9", HW: HWInfo{Serial: "RJ6W"}},
	}
	ms := GroupByMachine(nodes)
	if len(ms) != 1 {
		t.Fatalf("want 1 machine, got %d", len(ms))
	}
	if s := staleAliases(ms); len(s) != 1 || s[0].Name != "old-reg" {
		t.Fatalf("expected old-reg as stale: %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run HealthCountsMachinesOnce -v`
Expected: At this point it may already PASS (it only exercises Task-2 helpers). That is fine — its purpose is to lock the grouping contract Health relies on. Proceed to wire Health/Run regardless.

- [ ] **Step 3: Implement — change Health and Run to take `[]Machine`**

In `internal/fleet/run.go`:

Change `Health` signature to `func Health(ctx context.Context, rep *report.Reporter, machines []Machine)`. In the loop, iterate `machines`; inside, use `n := m.Canonical()` everywhere the old code used the node, and emit with `m.Label()`. After the existing `wg.Wait()` and the summary `rep.Info("fleet-health", ...)`, append the prune hints:
```go
	for _, s := range staleAliases(machines) {
		rep.Info("stale-alias",
			report.KV{K: "machine", V: s.Machine},
			report.KV{K: "name", V: s.Name},
			report.KV{K: "ip", V: s.IP},
			report.KV{K: "hint", V: "delete in tailnet admin"})
	}
```
Concretely, the per-item goroutine currently closes over a `Node n`; change the loop to:
```go
	for _, m := range machines {
		n := m.Canonical()
		if !n.Online {
			emit(n, StateOffline)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(label string, n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, derr := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(n.IP, sshPort))
			tcpOK := derr == nil
			if conn != nil {
				conn.Close()
			}
			sshOK := false
			if tcpOK {
				_, serr := sys.Run(ctx, "ssh", append(sshBase(n), "true")...)
				sshOK = serr == nil
			}
			emitLabel(label, classify(true, tcpOK, sshOK))
		}(m.Label(), n)
	}
```
The existing `emit(n Node, state)` helper logs by `n.Label()`. Add a sibling `emitLabel(label string, state string)` that does the same counting + logging but takes an explicit label (so the canonical node logs under the machine label), and have `emit` delegate: `emit := func(n Node, state string){ emitLabel(n.Label(), state) }`. Keep the `counts`/`mu` closure as-is.

Change `Run` signature to `func Run(ctx context.Context, rep *report.Reporter, machines []Machine, cmd []string, dryRun, yes bool) error`. Replace every `for _, n := range nodes` with `for _, m := range machines { n := m.Canonical(); ... }`, using `n` exactly as before (`n.Online`, `sshBase(n)`, `n.Label()` → use `m.Label()`). The dry-run/confirm counts now reflect machines. Use `len(machines)` where the old code used `len(nodes)`.

- [ ] **Step 4: Run tests + build the package**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -v && go build ./internal/fleet/`
Expected: PASS + compiles. (Full `go build ./...` still fails on main.go until Task 6.)

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/run.go internal/fleet/run_test.go
git commit -m "feat(fleet): health + run operate per-machine; stale-alias prune hint"
```

---

## Task 5: SyncSigners over machines

**Files:**
- Modify: `internal/fleet/sync.go` (SyncSigners, reachable)
- Test: `internal/fleet/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/sync_test.go`:

```go
func TestReachableMachines(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Online: true, IP: "100.0.0.1", SSHUser: "lodripeter", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "old-reg", Online: false, IP: "100.0.0.9", SSHUser: "lodripeter", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "down", Online: false, IP: "100.0.0.2", SSHUser: "root", HW: HWInfo{Serial: "Z"}},
	}
	ms := GroupByMachine(nodes)
	r := reachableMachines(ms)
	if len(r) != 1 || r[0].Canonical().Name != "dev-main" {
		t.Fatalf("reachableMachines wrong: %+v", r)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run ReachableMachines -v`
Expected: FAIL — undefined: reachableMachines.

- [ ] **Step 3: Implement**

In `internal/fleet/sync.go`:

Add a machine-level reachability filter (mirrors the existing node `reachable`):
```go
// reachableMachines returns machines whose canonical alias is online with an IP
// and an ssh user (sync targets).
func reachableMachines(machines []Machine) []Machine {
	var out []Machine
	for _, m := range machines {
		n := m.Canonical()
		if n.Online && n.IP != "" && n.SSHUser != "" {
			out = append(out, m)
		}
	}
	return out
}
```

Change `SyncSigners` signature to `func SyncSigners(ctx context.Context, rep *report.Reporter, machines []Machine, anchor []byte, localPath string, dryRun, yes bool) error`. Replace `targets := reachable(nodes)` with `targets := reachableMachines(machines)`. In both the dry-run loop and the live loop, replace `for _, n := range targets` with `for _, m := range targets { n := m.Canonical(); ... }` and use `n.SSHUser`, `n.IP`, `m.Label()` exactly where the old node fields were used. The local-write block is unchanged.

Leave the existing node-level `reachable` function in place if anything else uses it; if `go vet`/compile reports it unused, delete it.

- [ ] **Step 4: Run tests + build the package**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -v && go build ./internal/fleet/`
Expected: PASS + compiles.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/sync.go internal/fleet/sync_test.go
git commit -m "feat(fleet): sync-signers operates per-machine via canonical"
```

---

## Task 6: Wire machines into main.go

**Files:**
- Modify: `main.go` (runFleet + the `fed status` discover block)

- [ ] **Step 1: Update runFleet to group into machines**

In `main.go` `runFleet`, after `nodes := fleet.Merge(ts, cfg, cache)` add:
```go
	machines := fleet.GroupByMachine(nodes)
```
Then update each case:
- `case "", "list":` — `fmt.Print(fleet.Render(machines, full))` (keep the `full` flag parsing from phase A).
- `case "health":` — `fleet.Health(ctx, rep, machines)`.
- `case "ssh":` — `sel := fleet.Select(machines, pos[2])`; the `len(sel) != 1` check stays; `fleet.SSH(ctx, sel[0].Canonical())`.
- `case "run":` — `sel := fleet.Select(machines, pos[2])`; `fleet.Run(ctx, rep, sel, pos[dd+1:], dryRun, yes)`.
- `case "sync-signers":` — `fleet.SyncSigners(ctx, rep, machines, verify.AnchorBytes(), verify.DefaultAnchorPath(), dryRun, yes)`.
- `case "probe-user":` — KEEP node-level: `sel := nodes; if len(pos) >= 3 { sel = fleet.SelectNodes(nodes, pos[2]) }; fleet.ProbeUsers(ctx, rep, sel, nil, fleet.DefaultCachePath())`.
- `case "ssh-config":` — KEEP `fleet.GenSSHConfig(nodes)` (aliases should all stay sshable by name).

In the `fed status` discover block (~line 119-125) that also calls `fleet.Merge`, no machine grouping is needed (it only maps IPs→labels); leave it using `nodes`.

- [ ] **Step 2: Build + vet + full test**

Run: `cd /Users/peter.lodri/workspace/pldev && go build ./... && go vet ./... && go test ./...`
Expected: clean build, clean vet, all tests pass.

- [ ] **Step 3: Manual live verification**

```bash
cd /Users/peter.lodri/workspace/pldev
go build -o pldev .
./pldev fleet list                 # ONE row for the MacBook with "+1 alias"
./pldev fleet list --full          # alias line for the duplicate registration
./pldev fleet health 2>&1 | grep -i 'stale-alias\|ssh-ok'   # machine counted once; prune hint if a dup is offline
./pldev fleet run @command -- true # execs once per command machine
```
Expected: the MacBook appears once; `ssh-ok` counts physical machines; a stale/offline alias prints the `delete in tailnet admin` hint. Paste `fleet list` output in the report.

- [ ] **Step 4: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add main.go
git commit -m "feat(fleet): wire machine grouping into fleet dispatch"
```

---

## Cross-compile check (final)

- [ ] Run: `cd /Users/peter.lodri/workspace/pldev && GOOS=linux GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./...`
  Expected: both succeed.

---

## Self-review notes (addressed)

- Spec coverage: Machine + GroupByMachine (T2), canonical pick rule incl. stale-pin edge (T2 test), Primary config (T1), Render machine rows + alias marker + --full alias lines (T3), Select alias→machine + SelectNodes kept for probe (T3), Health/Run per-machine + prune hint (T4), SyncSigners per-machine (T5), main wiring + ssh-config/probe-user left node-level (T6). All covered.
- Type consistency: `Render([]Machine,bool)`, `Select([]Machine,string)[]Machine`, `SelectNodes([]Node,string)[]Node`, `Health([]Machine)`, `Run([]Machine,...)`, `SyncSigners([]Machine,...)`, `reachableMachines([]Machine)` — consistent across tasks and the main wiring.
- `fleet.SSH` still takes a `Node` — main passes `sel[0].Canonical()` (T6), no signature change needed.
