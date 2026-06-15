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
