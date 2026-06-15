// Package fed is pldev's federation observability: a read-only view of the
// swarm via the NATS bus monitoring endpoint, mapped to fleet nicknames.
package fed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/fleet"
	"github.com/peterlodri-sec/pldev/internal/report"
)

// Varz is the subset of NATS /varz used.
type Varz struct {
	ServerName    string `json:"server_name"`
	Version       string `json:"version"`
	Connections   int    `json:"connections"`
	Subscriptions int    `json:"subscriptions"`
	Uptime        string `json:"uptime"`
}

// ConnInfo is one client from NATS /connz.
type ConnInfo struct {
	IP      string `json:"ip"`
	Name    string `json:"name"`
	Cid     int    `json:"cid"`
	NumSubs int    `json:"subscriptions"` // a count in /connz
	Uptime  string `json:"uptime"`
}

// Connz is the subset of NATS /connz used.
type Connz struct {
	NumConnections int        `json:"num_connections"`
	Connections    []ConnInfo `json:"connections"`
}

// httpGet is the indirection point for tests.
var httpGet = func(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func fetch[T any](ctx context.Context, url string) (T, error) {
	var v T
	b, err := httpGet(ctx, url)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func FetchVarz(ctx context.Context, base string) (Varz, error) {
	return fetch[Varz](ctx, base+"/varz")
}
func FetchConnz(ctx context.Context, base string) (Connz, error) {
	return fetch[Connz](ctx, base+"/connz")
}

// BusURL resolves the NATS monitoring base URL: the override if set, else the
// fleet node with role "bus" → http://<ip>:8222.
func BusURL(nodes []fleet.Node, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	for _, n := range nodes {
		if n.Role == "bus" {
			return "http://" + n.IP + ":8222", nil
		}
	}
	return "", fmt.Errorf("no bus node (role=\"bus\") in the fleet and no monitor_url configured")
}

// Status renders the bus health + connected clients (mapped to nicknames).
func Status(ctx context.Context, rep *report.Reporter, nodes []fleet.Node, base string) error {
	varz, err := FetchVarz(ctx, base)
	if err != nil {
		e := fmt.Errorf("bus monitoring unreachable at %s: %w", base, err)
		rep.Fail("fed-status", base, e)
		return e
	}
	connz, _ := FetchConnz(ctx, base) // best effort

	fmt.Print(Render(varz, connz, nodes)) // data → stdout
	rep.OK("fed-status", varz.ServerName, report.KV{K: "conns", V: strconv.Itoa(connz.NumConnections)})
	return nil
}

// Render builds the status panel; IPs are mapped to fleet nicknames.
func Render(v Varz, c Connz, nodes []fleet.Node) string {
	byIP := map[string]string{}
	for _, n := range nodes {
		byIP[n.IP] = n.Label()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "bus: %s  v%s  up %s  conns=%d  subs=%d\n\n",
		v.ServerName, v.Version, v.Uptime, v.Connections, v.Subscriptions)
	fmt.Fprintf(&b, "%-18s %-20s %-5s %s\n", "CLIENT", "NAME", "SUBS", "UPTIME")
	if len(c.Connections) == 0 {
		fmt.Fprintln(&b, "(no clients connected)")
		return b.String()
	}
	for _, cn := range c.Connections {
		client := byIP[cn.IP]
		if client == "" {
			client = cn.IP
		}
		fmt.Fprintf(&b, "%-18s %-20s %-5d %s\n", client, cn.Name, cn.NumSubs, cn.Uptime)
	}
	return b.String()
}
