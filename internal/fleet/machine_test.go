package fleet

import "testing"

func TestPickCanonical(t *testing.T) {
	if got := pickCanonical([]Node{{Name: "a"}}); got != 0 {
		t.Fatalf("single=%d", got)
	}
	off := []Node{{Name: "b", Online: false}, {Name: "a", Online: false}}
	if got := pickCanonical(off); off[got].Name != "a" {
		t.Fatalf("all-offline want a, got %q", off[got].Name)
	}
	mix := []Node{{Name: "a", Online: false}, {Name: "b", Online: true}}
	if got := pickCanonical(mix); mix[got].Name != "b" {
		t.Fatalf("online want b, got %q", mix[got].Name)
	}
	pin := []Node{{Name: "a", Online: true}, {Name: "b", Online: true, Primary: true}}
	if got := pickCanonical(pin); pin[got].Name != "b" {
		t.Fatalf("primary want b, got %q", pin[got].Name)
	}
	two := []Node{{Name: "b", Online: true}, {Name: "a", Online: true}}
	if got := pickCanonical(two); two[got].Name != "a" {
		t.Fatalf("lexical want a, got %q", two[got].Name)
	}
	stale := []Node{{Name: "a", Online: false, Primary: true}, {Name: "b", Online: true}}
	if got := pickCanonical(stale); stale[got].Name != "b" {
		t.Fatalf("online-over-stale-pin want b, got %q", stale[got].Name)
	}
}

func TestGroupByMachine(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "lodris-macbook-pro", Nickname: "Squire", Online: true, HW: HWInfo{Serial: "RJ6W"}},
		{Name: "warforge", Nickname: "Warforge", Online: true, HW: HWInfo{Serial: "XEON"}},
		{Name: "ghost", Nickname: "Ghost", HW: HWInfo{Serial: ""}},
	}
	ms := GroupByMachine(nodes)
	if len(ms) != 3 {
		t.Fatalf("want 3 machines, got %d", len(ms))
	}
	var mac *Machine
	for i := range ms {
		if ms[i].Serial == "RJ6W" {
			mac = &ms[i]
		}
	}
	if mac == nil || len(mac.Nodes) != 2 || !mac.HasDup() {
		t.Fatalf("RJ6W machine wrong: %+v", mac)
	}
	if mac.Canonical().Name != "dev-main" {
		t.Fatalf("canonical want dev-main, got %q", mac.Canonical().Name)
	}
	if len(mac.Aliases()) != 1 || mac.Aliases()[0].Name != "lodris-macbook-pro" {
		t.Fatalf("aliases wrong: %+v", mac.Aliases())
	}
}

func TestStaleAliases(t *testing.T) {
	nodes := []Node{
		{Name: "dev-main", Nickname: "Forgemaster", Online: true, IP: "100.0.0.1", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "old-reg", Nickname: "Forgemaster", Online: false, IP: "100.0.0.9", HW: HWInfo{Serial: "RJ6W"}},
		{Name: "solo", Online: true, HW: HWInfo{Serial: "X"}},
	}
	ms := GroupByMachine(nodes)
	stale := staleAliases(ms)
	if len(stale) != 1 || stale[0].Name != "old-reg" || stale[0].IP != "100.0.0.9" {
		t.Fatalf("staleAliases wrong: %+v", stale)
	}
}
