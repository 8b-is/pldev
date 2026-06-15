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
