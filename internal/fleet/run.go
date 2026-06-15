package fleet

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

const maxParallel = 8

// confirm is the gate for live fan-out exec; overridable in tests.
var confirm = tui.Confirm

func sshBase(n Node) []string {
	return []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=6", n.SSHUser + "@" + n.IP}
}

// Health probes every node in parallel: offline nodes are skipped, online ones
// get an ssh reachability check. Reports one line per node + a summary.
func Health(ctx context.Context, rep *report.Reporter, nodes []Node) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	var mu sync.Mutex
	var ok, down, unreach int

	for _, n := range nodes {
		if !n.Online {
			rep.Skip("health", n.Label(), "offline")
			down++
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := sys.Run(ctx, "ssh", append(sshBase(n), "true")...)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				rep.Fail("health", n.Label(), err)
				unreach++
				return
			}
			rep.OK("health", n.Label())
			ok++
		}(n)
	}
	wg.Wait()
	rep.Info("fleet-health",
		report.KV{K: "ok", V: strconv.Itoa(ok)},
		report.KV{K: "offline", V: strconv.Itoa(down)},
		report.KV{K: "unreachable", V: strconv.Itoa(unreach)},
	)
}

// SSH opens an interactive shell to the node.
func SSH(ctx context.Context, n Node) error {
	return sys.RunInteractive(ctx, "ssh", n.SSHUser+"@"+n.IP)
}

// Run fan-outs cmd across the given nodes. dryRun prints the plan and runs
// nothing; otherwise it confirms (unless yes), then runs in parallel over
// BatchMode ssh, printing per-node output to stdout and ok/fail via the report.
func Run(ctx context.Context, rep *report.Reporter, nodes []Node, cmd []string, dryRun, yes bool) error {
	cmdStr := strings.Join(cmd, " ")
	if dryRun {
		rep.SetDryRun(true) // so Do emits status=dry
		for _, n := range nodes {
			rep.Do("run", n.Label())
		}
		fmt.Printf("would run on %d node(s): %s\n", len(nodes), cmdStr)
		return nil
	}
	if len(nodes) == 0 {
		rep.Info("no matching nodes")
		return nil
	}
	if !yes && !confirm(fmt.Sprintf("Run %q on %d node(s)?", cmdStr, len(nodes))) {
		rep.Info("aborted")
		return nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	var mu sync.Mutex
	var ok, failed int

	for _, n := range nodes {
		if !n.Online {
			rep.Skip("run", n.Label(), "offline")
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := sys.Run(ctx, "ssh", append(sshBase(n), cmd...)...)
			mu.Lock()
			defer mu.Unlock()
			fmt.Printf("=== %s ===\n%s\n", n.Label(), strings.TrimRight(out, "\n"))
			if err != nil {
				rep.Fail("run", n.Label(), err)
				failed++
				return
			}
			rep.OK("run", n.Label())
			ok++
		}(n)
	}
	wg.Wait()
	rep.Info("fleet-run", report.KV{K: "ok", V: strconv.Itoa(ok)}, report.KV{K: "failed", V: strconv.Itoa(failed)})
	if failed > 0 {
		return fmt.Errorf("fleet run: %d node(s) failed", failed)
	}
	return nil
}
