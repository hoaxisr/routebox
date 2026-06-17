package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ValidateLocalRuleSetPaths checks that every rule_set of type "local" points
// to a file that exists on disk. sing-box's own failure for this case surfaces
// as a raw FATAL with ANSI codes, so we catch it earlier with a friendly message.
//
// Relative paths are resolved against configDir — the directory holding the
// active config.json — because that's how sing-box's filemanager resolves them.
// If we can't trust configDir (empty string), we skip the existence check
// entirely and let sing-box validate; reporting a bogus "file missing" for a
// file that actually exists (just not relative to RouteBox's CWD) is worse
// than letting sing-box surface the real error.
func ValidateLocalRuleSetPaths(config map[string]interface{}, configDir string) []string {
	var errors []string
	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return errors
	}
	ruleSets, ok := route["rule_set"].([]interface{})
	if !ok {
		return errors
	}
	for i, rs := range ruleSets {
		obj, ok := rs.(map[string]interface{})
		if !ok {
			continue
		}
		if rsType, _ := obj["type"].(string); rsType != "local" {
			continue
		}
		path, _ := obj["path"].(string)
		if path == "" {
			continue // already caught by validateRuleSet
		}
		tag, _ := obj["tag"].(string)
		label := tag
		if label == "" {
			label = fmt.Sprintf("rule_set[%d]", i)
		}

		// Resolve relative to the config directory — same as sing-box does via
		// filemanager.BasePath. Skip the check if we have no base to resolve
		// against, so we never falsely reject a valid relative path.
		resolved := path
		if !filepath.IsAbs(path) {
			if configDir == "" {
				continue
			}
			resolved = filepath.Join(configDir, path)
		}

		info, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("rule-set %q: file %q does not exist", label, resolved))
			} else {
				errors = append(errors, fmt.Sprintf("rule-set %q: cannot access %q: %v", label, resolved, err))
			}
			continue
		}
		if info.IsDir() {
			errors = append(errors, fmt.Sprintf("rule-set %q: %q is a directory, expected a .srs file", label, resolved))
		}
	}
	return errors
}

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

		// Cross-inbound: reject a duplicate (listen, listen_port). Gate on listen_port
		// presence so non-listening inbounds (tun, awg, etc.) are ignored. Wildcard
		// listen spellings (absent, "", "::", "0.0.0.0", ...) are normalized to one
		// bucket via normalizeListenAddr, so an inbound the panel sets to "::" collides
		// with an older/hand-made one that omits listen on the same port (both bind the
		// same wildcard at runtime). Distinct explicit IPs on the same port are allowed.
		type addrKey struct {
			listen string
			port   float64
		}
		seenPorts := map[addrKey]string{} // key -> first inbound tag
		for _, ib := range inbounds {
			obj, ok := ib.(map[string]interface{})
			if !ok {
				continue
			}
			port, ok := obj["listen_port"].(float64)
			if !ok || port == 0 {
				continue
			}
			listen, _ := obj["listen"].(string)
			listen = normalizeListenAddr(listen)
			tag, _ := obj["tag"].(string)
			key := addrKey{listen: listen, port: port}
			if prev, dup := seenPorts[key]; dup {
				errors = append(errors, fmt.Sprintf("inbounds %q and %q share listen %s:%d (ports must be unique across inbounds)", prev, tag, listen, int(port)))
			} else {
				seenPorts[key] = tag
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

	// Cross-section validation: conflicts and incompatibilities from sing-box docs
	errors = append(errors, validateAutoRedirectMarkConflict(config)...)
	errors = append(errors, validateRouteFieldConflicts(config)...)
	errors = append(errors, validateTunAutoRouteRequirements(config)...)
	errors = append(errors, validateTunInterfaceFilterConflict(config)...)
	errors = append(errors, validateDialFieldConflicts(config)...)

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
	case "shadowsocks":
		// Required: server, server_port, method, password
		if s, ok := ob["server"].(string); !ok || s == "" {
			errors = append(errors, fmt.Sprintf("%s: shadowsocks requires 'server'", prefix))
		}
		if _, ok := ob["server_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: shadowsocks requires 'server_port'", prefix))
		}
		if method, ok := ob["method"].(string); !ok || method == "" {
			errors = append(errors, fmt.Sprintf("%s: shadowsocks requires 'method'", prefix))
		} else {
			validMethods := map[string]bool{
				"2022-blake3-aes-128-gcm":       true,
				"2022-blake3-aes-256-gcm":       true,
				"2022-blake3-chacha20-poly1305": true,
				"aes-128-gcm":                   true,
				"aes-192-gcm":                   true,
				"aes-256-gcm":                   true,
				"chacha20-ietf-poly1305":        true,
				"xchacha20-ietf-poly1305":       true,
				"none":                          true,
			}
			if !validMethods[method] {
				errors = append(errors, fmt.Sprintf("%s: invalid shadowsocks method '%s'", prefix, method))
			}
		}
		if pw, ok := ob["password"].(string); !ok || pw == "" {
			errors = append(errors, fmt.Sprintf("%s: shadowsocks requires 'password'", prefix))
		}
		// Conflict check: multiplex and udp_over_tcp
		if mux, ok := ob["multiplex"].(map[string]interface{}); ok {
			if enabled, ok := mux["enabled"].(bool); ok && enabled {
				if uot, ok := ob["udp_over_tcp"].(bool); ok && uot {
					errors = append(errors, fmt.Sprintf("%s: shadowsocks multiplex conflicts with udp_over_tcp", prefix))
				}
			}
		}
	case "shadowtls":
		// Required: server, server_port, tls
		if s, ok := ob["server"].(string); !ok || s == "" {
			errors = append(errors, fmt.Sprintf("%s: shadowtls requires 'server'", prefix))
		}
		if _, ok := ob["server_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: shadowtls requires 'server_port'", prefix))
		}
		if _, ok := ob["tls"].(map[string]interface{}); !ok {
			errors = append(errors, fmt.Sprintf("%s: shadowtls requires 'tls' object", prefix))
		}
		// Password required for version 2 or 3
		version := 0
		if v, ok := ob["version"].(float64); ok {
			version = int(v)
		}
		if version >= 2 {
			if pw, ok := ob["password"].(string); !ok || pw == "" {
				errors = append(errors, fmt.Sprintf("%s: shadowtls v%d requires 'password'", prefix, version))
			}
		}
	case "anytls":
		// Required: server, server_port, password, tls
		if s, ok := ob["server"].(string); !ok || s == "" {
			errors = append(errors, fmt.Sprintf("%s: anytls requires 'server'", prefix))
		}
		if _, ok := ob["server_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: anytls requires 'server_port'", prefix))
		}
		if pw, ok := ob["password"].(string); !ok || pw == "" {
			errors = append(errors, fmt.Sprintf("%s: anytls requires 'password'", prefix))
		}
		if _, ok := ob["tls"].(map[string]interface{}); !ok {
			errors = append(errors, fmt.Sprintf("%s: anytls requires 'tls' object", prefix))
		}
	case "trojan":
		if s, ok := ob["server"].(string); !ok || s == "" {
			errors = append(errors, fmt.Sprintf("%s: trojan requires 'server'", prefix))
		}
		if _, ok := ob["server_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: trojan requires 'server_port'", prefix))
		}
		if pw, ok := ob["password"].(string); !ok || pw == "" {
			errors = append(errors, fmt.Sprintf("%s: trojan requires 'password'", prefix))
		}
		if tlsVal, present := ob["tls"]; present {
			if _, ok := tlsVal.(map[string]interface{}); !ok {
				errors = append(errors, fmt.Sprintf("%s: trojan 'tls' must be an object", prefix))
			}
		}
		// trojan has no flow field — no Vision conflict possible.
	case "vless":
		errors = append(errors, validateOutboundVisionFlow(ob, prefix)...)
	}

	return errors
}

// validateOutboundVisionFlow enforces the Vision-flow conflict on a vless client
// outbound, mirroring the inbound guard validateInboundTransport. xtls-rprx-vision
// requires raw TCP: a non-raw stream transport (ws/grpc/http/httpupgrade) present
// together with that flow is invalid. This is the ONLY config-time guard — same
// as the inbound case, amnezia-box `check` PASSES this config and only fails
// per-connection at runtime ("vision: not a valid supported TLS connection").
func validateOutboundVisionFlow(ob map[string]interface{}, prefix string) []string {
	var errors []string
	flow, _ := ob["flow"].(string)
	if flow != "xtls-rprx-vision" {
		return errors
	}
	tr, ok := ob["transport"].(map[string]interface{})
	if !ok {
		return errors // raw / absent: nothing to validate
	}
	trType, _ := tr["type"].(string)
	if trType == "" || trType == "raw" || trType == "tcp" {
		return errors
	}
	errors = append(errors, fmt.Sprintf("%s: vless: xtls-rprx-vision flow requires raw transport (remove the transport or clear the flow)", prefix))
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
	case "vless", "trojan", "naive", "hysteria2":
		// All server inbounds need a listen_port.
		if _, ok := ib["listen_port"].(float64); !ok {
			errors = append(errors, fmt.Sprintf("%s: %s requires 'listen_port'", prefix, ibType))
		}
		// naive/hysteria2 require enabled TLS. trojan is RouteBox-policy: require
		// tls.enabled OR tls.reality.enabled (the fork allows trojan-over-plaintext;
		// we don't). vless may run plain/Reality without an enabled tls block.
		if ibType == "naive" || ibType == "hysteria2" {
			tls, _ := ib["tls"].(map[string]interface{})
			enabled := false
			if tls != nil {
				enabled, _ = tls["enabled"].(bool)
			}
			if !enabled {
				errors = append(errors, fmt.Sprintf("%s: %s requires TLS (tls.enabled = true)", prefix, ibType))
			}
		}
		if ibType == "trojan" {
			tls, _ := ib["tls"].(map[string]interface{})
			tlsOn, realityOn := false, false
			if tls != nil {
				tlsOn, _ = tls["enabled"].(bool)
				if reality, ok := tls["reality"].(map[string]interface{}); ok {
					realityOn, _ = reality["enabled"].(bool)
				}
			}
			if !tlsOn && !realityOn {
				errors = append(errors, fmt.Sprintf("%s: trojan requires TLS (tls.enabled = true) or Reality (tls.reality.enabled = true)", prefix))
			}
		}
		// Inbound tls must NOT carry utls (outbound/client-only; the binary
		// rejects tls.utls on an inbound).
		if ibType == "vless" || ibType == "trojan" {
			if tls, ok := ib["tls"].(map[string]interface{}); ok {
				if _, hasUtls := tls["utls"]; hasUtls {
					errors = append(errors, fmt.Sprintf("%s: inbound tls.utls is not allowed (utls is outbound/client-only)", prefix))
				}
			}
		}
		// Reject duplicate credentials within this inbound (uuid/username/password
		// by protocol). This keeps the (tag, credential) registry join key unique.
		credField := map[string]string{"vless": "uuid", "trojan": "password", "naive": "username", "hysteria2": "password"}[ibType]
		if users, ok := ib["users"].([]interface{}); ok {
			seen := map[string]bool{}
			for _, u := range users {
				um, ok := u.(map[string]interface{})
				if !ok {
					continue
				}
				cred, _ := um[credField].(string)
				if cred == "" {
					continue
				}
				if seen[cred] {
					errors = append(errors, fmt.Sprintf("%s: duplicate %s %q among users", prefix, credField, cred))
				}
				seen[cred] = true
			}
		}
		errors = append(errors, validateInboundTransport(ib, prefix)...)
	}

	return errors
}

// validateInboundTransport validates the optional stream-transport block of a
// vless/trojan inbound and enforces the Vision-flow conflict. This is the ONLY
// config-time guard for the conflict: amnezia-box `check` PASSES a config with
// xtls-rprx-vision flow + a non-raw transport and only fails per-connection at
// runtime ("vision: not a valid supported TLS connection").
func validateInboundTransport(ib map[string]interface{}, prefix string) []string {
	var errors []string
	ibType, _ := ib["type"].(string)
	if ibType != "vless" && ibType != "trojan" {
		return errors
	}
	tr, ok := ib["transport"].(map[string]interface{})
	if !ok {
		return errors // raw / absent: nothing to validate
	}
	trType, _ := tr["type"].(string)
	if trType == "" || trType == "raw" {
		return errors
	}
	valid := map[string]bool{"ws": true, "grpc": true, "httpupgrade": true}
	if !valid[trType] {
		errors = append(errors, fmt.Sprintf("%s: invalid transport type %q (must be ws, grpc, or httpupgrade; omit transport for raw TCP)", prefix, trType))
		return errors
	}
	// Vision conflict (vless only — trojan has no flow). A non-raw transport
	// present together with any user flow == xtls-rprx-vision is invalid.
	if ibType == "vless" {
		if users, ok := ib["users"].([]interface{}); ok {
			for _, u := range users {
				um, ok := u.(map[string]interface{})
				if !ok {
					continue
				}
				if f, _ := um["flow"].(string); f == "xtls-rprx-vision" {
					errors = append(errors, fmt.Sprintf("%s: vless: xtls-rprx-vision flow requires raw transport (remove the transport or clear the flow)", prefix))
					break
				}
			}
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
		// Check default outbound (selector only — urltest has no default field)
		if obType == "selector" {
			if def, ok := ob["default"].(string); ok && def != "" {
				if !endpointTags[def] && !outboundTags[def] {
					errors = append(errors, fmt.Sprintf("%s: default outbound '%s' does not exist", prefix, def))
				}
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
	} else if rsType != "remote" && rsType != "local" && rsType != "inline" {
		errors = append(errors, fmt.Sprintf("%s: type must be 'remote', 'local', or 'inline'", prefix))
	}

	// Type-specific validation
	switch rsType {
	case "remote":
		if url, ok := rs["url"].(string); !ok || url == "" {
			errors = append(errors, fmt.Sprintf("%s: remote type requires 'url'", prefix))
		}
	case "local":
		if path, ok := rs["path"].(string); !ok || path == "" {
			errors = append(errors, fmt.Sprintf("%s: local type requires 'path'", prefix))
		}
	case "inline":
		if _, ok := rs["rules"].([]interface{}); !ok {
			errors = append(errors, fmt.Sprintf("%s: inline type requires 'rules' array", prefix))
		}
	}

	return errors
}

// validateRule validates a single route rule object
func validateRule(rule map[string]interface{}, index int, outboundTags, endpointTags, ruleSetTags map[string]bool) []string {
	var errors []string
	prefix := fmt.Sprintf("rules[%d]", index)

	// Check action type
	action, _ := rule["action"].(string)
	if action == "" {
		action = "route" // default
	}

	// Validate action type
	validActions := map[string]bool{
		"route": true, "reject": true, "sniff": true, "hijack-dns": true,
		"route-options": true, "resolve": true,
	}
	if !validActions[action] {
		errors = append(errors, fmt.Sprintf("%s: invalid action '%s'", prefix, action))
	}

	// Outbound required only for 'route' action
	if action == "route" {
		outbound, hasOutbound := rule["outbound"].(string)
		if !hasOutbound || outbound == "" {
			errors = append(errors, fmt.Sprintf("%s: missing 'outbound' for route action", prefix))
		} else if !outboundTags[outbound] && !endpointTags[outbound] {
			errors = append(errors, fmt.Sprintf("%s: outbound '%s' does not exist", prefix, outbound))
		}
	}

	// Validate reject action fields
	if action == "reject" {
		if method, ok := rule["method"].(string); ok && method != "" {
			validMethods := map[string]bool{"default": true, "drop": true}
			if !validMethods[method] {
				errors = append(errors, fmt.Sprintf("%s: invalid reject method '%s' (must be 'default' or 'drop')", prefix, method))
			}
		}
	}

	// Validate resolve action fields
	if action == "resolve" {
		if strategy, ok := rule["strategy"].(string); ok && strategy != "" {
			validStrategies := map[string]bool{"prefer_ipv4": true, "prefer_ipv6": true, "ipv4_only": true, "ipv6_only": true}
			if !validStrategies[strategy] {
				errors = append(errors, fmt.Sprintf("%s: invalid resolve strategy '%s'", prefix, strategy))
			}
		}
	}

	// Validate route-options / route action fields
	if action == "route-options" || action == "route" {
		if strategy, ok := rule["network_strategy"].(string); ok && strategy != "" {
			validStrategies := map[string]bool{"default": true, "hybrid": true, "fallback": true}
			if !validStrategies[strategy] {
				errors = append(errors, fmt.Sprintf("%s: invalid network_strategy '%s'", prefix, strategy))
			}
		}
		if port, ok := rule["override_port"].(float64); ok && port <= 0 {
			errors = append(errors, fmt.Sprintf("%s: override_port must be > 0", prefix))
		}
	}

	// TLS fragment fields (≥1.12): tls_fragment and tls_record_fragment are booleans,
	// tls_fragment_fallback_delay is a duration string — sing-box validates the values

	// Validate matching conditions
	if ipVersion, ok := rule["ip_version"].(float64); ok {
		if ipVersion != 4 && ipVersion != 6 {
			errors = append(errors, fmt.Sprintf("%s: ip_version must be 4 or 6", prefix))
		}
	}

	// Validate process_path_regex - check regex compilation
	if regexes, ok := rule["process_path_regex"].([]interface{}); ok {
		for i, r := range regexes {
			if s, ok := r.(string); ok {
				if _, err := regexp.Compile(s); err != nil {
					errors = append(errors, fmt.Sprintf("%s: process_path_regex[%d] invalid regex: %v", prefix, i, err))
				}
			}
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

// validateAutoRedirectMarkConflict checks that auto_redirect on TUN inbounds
// does not conflict with route.default_mark or outbound routing_mark.
// See: https://sing-box.sagernet.org/configuration/inbound/tun/
func validateAutoRedirectMarkConflict(config map[string]interface{}) []string {
	var errors []string

	// Check if any TUN inbound has auto_redirect enabled
	hasAutoRedirect := false
	if inbounds, ok := config["inbounds"].([]interface{}); ok {
		for _, ib := range inbounds {
			if obj, ok := ib.(map[string]interface{}); ok {
				ibType, _ := obj["type"].(string)
				if ibType == "tun" {
					if ar, ok := obj["auto_redirect"].(bool); ok && ar {
						hasAutoRedirect = true
						break
					}
				}
			}
		}
	}

	if !hasAutoRedirect {
		return errors
	}

	// Check route.default_mark conflict
	if route, ok := config["route"].(map[string]interface{}); ok {
		if mark, exists := route["default_mark"]; exists {
			if markNum, ok := mark.(float64); ok && markNum != 0 {
				errors = append(errors, "route.default_mark conflicts with tun auto_redirect — they use overlapping firewall marks (see sing-box docs)")
			}
		}
	}

	// Check outbound routing_mark conflict
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for i, ob := range outbounds {
			if obj, ok := ob.(map[string]interface{}); ok {
				if mark, exists := obj["routing_mark"]; exists {
					if markNum, ok := mark.(float64); ok && markNum != 0 {
						tag, _ := obj["tag"].(string)
						errors = append(errors, fmt.Sprintf("outbounds[%d] (%s): routing_mark conflicts with tun auto_redirect — they use overlapping firewall marks (see sing-box docs)", i, tag))
					}
				}
			}
		}
	}

	return errors
}

// validateRouteFieldConflicts checks route-level field conflicts.
// See: https://sing-box.sagernet.org/configuration/route/
func validateRouteFieldConflicts(config map[string]interface{}) []string {
	var errors []string

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return errors
	}

	// default_interface conflicts with default_network_strategy
	_, hasDefaultInterface := route["default_interface"].(string)
	_, hasNetworkStrategy := route["default_network_strategy"].(string)
	if hasDefaultInterface && hasNetworkStrategy {
		errors = append(errors, "route: default_interface conflicts with default_network_strategy — cannot use both simultaneously")
	}

	// auto_detect_interface makes default_interface ineffective
	autoDetect, _ := route["auto_detect_interface"].(bool)
	if autoDetect && hasDefaultInterface {
		errors = append(errors, "warning: route.default_interface takes no effect when auto_detect_interface is enabled")
	}

	return errors
}

// validateTunAutoRouteRequirements checks that TUN auto_route has a loop prevention mechanism.
// See: https://sing-box.sagernet.org/configuration/inbound/tun/
func validateTunAutoRouteRequirements(config map[string]interface{}) []string {
	var errors []string

	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return errors
	}

	for i, ib := range inbounds {
		obj, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		ibType, _ := obj["type"].(string)
		if ibType != "tun" {
			continue
		}
		autoRoute, _ := obj["auto_route"].(bool)
		if !autoRoute {
			continue
		}

		// auto_route requires at least one loop prevention mechanism:
		// route.auto_detect_interface, route.default_interface, or outbound.bind_interface
		hasLoopPrevention := false

		if route, ok := config["route"].(map[string]interface{}); ok {
			if adi, ok := route["auto_detect_interface"].(bool); ok && adi {
				hasLoopPrevention = true
			}
			if di, ok := route["default_interface"].(string); ok && di != "" {
				hasLoopPrevention = true
			}
		}

		// Check if any outbound has bind_interface
		if !hasLoopPrevention {
			if outbounds, ok := config["outbounds"].([]interface{}); ok {
				for _, ob := range outbounds {
					if obObj, ok := ob.(map[string]interface{}); ok {
						if bi, ok := obObj["bind_interface"].(string); ok && bi != "" {
							hasLoopPrevention = true
							break
						}
					}
				}
			}
		}

		if !hasLoopPrevention {
			errors = append(errors, fmt.Sprintf("inbounds[%d]: tun auto_route requires route.auto_detect_interface, route.default_interface, or outbound.bind_interface to prevent routing loops", i))
		}

		// route_address_set / route_exclude_address_set require auto_route + auto_redirect
		autoRedirect, _ := obj["auto_redirect"].(bool)
		hasRouteAddressSet := false
		if _, ok := obj["route_address_set"].([]interface{}); ok {
			hasRouteAddressSet = true
		}
		if _, ok := obj["route_exclude_address_set"].([]interface{}); ok {
			hasRouteAddressSet = true
		}
		if hasRouteAddressSet && !autoRedirect {
			errors = append(errors, fmt.Sprintf("warning: inbounds[%d]: route_address_set/route_exclude_address_set with auto_redirect disabled will be treated as route_address/route_exclude_address (nftables optimization unavailable)", i))
		}
	}

	return errors
}

// validateTunInterfaceFilterConflict checks that include_interface and exclude_interface
// are not used together on TUN inbounds.
// See: https://sing-box.sagernet.org/configuration/inbound/tun/
func validateTunInterfaceFilterConflict(config map[string]interface{}) []string {
	var errors []string

	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return errors
	}

	for i, ib := range inbounds {
		obj, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		ibType, _ := obj["type"].(string)
		if ibType != "tun" {
			continue
		}

		_, hasInclude := obj["include_interface"].([]interface{})
		_, hasExclude := obj["exclude_interface"].([]interface{})
		if hasInclude && hasExclude {
			errors = append(errors, fmt.Sprintf("inbounds[%d]: include_interface and exclude_interface conflict — cannot use both on the same tun inbound", i))
		}
	}

	return errors
}

// validateDialFieldConflicts checks outbound dial field conflicts.
// See: https://sing-box.sagernet.org/configuration/shared/dial/
func validateDialFieldConflicts(config map[string]interface{}) []string {
	var errors []string

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return errors
	}

	for i, ob := range outbounds {
		obj, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := obj["tag"].(string)
		prefix := fmt.Sprintf("outbounds[%d] (%s)", i, tag)

		// network_strategy conflicts with bind_interface, inet4_bind_address, inet6_bind_address
		_, hasNetStrategy := obj["network_strategy"].(string)
		_, hasBindInterface := obj["bind_interface"].(string)
		_, hasInet4Bind := obj["inet4_bind_address"].(string)
		_, hasInet6Bind := obj["inet6_bind_address"].(string)

		if hasNetStrategy && (hasBindInterface || hasInet4Bind || hasInet6Bind) {
			errors = append(errors, fmt.Sprintf("%s: network_strategy conflicts with bind_interface/inet4_bind_address/inet6_bind_address", prefix))
		}
	}

	// Check route.default_network_strategy vs outbound bind fields (warning only — per-outbound overrides route default)
	route, hasRoute := config["route"].(map[string]interface{})
	if hasRoute {
		if _, hasDefaultNetStrategy := route["default_network_strategy"].(string); hasDefaultNetStrategy {
			for i, ob := range outbounds {
				obj, ok := ob.(map[string]interface{})
				if !ok {
					continue
				}
				tag, _ := obj["tag"].(string)
				_, hasBindInterface := obj["bind_interface"].(string)
				_, hasInet4Bind := obj["inet4_bind_address"].(string)
				_, hasInet6Bind := obj["inet6_bind_address"].(string)
				if hasBindInterface || hasInet4Bind || hasInet6Bind {
					errors = append(errors, fmt.Sprintf("warning: outbounds[%d] (%s): route.default_network_strategy takes no effect because bind_interface/bind_address is set on this outbound", i, tag))
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

	// Server is required only for route action (default when action is not set)
	action, _ := rule["action"].(string)
	if action == "" || action == "route" {
		server, hasServer := rule["server"].(string)
		if !hasServer || server == "" {
			errors = append(errors, fmt.Sprintf("%s: missing 'server'", prefix))
		} else if !serverTags[server] {
			errors = append(errors, fmt.Sprintf("%s: DNS server '%s' does not exist", prefix, server))
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
