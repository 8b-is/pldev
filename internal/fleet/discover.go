// Package fleet is pldev's federation substrate: a tailscale-discovered,
// config-annotated registry of the user's nodes with list/health/ssh/run.
package fleet

import (
	"context"
	"net/netip"
	"strings"

	"tailscale.com/client/tailscale"
	"tailscale.com/ipn/ipnstate"
)

// TSNode is the slice of tailscale's view that fleet uses.
type TSNode struct {
	Name   string // short name = first DNS label
	IP     string // first 100.x tailnet IP
	OS     string
	Online bool
	Tags   []string
}

func shortName(dns, host string) string {
	if dns != "" {
		return strings.SplitN(strings.TrimSuffix(dns, "."), ".", 2)[0]
	}
	return host
}

func firstV4(ips []netip.Addr) string {
	for _, ip := range ips {
		if ip.Is4() {
			return ip.String()
		}
	}
	if len(ips) > 0 {
		return ips[0].String()
	}
	return ""
}

func tags(t *ipnstate.PeerStatus) []string {
	if t.Tags == nil {
		return nil
	}
	out := make([]string, 0, t.Tags.Len())
	for i := 0; i < t.Tags.Len(); i++ {
		out = append(out, t.Tags.At(i))
	}
	return out
}

func peerNode(p *ipnstate.PeerStatus) TSNode {
	return TSNode{
		Name:   shortName(p.DNSName, p.HostName),
		IP:     firstV4(p.TailscaleIPs),
		OS:     p.OS,
		Online: p.Online,
		Tags:   tags(p),
	}
}

// fromStatus maps a tailscale LocalAPI status into fleet nodes (Self + Peers).
// Self is always treated as online (it's the local machine).
func fromStatus(st *ipnstate.Status) []TSNode {
	var nodes []TSNode
	if st.Self != nil {
		self := peerNode(st.Self)
		self.Online = true
		nodes = append(nodes, self)
	}
	for _, p := range st.Peer {
		nodes = append(nodes, peerNode(p))
	}
	return nodes
}

// Discover queries tailscaled directly via its local API (no `tailscale` CLI)
// and maps the result into fleet nodes.
func Discover(ctx context.Context) ([]TSNode, error) {
	var lc tailscale.LocalClient
	st, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}
	return fromStatus(st), nil
}
