package fleet

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
)

// DefaultUserCandidates are tried (in order) to find a node's working ssh user.
var DefaultUserCandidates = []string{"root", "lodripeter", "ubuntu", "admin", "dev", "peter", "ec2-user"}

// trySSHUser reports whether <user>@<ip> accepts a key-auth ssh command.
// Indirection point for tests.
var trySSHUser = func(ctx context.Context, user, ip string) bool {
	args := append(append([]string{}, sshOpts...), user+"@"+ip, "true")
	_, err := sys.Run(ctx, "ssh", args...)
	return err == nil
}

// ProbeResult is the discovered ssh user for a node ("" = none of the candidates worked).
type ProbeResult struct {
	Node Node
	User string
}

// probeNode returns the first candidate ssh user that authenticates. The node's
// currently-configured user is tried first (it's the likeliest answer).
func probeNode(ctx context.Context, n Node, candidates []string) string {
	tried := map[string]bool{}
	order := candidates
	if n.SSHUser != "" {
		order = append([]string{n.SSHUser}, candidates...)
	}
	for _, u := range order {
		if u == "" || tried[u] {
			continue
		}
		tried[u] = true
		if trySSHUser(ctx, u, n.IP) {
			return u
		}
	}
	return ""
}

// ProbeUsers probes the given nodes (online + with an IP) in parallel, prints a
// report + a fleet.toml snippet for the matches, and returns the results.
// Read-only (ssh true probes); it does not write config.
func ProbeUsers(ctx context.Context, rep *report.Reporter, nodes []Node, candidates []string) []ProbeResult {
	if len(candidates) == 0 {
		candidates = DefaultUserCandidates
	}
	var targets []Node
	for _, n := range nodes {
		if n.Online && n.IP != "" {
			targets = append(targets, n)
		}
	}

	results := make([]ProbeResult, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	for i, n := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, n Node) {
			defer wg.Done()
			defer func() { <-sem }()
			user := probeNode(ctx, n, candidates)
			results[i] = ProbeResult{Node: n, User: user}
			if user == "" {
				rep.Fail("probe-user", n.Label(), fmt.Errorf("no candidate ssh user worked"))
			} else {
				rep.OK("probe-user", n.Label(), report.KV{K: "user", V: user})
			}
		}(i, n)
	}
	wg.Wait()

	// fleet.toml snippet for the matches (data → stdout)
	var found int
	fmt.Println("\n# discovered ssh_user — add to ~/.config/pldev/fleet.toml:")
	for _, r := range results {
		if r.User == "" {
			continue
		}
		found++
		fmt.Printf("[node.%s]\nssh_user = %q\n", r.Node.Name, r.User)
	}
	rep.Info("fleet-probe-user", report.KV{K: "found", V: strconv.Itoa(found)}, report.KV{K: "nodes", V: strconv.Itoa(len(targets))})
	return results
}
