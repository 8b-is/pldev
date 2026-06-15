package sysinfo

import (
	"strings"
	"testing"
	"time"

	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func sampleInfo() SysInfo {
	return SysInfo{
		Hostname: "testbox",
		OS:       "macOS 15.0",
		Kernel:   "24.0.0",
		Model:    "Mac15,3",
		Arch:     "arm64",
		CPU:      "Apple M3",
		Cores:    8,
		Uptime:   20 * 24 * time.Hour,
		Mem:      probe.MemStat{Total: 36 << 30, Used: 27 << 30, Free: 9 << 30, Available: 11 << 30},
		Swap:     probe.SwapStat{Total: 4 << 30, Used: 2 << 30},
		Disks: []Disk{
			{Mount: "/", Total: 500 << 30, Used: 420 << 30, Free: 80 << 30, UsePct: 84},
		},
		Nets: []Net{
			{Name: "en0", IPv4: []string{"192.168.0.113"}},
		},
	}
}

func TestRenderPanelPlain(t *testing.T) {
	out := RenderPanel(sampleInfo(), false)

	for _, want := range []string{
		"SYSTEM", "CPU & MEM", "DISK", "NETWORK",
		"testbox", "macOS 15.0", "24.0.0", "Mac15,3", "arm64",
		"Apple M3", "en0", "192.168.0.113", "/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain render missing %q\n%s", want, out)
		}
	}
	// pure ASCII box: must use + - | and contain no unicode box-drawing
	if !strings.Contains(out, "+") || !strings.Contains(out, "|") {
		t.Errorf("expected ASCII box chars + and |")
	}
	for _, bad := range []string{"─", "│", "┌", "┐", "└", "┘", "├", "┤"} {
		if strings.Contains(out, bad) {
			t.Errorf("found unicode box char %q; must be pure ASCII", bad)
		}
	}
	// no ANSI escape when color disabled
	if strings.Contains(out, "\x1b[") {
		t.Errorf("color disabled but found ANSI escape")
	}
}
