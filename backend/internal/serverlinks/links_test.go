package serverlinks

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"routebox/backend/internal/subscriptions"
)

// parseBack runs a built link through the real subscription parser and returns
// the single resulting client outbound.
func parseBack(t *testing.T, link string) map[string]interface{} {
	t.Helper()
	nodes, skipped := subscriptions.ParseLinks([]string{link})
	if skipped != 0 || len(nodes) != 1 {
		t.Fatalf("parse round-trip failed: link=%q skipped=%d nodes=%d", link, skipped, len(nodes))
	}
	return nodes[0].Outbound
}

func TestBuildShareLinkVlessReality(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen": "::", "listen_port": float64(443),
		"tls": map[string]interface{}{
			"enabled": true, "server_name": "www.microsoft.com",
			// sing-box stores short_id as a JSON array — this is the real
			// on-disk shape (a bare string here previously hid a bug where the
			// builder dropped sid for array configs → Reality rejects clients).
			"reality": map[string]interface{}{
				"enabled": true, "private_key": fixturePriv, "short_id": []interface{}{"0123abcd"},
			},
		},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555", "flow": "xtls-rprx-vision"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.HasPrefix(link, "vless://11111111-2222-3333-4444-555555555555@vpn.example.com:443?") {
		t.Fatalf("unexpected prefix: %s", link)
	}

	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 443 {
		t.Fatalf("server/port mismatch: %v", ob)
	}
	if ob["uuid"] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid mismatch: %v", ob["uuid"])
	}
	if ob["flow"] != "xtls-rprx-vision" {
		t.Fatalf("flow mismatch: %v", ob["flow"])
	}
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "www.microsoft.com" {
		t.Fatalf("sni mismatch: %v", tls["server_name"])
	}
	reality := tls["reality"].(map[string]interface{})
	if reality["public_key"] != fixturePub {
		t.Fatalf("pbk mismatch: got %v want %v", reality["public_key"], fixturePub)
	}
	if reality["short_id"] != "0123abcd" {
		t.Fatalf("sid mismatch: %v", reality["short_id"])
	}
}

func TestBuildShareLinkVlessPlainTLS(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen_port": float64(8443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	user := map[string]interface{}{"name": "laptop", "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	ob := parseBack(t, link)
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "vpn.example.com" {
		t.Fatalf("sni mismatch: %v", tls["server_name"])
	}
	if _, hasReality := tls["reality"]; hasReality {
		t.Fatalf("plain TLS link should not carry reality params")
	}
}

func TestBuildShareLinkErrors(t *testing.T) {
	base := map[string]interface{}{"type": "vless", "listen_port": float64(443)}
	user := map[string]interface{}{"uuid": "u"}
	cases := []struct {
		name    string
		inbound map[string]interface{}
		user    map[string]interface{}
		host    string
	}{
		{"empty host", base, user, ""},
		{"no port", map[string]interface{}{"type": "vless"}, user, "h"},
		{"no uuid", base, map[string]interface{}{}, "h"},
		{"bad type", map[string]interface{}{"type": "tun", "listen_port": float64(1)}, user, "h"},
		{"naive no username", map[string]interface{}{"type": "naive", "listen_port": float64(443)}, map[string]interface{}{}, "h"},
		{"hy2 no password", map[string]interface{}{"type": "hysteria2", "listen_port": float64(443)}, map[string]interface{}{}, "h"},
	}
	for _, c := range cases {
		if _, err := BuildShareLink(c.inbound, c.user, PublicAddr{Host: c.host}); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestBuildShareLinkDisabledReality(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "listen_port": float64(443),
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "vpn.example.com",
			"reality": map[string]interface{}{
				"enabled":     false,
				"private_key": fixturePriv,
				"short_id":    "0123abcd",
			},
		},
	}
	user := map[string]interface{}{"name": "test", "uuid": "11111111-2222-3333-4444-555555555555"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if strings.Contains(link, "security=reality") {
		t.Fatalf("disabled reality must not emit security=reality: %s", link)
	}
	ob := parseBack(t, link)
	tls := ob["tls"].(map[string]interface{})
	if _, hasReality := tls["reality"]; hasReality {
		t.Fatalf("disabled reality link should not carry reality params after round-trip")
	}
}

func TestBuildShareLinkIPv6Host(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "listen_port": float64(443),
	}
	user := map[string]interface{}{"name": "ipv6user", "uuid": "11111111-2222-3333-4444-555555555555"}
	host := "2001:db8::1"

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: host})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.Contains(link, "[2001:db8::1]:443") {
		t.Fatalf("IPv6 host not bracketed in link: %s", link)
	}
	ob := parseBack(t, link)
	if ob["server"] != host {
		t.Fatalf("server mismatch after round-trip: got %v want %v", ob["server"], host)
	}
	if ob["server_port"] != 443 {
		t.Fatalf("server_port mismatch after round-trip: got %v", ob["server_port"])
	}
}

func TestBuildShareLinkNaive(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "naive", "tag": "naive-in", "listen_port": float64(443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	user := map[string]interface{}{"username": "alice", "password": "s3cr3t"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.HasPrefix(link, "naive+https://alice:s3cr3t@vpn.example.com:443") {
		t.Fatalf("unexpected naive link: %s", link)
	}
	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 443 {
		t.Fatalf("server/port mismatch: %v", ob)
	}
	if ob["username"] != "alice" || ob["password"] != "s3cr3t" {
		t.Fatalf("creds mismatch: %v", ob)
	}
}

func TestBuildShareLinkHysteria2(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "hysteria2", "tag": "hy2-in", "listen_port": float64(8443),
		"tls":  map[string]interface{}{"enabled": true, "server_name": "cdn.example.com"},
		"obfs": map[string]interface{}{"type": "salamander", "password": "obfspw"},
	}
	user := map[string]interface{}{"name": "phone", "password": "hunter2"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.HasPrefix(link, "hy2://hunter2@vpn.example.com:8443?") {
		t.Fatalf("unexpected hy2 link: %s", link)
	}
	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 8443 {
		t.Fatalf("server/port mismatch: %v", ob)
	}
	if ob["password"] != "hunter2" {
		t.Fatalf("password mismatch: %v", ob["password"])
	}
	obfs := ob["obfs"].(map[string]interface{})
	if obfs["type"] != "salamander" || obfs["password"] != "obfspw" {
		t.Fatalf("obfs mismatch: %v", obfs)
	}
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "cdn.example.com" {
		t.Fatalf("sni mismatch: got %v want cdn.example.com", tls["server_name"])
	}
}

func TestBuildShareLinkHysteria2Gecko(t *testing.T) {
	// gecko packet sizes only work when client and server agree, so the link has
	// to carry them through the round-trip (#48).
	inbound := map[string]interface{}{
		"type": "hysteria2", "tag": "hy2-in", "listen_port": float64(8443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
		"obfs": map[string]interface{}{
			"type": "gecko", "password": "obfspw",
			"min_packet_size": float64(700), "max_packet_size": float64(1300),
		},
	}
	user := map[string]interface{}{"name": "phone", "password": "hunter2"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	ob := parseBack(t, link)
	obfs := ob["obfs"].(map[string]interface{})
	if obfs["type"] != "gecko" || obfs["password"] != "obfspw" {
		t.Fatalf("obfs mismatch: %v", obfs)
	}
	if obfs["min_packet_size"] != 700 || obfs["max_packet_size"] != 1300 {
		t.Fatalf("gecko packet sizes lost: %v", obfs)
	}
}

func TestBuildShareLinkHysteria2GeckoDefaultSizes(t *testing.T) {
	// Unset sizes stay unset: hysteria's own defaults are the agreement.
	inbound := map[string]interface{}{
		"type": "hysteria2", "listen_port": float64(8443),
		"tls":  map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
		"obfs": map[string]interface{}{"type": "gecko", "password": "obfspw"},
	}
	user := map[string]interface{}{"name": "phone", "password": "hunter2"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if strings.Contains(link, "packet-size") {
		t.Fatalf("unset sizes should not appear in the link: %s", link)
	}
	obfs := parseBack(t, link)["obfs"].(map[string]interface{})
	if _, ok := obfs["min_packet_size"]; ok {
		t.Fatalf("min_packet_size invented: %v", obfs)
	}
	if _, ok := obfs["max_packet_size"]; ok {
		t.Fatalf("max_packet_size invented: %v", obfs)
	}
}

func TestBuildShareLinkHysteria2NoObfs(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "hysteria2", "listen_port": float64(8443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	user := map[string]interface{}{"name": "laptop", "password": "hunter2"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 8443 {
		t.Fatalf("server/port mismatch: %v", ob)
	}
	if _, hasObfs := ob["obfs"]; hasObfs {
		t.Fatalf("no-obfs link should not carry obfs key after round-trip: %v", ob["obfs"])
	}
}

func TestBuildShareLinkSpecialCharCredentials(t *testing.T) {
	// naive: username and password containing ':' and '@'
	naive := map[string]interface{}{
		"type": "naive", "listen_port": float64(443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	nUser := map[string]interface{}{"username": "us:er", "password": "p@ss:word"}
	link, err := BuildShareLink(naive, nUser, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("naive: %v", err)
	}
	ob := parseBack(t, link)
	if ob["server"] != "vpn.example.com" || ob["server_port"] != 443 {
		t.Fatalf("naive server/port corrupted: %v", ob)
	}
	if ob["username"] != "us:er" || ob["password"] != "p@ss:word" {
		t.Fatalf("naive creds corrupted: user=%v pass=%v", ob["username"], ob["password"])
	}

	// hysteria2: password containing '@' and ':'
	hy2 := map[string]interface{}{
		"type": "hysteria2", "listen_port": float64(8443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
	}
	hUser := map[string]interface{}{"name": "phone", "password": "p@ss:w0rd"}
	link2, err := BuildShareLink(hy2, hUser, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("hy2: %v", err)
	}
	ob2 := parseBack(t, link2)
	if ob2["server"] != "vpn.example.com" || ob2["server_port"] != 8443 {
		t.Fatalf("hy2 server/port corrupted: %v", ob2)
	}
	if ob2["password"] != "p@ss:w0rd" {
		t.Fatalf("hy2 password corrupted: %v", ob2["password"])
	}
}

func TestFirstShortID(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"array", []interface{}{"0123456789abcdef"}, "0123456789abcdef"},
		{"array-first-nonempty", []interface{}{"", "abcd"}, "abcd"},
		{"bare-string", "0123abcd", "0123abcd"},
		{"typed-string-slice", []string{"deadbeef"}, "deadbeef"},
		{"empty-array", []interface{}{}, ""},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		if got := firstShortID(c.in); got != c.want {
			t.Errorf("%s: firstShortID(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestBuildVlessTransportFlowOnlyWhenRaw(t *testing.T) {
	// ws transport: flow must be omitted even if present on the user.
	inbound := map[string]interface{}{
		"type": "vless", "tag": "v", "listen_port": float64(443),
		"tls":       map[string]interface{}{"enabled": true, "server_name": "ex.com"},
		"transport": map[string]interface{}{"type": "ws", "path": "/p", "headers": map[string]interface{}{"Host": "cdn.ex.com"}},
	}
	user := map[string]interface{}{"name": "p", "uuid": "11111111-2222-3333-4444-555555555555", "flow": "xtls-rprx-vision"}
	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.ex.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if strings.Contains(link, "flow=") {
		t.Fatalf("ws transport must omit flow: %s", link)
	}
	if !strings.Contains(link, "type=ws") || !strings.Contains(link, "host=cdn.ex.com") || !strings.Contains(link, "path=%2Fp") {
		t.Fatalf("ws transport params missing/wrong: %s", link)
	}
	// Round-trip: subscription parser maps URL host= → headers.Host for ws.
	ob := parseBack(t, link)
	tr := ob["transport"].(map[string]interface{})
	if tr["type"] != "ws" {
		t.Fatalf("transport type: %v", tr)
	}
	if tr["headers"].(map[string]interface{})["Host"] != "cdn.ex.com" {
		t.Fatalf("ws host should round-trip to headers.Host: %v", tr)
	}
}

func TestBuildVlessRawKeepsFlow(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "v", "listen_port": float64(443),
		"tls": map[string]interface{}{"enabled": true, "server_name": "ex.com"},
	}
	user := map[string]interface{}{"uuid": "11111111-2222-3333-4444-555555555555", "flow": "xtls-rprx-vision"}
	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.ex.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.Contains(link, "flow=xtls-rprx-vision") {
		t.Fatalf("raw transport must keep flow: %s", link)
	}
	if strings.Contains(link, "type=") {
		t.Fatalf("raw transport must not emit a transport type param: %s", link)
	}
}

func TestTlsParams(t *testing.T) {
	cases := []struct {
		name   string
		tls    map[string]interface{}
		host   string
		want   map[string]string
		absent []string
	}{
		{
			name: "reality emits pbk+sid+sni+fp",
			tls: map[string]interface{}{
				"enabled": true, "server_name": "www.microsoft.com",
				"reality": map[string]interface{}{"enabled": true, "private_key": fixturePriv, "short_id": []interface{}{"0123abcd"}},
			},
			host: "vpn.example.com",
			want: map[string]string{"security": "reality", "pbk": fixturePub, "sid": "0123abcd", "sni": "www.microsoft.com", "fp": "chrome"},
		},
		{
			name: "reality with bare-string short_id",
			tls:  map[string]interface{}{"enabled": true, "server_name": "a.com", "reality": map[string]interface{}{"enabled": true, "private_key": fixturePriv, "short_id": "beef"}},
			host: "h",
			want: map[string]string{"security": "reality", "sid": "beef", "sni": "a.com", "fp": "chrome"},
		},
		{
			name:   "plain tls emits security=tls+sni+fp, no pbk/sid",
			tls:    map[string]interface{}{"enabled": true, "server_name": "vpn.example.com"},
			host:   "vpn.example.com",
			want:   map[string]string{"security": "tls", "sni": "vpn.example.com", "fp": "chrome"},
			absent: []string{"pbk", "sid"},
		},
		{
			name: "acme domain used as sni when no server_name",
			tls:  map[string]interface{}{"enabled": true, "acme": map[string]interface{}{"domain": "le.example.com"}},
			host: "1.2.3.4",
			want: map[string]string{"security": "tls", "sni": "le.example.com", "fp": "chrome"},
		},
		{
			name:   "disabled tls => empty",
			tls:    map[string]interface{}{"enabled": false},
			host:   "h",
			want:   map[string]string{},
			absent: []string{"security", "sni", "fp", "pbk", "sid"},
		},
		{
			name:   "nil tls => empty",
			tls:    nil,
			host:   "h",
			want:   map[string]string{},
			absent: []string{"security", "sni"},
		},
		{
			name:   "reality block present but disabled => plain tls path",
			tls:    map[string]interface{}{"enabled": true, "server_name": "v.com", "reality": map[string]interface{}{"enabled": false, "private_key": fixturePriv, "short_id": "x"}},
			host:   "v.com",
			want:   map[string]string{"security": "tls", "sni": "v.com"},
			absent: []string{"pbk", "sid"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := tlsParams(c.tls, c.host)
			if err != nil {
				t.Fatalf("tlsParams: %v", err)
			}
			for k, v := range c.want {
				if q.Get(k) != v {
					t.Errorf("%s=%q, want %q (full=%v)", k, q.Get(k), v, q)
				}
			}
			for _, k := range c.absent {
				if q.Has(k) {
					t.Errorf("%s must be absent, got %q", k, q.Get(k))
				}
			}
		})
	}
}

func TestTransportParams(t *testing.T) {
	cases := []struct {
		name   string
		ib     map[string]interface{}
		want   map[string]string
		absent []string
	}{
		{"raw absent => empty", map[string]interface{}{}, map[string]string{}, []string{"type", "path", "host", "serviceName"}},
		{"type raw => empty", map[string]interface{}{"transport": map[string]interface{}{"type": "raw"}}, map[string]string{}, []string{"type"}},
		{
			"ws emits type+path+host (host UNIFORM on URL, from headers.Host)",
			map[string]interface{}{"transport": map[string]interface{}{"type": "ws", "path": "/ws", "headers": map[string]interface{}{"Host": "cdn.example.com"}}},
			map[string]string{"type": "ws", "path": "/ws", "host": "cdn.example.com"}, nil,
		},
		{
			"ws without host => no host param",
			map[string]interface{}{"transport": map[string]interface{}{"type": "ws", "path": "/ws"}},
			map[string]string{"type": "ws", "path": "/ws"}, []string{"host"},
		},
		{
			"httpupgrade emits type+path+host (top-level host string source)",
			map[string]interface{}{"transport": map[string]interface{}{"type": "httpupgrade", "path": "/up", "host": "h.example.com"}},
			map[string]string{"type": "httpupgrade", "path": "/up", "host": "h.example.com"}, nil,
		},
		{
			"xhttp emits type+path+host (top-level host string source)",
			map[string]interface{}{"transport": map[string]interface{}{"type": "xhttp", "path": "/dl", "host": "cdn.example.com"}},
			map[string]string{"type": "xhttp", "path": "/dl", "host": "cdn.example.com"}, nil,
		},
		{
			"grpc emits type+serviceName, no host/path",
			map[string]interface{}{"transport": map[string]interface{}{"type": "grpc", "service_name": "gsvc"}},
			map[string]string{"type": "grpc", "serviceName": "gsvc"}, []string{"host", "path"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := transportParams(c.ib)
			for k, v := range c.want {
				if q.Get(k) != v {
					t.Errorf("%s=%q want %q (%v)", k, q.Get(k), v, q)
				}
			}
			for _, k := range c.absent {
				if q.Has(k) {
					t.Errorf("%s must be absent (%v)", k, q)
				}
			}
		})
	}
}

func TestBuildTrojanReality(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "trojan", "tag": "tr", "listen_port": float64(443),
		"tls": map[string]interface{}{
			"enabled": true, "server_name": "www.microsoft.com",
			"reality": map[string]interface{}{"enabled": true, "private_key": fixturePriv, "short_id": []interface{}{"0123abcd"}},
		},
	}
	user := map[string]interface{}{"name": "phone", "password": "pw-secret"}
	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if !strings.HasPrefix(link, "trojan://pw-secret@vpn.example.com:443?") {
		t.Fatalf("unexpected prefix: %s", link)
	}
	if !strings.Contains(link, "security=reality") || !strings.Contains(link, "pbk="+fixturePub) || !strings.Contains(link, "sid=0123abcd") || !strings.Contains(link, "fp=chrome") {
		t.Fatalf("trojan reality params missing: %s", link)
	}
}

func TestBuildTrojanTlsWithTransport(t *testing.T) {
	transports := map[string]map[string]interface{}{
		"raw":         nil,
		"ws":          {"type": "ws", "path": "/wp", "headers": map[string]interface{}{"Host": "cdn.com"}},
		"grpc":        {"type": "grpc", "service_name": "gsvc"},
		"httpupgrade": {"type": "httpupgrade", "path": "/hp", "host": "hu.com"},
	}
	for trName, tr := range transports {
		t.Run(trName, func(t *testing.T) {
			ib := map[string]interface{}{
				"type": "trojan", "tag": "tr", "listen_port": float64(8443),
				"tls": map[string]interface{}{"enabled": true, "server_name": "ex.com"},
			}
			if tr != nil {
				ib["transport"] = tr
			}
			user := map[string]interface{}{"name": "p", "password": "p@ss:w0rd"}
			link, err := BuildShareLink(ib, user, PublicAddr{Host: "ex.com"})
			if err != nil {
				t.Fatalf("BuildShareLink: %v", err)
			}
			pu, err := url.Parse(link)
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}
			if pu.User.Username() != "p@ss:w0rd" {
				t.Errorf("password=%q (%s)", pu.User.Username(), link)
			}
			if pu.Host != "ex.com:8443" {
				t.Errorf("host=%q", pu.Host)
			}
			vals := pu.Query()
			if vals.Has("flow") {
				t.Errorf("trojan must never carry flow: %s", link)
			}
			if vals.Get("security") != "tls" {
				t.Errorf("security=%q want tls", vals.Get("security"))
			}
			switch trName {
			case "raw":
				if vals.Has("type") {
					t.Errorf("raw must omit transport type: %s", link)
				}
			case "ws":
				if vals.Get("type") != "ws" || vals.Get("path") != "/wp" || vals.Get("host") != "cdn.com" {
					t.Errorf("ws: %v", vals)
				}
			case "grpc":
				if vals.Get("type") != "grpc" || vals.Get("serviceName") != "gsvc" {
					t.Errorf("grpc: %v", vals)
				}
			case "httpupgrade":
				if vals.Get("type") != "httpupgrade" || vals.Get("path") != "/hp" || vals.Get("host") != "hu.com" {
					t.Errorf("httpupgrade: %v", vals)
				}
			}
		})
	}
}

// TestShareLinkRemarkIncludesInboundTag proves every builder labels its node with
// "<name> · <inbound-tag>" (3x-ui style) so a user's nodes across inbounds are
// distinctly named. When the tag is empty the remark falls back to name only.
func TestShareLinkRemarkIncludesInboundTag(t *testing.T) {
	cases := []struct {
		name    string
		inbound map[string]interface{}
		user    map[string]interface{}
		want    string // decoded remark expected after '#'
	}{
		{
			"vless",
			map[string]interface{}{"type": "vless", "tag": "vless-in", "listen_port": float64(443)},
			map[string]interface{}{"name": "alice", "uuid": "11111111-2222-3333-4444-555555555555"},
			"alice · vless-in",
		},
		{
			"trojan",
			map[string]interface{}{"type": "trojan", "tag": "trojan-in", "listen_port": float64(443)},
			map[string]interface{}{"name": "alice", "password": "pw"},
			"alice · trojan-in",
		},
		{
			"naive",
			map[string]interface{}{"type": "naive", "tag": "naive-in", "listen_port": float64(443)},
			map[string]interface{}{"username": "alice", "password": "pw"},
			"alice · naive-in",
		},
		{
			"hysteria2",
			map[string]interface{}{"type": "hysteria2", "tag": "hy2-in", "listen_port": float64(443),
				"tls": map[string]interface{}{"enabled": true, "server_name": "h.com"}},
			map[string]interface{}{"name": "alice", "password": "pw"},
			"alice · hy2-in",
		},
		{
			"empty-tag-falls-back-to-name",
			map[string]interface{}{"type": "vless", "listen_port": float64(443)},
			map[string]interface{}{"name": "alice", "uuid": "11111111-2222-3333-4444-555555555555"},
			"alice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			link, err := BuildShareLink(c.inbound, c.user, PublicAddr{Host: "vpn.example.com"})
			if err != nil {
				t.Fatalf("BuildShareLink: %v", err)
			}
			hash := strings.LastIndex(link, "#")
			if hash < 0 {
				t.Fatalf("link has no remark: %s", link)
			}
			frag := link[hash+1:]
			got, err := url.PathUnescape(frag)
			if err != nil {
				t.Fatalf("unescape remark %q: %v", frag, err)
			}
			if got != c.want {
				t.Fatalf("remark = %q, want %q (link=%s)", got, c.want, link)
			}
		})
	}
	// Spot-check the on-the-wire escaping for the middle dot (U+00B7 → %C2%B7).
	link, _ := BuildShareLink(
		map[string]interface{}{"type": "vless", "tag": "vless-in", "listen_port": float64(443)},
		map[string]interface{}{"name": "alice", "uuid": "11111111-2222-3333-4444-555555555555"},
		PublicAddr{Host: "vpn.example.com"})
	if !strings.Contains(link, "#alice%20%C2%B7%20vless-in") {
		t.Fatalf("expected escaped remark '#alice%%20%%C2%%B7%%20vless-in' in %s", link)
	}
}

func TestBuildTrojanNoPassword(t *testing.T) {
	ib := map[string]interface{}{"type": "trojan", "listen_port": float64(8443), "tls": map[string]interface{}{"enabled": true}}
	if _, err := BuildShareLink(ib, map[string]interface{}{"name": "x"}, PublicAddr{Host: "h"}); err == nil {
		t.Fatalf("trojan with no password must error")
	}
}

func TestBuildMieru(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "mieru", "tag": "m-in", "listen_port": float64(2020), "transport": "UDP",
	}
	user := map[string]interface{}{"name": "alice", "password": "p@ss w#rd"}

	link, err := buildMieru(inbound, user, "example.com", 2020)
	if err != nil {
		t.Fatalf("buildMieru error: %v", err)
	}
	// scheme + userinfo (percent-encoded), host in authority with NO port
	if !strings.HasPrefix(link, "mierus://") {
		t.Fatalf("bad scheme: %s", link)
	}
	if strings.Contains(link, "@example.com:2020") {
		t.Fatalf("port must be in the query, not the authority: %s", link)
	}
	// Anchor the query-value asserts on real param boundaries (a bare substring
	// like "port=2020" would also match inside another value): parse the link and
	// read the decoded query values back.
	pu, err := url.Parse(link)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", link, err)
	}
	q := pu.Query()
	if q.Get("port") != "2020" {
		t.Errorf("port=%q want 2020 (%s)", q.Get("port"), link)
	}
	if q.Get("protocol") != "UDP" {
		t.Errorf("protocol=%q want UDP (%s)", q.Get("protocol"), link)
	}
	if q.Get("profile") == "" {
		t.Errorf("profile must be present and non-empty: %s", link)
	}
	if strings.Contains(link, "multiplexing") {
		t.Errorf("inbound link must not carry multiplexing: %s", link)
	}
	// userinfo is percent-encoded (password has a space and '#').
	if !strings.Contains(link, "alice:p%40ss%20w%23rd@") {
		t.Errorf("userinfo not percent-encoded as expected: %s", link)
	}

	// IPv6 host is bracketed in the authority.
	link6, err := buildMieru(inbound, user, "2001:db8::1", 2020)
	if err != nil {
		t.Fatalf("buildMieru ipv6 error: %v", err)
	}
	if !strings.Contains(link6, "@[2001:db8::1]?") {
		t.Errorf("IPv6 host must be bracketed with no port: %s", link6)
	}

	// traffic_pattern only when the inbound has one
	inbound["traffic_pattern"] = "YWJj"
	linkTP, _ := buildMieru(inbound, user, "example.com", 2020)
	if !strings.Contains(linkTP, "traffic-pattern=YWJj") {
		t.Errorf("expected traffic-pattern in link: %s", linkTP)
	}
	delete(inbound, "traffic_pattern")
	linkNoTP, _ := buildMieru(inbound, user, "example.com", 2020)
	if strings.Contains(linkNoTP, "traffic-pattern") {
		t.Errorf("link should omit traffic-pattern when inbound has none: %s", linkNoTP)
	}
}

func TestBuildMieruNoPassword(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "mieru", "tag": "m-in", "listen_port": float64(2020),
	}
	user := map[string]interface{}{"name": "alice"} // no password
	link, err := buildMieru(inbound, user, "example.com", 2020)
	if err == nil {
		t.Fatalf("mieru with no password must error, got link=%q", link)
	}
	// The error must never echo the (empty) password value; only the name.
	if strings.Contains(err.Error(), ":@") {
		t.Fatalf("error must not leak the emitted userinfo: %v", err)
	}
	if !strings.Contains(err.Error(), "alice") {
		t.Fatalf("error should name the offending user: %v", err)
	}
}

// TestBuildMieruTransportDefault proves buildMieru defaults an absent transport
// to "TCP" so the emitted link always carries a valid protocol= (the mierus://
// parser requires it). Anchored on the decoded query value, not a substring.
func TestBuildMieruTransportDefault(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "mieru", "tag": "m-in", "listen_port": float64(2020), // NO transport field
	}
	user := map[string]interface{}{"name": "alice", "password": "pw"}

	link, err := buildMieru(inbound, user, "example.com", 2020)
	if err != nil {
		t.Fatalf("buildMieru error: %v", err)
	}
	pu, err := url.Parse(link)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", link, err)
	}
	if got := pu.Query().Get("protocol"); got != "TCP" {
		t.Fatalf("absent transport must default to protocol=TCP, got %q (%s)", got, link)
	}
}

// Issue #37: a mieru server may bind port ranges (listen_ports) as well as, or
// instead of, a single port. The share link has to carry them or the client
// never learns about the extra ports. mieru's parser pairs port[i]/protocol[i]
// and rejects a link whose counts differ (#46), so every port gets its own
// protocol=.
func TestBuildShareLinkMieruListenPorts(t *testing.T) {
	user := map[string]interface{}{"name": "alice", "password": "pw"}

	t.Run("single port plus ranges", func(t *testing.T) {
		ib := map[string]interface{}{
			"type": "mieru", "tag": "m-in", "listen_port": float64(2020), "transport": "UDP",
			"listen_ports": []interface{}{"25010-25012", "26000-26100"},
		}
		link, err := BuildShareLink(ib, user, PublicAddr{Host: "vpn.example.com"})
		if err != nil {
			t.Fatalf("BuildShareLink: %v", err)
		}
		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		ports := u.Query()["port"]
		want := []string{"2020", "25010-25012", "26000-26100"}
		if len(ports) != len(want) {
			t.Fatalf("port params = %v, want %v", ports, want)
		}
		for i := range want {
			if ports[i] != want[i] {
				t.Fatalf("port params = %v, want %v", ports, want)
			}
		}
		// One protocol PER port: mieru pairs them positionally and rejects the
		// link outright when the counts differ.
		if got := u.Query()["protocol"]; len(got) != len(want) {
			t.Fatalf("protocol params = %v, want one per port (%d)", got, len(want))
		} else {
			for _, p := range got {
				if p != "UDP" {
					t.Fatalf("protocol params = %v, want all UDP", got)
				}
			}
		}
	})

	t.Run("ranges only, no single port", func(t *testing.T) {
		ib := map[string]interface{}{
			"type": "mieru", "tag": "m-in", "transport": "TCP",
			"listen_ports": []interface{}{"25010-25012"},
		}
		link, err := BuildShareLink(ib, user, PublicAddr{Host: "vpn.example.com"})
		if err != nil {
			t.Fatalf("BuildShareLink with ranges only: %v", err)
		}
		u, _ := url.Parse(link)
		if got := u.Query()["port"]; len(got) != 1 || got[0] != "25010-25012" {
			t.Fatalf("port params = %v, want [25010-25012]", got)
		}
		if got := u.Query()["protocol"]; len(got) != 1 || got[0] != "TCP" {
			t.Fatalf("protocol params = %v, want [TCP]", got)
		}
	})

	t.Run("no ports at all is still an error", func(t *testing.T) {
		ib := map[string]interface{}{"type": "mieru", "tag": "m-in", "transport": "TCP"}
		if _, err := BuildShareLink(ib, user, PublicAddr{Host: "vpn.example.com"}); err == nil {
			t.Fatal("expected an error when the inbound exposes no port at all")
		}
	})
}

// --- public endpoint (inbounds behind a front) -------------------------------

// An inbound bound to loopback is unreachable at its own listen_port by
// definition: whatever fronts it on the public port is what a client must dial.
func TestBuildShareLinkLoopbackUsesFrontPort(t *testing.T) {
	cases := []struct {
		name    string
		inbound map[string]interface{}
		user    map[string]interface{}
		want    string // authority expected in the link
	}{
		{
			name: "vless grpc behind front",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "vless-grpc", "listen": "127.0.0.1", "listen_port": float64(10001),
				"tls":       map[string]interface{}{"enabled": true, "server_name": "example.com"},
				"transport": map[string]interface{}{"type": "grpc", "service_name": "Zt7qP1"},
			},
			user: map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"},
			want: "example.com:443",
		},
		{
			name: "trojan ws behind front",
			inbound: map[string]interface{}{
				"type": "trojan", "tag": "trojan-ws", "listen": "127.0.0.1", "listen_port": float64(10002),
				"tls":       map[string]interface{}{"enabled": true, "server_name": "example.com"},
				"transport": map[string]interface{}{"type": "ws", "path": "/Kx9mB2"},
			},
			user: map[string]interface{}{"name": "phone", "password": "s3cret"},
			want: "example.com:443",
		},
		{
			name: "ipv6 loopback counts as loopback",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "vless-grpc", "listen": "::1", "listen_port": float64(10001),
				"tls":       map[string]interface{}{"enabled": true, "server_name": "example.com"},
				"transport": map[string]interface{}{"type": "grpc", "service_name": "Zt7qP1"},
			},
			user: map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"},
			want: "example.com:443",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link, err := BuildShareLink(tc.inbound, tc.user, PublicAddr{Host: "example.com", Port: 443})
			if err != nil {
				t.Fatalf("BuildShareLink: %v", err)
			}
			u, err := url.Parse(link)
			if err != nil {
				t.Fatalf("parse link %q: %v", link, err)
			}
			if u.Host != tc.want {
				t.Errorf("authority = %q, want %q (link=%q)", u.Host, tc.want, link)
			}
			if strings.Contains(link, "10001") || strings.Contains(link, "10002") {
				t.Errorf("internal listen_port leaked into link: %q", link)
			}
			// The port rewrite must not disturb the transport coordinates: without
			// them the client reaches the front but never the inbound behind it.
			if tr := mapOf(tc.inbound["transport"]); tr != nil {
				q := u.Query()
				if p, _ := tr["path"].(string); p != "" && q.Get("path") != p {
					t.Errorf("path = %q, want %q", q.Get("path"), p)
				}
				if sn, _ := tr["service_name"].(string); sn != "" && q.Get("serviceName") != sn {
					t.Errorf("serviceName = %q, want %q", q.Get("serviceName"), sn)
				}
			}
		})
	}
}

// The front port must not disturb an inbound that is already reachable from
// outside — that is every inbound of every existing installation.
func TestBuildShareLinkExternalListenKeepsItsPort(t *testing.T) {
	cases := []struct{ name, listen string }{
		{"wildcard v6", "::"},
		{"wildcard v4", "0.0.0.0"},
		{"absent listen", ""},
		{"specific public address", "203.0.113.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inbound := map[string]interface{}{
				"type": "vless", "tag": "vless-in", "listen_port": float64(8443),
				"tls": map[string]interface{}{"enabled": true, "server_name": "example.com"},
			}
			if tc.listen != "" {
				inbound["listen"] = tc.listen
			}
			user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}

			link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
			if err != nil {
				t.Fatalf("BuildShareLink: %v", err)
			}
			u, _ := url.Parse(link)
			if u.Host != "example.com:8443" {
				t.Errorf("authority = %q, want example.com:8443 — front port must not override a reachable inbound", u.Host)
			}
		})
	}
}

// Without a configured front port a loopback inbound has no dialable port at
// all. Emitting the loopback one would hand the client a link that cannot work;
// failing lets the caller skip the binding and log it.
func TestBuildShareLinkLoopbackWithoutFrontPortFails(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-grpc", "listen": "127.0.0.1", "listen_port": float64(10001),
		"tls": map[string]interface{}{"enabled": true, "server_name": "example.com"},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}

	if _, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com"}); err == nil {
		t.Fatal("expected an error for a loopback inbound with no front port")
	}
}

// The SNI a client presents comes from the inbound's configured name, never
// from the address it happens to be bound to.
func TestBuildShareLinkSNIComesFromConfigNotListenAddress(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-grpc", "listen": "127.0.0.1", "listen_port": float64(10001),
		"tls":       map[string]interface{}{"enabled": true, "server_name": "cdn.example.com"},
		"transport": map[string]interface{}{"type": "grpc", "service_name": "Zt7qP1"},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	u, _ := url.Parse(link)
	if got := u.Query().Get("sni"); got != "cdn.example.com" {
		t.Errorf("sni = %q, want cdn.example.com", got)
	}
	if got := u.Query().Get("serviceName"); got != "Zt7qP1" {
		t.Errorf("serviceName = %q, want Zt7qP1 — transport path must survive the port rewrite", got)
	}
}

// mieru binds port RANGES, and a front maps single ports. There is no image of a
// range through a front, so a loopback-bound mieru is unbuildable rather than
// silently mapped: emitting it leaked the internal range into the client link
// AND paired it with a front port the inbound never listened on.
func TestBuildShareLinkMieruLoopbackRangesFail(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "mieru", "tag": "mieru-in", "listen": "127.0.0.1",
		"listen_ports": []interface{}{"25010-25012"}, "transport": "UDP",
	}
	user := map[string]interface{}{"name": "alice", "password": "pw"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
	if err == nil {
		t.Fatalf("expected an error, got link %q", link)
	}
	if strings.Contains(link, "25010") {
		t.Errorf("internal range leaked into link: %q", link)
	}
}

// mieru reachable from outside keeps every port it actually binds, front or no
// front — this is the shape every existing installation has.
func TestBuildShareLinkMieruExternalKeepsRanges(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "mieru", "tag": "mieru-in", "listen": "::",
		"listen_ports": []interface{}{"25010-25012"}, "transport": "UDP",
	}
	user := map[string]interface{}{"name": "alice", "password": "pw"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	u, _ := url.Parse(link)
	if ports := u.Query()["port"]; len(ports) != 1 || ports[0] != "25010-25012" {
		t.Errorf("port = %v, want exactly [25010-25012] — the front port must not be added", ports)
	}
}

// "localhost" is loopback spelled as a name; a link built from it would be dead.
func TestBuildShareLinkLocalhostCountsAsLoopback(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen": "localhost", "listen_port": float64(10001),
		"tls": map[string]interface{}{"enabled": true, "server_name": "example.com"},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}

	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if u, _ := url.Parse(link); u.Host != "example.com:443" {
		t.Errorf("authority = %q, want example.com:443", u.Host)
	}
}

// The front-less skip is configuration policy, not a malformed inbound: /sub is
// public and unauthenticated, so it must not log once per request.
func TestErrNoFrontIsIdentifiable(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-in", "listen": "127.0.0.1", "listen_port": float64(10001),
		"tls": map[string]interface{}{"enabled": true, "server_name": "example.com"},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}

	_, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com"})
	if !errors.Is(err, ErrNoFront) {
		t.Fatalf("err = %v, want it to wrap ErrNoFront", err)
	}
}

// An inbound behind the 443 front carries no tls block of its own: dest
// terminates TLS with the real certificate and forwards plaintext on loopback.
// The client still speaks TLS — to the front — so the link must say so. Without
// this the link looks right, resolves, and dies in the handshake.
func TestBuildShareLinkBehindFrontEmitsFrontTLS(t *testing.T) {
	cases := []struct {
		name    string
		inbound map[string]interface{}
		user    map[string]interface{}
	}{
		{
			name: "plaintext vless grpc behind front",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "vless-grpc-in", "listen": "127.0.0.1", "listen_port": float64(8444),
				"transport": map[string]interface{}{"type": "grpc", "service_name": "grpc-7a1c9e"},
			},
			user: map[string]interface{}{"name": "owner", "uuid": "11111111-2222-3333-4444-555555555555"},
		},
		{
			name: "plaintext trojan ws behind front",
			inbound: map[string]interface{}{
				"type": "trojan", "tag": "trojan-ws-in", "listen": "127.0.0.1", "listen_port": float64(8445),
				"transport": map[string]interface{}{"type": "ws", "path": "/ws-4f2b8d"},
			},
			user: map[string]interface{}{"name": "owner", "password": "s3cret-pw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link, err := BuildShareLink(tc.inbound, tc.user, PublicAddr{Host: "media.example.com", Port: 443})
			if err != nil {
				t.Fatalf("BuildShareLink: %v", err)
			}
			u, err := url.Parse(link)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			q := u.Query()
			if q.Get("security") != "tls" {
				t.Errorf("security = %q, want tls: %s", q.Get("security"), link)
			}
			// The front answers for the public host; that is the name the client
			// must ask for, and the only one dest holds a certificate for.
			if q.Get("sni") != "media.example.com" {
				t.Errorf("sni = %q, want media.example.com: %s", q.Get("sni"), link)
			}
			if q.Get("fp") == "" {
				t.Errorf("no fingerprint: %s", link)
			}

			// The round-trip is the real check: a client parsing this link must
			// end up with TLS enabled.
			ob := parseBack(t, link)
			tls, ok := ob["tls"].(map[string]interface{})
			if !ok {
				t.Fatalf("client outbound has no tls block: %v", ob)
			}
			if enabled, _ := tls["enabled"].(bool); !enabled {
				t.Errorf("client outbound TLS not enabled: %v", tls)
			}
			if tls["server_name"] != "media.example.com" {
				t.Errorf("client SNI = %v, want media.example.com", tls["server_name"])
			}
		})
	}
}

// A plaintext inbound reachable from outside is NOT behind a front — inventing
// TLS for it would produce a link that cannot connect, the same failure in the
// other direction.
func TestBuildShareLinkPublicPlaintextStaysPlaintext(t *testing.T) {
	inbound := map[string]interface{}{
		"type": "vless", "tag": "vless-plain", "listen": "::", "listen_port": float64(8080),
		"transport": map[string]interface{}{"type": "ws", "path": "/p"},
	}
	user := map[string]interface{}{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}
	link, err := BuildShareLink(inbound, user, PublicAddr{Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildShareLink: %v", err)
	}
	if strings.Contains(link, "security=") {
		t.Fatalf("plaintext public inbound must not claim TLS: %s", link)
	}
}

// In an out-of-the-box install naive is served by dest, not sing-box: there is
// no inbound to hand BuildShareLink, and the client still needs a link. Same
// scheme the rest of the panel emits and its own parser reads.
func TestBuildNaiveLinkForDest(t *testing.T) {
	link, err := BuildNaiveLink("alice", "s3cr3t", PublicAddr{Host: "vpn.example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildNaiveLink: %v", err)
	}
	if want := "naive+https://alice:s3cr3t@vpn.example.com:443#alice%20%C2%B7%20naive"; link != want {
		t.Fatalf("link = %q, want %q", link, want)
	}
}

// Credentials are operator text and land in a URL's userinfo: a ':' or '@' left
// unescaped moves the boundary between user, password and host.
func TestBuildNaiveLinkEscapesCredentials(t *testing.T) {
	link, err := BuildNaiveLink("a:b", "p@ss word", PublicAddr{Host: "vpn.example.com", Port: 443})
	if err != nil {
		t.Fatalf("BuildNaiveLink: %v", err)
	}
	if strings.Contains(link, "a:b:") || strings.Contains(link, "p@ss") {
		t.Fatalf("credentials went in raw: %s", link)
	}
	if !strings.Contains(link, "@vpn.example.com:443#") {
		t.Fatalf("host boundary moved: %s", link)
	}
}

// Nothing usable comes out of a missing name, a missing host or a port that no
// front ever published — and a link that looks right and cannot work is worse
// than none.
func TestBuildNaiveLinkRefusesTheUnusable(t *testing.T) {
	cases := []struct {
		name, user, pass string
		ep               PublicAddr
	}{
		{"no user", "", "pw", PublicAddr{Host: "h", Port: 443}},
		{"no host", "alice", "pw", PublicAddr{Port: 443}},
		{"no port", "alice", "pw", PublicAddr{Host: "h"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := BuildNaiveLink(c.user, c.pass, c.ep); err == nil {
				t.Fatal("want an error, got a link")
			}
		})
	}
}
