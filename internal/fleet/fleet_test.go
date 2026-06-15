package fleet

import (
	"context"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func tsNodes() []TSNode {
	return []TSNode{
		{Name: "dev-cx53", IP: "100.0.0.1", OS: "linux", Online: true},
		{Name: "rpi3b-recon", IP: "100.0.0.2", OS: "linux", Online: false},
		{Name: "secret-box", IP: "100.0.0.3", OS: "linux", Online: true},
	}
}

func cfg() FleetConfig {
	return FleetConfig{
		DefaultSSHUser: "lodripeter",
		Node: map[string]NodeConfig{
			"dev-cx53":    {Nickname: "Katedralis", Role: "hetzner", Caps: []string{"docker", "ollama"}, SSHUser: "dev"},
			"secret-box":  {Exclude: true},
			"rpi3b-recon": {Role: "pi"},
		},
	}
}

func TestMergeAnnotatesExcludesAndUsers(t *testing.T) {
	nodes := Merge(tsNodes(), cfg())
	if len(nodes) != 2 {
		t.Fatalf("excluded node should be dropped; got %d", len(nodes))
	}
	var cx Node
	for _, n := range nodes {
		if n.Name == "dev-cx53" {
			cx = n
		}
		if n.Name == "secret-box" {
			t.Error("excluded node present")
		}
	}
	if cx.Role != "hetzner" || cx.SSHUser != "dev" {
		t.Errorf("annotation wrong: %+v", cx)
	}
	// rpi uses default ssh user
	for _, n := range nodes {
		if n.Name == "rpi3b-recon" && n.SSHUser != "lodripeter" {
			t.Errorf("default ssh user not applied: %q", n.SSHUser)
		}
	}
}

func TestSelect(t *testing.T) {
	nodes := Merge(tsNodes(), cfg())
	if len(Select(nodes, "all")) != 2 {
		t.Error("all")
	}
	if r := Select(nodes, "@hetzner"); len(r) != 1 || r[0].Name != "dev-cx53" {
		t.Errorf("@role: %v", r)
	}
	if r := Select(nodes, "rpi3b-recon"); len(r) != 1 {
		t.Error("by name")
	}
	// select by nickname (case-insensitive)
	if r := Select(nodes, "katedralis"); len(r) != 1 || r[0].Name != "dev-cx53" {
		t.Errorf("by nickname: %v", r)
	}
	if Select(nodes, "nope") != nil {
		t.Error("unknown selector should be empty")
	}
}

func TestLabelFallsBackToName(t *testing.T) {
	nodes := Merge(tsNodes(), cfg())
	for _, n := range nodes {
		if n.Name == "dev-cx53" && n.Label() != "Katedralis" {
			t.Errorf("nickname should be the label, got %q", n.Label())
		}
		if n.Name == "rpi3b-recon" && n.Label() != "rpi3b-recon" {
			t.Errorf("no nickname => label is the name, got %q", n.Label())
		}
	}
}

func TestRender(t *testing.T) {
	out := Render(Merge(tsNodes(), cfg()))
	for _, want := range []string{"NICK", "HOST", "Katedralis", "dev-cx53"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "offline") {
		t.Error("offline status not rendered")
	}
}

func TestRunDryRunNoExec(t *testing.T) {
	var buf strings.Builder
	rep := report.New(&buf, "fleet")
	nodes := Merge(tsNodes(), cfg())
	// dry-run must not touch ssh; returns nil regardless of reachability
	if err := Run(context.Background(), rep, nodes, []string{"uname", "-a"}, true, false); err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(buf.String(), "status=dry") {
		t.Errorf("dry-run should log dry: %q", buf.String())
	}
}

func TestRunConfirmDeclineNoExec(t *testing.T) {
	old := confirm
	defer func() { confirm = old }()
	confirm = func(string) bool { return false }
	var buf strings.Builder
	rep := report.New(&buf, "fleet")
	// declined → returns nil before any ssh
	if err := Run(context.Background(), rep, Merge(tsNodes(), cfg()), []string{"echo"}, false, false); err != nil {
		t.Fatalf("declined run err: %v", err)
	}
	if !strings.Contains(buf.String(), "aborted") {
		t.Errorf("expected aborted: %q", buf.String())
	}
}
