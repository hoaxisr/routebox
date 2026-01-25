package config

import "fmt"

// Validate checks if config is valid (comprehensive validation)
func (m *Manager) Validate(config map[string]interface{}) []string {
	var errors []string

	// Check required sections exist
	if _, ok := config["inbounds"]; !ok {
		errors = append(errors, "missing 'inbounds' section")
	}
	if _, ok := config["outbounds"]; !ok {
		errors = append(errors, "missing 'outbounds' section")
	}

	// Collect all valid tags for reference validation
	endpointTags := make(map[string]bool)
	outboundTags := make(map[string]bool)

	// Validate endpoints
	if endpoints, ok := config["endpoints"].([]interface{}); ok {
		for i, ep := range endpoints {
			if obj, ok := ep.(map[string]interface{}); ok {
				errs := validateEndpoint(obj, i)
				errors = append(errors, errs...)
				if tag, ok := obj["tag"].(string); ok && tag != "" {
					endpointTags[tag] = true
				}
			}
		}
	}

	// Validate outbounds and collect tags
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for i, ob := range outbounds {
			if obj, ok := ob.(map[string]interface{}); ok {
				errs := validateOutbound(obj, i)
				errors = append(errors, errs...)
				if tag, ok := obj["tag"].(string); ok && tag != "" {
					outboundTags[tag] = true
				}
			}
		}
	}

	// Validate inbounds
	if inbounds, ok := config["inbounds"].([]interface{}); ok {
		for i, ib := range inbounds {
			if obj, ok := ib.(map[string]interface{}); ok {
				errs := validateInbound(obj, i)
				errors = append(errors, errs...)
			}
		}
	}

	// Reference validation: check outbound references
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for i, ob := range outbounds {
			if obj, ok := ob.(map[string]interface{}); ok {
				errs := validateOutboundReferences(obj, i, endpointTags, outboundTags)
				errors = append(errors, errs...)
			}
		}
	}

	// Reference validation: check route rules reference valid outbounds or endpoints
	// Note: AWG/WireGuard endpoints can be used directly as outbounds in routes
	if route, ok := config["route"].(map[string]interface{}); ok {
		if rules, ok := route["rules"].([]interface{}); ok {
			for i, rule := range rules {
				if obj, ok := rule.(map[string]interface{}); ok {
					if outbound, ok := obj["outbound"].(string); ok {
						// Check both outbounds and endpoints (AWG/WG endpoints work as outbounds)
						if !outboundTags[outbound] && !endpointTags[outbound] {
							errors = append(errors, fmt.Sprintf("route.rules[%d]: outbound '%s' does not exist", i, outbound))
						}
					}
				}
			}
		}
		// Validate final outbound (can also be an endpoint tag)
		if final, ok := route["final"].(string); ok {
			if !outboundTags[final] && !endpointTags[final] {
				errors = append(errors, fmt.Sprintf("route.final: outbound '%s' does not exist", final))
			}
		}
	}

	return errors
}

// validateEndpoint validates a single endpoint object
func validateEndpoint(ep map[string]interface{}, index int) []string {
	var errors []string
	prefix := fmt.Sprintf("endpoints[%d]", index)

	// Required: tag
	tag, hasTag := ep["tag"].(string)
	if !hasTag || tag == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'tag'", prefix))
	}

	// Required: type
	epType, hasType := ep["type"].(string)
	if !hasType || epType == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'type'", prefix))
	}

	// Type-specific validation
	if epType == "awg" || epType == "wireguard" {
		// Required: private_key
		if pk, ok := ep["private_key"].(string); !ok || pk == "" {
			errors = append(errors, fmt.Sprintf("%s: missing 'private_key'", prefix))
		}

		// Required: address (array)
		if addr, ok := ep["address"].([]interface{}); !ok || len(addr) == 0 {
			errors = append(errors, fmt.Sprintf("%s: missing or empty 'address'", prefix))
		}

		// Required: at least one peer
		if peers, ok := ep["peers"].([]interface{}); !ok || len(peers) == 0 {
			errors = append(errors, fmt.Sprintf("%s: missing or empty 'peers'", prefix))
		} else {
			for j, peer := range peers {
				if peerObj, ok := peer.(map[string]interface{}); ok {
					peerPrefix := fmt.Sprintf("%s.peers[%d]", prefix, j)
					if addr, ok := peerObj["address"].(string); !ok || addr == "" {
						errors = append(errors, fmt.Sprintf("%s: missing 'address'", peerPrefix))
					}
					if pk, ok := peerObj["public_key"].(string); !ok || pk == "" {
						errors = append(errors, fmt.Sprintf("%s: missing 'public_key'", peerPrefix))
					}
				}
			}
		}
	}

	return errors
}

// validateOutbound validates a single outbound object
func validateOutbound(ob map[string]interface{}, index int) []string {
	var errors []string
	prefix := fmt.Sprintf("outbounds[%d]", index)

	// Required: tag
	tag, hasTag := ob["tag"].(string)
	if !hasTag || tag == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'tag'", prefix))
	}

	// Required: type
	obType, hasType := ob["type"].(string)
	if !hasType || obType == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'type'", prefix))
	}

	// Type-specific validation
	switch obType {
	case "selector", "urltest":
		// Required: outbounds array
		if outbounds, ok := ob["outbounds"].([]interface{}); !ok || len(outbounds) == 0 {
			errors = append(errors, fmt.Sprintf("%s: selector/urltest requires 'outbounds' array", prefix))
		}
	case "endpoint":
		// Required: endpoint_tag
		if epTag, ok := ob["endpoint_tag"].(string); !ok || epTag == "" {
			errors = append(errors, fmt.Sprintf("%s: endpoint type requires 'endpoint_tag'", prefix))
		}
	}

	return errors
}

// validateInbound validates a single inbound object
func validateInbound(ib map[string]interface{}, index int) []string {
	var errors []string
	prefix := fmt.Sprintf("inbounds[%d]", index)

	// Required: tag
	tag, hasTag := ib["tag"].(string)
	if !hasTag || tag == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'tag'", prefix))
	}

	// Required: type
	ibType, hasType := ib["type"].(string)
	if !hasType || ibType == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'type'", prefix))
	}

	// Type-specific validation
	switch ibType {
	case "tun":
		// TUN requires address - check new unified format first, then legacy
		hasAddress := false
		if addr, ok := ib["address"].([]interface{}); ok && len(addr) > 0 {
			hasAddress = true
		}
		// Fallback: check legacy inet4_address/inet6_address
		if !hasAddress {
			if addr, ok := ib["inet4_address"].(string); ok && addr != "" {
				hasAddress = true
			}
			if addr, ok := ib["inet6_address"].(string); ok && addr != "" {
				hasAddress = true
			}
		}
		if !hasAddress {
			errors = append(errors, fmt.Sprintf("%s: TUN requires 'address' array or 'inet4_address'/'inet6_address'", prefix))
		}
	case "mixed", "socks", "http":
		// Should have listen_port
		if _, ok := ib["listen_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: %s requires 'listen_port'", prefix, ibType))
		}
	}

	return errors
}

// validateOutboundReferences validates that outbound references point to existing objects
func validateOutboundReferences(ob map[string]interface{}, index int, endpointTags, outboundTags map[string]bool) []string {
	var errors []string
	prefix := fmt.Sprintf("outbounds[%d]", index)
	currentTag, _ := ob["tag"].(string)

	obType, _ := ob["type"].(string)

	switch obType {
	case "selector", "urltest":
		// Check that referenced outbounds exist
		if outbounds, ok := ob["outbounds"].([]interface{}); ok {
			for _, ref := range outbounds {
				if refTag, ok := ref.(string); ok {
					// Can reference endpoints or other outbounds (but not self)
					if refTag == currentTag {
						errors = append(errors, fmt.Sprintf("%s: cannot reference itself", prefix))
					} else if !endpointTags[refTag] && !outboundTags[refTag] {
						errors = append(errors, fmt.Sprintf("%s: referenced outbound '%s' does not exist", prefix, refTag))
					}
				}
			}
		}
		// Check default outbound
		if def, ok := ob["default"].(string); ok && def != "" {
			if !endpointTags[def] && !outboundTags[def] {
				errors = append(errors, fmt.Sprintf("%s: default outbound '%s' does not exist", prefix, def))
			}
		}
	case "endpoint":
		// Check that referenced endpoint exists
		if epTag, ok := ob["endpoint_tag"].(string); ok && epTag != "" {
			if !endpointTags[epTag] {
				errors = append(errors, fmt.Sprintf("%s: endpoint '%s' does not exist", prefix, epTag))
			}
		}
	}

	return errors
}

// validateRuleSet validates a single rule set object
func validateRuleSet(rs map[string]interface{}, index int) []string {
	var errors []string
	prefix := fmt.Sprintf("rule_set[%d]", index)

	// Required: tag
	tag, hasTag := rs["tag"].(string)
	if !hasTag || tag == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'tag'", prefix))
	}

	// Required: type
	rsType, hasType := rs["type"].(string)
	if !hasType || rsType == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'type'", prefix))
	} else if rsType != "remote" && rsType != "local" {
		errors = append(errors, fmt.Sprintf("%s: type must be 'remote' or 'local'", prefix))
	}

	// Type-specific validation
	if rsType == "remote" {
		if url, ok := rs["url"].(string); !ok || url == "" {
			errors = append(errors, fmt.Sprintf("%s: remote type requires 'url'", prefix))
		}
	} else if rsType == "local" {
		if path, ok := rs["path"].(string); !ok || path == "" {
			errors = append(errors, fmt.Sprintf("%s: local type requires 'path'", prefix))
		}
	}

	return errors
}

// validateRule validates a single route rule object
// Note: endpointTags are checked because AWG/WireGuard endpoints can be used as outbounds in routes
func validateRule(rule map[string]interface{}, index int, outboundTags, endpointTags, ruleSetTags map[string]bool) []string {
	var errors []string
	prefix := fmt.Sprintf("rules[%d]", index)

	// Check action type - outbound only required for 'route'
	action, _ := rule["action"].(string)
	if action == "" {
		action = "route" // default
	}

	// Validate action type
	validActions := map[string]bool{"route": true, "reject": true, "sniff": true, "hijack-dns": true}
	if !validActions[action] {
		errors = append(errors, fmt.Sprintf("%s: invalid action '%s'", prefix, action))
	}

	// Outbound required only for 'route' action
	if action == "route" {
		outbound, hasOutbound := rule["outbound"].(string)
		if !hasOutbound || outbound == "" {
			errors = append(errors, fmt.Sprintf("%s: missing 'outbound' for route action", prefix))
		} else if !outboundTags[outbound] && !endpointTags[outbound] {
			// Check both outbounds and endpoints (AWG/WG endpoints work as outbounds)
			errors = append(errors, fmt.Sprintf("%s: outbound '%s' does not exist", prefix, outbound))
		}
	}

	// Validate rule_set references
	if ruleSets, ok := rule["rule_set"].([]interface{}); ok {
		for _, rs := range ruleSets {
			if rsTag, ok := rs.(string); ok {
				if !ruleSetTags[rsTag] {
					errors = append(errors, fmt.Sprintf("%s: rule_set '%s' does not exist", prefix, rsTag))
				}
			}
		}
	}

	return errors
}

// validateDnsServer validates a single DNS server object
func validateDnsServer(server map[string]interface{}, index int) []string {
	var errors []string
	prefix := fmt.Sprintf("dns.servers[%d]", index)

	// Required: tag
	tag, hasTag := server["tag"].(string)
	if !hasTag || tag == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'tag'", prefix))
	}

	// Required: type
	serverType, hasType := server["type"].(string)
	if !hasType || serverType == "" {
		errors = append(errors, fmt.Sprintf("%s: missing or empty 'type'", prefix))
	}

	// Validate type value
	validTypes := map[string]bool{"udp": true, "tcp": true, "tls": true, "https": true, "local": true, "fakeip": true}
	if hasType && !validTypes[serverType] {
		errors = append(errors, fmt.Sprintf("%s: invalid type '%s'", prefix, serverType))
	}

	// server address required for most types
	if serverType == "udp" || serverType == "tcp" || serverType == "tls" || serverType == "https" {
		if addr, ok := server["server"].(string); !ok || addr == "" {
			errors = append(errors, fmt.Sprintf("%s: '%s' type requires 'server' address", prefix, serverType))
		}
	}

	return errors
}

// validateDnsRule validates a single DNS rule object
func validateDnsRule(rule map[string]interface{}, index int, serverTags, ruleSetTags map[string]bool) []string {
	var errors []string
	prefix := fmt.Sprintf("dns.rules[%d]", index)

	// Required: server
	server, hasServer := rule["server"].(string)
	if !hasServer || server == "" {
		errors = append(errors, fmt.Sprintf("%s: missing 'server'", prefix))
	} else if !serverTags[server] {
		errors = append(errors, fmt.Sprintf("%s: DNS server '%s' does not exist", prefix, server))
	}

	// Validate rule_set references
	if ruleSets, ok := rule["rule_set"].([]interface{}); ok {
		for _, rs := range ruleSets {
			if rsTag, ok := rs.(string); ok {
				if !ruleSetTags[rsTag] {
					errors = append(errors, fmt.Sprintf("%s: rule_set '%s' does not exist", prefix, rsTag))
				}
			}
		}
	}

	return errors
}
