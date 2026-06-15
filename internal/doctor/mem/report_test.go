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
