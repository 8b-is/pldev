package fleet

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
	"github.com/peterlodri-sec/pldev/internal/tui"
)

const maxParallel = 8

// confirm is the gate for live fan-out exec; overridable in tests.
var confirm = tui.Confirm

// sshOpts are the common non-interactive ssh flags: key-auth only, accept a
// node's host key on first contact (TOFU — else unknown hosts are rejected and
// look like access failures), bounded connect.
var sshOpts = []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=6"}

func sshBase(n Node) []string {
	return append(append([]string{}, sshOpts...), n.SSHUser+"@"+n.IP)
}

// Health states, from worst to best.
const (
	StateOffline     = "offline"     // tailscale reports the node down
	StateUnreachable = "unreachable" // online per tailscale but TCP :22 won't connect (stale/firewalled)
	StateReachable   = "reachable"   // ssh port open, but the ssh command failed (wrong user/key)
	StateSSHOK       = "ssh-ok"      // ssh handshake + command succeeded — fully usable
)

// classify derives the health state from the three probes. Pure + testable.
func classify(online, pingOK, sshOK bool) string {
	switch {
	case !online:
		return StateOffline
	case !pingOK:
		return StateUnreachable
	case !sshOK:
		return StateReachable
	default:
		return StateSSHOK
	}
}

// sshPort is the port probed for the reachable tier.
const sshPort = "22"

// Health probes every machine in parallel through three tiers — tailscale online,
// TCP :22 open (reachable), ssh command (ssh-ok) — and reports one line per machine
// with its state, plus a per-state summary. Stale offline aliases are listed with a
// hint to delete them in the tailnet admin.
func Health(ctx context.Context, rep *report.Reporter, machines []Machine) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	var mu sync.Mutex
	counts := map[string]int{}

	emitLabel := func(label, state string) {
		mu.Lock()
		defer mu.Unlock()
		counts[state]++
		kv := report.KV{K: "state", V: state}
		switch state {
		case StateSSHOK, StateReachable:
			rep.OK("health", label, kv)
		case StateOffline:
			rep.Skip("health", label, state)
		default: // unreachable
			rep.Fail("health", label, fmt.Errorf("online but unreachable"), kv)
		}
	}

	for _, m := range machines {
		n := m.Canonical()
		if !n.Online {
			emitLabel(m.Label(), StateOffline)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(label string, n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, derr := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(n.IP, sshPort))
			tcpOK := derr == nil
			if conn != nil {
				conn.Close()
			}
			sshOK := false
			if tcpOK {
				_, serr := sys.Run(ctx, "ssh", append(sshBase(n), "true")...)
				sshOK = serr == nil
			}
			emitLabel(label, classify(true, tcpOK, sshOK))
		}(m.Label(), n)
	}
	wg.Wait()
	rep.Info("fleet-health",
		report.KV{K: "ssh-ok", V: strconv.Itoa(counts[StateSSHOK])},
		report.KV{K: "reachable", V: strconv.Itoa(counts[StateReachable])},
		report.KV{K: "unreachable", V: strconv.Itoa(counts[StateUnreachable])},
		report.KV{K: "offline", V: strconv.Itoa(counts[StateOffline])},
	)
	for _, s := range staleAliases(machines) {
		rep.Info("stale-alias",
			report.KV{K: "machine", V: s.Machine},
			report.KV{K: "name", V: s.Name},
			report.KV{K: "ip", V: s.IP},
			report.KV{K: "hint", V: "delete in tailnet admin"})
	}
}

// SSH opens an interactive shell to the node (accepts the host key on first
// contact; interactive auth otherwise — no BatchMode).
func SSH(ctx context.Context, n Node) error {
	return sys.RunInteractive(ctx, "ssh", "-o", "StrictHostKeyChecking=accept-new", n.SSHUser+"@"+n.IP)
}

// Run fan-outs cmd across the given machines. dryRun prints the plan and runs
// nothing; otherwise it confirms (unless yes), then runs in parallel over
// BatchMode ssh on each machine's canonical node, printing per-machine output
// to stdout and ok/fail via the report.
func Run(ctx context.Context, rep *report.Reporter, machines []Machine, cmd []string, dryRun, yes bool) error {
	cmdStr := strings.Join(cmd, " ")
	if dryRun {
		rep.SetDryRun(true) // so Do emits status=dry
		for _, m := range machines {
			rep.Do("run", m.Label())
		}
		fmt.Printf("would run on %d node(s): %s\n", len(machines), cmdStr)
		return nil
	}
	if len(machines) == 0 {
		rep.Info("no matching nodes")
		return nil
	}
	if !yes && !confirm(fmt.Sprintf("Run %q on %d node(s)?", cmdStr, len(machines))) {
		rep.Info("aborted")
		return nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	var mu sync.Mutex
	var ok, failed int

	for _, m := range machines {
		n := m.Canonical()
		if !n.Online {
			rep.Skip("run", m.Label(), "offline")
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(label string, n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := sys.Run(ctx, "ssh", append(sshBase(n), cmd...)...)
			mu.Lock()
			defer mu.Unlock()
			fmt.Printf("=== %s ===\n%s\n", label, strings.TrimRight(out, "\n"))
			if err != nil {
				rep.Fail("run", label, err)
				failed++
				return
			}
			rep.OK("run", label)
			ok++
		}(m.Label(), n)
	}
	wg.Wait()
	rep.Info("fleet-run", report.KV{K: "ok", V: strconv.Itoa(ok)}, report.KV{K: "failed", V: strconv.Itoa(failed)})
	if failed > 0 {
		return fmt.Errorf("fleet run: %d node(s) failed", failed)
	}
	return nil
}
