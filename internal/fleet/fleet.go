package fleet

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Node is a discovered node enriched with config annotations + cached hardware.
type Node struct {
	Name     string
	Nickname string
	IP       string
	OS       string
	Online   bool
	Tags     []string
	Role     string
	Caps     []string
	SSHUser  string
	Excluded bool
	HW       HWInfo
	Primary  bool
	Dup      bool   // shares a serial with an earlier node
	DupOf    string // label of that earlier node
}

// Label is the display identity: the nickname when set, else the tailnet name.
func (n Node) Label() string {
	if n.Nickname != "" {
		return n.Nickname
	}
	return n.Name
}

// Merge annotates tailscale nodes with config + cached hardware, drops excluded
// nodes, fills ssh_user (per-node → default → $USER), then flags serial dups.
func Merge(ts []TSNode, cfg FleetConfig, cache FleetCache) []Node {
	def := cfg.DefaultSSHUser
	if def == "" {
		def = os.Getenv("USER")
	}
	var out []Node
	for _, t := range ts {
		nc := cfg.Node[t.Name]
		if nc.Exclude {
			continue
		}
		user := nc.SSHUser
		if user == "" {
			user = def
		}
		out = append(out, Node{
			Name: t.Name, Nickname: nc.Nickname, IP: t.IP, OS: t.OS, Online: t.Online, Tags: t.Tags,
			Role: nc.Role, Caps: nc.Caps, SSHUser: user, Primary: nc.Primary, HW: cache[t.Name].HW,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label() < out[j].Label() })
	detectDups(out)
	return out
}

// detectDups marks every node that shares a non-empty Serial with an earlier
// (sorted-order) node. The first keeps Dup=false; later ones get Dup=true and
// DupOf set to the first node's label. Empty serials never match.
func detectDups(nodes []Node) {
	firstBySerial := map[string]string{}
	for i := range nodes {
		s := nodes[i].HW.Serial
		if s == "" {
			continue
		}
		if first, ok := firstBySerial[s]; ok {
			nodes[i].Dup = true
			nodes[i].DupOf = first
		} else {
			firstBySerial[s] = nodes[i].Label()
		}
	}
}

// SelectNodes resolves a selector against the node fleet (kept for probe-user):
//   - "all"        → every node
//   - "@<role>"    → nodes whose role matches
//   - "<name>"     → the single node by name or nickname (case-insensitive)
func SelectNodes(nodes []Node, selector string) []Node {
	switch {
	case selector == "all" || selector == "@all":
		return nodes
	case strings.HasPrefix(selector, "@"):
		role := selector[1:]
		var out []Node
		for _, n := range nodes {
			if n.Role == role {
				out = append(out, n)
			}
		}
		return out
	default:
		// match the tailnet name or the nickname (case-insensitive)
		for _, n := range nodes {
			if n.Name == selector || strings.EqualFold(n.Nickname, selector) {
				return []Node{n}
			}
		}
		return nil
	}
}

// Select resolves a selector against machines:
//   - "all"/"@all" → every machine
//   - "@<role>"    → machines whose canonical role matches
//   - "<name>"     → the machine containing a node matching name or nickname
//     (case-insensitive), including a non-canonical alias name.
func Select(machines []Machine, selector string) []Machine {
	switch {
	case selector == "all" || selector == "@all":
		return machines
	case strings.HasPrefix(selector, "@"):
		role := selector[1:]
		var out []Machine
		for _, m := range machines {
			if m.Canonical().Role == role {
				out = append(out, m)
			}
		}
		return out
	default:
		for _, m := range machines {
			for _, n := range m.Nodes {
				if n.Name == selector || strings.EqualFold(n.Nickname, selector) {
					return []Machine{m}
				}
			}
		}
		return nil
	}
}

// trunc shortens s to n runes, ending with ".." when cut.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 2 {
		return s[:n]
	}
	return s[:n-2] + ".."
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Render returns the fleet as an aligned wide ASCII table (data → stdout), one
// row per physical machine (its canonical alias). A machine with extra tailnet
// registrations shows a trailing "+N alias"; --full adds CAPS and indented alias
// lines.
func Render(machines []Machine, full bool) string {
	const base = "%-12s %-14s %-8s %-3s %-14s %-3s %-8s %-6s"
	var b strings.Builder
	if full {
		fmt.Fprintf(&b, base+" %s\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH", "CAPS")
	} else {
		fmt.Fprintf(&b, base+"\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH")
	}
	for _, m := range machines {
		n := m.Canonical()
		st := "on"
		if !n.Online {
			st = "off"
		}
		cores := "-"
		if n.HW.Cores > 0 {
			cores = strconv.Itoa(n.HW.Cores)
		}
		mem := "-"
		if n.HW.MemGB > 0 {
			mem = fmt.Sprintf("%dG/%dG", n.HW.MemGB, n.HW.MemFreeGB)
		}
		row := fmt.Sprintf(base,
			trunc(n.Label(), 12), trunc(n.Name, 14), dash(n.Role), st,
			trunc(cpuCode(n.HW.CPU), 14), cores, mem, dash(n.HW.Arch))
		if full {
			row += " " + strings.Join(n.Caps, ",")
		}
		if al := m.Aliases(); len(al) > 0 {
			row += fmt.Sprintf(" +%d alias", len(al))
		}
		b.WriteString(strings.TrimRight(row, " ") + "\n")
		if full {
			for _, a := range m.Aliases() {
				ast := "on"
				if !a.Online {
					ast = "off"
				}
				fmt.Fprintf(&b, "    alias %-20s %-16s %s\n", a.Name, a.IP, ast)
			}
		}
	}
	return b.String()
}
