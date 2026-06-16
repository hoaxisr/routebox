// Package cps builds AmneziaWG "Custom Protocol Signature" client fields (I1-I5 +
// Itime) that mimic real protocols, so client handshakes look like ordinary
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
	Itime              int
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

// build wraps a real first packet as I1 and fills I2-I5 with entropy tags + a
// randomised Itime. I1 is the protocol snapshot; I2-I5 raise entropy so the chain
// is not a static fingerprint.
func build(i1 []byte) Set {
	return Set{
		Itime: randInt(4, 15),
		I1:    bTag(i1),
		I2:    fmt.Sprintf("<r %d>", randInt(16, 48)),
		I3:    fmt.Sprintf("<r %d><t>", randInt(8, 24)),
		I4:    fmt.Sprintf("<r %d>", randInt(24, 64)),
		I5:    fmt.Sprintf("<t><r %d>", randInt(8, 16)),
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

// TEMPORARY stubs so the package compiles; replaced by tasks B2 (TLS) and B3 (SIP).
func mimicTLS() []byte { return randBytes(64) }
func mimicSIP() []byte { return randBytes(64) }
