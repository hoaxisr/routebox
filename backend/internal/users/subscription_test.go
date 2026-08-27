package users

import (
	"encoding/base64"
	"strings"
	"testing"

	"routebox/backend/internal/subscriptions"

	"routebox/backend/internal/serverlinks"
)

// activeMultiInbound builds an active config with vless, hysteria2 and naive
// inbounds, each carrying one user keyed by the credentials we bind below.
func activeMultiInbound() map[string]interface{} {
	return map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "vless", "tag": "vless-in", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"name": "alice", "uuid": "uuid-1",
						"flow": "xtls-rprx-vision"},
				},
			},
			map[string]interface{}{
				"type": "hysteria2", "tag": "hy2-in", "listen_port": float64(8443),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"name": "alice", "password": "pw-1"},
				},
			},
			map[string]interface{}{
				"type": "naive", "tag": "naive-in", "listen_port": float64(2096),
				"tls": map[string]interface{}{"enabled": true},
				"users": []interface{}{
					map[string]interface{}{"username": "alice-naive", "password": "np-1"},
				},
			},
		},
	}
}

func userBoundTo() *PanelUser {
	return &PanelUser{
		ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"},
			{InboundTag: "hy2-in", Credential: "pw-1", Protocol: "hysteria2"},
			{InboundTag: "naive-in", Credential: "alice-naive", Protocol: "naive"},
		},
	}
}

// nodesByPort indexes parsed nodes by their server_port (int) for per-binding
// assertions. The parsers store server_port as int.
func nodesByPort(nodes []subscriptions.ParsedNode) map[int]map[string]interface{} {
	out := map[int]map[string]interface{}{}
	for _, n := range nodes {
		if p, ok := n.Outbound["server_port"].(int); ok {
			out[p] = n.Outbound
		}
	}
	return out
}

func TestBuildSubscription_RoundTripMultiBinding(t *testing.T) {
	out, err := BuildSubscription(userBoundTo(), activeMultiInbound(), serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		t.Fatalf("output is not std base64: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 links, got %d: %q", len(lines), string(raw))
	}
	nodes, skipped := subscriptions.ParseLinks(lines)
	if skipped != 0 {
		t.Fatalf("client parser skipped %d of our own links", skipped)
	}
	if len(nodes) != 3 {
		t.Fatalf("parser produced %d nodes, want 3", len(nodes))
	}
	// Every node must point at the public host.
	for _, n := range nodes {
		if n.Outbound["server"] != "vpn.example.com" {
			t.Fatalf("node server = %v, want vpn.example.com", n.Outbound["server"])
		}
	}

	// Each node, keyed by its port, must carry the exact bound credential. This
	// catches a builder that emits the wrong credential or the wrong port.
	byPort := nodesByPort(nodes)

	vless := byPort[443]
	if vless == nil {
		t.Fatalf("no node on vless port 443; ports=%v", portsOf(nodes))
	}
	if vless["type"] != "vless" {
		t.Fatalf("port 443 node type = %v, want vless", vless["type"])
	}
	if vless["uuid"] != "uuid-1" {
		t.Fatalf("vless uuid = %v, want uuid-1", vless["uuid"])
	}

	hy2 := byPort[8443]
	if hy2 == nil {
		t.Fatalf("no node on hysteria2 port 8443; ports=%v", portsOf(nodes))
	}
	if hy2["type"] != "hysteria2" {
		t.Fatalf("port 8443 node type = %v, want hysteria2", hy2["type"])
	}
	if hy2["password"] != "pw-1" {
		t.Fatalf("hysteria2 password = %v, want pw-1", hy2["password"])
	}

	naive := byPort[2096]
	if naive == nil {
		t.Fatalf("no node on naive port 2096; ports=%v", portsOf(nodes))
	}
	if naive["type"] != "naive" {
		t.Fatalf("port 2096 node type = %v, want naive", naive["type"])
	}
	if naive["username"] != "alice-naive" {
		t.Fatalf("naive username = %v, want alice-naive", naive["username"])
	}
}

// portsOf is a small diagnostic helper for failure messages.
func portsOf(nodes []subscriptions.ParsedNode) []interface{} {
	var ps []interface{}
	for _, n := range nodes {
		ps = append(ps, n.Outbound["server_port"])
	}
	return ps
}

func TestBuildSubscription_SkipsBindingMissingFromActive(t *testing.T) {
	u := userBoundTo()
	// Add a binding whose inbound is NOT in active — must be skipped, not fatal.
	u.Bindings = append(u.Bindings, Binding{
		InboundTag: "ghost-in", Credential: "x", Protocol: "vless"})
	out, err := BuildSubscription(u, activeMultiInbound(), serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription must not fail on a missing binding: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("missing binding not skipped; got %d lines: %q", len(lines), string(raw))
	}
}

// A binding that references a REAL active inbound but whose Credential is NOT
// among that inbound's users must be skipped (exercises the credential-miss
// `continue` in BuildSubscription), while OTHER valid bindings still appear.
// Distinct from the inbound-missing test above.
func TestBuildSubscription_SkipsBindingCredentialNotInActive(t *testing.T) {
	u := &PanelUser{
		ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"},
			// Real inbound tag, credential absent from that inbound's users.
			{InboundTag: "hy2-in", Credential: "not-a-real-password", Protocol: "hysteria2"},
		},
	}
	out, err := BuildSubscription(u, activeMultiInbound(), serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("credential-miss binding not skipped; got %d lines: %q", len(lines), string(raw))
	}
	nodes, skipped := subscriptions.ParseLinks(lines)
	if skipped != 0 || len(nodes) != 1 {
		t.Fatalf("want 1 node, 0 skipped; got %d nodes, %d skipped", len(nodes), skipped)
	}
	if nodes[0].Outbound["uuid"] != "uuid-1" {
		t.Fatalf("surviving node uuid = %v, want uuid-1", nodes[0].Outbound["uuid"])
	}
}

// An empty binding credential must be skipped at the resolver (findUserByCredential)
// rather than false-matching a user that merely lacks the key field ("" == "").
func TestBuildSubscription_SkipsEmptyCredential(t *testing.T) {
	u := &PanelUser{
		ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"},
			// Empty credential against a real inbound: must NOT match.
			{InboundTag: "hy2-in", Credential: "", Protocol: "hysteria2"},
		},
	}
	out, err := BuildSubscription(u, activeMultiInbound(), serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("empty-credential binding not skipped; got %d lines: %q", len(lines), string(raw))
	}

	// Direct resolver-level guard check.
	inbound := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{"name": "noPassword"}, // no "password" key
		},
	}
	if _, found := findUserByCredential(inbound, "password", ""); found {
		t.Fatal("findUserByCredential matched an empty credential against a user missing the key")
	}
}

// A binding whose Protocol is unknown (CredentialKey returns "") is skipped
// without error, leaving other valid bindings intact.
func TestBuildSubscription_SkipsUnknownProtocol(t *testing.T) {
	u := &PanelUser{
		ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"},
			{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "bogus-proto"},
		},
	}
	out, err := BuildSubscription(u, activeMultiInbound(), serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("unknown-protocol binding not skipped; got %d lines: %q", len(lines), string(raw))
	}
}

func TestBuildSubscription_EmptyWhenNoActiveNodes(t *testing.T) {
	u := &PanelUser{ID: "u1", Name: "alice", Token: "tok",
		Bindings: []Binding{{InboundTag: "vless-in", Credential: "uuid-1", Protocol: "vless"}}}
	out, err := BuildSubscription(u, map[string]interface{}{"inbounds": []interface{}{}}, serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	// Empty list => base64.StdEncoding of "" => "".
	if out != "" {
		t.Fatalf("expected empty base64 for no resolvable nodes, got %q", out)
	}
}

// Host-agnostic: an empty host is NOT a builder error (the handler owns the 503
// policy). With no host, BuildShareLink errors per binding and every binding is
// skipped, yielding base64 of "" (i.e. ""). The builder must NOT hard-error.
func TestBuildSubscription_EmptyHostIsNotAnError(t *testing.T) {
	out, err := BuildSubscription(userBoundTo(), activeMultiInbound(), serverlinks.PublicAddr{Host: ""})
	if err != nil {
		t.Fatalf("empty host must not error the pure builder: %v", err)
	}
	if out != "" {
		t.Fatalf("empty host should skip all bindings -> empty base64, got %q", out)
	}
}
