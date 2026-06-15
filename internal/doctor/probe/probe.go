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

type MemStat struct{ Total, Used, Free, Available uint64 } // bytes
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
	step := func(name string, fn func(cctx context.Context) error) {
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		if err := fn(cctx); err != nil {
			r.Unavailable = append(r.Unavailable, name)
		}
	}
	step("mem", func(cctx context.Context) (err error) { r.Mem, err = c.Mem(cctx); return })
	step("swap", func(cctx context.Context) (err error) { r.Swap, err = c.Swap(cctx); return })
	step("procs", func(cctx context.Context) (err error) { r.TopProcs, err = c.TopProcs(cctx, 10); return })
	step("fsevents", func(cctx context.Context) (err error) { r.FSEvents, err = c.FSEvents(cctx); return })
	step("node_modules", func(cctx context.Context) (err error) { r.NodeModules, err = c.NodeModules(cctx, nmRoots); return })
	step("zombies", func(cctx context.Context) (err error) { r.Zombies, err = c.Zombies(cctx); return })
	step("uptime", func(cctx context.Context) (err error) { r.Uptime, err = c.Uptime(cctx); return })
	return r
}
