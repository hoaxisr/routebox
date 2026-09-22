package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func parseJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func ruleSets(cfg map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	for _, v := range cfg["route"].(map[string]interface{})["rule_set"].([]interface{}) {
		out = append(out, v.(map[string]interface{}))
	}
	return out
}

func httpClients(cfg map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	arr, _ := cfg["http_clients"].([]interface{})
	for _, v := range arr {
		out = append(out, v.(map[string]interface{}))
	}
	return out
}

const routerConfig = `{"outbounds":[{"tag":"direct","type":"direct"},{"tag":"proxy","type":"socks","server":"1.2.3.4","server_port":1}],
	"route":{"final":"proxy","rule_set":[
		{"tag":"ads","type":"remote","url":"u","download_detour":"direct"},
		{"tag":"geo","type":"remote","url":"u","download_detour":"proxy"},
		{"tag":"plain","type":"remote","url":"u"},
		{"tag":"loc","type":"local","path":"p"}]}}`

// The old implicit client dialed through the DEFAULT outbound (route.final).
// The shared replacement must keep that, and an explicit direct download must
// stay direct — without `detour: direct`, which sing-box rejects.
func TestMigrate115RuleSetsRouterFinalIsProxy(t *testing.T) {
	cfg := parseJSON(t, routerConfig)
	migrateSingbox115(cfg)

	rs := ruleSets(cfg)
	for _, r := range rs {
		if _, has := r["download_detour"]; has {
			t.Errorf("rule_set %v kept download_detour", r["tag"])
		}
	}
	if rs[0]["http_client"] != ruleSetDirectHTTPClientTag {
		t.Errorf("download_detour → empty direct must reference the direct client, got %v", rs[0]["http_client"])
	}
	if !reflect.DeepEqual(rs[1]["http_client"], map[string]interface{}{"detour": "proxy"}) {
		t.Errorf("download_detour → proxy must become http_client{detour}: %v", rs[1]["http_client"])
	}
	if rs[2]["http_client"] != nil {
		t.Errorf("rule_set without download_detour uses the shared default: %v", rs[2]["http_client"])
	}

	clients := httpClients(cfg)
	if len(clients) != 2 || clients[0]["tag"] != ruleSetHTTPClientTag || clients[1]["tag"] != ruleSetDirectHTTPClientTag {
		t.Fatalf("http_clients = %v (shared default must come FIRST — sing-box's fallback is the first entry)", clients)
	}
	if clients[0]["detour"] != "proxy" {
		t.Errorf("shared client must dial through route.final like the old implicit client: %v", clients[0])
	}
	if _, has := clients[1]["detour"]; has {
		t.Errorf("direct client must have no detour: %v", clients[1])
	}
	if cfg["route"].(map[string]interface{})["default_http_client"] != ruleSetHTTPClientTag {
		t.Error("route.default_http_client not set")
	}
	if again := migrateSingbox115(cfg); len(again) != 0 {
		t.Errorf("second run must be a no-op, got %v", again)
	}
}

func TestMigrate115RuleSetsVPSFinalIsDirect(t *testing.T) {
	cfg := parseJSON(t, `{"outbounds":[{"tag":"direct","type":"direct"}],"route":{"final":"direct","rule_set":[
		{"tag":"ads","type":"remote","url":"u","download_detour":"direct"},{"tag":"plain","type":"remote","url":"u"}]}}`)
	migrateSingbox115(cfg)
	clients := httpClients(cfg)
	if len(clients) != 1 || clients[0]["tag"] != ruleSetHTTPClientTag {
		t.Fatalf("one shared client expected: %v", clients)
	}
	if _, has := clients[0]["detour"]; has {
		t.Errorf("final is an empty direct outbound → shared client dials direct, no detour: %v", clients[0])
	}
	if ruleSets(cfg)[0]["http_client"] != nil {
		t.Errorf("explicit direct download is the shared default here, no per-rule-set client: %v", ruleSets(cfg)[0]["http_client"])
	}
}

// "direct" is a TYPE (plus no dial options), not the tag "direct".
func TestMigrate115EmptyDirectIsByTypeNotTag(t *testing.T) {
	cfg := parseJSON(t, `{"outbounds":[{"tag":"wan","type":"direct"},{"tag":"direct","type":"direct","bind_interface":"eth1"}],
		"route":{"final":"wan","rule_set":[
			{"tag":"a","type":"remote","url":"u","download_detour":"wan"},
			{"tag":"b","type":"remote","url":"u","download_detour":"direct"}]}}`)
	migrateSingbox115(cfg)
	rs := ruleSets(cfg)
	if rs[0]["http_client"] != nil {
		t.Errorf("detour to empty direct outbound 'wan' must be dropped: %v", rs[0]["http_client"])
	}
	if !reflect.DeepEqual(rs[1]["http_client"], map[string]interface{}{"detour": "direct"}) {
		t.Errorf("a direct outbound WITH dial options is a valid detour: %v", rs[1]["http_client"])
	}
	if _, has := httpClients(cfg)[0]["detour"]; has {
		t.Errorf("final 'wan' is an empty direct → shared client has no detour: %v", httpClients(cfg)[0])
	}
}

func TestMigrate115KeepsExistingHTTPClients(t *testing.T) {
	cfg := parseJSON(t, `{"http_clients":[{"tag":"mine","detour":"proxy"}],"route":{"rule_set":[{"tag":"ads","type":"remote","url":"u"}]}}`)
	if notes := migrateSingbox115(cfg); len(notes) != 0 {
		t.Errorf("must not touch a config that already declares http_clients: %v", notes)
	}
	cfg = parseJSON(t, `{"route":{"default_http_client":"x","rule_set":[{"tag":"ads","type":"remote","url":"u"}]}}`)
	if notes := migrateSingbox115(cfg); len(notes) != 0 {
		t.Errorf("must not override route.default_http_client: %v", notes)
	}
	// Existing per-rule-set http_client wins over download_detour.
	cfg = parseJSON(t, `{"outbounds":[{"tag":"proxy","type":"socks"}],"route":{"rule_set":[{"tag":"a","type":"remote","url":"u","download_detour":"proxy","http_client":"mine"}]},"http_clients":[{"tag":"mine"}]}`)
	migrateSingbox115(cfg)
	if ruleSets(cfg)[0]["http_client"] != "mine" {
		t.Errorf("existing http_client overwritten: %v", ruleSets(cfg)[0]["http_client"])
	}
}

func TestMigrate115NoRemoteRuleSetsNoClient(t *testing.T) {
	cfg := parseJSON(t, `{"route":{"rule_set":[{"tag":"loc","type":"local","path":"p"}]}}`)
	migrateSingbox115(cfg)
	if cfg["http_clients"] != nil {
		t.Error("no remote rule-sets → no http_clients")
	}
	cfg = parseJSON(t, `{"http_clients":[],"route":{"rule_set":[{"tag":"r","type":"remote","url":"u"}]}}`)
	migrateSingbox115(cfg)
	if len(httpClients(cfg)) != 1 {
		t.Error("an empty http_clients list counts as none")
	}
}

func TestMigrate115StoreRDRC(t *testing.T) {
	cases := []struct {
		in       string
		wantDNS  bool
		wantNote bool
	}{
		{`{"enabled":true,"store_rdrc":true,"rdrc_timeout":"7d"}`, true, true},
		{`{"enabled":true,"store_rdrc":false}`, false, true},
		{`{"enabled":true,"rdrc_timeout":"7d"}`, false, true},
		{`{"enabled":true,"store_dns":true}`, true, false},
	}
	for _, tc := range cases {
		cfg := parseJSON(t, `{"experimental":{"cache_file":`+tc.in+`}}`)
		notes := migrateSingbox115(cfg)
		cf := cfg["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
		if _, has := cf["store_rdrc"]; has {
			t.Errorf("%s: store_rdrc must be removed", tc.in)
		}
		if _, has := cf["rdrc_timeout"]; has {
			t.Errorf("%s: rdrc_timeout must be removed", tc.in)
		}
		if got, _ := cf["store_dns"].(bool); got != tc.wantDNS {
			t.Errorf("%s: store_dns = %v, want %v", tc.in, got, tc.wantDNS)
		}
		if (len(notes) > 0) != tc.wantNote {
			t.Errorf("%s: notes = %v", tc.in, notes)
		}
		if again := migrateSingbox115(cfg); len(again) != 0 {
			t.Errorf("%s: not idempotent: %v", tc.in, again)
		}
	}
}

func TestMigrate115InlineACME(t *testing.T) {
	cfg := parseJSON(t, `{"inbounds":[{"tag":"in","type":"vless","tls":{"enabled":true,"acme":{"domain":"vpn.example.com","email":"a@b.c","data_directory":"/x"}}}]}`)
	if notes := migrateSingbox115(cfg); len(notes) != 1 {
		t.Fatalf("notes = %v", notes)
	}
	tls := cfg["inbounds"].([]interface{})[0].(map[string]interface{})["tls"].(map[string]interface{})
	if _, has := tls["acme"]; has {
		t.Error("tls.acme must be removed")
	}
	want := map[string]interface{}{"type": "acme", "domain": "vpn.example.com", "email": "a@b.c", "data_directory": "/x"}
	if !reflect.DeepEqual(tls["certificate_provider"], want) {
		t.Errorf("certificate_provider = %v, want %v", tls["certificate_provider"], want)
	}
	if again := migrateSingbox115(cfg); len(again) != 0 {
		t.Errorf("not idempotent: %v", again)
	}
	// Both present: acme goes, the provider is kept as is.
	cfg = parseJSON(t, `{"inbounds":[{"tag":"in","type":"vless","tls":{"acme":{"domain":"a"},"certificate_provider":"shared"}}]}`)
	migrateSingbox115(cfg)
	tls = cfg["inbounds"].([]interface{})[0].(map[string]interface{})["tls"].(map[string]interface{})
	if tls["certificate_provider"] != "shared" {
		t.Errorf("existing certificate_provider overwritten: %v", tls["certificate_provider"])
	}
}

func TestMigrate115DropsUnknownDomainStrategyFields(t *testing.T) {
	cfg := parseJSON(t, `{"route":{"default_domain_strategy":"prefer_ipv4","final":"x"},"dns":{"servers":[{"tag":"d","type":"tls","server":"1.1.1.1","domain_strategy":"ipv4_only"}]}}`)
	if notes := migrateSingbox115(cfg); len(notes) != 2 {
		t.Fatalf("notes = %v", notes)
	}
	if _, has := cfg["route"].(map[string]interface{})["default_domain_strategy"]; has {
		t.Error("route.default_domain_strategy must be removed")
	}
	srv := cfg["dns"].(map[string]interface{})["servers"].([]interface{})[0].(map[string]interface{})
	if _, has := srv["domain_strategy"]; has {
		t.Error("dns server domain_strategy must be removed")
	}
	if again := migrateSingbox115(cfg); len(again) != 0 {
		t.Errorf("not idempotent: %v", again)
	}
}

func TestMigrate115CleanConfigUntouched(t *testing.T) {
	src := `{"log":{"level":"info"},"inbounds":[{"tag":"m","type":"mixed","listen":"127.0.0.1","listen_port":1}],"outbounds":[{"tag":"direct","type":"direct"}],"route":{"final":"direct"}}`
	cfg := parseJSON(t, src)
	if notes := migrateSingbox115(cfg); len(notes) != 0 {
		t.Errorf("clean config must be untouched: %v", notes)
	}
	if !reflect.DeepEqual(cfg, parseJSON(t, src)) {
		t.Error("clean config mutated")
	}
}

func TestMigrateSingbox115Gate(t *testing.T) {
	Singbox115Migration = false
	t.Cleanup(func() { Singbox115Migration = true })
	cfg := parseJSON(t, routerConfig)
	if notes := MigrateSingbox115(cfg); notes != nil {
		t.Errorf("gated off: %v", notes)
	}
	if !reflect.DeepEqual(cfg, parseJSON(t, routerConfig)) {
		t.Error("gated off must not touch the config (an older fork rejects the migrated fields)")
	}
}
