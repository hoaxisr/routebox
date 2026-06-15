package config

import "reflect"

// buildRejectRule returns the RouteBox-managed route rule that rejects the given
// inbound user names: {"auth_user": names, "action": "reject"}. Returns nil when
// names is empty (caller removes the rule). auth_user is []interface{} to match
// the JSON-decoded config shape. names are copied into a fresh slice so the caller
// cannot alias the rule's backing array. Caller passes already-sorted/deduped
// names. PURE.
func buildRejectRule(names []string) map[string]interface{} {
	if len(names) == 0 {
		return nil
	}
	authUser := make([]interface{}, len(names))
	for i, n := range names {
		authUser[i] = n
	}
	return map[string]interface{}{
		"auth_user": authUser,
		"action":    "reject",
	}
}

// managedRejectRule reports whether a route rule is RouteBox's managed reject
// rule: action=="reject", a NON-EMPTY auth_user list, and EXACTLY those two keys
// (no other match keys). sing-box rejects unknown fields, so a structural marker
// is the only reliable identity — any extra key means it is a user-authored rule
// RouteBox must not touch.
func managedRejectRule(rule map[string]interface{}) bool {
	if len(rule) != 2 {
		return false
	}
	if action, _ := rule["action"].(string); action != "reject" {
		return false
	}
	au, ok := rule["auth_user"].([]interface{})
	return ok && len(au) > 0
}

// SyncRejectRuleActive idempotently reconciles RouteBox's managed reject rule in
// the ACTIVE config to `names` (already sorted/deduped by the caller), persisting
// to disk only on change. It is the twin of SyncV2RayAPI: deep-COPY active,
// mutate the copy, saveLocked (which assigns m.activeConfig ONLY on success).
//
// It returns (false, nil) — a deferral / no-op — when:
//   - the manager is read-only or has no path (additivity), OR
//   - a draft is pending (m.hasDraft): never write active mid-edit; the pending
//     Apply will recompute and write the rule itself.
//
// The managed rule is PREPENDED at route.rules[0] so reject wins over any later
// allow rule. Empty names removes the managed rule (and drops route.rules /
// route if they become empty, mirroring SyncV2RayAPI's experimental cleanup).
func (m *Manager) SyncRejectRuleActive(names []string) (changed bool, err error) {
	want := buildRejectRule(names)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readOnly || m.path == "" {
		return false, nil // additivity: read-only / unconfigured => never write
	}
	if m.hasDraft {
		return false, nil // defer: pending Apply recomputes
	}

	cfg := m.deepCopy(m.activeConfig)
	route, _ := cfg["route"].(map[string]interface{})
	var oldRules []interface{}
	if route != nil {
		oldRules, _ = route["rules"].([]interface{})
	}

	// Rebuild rules without any managed reject rule.
	newRules := make([]interface{}, 0, len(oldRules)+1)
	for _, r := range oldRules {
		if rm, ok := r.(map[string]interface{}); ok && managedRejectRule(rm) {
			continue
		}
		newRules = append(newRules, r)
	}
	// Prepend the new managed rule (if any).
	if want != nil {
		newRules = append([]interface{}{want}, newRules...)
	}
	// Normalize empty -> nil so a no-op against an absent/nil rules slice compares
	// equal (reflect.DeepEqual distinguishes nil from a non-nil empty slice).
	if len(newRules) == 0 {
		newRules = nil
	}

	// No structural change vs. the original rules slice -> no-op.
	if reflect.DeepEqual(oldRules, newRules) {
		return false, nil
	}

	if len(newRules) == 0 {
		// Rules became empty: drop route.rules, and route if it is now empty
		// (twin of SyncV2RayAPI dropping an empty experimental).
		if route != nil {
			delete(route, "rules")
			if len(route) == 0 {
				delete(cfg, "route")
			}
		}
	} else {
		if route == nil {
			route = map[string]interface{}{}
			cfg["route"] = route
		}
		route["rules"] = newRules
	}

	if err := m.saveLocked(cfg); err != nil {
		return true, err // disk write failed; activeConfig left untouched by saveLocked
	}
	return true, nil
}
