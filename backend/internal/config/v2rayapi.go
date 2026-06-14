package config

import (
	"reflect"
	"sort"
	"strings"
)

// BuildV2RayAPIBlock builds the experimental.v2ray_api map RouteBox owns: a
// loopback gRPC listener + stats enabled for the given panel-user display-names.
// Names are trimmed, blanks dropped, then sorted+deduped (deterministic block =
// idempotent sync). users is []interface{} to match the JSON-decoded config
// shape. Empty result → nil (caller removes the block: router mode / no users).
// PURE.
func BuildV2RayAPIBlock(listen string, names []string) map[string]interface{} {
	seen := map[string]bool{}
	clean := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		clean = append(clean, n)
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Strings(clean)
	users := make([]interface{}, len(clean))
	for i, n := range clean {
		users[i] = n
	}
	return map[string]interface{}{
		"listen": listen,
		"stats": map[string]interface{}{
			"enabled": true,
			"users":   users,
		},
	}
}

// SyncV2RayAPI makes experimental.v2ray_api in the ACTIVE config reflect names,
// persisting to disk. RouteBox OWNS only the v2ray_api key — sibling experimental
// keys (clash_api, cache_file, ...) are never touched. listen is the loopback
// StatsService addr. names empty → block removed (and experimental dropped if it
// becomes empty). Returns changed=true iff the config was modified.
//
// It builds a deep COPY first, then saveLocked (which backs up + prunes + writes
// atomically AND assigns m.activeConfig ONLY on success) — so a save failure
// never leaves activeConfig diverged from disk. Call from ApplyConfig AFTER
// reconcile.
func (m *Manager) SyncV2RayAPI(listen string, names []string) (changed bool, err error) {
	want := BuildV2RayAPIBlock(listen, names)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readOnly || m.path == "" {
		return false, nil // additivity: read-only / unconfigured => never write
	}

	cfg := m.deepCopy(m.activeConfig)
	exp, _ := cfg["experimental"].(map[string]interface{})
	current, _ := exp["v2ray_api"].(map[string]interface{})

	if reflect.DeepEqual(current, want) {
		return false, nil
	}

	if want == nil {
		if exp != nil {
			delete(exp, "v2ray_api")
			if len(exp) == 0 {
				delete(cfg, "experimental")
			}
		}
	} else {
		if exp == nil {
			exp = map[string]interface{}{}
			cfg["experimental"] = exp
		}
		exp["v2ray_api"] = want
	}

	if err := m.saveLocked(cfg); err != nil {
		return true, err // disk write failed; activeConfig left untouched by saveLocked
	}
	return true, nil
}
