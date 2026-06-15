package fleet

import (
	"net/netip"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func addrs(ss ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

func TestFromStatus(t *testing.T) {
	st := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			DNSName: "m3-macbook.tail.ts.net.", OS: "macOS", Online: false,
			TailscaleIPs: addrs("fd7a::1", "100.123.33.67"),
		},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			key.NewNode().Public(): {DNSName: "dev-cx53.tail.ts.net.", OS: "linux", Online: true, TailscaleIPs: addrs("100.105.72.88")},
			key.NewNode().Public(): {DNSName: "rpi3b-recon.tail.ts.net.", OS: "linux", Online: false, TailscaleIPs: addrs("100.95.247.98")},
		},
	}
	nodes := fromStatus(st)
	if len(nodes) != 3 {
		t.Fatalf("want 3 nodes, got %d", len(nodes))
	}
	byName := map[string]TSNode{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	if !byName["m3-macbook"].Online {
		t.Error("Self must be forced online")
	}
	if byName["m3-macbook"].IP != "100.123.33.67" {
		t.Errorf("Self should pick the v4 IP, got %q", byName["m3-macbook"].IP)
	}
	if byName["dev-cx53"].IP != "100.105.72.88" || !byName["dev-cx53"].Online {
		t.Errorf("peer mapped wrong: %+v", byName["dev-cx53"])
	}
	if byName["rpi3b-recon"].Online {
		t.Error("offline peer should stay offline")
	}
}

func TestFirstV4(t *testing.T) {
	if got := firstV4(addrs("fd7a::1", "100.1.2.3")); got != "100.1.2.3" {
		t.Errorf("firstV4 = %q", got)
	}
	if got := firstV4(nil); got != "" {
		t.Errorf("firstV4(nil) = %q", got)
	}
}

func TestShortName(t *testing.T) {
	if got := shortName("dev-cx53.tail2870dc.ts.net.", "host"); got != "dev-cx53" {
		t.Errorf("shortName = %q", got)
	}
	if got := shortName("", "fallback"); got != "fallback" {
		t.Errorf("shortName fallback = %q", got)
	}
}
