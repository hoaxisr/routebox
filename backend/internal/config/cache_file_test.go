package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableCacheFile(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, cache, wantPath string
		wantEnabled           interface{}
		changed               bool
	}{
		{"path means on, tied to config dir", `{"path":"cache.db"}`, filepath.Join(dir, "cache.db"), true, true},
		{"absolute path kept", `{"path":"/var/lib/x.db"}`, "/var/lib/x.db", true, true},
		{"explicit off respected", `{"enabled":false,"path":"cache.db"}`, "cache.db", false, false},
		{"already on untouched", `{"enabled":true,"path":"cache.db"}`, "cache.db", true, false},
		{"no path, no intent", `{"store_dns":true}`, "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := parseJSON(t, `{"experimental":{"cache_file":`+c.cache+`}}`)
			if got := enableCacheFile(cfg, dir); got != c.changed {
				t.Fatalf("changed = %v", got)
			}
			cf := cfg["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
			if cf["enabled"] != c.wantEnabled {
				t.Errorf("enabled = %v", cf["enabled"])
			}
			if p, _ := cf["path"].(string); p != c.wantPath {
				t.Errorf("path = %q", p)
			}
		})
	}
}

// sing-box dies at start on a cache path it cannot open while `check` passes,
// so an unwritable directory must leave the cache off.
func TestEnableCacheFileSkipsUnwritableDir(t *testing.T) {
	cfg := parseJSON(t, `{"experimental":{"cache_file":{"path":"cache.db"}}}`)
	if enableCacheFile(cfg, filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("enabled in a directory that cannot be written")
	}
}

// What the settings form saves on "off": the path must go and stay off.
func TestSwitchedOffCacheStaysOff(t *testing.T) {
	m := newTestManager(t)
	if err := m.UpdateExperimental(map[string]interface{}{"cache_file": map[string]interface{}{"enabled": true, "path": "cache.db"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateExperimental(map[string]interface{}{"cache_file": map[string]interface{}{"enabled": false}}); err != nil {
		t.Fatal(err)
	}
	cfg := m.Get()
	if enableCacheFile(cfg, t.TempDir()) {
		t.Fatal("re-enabled a cache the user switched off")
	}
	cf := cfg["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
	if _, has := cf["path"]; has || cf["enabled"] != false {
		t.Fatalf("cache_file: %v", cf)
	}
}

func TestLoadEnablesCacheFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	active := `{"outbounds":[{"type":"direct","tag":"direct"}],"experimental":{"cache_file":{"path":"cache.db"}}}`
	if err := os.WriteFile(path, []byte(active), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"enabled": true`) || !strings.Contains(string(b), filepath.Join(dir, "cache.db")) {
		t.Errorf("on disk: %s", b)
	}
}

func TestRuleSetDownloadWarning(t *testing.T) {
	const outbounds = `"outbounds":[{"type":"direct","tag":"direct"},{"type":"fallback","tag":"awg-fall","outbounds":["a"]}]`
	remote := func(hc string) string {
		return `{"tag":"yt","type":"remote","url":"https://x/y.srs"` + hc + `}`
	}
	cases := []struct {
		name, rest, want string
	}{
		{"inline detour-object, cache off", `"route":{"rule_set":[` + remote(`,"http_client":{"detour":"awg-fall"}`) + `]}`, `"yt"`},
		{"cache on", `"experimental":{"cache_file":{"enabled":true}},"route":{"rule_set":[` + remote(`,"http_client":{"detour":"awg-fall"}`) + `]}`, ""},
		{"direct detour", `"route":{"rule_set":[` + remote(`,"http_client":{"detour":"direct"}`) + `]}`, ""},
		{"no detour", `"route":{"rule_set":[` + remote(``) + `]}`, ""},
		{"named client", `"http_clients":[{"tag":"c","detour":"awg-fall"}],"route":{"rule_set":[` + remote(`,"http_client":"c"`) + `]}`, `"yt"`},
		{"default client", `"http_clients":[{"tag":"c","detour":"awg-fall"}],"route":{"default_http_client":"c","rule_set":[` + remote(``) + `]}`, `"yt"`},
		{"first client as fallback", `"http_clients":[{"tag":"c","detour":"awg-fall"}],"route":{"rule_set":[` + remote(``) + `]}`, `"yt"`},
		{"not remote", `"route":{"rule_set":[{"tag":"yt","type":"inline","rules":[{"domain":["a"]}]}]}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RuleSetDownloadWarning(parseJSON(t, `{`+outbounds+`,`+c.rest+`}`))
			if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
