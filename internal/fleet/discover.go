// Package fleet is pldev's federation substrate: a tailscale-discovered,
// config-annotated registry of the user's nodes with list/health/ssh/run.
package fleet

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// TSNode is the slice of tailscale's view that fleet uses.
type TSNode struct {
	Name   string // short name = first DNS label
	IP     string // first 100.x tailnet IP
	OS     string
	Online bool
	Tags   []string
}

type tsPeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	Online       bool     `json:"Online"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Tags         []string `json:"Tags"`
}

type tsStatus struct {
	Self *tsPeer            `json:"Self"`
	Peer map[string]*tsPeer `json:"Peer"`
}

func shortName(dns, host string) string {
	if dns != "" {
		return strings.SplitN(strings.TrimSuffix(dns, "."), ".", 2)[0]
	}
	return host
}

func firstV4(ips []string) string {
	for _, ip := range ips {
		if strings.Count(ip, ".") == 3 {
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

func (p *tsPeer) toNode() TSNode {
	return TSNode{
		Name:   shortName(p.DNSName, p.HostName),
		IP:     firstV4(p.TailscaleIPs),
		OS:     p.OS,
		Online: p.Online,
		Tags:   p.Tags,
	}
}

// ParseStatus turns `tailscale status --json` output into nodes (Self + Peers).
// Self is always treated as online (it's the local machine).
func ParseStatus(data []byte) ([]TSNode, error) {
	var s tsStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	var nodes []TSNode
	if s.Self != nil {
		self := s.Self.toNode()
		self.Online = true
		nodes = append(nodes, self)
	}
	for _, p := range s.Peer {
		nodes = append(nodes, p.toNode())
	}
	return nodes, nil
}

// Discover runs tailscale and parses the fleet.
func Discover(ctx context.Context) ([]TSNode, error) {
	out, err := sys.Run(ctx, "tailscale", "status", "--json")
	if err != nil {
		return nil, err
	}
	return ParseStatus([]byte(out))
}
