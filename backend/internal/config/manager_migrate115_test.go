package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const legacy115File = `{"outbounds":[{"tag":"direct","type":"direct"},{"tag":"proxy","type":"socks","server":"1.2.3.4","server_port":1}],
	"inbounds":[{"tag":"in","type":"vless","listen":"::","listen_port":443,"users":[{"name":"u","uuid":"x"}],
		"tls":{"enabled":true,"acme":{"domain":"vpn.example.com","email":"a@b.c"}}}],
	"route":{"final":"proxy","default_domain_strategy":"prefer_ipv4","rule_set":[{"tag":"ads","type":"remote","url":"u","download_detour":"direct"}]},
	"experimental":{"cache_file":{"enabled":true,"store_rdrc":true}}}`

func assertMigrated115(t *testing.T, cfg map[string]interface{}, where string) {
	t.Helper()
	route := cfg["route"].(map[string]interface{})
	if _, has := route["default_domain_strategy"]; has {
		t.Errorf("%s: default_domain_strategy kept", where)
	}
	rs := route["rule_set"].([]interface{})[0].(map[string]interface{})
	if _, has := rs["download_detour"]; has {
		t.Errorf("%s: download_detour kept", where)
	}
	if route["default_http_client"] != ruleSetHTTPClientTag {
		t.Errorf("%s: default_http_client missing", where)
	}
	if clients, _ := cfg["http_clients"].([]interface{}); len(clients) == 0 {
		t.Errorf("%s: http_clients missing", where)
	}
	tls := cfg["inbounds"].([]interface{})[0].(map[string]interface{})["tls"].(map[string]interface{})
	if _, has := tls["acme"]; has || tls["certificate_provider"] == nil {
		t.Errorf("%s: tls.acme not migrated: %v", where, tls)
	}
	cf := cfg["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
	if _, has := cf["store_rdrc"]; has || cf["store_dns"] != true {
		t.Errorf("%s: store_rdrc not migrated: %v", where, cf)
	}
}

// Load must migrate both in memory and ON DISK: the process is started from
// the file, not from RouteBox's memory.
func TestLoadMigratesSingbox115(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(legacy115File), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	assertMigrated115(t, m.Get(), "memory")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]interface{}{}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	assertMigrated115(t, onDisk, "disk")
}

func TestLoadSkipsSingbox115WhenGatedOff(t *testing.T) {
	Singbox115Migration = false
	t.Cleanup(func() { Singbox115Migration = true })
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(legacy115File), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, has := m.Get()["http_clients"]; has {
		t.Error("gated off (old fork installed): config must not gain 1.14+ fields")
	}
	data, _ := os.ReadFile(path)
	if string(data) != legacy115File {
		t.Error("gated off: file must stay byte-identical")
	}
}

func TestCreateRuleSetAddsHTTPClient(t *testing.T) {
	m := NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	m.activeConfig = map[string]interface{}{
		"outbounds": []interface{}{map[string]interface{}{"tag": "direct", "type": "direct"}},
		"route":     map[string]interface{}{"final": "direct"},
	}
	if err := m.CreateRuleSet(map[string]interface{}{"tag": "loc", "type": "local", "path": "p"}); err != nil {
		t.Fatal(err)
	}
	if _, has := m.Get()["http_clients"]; has {
		t.Error("local rule-set needs no http client")
	}
	if err := m.CreateRuleSet(map[string]interface{}{"tag": "ads", "type": "remote", "url": "u"}); err != nil {
		t.Fatal(err)
	}
	cfg := m.Get()
	clients, _ := cfg["http_clients"].([]interface{})
	if len(clients) != 1 || clients[0].(map[string]interface{})["tag"] != ruleSetHTTPClientTag {
		t.Fatalf("remote rule-set must get the shared client: %v", clients)
	}
	if cfg["route"].(map[string]interface{})["default_http_client"] != ruleSetHTTPClientTag {
		t.Error("default_http_client not set")
	}
	if err := m.CreateRuleSet(map[string]interface{}{"tag": "geo", "type": "remote", "url": "u"}); err != nil {
		t.Fatal(err)
	}
	if clients, _ := m.Get()["http_clients"].([]interface{}); len(clients) != 1 {
		t.Errorf("second remote rule-set must not duplicate the client: %v", clients)
	}
	// Update local → remote also provisions the client.
	m2 := NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	m2.activeConfig = map[string]interface{}{"route": map[string]interface{}{
		"rule_set": []interface{}{map[string]interface{}{"tag": "x", "type": "local", "path": "p"}}}}
	if err := m2.UpdateRuleSet("x", map[string]interface{}{"tag": "x", "type": "remote", "url": "u"}); err != nil {
		t.Fatal(err)
	}
	if clients, _ := m2.Get()["http_clients"].([]interface{}); len(clients) != 1 {
		t.Errorf("update to remote must provision the client: %v", clients)
	}
}

func TestRuleSetRejectsDetourToEmptyDirect(t *testing.T) {
	m := NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	m.activeConfig = map[string]interface{}{
		"outbounds": []interface{}{map[string]interface{}{"tag": "direct", "type": "direct"}, map[string]interface{}{"tag": "proxy", "type": "socks"}},
		"route":     map[string]interface{}{"final": "direct"},
	}
	err := m.CreateRuleSet(map[string]interface{}{"tag": "ads", "type": "remote", "url": "u", "http_client": map[string]interface{}{"detour": "direct"}})
	if err == nil || !hasErrContaining([]string{err.Error()}, "empty direct") {
		t.Errorf("sing-box refuses detour to an empty direct outbound at start: err = %v", err)
	}
	if err := m.CreateRuleSet(map[string]interface{}{"tag": "ads", "type": "remote", "url": "u", "http_client": map[string]interface{}{"detour": "proxy"}}); err != nil {
		t.Errorf("detour to a proxy is fine: %v", err)
	}
}

func TestUpdateExperimentalStoreDNS(t *testing.T) {
	m := NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	m.activeConfig = map[string]interface{}{}
	if err := m.UpdateExperimental(map[string]interface{}{"cache_file": map[string]interface{}{"enabled": true, "store_dns": true, "store_rdrc": true}}); err != nil {
		t.Fatal(err)
	}
	cf := m.Get()["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
	if cf["store_dns"] != true {
		t.Errorf("store_dns not written: %v", cf)
	}
	if _, has := cf["store_rdrc"]; has {
		t.Errorf("store_rdrc must not be written: %v", cf)
	}
	if got := m.GetExperimental()["cache_file"].(map[string]interface{}); got["store_dns"] != true {
		t.Errorf("GetExperimental must read store_dns: %v", got)
	}
}
