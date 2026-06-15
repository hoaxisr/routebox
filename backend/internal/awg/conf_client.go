package awg

import (
	"fmt"
	"strings"
)

// Obfuscation is the AWG 2.0 obfuscation preset. Numeric J/S fields; string H
// fields (ranges like "lo-hi"). I-fields are intentionally NOT modelled in v1
// (the live box uses none; they are not in the validated set).
type Obfuscation struct {
	Jc, Jmin, Jmax int
	S1, S2, S3, S4 int
	H1, H2, H3, H4 string
}

// ClientConf is the fully-assembled input to BuildClient. Pure — no FS/settings
// reads inside the builder; the handler assembles this.
type ClientConf struct {
	PrivateKey string
	Address    string
	DNS        []string
	MTU        int
	Obf        Obfuscation
	ServerPub  string
	PSK        string
	Endpoint   string // host:port, IPv6 already bracketed
	AllowedIPs []string
	Keepalive  int
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
	fmt.Fprintf(&b, "Address = %s\n", c.Address)
	if len(c.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(c.DNS, ", "))
	}
	if c.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", c.MTU)
	}
	writeObf(&b, c.Obf)
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", c.ServerPub)
	if c.PSK != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", c.PSK)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", c.Endpoint)
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(c.AllowedIPs, ", "))
	if c.Keepalive > 0 {
		fmt.Fprintf(&b, "PersistentKeepalive = %d\n", c.Keepalive)
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
	str := []struct{ k, v string }{{"H1", o.H1}, {"H2", o.H2}, {"H3", o.H3}, {"H4", o.H4}}
	for _, f := range str {
		if f.v != "" {
			fmt.Fprintf(b, "%s = %s\n", f.k, f.v)
		}
	}
}
