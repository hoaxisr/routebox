package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// enableCacheFile turns on a cache_file that names a path but says nothing
// about enabled: sing-box treats that as off, while the config reads as if the
// cache were set up, and without it every start re-downloads remote rule-sets.
// An explicit "enabled": false stays: that is what the UI saves when the cache
// is switched off, so a bare path can only be an older form's leftover.
//
// A relative path follows the sing-box process's working directory, which may
// be read-only, and an unopenable path kills sing-box at start although
// `check` passes. So it moves next to the config, and the cache stays off if
// that directory cannot be written.
func enableCacheFile(config map[string]interface{}, configDir string) bool {
	exp, _ := config["experimental"].(map[string]interface{})
	cf, _ := exp["cache_file"].(map[string]interface{})
	path, _ := cf["path"].(string)
	if _, has := cf["enabled"]; has || path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		probe, err := os.CreateTemp(configDir, ".routebox-write-probe-*")
		if err != nil {
			return false
		}
		probe.Close()
		os.Remove(probe.Name())
		cf["path"] = filepath.Join(configDir, path)
	}
	cf["enabled"] = true
	return true
}

// ruleSetDetour returns the outbound a remote rule-set downloads through, the
// way sing-box picks the client: its own, else route.default_http_client, else
// the first declared client.
func ruleSetDetour(config, rs map[string]interface{}) string {
	route, _ := config["route"].(map[string]interface{})
	clients, _ := config["http_clients"].([]interface{})
	var name string
	switch hc := rs["http_client"].(type) {
	case map[string]interface{}:
		detour, _ := hc["detour"].(string)
		return detour
	case string:
		name = hc
	default:
		name, _ = route["default_http_client"].(string)
	}
	for i, v := range clients {
		c, _ := v.(map[string]interface{})
		if tag, _ := c["tag"].(string); tag == name || (name == "" && i == 0) {
			detour, _ := c["detour"].(string)
			return detour
		}
	}
	return ""
}

// RuleSetDownloadWarning names remote rule-sets that sing-box must fetch
// through a tunnel at every start. Without a cache the fetch blocks startup
// until it succeeds, and a group has not tested its members yet at that point:
// it dials the first one even when another is alive. Empty = nothing to warn.
func RuleSetDownloadWarning(config map[string]interface{}) string {
	exp, _ := config["experimental"].(map[string]interface{})
	cf, _ := exp["cache_file"].(map[string]interface{})
	if on, _ := cf["enabled"].(bool); on {
		return ""
	}
	route, _ := config["route"].(map[string]interface{})
	sets, _ := route["rule_set"].([]interface{})
	var tags []string
	for _, v := range sets {
		rs, _ := v.(map[string]interface{})
		if t, _ := rs["type"].(string); t != "remote" {
			continue
		}
		if detour := ruleSetDetour(config, rs); detour != "" && !isEmptyDirectOutbound(config, detour) {
			tags = append(tags, fmt.Sprintf("%q", rs["tag"]))
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return fmt.Sprintf("rule sets %s download through a tunnel and the cache file is off: if the tunnel is down, sing-box will not start until the download succeeds. Enable the cache file in Experimental settings", strings.Join(tags, ", "))
}
