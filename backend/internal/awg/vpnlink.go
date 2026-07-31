// Builds the Amnezia "vpn://" link for a peer. The wire format is read out of
// amnezia-client (HEAD 30ec46a); see the design spec for citations.
package awg

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// qCompress mirrors Qt's QByteArray qCompress: a 4-byte big-endian uncompressed
// length followed by a zlib stream. The Amnezia client's qUncompress ignores the
// length prefix but hard-requires the zlib framing — raw deflate or gzip make it
// return empty, after which the import fails as an invalid config. Level 8 is what
// their own exporter uses (exportController.cpp:44).
func qCompress(b []byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(len(b)))
	zw, err := zlib.NewWriterLevel(&out, 8)
	if err != nil { // only possible for an invalid level; 8 is valid
		panic("awg: zlib level 8 rejected: " + err.Error())
	}
	_, _ = zw.Write(b)
	_ = zw.Close()
	return out.Bytes()
}

// Amnezia container and protocol identifiers. amnezia-awg2 also exists on their
// side but is the install wizard's container; their own third-party importer emits
// amnezia-awg (importController.cpp:638) and at connect time the two are
// indistinguishable. Do not "fix" this to awg2.
const (
	amneziaContainerAwg = "amnezia-awg"
	amneziaContainerWG  = "amnezia-wireguard"
	amneziaProtoAwg     = "awg"
	amneziaProtoWG      = "wireguard"
	// The client defines no version 3 (protocolConstants.h:201). awg3 fields are
	// forwarded to the daemon on the container name alone, so "2" is correct.
	amneziaProtocolVersion = "2"
)

// ErrLinkUnrepresentable is returned when a peer's parameters cannot be expressed
// as an Amnezia link at all. Callers should surface the message: it names what the
// operator has to fix, and by construction contains no secrets — only static text
// and the literals "H1".."H4".
var ErrLinkUnrepresentable = errors.New("peer cannot be expressed as an Amnezia link")

// obfState classifies a peer's obfuscation. The Amnezia client's rule is
// all-or-nothing on nine fields (Jc/Jmin/Jmax/S1/S2/H1..H4), so there are three
// cases and not two. The header magics are the discriminator: the numerics are
// legitimately zero, but an empty H means the peer is not AmneziaWG.
func obfState(o Obfuscation) (full, partial bool) {
	if o.H1 != "" && o.H2 != "" && o.H3 != "" && o.H4 != "" {
		return true, false
	}
	any := o.H1 != "" || o.H2 != "" || o.H3 != "" || o.H4 != "" ||
		o.Jc != 0 || o.Jmin != 0 || o.Jmax != 0 ||
		o.S1 != 0 || o.S2 != 0 || o.S3 != 0 || o.S4 != 0
	return false, any
}

// missingHeaderFields lists the empty header magics, for the error message.
func missingHeaderFields(o Obfuscation) []string {
	var out []string
	for _, f := range []struct{ name, val string }{
		{"H1", o.H1}, {"H2", o.H2}, {"H3", o.H3}, {"H4", o.H4},
	} {
		if f.val == "" {
			out = append(out, f.name)
		}
	}
	return out
}

// hasAwg3 reports whether any awg3-only parameter is set. These reach the client
// only through the awg container.
func hasAwg3(o Obfuscation, headerKey string) bool {
	return headerKey != "" || o.CPA != "" || o.RAT != "" || o.RekeyTimeout != "" ||
		o.RejectAfterTime != "" || o.KeepaliveTimeout != "" || o.MaxHandshakeAttempts != ""
}

// AmneziaLink renders a peer as a vpn:// link for the Amnezia client. peerName is
// used for the server label shown in the app; it is not otherwise interpreted.
func AmneziaLink(c ClientConf, peerName string) (string, error) {
	conf, err := BuildClient(c)
	if err != nil {
		return "", err
	}
	host, portStr, err := net.SplitHostPort(c.Endpoint)
	if err != nil {
		return "", fmt.Errorf("endpoint %q: %w", c.Endpoint, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("endpoint %q: port is not a number", c.Endpoint)
	}
	// The desktop daemon writes endpoint=<host>:<port> unbracketed, so an IPv6
	// literal is malformed there and fails with no error anywhere.
	if a, perr := netip.ParseAddr(host); perr == nil && a.Is6() {
		return "", fmt.Errorf("%w: the server address is an IPv6 literal, which the "+
			"Amnezia client cannot use — set a hostname or an IPv4 address on the AWG page",
			ErrLinkUnrepresentable)
	}

	full, partial := obfState(c.Obf)
	if !full {
		switch {
		case partial:
			return "", fmt.Errorf("%w: obfuscation is only partly configured (%s empty) — the "+
				"client requires every one of the nine obfuscation fields, or none of them",
				ErrLinkUnrepresentable, strings.Join(missingHeaderFields(c.Obf), ", "))
		case hasAwg3(c.Obf, c.HeaderProtectionKey):
			return "", fmt.Errorf("%w: a header protection key is set but obfuscation is off — "+
				"the client can only carry awg3 parameters on a fully obfuscated peer",
				ErrLinkUnrepresentable)
		}
	}

	// client_ip is the IPv4 prefix ALONE: Linux and macOS feed it to
	// QHostAddress::parseSubnet, which rejects the comma-joined dual-stack form and
	// then configures the interface with an uninitialised address. allowed_ips must
	// then drop ::/0 too — matching the client's exact ["0.0.0.0/0","::/0"] literal
	// would route IPv6 into a tunnel that has no IPv6 address. The full line and
	// both ranges survive inside the embedded .conf text.
	last := map[string]any{
		"config":          conf,
		"hostName":        host,
		"port":            port, // NUMBER here; string in the outer object
		"client_ip":       c.Address,
		"client_priv_key": c.PrivateKey,
		"server_pub_key":  c.ServerPub,
		"allowed_ips":     []string{"0.0.0.0/0"},
	}
	if c.PSK != "" {
		last["psk_key"] = c.PSK
	}
	if c.MTU > 0 {
		// Overwritten by the client with its platform default; emitted anyway so the
		// object is complete if they ever stop doing that.
		last["mtu"] = strconv.Itoa(c.MTU)
	}
	if c.Keepalive != "" && c.Keepalive != "0" {
		// A string on their side already, so an AWG 3.0 "lo-hi" range passes through
		// as typed — the embedded .conf carries the same value.
		last["persistent_keep_alive"] = c.Keepalive
	}

	proto, containerName := amneziaProtoWG, amneziaContainerWG
	if full {
		proto, containerName = amneziaProtoAwg, amneziaContainerAwg
		o := c.Obf
		// All nine required fields, always — a zero is spelled out, never omitted.
		last["Jc"] = strconv.Itoa(o.Jc)
		last["Jmin"] = strconv.Itoa(o.Jmin)
		last["Jmax"] = strconv.Itoa(o.Jmax)
		last["S1"] = strconv.Itoa(o.S1)
		last["S2"] = strconv.Itoa(o.S2)
		last["H1"], last["H2"], last["H3"], last["H4"] = o.H1, o.H2, o.H3, o.H4
		// Optional on their side: a zero means "not configured".
		if o.S3 != 0 {
			last["S3"] = strconv.Itoa(o.S3)
		}
		if o.S4 != 0 {
			last["S4"] = strconv.Itoa(o.S4)
		}
		for k, v := range map[string]string{
			"I1": c.Mimic.I1, "I2": c.Mimic.I2, "I3": c.Mimic.I3,
			"I4": c.Mimic.I4, "I5": c.Mimic.I5,
			"HeaderProtectionKey":    c.HeaderProtectionKey,
			"ContentPaddingAddition": o.CPA,
			"RekeyAfterTime":         o.RAT,
			"RekeyTimeout":           o.RekeyTimeout,
			"RejectAfterTime":        o.RejectAfterTime,
			"KeepaliveTimeout":       o.KeepaliveTimeout,
			"MaxHandshakeAttempts":   o.MaxHandshakeAttempts,
		} {
			if v != "" {
				last[k] = v
			}
		}
	}

	lastJSON, err := json.Marshal(last)
	if err != nil {
		return "", err
	}

	// isThirdPartyConfig is load-bearing: it makes configTypeFromJson return
	// ConfigType::Native. Without it addServer re-derives a different type and
	// silently returns, so the import appears to succeed and nothing is added.
	// isObfuscationEnabled is deliberately ABSENT: Awg.kt sets the protocol
	// extension unconditionally, and the flag is read only on the plain-WireGuard
	// path (Wireguard.kt:109).
	inner := map[string]any{
		"last_config":        string(lastJSON), // a STRING, not a nested object
		"isThirdPartyConfig": true,
		"port":               portStr, // STRING here; number inside last_config
		"transport_proto":    "udp",
	}
	if full {
		inner["protocol_version"] = amneziaProtocolVersion
	}

	description := host
	if peerName != "" {
		description = peerName + " — " + host
	}
	outer := map[string]any{
		"containers": []any{map[string]any{
			"container": containerName,
			proto:       inner,
		}},
		"defaultContainer": containerName,
		"description":      description,
		"hostName":         host,
	}
	// The client discards a non-IPv4 entry and substitutes its own default, so
	// there is nothing to gain by emitting more than two.
	if len(c.DNS) > 0 {
		outer["dns1"] = c.DNS[0]
	}
	if len(c.DNS) > 1 {
		outer["dns2"] = c.DNS[1]
	}

	payload, err := json.Marshal(outer)
	if err != nil {
		return "", err
	}
	return "vpn://" + base64.RawURLEncoding.EncodeToString(qCompress(payload)), nil
}
