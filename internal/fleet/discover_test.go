package fleet

import "testing"

const fixture = `{
  "Self":  {"HostName":"m3","DNSName":"m3-macbook.tail.ts.net.","OS":"macOS","Online":false,"TailscaleIPs":["100.123.33.67","fd7a::1"]},
  "Peer": {
    "k1": {"HostName":"dev-cx53","DNSName":"dev-cx53.tail.ts.net.","OS":"linux","Online":true,"TailscaleIPs":["100.105.72.88"],"Tags":["tagged-devices"]},
    "k2": {"HostName":"rpi3b","DNSName":"rpi3b-recon.tail.ts.net.","OS":"linux","Online":false,"TailscaleIPs":["100.95.247.98"]}
  }
}`

func TestParseStatus(t *testing.T) {
	nodes, err := ParseStatus([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("want 3 nodes, got %d", len(nodes))
	}
	byName := map[string]TSNode{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	if _, ok := byName["m3-macbook"]; !ok {
		t.Errorf("short name from DNSName missing: %v", byName)
	}
	if !byName["m3-macbook"].Online {
		t.Error("Self must be forced online")
	}
	if byName["dev-cx53"].IP != "100.105.72.88" {
		t.Errorf("v4 IP wrong: %q", byName["dev-cx53"].IP)
	}
	if byName["rpi3b-recon"].Online {
		t.Error("offline peer should stay offline")
	}
}

func TestFirstV4(t *testing.T) {
	if got := firstV4([]string{"fd7a::1", "100.1.2.3"}); got != "100.1.2.3" {
		t.Errorf("firstV4 = %q", got)
	}
}
