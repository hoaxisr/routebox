package config

import "reflect"

// AwgServerPeer is one roaming (inbound) peer of the managed AWG server endpoint:
// public identity only, no address/port (the server waits for the client to dial).
type AwgServerPeer struct {
	PublicKey    string
	PresharedKey string
	AllowedIP    string // "<ip>/32"
	AllowedIP6   string // "<v6>/128", "" when broker off
}

// AwgServerSpec is the fully-resolved input for the managed awg-server endpoint.
// Obf holds the already-validated jc/jmin/jmax/s1-s4/h1-h4 plus the awg3
// cpa/rat values (device-level).
type AwgServerSpec struct {
	PrivateKey          string
	Address             string
	Address6            string // server v6 CIDR, "" when broker off
	ListenPort          int
	MTU                 int
	HeaderProtectionKey string
	Obf                 map[string]interface{}
	Peers               []AwgServerPeer
}

// obfKeyOrder fixes the emit order so the rendered map is deterministic (a stable
// block => the change-gate in SyncAwgEndpointActive is a true no-op when unchanged).
var obfKeyOrder = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "content_padding_addition", "rekey_after_time", "rekey_timeout", "reject_after_time", "keepalive_timeout", "max_handshake_attempts"}

// BuildAwgServerEndpoint renders the sing-box endpoints[] element for the fork's
// AWG server: type "awg", listen_port set, peers without address/port. Zero/empty
// obf values are omitted. PURE. The fork peer field is preshared_key (NOT
// pre_shared_key) and the endpoint flag is camelCase useIntegratedTun.
func BuildAwgServerEndpoint(tag string, spec AwgServerSpec) map[string]interface{} {
	addr := []interface{}{spec.Address}
	if spec.Address6 != "" {
		addr = append(addr, spec.Address6)
	}
	ep := map[string]interface{}{
		"type":             "awg",
		"tag":              tag,
		"useIntegratedTun": false,
		"private_key":      spec.PrivateKey,
		"address":          addr,
		"listen_port":      spec.ListenPort,
		"mtu":              spec.MTU,
	}
	for _, k := range obfKeyOrder {
		v, ok := spec.Obf[k]
		if !ok {
			continue
		}
		switch vv := v.(type) {
		case int:
			if vv != 0 {
				ep[k] = vv
			}
		case string:
			if vv != "" {
				ep[k] = vv
			}
		}
	}
	if spec.HeaderProtectionKey != "" {
		ep["header_protection_key"] = spec.HeaderProtectionKey
	}
	if len(spec.Peers) > 0 {
		peers := make([]interface{}, 0, len(spec.Peers))
		for _, p := range spec.Peers {
			aip := []interface{}{p.AllowedIP}
			if p.AllowedIP6 != "" {
				aip = append(aip, p.AllowedIP6)
			}
			peers = append(peers, map[string]interface{}{
				"public_key":    p.PublicKey,
				"preshared_key": p.PresharedKey,
				"allowed_ips":   aip,
			})
		}
		ep["peers"] = peers
	}
	return ep
}

// SyncAwgEndpointActive reconciles the RouteBox-managed AWG server endpoint
// (identified by tag) in the ACTIVE config. spec==nil removes it. It is the twin
// of SyncV2RayAPI: deep-COPY active, mutate the copy, saveLocked (assigns
// m.activeConfig ONLY on success). Returns (false,nil) — a deferral — when the
// manager is unconfigured or a draft is pending (never write active mid-edit;
// the pending Apply re-renders), and (false, ErrReadOnly) when the config cannot
// be written at all: unlike its background siblings, this sync serves
// interactive operations that would otherwise report success (see the comment on
// the guard below). Change-gated so an unchanged spec is a true no-op (no reload
// => no dropped tunnels). RouteBox OWNS only this tag; sibling endpoints[]
// entries are never touched.
func (m *Manager) SyncAwgEndpointActive(tag string, spec *AwgServerSpec) (changed bool, err error) {
	var want map[string]interface{}
	if spec != nil {
		want = BuildAwgServerEndpoint(tag, *spec)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Deliberately louder than its siblings SyncRejectRuleActive / SyncV2RayAPI:
	// those are background reconciliations where a silent no-op is right, while
	// this one is called by interactive operations (Enable AWG, add peer) whose
	// caller reads changed==false as "nothing to do" and reports success. On a
	// read-only config that would store the peer's secrets and show a client
	// that never reaches sing-box.
	if err := m.blockedByReadOnly(); err != nil {
		return false, err
	}
	if m.path == "" {
		return false, nil
	}
	if m.hasDraft {
		return false, nil
	}

	cfg := m.deepCopy(m.activeConfig)
	old, _ := cfg["endpoints"].([]interface{})

	// Rebuild endpoints without the managed tag.
	newEps := make([]interface{}, 0, len(old)+1)
	var current map[string]interface{}
	for _, e := range old {
		em, ok := e.(map[string]interface{})
		if ok {
			if t, _ := em["tag"].(string); t == tag {
				current = em
				continue
			}
		}
		newEps = append(newEps, e)
	}
	// Normalise want through deepCopy (JSON round-trip): `want` is built with Go
	// ints while `current` was decoded as float64, so the change-gate compare and
	// the stored active config must both use the JSON-normalised form.
	var wantNorm map[string]interface{}
	if want != nil {
		wantNorm = m.deepCopy(map[string]interface{}{"e": want})["e"].(map[string]interface{})
		newEps = append(newEps, wantNorm)
	}
	if reflect.DeepEqual(current, wantNorm) {
		return false, nil
	}

	if len(newEps) == 0 {
		delete(cfg, "endpoints")
	} else {
		cfg["endpoints"] = newEps
	}

	if err := m.saveLocked(cfg); err != nil {
		return true, err
	}
	return true, nil
}
