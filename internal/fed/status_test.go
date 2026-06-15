package fed

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/fleet"
	"github.com/peterlodri-sec/pldev/internal/report"
)

func nodes() []fleet.Node {
	return []fleet.Node{
		{Name: "crabcc-nats", Nickname: "Herald", IP: "100.73.72.35", Role: "bus"},
		{Name: "dev-cx53", Nickname: "Katedralis", IP: "100.105.72.88", Role: "hetzner"},
	}
}

func TestBusURL(t *testing.T) {
	u, err := BusURL(nodes(), "")
	if err != nil || u != "http://100.73.72.35:8222" {
		t.Errorf("derived bus url wrong: %q %v", u, err)
	}
	if u, _ := BusURL(nodes(), "http://x:9999"); u != "http://x:9999" {
		t.Errorf("override not honored: %q", u)
	}
	if _, err := BusURL([]fleet.Node{{Role: "agent"}}, ""); err == nil {
		t.Error("no bus node should error")
	}
}

func TestRenderMapsNicknames(t *testing.T) {
	v := Varz{ServerName: "crabcc-nats", Version: "2.14.2", Uptime: "9d", Connections: 1, Subscriptions: 83}
	c := Connz{NumConnections: 1, Connections: []ConnInfo{
		{IP: "100.105.72.88", Name: "ruflo-agent", NumSubs: 4, Uptime: "1h"},
		{IP: "10.0.0.9", Name: "unknown-client", NumSubs: 1, Uptime: "2m"},
	}}
	out := Render(v, c, nodes())
	if !strings.Contains(out, "crabcc-nats") || !strings.Contains(out, "9d") {
		t.Errorf("server summary missing: %q", out)
	}
	if !strings.Contains(out, "Katedralis") {
		t.Errorf("known IP should map to nickname: %q", out)
	}
	if !strings.Contains(out, "10.0.0.9") {
		t.Errorf("unknown IP should fall back to raw IP: %q", out)
	}
}

func TestRenderNoClients(t *testing.T) {
	out := Render(Varz{ServerName: "crabcc-nats"}, Connz{}, nodes())
	if !strings.Contains(out, "no clients connected") {
		t.Errorf("empty connz should say so: %q", out)
	}
}

func TestStatusUnreachable(t *testing.T) {
	old := httpGet
	defer func() { httpGet = old }()
	httpGet = func(context.Context, string) ([]byte, error) { return nil, errors.New("dial tcp: timeout") }
	var buf bytes.Buffer
	rep := report.New(&buf, "fed")
	err := Status(context.Background(), rep, nodes(), "http://100.73.72.35:8222")
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("unreachable bus should error clearly, got %v", err)
	}
	if !strings.Contains(buf.String(), "status=fail") {
		t.Errorf("expected fail log: %q", buf.String())
	}
}

func TestStatusOK(t *testing.T) {
	old := httpGet
	defer func() { httpGet = old }()
	httpGet = func(_ context.Context, url string) ([]byte, error) {
		if strings.HasSuffix(url, "/varz") {
			return []byte(`{"server_name":"crabcc-nats","version":"2.14.2","uptime":"9d","connections":0,"subscriptions":83}`), nil
		}
		return []byte(`{"num_connections":0,"connections":[]}`), nil
	}
	var buf bytes.Buffer
	rep := report.New(&buf, "fed")
	if err := Status(context.Background(), rep, nodes(), "http://x:8222"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "status=ok") || !strings.Contains(buf.String(), "conns=0") {
		t.Errorf("expected ok+conns: %q", buf.String())
	}
}
