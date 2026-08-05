package config

import (
	"fmt"
	"strings"
)

// ListInbounds returns all inbounds from the working config (draft or active)
func (m *Manager) ListInbounds() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("inbounds")
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, obj)
		}
	}
	return result
}

// GetInbound returns inbound by tag from the working config (draft or active)
func (m *Manager) GetInbound(tag string) (map[string]interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return nil, false
	}
	if obj, ok := arr[idx].(map[string]interface{}); ok {
		return obj, true
	}
	return nil, false
}

// normalizeListenAddr collapses every wildcard spelling — "", "::", "[::]", "::0",
// "0.0.0.0", "*" — plus an ABSENT listen into a single "*" bucket. Specific IPs keep
// their own identity. Used for port-conflict detection so "::" (set by the panel) and
// an absent listen (on older/hand-made inbounds) are recognised as colliding.
//
// Folding absent in is a deliberate over-approximation for THIS purpose, not a claim
// about where it binds: sing-box builds the bind address with a 127.0.0.1 fallback, so
// an absent listen is loopback-only. Two inbounds on the same port still cannot both
// be trusted to come up, so conflict detection stays conservative — but nothing else
// should read this function as "absent means wildcard" (the panel's inbound list
// deliberately renders it as loopback).
func normalizeListenAddr(s string) string {
	switch strings.TrimSpace(s) {
	case "", "::", "[::]", "::0", "0.0.0.0", "*":
		return "*"
	default:
		return strings.TrimSpace(s)
	}
}

// inboundNetworks reports which L4 protocols an inbound actually binds, so two
// inbounds can share a port when they cannot possibly contend for the same
// socket. 443 is the port that survives hostile networks, and operators want a
// QUIC inbound there next to the TCP one — which a port-only check forbade.
//
// The table follows what the fork's inbounds register with the shared listener
// (protocol/*/inbound.go: `Network: []string{...}`), plus the QUIC inbounds,
// which bind UDP themselves. mieru is the odd one: it declares both but never
// starts that listener — its server binds one protocol, picked by `transport`.
//
// Anything not listed counts as binding BOTH. A wrong "no conflict" here is two
// inbounds fighting for a socket and amnezia-box failing to reload, so an
// unknown type must stay conservative.
func inboundNetworks(inbound map[string]interface{}) map[string]bool {
	both := map[string]bool{"tcp": true, "udp": true}
	typ, _ := inbound["type"].(string)
	switch typ {
	case "vless", "trojan", "vmess", "http", "socks", "mixed", "anytls", "shadowtls", "snell", "naive":
		return map[string]bool{"tcp": true}
	case "hysteria2", "tuic":
		return map[string]bool{"udp": true}
	case "mieru":
		switch tr, _ := inbound["transport"].(string); tr {
		case "TCP":
			return map[string]bool{"tcp": true}
		case "UDP":
			return map[string]bool{"udp": true}
		}
		return both // no/!invalid transport: the inbound is invalid anyway, stay strict
	}
	return both
}

// networksOverlap reports whether two inbounds could contend for the same socket.
func networksOverlap(a, b map[string]bool) bool {
	for net := range a {
		if b[net] {
			return true
		}
	}
	return false
}

// listenPortConflict reports an error if inbound's (listen, listen_port) collides with
// another inbound in arr, skipping the entry whose tag == selfTag (the one being
// updated). This mirrors the cross-inbound check in the full-config validator so the
// per-inbound CRUD path — which only runs the single-inbound validator — can't stage
// two inbounds on the same address:port, which crashes amnezia-box on reload. Inbounds
// without a listen_port (tun/awg) are ignored; wildcard listen spellings are normalized.
func listenPortConflict(arr []interface{}, inbound map[string]interface{}, selfTag string) error {
	port, ok := inbound["listen_port"].(float64)
	if !ok || port == 0 {
		return nil
	}
	listen, _ := inbound["listen"].(string)
	listen = normalizeListenAddr(listen)
	nets := inboundNetworks(inbound)
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := obj["tag"].(string); t == selfTag {
			continue
		}
		p, ok := obj["listen_port"].(float64)
		if !ok || p != port {
			continue
		}
		l, _ := obj["listen"].(string)
		if normalizeListenAddr(l) != listen {
			continue
		}
		if !networksOverlap(nets, inboundNetworks(obj)) {
			continue // e.g. a QUIC inbound next to a TCP one on 443
		}
		other, _ := obj["tag"].(string)
		return fmt.Errorf("listen %s:%d is already used by inbound %q (ports must be unique across inbounds using the same protocol)", listen, int(port), other)
	}
	return nil
}

// CreateInbound adds new inbound to draft with validation
func (m *Manager) CreateInbound(inbound map[string]interface{}) error {
	if tag, _ := inbound["tag"].(string); tag == ManagedMtprotoSocksTag {
		return ErrReservedMtprotoTag
	}

	// Validate inbound before adding
	errs := validateInbound(inbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tag := inbound["tag"].(string)

	arr := m.getArray("inbounds")
	if findByTag(arr, tag) >= 0 {
		return fmt.Errorf("inbound with tag '%s' already exists", tag)
	}
	if err := listenPortConflict(arr, inbound, ""); err != nil {
		return err
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	m.setDraftValue("inbounds", append(draftArr, inbound))

	return m.saveDraftToDisk()
}

// UpdateInbound updates existing inbound in draft with validation
func (m *Manager) UpdateInbound(tag string, inbound map[string]interface{}) error {
	if tag == ManagedMtprotoSocksTag {
		return ErrReservedMtprotoTag
	}

	if newTag, _ := inbound["tag"].(string); newTag == ManagedMtprotoSocksTag {
		return ErrReservedMtprotoTag
	}

	// Validate inbound before updating
	errs := validateInbound(inbound, 0)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs[0])
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound '%s' not found", tag)
	}
	if err := listenPortConflict(arr, inbound, tag); err != nil {
		return err
	}

	newTag, _ := inbound["tag"].(string)
	if newTag == "" {
		inbound["tag"] = tag
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		draftArr[draftIdx] = inbound
		m.setDraftValue("inbounds", draftArr)
	}

	return m.saveDraftToDisk()
}

// MutateInbound stages a change to the named inbound atomically under the write
// lock: it ensures a draft exists, applies fn to a deep copy of the inbound (so
// the live active config is never mutated), writes the result into the draft,
// and persists. fn receives a private deep copy it may mutate freely; returning
// an error aborts and persists no further change (the draft is left as-is,
// matching UpdateInbound's behavior on its own error paths).
//
// This is the single-critical-section equivalent of GetInbound+UpdateInbound:
// two concurrent MutateInbound calls on the same inbound serialize, so neither
// can lose the other's update (no get-clone-update race across two locks).
func (m *Manager) MutateInbound(tag string, fn func(inbound map[string]interface{}) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure draft exists before modifying (snapshots active→draft once).
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	draftArr := m.getDraftArray("inbounds")
	idx := findByTag(draftArr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound %q: %w", tag, ErrInboundNotFound)
	}
	orig, _ := draftArr[idx].(map[string]interface{})

	// Operate on a deep copy so fn cannot mutate the (possibly active-aliased)
	// original in place; only the staged clone is written back to the draft.
	clone := m.deepCopy(orig)
	if err := fn(clone); err != nil {
		return err // abort: do not persist the change
	}

	draftArr[idx] = clone
	m.setDraftValue("inbounds", draftArr)

	return m.saveDraftToDisk()
}

// DeleteInbound removes inbound by tag from draft
func (m *Manager) DeleteInbound(tag string) error {
	if tag == ManagedMtprotoSocksTag {
		// Deleting it by hand would leave the Telegram proxy dialing a listener
		// that is not there. Switching the proxy back to a direct exit removes
		// it — that is the supported way out.
		return ErrReservedMtprotoTag
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	arr := m.getArray("inbounds")
	idx := findByTag(arr, tag)
	if idx < 0 {
		return fmt.Errorf("inbound '%s' not found", tag)
	}

	// Ensure draft exists before modifying
	if err := m.ensureDraftUnlocked(); err != nil {
		return err
	}

	// Modify draft
	draftArr := m.getDraftArray("inbounds")
	draftIdx := findByTag(draftArr, tag)
	if draftIdx >= 0 {
		m.setDraftValue("inbounds", append(draftArr[:draftIdx], draftArr[draftIdx+1:]...))
	}

	return m.saveDraftToDisk()
}
