package config

// --- Helper functions ---

// findByTag finds object index in array by tag field
func findByTag(arr []interface{}, tag string) int {
	for i, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			if t, ok := obj["tag"].(string); ok && t == tag {
				return i
			}
		}
	}
	return -1
}

// RemoveIPv6FromTunInbounds removes IPv6 addresses from all TUN inbounds in the draft.
// This is necessary when IPv6 is disabled in the system (net.ipv6.conf.all.disable_ipv6=1)
// because sing-box will fail to bind IPv6 addresses to the TUN interface.
// Returns true if any changes were made.
func (m *Manager) RemoveIPv6FromTunInbounds() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure we're working with draft
	if err := m.ensureDraftUnlocked(); err != nil {
		return false
	}

	inbounds, ok := m.draftConfig["inbounds"].([]interface{})
	if !ok {
		return false
	}

	modified := false

	for _, ib := range inbounds {
		inbound, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}

		// Only process TUN inbounds
		if inbound["type"] != "tun" {
			continue
		}

		// Remove IPv6 from modern "address" array format
		if addresses, ok := inbound["address"].([]interface{}); ok {
			var ipv4Only []interface{}
			for _, addr := range addresses {
				addrStr, ok := addr.(string)
				if !ok {
					continue
				}
				// IPv6 addresses contain colons, IPv4 don't
				if !isIPv6Address(addrStr) {
					ipv4Only = append(ipv4Only, addr)
				} else {
					modified = true
				}
			}
			if len(ipv4Only) > 0 {
				inbound["address"] = ipv4Only
			}
		}

		// Remove legacy inet6_address field
		if _, hasIPv6 := inbound["inet6_address"]; hasIPv6 {
			delete(inbound, "inet6_address")
			modified = true
		}
	}

	// Save draft if modified
	if modified {
		m.saveDraftToDisk()
	}

	return modified
}

// isIPv6Address checks if an address string is IPv6 (contains colon)
func isIPv6Address(addr string) bool {
	// IPv6 addresses always contain colons, IPv4 CIDR like "172.19.0.1/30" never do
	for _, c := range addr {
		if c == ':' {
			return true
		}
	}
	return false
}
