package fleet

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Node is a discovered node enriched with config annotations.
type Node struct {
	Name     string
	IP       string
	OS       string
	Online   bool
	Tags     []string
	Role     string
	Caps     []string
	SSHUser  string
	Excluded bool
}

// Merge annotates tailscale nodes with config, drops excluded nodes, and fills
// ssh_user from the per-node value, then the default, then $USER.
func Merge(ts []TSNode, cfg FleetConfig) []Node {
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
			Name: t.Name, IP: t.IP, OS: t.OS, Online: t.Online, Tags: t.Tags,
			Role: nc.Role, Caps: nc.Caps, SSHUser: user,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
		for _, n := range nodes {
			if n.Name == selector {
				return []Node{n}
			}
		}
		return nil
	}
}

// Render returns the fleet as an aligned ASCII table (data → stdout).
func Render(nodes []Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-22s %-10s %-7s %-8s %s\n", "NODE", "ROLE", "OS", "STATUS", "CAPS")
	for _, n := range nodes {
		status := "online"
		if !n.Online {
			status = "offline"
		}
		role := n.Role
		if role == "" {
			role = "-"
		}
		fmt.Fprintf(&b, "%-22s %-10s %-7s %-8s %s\n",
			n.Name, role, n.OS, status, strings.Join(n.Caps, ","))
	}
	return b.String()
}
