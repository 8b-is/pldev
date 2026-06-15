package fleet

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func stubTry(t *testing.T, ok func(user, ip string) bool) {
	t.Helper()
	old := trySSHUser
	t.Cleanup(func() { trySSHUser = old })
	trySSHUser = func(_ context.Context, user, ip string) bool { return ok(user, ip) }
}

func TestProbeNodeFirstMatch(t *testing.T) {
	stubTry(t, func(user, _ string) bool { return user == "dev" })
	got := probeNode(context.Background(), Node{IP: "100.0.0.1"}, []string{"root", "ubuntu", "dev"})
	if got != "dev" {
		t.Errorf("want dev, got %q", got)
	}
}

func TestProbeNodeTriesCurrentUserFirst(t *testing.T) {
	var order []string
	stubTry(t, func(user, _ string) bool { order = append(order, user); return user == "ubuntu" })
	got := probeNode(context.Background(), Node{IP: "100.0.0.1", SSHUser: "ubuntu"}, []string{"root", "dev"})
	if got != "ubuntu" {
		t.Errorf("want ubuntu, got %q", got)
	}
	if len(order) != 1 || order[0] != "ubuntu" {
		t.Errorf("configured user should be tried first (and short-circuit): %v", order)
	}
}

func TestProbeNodeNoneWork(t *testing.T) {
	stubTry(t, func(string, string) bool { return false })
	if got := probeNode(context.Background(), Node{IP: "100.0.0.1"}, []string{"root", "dev"}); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestProbeNodeNoDuplicateTries(t *testing.T) {
	var n int
	stubTry(t, func(string, string) bool { n++; return false })
	// configured user duplicated in candidates must be tried once
	probeNode(context.Background(), Node{IP: "100.0.0.1", SSHUser: "root"}, []string{"root", "root", "dev"})
	if n != 2 { // root, dev — not root three times
		t.Errorf("expected 2 distinct tries, got %d", n)
	}
}

func TestProbeUsersWritesCache(t *testing.T) {
	origTry, origGather := trySSHUser, gatherHW
	defer func() { trySSHUser, gatherHW = origTry, origGather }()
	trySSHUser = func(ctx context.Context, user, ip string) bool { return user == "root" }
	gatherHW = func(ctx context.Context, user, ip string) (HWInfo, error) {
		return HWInfo{Serial: "S1", CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}, nil
	}

	path := filepath.Join(t.TempDir(), "cache.json")
	rep := report.New(io.Discard, "fleet")
	nodes := []Node{{Name: "n1", IP: "100.0.0.1", Online: true}}
	ProbeUsers(context.Background(), rep, nodes, nil, path)

	cache, err := LoadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	got := cache["n1"]
	if got.SSHUser != "root" || got.HW.Serial != "S1" || got.HW.Cores != 16 {
		t.Fatalf("cache not written correctly: %+v", got)
	}
}
