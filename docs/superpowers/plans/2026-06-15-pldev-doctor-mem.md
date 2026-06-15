# pldev `doctor mem` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `pldev doctor mem` — a cross-platform (macOS/Linux) Go TUI that diagnoses memory/swap/fs-event/disk-bloat problems and offers guided safe fixes.

**Architecture:** Single Go binary. A platform-implemented `Collector` interface fills a `MemReport`; a renderer colors it by severity; a `Fix` registry offers interactive guided actions. Bubble Tea/Lipgloss drive the TUI. Build tags split darwin/linux.

**Tech Stack:** Go 1.22+, charmbracelet bubbletea + lipgloss + bubbles, go-task (Taskfile), stdlib `testing`.

Working dir: `~/workspace/pldev` (already git-init'd, remote `origin` = github.com/peterlodri-sec/pldev).

---

## File Structure

```
go.mod
Taskfile.yml
main.go
SKILL.md
README.md
internal/sys/exec.go            + exec_test.go
internal/doctor/probe/probe.go  + probe_test.go        (types, Collector iface, severity)
internal/doctor/probe/parse.go  + parse_test.go        (pure parsers, fixture-tested)
internal/doctor/probe/probe_darwin.go
internal/doctor/probe/probe_linux.go
internal/doctor/probe/testdata/  (vm_stat.txt, meminfo.txt, swapon.txt, swapusage.txt)
internal/doctor/fix/fix.go      + fix_test.go          (Fix iface, registry, fake-fs)
internal/doctor/fix/fix_darwin.go
internal/doctor/fix/fix_linux.go
internal/doctor/mem/report.go   + report_test.go       (render, golden)
internal/doctor/mem/mem.go                              (orchestrator)
internal/tui/styles.go
internal/tui/confirm.go
internal/tui/menu.go
```

---

## Task 0: Project init

**Files:** Create `go.mod`, `Taskfile.yml`, `README.md`

- [ ] **Step 1: Init module + deps**

Run:
```bash
cd ~/workspace/pldev
go mod init github.com/peterlodri-sec/pldev
go get github.com/charmbracelet/bubbletea@latest github.com/charmbracelet/lipgloss@latest github.com/charmbracelet/bubbles@latest
```
Expected: `go.mod` + `go.sum` created with the three deps.

- [ ] **Step 2: Write `Taskfile.yml`**

```yaml
version: '3'
tasks:
  build:
    cmds: ['go build -o pldev .']
  install:
    cmds: ['go build -o "{{.HOME}}/.local/bin/pldev" .']
    env: { HOME: '{{.HOME}}' }
  test:
    cmds: ['go test ./...']
  lint:
    cmds: ['go vet ./...']
  cross:
    cmds:
      - GOOS=darwin GOARCH=arm64 go build -o dist/pldev-darwin-arm64 .
      - GOOS=darwin GOARCH=amd64 go build -o dist/pldev-darwin-amd64 .
      - GOOS=linux  GOARCH=arm64 go build -o dist/pldev-linux-arm64 .
      - GOOS=linux  GOARCH=amd64 go build -o dist/pldev-linux-amd64 .
```

- [ ] **Step 3: Write `README.md`**

```markdown
# pldev

Single-entrypoint personal machine helper. v1: `doctor mem`.

## Build
    task build      # -> ./pldev
    task install    # -> ~/.local/bin/pldev
    task test

## Use
    pldev                 # interactive root menu
    pldev doctor mem      # memory doctor (report + guided fixes)
    pldev doctor mem --report   # non-interactive dump (ssh/scripts)

Flags: --report, --yes (safe fixes only), --no-color.
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum Taskfile.yml README.md
git commit -m "chore: project init, taskfile, deps"
```

---

## Task 1: `sys.Run` command runner

**Files:** Create `internal/sys/exec.go`, `internal/sys/exec_test.go`

- [ ] **Step 1: Write failing test**

```go
package sys

import (
	"context"
	"testing"
	"time"
)

func TestRunCapturesStdout(t *testing.T) {
	out, err := Run(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hi\n" {
		t.Fatalf("got %q", out)
	}
}

func TestRunTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/sys/` — Expected: FAIL (undefined: Run).

- [ ] **Step 3: Implement**

```go
package sys

import (
	"context"
	"os/exec"
	"strings"
)

// Run executes name+args, returns trimmed-of-nothing stdout. Honors ctx deadline.
func Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var b strings.Builder
	cmd.Stdout = &b
	err := cmd.Run()
	return b.String(), err
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/sys/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sys/
git commit -m "feat(sys): command runner with context timeout"
```

---

## Task 2: probe types + severity

**Files:** Create `internal/doctor/probe/probe.go`, `internal/doctor/probe/probe_test.go`

- [ ] **Step 1: Write failing test**

```go
package probe

import "testing"

func TestSwapSeverity(t *testing.T) {
	cases := []struct {
		used, total uint64
		want        Severity
	}{
		{10, 100, SevOK},
		{60, 100, SevWarn},
		{80, 100, SevCrit},
	}
	for _, c := range cases {
		if got := SwapSeverity(c.used, c.total); got != c.want {
			t.Errorf("used=%d total=%d got %v want %v", c.used, c.total, got, c.want)
		}
	}
}

func TestFreeRAMSeverity(t *testing.T) {
	if FreeRAMSeverity(5, 100) != SevCrit {
		t.Error("5% free should be crit")
	}
	if FreeRAMSeverity(50, 100) != SevOK {
		t.Error("50% free should be ok")
	}
}

func TestFSEventsSeverity(t *testing.T) {
	mb := uint64(1 << 20)
	if FSEventsSeverity(300*mb) != SevOK {
		t.Error("300MB ok")
	}
	if FSEventsSeverity(700*mb) != SevWarn {
		t.Error("700MB warn")
	}
	if FSEventsSeverity(2000*mb) != SevCrit {
		t.Error("2GB crit")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/doctor/probe/` — Expected: FAIL (undefined types).

- [ ] **Step 3: Implement `probe.go`**

```go
package probe

import (
	"context"
	"time"
)

type Severity int

const (
	SevOK Severity = iota
	SevWarn
	SevCrit
	SevUnknown
)

func (s Severity) String() string {
	switch s {
	case SevOK:
		return "OK"
	case SevWarn:
		return "WARN"
	case SevCrit:
		return "CRIT"
	default:
		return "?"
	}
}

type MemStat struct{ Total, Used, Free uint64 } // bytes
type SwapStat struct{ Total, Used uint64 }      // bytes
type Proc struct {
	PID  int
	RSS  uint64 // bytes
	CPU  float64
	Name string
}
type DirSize struct {
	Path string
	Size uint64 // bytes
}
type Service struct {
	Label string
	Path  string // plist / unit file path
	Dead  bool   // present but not healthy (zombie)
}

// MemReport is the full diagnostic snapshot. Unavailable sections stay zero/nil
// and are flagged in Unavailable.
type MemReport struct {
	Mem         MemStat
	Swap        SwapStat
	TopProcs    []Proc
	FSEvents    *Proc // nil on linux / when absent
	NodeModules []DirSize
	Zombies     []Service
	Uptime      time.Duration
	Unavailable []string
}

// Collector is implemented per-platform.
type Collector interface {
	Mem(ctx context.Context) (MemStat, error)
	Swap(ctx context.Context) (SwapStat, error)
	TopProcs(ctx context.Context, n int) ([]Proc, error)
	FSEvents(ctx context.Context) (*Proc, error)
	NodeModules(ctx context.Context, roots []string) ([]DirSize, error)
	Zombies(ctx context.Context) ([]Service, error)
	Uptime(ctx context.Context) (time.Duration, error)
}

func pct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func SwapSeverity(used, total uint64) Severity {
	p := pct(used, total)
	switch {
	case p > 75:
		return SevCrit
	case p > 50:
		return SevWarn
	default:
		return SevOK
	}
}

func FreeRAMSeverity(free, total uint64) Severity {
	if pct(free, total) < 10 {
		return SevCrit
	}
	return SevOK
}

func FSEventsSeverity(rss uint64) Severity {
	const mb = uint64(1 << 20)
	switch {
	case rss > 1024*mb:
		return SevCrit
	case rss > 500*mb:
		return SevWarn
	default:
		return SevOK
	}
}

// Collect runs every probe with per-call timeout, recording failures in Unavailable.
func Collect(ctx context.Context, c Collector, nmRoots []string) MemReport {
	r := MemReport{}
	step := func(name string, fn func() error) {
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		_ = cctx
		if err := fn(); err != nil {
			r.Unavailable = append(r.Unavailable, name)
		}
	}
	step("mem", func() (err error) { r.Mem, err = c.Mem(ctx); return })
	step("swap", func() (err error) { r.Swap, err = c.Swap(ctx); return })
	step("procs", func() (err error) { r.TopProcs, err = c.TopProcs(ctx, 10); return })
	step("fsevents", func() (err error) { r.FSEvents, err = c.FSEvents(ctx); return })
	step("node_modules", func() (err error) { r.NodeModules, err = c.NodeModules(ctx, nmRoots); return })
	step("zombies", func() (err error) { r.Zombies, err = c.Zombies(ctx); return })
	step("uptime", func() (err error) { r.Uptime, err = c.Uptime(ctx); return })
	return r
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/doctor/probe/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/probe/probe.go internal/doctor/probe/probe_test.go
git commit -m "feat(probe): report types, Collector interface, severity thresholds"
```

---

## Task 3: pure parsers (fixture-tested)

**Files:** Create `internal/doctor/probe/parse.go`, `internal/doctor/probe/parse_test.go`, fixtures under `internal/doctor/probe/testdata/`

- [ ] **Step 1: Write fixtures**

`testdata/meminfo.txt`:
```
MemTotal:       16384000 kB
MemFree:          512000 kB
MemAvailable:    2048000 kB
```
`testdata/swapusage.txt`:
```
vm.swapusage: total = 44032.00M  used = 42916.19M  free = 1115.81M  (encrypted)
```

- [ ] **Step 2: Write failing test**

```go
package probe

import (
	"os"
	"testing"
)

func TestParseMeminfo(t *testing.T) {
	b, _ := os.ReadFile("testdata/meminfo.txt")
	m, err := parseMeminfo(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 16384000*1024 {
		t.Errorf("total=%d", m.Total)
	}
	// used = total - available
	if m.Used != (16384000-2048000)*1024 {
		t.Errorf("used=%d", m.Used)
	}
}

func TestParseSwapusage(t *testing.T) {
	b, _ := os.ReadFile("testdata/swapusage.txt")
	s, err := parseSwapusage(string(b))
	if err != nil {
		t.Fatal(err)
	}
	// 44032.00M total
	if s.Total != uint64(44032.0*1024*1024) {
		t.Errorf("total=%d", s.Total)
	}
	if s.Used == 0 || s.Used >= s.Total {
		t.Errorf("used=%d total=%d", s.Used, s.Total)
	}
}
```

- [ ] **Step 3: Run, expect FAIL**

Run: `go test ./internal/doctor/probe/ -run Parse` — Expected: FAIL (undefined parsers).

- [ ] **Step 4: Implement `parse.go`**

```go
package probe

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// parseMeminfo reads /proc/meminfo content. Used = Total - Available.
func parseMeminfo(s string) (MemStat, error) {
	vals := map[string]uint64{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		key := strings.TrimSuffix(f[0], ":")
		kb, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = kb * 1024
	}
	total := vals["MemTotal"]
	if total == 0 {
		return MemStat{}, fmt.Errorf("no MemTotal")
	}
	avail := vals["MemAvailable"]
	return MemStat{Total: total, Free: vals["MemFree"], Used: total - avail}, nil
}

// parseSwapusage reads `sysctl vm.swapusage` output (macOS), values like 44032.00M.
func parseSwapusage(s string) (SwapStat, error) {
	get := func(label string) (uint64, error) {
		i := strings.Index(s, label+" = ")
		if i < 0 {
			return 0, fmt.Errorf("missing %s", label)
		}
		rest := s[i+len(label)+3:]
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, fmt.Errorf("empty %s", label)
		}
		return parseSizeToken(fields[0])
	}
	total, err := get("total")
	if err != nil {
		return SwapStat{}, err
	}
	used, err := get("used")
	if err != nil {
		return SwapStat{}, err
	}
	return SwapStat{Total: total, Used: used}, nil
}

// parseSizeToken converts "44032.00M" / "1115.81M" / "1.5G" to bytes.
func parseSizeToken(tok string) (uint64, error) {
	if tok == "" {
		return 0, fmt.Errorf("empty token")
	}
	mult := float64(1)
	switch tok[len(tok)-1] {
	case 'K':
		mult, tok = 1024, tok[:len(tok)-1]
	case 'M':
		mult, tok = 1024*1024, tok[:len(tok)-1]
	case 'G':
		mult, tok = 1024*1024*1024, tok[:len(tok)-1]
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, err
	}
	return uint64(f * mult), nil
}
```

- [ ] **Step 5: Run, expect PASS**

Run: `go test ./internal/doctor/probe/ -run Parse` — Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/doctor/probe/parse.go internal/doctor/probe/parse_test.go internal/doctor/probe/testdata/
git commit -m "feat(probe): pure parsers for meminfo and swapusage with fixtures"
```

---

## Task 4: darwin + linux collectors

**Files:** Create `internal/doctor/probe/probe_darwin.go`, `internal/doctor/probe/probe_linux.go`

These shell out via `sys.Run`; pure parsing lives in Task 3. Not unit-tested (integration-only) — keep them thin.

- [ ] **Step 1: Implement `probe_darwin.go`**

```go
//go:build darwin

package probe

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

type darwinCollector struct{}

func New() Collector { return darwinCollector{} }

func (darwinCollector) Mem(ctx context.Context) (MemStat, error) {
	tot, err := sys.Run(ctx, "sysctl", "-n", "hw.memsize")
	if err != nil {
		return MemStat{}, err
	}
	total, _ := strconv.ParseUint(strings.TrimSpace(tot), 10, 64)
	vmstat, err := sys.Run(ctx, "vm_stat")
	if err != nil {
		return MemStat{Total: total}, err
	}
	free := parseVMStatFree(vmstat) // page-based free+inactive
	return MemStat{Total: total, Free: free, Used: total - free}, nil
}

func (darwinCollector) Swap(ctx context.Context) (SwapStat, error) {
	out, err := sys.Run(ctx, "sysctl", "vm.swapusage")
	if err != nil {
		return SwapStat{}, err
	}
	return parseSwapusage(out)
}

func (darwinCollector) TopProcs(ctx context.Context, n int) ([]Proc, error) {
	out, err := sys.Run(ctx, "ps", "axo", "pid=,rss=,pcpu=,comm=")
	if err != nil {
		return nil, err
	}
	return topProcs(out, n), nil
}

func (darwinCollector) FSEvents(ctx context.Context) (*Proc, error) {
	out, err := sys.Run(ctx, "pgrep", "-x", "fseventsd")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil, nil
	}
	pid, _ := strconv.Atoi(strings.Fields(out)[0])
	ps, err := sys.Run(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	if err != nil {
		return nil, err
	}
	rssKB, _ := strconv.ParseUint(strings.TrimSpace(ps), 10, 64)
	return &Proc{PID: pid, RSS: rssKB * 1024, Name: "fseventsd"}, nil
}

func (darwinCollector) NodeModules(ctx context.Context, roots []string) ([]DirSize, error) {
	return findNodeModules(ctx, roots)
}

func (darwinCollector) Zombies(ctx context.Context) ([]Service, error) {
	var out []Service
	la := filepath.Join(os.Getenv("HOME"), "Library/LaunchAgents")
	entries, _ := os.ReadDir(la)
	listed, _ := sys.Run(ctx, "launchctl", "list")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".plist") {
			continue
		}
		label := strings.TrimSuffix(e.Name(), ".plist")
		// dead = listed with negative last-exit status
		dead := false
		for _, line := range strings.Split(listed, "\n") {
			if strings.HasSuffix(line, label) {
				f := strings.Fields(line)
				if len(f) >= 2 && strings.HasPrefix(f[1], "-") {
					dead = true
				}
			}
		}
		out = append(out, Service{Label: label, Path: filepath.Join(la, e.Name()), Dead: dead})
	}
	return out, nil
}

func (darwinCollector) Uptime(ctx context.Context) (time.Duration, error) {
	out, err := sys.Run(ctx, "sysctl", "-n", "kern.boottime")
	if err != nil {
		return 0, err
	}
	// e.g. "{ sec = 1716600000, usec = 0 } ..."
	i := strings.Index(out, "sec = ")
	if i < 0 {
		return 0, nil
	}
	rest := out[i+6:]
	secStr := strings.FieldsFunc(rest, func(r rune) bool { return r < '0' || r > '9' })[0]
	boot, _ := strconv.ParseInt(secStr, 10, 64)
	return time.Since(time.Unix(boot, 0)), nil
}

func parseVMStatFree(s string) uint64 {
	pageSize := uint64(16384)
	var free, inactive uint64
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.Contains(line, "page size of"):
			f := strings.FieldsFunc(line, func(r rune) bool { return r < '0' || r > '9' })
			if len(f) > 0 {
				if v, err := strconv.ParseUint(f[len(f)-1], 10, 64); err == nil {
					pageSize = v
				}
			}
		case strings.HasPrefix(line, "Pages free:"):
			free = pagesIn(line)
		case strings.HasPrefix(line, "Pages inactive:"):
			inactive = pagesIn(line)
		}
	}
	return (free + inactive) * pageSize
}

func pagesIn(line string) uint64 {
	line = strings.TrimRight(strings.TrimSpace(line), ".")
	f := strings.Fields(line)
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseUint(f[len(f)-1], 10, 64)
	return v
}
```

- [ ] **Step 2: Implement shared helpers `procutil.go`** (no build tag — used by both)

Create `internal/doctor/probe/procutil.go`:

```go
package probe

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// topProcs parses `ps axo pid=,rss=,pcpu=,comm=` (rss in KB), returns top-n by RSS.
func topProcs(out string, n int) []Proc {
	var ps []Proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		rssKB, _ := strconv.ParseUint(f[1], 10, 64)
		cpu, _ := strconv.ParseFloat(f[2], 64)
		ps = append(ps, Proc{PID: pid, RSS: rssKB * 1024, CPU: cpu, Name: strings.Join(f[3:], " ")})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].RSS > ps[j].RSS })
	if len(ps) > n {
		ps = ps[:n]
	}
	return ps
}

// findNodeModules locates node_modules dirs under roots and sizes them with `du -sk`.
func findNodeModules(ctx context.Context, roots []string) ([]DirSize, error) {
	var dirs []DirSize
	for _, root := range roots {
		out, err := sys.Run(ctx, "find", root, "-type", "d", "-name", "node_modules", "-prune")
		if err != nil {
			continue
		}
		for _, p := range strings.Split(strings.TrimSpace(out), "\n") {
			if p == "" {
				continue
			}
			du, err := sys.Run(ctx, "du", "-sk", p)
			if err != nil {
				continue
			}
			kb, _ := strconv.ParseUint(strings.Fields(du)[0], 10, 64)
			dirs = append(dirs, DirSize{Path: p, Size: kb * 1024})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Size > dirs[j].Size })
	return dirs, nil
}
```

- [ ] **Step 3: Implement `probe_linux.go`**

```go
//go:build linux

package probe

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

type linuxCollector struct{}

func New() Collector { return linuxCollector{} }

func (linuxCollector) Mem(ctx context.Context) (MemStat, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemStat{}, err
	}
	return parseMeminfo(string(b))
}

func (linuxCollector) Swap(ctx context.Context) (SwapStat, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SwapStat{}, err
	}
	var total, free uint64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		kb, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "SwapTotal:":
			total = kb * 1024
		case "SwapFree:":
			free = kb * 1024
		}
	}
	return SwapStat{Total: total, Used: total - free}, nil
}

func (linuxCollector) TopProcs(ctx context.Context, n int) ([]Proc, error) {
	out, err := sys.Run(ctx, "ps", "axo", "pid=,rss=,pcpu=,comm=")
	if err != nil {
		return nil, err
	}
	return topProcs(out, n), nil
}

func (linuxCollector) FSEvents(ctx context.Context) (*Proc, error) { return nil, nil }

func (linuxCollector) NodeModules(ctx context.Context, roots []string) ([]DirSize, error) {
	return findNodeModules(ctx, roots)
}

func (linuxCollector) Zombies(ctx context.Context) ([]Service, error) {
	out, err := sys.Run(ctx, "systemctl", "--user", "list-units", "--state=failed", "--no-legend", "--plain")
	if err != nil {
		return nil, err
	}
	var svc []Service
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		svc = append(svc, Service{Label: f[0], Path: f[0], Dead: true})
	}
	return svc, nil
}

func (linuxCollector) Uptime(ctx context.Context) (time.Duration, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	secs, _ := strconv.ParseFloat(strings.Fields(string(b))[0], 64)
	return time.Duration(secs) * time.Second, nil
}
```

- [ ] **Step 4: Build for both platforms**

Run:
```bash
go build ./... && GOOS=linux go build ./...
```
Expected: both succeed, no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/probe/
git commit -m "feat(probe): darwin + linux collectors and shared proc/nodemodules helpers"
```

---

## Task 5: Fix interface + registry

**Files:** Create `internal/doctor/fix/fix.go`, `internal/doctor/fix/fix_test.go`

- [ ] **Step 1: Write failing test**

```go
package fix

import (
	"context"
	"testing"
)

type fakeFix struct {
	name string
	safe bool
	done *bool
}

func (f fakeFix) Title() string                    { return f.name }
func (f fakeFix) Safe() bool                        { return f.safe }
func (f fakeFix) Describe() string                  { return "does " + f.name }
func (f fakeFix) Apply(context.Context) (string, error) { *f.done = true; return "ok", nil }

func TestSafeOnlyFilters(t *testing.T) {
	var a, b bool
	all := []Fix{
		fakeFix{"safe1", true, &a},
		fakeFix{"danger", false, &b},
	}
	got := SafeOnly(all)
	if len(got) != 1 || got[0].Title() != "safe1" {
		t.Fatalf("SafeOnly wrong: %v", got)
	}
}

func TestApplyAllSafe(t *testing.T) {
	var a, b bool
	all := []Fix{fakeFix{"s", true, &a}, fakeFix{"d", false, &b}}
	ApplyAll(context.Background(), SafeOnly(all))
	if !a || b {
		t.Fatalf("expected only safe applied: a=%v b=%v", a, b)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/doctor/fix/` — Expected: FAIL (undefined Fix).

- [ ] **Step 3: Implement `fix.go`**

```go
package fix

import "context"

// Fix is one guided remediation. Safe() fixes are reversible and may be
// auto-applied with --yes; destructive fixes require explicit confirmation.
type Fix interface {
	Title() string
	Describe() string // what it will do, shown before applying
	Safe() bool
	Apply(ctx context.Context) (result string, err error)
}

func SafeOnly(in []Fix) []Fix {
	var out []Fix
	for _, f := range in {
		if f.Safe() {
			out = append(out, f)
		}
	}
	return out
}

// ApplyAll applies fixes in order, returning per-fix results (best effort).
func ApplyAll(ctx context.Context, fixes []Fix) []string {
	var res []string
	for _, f := range fixes {
		out, err := f.Apply(ctx)
		if err != nil {
			res = append(res, f.Title()+": ERROR "+err.Error())
			continue
		}
		res = append(res, f.Title()+": "+out)
	}
	return res
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/doctor/fix/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/fix/fix.go internal/doctor/fix/fix_test.go
git commit -m "feat(fix): Fix interface, SafeOnly filter, ApplyAll"
```

---

## Task 6: concrete fixes (darwin + linux)

**Files:** Create `internal/doctor/fix/fix_darwin.go`, `internal/doctor/fix/fix_linux.go`, and shared `internal/doctor/fix/common.go`

- [ ] **Step 1: Implement shared `common.go`**

```go
package fix

import (
	"context"
	"fmt"
	"os"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// PurgeNodeModules deletes the given node_modules dirs (Destructive).
type PurgeNodeModules struct{ Dirs []string }

func (p PurgeNodeModules) Title() string { return "Purge selected node_modules" }
func (p PurgeNodeModules) Safe() bool     { return false }
func (p PurgeNodeModules) Describe() string {
	return fmt.Sprintf("rm -rf %d node_modules dir(s) (recoverable via reinstall)", len(p.Dirs))
}
func (p PurgeNodeModules) Apply(_ context.Context) (string, error) {
	for _, d := range p.Dirs {
		if err := os.RemoveAll(d); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("removed %d dirs", len(p.Dirs)), nil
}

// RebootHint is advice only (Safe, no action).
type RebootHint struct{ Reason string }

func (RebootHint) Title() string             { return "Reboot recommended" }
func (RebootHint) Safe() bool                 { return true }
func (h RebootHint) Describe() string         { return h.Reason }
func (h RebootHint) Apply(context.Context) (string, error) {
	return "advice only — reboot when convenient", nil
}

func _unused(ctx context.Context) { _, _ = sys.Run(ctx, "true") } // keep sys imported where needed
```

(Remove `_unused` if `sys` is otherwise used; it exists to avoid an unused-import in case the linux file is the only sys user. If `go vet` complains, delete it.)

- [ ] **Step 2: Implement `fix_darwin.go`**

```go
//go:build darwin

package fix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// SpotlightExclude writes .metadata_never_index markers (Safe, reversible).
type SpotlightExclude struct{ Dirs []string }

func (SpotlightExclude) Title() string { return "Exclude heavy dirs from Spotlight" }
func (SpotlightExclude) Safe() bool     { return true }
func (s SpotlightExclude) Describe() string {
	return fmt.Sprintf("touch .metadata_never_index in %d dir(s)", len(s.Dirs))
}
func (s SpotlightExclude) Apply(_ context.Context) (string, error) {
	var done int
	for _, d := range s.Dirs {
		marker := filepath.Join(d, ".metadata_never_index")
		f, err := os.Create(marker)
		if err != nil {
			continue // e.g. read-only / SIP — skip, report below
		}
		f.Close()
		done++
	}
	return fmt.Sprintf("marked %d/%d dirs", done, len(s.Dirs)), nil
}

// RemoveLaunchAgent boots out + deletes a zombie LaunchAgent plist (Destructive).
type RemoveLaunchAgent struct {
	Label string
	Path  string
}

func (RemoveLaunchAgent) Title() string { return "Remove zombie LaunchAgent" }
func (RemoveLaunchAgent) Safe() bool     { return false }
func (r RemoveLaunchAgent) Describe() string {
	return "launchctl bootout + rm " + r.Path
}
func (r RemoveLaunchAgent) Apply(ctx context.Context) (string, error) {
	uid := os.Getuid()
	_, _ = sys.Run(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, r.Label))
	if err := os.Remove(r.Path); err != nil {
		return "", err
	}
	return "booted out + removed " + r.Label, nil
}
```

- [ ] **Step 3: Implement `fix_linux.go`**

```go
//go:build linux

package fix

import (
	"context"
	"fmt"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// DisableUnit stops+disables a failed systemd --user unit (Destructive-ish).
type DisableUnit struct{ Unit string }

func (DisableUnit) Title() string { return "Disable failed systemd unit" }
func (DisableUnit) Safe() bool     { return false }
func (d DisableUnit) Describe() string {
	return "systemctl --user disable --now " + d.Unit
}
func (d DisableUnit) Apply(ctx context.Context) (string, error) {
	if _, err := sys.Run(ctx, "systemctl", "--user", "disable", "--now", d.Unit); err != nil {
		return "", err
	}
	return fmt.Sprintf("disabled %s", d.Unit), nil
}
```

- [ ] **Step 4: Build both platforms**

Run: `go build ./... && GOOS=linux go build ./...` — Expected: success. If `go vet ./...` flags `_unused`, delete it from `common.go`.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/fix/
git commit -m "feat(fix): spotlight/launchagent/systemd/node_modules/reboot fixes"
```

---

## Task 7: report rendering (golden test)

**Files:** Create `internal/doctor/mem/report.go`, `internal/doctor/mem/report_test.go`

- [ ] **Step 1: Write failing test**

```go
package mem

import (
	"strings"
	"testing"
	"time"

	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func TestRenderPlain(t *testing.T) {
	r := probe.MemReport{
		Mem:    probe.MemStat{Total: 100 << 30, Used: 80 << 30, Free: 20 << 30},
		Swap:   probe.SwapStat{Total: 44 << 30, Used: 42 << 30},
		Uptime: 20 * 24 * time.Hour,
		NodeModules: []probe.DirSize{
			{Path: "/a/node_modules", Size: 5 << 30},
		},
	}
	out := RenderPlain(r)
	if !strings.Contains(out, "Swap") {
		t.Error("missing swap line")
	}
	if !strings.Contains(out, "CRIT") {
		t.Error("swap 95% should be CRIT")
	}
	if !strings.Contains(out, "node_modules") {
		t.Error("missing node_modules section")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/doctor/mem/` — Expected: FAIL (undefined RenderPlain).

- [ ] **Step 3: Implement `report.go`**

```go
package mem

import (
	"fmt"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func gib(b uint64) float64 { return float64(b) / (1 << 30) }

// RenderPlain produces a no-color textual report (used by --report and golden tests).
func RenderPlain(r probe.MemReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== pldev doctor mem ==\n")
	fmt.Fprintf(&b, "Uptime: %s\n", r.Uptime.Round(1e9))
	memSev := probe.FreeRAMSeverity(r.Mem.Free, r.Mem.Total)
	fmt.Fprintf(&b, "RAM:  %.1f/%.1f GiB used  [%s]\n", gib(r.Mem.Used), gib(r.Mem.Total), memSev)
	swSev := probe.SwapSeverity(r.Swap.Used, r.Swap.Total)
	fmt.Fprintf(&b, "Swap: %.1f/%.1f GiB used  [%s]\n", gib(r.Swap.Used), gib(r.Swap.Total), swSev)
	if r.FSEvents != nil {
		fmt.Fprintf(&b, "fseventsd RSS: %.0f MiB  [%s]\n",
			float64(r.FSEvents.RSS)/(1<<20), probe.FSEventsSeverity(r.FSEvents.RSS))
	}
	if len(r.NodeModules) > 0 {
		var total uint64
		for _, d := range r.NodeModules {
			total += d.Size
		}
		fmt.Fprintf(&b, "node_modules: %d dirs, %.1f GiB total\n", len(r.NodeModules), gib(total))
		for i, d := range r.NodeModules {
			if i >= 10 {
				fmt.Fprintf(&b, "  ... %d more\n", len(r.NodeModules)-10)
				break
			}
			fmt.Fprintf(&b, "  %6.1f GiB  %s\n", gib(d.Size), d.Path)
		}
	}
	for _, z := range r.Zombies {
		if z.Dead {
			fmt.Fprintf(&b, "ZOMBIE service: %s (%s)\n", z.Label, z.Path)
		}
	}
	if len(r.Unavailable) > 0 {
		fmt.Fprintf(&b, "unavailable: %s\n", strings.Join(r.Unavailable, ", "))
	}
	return b.String()
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/doctor/mem/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/mem/report.go internal/doctor/mem/report_test.go
git commit -m "feat(mem): plain report renderer with severity tags"
```

---

## Task 8: TUI (styles, confirm, menu)

**Files:** Create `internal/tui/styles.go`, `internal/tui/confirm.go`, `internal/tui/menu.go`

TUI is integration-level; keep models small. No unit tests (manual verify in Task 10).

- [ ] **Step 1: Implement `styles.go`**

```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	OK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	Warn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	Crit  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	Dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)
```

- [ ] **Step 2: Implement `confirm.go`**

```go
package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm prints prompt and reads y/N from stdin. Returns true only on y/yes.
func Confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(sc.Text()))
	return a == "y" || a == "yes"
}
```

- [ ] **Step 3: Implement `menu.go` (Bubble Tea list model)**

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// SelectModel is a minimal arrow-key chooser returning the chosen index via Chosen.
type SelectModel struct {
	Title   string
	Items   []string
	cursor  int
	Chosen  int
	Aborted bool
}

func NewSelect(title string, items []string) SelectModel {
	return SelectModel{Title: title, Items: items, Chosen: -1}
}

func (m SelectModel) Init() tea.Cmd { return nil }

func (m SelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "q", "esc":
			m.Aborted = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case "enter":
			m.Chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SelectModel) View() string {
	s := Title.Render(m.Title) + "\n\n"
	for i, it := range m.Items {
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		s += cur + it + "\n"
	}
	s += "\n" + Dim.Render("↑/↓ move · enter select · q quit")
	return s
}
```

- [ ] **Step 4: Build**

Run: `go build ./...` — Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): lipgloss styles, confirm prompt, bubbletea select menu"
```

---

## Task 9: mem orchestrator

**Files:** Create `internal/doctor/mem/mem.go`

- [ ] **Step 1: Implement `mem.go`**

```go
package mem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

type Options struct {
	ReportOnly bool
	YesSafe    bool
}

func defaultRoots() []string {
	home := os.Getenv("HOME")
	return []string{filepath.Join(home, "workspace")}
}

// Run collects the report, prints it, and (unless ReportOnly) offers fixes.
func Run(ctx context.Context, c probe.Collector, opts Options) error {
	r := probe.Collect(ctx, c, defaultRoots())
	fmt.Println(RenderPlain(r))

	if opts.ReportOnly {
		return nil
	}

	fixes := buildFixes(r)
	if len(fixes) == 0 {
		fmt.Println("No fixes suggested. System looks OK.")
		return nil
	}

	if opts.YesSafe {
		for _, f := range fix.SafeOnly(fixes) {
			out, err := f.Apply(ctx)
			fmt.Printf("[auto] %s: %s %v\n", f.Title(), out, err)
		}
		return nil
	}

	return interactiveFixMenu(ctx, fixes)
}

func buildFixes(r probe.MemReport) []fix.Fix {
	var fixes []fix.Fix
	// reboot hint
	if r.Uptime > 7*24*time.Hour && probe.SwapSeverity(r.Swap.Used, r.Swap.Total) != probe.SevOK {
		fixes = append(fixes, fix.RebootHint{Reason: fmt.Sprintf(
			"uptime %s + high swap — reboot clears leaked daemons and swap", r.Uptime.Round(time.Hour))})
	}
	// node_modules purge (top dirs)
	if len(r.NodeModules) > 0 {
		var paths []string
		for _, d := range r.NodeModules {
			paths = append(paths, d.Path)
		}
		fixes = append(fixes, fix.PurgeNodeModules{Dirs: paths})
	}
	fixes = append(fixes, platformFixes(r)...)
	return fixes
}

func interactiveFixMenu(ctx context.Context, fixes []fix.Fix) error {
	items := make([]string, len(fixes))
	for i, f := range fixes {
		tag := "SAFE"
		if !f.Safe() {
			tag = "DESTRUCTIVE"
		}
		items[i] = fmt.Sprintf("[%s] %s — %s", tag, f.Title(), f.Describe())
	}
	items = append(items, "Quit")

	m := tui.NewSelect("doctor mem — choose a fix", items)
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	sm := res.(tui.SelectModel)
	if sm.Aborted || sm.Chosen < 0 || sm.Chosen >= len(fixes) {
		return nil
	}
	chosen := fixes[sm.Chosen]
	if !chosen.Safe() {
		if !tui.Confirm(fmt.Sprintf("Confirm DESTRUCTIVE: %s", chosen.Describe())) {
			fmt.Println("aborted.")
			return nil
		}
	}
	out, err := chosen.Apply(ctx)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return err
	}
	fmt.Printf("done: %s\n", out)
	return nil
}
```

- [ ] **Step 2: Implement `platformFixes` for darwin** — create `internal/doctor/mem/fixes_darwin.go`

```go
//go:build darwin

package mem

import (
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func platformFixes(r probe.MemReport) []fix.Fix {
	var f []fix.Fix
	home := os.Getenv("HOME")
	f = append(f, fix.SpotlightExclude{Dirs: []string{
		filepath.Join(home, "workspace"),
		filepath.Join(home, ".orbstack"),
	}})
	for _, z := range r.Zombies {
		if z.Dead {
			f = append(f, fix.RemoveLaunchAgent{Label: z.Label, Path: z.Path})
		}
	}
	return f
}
```

- [ ] **Step 3: Implement `platformFixes` for linux** — create `internal/doctor/mem/fixes_linux.go`

```go
//go:build linux

package mem

import (
	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func platformFixes(r probe.MemReport) []fix.Fix {
	var f []fix.Fix
	for _, z := range r.Zombies {
		if z.Dead {
			f = append(f, fix.DisableUnit{Unit: z.Label})
		}
	}
	return f
}
```

- [ ] **Step 4: Build both platforms + test**

Run: `go build ./... && GOOS=linux go build ./... && go test ./...` — Expected: success, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/mem/
git commit -m "feat(mem): orchestrator wiring collect→render→interactive fixes"
```

---

## Task 10: main.go entrypoint + manual verify

**Files:** Create `main.go`

- [ ] **Step 1: Implement `main.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/peterlodri-sec/pldev/internal/doctor/mem"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

func main() {
	args := os.Args[1:]
	opts := mem.Options{}
	var positional []string
	for _, a := range args {
		switch a {
		case "--report":
			opts.ReportOnly = true
		case "--yes":
			opts.YesSafe = true
		case "--no-color":
			os.Setenv("NO_COLOR", "1")
		default:
			positional = append(positional, a)
		}
	}

	ctx := context.Background()

	// direct subcommand: pldev doctor mem
	if len(positional) >= 2 && positional[0] == "doctor" && positional[1] == "mem" {
		runMem(ctx, opts)
		return
	}
	if len(positional) == 0 {
		rootMenu(ctx, opts)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: pldev [doctor mem] [--report] [--yes] [--no-color]")
	os.Exit(2)
}

func runMem(ctx context.Context, opts mem.Options) {
	if err := mem.Run(ctx, probe.New(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootMenu(ctx context.Context, opts mem.Options) {
	m := tui.NewSelect("pldev", []string{"doctor mem", "Quit"})
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sm := res.(tui.SelectModel)
	if sm.Chosen == 0 {
		runMem(ctx, opts)
	}
}
```

- [ ] **Step 2: Build + run report mode**

Run: `go build -o pldev . && ./pldev doctor mem --report`
Expected: prints RAM/Swap/node_modules report with severity tags, exits 0. (On the dev Mac, swap should show CRIT.)

- [ ] **Step 3: Run full test suite + vet**

Run: `go test ./... && go vet ./...` — Expected: PASS, no vet errors.

- [ ] **Step 4: Commit**

```bash
git add main.go pldev 2>/dev/null; git rm --cached pldev 2>/dev/null; git add main.go
git commit -m "feat: main entrypoint, arg parsing, root menu + doctor mem"
```

---

## Task 11: SKILL.md

**Files:** Create `SKILL.md`

- [ ] **Step 1: Write `SKILL.md`**

```markdown
---
name: pldev
description: Use pldev to diagnose and fix machine memory/swap/disk problems. Triggers on "check my memory", "what's eating my RAM", "diagnose this machine", "free up space", "pldev".
---

# pldev

`pldev` is a cross-platform (macOS/Linux) machine doctor. v1 module: `doctor mem`.

## Run

- Interactive: `pldev doctor mem` — shows a severity-coded report, then an
  arrow-key menu of fixes.
- Non-interactive (capture/ssh): `pldev doctor mem --report` — prints the report
  only, no prompts. Use this when reading output programmatically.
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
```

- [ ] **Step 2: Commit + push everything**

```bash
git add SKILL.md
git commit -m "docs: pldev SKILL.md for agent usage"
git push -u origin main
```

---

## Self-Review Checklist (completed by plan author)

- **Spec coverage:** stack ✓(T0), command surface ✓(T10), MemReport+Collector ✓(T2), parsers ✓(T3), platform collectors ✓(T4), severity ✓(T2), fixes+safety ✓(T5/6/9), report ✓(T7), TUI ✓(T8), error/timeout (Collect) ✓(T2), testing ✓(T1-7), SKILL.md ✓(T11), Taskfile/distribution ✓(T0), repo ✓(pre-existing).
- **Type consistency:** `Collector`, `MemReport`, `Fix`, `Severity`, `probe.New()` used consistently across tasks. `RenderPlain` defined T7 used T9/T10. `tui.NewSelect`/`SelectModel.Chosen/Aborted` defined T8 used T9/T10.
- **Placeholders:** none — all steps carry real code/commands.
- Note: `common.go` `_unused` shim may trip `go vet`; T6 step 4 instructs deletion if so.
```
