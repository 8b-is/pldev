//go:build linux

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

type linuxCollector struct{}

func New() Collector { return linuxCollector{} }

func (linuxCollector) Host(ctx context.Context) (Host, error) {
	h := Host{Arch: runtime.GOARCH, Cores: runtime.NumCPU()}

	if hn, err := os.Hostname(); err == nil {
		h.Hostname = hn
	}

	h.OS = parseOSRelease(readFile("/etc/os-release"))
	if h.OS == "" {
		h.OS = "linux"
	}

	if k, err := sys.Run(ctx, "uname", "-r"); err == nil {
		h.Kernel = strings.TrimSpace(k)
	}

	h.Model = "linux"
	if pn := strings.TrimSpace(readFile("/sys/devices/virtual/dmi/id/product_name")); pn != "" {
		h.Model = pn
	}

	h.CPU = parseCPUModel(readFile("/proc/cpuinfo"))

	h.Uptime = linuxUptime(readFile("/proc/uptime"))
	return h, nil
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseOSRelease extracts PRETTY_NAME from /etc/os-release content.
func parseOSRelease(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			v := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}

// parseCPUModel extracts the first "model name" from /proc/cpuinfo content.
func parseCPUModel(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "model name") {
			if i := strings.Index(line, ":"); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

func linuxUptime(s string) time.Duration {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(f[0], 64)
	return time.Duration(secs) * time.Second
}

func (linuxCollector) Disk(ctx context.Context) ([]Disk, error) {
	out, err := sys.Run(ctx, "df", "-k")
	if err != nil {
		return nil, err
	}
	return parseDf(out), nil
}

func (linuxCollector) Net(ctx context.Context) ([]Net, error) {
	out, err := sys.Run(ctx, "ip", "-o", "-4", "addr")
	if err != nil {
		return nil, err
	}
	return parseIPAddr(out), nil
}
