package config

import (
	"errors"
	"fmt"
	"reflect"
)

// ManagedMtprotoSocksTag is the reserved inbound tag owned by the Telegram
// proxy's outbound routing. User CRUD may not create, edit, or delete it — only
// SyncMtprotoSocksActive mutates it.
//
// It exists because sing-box has no way to hand a specific outbound to a process
// outside itself. Giving the MTProto proxy a loopback SOCKS inbound and pinning
// that inbound to one outbound with a route rule is how "send Telegram through
// this exit" is expressed.
const ManagedMtprotoSocksTag = "mtproto-socks"

// ErrReservedMtprotoTag is returned by inbound CRUD when the caller targets the
// Telegram-proxy-managed tag.
var ErrReservedMtprotoTag = errors.New("inbound tag 'mtproto-socks' is managed by the Telegram proxy")

// BuildMtprotoSocksInbound renders the loopback SOCKS inbound the MTProto proxy
// dials. PURE.
//
// listen is 127.0.0.1 and not configurable: this is an in-process side channel,
// and an open SOCKS proxy is the kind of thing that gets a server enrolled in
// somebody else's spam run. Nothing outside the host has any business reaching
// it.
func BuildMtprotoSocksInbound(port int) map[string]interface{} {
	return map[string]interface{}{
		"type":        "socks",
		"tag":         ManagedMtprotoSocksTag,
		"listen":      "127.0.0.1",
		"listen_port": port,
	}
}

// buildMtprotoSocksRule renders the route rule pinning the managed inbound to
// one outbound (or endpoint — sing-box routes to either by tag). Returns nil for
// an empty outbound, meaning "no rule", which is how direct is expressed. PURE.
//
// The `action` is left implicit: sing-box defaults it to "route", and the two
// keys are what makes managedMtprotoSocksRule able to recognise this rule again.
func buildMtprotoSocksRule(outbound string) map[string]interface{} {
	if outbound == "" {
		return nil
	}

	return map[string]interface{}{
		"inbound":  []interface{}{ManagedMtprotoSocksTag},
		"outbound": outbound,
	}
}

// managedMtprotoSocksRule reports whether a route rule is the one this file
// owns: EXACTLY an inbound list naming only the managed tag plus a non-empty
// outbound. As with the managed reject rule, a structural marker is the only
// reliable identity — any extra key means an operator wrote it and RouteBox must
// leave it alone.
func managedMtprotoSocksRule(rule map[string]interface{}) bool {
	if len(rule) != 2 {
		return false
	}

	if outbound, _ := rule["outbound"].(string); outbound == "" {
		return false
	}

	inbounds, ok := rule["inbound"].([]interface{})
	if !ok || len(inbounds) != 1 {
		return false
	}

	tag, _ := inbounds[0].(string)

	return tag == ManagedMtprotoSocksTag
}

// SyncMtprotoSocksActive reconciles the RouteBox-managed SOCKS inbound and its
// route rule in the ACTIVE config. An empty outbound removes both, which is what
// "Telegram connects directly" means — no listener, no rule, no trace.
//
// It is the twin of SyncAwgEndpointActive, and loud for the same reason: its
// caller is an interactive settings save whose success message would otherwise
// promise routing that was never written. It returns (false, nil) — a deferral —
// when the manager is unconfigured or a draft is pending (never write active
// mid-edit), and (false, ErrReadOnly) when the config cannot be written at all.
//
// Change-gated, so re-saving the Telegram page with nothing altered does not
// reload sing-box and drop every live VPN connection.
//
// RouteBox owns ONLY this inbound tag and this rule shape; sibling inbounds and
// user-authored rules are never touched.
func (m *Manager) SyncMtprotoSocksActive(port int, outbound string) (changed bool, err error) {
	if outbound != "" && (port <= 0 || port > 65535) {
		return false, fmt.Errorf("invalid socks port %d for the Telegram proxy", port)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.blockedByReadOnly(); err != nil {
		return false, err
	}

	if m.path == "" || m.hasDraft {
		return false, nil
	}

	cfg := m.deepCopy(m.activeConfig)

	// --- inbounds ---

	oldInbounds, _ := cfg["inbounds"].([]interface{})
	newInbounds := make([]interface{}, 0, len(oldInbounds)+1)

	var currentInbound map[string]interface{}

	for _, ib := range oldInbounds {
		if obj, ok := ib.(map[string]interface{}); ok {
			if tag, _ := obj["tag"].(string); tag == ManagedMtprotoSocksTag {
				currentInbound = obj

				continue
			}
		}

		newInbounds = append(newInbounds, ib)
	}

	var wantInbound map[string]interface{}

	if outbound != "" {
		// Normalised through deepCopy for the same reason the AWG endpoint is:
		// the builder emits Go ints while anything read back off disk is
		// float64, and the change-gate below compares the two.
		wantInbound = m.deepCopy(map[string]interface{}{
			"i": BuildMtprotoSocksInbound(port),
		})["i"].(map[string]interface{})

		// A colliding port takes amnezia-box down on the next reload, which
		// would strand the operator with a dead VPN over a Telegram setting.
		if err := listenPortConflict(newInbounds, wantInbound, ManagedMtprotoSocksTag); err != nil {
			return false, fmt.Errorf("the Telegram proxy cannot use port %d: %w", port, err)
		}

		newInbounds = append(newInbounds, wantInbound)
	}

	// --- route rules ---

	route, _ := cfg["route"].(map[string]interface{})

	var oldRules []interface{}
	if route != nil {
		oldRules, _ = route["rules"].([]interface{})
	}

	newRules := make([]interface{}, 0, len(oldRules)+1)

	for _, r := range oldRules {
		if obj, ok := r.(map[string]interface{}); ok && managedMtprotoSocksRule(obj) {
			continue
		}

		newRules = append(newRules, r)
	}

	// Prepended, so an operator's catch-all rule further down cannot swallow the
	// proxy's traffic and send it out the wrong exit.
	if want := buildMtprotoSocksRule(outbound); want != nil {
		newRules = append([]interface{}{want}, newRules...)
	}

	// Normalise empty -> nil on both sides so "remove when already absent" stays
	// a true no-op regardless of whether the array was missing or present-empty.
	if len(oldRules) == 0 {
		oldRules = nil
	}

	if len(newRules) == 0 {
		newRules = nil
	}

	if reflect.DeepEqual(currentInbound, wantInbound) && reflect.DeepEqual(oldRules, newRules) {
		return false, nil
	}

	if len(newInbounds) == 0 {
		delete(cfg, "inbounds")
	} else {
		cfg["inbounds"] = newInbounds
	}

	if len(newRules) == 0 {
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
		return true, err
	}

	return true, nil
}

// RoutableTag is one candidate exit for the Telegram proxy: an outbound or an
// endpoint, which sing-box routes to interchangeably by tag.
type RoutableTag struct {
	Tag  string `json:"tag"`
	Type string `json:"type"`
	// Kind is "outbound" or "endpoint", so the panel can group them the way the
	// rest of the config UI does.
	Kind string `json:"kind"`
}

// ListRoutableTags returns everything a route rule may name as its outbound, for
// the Telegram page's exit picker.
//
// The managed AWG server endpoint is excluded: it is a listener for inbound
// peers, not a way out, and routing Telegram into it would black-hole the proxy.
func (m *Manager) ListRoutableTags() []RoutableTag {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]RoutableTag, 0)

	for _, kind := range []string{"outbounds", "endpoints"} {
		for _, item := range m.getArray(kind) {
			obj, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			tag, _ := obj["tag"].(string)
			if tag == "" || tag == ManagedAwgServerTag {
				continue
			}

			typ, _ := obj["type"].(string)

			out = append(out, RoutableTag{
				Tag:  tag,
				Type: typ,
				Kind: kind[:len(kind)-1], // "outbounds" -> "outbound"
			})
		}
	}

	return out
}
