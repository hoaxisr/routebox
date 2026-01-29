package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ConnectTestRequest is the request body for connection testing
type ConnectTestRequest struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`    // default 443
	Timeout int    `json:"timeout"` // seconds, default 5
}

// ConnectTestResponse is the response for connection testing
type ConnectTestResponse struct {
	Success    bool   `json:"success"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	ResolvedIP string `json:"resolved_ip,omitempty"`
	Error      string `json:"error,omitempty"`

	// GeoIP info (if available)
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`

	// Route match info (reuse inspector logic)
	MatchedRule *MatchedRuleInfo `json:"matched_rule,omitempty"`
}

// MatchedRuleInfo contains info about the matched routing rule
type MatchedRuleInfo struct {
	Index    int    `json:"index"`
	Action   string `json:"action"`
	Outbound string `json:"outbound,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ConnectTest performs a TCP connection test to a host
func (h *Handler) ConnectTest(w http.ResponseWriter, r *http.Request) {
	var req ConnectTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required")
		return
	}

	// Defaults
	if req.Port == 0 {
		req.Port = 443
	}
	if req.Timeout == 0 {
		req.Timeout = 5
	}

	resp := ConnectTestResponse{}

	// 1. DNS Resolve
	start := time.Now()
	ips, err := net.LookupHost(req.Host)
	if err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("DNS resolution failed: %v", err)
		resp.LatencyMs = time.Since(start).Milliseconds()

		// Try to find matched rule for the domain anyway
		resp.MatchedRule = h.findMatchedRule(req.Host, "domain")

		writeSuccess(w, resp)
		return
	}

	ip := ips[0]
	resp.ResolvedIP = ip

	// 2. TCP Connect
	addr := fmt.Sprintf("%s:%d", ip, req.Port)
	conn, err := net.DialTimeout("tcp", addr, time.Duration(req.Timeout)*time.Second)
	resp.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("Connection failed: %v", err)
	} else {
		resp.Success = true
		conn.Close()
	}

	// 3. GeoIP lookup
	if h.geoip != nil {
		if geoInfo := h.geoip.Lookup(ip); geoInfo != nil {
			resp.Country = geoInfo.Country
			resp.CountryCode = geoInfo.CountryCode
		}
	}

	// 4. Find matched routing rule
	// For domains, check domain rules; for IPs, check IP rules
	inputType := "domain"
	testInput := req.Host
	if net.ParseIP(req.Host) != nil {
		inputType = "ip"
	}
	resp.MatchedRule = h.findMatchedRule(testInput, inputType)

	writeSuccess(w, resp)
}

// findMatchedRule finds the first matching routing rule for the given input
func (h *Handler) findMatchedRule(input, inputType string) *MatchedRuleInfo {
	rules := h.config.ListRules()
	ruleSets := h.config.ListRuleSets()
	routeSettings := h.config.GetRouteSettings()

	// Build rule set map for quick lookup
	ruleSetMap := make(map[string]map[string]interface{})
	for _, rs := range ruleSets {
		if tag, ok := rs["tag"].(string); ok {
			ruleSetMap[tag] = rs
		}
	}

	for i, rule := range rules {
		match := checkRuleMatch(input, inputType, rule, ruleSetMap)
		if match.Matched {
			action := "route"
			if a, ok := rule["action"].(string); ok {
				action = a
			}

			// Skip non-terminal actions
			if action == "sniff" || action == "resolve" || action == "route-options" {
				continue
			}

			info := &MatchedRuleInfo{
				Index:  i,
				Action: action,
				Reason: match.Reason,
			}

			if ob, ok := rule["outbound"].(string); ok {
				info.Outbound = ob
			} else if action == "reject" {
				info.Outbound = "REJECT"
			}

			return info
		}
	}

	// No rule matched - use final outbound
	finalOutbound := "direct"
	if final, ok := routeSettings["final"].(string); ok && final != "" {
		finalOutbound = final
	}

	return &MatchedRuleInfo{
		Index:    -1,
		Action:   "route",
		Outbound: finalOutbound,
		Reason:   "no rule matched, using final outbound",
	}
}
