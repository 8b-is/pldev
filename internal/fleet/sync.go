package fleet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
)

// reachable returns online nodes that have an IP + ssh user (sync/ssh targets).
func reachable(nodes []Node) []Node {
	var out []Node
	for _, n := range nodes {
		if n.Online && n.IP != "" && n.SSHUser != "" {
			out = append(out, n)
		}
	}
	return out
}

// reachableMachines returns machines whose canonical alias is online with an IP
// and an ssh user (sync targets).
func reachableMachines(machines []Machine) []Machine {
	var out []Machine
	for _, m := range machines {
		n := m.Canonical()
		if n.Online && n.IP != "" && n.SSHUser != "" {
			out = append(out, m)
		}
	}
	return out
}

// SyncSigners pushes the PUBLIC allowed_signers anchor to every reachable machine's
// ~/.config/pldev/allowed_signers (so all nodes verify the same identities). It
// also writes the anchor locally (promoting the embedded default to the runtime
// file). Private keys are never touched. Gated: dry-run + confirm + BatchMode.
func SyncSigners(ctx context.Context, rep *report.Reporter, machines []Machine, anchor []byte, localPath string, dryRun, yes bool) error {
	targets := reachableMachines(machines)
	if dryRun {
		rep.SetDryRun(true)
		for _, m := range targets {
			rep.Do("sync-signers", m.Label())
		}
		fmt.Printf("would sync allowed_signers (%d bytes) to %d reachable node(s) + local\n", len(anchor), len(targets))
		return nil
	}
	if !yes && !confirm(fmt.Sprintf("Sync allowed_signers to %d node(s) + local?", len(targets))) {
		rep.Info("aborted")
		return nil
	}

	// local first: promote embedded -> runtime file
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err == nil {
		if err := os.WriteFile(localPath, anchor, 0o644); err == nil {
			rep.OK("sync-signers", "local")
		} else {
			rep.Fail("sync-signers", "local", err)
		}
	}

	tmp, err := os.CreateTemp("", "pldev-signers-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Write(anchor)
	tmp.Close()

	var failed int
	for _, m := range targets {
		n := m.Canonical()
		rep.Do("sync-signers", m.Label())
		dst := n.SSHUser + "@" + n.IP
		bm := append([]string{}, sshOpts...) // BatchMode + accept-new + timeout
		if _, err := sys.Run(ctx, "ssh", append(bm, dst, "mkdir -p ~/.config/pldev")...); err != nil {
			rep.Fail("sync-signers", m.Label(), err)
			failed++
			continue
		}
		scpArgs := append(bm, tmp.Name(), dst+":.config/pldev/allowed_signers")
		if _, err := sys.Run(ctx, "scp", scpArgs...); err != nil {
			rep.Fail("sync-signers", m.Label(), err)
			failed++
			continue
		}
		rep.OK("sync-signers", m.Label())
	}
	rep.Info("fleet-sync-signers", report.KV{K: "nodes", V: strconv.Itoa(len(targets))}, report.KV{K: "failed", V: strconv.Itoa(failed)})
	if failed > 0 {
		return fmt.Errorf("sync-signers: %d node(s) failed", failed)
	}
	return nil
}

const (
	sshMarkBegin = "# >>> pldev fleet >>>"
	sshMarkEnd   = "# <<< pldev fleet <<<"
)

// GenSSHConfig renders the fleet as an ~/.ssh/config managed block: each node is
// reachable as both its nickname and its tailnet name.
func GenSSHConfig(nodes []Node) string {
	var b strings.Builder
	b.WriteString(sshMarkBegin + "\n")
	for _, n := range nodes {
		if n.IP == "" || n.SSHUser == "" {
			continue
		}
		host := n.Name
		if n.Nickname != "" {
			host = n.Nickname + " " + n.Name
		}
		fmt.Fprintf(&b, "Host %s\n    HostName %s\n    User %s\n", host, n.IP, n.SSHUser)
	}
	b.WriteString(sshMarkEnd + "\n")
	return b.String()
}

// WriteSSHConfig replaces (or appends) the pldev-managed block in the ssh config
// at path, backing up an existing file first. Only the managed region is touched.
func WriteSSHConfig(path, block string) error {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(old) > 0 {
		bak := path + ".bak-" + time.Now().Format("20060102-150405")
		if werr := os.WriteFile(bak, old, 0o600); werr != nil {
			return fmt.Errorf("backup: %w", werr)
		}
	}
	s := string(old)
	if i := strings.Index(s, sshMarkBegin); i >= 0 {
		if j := strings.Index(s, sshMarkEnd); j >= i {
			s = s[:i] + strings.TrimSuffix(block, "\n") + s[j+len(sshMarkEnd):]
		}
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += block
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(s), 0o600)
}
