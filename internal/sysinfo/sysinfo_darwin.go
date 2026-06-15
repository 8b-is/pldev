//go:build darwin

package sysinfo

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

type darwinCollector struct{}

func New() Collector { return darwinCollector{} }

func (darwinCollector) Host(ctx context.Context) (Host, error) {
	h := Host{Arch: runtime.GOARCH, Cores: runtime.NumCPU()}

	if name, err := sys.Run(ctx, "scutil", "--get", "LocalHostName"); err == nil && strings.TrimSpace(name) != "" {
		h.Hostname = strings.TrimSpace(name)
	} else if hn, err := os.Hostname(); err == nil {
		h.Hostname = hn
	}

	prod, _ := sys.Run(ctx, "sw_vers", "-productName")
	ver, _ := sys.Run(ctx, "sw_vers", "-productVersion")
	h.OS = strings.TrimSpace(strings.TrimSpace(prod) + " " + strings.TrimSpace(ver))

	if k, err := sys.Run(ctx, "uname", "-r"); err == nil {
		h.Kernel = strings.TrimSpace(k)
	}
	if m, err := sys.Run(ctx, "sysctl", "-n", "hw.model"); err == nil {
		h.Model = strings.TrimSpace(m)
	}
	if cpu, err := sys.Run(ctx, "sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
		h.CPU = strings.TrimSpace(cpu)
	}
	if c, err := sys.Run(ctx, "sysctl", "-n", "hw.ncpu"); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(c)); e == nil && n > 0 {
			h.Cores = n
		}
	}
	h.Uptime = darwinUptime(ctx)
	return h, nil
}

func darwinUptime(ctx context.Context) time.Duration {
	out, err := sys.Run(ctx, "sysctl", "-n", "kern.boottime")
	if err != nil {
		return 0
	}
	i := strings.Index(out, "sec = ")
	if i < 0 {
		return 0
	}
	rest := out[i+6:]
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if len(parts) == 0 {
		return 0
	}
	boot, _ := strconv.ParseInt(parts[0], 10, 64)
	return time.Since(time.Unix(boot, 0))
}

func (darwinCollector) Disk(ctx context.Context) ([]Disk, error) {
	out, err := sys.Run(ctx, "df", "-k")
	if err != nil {
		return nil, err
	}
	return parseDf(out), nil
}

func (darwinCollector) Net(ctx context.Context) ([]Net, error) {
	out, err := sys.Run(ctx, "ifconfig")
	if err != nil {
		return nil, err
	}
	return parseIfconfig(out), nil
}
