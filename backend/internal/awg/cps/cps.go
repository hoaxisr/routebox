// Package cps builds AmneziaWG "Custom Protocol Signature" client fields (I1-I5)
// that mimic real protocols, so client handshakes look like ordinary
// DNS/TLS/SIP traffic to a DPI. All output is machine-generated tag text emitted
// verbatim into a client .conf (never the server's root-shell conf).
package cps

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// Set is the client-side CPS for one profile. Zero value = no mimicry emitted.
type Set struct {
	I1, I2, I3, I4, I5 string
}

// Mimic returns the CPS Set for a profile ("dns"|"web"|"stealth"); any other value
// (off/custom) returns the zero Set.
func Mimic(profile string) Set {
	switch profile {
	case "dns":
		return build(mimicDNS())
	case "web":
		return build(mimicTLS())
	case "stealth":
		return build(mimicSIP())
	default:
		return Set{}
	}
}

// build wraps a real first packet as I1 and fills I2-I5 with entropy tags. I1 is
// the protocol snapshot; I2-I5 raise entropy so the chain is not a static
// fingerprint.
func build(i1 []byte) Set {
	return Set{
		I1: bTag(i1),
		I2: fmt.Sprintf("<r %d>", randInt(16, 48)),
		I3: fmt.Sprintf("<r %d><t>", randInt(8, 24)),
		I4: fmt.Sprintf("<r %d>", randInt(24, 64)),
		I5: fmt.Sprintf("<t><r %d>", randInt(8, 16)),
	}
}

func bTag(b []byte) string { return "<b 0x" + hex.EncodeToString(b) + ">" }

// randInt returns an inclusive random int in [lo, hi].
func randInt(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	return lo + int(n.Int64())
}

func randBytes(n int) []byte { b := make([]byte, n); _, _ = rand.Read(b); return b }

func pick(pool []string) string { return pool[randInt(0, len(pool)-1)] }

var domainPool = []string{"www.icloud.com", "www.microsoft.com", "update.googleapis.com", "cdn.cloudflare.net", "s3.amazonaws.com"}

// mimicDNS builds a standard recursive DNS query: random TXID, RD flag, one
// question (random domain, A), and an EDNS0 OPT record in additional.
func mimicDNS() []byte {
	buf := make([]byte, 0, 64)
	buf = append(buf, randBytes(2)...) // ID (random TXID)
	buf = append(buf, 0x01, 0x00)      // flags: standard query, RD=1
	buf = append(buf, 0x00, 0x01)      // QDCOUNT=1
	buf = append(buf, 0x00, 0x00)      // ANCOUNT=0
	buf = append(buf, 0x00, 0x00)      // NSCOUNT=0
	buf = append(buf, 0x00, 0x01)      // ARCOUNT=1 (EDNS0)
	for _, label := range strings.Split(pick(domainPool), ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0x00)       // root label
	buf = append(buf, 0x00, 0x01) // QTYPE=A
	buf = append(buf, 0x00, 0x01) // QCLASS=IN
	// EDNS0 OPT pseudo-record: name=root, type=41, UDP size 4096, no flags/rdata
	buf = append(buf, 0x00, 0x00, 0x29, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	return buf
}

func hexID(n int) string { return hex.EncodeToString(randBytes(n)) }

var sipDomainPool = []string{"sip.example.com", "voip.provider.net", "pbx.telco.io"}

// mimicSIP builds a plausible SIP REGISTER request (CRLF-terminated headers) with
// random Call-ID, branch, and tag so it is not a static signature.
func mimicSIP() []byte {
	domain := pick(sipDomainPool)
	branch := "z9hG4bK" + hexID(6)
	user := fmt.Sprintf("user%d", randInt(1000, 9999))
	localIP := fmt.Sprintf("192.168.%d.%d", randInt(0, 255), randInt(2, 254))
	msg := strings.Join([]string{
		fmt.Sprintf("REGISTER sip:%s SIP/2.0", domain),
		fmt.Sprintf("Via: SIP/2.0/UDP %s:5060;branch=%s;rport", localIP, branch),
		"Max-Forwards: 70",
		fmt.Sprintf("From: <sip:%s@%s>;tag=%s", user, domain, hexID(4)),
		fmt.Sprintf("To: <sip:%s@%s>", user, domain),
		fmt.Sprintf("Call-ID: %s@%s", hexID(8), localIP),
		fmt.Sprintf("CSeq: %d REGISTER", randInt(1, 9999)),
		fmt.Sprintf("Contact: <sip:%s@%s:5060>", user, localIP),
		"User-Agent: Linphone/5.2.0",
		"Expires: 3600",
		"Content-Length: 0",
		"", "",
	}, "\r\n")
	return []byte(msg)
}

// GREASE values (RFC 8701) — Chrome sprinkles these in cipher/extension/group lists.
var greasePool = []uint16{0x0a0a, 0x1a1a, 0x2a2a, 0x3a3a, 0x4a4a, 0x5a5a, 0x6a6a, 0x7a7a,
	0x8a8a, 0x9a9a, 0xaaaa, 0xbaba, 0xcaca, 0xdada, 0xeaea, 0xfafa}

func grease() uint16 { return greasePool[randInt(0, len(greasePool)-1)] }

func u16(v uint16) []byte { return []byte{byte(v >> 8), byte(v)} }

// mimicTLS builds a Chrome-like TLS 1.3 ClientHello inside a TLS record. The SNI host
// is random (from the pool) so the packet is not a static signature; cipher/extension
// order follows Chrome. (verify-at-impl: diff against a real captured Chrome
// ClientHello and align byte-for-byte where practical.)
func mimicTLS() []byte {
	host := pick(domainPool)

	// --- ClientHello body ---
	ch := make([]byte, 0, 256)
	ch = append(ch, 0x03, 0x03)       // legacy_version TLS 1.2
	ch = append(ch, randBytes(32)...) // random
	sid := randBytes(32)
	ch = append(ch, byte(len(sid))) // session id len
	ch = append(ch, sid...)

	// cipher suites: GREASE + a Chrome-ish set
	ciphers := []uint16{grease(), 0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030,
		0xcca9, 0xcca8, 0xc013, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035}
	cb := []byte{}
	for _, c := range ciphers {
		cb = append(cb, u16(c)...)
	}
	ch = append(ch, u16(uint16(len(cb)))...)
	ch = append(ch, cb...)
	ch = append(ch, 0x01, 0x00) // compression: 1 method, null

	// --- extensions ---
	ext := []byte{}
	addExt := func(typ uint16, data []byte) {
		ext = append(ext, u16(typ)...)
		ext = append(ext, u16(uint16(len(data)))...)
		ext = append(ext, data...)
	}
	addExt(grease(), nil) // GREASE ext, empty
	// SNI (type 0): server_name_list -> host_name(0) -> host
	sni := append([]byte{0x00}, u16(uint16(len(host)))...)
	sni = append(sni, []byte(host)...)
	sniList := append(u16(uint16(len(sni))), sni...)
	addExt(0x0000, sniList)
	// supported_groups (10): GREASE + x25519, secp256r1, secp384r1
	groups := []uint16{grease(), 0x001d, 0x0017, 0x0018}
	gb := []byte{}
	for _, g := range groups {
		gb = append(gb, u16(g)...)
	}
	addExt(0x000a, append(u16(uint16(len(gb))), gb...))
	// ALPN (16): h2, http/1.1
	alpn := []byte{0x08, 'h', '2', 0x08, 'h', 't', 't', 'p', '/', '1', '.', '1'}
	addExt(0x0010, append(u16(uint16(len(alpn))), alpn...))
	// supported_versions (43): GREASE + TLS 1.3 + 1.2
	sv := []byte{0x06}
	sv = append(sv, u16(grease())...)
	sv = append(sv, u16(0x0304)...)
	sv = append(sv, u16(0x0303)...)
	addExt(0x002b, sv)

	ch = append(ch, u16(uint16(len(ext)))...)
	ch = append(ch, ext...)

	// --- handshake header (type 1, 3-byte length) ---
	hs := []byte{0x01, byte(len(ch) >> 16), byte(len(ch) >> 8), byte(len(ch))}
	hs = append(hs, ch...)

	// --- TLS record header (type 22, version 0x0301, 2-byte length) ---
	rec := []byte{0x16, 0x03, 0x01, byte(len(hs) >> 8), byte(len(hs))}
	rec = append(rec, hs...)
	return rec
}
