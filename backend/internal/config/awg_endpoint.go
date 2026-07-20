package config

// AwgServerPeer is one roaming (inbound) peer of the managed AWG server endpoint:
// public identity only, no address/port (the server waits for the client to dial).
type AwgServerPeer struct {
	PublicKey    string
	PresharedKey string
	AllowedIP    string // "<ip>/32"
}

// AwgServerSpec is the fully-resolved input for the managed awg-server endpoint.
// Obf holds the already-validated jc/jmin/jmax/s1-s4/h1-h4 values (device-level).
type AwgServerSpec struct {
	PrivateKey string
	Address    string
	ListenPort int
	MTU        int
	Obf        map[string]interface{}
	Peers      []AwgServerPeer
}

// obfKeyOrder fixes the emit order so the rendered map is deterministic (a stable
// block => the change-gate in SyncAwgEndpointActive is a true no-op when unchanged).
var obfKeyOrder = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4"}

// BuildAwgServerEndpoint renders the sing-box endpoints[] element for the fork's
// AWG server: type "awg", listen_port set, peers without address/port. Zero/empty
// obf values are omitted. PURE. The fork peer field is preshared_key (NOT
// pre_shared_key) and the endpoint flag is camelCase useIntegratedTun.
func BuildAwgServerEndpoint(tag string, spec AwgServerSpec) map[string]interface{} {
	ep := map[string]interface{}{
		"type":             "awg",
		"tag":              tag,
		"useIntegratedTun": false,
		"private_key":      spec.PrivateKey,
		"address":          []interface{}{spec.Address},
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
	if len(spec.Peers) > 0 {
		peers := make([]interface{}, 0, len(spec.Peers))
		for _, p := range spec.Peers {
			peers = append(peers, map[string]interface{}{
				"public_key":    p.PublicKey,
				"preshared_key": p.PresharedKey,
				"allowed_ips":   []interface{}{p.AllowedIP},
			})
		}
		ep["peers"] = peers
	}
	return ep
}
