package fleet

import "sort"

// Machine is one physical box (keyed on hardware serial) and the tailnet nodes
// that resolve to it. A serial-less node forms its own singleton Machine.
type Machine struct {
	Serial    string // "" for singletons (never-probed / no serial)
	Nodes     []Node // aliases, sorted by Name; len >= 1
	canonical int    // index into Nodes of the operational alias
}

// Canonical is the alias operations act on.
func (m Machine) Canonical() Node { return m.Nodes[m.canonical] }

// Label is the canonical node's display label.
func (m Machine) Label() string { return m.Canonical().Label() }

// Aliases are the non-canonical nodes (may be empty).
func (m Machine) Aliases() []Node {
	var out []Node
	for i, n := range m.Nodes {
		if i != m.canonical {
			out = append(out, n)
		}
	}
	return out
}

// HasDup reports whether this machine has more than one tailnet registration.
func (m Machine) HasDup() bool { return m.Serial != "" && len(m.Nodes) > 1 }

// preferred reports whether a should beat b as the canonical alias:
// online beats offline; then a primary pin wins; then lexically-first name.
func preferred(a, b Node) bool {
	if a.Online != b.Online {
		return a.Online
	}
	if a.Primary != b.Primary {
		return a.Primary
	}
	return a.Name < b.Name
}

// pickCanonical returns the index of the operational alias among nodes.
func pickCanonical(nodes []Node) int {
	pick := 0
	for i := 1; i < len(nodes); i++ {
		if preferred(nodes[i], nodes[pick]) {
			pick = i
		}
	}
	return pick
}

// GroupByMachine collapses nodes sharing a non-empty serial into one Machine and
// leaves serial-less nodes as singletons. Machines are sorted by canonical label.
func GroupByMachine(nodes []Node) []Machine {
	bySerial := map[string][]Node{}
	var order []string
	var singles []Node
	for _, n := range nodes {
		if n.HW.Serial == "" {
			singles = append(singles, n)
			continue
		}
		if _, ok := bySerial[n.HW.Serial]; !ok {
			order = append(order, n.HW.Serial)
		}
		bySerial[n.HW.Serial] = append(bySerial[n.HW.Serial], n)
	}
	var machines []Machine
	for _, s := range order {
		g := bySerial[s]
		sort.Slice(g, func(i, j int) bool { return g[i].Name < g[j].Name })
		machines = append(machines, Machine{Serial: s, Nodes: g, canonical: pickCanonical(g)})
	}
	for _, n := range singles {
		machines = append(machines, Machine{Serial: "", Nodes: []Node{n}, canonical: 0})
	}
	sort.Slice(machines, func(i, j int) bool { return machines[i].Label() < machines[j].Label() })
	return machines
}

// StaleAlias is a non-canonical registration that tailscale reports offline —
// a likely ghost the user should delete in the tailnet admin (pldev can't).
type StaleAlias struct {
	Machine string
	Name    string
	IP      string
}

// staleAliases lists offline aliases of multi-registration machines.
func staleAliases(machines []Machine) []StaleAlias {
	var out []StaleAlias
	for _, m := range machines {
		for _, a := range m.Aliases() {
			if !a.Online {
				out = append(out, StaleAlias{Machine: m.Label(), Name: a.Name, IP: a.IP})
			}
		}
	}
	return out
}
