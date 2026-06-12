package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// RuleSetCache caches downloaded rule sets
type RuleSetCache struct {
	mu       sync.RWMutex
	cacheDir string
	entries  map[string]cacheEntry
}

type cacheEntry struct {
	path      string
	expiresAt time.Time
}

var ruleSetCache = &RuleSetCache{
	cacheDir: filepath.Join(os.TempDir(), "routebox-rulesets"),
	entries:  make(map[string]cacheEntry),
}

// TestRouteRequest is the request body for route testing
type TestRouteRequest struct {
	Domain string `json:"domain"` // Domain or IP to test
	Port   int    `json:"port"`   // Optional port (default 443)
}

// TestRouteResponse is the response for route testing
type TestRouteResponse struct {
	Input       string          `json:"input"`
	InputType   string          `json:"input_type"` // "domain" or "ip"
	Matches     []RuleMatch     `json:"matches"`
	Destination string          `json:"destination"`  // Final outbound
	MatchedRule int             `json:"matched_rule"` // Index of matched rule, -1 if final
	Debug       *InspectorDebug `json:"debug,omitempty"`
}

// InspectorDebug contains debug info for troubleshooting
type InspectorDebug struct {
	RuleCount     int      `json:"rule_count"`
	RuleSetCount  int      `json:"rule_set_count"`
	RuleSetTags   []string `json:"rule_set_tags"`
	FinalOutbound string   `json:"final_outbound"`
}

// RuleMatch represents a single rule check result
type RuleMatch struct {
	Index      int      `json:"index"`
	Matched    bool     `json:"matched"`
	Action     string   `json:"action"` // route, reject, sniff, hijack-dns
	Outbound   string   `json:"outbound,omitempty"`
	Reason     string   `json:"reason,omitempty"`     // Why it matched/didn't match
	Conditions []string `json:"conditions,omitempty"` // What conditions were checked
	RuleKeys   []string `json:"rule_keys,omitempty"`  // Debug: all keys in the rule
}

// TestRoute tests which rule matches a given domain/IP
func (h *Handler) TestRoute(w http.ResponseWriter, r *http.Request) {
	var req TestRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	if req.Port == 0 {
		req.Port = 443
	}

	// Detect if input is IP or domain
	inputType := "domain"
	if net.ParseIP(req.Domain) != nil {
		inputType = "ip"
	}

	// Get rules and rule sets from config
	rules := h.config.ListRules()
	ruleSets := h.config.ListRuleSets()
	routeSettings := h.config.GetRouteSettings()

	// Build rule set map for quick lookup
	ruleSetMap := make(map[string]map[string]interface{})
	ruleSetTags := []string{}
	for _, rs := range ruleSets {
		if tag, ok := rs["tag"].(string); ok {
			ruleSetMap[tag] = rs
			ruleSetTags = append(ruleSetTags, tag)
		}
	}

	configDir := filepath.Dir(h.config.GetPath())

	var matches []RuleMatch
	var destination string
	matchedRule := -1

	for i, rule := range rules {
		match := checkRuleMatch(req.Domain, inputType, rule, ruleSetMap, configDir)
		match.Index = i

		matches = append(matches, match)

		// First matching rule determines destination
		if match.Matched && matchedRule == -1 {
			matchedRule = i
			action := "route"
			if a, ok := rule["action"].(string); ok {
				action = a
			}

			switch action {
			case "route":
				if ob, ok := rule["outbound"].(string); ok {
					destination = ob
				}
			case "reject":
				method := "default"
				if m, ok := rule["method"].(string); ok {
					method = m
				}
				if method == "drop" {
					destination = "REJECT (drop)"
				} else {
					destination = "REJECT"
				}
			case "resolve":
				// resolve is a non-final action — it resolves DNS then continues
				matchedRule = -1 // reset so processing continues
			case "route-options":
				// route-options is a non-final action — it sets options then continues
				matchedRule = -1 // reset so processing continues
			}
			// sniff and hijack-dns don't set final destination
		}
	}

	// Get final outbound
	finalOutbound := "direct"
	if final, ok := routeSettings["final"].(string); ok && final != "" {
		finalOutbound = final
	}

	// If no rule matched, use final outbound
	if destination == "" {
		destination = finalOutbound
	}

	writeSuccess(w, TestRouteResponse{
		Input:       req.Domain,
		InputType:   inputType,
		Matches:     matches,
		Destination: destination,
		MatchedRule: matchedRule,
		Debug: &InspectorDebug{
			RuleCount:     len(rules),
			RuleSetCount:  len(ruleSets),
			RuleSetTags:   ruleSetTags,
			FinalOutbound: finalOutbound,
		},
	})
}

// checkRuleMatch checks if a domain/IP matches a rule. configDir is the
// directory of the active config file; relative local rule-set paths resolve
// against it (matching sing-box's filemanager behavior).
func checkRuleMatch(input, inputType string, rule map[string]interface{}, ruleSetMap map[string]map[string]interface{}, configDir string) RuleMatch {
	match := RuleMatch{
		Matched: false,
	}

	// Debug: capture all keys in the rule
	ruleKeys := make([]string, 0, len(rule))
	for k := range rule {
		ruleKeys = append(ruleKeys, k)
	}
	match.RuleKeys = ruleKeys

	// Get action
	action := "route"
	if a, ok := rule["action"].(string); ok {
		action = a
	}
	match.Action = action

	// Get outbound
	if ob, ok := rule["outbound"].(string); ok {
		match.Outbound = ob
	}

	var conditions []string
	var reasons []string

	// Handle logical rules (type: "logical" with mode: "and" | "or")
	if ruleType, ok := rule["type"].(string); ok && ruleType == "logical" {
		mode, _ := rule["mode"].(string)
		if mode == "" {
			mode = "and" // default
		}
		conditions = append(conditions, fmt.Sprintf("logical(%s)", mode))

		if nestedRules, ok := rule["rules"].([]interface{}); ok && len(nestedRules) > 0 {
			var nestedMatches []bool
			for _, nr := range nestedRules {
				if nestedRule, ok := nr.(map[string]interface{}); ok {
					nestedMatch := checkRuleMatch(input, inputType, nestedRule, ruleSetMap, configDir)
					nestedMatches = append(nestedMatches, nestedMatch.Matched)
					if nestedMatch.Matched {
						reasons = append(reasons, nestedMatch.Reason)
					}
				}
			}

			if mode == "and" {
				match.Matched = true
				for _, m := range nestedMatches {
					if !m {
						match.Matched = false
						break
					}
				}
			} else { // or
				for _, m := range nestedMatches {
					if m {
						match.Matched = true
						break
					}
				}
			}
		}

		// Handle invert for logical rules
		if invert, ok := rule["invert"].(bool); ok && invert {
			match.Matched = !match.Matched
			if match.Matched {
				reasons = append(reasons, "inverted")
			}
		}

		match.Conditions = conditions
		if len(reasons) > 0 {
			match.Reason = strings.Join(reasons, ", ")
		} else if match.Matched {
			match.Reason = "logical match"
		} else {
			match.Reason = "no match"
		}
		return match
	}

	// Check ip_is_private
	if ipPrivate, ok := rule["ip_is_private"].(bool); ok && ipPrivate {
		conditions = append(conditions, "ip_is_private")
		if inputType == "ip" {
			if isPrivateIP(input) {
				match.Matched = true
				reasons = append(reasons, "private IP")
			}
			// If IP is not private, no reason is added, will show "no match"
		} else {
			reasons = append(reasons, "ip_is_private: need IP input")
		}
	}

	// Check domain conditions (only for domain input)
	if inputType == "domain" {
		// domain (exact match)
		if domains, ok := getStringSlice(rule, "domain"); ok && len(domains) > 0 {
			conditions = append(conditions, fmt.Sprintf("domain(%d)", len(domains)))
			for _, d := range domains {
				if strings.EqualFold(input, d) {
					match.Matched = true
					reasons = append(reasons, fmt.Sprintf("domain=%s", d))
					break
				}
			}
		}

		// domain_suffix
		if suffixes, ok := getStringSlice(rule, "domain_suffix"); ok && len(suffixes) > 0 {
			conditions = append(conditions, fmt.Sprintf("domain_suffix(%d)", len(suffixes)))
			for _, suffix := range suffixes {
				if matchesDomainSuffix(input, suffix) {
					match.Matched = true
					reasons = append(reasons, fmt.Sprintf("domain_suffix=%s", suffix))
					break
				}
			}
		}

		// domain_keyword
		if keywords, ok := getStringSlice(rule, "domain_keyword"); ok && len(keywords) > 0 {
			conditions = append(conditions, fmt.Sprintf("domain_keyword(%d)", len(keywords)))
			for _, kw := range keywords {
				if strings.Contains(strings.ToLower(input), strings.ToLower(kw)) {
					match.Matched = true
					reasons = append(reasons, fmt.Sprintf("domain_keyword=%s", kw))
					break
				}
			}
		}

		// domain_regex
		if regexes, ok := getStringSlice(rule, "domain_regex"); ok && len(regexes) > 0 {
			conditions = append(conditions, fmt.Sprintf("domain_regex(%d)", len(regexes)))
			for _, pattern := range regexes {
				if re, err := regexp.Compile(pattern); err == nil && re.MatchString(input) {
					match.Matched = true
					reasons = append(reasons, fmt.Sprintf("domain_regex=%s", pattern))
					break
				}
			}
		}
	}

	// Check IP conditions (only for IP input, or domain that could resolve)
	if inputType == "ip" {
		if cidrs, ok := getStringSlice(rule, "ip_cidr"); ok && len(cidrs) > 0 {
			conditions = append(conditions, fmt.Sprintf("ip_cidr(%d)", len(cidrs)))
			ip := net.ParseIP(input)
			if ip != nil {
				for _, cidr := range cidrs {
					_, network, err := net.ParseCIDR(cidr)
					if err != nil {
						// Try as single IP
						if singleIP := net.ParseIP(cidr); singleIP != nil && singleIP.Equal(ip) {
							match.Matched = true
							reasons = append(reasons, fmt.Sprintf("ip=%s", cidr))
							break
						}
						continue
					}
					if network.Contains(ip) {
						match.Matched = true
						reasons = append(reasons, fmt.Sprintf("ip_cidr=%s", cidr))
						break
					}
				}
			}
		}
	}

	// Check rule_set (requires sing-box CLI)
	if ruleSetsRefs, ok := getStringSlice(rule, "rule_set"); ok && len(ruleSetsRefs) > 0 {
		conditions = append(conditions, fmt.Sprintf("rule_set(%d)", len(ruleSetsRefs)))
		for _, rsTag := range ruleSetsRefs {
			if rs, exists := ruleSetMap[rsTag]; exists {
				matched, err := checkRuleSetMatch(input, rs, configDir)
				if err != nil {
					reasons = append(reasons, fmt.Sprintf("rule_set %s: error - %v", rsTag, err))
				} else if matched {
					match.Matched = true
					reasons = append(reasons, fmt.Sprintf("rule_set=%s", rsTag))
					break
				} else {
					// Rule set was checked but didn't match - include URL/path for debugging
					rsURL, _ := rs["url"].(string)
					rsPath, _ := rs["path"].(string)
					location := rsURL
					if location == "" {
						location = rsPath
					}
					reasons = append(reasons, fmt.Sprintf("rule_set %s: no match (%s)", rsTag, location))
				}
			} else {
				reasons = append(reasons, fmt.Sprintf("rule_set %s: not found in config", rsTag))
			}
		}
	}

	// Note other conditions we can't check but should show they exist
	// These conditions require runtime network context
	if protocols, ok := getStringSlice(rule, "protocol"); ok && len(protocols) > 0 {
		conditions = append(conditions, fmt.Sprintf("protocol(%d)", len(protocols)))
		reasons = append(reasons, "protocol: can't verify")
	}
	if inbounds, ok := getStringSlice(rule, "inbound"); ok && len(inbounds) > 0 {
		conditions = append(conditions, fmt.Sprintf("inbound(%d)", len(inbounds)))
		reasons = append(reasons, "inbound: can't verify")
	}
	if _, ok := rule["network"].(string); ok {
		conditions = append(conditions, "network")
		reasons = append(reasons, "network: can't verify")
	}
	if ports, ok := rule["port"].([]interface{}); ok && len(ports) > 0 {
		conditions = append(conditions, fmt.Sprintf("port(%d)", len(ports)))
		reasons = append(reasons, "port: can't verify")
	}
	if portRanges, ok := getStringSlice(rule, "port_range"); ok && len(portRanges) > 0 {
		conditions = append(conditions, fmt.Sprintf("port_range(%d)", len(portRanges)))
		reasons = append(reasons, "port_range: can't verify")
	}
	if processNames, ok := getStringSlice(rule, "process_name"); ok && len(processNames) > 0 {
		conditions = append(conditions, fmt.Sprintf("process_name(%d)", len(processNames)))
		reasons = append(reasons, "process_name: can't verify")
	}
	if sourceCidrs, ok := getStringSlice(rule, "source_ip_cidr"); ok && len(sourceCidrs) > 0 {
		conditions = append(conditions, fmt.Sprintf("source_ip_cidr(%d)", len(sourceCidrs)))
		reasons = append(reasons, "source_ip_cidr: can't verify")
	}
	if _, ok := rule["source_ip_is_private"].(bool); ok {
		conditions = append(conditions, "source_ip_is_private")
		reasons = append(reasons, "source_ip_is_private: can't verify")
	}
	if _, ok := rule["ip_version"]; ok {
		conditions = append(conditions, "ip_version")
	}
	if _, ok := rule["clash_mode"].(string); ok {
		conditions = append(conditions, "clash_mode")
		reasons = append(reasons, "clash_mode: can't verify")
	}
	if authUsers, ok := getStringSlice(rule, "auth_user"); ok && len(authUsers) > 0 {
		conditions = append(conditions, fmt.Sprintf("auth_user(%d)", len(authUsers)))
		reasons = append(reasons, "auth_user: can't verify")
	}
	if clients, ok := getStringSlice(rule, "client"); ok && len(clients) > 0 {
		conditions = append(conditions, fmt.Sprintf("client(%d)", len(clients)))
		reasons = append(reasons, "client: can't verify")
	}
	if users, ok := getStringSlice(rule, "user"); ok && len(users) > 0 {
		conditions = append(conditions, fmt.Sprintf("user(%d)", len(users)))
		reasons = append(reasons, "user: can't verify")
	}
	if processPaths, ok := getStringSlice(rule, "process_path"); ok && len(processPaths) > 0 {
		conditions = append(conditions, fmt.Sprintf("process_path(%d)", len(processPaths)))
		reasons = append(reasons, "process_path: can't verify")
	}
	if processPathRegex, ok := getStringSlice(rule, "process_path_regex"); ok && len(processPathRegex) > 0 {
		conditions = append(conditions, fmt.Sprintf("process_path_regex(%d)", len(processPathRegex)))
		reasons = append(reasons, "process_path_regex: can't verify")
	}

	// Handle invert
	if invert, ok := rule["invert"].(bool); ok && invert {
		match.Matched = !match.Matched
		if match.Matched {
			reasons = append(reasons, "inverted")
		}
	}

	match.Conditions = conditions
	if len(reasons) > 0 {
		match.Reason = strings.Join(reasons, ", ")
	} else if len(conditions) > 0 {
		match.Reason = "no match"
	} else {
		match.Reason = "no conditions"
	}

	return match
}

// checkRuleSetMatch uses sing-box CLI to check if input matches a rule set.
// Relative local paths resolve against configDir (the config file's directory).
func checkRuleSetMatch(input string, ruleSet map[string]interface{}, configDir string) (bool, error) {
	rsType, _ := ruleSet["type"].(string)
	rsURL, _ := ruleSet["url"].(string)
	rsPath, _ := ruleSet["path"].(string)
	format, _ := ruleSet["format"].(string)

	var ruleSetPath string

	if rsType == "remote" && rsURL != "" {
		// Determine format from config or URL extension
		if format == "" {
			if strings.HasSuffix(rsURL, ".srs") {
				format = "binary"
			} else if strings.HasSuffix(rsURL, ".json") {
				format = "source"
			} else {
				format = "binary" // sing-box remote default
			}
		}
		// Download and cache
		var err error
		ruleSetPath, err = ruleSetCache.getOrDownload(rsURL, format)
		if err != nil {
			return false, fmt.Errorf("download %s: %w", rsURL, err)
		}
	} else if rsType == "local" && rsPath != "" {
		ruleSetPath = rsPath
		if !filepath.IsAbs(ruleSetPath) && configDir != "" {
			ruleSetPath = filepath.Join(configDir, ruleSetPath)
		}
		// Determine format from config or extension
		if format == "" {
			if strings.HasSuffix(rsPath, ".srs") {
				format = "binary"
			} else {
				format = "source"
			}
		}
	} else {
		return false, fmt.Errorf("invalid rule set: type=%s, url=%s, path=%s", rsType, rsURL, rsPath)
	}

	// Check file exists
	if _, err := os.Stat(ruleSetPath); err != nil {
		return false, fmt.Errorf("rule set file not found: %s", ruleSetPath)
	}

	// Find sing-box binary
	singboxPath := findSingboxBinary()
	if singboxPath == "" {
		return false, fmt.Errorf("sing-box/amnezia-box binary not found")
	}

	// Run sing-box rule-set match
	args := []string{"rule-set", "match", "-f", format, ruleSetPath, input}
	cmd := exec.Command(singboxPath, args...)

	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Get output strings
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	// sing-box outputs match results to stderr (not stdout!)
	// Check if stderr contains a match indicator
	hasMatch := strings.Contains(stderrStr, "match rules.") || strings.Contains(stdoutStr, "match rules.")

	if err != nil {
		// Check if it's just a non-zero exit (which sing-box uses for "no match")
		if exitErr, ok := err.(*exec.ExitError); ok {
			// If we have match output despite the error, it's still a match
			if hasMatch {
				return true, nil
			}
			// Non-zero exit with stderr that's NOT a match means actual error
			if stderrStr != "" && !strings.Contains(stderrStr, "match") {
				return false, fmt.Errorf("sing-box error: %s", stderrStr)
			}
			// Non-zero exit without match output means no match
			_ = exitErr // silence unused warning
			return false, nil
		} else {
			return false, fmt.Errorf("exec error: %w", err)
		}
	}

	// Check for match in either stdout or stderr
	return hasMatch, nil
}

// getOrDownload gets a cached rule set or downloads it
func (c *RuleSetCache) getOrDownload(url, format string) (string, error) {
	// Create cache key from URL hash
	hash := sha256.Sum256([]byte(url))
	cacheKey := hex.EncodeToString(hash[:8])

	ext := ".srs"
	if format == "source" {
		ext = ".json"
	}
	filename := cacheKey + ext

	c.mu.RLock()
	entry, exists := c.entries[url]
	c.mu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		if _, err := os.Stat(entry.path); err == nil {
			return entry.path, nil
		}
	}

	// Need to download
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return "", err
	}

	filePath := filepath.Join(c.cacheDir, filename)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return "", err
	}

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		return "", err
	}

	// Ensure data is flushed to disk
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	// Verify file was written correctly
	if written == 0 {
		return "", fmt.Errorf("downloaded 0 bytes from %s", url)
	}

	// Cache for 1 hour
	c.mu.Lock()
	c.entries[url] = cacheEntry{
		path:      filePath,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	c.mu.Unlock()

	return filePath, nil
}

// Helper functions

func findSingboxBinary() string {
	// Check common locations
	paths := []string{
		"/usr/local/bin/sing-box",
		"/usr/bin/sing-box",
		"/usr/local/bin/amnezia-box",
		"/usr/bin/amnezia-box",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try PATH
	if path, err := exec.LookPath("sing-box"); err == nil {
		return path
	}
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		return path
	}

	return ""
}

func getStringSlice(m map[string]interface{}, key string) ([]string, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}

	switch v := val.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, len(result) > 0
	}
	return nil, false
}

func matchesDomainSuffix(domain, suffix string) bool {
	domain = strings.ToLower(domain)
	suffix = strings.ToLower(suffix)

	if domain == suffix {
		return true
	}

	if strings.HasSuffix(domain, "."+suffix) {
		return true
	}

	return false
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"fc00::/7",
		"fe80::/10",
		"::1/128",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}
