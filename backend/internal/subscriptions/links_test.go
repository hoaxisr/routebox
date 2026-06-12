package subscriptions

import (
	"reflect"
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantHost string
		wantPort int
		wantOK   bool
	}{
		{"plain", "example.com:443", "example.com", 443, true},
		{"ipv6 bracket", "[::1]:443", "::1", 443, true},
		{"ipv6 bracket long", "[2001:db8::1]:8443", "2001:db8::1", 8443, true},
		{"missing port plain", "example.com", "", 0, false},
		{"missing port bracket", "[::1]", "", 0, false},
		{"port zero", "example.com:0", "", 0, false},
		{"port too big", "example.com:70000", "", 0, false},
		{"port non-numeric", "example.com:abc", "", 0, false},
		{"empty host", ":443", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, ok := splitHostPort(tt.in)
			if ok != tt.wantOK || host != tt.wantHost || port != tt.wantPort {
				t.Fatalf("splitHostPort(%q) = (%q,%d,%v), want (%q,%d,%v)",
					tt.in, host, port, ok, tt.wantHost, tt.wantPort, tt.wantOK)
			}
		})
	}
}

func TestDecodeSsUserinfo(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"std base64", "YWVzLTI1Ni1nY206dGVzdDEyMzQ=", "aes-256-gcm:test1234"},
		{"base64url unpadded", "YWVzLTI1Ni1nY206ays_Lz5-fg", "aes-256-gcm:k+?/>~~"},
		{"percent fallback", "aes-256-gcm%3Apassword", "aes-256-gcm:password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSsUserinfo(tt.in)
			if err != nil {
				t.Fatalf("decodeSsUserinfo(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("decodeSsUserinfo(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseVless(t *testing.T) {
	t.Run("basic tls", func(t *testing.T) {
		ob, name, err := parseVless("vless://b831381d-6324-4d53-ad4f-8cca48b30811@example.com:443?security=tls&sni=foo.com&fp=chrome#My%20Node")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if name != "My Node" {
			t.Fatalf("name = %q", name)
		}
		want := map[string]interface{}{
			"type": "vless", "server": "example.com", "server_port": 443,
			"uuid": "b831381d-6324-4d53-ad4f-8cca48b30811",
			"tls": map[string]interface{}{
				"enabled": true, "server_name": "foo.com",
				"utls": map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
			},
		}
		if !reflect.DeepEqual(ob, want) {
			t.Fatalf("got %#v\nwant %#v", ob, want)
		}
	})
	t.Run("reality", func(t *testing.T) {
		ob, _, err := parseVless("vless://uuid@srv:443?security=reality&pbk=PUBKEY&sid=ab12&fp=chrome&flow=xtls-rprx-vision#r")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["flow"] != "xtls-rprx-vision" {
			t.Fatalf("flow = %v", ob["flow"])
		}
		reality := ob["tls"].(map[string]interface{})["reality"].(map[string]interface{})
		if reality["enabled"] != true || reality["public_key"] != "PUBKEY" || reality["short_id"] != "ab12" {
			t.Fatalf("reality = %#v", reality)
		}
	})
	t.Run("ws transport", func(t *testing.T) {
		ob, _, err := parseVless("vless://uuid@srv:443?type=ws&path=%2Fws&host=cdn.example.com#w")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		tr := ob["transport"].(map[string]interface{})
		if tr["type"] != "ws" || tr["path"] != "/ws" {
			t.Fatalf("transport = %#v", tr)
		}
		if tr["headers"].(map[string]interface{})["Host"] != "cdn.example.com" {
			t.Fatalf("headers = %#v", tr["headers"])
		}
	})
	t.Run("ipv6 host", func(t *testing.T) {
		ob, _, err := parseVless("vless://uuid@[2001:db8::1]:443?security=tls#v6")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["server"] != "2001:db8::1" || ob["server_port"] != 443 {
			t.Fatalf("server/port = %v %v", ob["server"], ob["server_port"])
		}
	})
	t.Run("missing @", func(t *testing.T) {
		if _, _, err := parseVless("vless://noatsign:443"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestParseHysteria2(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		ob, name, err := parseHysteria2("hy2://pass@example.com:8443?sni=foo.com#H")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if name != "H" {
			t.Fatalf("name = %q", name)
		}
		want := map[string]interface{}{
			"type": "hysteria2", "server": "example.com", "server_port": 8443, "password": "pass",
			"tls": map[string]interface{}{"enabled": true, "server_name": "foo.com", "insecure": false},
		}
		if !reflect.DeepEqual(ob, want) {
			t.Fatalf("got %#v\nwant %#v", ob, want)
		}
	})
	t.Run("obfs and insecure", func(t *testing.T) {
		ob, _, err := parseHysteria2("hy2://pass@srv:8443?insecure=1&obfs=salamander&obfs-password=xyz#o")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["tls"].(map[string]interface{})["insecure"] != true {
			t.Fatalf("insecure = %v", ob["tls"])
		}
		obfs := ob["obfs"].(map[string]interface{})
		if obfs["type"] != "salamander" || obfs["password"] != "xyz" {
			t.Fatalf("obfs = %#v", obfs)
		}
	})
	t.Run("ipv6 host", func(t *testing.T) {
		ob, _, err := parseHysteria2("hy2://pass@[2001:db8::1]:8443#v6")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["server"] != "2001:db8::1" || ob["server_port"] != 8443 {
			t.Fatalf("server/port = %v %v", ob["server"], ob["server_port"])
		}
		if ob["tls"].(map[string]interface{})["server_name"] != "2001:db8::1" {
			t.Fatalf("server_name fallback = %v", ob["tls"])
		}
	})
	t.Run("hysteria2 scheme", func(t *testing.T) {
		if _, _, err := parseHysteria2("hysteria2://pass@srv:8443#x"); err != nil {
			t.Fatalf("err: %v", err)
		}
	})
}

func TestParseShadowsocks(t *testing.T) {
	t.Run("standard base64 userinfo", func(t *testing.T) {
		ob, name, err := parseShadowsocks("ss://YWVzLTI1Ni1nY206dGVzdDEyMzQ=@example.com:8388#MyServer")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if name != "MyServer" {
			t.Fatalf("name = %q", name)
		}
		want := map[string]interface{}{
			"type": "shadowsocks", "server": "example.com", "server_port": 8388,
			"method": "aes-256-gcm", "password": "test1234",
		}
		if !reflect.DeepEqual(ob, want) {
			t.Fatalf("got %#v\nwant %#v", ob, want)
		}
	})
	t.Run("base64url userinfo", func(t *testing.T) {
		ob, _, err := parseShadowsocks("ss://YWVzLTI1Ni1nY206ays_Lz5-fg@example.com:8388#URLSafe")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["method"] != "aes-256-gcm" || ob["password"] != "k+?/>~~" {
			t.Fatalf("method/pw = %v %v", ob["method"], ob["password"])
		}
	})
	t.Run("plugin", func(t *testing.T) {
		ob, _, err := parseShadowsocks("ss://YWVzLTI1Ni1nY206dGVzdDEyMzQ=@srv:8388/?plugin=obfs-local%3Bobfs%3Dhttp#p")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["plugin"] != "obfs-local" || ob["plugin_opts"] != "obfs=http" {
			t.Fatalf("plugin/opts = %v %v", ob["plugin"], ob["plugin_opts"])
		}
	})
	t.Run("ipv6 host", func(t *testing.T) {
		ob, _, err := parseShadowsocks("ss://YWVzLTI1Ni1nY206dGVzdDEyMzQ=@[2001:db8::1]:8388#v6")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["server"] != "2001:db8::1" || ob["server_port"] != 8388 {
			t.Fatalf("server/port = %v %v", ob["server"], ob["server_port"])
		}
	})
}

func TestParseNaive(t *testing.T) {
	t.Run("basic https with creds", func(t *testing.T) {
		ob, name, err := parseNaive("naive+https://user:pass@example.com:443#My%20Proxy")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if name != "My Proxy" {
			t.Fatalf("name = %q", name)
		}
		want := map[string]interface{}{
			"type": "naive", "server": "example.com", "server_port": 443,
			"tls":      map[string]interface{}{"enabled": true, "server_name": "example.com"},
			"username": "user", "password": "pass",
		}
		if !reflect.DeepEqual(ob, want) {
			t.Fatalf("got %#v\nwant %#v", ob, want)
		}
	})
	t.Run("quic flag", func(t *testing.T) {
		ob, _, err := parseNaive("naive+quic://user:pass@example.com:443#q")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["quic"] != true {
			t.Fatalf("quic = %v", ob["quic"])
		}
	})
	t.Run("no credentials", func(t *testing.T) {
		ob, name, err := parseNaive("naive+https://example.com:8443")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if name != "NaiveProxy" {
			t.Fatalf("name = %q", name)
		}
		if _, ok := ob["username"]; ok {
			t.Fatalf("username should be absent: %#v", ob)
		}
	})
	t.Run("username without password colon", func(t *testing.T) {
		ob, _, err := parseNaive("naive+https://onlyuser@example.com:443")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["username"] != "onlyuser" || ob["password"] != "" {
			t.Fatalf("username/password = %v %v", ob["username"], ob["password"])
		}
	})
	t.Run("ipv6 host", func(t *testing.T) {
		ob, _, err := parseNaive("naive+https://u:p@[2001:db8::1]:443#v6")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ob["server"] != "2001:db8::1" || ob["server_port"] != 443 {
			t.Fatalf("server/port = %v %v", ob["server"], ob["server_port"])
		}
	})
	t.Run("missing port fails", func(t *testing.T) {
		if _, _, err := parseNaive("naive+https://user:pass@example.com"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("wrong prefix fails", func(t *testing.T) {
		if _, _, err := parseNaive("https://example.com:443"); err == nil {
			t.Fatal("want error")
		}
	})
}
