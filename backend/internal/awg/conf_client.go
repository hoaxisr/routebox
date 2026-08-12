package awg

import (
	"fmt"
	"strings"

	"routebox/backend/internal/awg/cps"
)

// Obfuscation is the AWG obfuscation preset. Numeric J/S fields; string H
// fields (ranges like "lo-hi"); plus the AWG 3.0 cookie-protection fields
// CPA/RAT. I-fields are intentionally NOT modelled in v1 (the live box uses
// none; they are not in the validated set).
type Obfuscation struct {
	Jc, Jmin, Jmax int
	S1, S2, S3, S4 int
	H1, H2, H3, H4 string
	// AWG 3.0 cookie-protection fields: UintRange strings ("N" or "lo-hi").
	CPA, RAT string
	// AWG3 device-timers: UintRange strings ("N" or "lo-hi"), seconds/count.
	RekeyTimeout, RejectAfterTime, KeepaliveTimeout, MaxHandshakeAttempts string
}

// stripAwg3 zeroes the six sing-box-only AWG3 obfuscation fields (CPA/RAT plus the
// four device-timers). The kernel awg-quick path must never emit these into a
// server/client conf (a pre-awg3 awg-quick hard-fails on unknown [Interface] keys),
// and a difference in them must not flag ConfigDirty on the kernel backend.
func (o *Obfuscation) stripAwg3() {
	o.CPA, o.RAT = "", ""
	o.RekeyTimeout, o.RejectAfterTime, o.KeepaliveTimeout, o.MaxHandshakeAttempts = "", "", "", ""
}

// collapseRange turns an AWG 3.0 "lo-hi" value into the plain number a pre-3.0
// client can parse, keeping the low end (the more conservative timer). Anything
// that is not a range comes back untouched.
func collapseRange(s string) string {
	if lo, _, isRange := strings.Cut(s, "-"); isRange {
		return lo
	}
	return s
}

// ClientConf is the fully-assembled input to BuildClient. Pure — no FS/settings
// reads inside the builder; the handler assembles this.
type ClientConf struct {
	PrivateKey string
	Address    string
	Address6   string // "" = v4-only
	DNS        []string
	MTU        int
	Obf        Obfuscation
	// HeaderProtectionKey is the AWG3 shared secret, must equal the server's; omitted when empty.
	HeaderProtectionKey string
	Mimic               cps.Set
	ServerPub           string
	PSK                 string
	Endpoint            string // host:port, IPv6 already bracketed
	AllowedIPs          []string
	// Keepalive is the PersistentKeepalive value verbatim: "25" or, on AWG 3.0, a
	// "lo-hi" range (the device draws a fresh interval every time it arms the
	// timer). "" and "0" omit the line.
	Keepalive string
}

// BuildClient renders the extended-WireGuard client config an AmneziaWG client
// imports (the QR encodes this text verbatim). Obfuscation keys are Capitalized;
// zero/absent fields are omitted.
func BuildClient(c ClientConf) (string, error) {
	if c.PrivateKey == "" || c.Address == "" || c.ServerPub == "" || c.Endpoint == "" {
		return "", fmt.Errorf("incomplete client conf")
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", c.PrivateKey)
	if c.Address6 != "" {
		fmt.Fprintf(&b, "Address = %s, %s\n", c.Address, c.Address6)
	} else {
		fmt.Fprintf(&b, "Address = %s\n", c.Address)
	}
	if len(c.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(c.DNS, ", "))
	}
	if c.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", c.MTU)
	}
	writeObf(&b, c.Obf)
	writeMimic(&b, c.Mimic)
	if c.HeaderProtectionKey != "" {
		fmt.Fprintf(&b, "HeaderProtectionKey = %s\n", c.HeaderProtectionKey)
	}
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", c.ServerPub)
	if c.PSK != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", c.PSK)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", c.Endpoint)
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(c.AllowedIPs, ", "))
	if c.Keepalive != "" && c.Keepalive != "0" {
		fmt.Fprintf(&b, "PersistentKeepalive = %s\n", c.Keepalive)
	}
	return b.String(), nil
}

// writeObf emits Capitalized obfuscation lines, omitting zero/empty values.
func writeObf(b *strings.Builder, o Obfuscation) {
	num := []struct {
		k string
		v int
	}{{"Jc", o.Jc}, {"Jmin", o.Jmin}, {"Jmax", o.Jmax}, {"S1", o.S1}, {"S2", o.S2}, {"S3", o.S3}, {"S4", o.S4}}
	for _, f := range num {
		if f.v != 0 {
			fmt.Fprintf(b, "%s = %d\n", f.k, f.v)
		}
	}
	str := []struct{ k, v string }{{"H1", o.H1}, {"H2", o.H2}, {"H3", o.H3}, {"H4", o.H4}, {"ContentPaddingAddition", o.CPA}, {"RekeyAfterTime", o.RAT}, {"RekeyTimeout", o.RekeyTimeout}, {"RejectAfterTime", o.RejectAfterTime}, {"KeepaliveTimeout", o.KeepaliveTimeout}, {"MaxHandshakeAttempts", o.MaxHandshakeAttempts}}
	for _, f := range str {
		if f.v != "" {
			fmt.Fprintf(b, "%s = %s\n", f.k, f.v)
		}
	}
}

// writeMimic emits the client-only CPS fields (I1..I5). Empty Set -> nothing.
func writeMimic(b *strings.Builder, m cps.Set) {
	for i, v := range []string{m.I1, m.I2, m.I3, m.I4, m.I5} {
		if v != "" {
			fmt.Fprintf(b, "I%d = %s\n", i+1, v)
		}
	}
}
