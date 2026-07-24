package awg

import "routebox/backend/internal/awg/cps"

// ClientEndpointSpec is the resolved input for a client sing-box endpoint export.
// Obf carries the SERVER's s/h/j values (symmetric where required); Mimic carries
// the generated I1-I5 (the server has none — they are client sender-side CPS).
type ClientEndpointSpec struct {
	Tag        string
	PrivateKey string
	Address    string
	MTU        int
	Obf        Obfuscation
	Mimic      cps.Set
	// HeaderProtectionKey is the AWG 3.0 shared secret; must equal the server's.
	HeaderProtectionKey string
	ServerPub           string
	PSK                 string
	Host                string
	Port                int
}

// BuildClientEndpoint renders a sing-box endpoints[] element another RouteBox can
// paste in: type "awg", client-side (peer carries address+port), full-tunnel
// allowed_ips, keepalive 25. Zero/empty obf and mimic values are omitted. The peer
// field is preshared_key (fork naming); i-fields are the generated CPS mimicry.
func BuildClientEndpoint(s ClientEndpointSpec) map[string]interface{} {
	ep := map[string]interface{}{
		"type":             "awg",
		"tag":              s.Tag,
		"useIntegratedTun": false,
		"private_key":      s.PrivateKey,
		"address":          []interface{}{s.Address},
	}
	if s.MTU > 0 {
		ep["mtu"] = s.MTU
	}
	putObfInt := func(k string, v int) {
		if v != 0 {
			ep[k] = v
		}
	}
	putObfStr := func(k, v string) {
		if v != "" {
			ep[k] = v
		}
	}
	putObfInt("jc", s.Obf.Jc)
	putObfInt("jmin", s.Obf.Jmin)
	putObfInt("jmax", s.Obf.Jmax)
	putObfInt("s1", s.Obf.S1)
	putObfInt("s2", s.Obf.S2)
	putObfInt("s3", s.Obf.S3)
	putObfInt("s4", s.Obf.S4)
	putObfStr("h1", s.Obf.H1)
	putObfStr("h2", s.Obf.H2)
	putObfStr("h3", s.Obf.H3)
	putObfStr("h4", s.Obf.H4)
	putObfStr("content_padding_addition", s.Obf.CPA)
	putObfStr("rekey_after_time", s.Obf.RAT)
	putObfStr("rekey_timeout", s.Obf.RekeyTimeout)
	putObfStr("reject_after_time", s.Obf.RejectAfterTime)
	putObfStr("keepalive_timeout", s.Obf.KeepaliveTimeout)
	putObfStr("max_handshake_attempts", s.Obf.MaxHandshakeAttempts)
	putObfStr("header_protection_key", s.HeaderProtectionKey)
	putObfStr("i1", s.Mimic.I1)
	putObfStr("i2", s.Mimic.I2)
	putObfStr("i3", s.Mimic.I3)
	putObfStr("i4", s.Mimic.I4)
	putObfStr("i5", s.Mimic.I5)

	peer := map[string]interface{}{
		"address":                       s.Host,
		"port":                          s.Port,
		"public_key":                    s.ServerPub,
		"allowed_ips":                   []interface{}{"0.0.0.0/0"},
		"persistent_keepalive_interval": 25,
	}
	if s.PSK != "" {
		peer["preshared_key"] = s.PSK
	}
	ep["peers"] = []interface{}{peer}
	return ep
}
