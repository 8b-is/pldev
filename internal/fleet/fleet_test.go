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
	nodes := Merge(tsNodes(), cfg(), nil)
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
	nodes := Merge(tsNodes(), cfg(), nil)
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
	nodes := Merge(tsNodes(), cfg(), nil)
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
	out := Render(Merge(tsNodes(), cfg(), nil))
	for _, want := range []string{"NICK", "HOST", "Katedralis", "dev-cx53"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "off") {
		t.Error("offline status not rendered")
	}
}

func TestClassifyHealthStates(t *testing.T) {
	cases := []struct {
		online, ping, ssh bool
		want              string
	}{
		{false, false, false, StateOffline},
		{true, false, false, StateUnreachable},
		{true, true, false, StateReachable},
		{true, true, true, StateSSHOK},
		{false, true, true, StateOffline}, // offline dominates
	}
	for _, c := range cases {
		if got := classify(c.online, c.ping, c.ssh); got != c.want {
			t.Errorf("classify(%v,%v,%v)=%q want %q", c.online, c.ping, c.ssh, got, c.want)
		}
	}
}

func TestRunDryRunNoExec(t *testing.T) {
	var buf strings.Builder
	rep := report.New(&buf, "fleet")
	nodes := Merge(tsNodes(), cfg(), nil)
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
	if err := Run(context.Background(), rep, Merge(tsNodes(), cfg(), nil), []string{"echo"}, false, false); err != nil {
		t.Fatalf("declined run err: %v", err)
	}
	if !strings.Contains(buf.String(), "aborted") {
		t.Errorf("expected aborted: %q", buf.String())
	}
}

func TestDetectDups(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "crabcc", Nickname: "Warforge", HW: HWInfo{Serial: "XEON1"}},
		{Name: "no-probe", Nickname: "Ghost", HW: HWInfo{Serial: ""}},
		{Name: "no-probe-2", Nickname: "Ghost2", HW: HWInfo{Serial: ""}},
	}
	detectDups(nodes)
	if nodes[0].Dup {
		t.Error("first node with a serial must not be marked dup")
	}
	if !nodes[1].Dup || nodes[1].DupOf != "Forgemaster" {
		t.Errorf("Squire should be dup of Forgemaster, got Dup=%v DupOf=%q", nodes[1].Dup, nodes[1].DupOf)
	}
	if nodes[2].Dup {
		t.Error("unique serial must not be dup")
	}
	if nodes[3].Dup || nodes[4].Dup {
		t.Error("empty serials must never match each other")
	}
}

func TestMergeFillsHWFromCache(t *testing.T) {
	ts := []TSNode{{Name: "dev-main", IP: "100.0.0.1", Online: true}}
	cfg := FleetConfig{Node: map[string]NodeConfig{"dev-main": {Nickname: "Forgemaster"}}}
	cache := FleetCache{"dev-main": {HW: HWInfo{Serial: "RJ6W", CPU: "Apple M3 Max", Cores: 16}}}
	nodes := Merge(ts, cfg, cache)
	if len(nodes) != 1 || nodes[0].HW.CPU != "Apple M3 Max" || nodes[0].HW.Cores != 16 {
		t.Fatalf("Merge did not fill HW from cache: %+v", nodes)
	}
}

func TestRenderWideHardware(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", OS: "macOS", Online: true, Role: "command",
			Caps: []string{"control"}, HW: HWInfo{CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", OS: "macOS", Online: true,
			HW: HWInfo{CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}, Dup: true, DupOf: "Forgemaster"},
		{Name: "blank", Nickname: "Blank", Online: false},
	}
	out := Render(nodes)
	if !strings.Contains(out, "NICK") || !strings.Contains(out, "CPU") || !strings.Contains(out, "ARCH") {
		t.Fatalf("missing header columns:\n%s", out)
	}
	if !strings.Contains(out, "M3 Max") || !strings.Contains(out, "64G") {
		t.Fatalf("missing hw values:\n%s", out)
	}
	if !strings.Contains(out, "⚠dup") {
		t.Fatalf("dup marker absent:\n%s", out)
	}
	if strings.Contains(out, "0G") {
		t.Fatalf("zero mem must render '-', not 0G:\n%s", out)
	}
}

func TestTrunc(t *testing.T) {
	if got := trunc("lodris-macbook-pro", 12); got != "lodris-mac.." {
		t.Fatalf("trunc=%q", got)
	}
	if got := trunc("short", 12); got != "short" {
		t.Fatalf("trunc must leave short strings: %q", got)
	}
}
