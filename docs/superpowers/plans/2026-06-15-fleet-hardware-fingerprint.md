# Fleet Hardware Fingerprint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture each fleet node's hardware (serial/model/cpu/cores/mem/arch) via the existing `probe-user` ssh probe, cache it, render a wide `fleet list` with hardware columns, and flag nodes sharing a serial as `⚠dup`.

**Architecture:** Two new files in `internal/fleet` (`hwid.go` for the hardware struct + parsing, `cache.go` for the JSON cache). `probe-user` gathers hardware in the same ssh round-trip and writes the cache; `Merge` fills `Node.HW` from the cache and `detectDups` flags shared serials; `Render` shows a wide table. Tailnet `Name` stays the internal key (phase A).

**Tech Stack:** Go 1.26, stdlib `encoding/json`, existing `internal/sys` ssh runner + `internal/report`.

---

## File Structure

- `internal/fleet/hwid.go` (new) — `HWInfo` struct, `gatherScript`, `parseHW`, `normArch`, `normMemBytes`, `normMemKB`, `shortCPU`, `gatherHW` (ssh, injectable).
- `internal/fleet/hwid_test.go` (new) — parse/normalize/shortCPU table tests.
- `internal/fleet/cache.go` (new) — `NodeCache`, `FleetCache`, `LoadCache`, `Save`, `DefaultCachePath`.
- `internal/fleet/cache_test.go` (new) — round-trip + missing-file tests.
- `internal/fleet/fleet.go` (modify) — `Node` gains `HW`/`Dup`/`DupOf`; `Merge` gains a cache param; new `detectDups`; `Render` rewritten wide + `trunc` helper.
- `internal/fleet/fleet_test.go` (modify) — update `Merge` calls; add `detectDups` + `Render` tests.
- `internal/fleet/probe.go` (modify) — `ProbeResult` gains `HW`; goroutine gathers hw; `ProbeUsers` gains `cachePath` and writes the cache.
- `internal/fleet/probe_test.go` (modify) — update `ProbeUsers` calls; stub `gatherHW`.
- `main.go` (modify) — two `Merge` callsites load the cache; `ProbeUsers` callsite passes `DefaultCachePath()`.

---

## Task 1: HWInfo struct, parsing, and normalizers

**Files:**
- Create: `internal/fleet/hwid.go`
- Test: `internal/fleet/hwid_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fleet/hwid_test.go`:

```go
package fleet

import "testing"

func TestParseHW_Darwin(t *testing.T) {
	out := "model=Mac15,11\ncpu=Apple M3 Max\ncores=16\nmembytes=68719476736\narch=arm64\nserial=HFK99M239G\n"
	hw := parseHW(out)
	if hw.Model != "Mac15,11" || hw.CPU != "Apple M3 Max" || hw.Cores != 16 ||
		hw.MemGB != 64 || hw.Arch != "arm64" || hw.Serial != "HFK99M239G" {
		t.Fatalf("got %+v", hw)
	}
}

func TestParseHW_Linux(t *testing.T) {
	out := "model=Standard PC\ncpu=Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\ncores=4\nmemkb=32827840\narch=x86_64\nserial=abc-123\n"
	hw := parseHW(out)
	if hw.Cores != 4 || hw.MemGB != 31 || hw.Arch != "amd64" || hw.Serial != "abc-123" {
		t.Fatalf("got %+v", hw)
	}
	if hw.CPU != "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz" {
		t.Fatalf("cpu stored raw, got %q", hw.CPU)
	}
}

func TestParseHW_MissingFields(t *testing.T) {
	hw := parseHW("arch=arm64\n")
	if hw.Arch != "arm64" || hw.Cores != 0 || hw.MemGB != 0 || hw.Serial != "" {
		t.Fatalf("got %+v", hw)
	}
}

func TestNormArch(t *testing.T) {
	cases := map[string]string{"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64", "amd64": "amd64", "riscv64": "riscv64", "": ""}
	for in, want := range cases {
		if got := normArch(in); got != want {
			t.Errorf("normArch(%q)=%q want %q", in, got, want)
		}
	}
}

func TestShortCPU(t *testing.T) {
	cases := map[string]string{
		"Apple M3 Max": "Apple M3 Max",
		"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz": "Xeon E5-2680 v4",
		"AMD EPYC 7282 16-Core Processor":           "EPYC 7282 16-Core Processor",
		"":                                          "",
	}
	for in, want := range cases {
		if got := shortCPU(in); got != want {
			t.Errorf("shortCPU(%q)=%q want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'ParseHW|NormArch|ShortCPU' -v`
Expected: FAIL — `undefined: parseHW` etc.

- [ ] **Step 3: Write the implementation**

Create `internal/fleet/hwid.go`:

```go
package fleet

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// HWInfo is a node's hardware fingerprint, gathered over ssh and cached.
type HWInfo struct {
	Serial string `json:"serial,omitempty"` // dup key; linux falls back to /etc/machine-id
	Model  string `json:"model,omitempty"`  // "Mac15,11", linux product_name
	CPU    string `json:"cpu,omitempty"`    // chip name, stored raw; shortened at render
	Cores  int    `json:"cores,omitempty"`  // physical cores
	MemGB  int    `json:"mem_gb,omitempty"` // total RAM rounded to GB
	Arch   string `json:"arch,omitempty"`   // normalized arm64|amd64|<raw>
}

// gatherScript is one POSIX sh script that emits key=val lines for either OS.
func gatherScript() string {
	return `os=$(uname -s)
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
fi`
}

// parseHW turns key=val lines (from gatherScript) into an HWInfo.
func parseHW(out string) HWInfo {
	var hw HWInfo
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "model":
			hw.Model = v
		case "cpu":
			hw.CPU = v
		case "cores":
			hw.Cores, _ = strconv.Atoi(v)
		case "membytes":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				hw.MemGB = normMemBytes(n)
			}
		case "memkb":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				hw.MemGB = normMemKB(n)
			}
		case "arch":
			hw.Arch = normArch(v)
		case "serial":
			hw.Serial = v
		}
	}
	return hw
}

func normArch(raw string) string {
	switch raw {
	case "aarch64", "arm64":
		return "arm64"
	case "x86_64", "amd64":
		return "amd64"
	default:
		return raw
	}
}

func normMemBytes(b int64) int { return int(math.Round(float64(b) / (1 << 30))) }
func normMemKB(kb int64) int   { return int(math.Round(float64(kb) / (1 << 20))) }

// shortCPU trims vendor noise for display: drops (R)/(TM), the word Intel/AMD/CPU,
// and anything after an "@" clock suffix; collapses runs of spaces.
func shortCPU(full string) string {
	if full == "" {
		return ""
	}
	s := full
	for _, junk := range []string{"(R)", "(r)", "(TM)", "(tm)"} {
		s = strings.ReplaceAll(s, junk, "")
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	var keep []string
	for _, w := range strings.Fields(s) {
		switch w {
		case "Intel", "AMD", "CPU", "Processor", "(R)":
			continue
		}
		keep = append(keep, w)
	}
	return strings.Join(keep, " ")
}

// gatherHW runs gatherScript over ssh and parses the result. Injectable for tests.
var gatherHW = func(ctx context.Context, user, ip string) (HWInfo, error) {
	args := append(append([]string{}, sshOpts...), user+"@"+ip, gatherScript())
	out, err := sys.Run(ctx, "ssh", args...)
	if err != nil {
		return HWInfo{}, err
	}
	return parseHW(out), nil
}
```

Note on `TestShortCPU`: `"AMD EPYC 7282 16-Core Processor"` → drop `AMD` and `Processor` → `"EPYC 7282 16-Core"`. Update the expected value in the test to `"EPYC 7282 16-Core"` before running (the dropped-word set is `Intel/AMD/CPU/Processor`).

- [ ] **Step 4: Fix the test expectation, run tests to verify they pass**

Edit `hwid_test.go`: change the EPYC expected value to `"EPYC 7282 16-Core"`.
Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'ParseHW|NormArch|ShortCPU' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/hwid.go internal/fleet/hwid_test.go
git commit -m "feat(fleet): HWInfo gather + parse + normalizers"
```

---

## Task 2: Fleet cache (JSON load/save)

**Files:**
- Create: `internal/fleet/cache.go`
- Test: `internal/fleet/cache_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fleet/cache_test.go`:

```go
package fleet

import (
	"path/filepath"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fleet-cache.json")
	c := FleetCache{
		"dev-main": {SSHUser: "lodripeter", HW: HWInfo{Serial: "RJ6W340X7F", Model: "MacBookPro18,3", CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}},
	}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["dev-main"].HW.Serial != "RJ6W340X7F" || got["dev-main"].SSHUser != "lodripeter" {
		t.Fatalf("round-trip lost data: %+v", got)
	}
}

func TestLoadCacheMissingFile(t *testing.T) {
	got, err := LoadCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("missing file → empty non-nil cache, got %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run Cache -v`
Expected: FAIL — `undefined: FleetCache`.

- [ ] **Step 3: Write the implementation**

Create `internal/fleet/cache.go`:

```go
package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// NodeCache is the per-node persisted probe result.
type NodeCache struct {
	SSHUser string `json:"ssh_user,omitempty"`
	HW      HWInfo `json:"hw,omitempty"`
}

// FleetCache maps a tailnet Name to its cached probe result.
type FleetCache map[string]NodeCache

// DefaultCachePath is where the fleet probe cache lives.
func DefaultCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "fleet-cache.json")
}

// LoadCache reads the cache; a missing file yields an empty (non-nil) cache.
func LoadCache(path string) (FleetCache, error) {
	c := FleetCache{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if len(b) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return FleetCache{}, err
	}
	if c == nil {
		c = FleetCache{}
	}
	return c, nil
}

// Save writes the cache as indented JSON, creating the parent dir.
func (c FleetCache) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run Cache -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/cache.go internal/fleet/cache_test.go
git commit -m "feat(fleet): JSON probe cache (ssh_user + hw)"
```

---

## Task 3: Node hardware fields, dup detection, Merge cache param

**Files:**
- Modify: `internal/fleet/fleet.go:10-22` (Node struct), `:34-56` (Merge)
- Modify: `internal/fleet/fleet_test.go` (existing Merge calls)
- Test: `internal/fleet/fleet_test.go` (new detectDups tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/fleet_test.go`:

```go
func TestDetectDups(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "crabcc", Nickname: "Warforge", HW: HWInfo{Serial: "XEON1"}},
		{Name: "no-probe", Nickname: "Ghost", HW: HWInfo{Serial: ""}},
		{Name: "no-probe-2", Nickname: "Ghost2", HW: HWInfo{Serial: ""}},
	}
	detectDups(nodes)
	if nodes[0].Dup {
		t.Error("first node with a serial must not be marked dup")
	}
	if !nodes[1].Dup || nodes[1].DupOf != "Forgemaster" {
		t.Errorf("Squire should be dup of Forgemaster, got Dup=%v DupOf=%q", nodes[1].Dup, nodes[1].DupOf)
	}
	if nodes[2].Dup {
		t.Error("unique serial must not be dup")
	}
	if nodes[3].Dup || nodes[4].Dup {
		t.Error("empty serials must never match each other")
	}
}

func TestMergeFillsHWFromCache(t *testing.T) {
	ts := []TSNode{{Name: "dev-main", IP: "100.0.0.1", Online: true}}
	cfg := FleetConfig{Node: map[string]NodeConfig{"dev-main": {Nickname: "Forgemaster"}}}
	cache := FleetCache{"dev-main": {HW: HWInfo{Serial: "RJ6W", CPU: "Apple M3 Max", Cores: 16}}}
	nodes := Merge(ts, cfg, cache)
	if len(nodes) != 1 || nodes[0].HW.CPU != "Apple M3 Max" || nodes[0].HW.Cores != 16 {
		t.Fatalf("Merge did not fill HW from cache: %+v", nodes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'DetectDups|MergeFillsHW' -v`
Expected: FAIL — `Node has no field Dup` / `too few arguments to Merge`.

- [ ] **Step 3: Implement — extend Node, change Merge, add detectDups**

In `internal/fleet/fleet.go`, replace the `Node` struct (lines 10-22) with:

```go
// Node is a discovered node enriched with config annotations + cached hardware.
type Node struct {
	Name     string
	Nickname string
	IP       string
	OS       string
	Online   bool
	Tags     []string
	Role     string
	Caps     []string
	SSHUser  string
	Excluded bool
	HW       HWInfo
	Dup      bool   // shares a serial with an earlier node
	DupOf    string // label of that earlier node
}
```

Replace `Merge` (lines 34-56) with:

```go
// Merge annotates tailscale nodes with config + cached hardware, drops excluded
// nodes, fills ssh_user (per-node → default → $USER), then flags serial dups.
func Merge(ts []TSNode, cfg FleetConfig, cache FleetCache) []Node {
	def := cfg.DefaultSSHUser
	if def == "" {
		def = os.Getenv("USER")
	}
	var out []Node
	for _, t := range ts {
		nc := cfg.Node[t.Name]
		if nc.Exclude {
			continue
		}
		user := nc.SSHUser
		if user == "" {
			user = def
		}
		out = append(out, Node{
			Name: t.Name, Nickname: nc.Nickname, IP: t.IP, OS: t.OS, Online: t.Online, Tags: t.Tags,
			Role: nc.Role, Caps: nc.Caps, SSHUser: user, HW: cache[t.Name].HW,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label() < out[j].Label() })
	detectDups(out)
	return out
}

// detectDups marks every node that shares a non-empty Serial with an earlier
// (sorted-order) node. The first keeps Dup=false; later ones get Dup=true and
// DupOf set to the first node's label. Empty serials never match.
func detectDups(nodes []Node) {
	firstBySerial := map[string]string{}
	for i := range nodes {
		s := nodes[i].HW.Serial
		if s == "" {
			continue
		}
		if first, ok := firstBySerial[s]; ok {
			nodes[i].Dup = true
			nodes[i].DupOf = first
		} else {
			firstBySerial[s] = nodes[i].Label()
		}
	}
}
```

- [ ] **Step 4: Update existing Merge callsites in the test file**

Search `internal/fleet/fleet_test.go` for existing `Merge(` calls and add a third arg `nil` (a nil `FleetCache` is safe — map reads return the zero `NodeCache`). Example:
```go
nodes := Merge(ts, cfg, nil)
```
Run: `cd /Users/peter.lodri/workspace/pldev && grep -n "Merge(" internal/fleet/*_test.go` and fix each to pass `nil` unless it already passes a cache.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'DetectDups|MergeFillsHW|Merge' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/fleet.go internal/fleet/fleet_test.go
git commit -m "feat(fleet): Node hardware fields + serial dup detection"
```

---

## Task 4: Wide Render with hardware columns + dup marker

**Files:**
- Modify: `internal/fleet/fleet.go:86-104` (Render)
- Test: `internal/fleet/fleet_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/fleet_test.go`:

```go
func TestRenderWideHardware(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", OS: "macOS", Online: true, Role: "command",
			Caps: []string{"control"}, HW: HWInfo{CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", OS: "macOS", Online: true,
			HW: HWInfo{CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}, Dup: true, DupOf: "Forgemaster"},
		{Name: "blank", Nickname: "Blank", Online: false},
	}
	out := Render(nodes)
	if !strings.Contains(out, "NICK") || !strings.Contains(out, "CPU") || !strings.Contains(out, "ARCH") {
		t.Fatalf("missing header columns:\n%s", out)
	}
	if !strings.Contains(out, "M3 Max") || !strings.Contains(out, "64G") {
		t.Fatalf("missing hw values:\n%s", out)
	}
	if !strings.Contains(out, "⚠dup") {
		t.Fatalf("dup marker absent:\n%s", out)
	}
	// blank node renders dashes, not "0G"
	if strings.Contains(out, "0G") {
		t.Fatalf("zero mem must render '-', not 0G:\n%s", out)
	}
}

func TestTrunc(t *testing.T) {
	if got := trunc("lodris-macbook-pro", 12); got != "lodris-mac.." {
		t.Fatalf("trunc=%q", got)
	}
	if got := trunc("short", 12); got != "short" {
		t.Fatalf("trunc must leave short strings: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'RenderWide|Trunc' -v`
Expected: FAIL — `undefined: trunc` and/or missing columns.

- [ ] **Step 3: Implement — rewrite Render, add trunc**

Replace `Render` (lines 86-104) in `internal/fleet/fleet.go` with:

```go
// trunc shortens s to n runes, ending with ".." when cut.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 2 {
		return s[:n]
	}
	return s[:n-2] + ".."
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Render returns the fleet as an aligned wide ASCII table (data → stdout):
// nickname, tailnet host, role, on/off, cpu, cores, mem, arch, caps, ⚠dup.
func Render(nodes []Node) string {
	const f = "%-12s %-14s %-8s %-3s %-14s %-3s %-5s %-6s %s\n"
	var b strings.Builder
	fmt.Fprintf(&b, f, "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH", "CAPS")
	for _, n := range nodes {
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
			mem = strconv.Itoa(n.HW.MemGB) + "G"
		}
		caps := strings.Join(n.Caps, ",")
		if n.Dup {
			if caps == "" {
				caps = "⚠dup"
			} else {
				caps += " ⚠dup"
			}
		}
		fmt.Fprintf(&b, f,
			trunc(n.Label(), 12), trunc(n.Name, 14), dash(n.Role), st,
			trunc(shortCPU(n.HW.CPU), 14), cores, mem, dash(n.HW.Arch), caps)
	}
	return b.String()
}
```

Add `"strconv"` to the `fleet.go` import block (it is not yet imported there).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run 'RenderWide|Trunc' -v`
Expected: PASS.

If an existing `Render` test asserts the OLD header (`HOST` width 22, `STATUS`, `online`), update it to match the new wide format or delete it in favor of `TestRenderWideHardware`. Run `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run Render -v` and fix any stale assertion.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/fleet.go internal/fleet/fleet_test.go
git commit -m "feat(fleet): wide list render with hardware columns + dup marker"
```

---

## Task 5: probe-user gathers hardware and writes the cache

**Files:**
- Modify: `internal/fleet/probe.go:24-28` (ProbeResult), `:53-96` (ProbeUsers)
- Modify: `internal/fleet/probe_test.go` (existing ProbeUsers calls + stub gatherHW)

- [ ] **Step 1: Write the failing test**

Add to `internal/fleet/probe_test.go` (keep existing imports; add `path/filepath` if absent):

```go
func TestProbeUsersWritesCache(t *testing.T) {
	origTry, origGather := trySSHUser, gatherHW
	defer func() { trySSHUser, gatherHW = origTry, origGather }()
	trySSHUser = func(ctx context.Context, user, ip string) bool { return user == "root" }
	gatherHW = func(ctx context.Context, user, ip string) (HWInfo, error) {
		return HWInfo{Serial: "S1", CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}, nil
	}

	path := filepath.Join(t.TempDir(), "cache.json")
	rep := report.New(io.Discard, report.Plain)
	nodes := []Node{{Name: "n1", IP: "100.0.0.1", Online: true}}
	ProbeUsers(context.Background(), rep, nodes, nil, path)

	cache, err := LoadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	got := cache["n1"]
	if got.SSHUser != "root" || got.HW.Serial != "S1" || got.HW.Cores != 16 {
		t.Fatalf("cache not written correctly: %+v", got)
	}
}
```

If `report.New`'s exact signature differs in `probe_test.go`'s existing tests, mirror however the other tests in that file build a `*report.Reporter` (reuse their constructor call). Ensure `io` and `path/filepath` are imported.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -run ProbeUsersWritesCache -v`
Expected: FAIL — `too many arguments to ProbeUsers`.

- [ ] **Step 3: Implement — ProbeResult.HW, gather in goroutine, write cache**

In `internal/fleet/probe.go`, replace `ProbeResult` (lines 24-28) with:

```go
// ProbeResult is the discovered ssh user + hardware for a node ("" user = none worked).
type ProbeResult struct {
	Node Node
	User string
	HW   HWInfo
}
```

Replace the `ProbeUsers` signature and goroutine body. Change the signature line to:

```go
func ProbeUsers(ctx context.Context, rep *report.Reporter, nodes []Node, candidates []string, cachePath string) []ProbeResult {
```

Inside the goroutine (currently lines 70-80), replace the body with:

```go
go func(i int, n Node) {
	defer wg.Done()
	defer func() { <-sem }()
	user := probeNode(ctx, n, candidates)
	var hw HWInfo
	if user != "" {
		if h, err := gatherHW(ctx, user, n.IP); err == nil {
			hw = h
		} else {
			rep.Fail("probe-hw", n.Label(), err)
		}
	}
	results[i] = ProbeResult{Node: n, User: user, HW: hw}
	if user == "" {
		rep.Fail("probe-user", n.Label(), fmt.Errorf("no candidate ssh user worked"))
	} else {
		rep.OK("probe-user", n.Label(), report.KV{K: "user", V: user})
	}
}(i, n)
```

After the snippet-printing loop (after line 93, before the final `rep.Info(...)`/`return`), add the cache write:

```go
	if cachePath != "" {
		cache, _ := LoadCache(cachePath)
		for _, r := range results {
			if r.User == "" && r.HW.Serial == "" {
				continue
			}
			cache[r.Node.Name] = NodeCache{SSHUser: r.User, HW: r.HW}
		}
		if err := cache.Save(cachePath); err != nil {
			rep.Fail("probe-cache", "save", err)
		} else {
			rep.OK("probe-cache", cachePath)
		}
	}
```

- [ ] **Step 4: Update the existing ProbeUsers callsites in the test file**

Run `cd /Users/peter.lodri/workspace/pldev && grep -n "ProbeUsers(" internal/fleet/probe_test.go` and add a trailing `""` arg (empty cache path = no write) to every existing call. Example: `ProbeUsers(ctx, rep, nodes, nil)` → `ProbeUsers(ctx, rep, nodes, nil, "")`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/fleet/ -v`
Expected: PASS (all fleet tests).

- [ ] **Step 6: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/probe.go internal/fleet/probe_test.go
git commit -m "feat(fleet): probe-user gathers hardware and writes cache"
```

---

## Task 6: Wire cache into main.go dispatch

**Files:**
- Modify: `main.go:119-125` and `main.go:189-196` (the two Discover+Merge blocks), `main.go:242-247` (probe-user case)

- [ ] **Step 1: Update both Merge callsites**

In `main.go`, both blocks currently read:
```go
cfg, _ := fleet.LoadFleetConfig(fleet.DefaultFleetConfigPath())
nodes := fleet.Merge(ts, cfg)
```
Replace each with:
```go
cfg, _ := fleet.LoadFleetConfig(fleet.DefaultFleetConfigPath())
cache, _ := fleet.LoadCache(fleet.DefaultCachePath())
nodes := fleet.Merge(ts, cfg, cache)
```

- [ ] **Step 2: Update the probe-user case to pass the cache path**

In `main.go` the `case "probe-user":` block calls:
```go
fleet.ProbeUsers(ctx, rep, sel, nil)
```
Replace with:
```go
fleet.ProbeUsers(ctx, rep, sel, nil, fleet.DefaultCachePath())
```

- [ ] **Step 3: Build + vet**

Run: `cd /Users/peter.lodri/workspace/pldev && go build ./... && go vet ./... && go test ./...`
Expected: build clean, vet clean, all tests PASS.

- [ ] **Step 4: Manual live verification**

Run:
```bash
cd /Users/peter.lodri/workspace/pldev
go build -o pldev .
./pldev fleet probe-user
cat ~/.config/pldev/fleet-cache.json
./pldev fleet list
```
Expected: cache file has `hw` per reachable node; `fleet list` shows CPU/CR/MEM/ARCH columns; Forgemaster + Squire both show the MacBookPro18,3 hardware and Squire carries `⚠dup`.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add main.go
git commit -m "feat(fleet): wire hardware cache into list + probe-user dispatch"
```

---

## Cross-compile check (final)

- [ ] Run: `cd /Users/peter.lodri/workspace/pldev && GOOS=linux GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./...`
  Expected: both succeed (no platform-specific code added; gather runs remotely over ssh).

---

## Phase B (NOT this plan — near-term follow-up)

Serial as canonical node identity; collapse multiple tailnet nodes sharing a serial into one logical machine with aliases nested under it. Touches `Merge`/`Select`/`Render`/health/sync. Its own spec → plan.
