package fleet

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func TestGenSSHConfig(t *testing.T) {
	out := GenSSHConfig(Merge(tsNodes(), cfg(), nil))
	for _, want := range []string{sshMarkBegin, sshMarkEnd, "Host Katedralis dev-cx53", "HostName 100.0.0.1", "User dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("ssh-config missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-box") {
		t.Error("excluded node must not appear")
	}
}

func TestWriteSSHConfigIdempotentRegion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	os.WriteFile(p, []byte("Host existing\n    HostName 1.2.3.4\n"), 0o600)

	block := GenSSHConfig(Merge(tsNodes(), cfg(), nil))
	if err := WriteSSHConfig(p, block); err != nil {
		t.Fatal(err)
	}
	if err := WriteSSHConfig(p, block); err != nil { // second write
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	// user's pre-existing entry preserved
	if !strings.Contains(s, "Host existing") {
		t.Error("must preserve non-managed entries")
	}
	// managed block appears exactly once (idempotent, not duplicated)
	if n := strings.Count(s, sshMarkBegin); n != 1 {
		t.Errorf("managed block should appear once, got %d", n)
	}
	// a backup was made
	entries, _ := os.ReadDir(dir)
	var baks int
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			baks++
		}
	}
	if baks == 0 {
		t.Error("expected a backup of the existing config")
	}
}

func TestSyncSignersDryRun(t *testing.T) {
	var buf bytes.Buffer
	rep := report.New(&buf, "fleet")
	nodes := Merge(tsNodes(), cfg(), nil) // dev-cx53 online, rpi offline
	err := SyncSigners(context.Background(), rep, nodes, []byte("anchor"), filepath.Join(t.TempDir(), "as"), true, false)
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(buf.String(), "status=dry") {
		t.Errorf("dry-run should log dry: %q", buf.String())
	}
}

func TestReachableSkipsOfflineAndNoUser(t *testing.T) {
	r := reachable(Merge(tsNodes(), cfg(), nil))
	for _, n := range r {
		if !n.Online || n.IP == "" || n.SSHUser == "" {
			t.Errorf("reachable returned a bad node: %+v", n)
		}
		if n.Name == "rpi3b-recon" {
			t.Error("offline node must not be reachable")
		}
	}
}
