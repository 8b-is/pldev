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

// Select resolves a selector against the fleet:
//   - "all"        → every node
//   - "@<role>"    → nodes whose role matches
//   - "<name>"     → the single node by name
func Select(nodes []Node, selector string) []Node {
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

// Render returns the fleet as an aligned wide ASCII table (data → stdout):
// nickname, tailnet host, role, on/off, cpu, cores, mem(total/free), arch, then a
// trailing ⚠dup marker. CAPS is shown only when full is true.
func Render(nodes []Node, full bool) string {
	const base = "%-12s %-14s %-8s %-3s %-14s %-3s %-8s %-6s"
	var b strings.Builder
	if full {
		fmt.Fprintf(&b, base+" %s\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH", "CAPS")
	} else {
		fmt.Fprintf(&b, base+"\n", "NICK", "HOST", "ROLE", "ST", "CPU", "CR", "MEM", "ARCH")
	}
	for _, n := range nodes {
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
		if n.Dup {
			row += " ⚠dup"
		}
		b.WriteString(strings.TrimRight(row, " ") + "\n")
	}
	return b.String()
}
