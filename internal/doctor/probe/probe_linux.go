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
