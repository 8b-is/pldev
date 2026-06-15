package fleet

import "testing"

func TestHealthCountsMachinesOnce(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, IP: "100.0.0.1", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "old-reg", Nickname: "Forgemaster", Online: false, IP: "100.0.0.9", HW: HWInfo{Serial: "RJ6W"}},
	}
	ms := GroupByMachine(nodes)
	if len(ms) != 1 {
		t.Fatalf("want 1 machine, got %d", len(ms))
	}
	if s := staleAliases(ms); len(s) != 1 || s[0].Name != "old-reg" {
		t.Fatalf("expected old-reg as stale: %+v", s)
	}
}
