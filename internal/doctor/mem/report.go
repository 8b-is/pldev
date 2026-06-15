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
