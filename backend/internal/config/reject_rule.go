package config

// buildRejectRule returns the RouteBox-managed route rule that rejects the given
// inbound user names: {"auth_user": names, "action": "reject"}. Returns nil when
// names is empty (caller removes the rule). users is []interface{} to match the
// JSON-decoded config shape. names are copied into a fresh slice so the caller
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
